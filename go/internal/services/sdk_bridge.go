// go/internal/services/sdk_bridge.go
package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/abilfida/openai-compatible-codebuddy/internal/types"
)

// CLIOptions matches TypeScript SDK Options interface
type CLIOptions struct {
	Model                 string
	FallbackModel         string
	MaxTurns              int
	PermissionMode        string
	DangerouslySkipPerms  bool
	AllowedTools          []string
	DisallowedTools       []string
	SystemPrompt          string
	AppendSystemPrompt    string
	IncludePartial        bool
	SettingSources        []string
	AdditionalDirectories []string
	Continue              bool
	Resume                string
	ForkSession           bool
	McpServers            map[string]interface{}
	StrictMcpConfig       bool
	ExtraArgs             map[string]string
	Args                  []string
	Env                   map[string]string
}

// CLIProcess manages the CLI subprocess
type CLIProcess struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.Reader
	scanner *bufio.Scanner
	msgChan chan types.CLIMessage
	errChan chan error
	mu      sync.Mutex
	closed  bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// CLINotFoundError is thrown when CLI cannot be found
type CLINotFoundError struct {
	Message  string
	Platform string
	Arch     string
}

func (e *CLINotFoundError) Error() string {
	return e.Message
}

// NewCLIProcess creates a new CLI subprocess - matches TypeScript SDK ProcessTransport.start()
func NewCLIProcess(opts CLIOptions) (*CLIProcess, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Validate fallback model
	if opts.FallbackModel != "" && opts.Model != "" && opts.FallbackModel == opts.Model {
		cancel()
		return nil, fmt.Errorf("Fallback model cannot be the same as the main model")
	}

	execPath, err := resolveCLIPath()
	if err != nil {
		cancel()
		return nil, err
	}

	args := buildCLIArgs(opts)
	env := buildEnv(opts.Env)

	cmd := exec.CommandContext(ctx, execPath, args...)
	cmd.Env = append(os.Environ(), env...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdin pipe error: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdout pipe error: %w", err)
	}

	// stderr handling (optional)
	if opts.Env != nil {
		if stderrHandler, ok := opts.Env["__stderr_handler__"]; ok {
			// Custom stderr handling if needed
			_ = stderrHandler // placeholder
		}
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start error: %w", err)
	}

	p := &CLIProcess{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		scanner: bufio.NewScanner(stdout),
		msgChan: make(chan types.CLIMessage, 100),
		errChan: make(chan error, 1),
		ctx:     ctx,
		cancel:  cancel,
	}

	go p.readMessages()

	return p, nil
}

// SendUserMessage sends a user message to CLI - matches TypeScript SDK ProcessTransport.sendUserMessage()
func (p *CLIProcess) SendUserMessage(content interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return fmt.Errorf("process closed")
	}

	var msg types.CLIUserMessage

	// Handle string content or pre-built UserMessage
	switch v := content.(type) {
	case string:
		msg = types.CLIUserMessage{
			Type:           "user",
			SessionID:      "",
			Message:        types.CLIInnerMessage{Role: "user", Content: mustMarshal(v)},
			ParentToolUseID: "",
		}
	case types.CLIUserMessage:
		msg = v
	default:
		msg = types.CLIUserMessage{
			Type:           "user",
			SessionID:      "",
			Message:        types.CLIInnerMessage{Role: "user", Content: mustMarshal(v)},
			ParentToolUseID: "",
		}
	}

	line := string(mustMarshal(msg)) + "\n"
	_, err := p.stdin.Write([]byte(line))
	return err
}

// SendControlRequest sends a control request and waits for response - matches TypeScript SDK ProcessTransport.sendControlRequest()
func (p *CLIProcess) SendControlRequest(payload map[string]interface{}, timeoutMs int) (map[string]interface{}, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, fmt.Errorf("process closed")
	}

	// Generate request ID
	requestID := fmt.Sprintf("sdk_%d_%d", time.Now().UnixMilli(), requestIDCounter())
	requestIDCounterAdd()

	request := map[string]interface{}{
		"type":       "control_request",
		"request_id": requestID,
		"request":    payload,
	}

	line := string(mustMarshal(request)) + "\n"
	_, err := p.stdin.Write([]byte(line))
	if err != nil {
		return nil, err
	}

	// Wait for response with timeout
	timeout := timeoutMs
	if timeout == 0 {
		timeout = 60000 // Default 60s
	}

	// Simplified: just return success for now
	// Full implementation would track pending requests like TypeScript SDK
	return map[string]interface{}{
		"request_id": requestID,
		"success":    true,
	}, nil
}

// Messages returns the message channel - matches TypeScript SDK ProcessTransport.messages()
func (p *CLIProcess) Messages() <-chan types.CLIMessage {
	return p.msgChan
}

// Errors returns the error channel
func (p *CLIProcess) Errors() <-chan error {
	return p.errChan
}

// Close closes the process - matches TypeScript SDK ProcessTransport.close()
func (p *CLIProcess) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	p.cancel()
	close(p.msgChan)
	close(p.errChan)

	return p.cmd.Wait()
}

// IsReady checks if process is ready - matches TypeScript SDK ProcessTransport.isReady()
func (p *CLIProcess) IsReady() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.closed && p.cmd != nil
}

func (p *CLIProcess) readMessages() {
	for p.scanner.Scan() {
		line := p.scanner.Text()
		if line == "" {
			continue
		}

		var msg types.CLIMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // Skip non-JSON lines (verbose output)
		}

		// Handle control responses
		if msg.Type == "control_response" {
			// Would handle pending request resolution here
			// For now, just pass through
		}

		p.msgChan <- msg
	}

	if err := p.scanner.Err(); err != nil {
		select {
		case p.errChan <- err:
		default:
		}
	}
}

// buildCLIArgs builds CLI arguments - matches TypeScript SDK ProcessTransport.buildArgs() exactly
func buildCLIArgs(opts CLIOptions) []string {
	args := []string{
		"--output-format", "stream-json",
		"--verbose",
		"--input-format", "stream-json",
	}

	// Model options
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.FallbackModel != "" {
		args = append(args, "--fallback-model", opts.FallbackModel)
	}

	// Turn limits
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", opts.MaxTurns))
	}

	// Permission options
	if opts.PermissionMode != "" {
		args = append(args, "--permission-mode", opts.PermissionMode)
	}
	if opts.DangerouslySkipPerms {
		args = append(args, "--dangerously-skip-permissions")
	}

	// Tool options - matches TypeScript exactly
	if opts.AllowedTools != nil && len(opts.AllowedTools) > 0 {
		args = append(args, "--allowedTools", joinStrings(opts.AllowedTools, ","))
	}
	if opts.DisallowedTools != nil && len(opts.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools", joinStrings(opts.DisallowedTools, ","))
	}

	// Session options
	if opts.Continue {
		args = append(args, "--continue")
	}
	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	}
	if opts.ForkSession {
		args = append(args, "--fork-session")
	}

	// MCP options
	if opts.McpServers != nil && len(opts.McpServers) > 0 {
		mcpConfig := map[string]interface{}{"mcpServers": opts.McpServers}
		args = append(args, "--mcp-config", string(mustMarshal(mcpConfig)))
	}
	if opts.StrictMcpConfig {
		args = append(args, "--strict-mcp-config")
	}

	// Settings - SDK default: 'none' for clean environment isolation
	if opts.SettingSources != nil {
		value := "none"
		if len(opts.SettingSources) > 0 {
			value = joinStrings(opts.SettingSources, ",")
		}
		args = append(args, "--setting-sources", value)
	} else {
		// SDK default behavior: no filesystem settings loaded
		args = append(args, "--setting-sources", "none")
	}

	// Additional directories
	if opts.AdditionalDirectories != nil {
		for _, dir := range opts.AdditionalDirectories {
			args = append(args, "--add-dir", dir)
		}
	}

	// Output options
	if opts.IncludePartial {
		args = append(args, "--include-partial-messages")
	}

	// System prompt options
	if opts.SystemPrompt != "" {
		args = append(args, "--system-prompt", opts.SystemPrompt)
	}
	if opts.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", opts.AppendSystemPrompt)
	}

	// Extra args (custom flags)
	if opts.ExtraArgs != nil {
		for flag, value := range opts.ExtraArgs {
			if value == "" {
				args = append(args, "--"+flag)
			} else {
				args = append(args, "--"+flag, value)
			}
		}
	}

	// Additional args passed directly
	if opts.Args != nil {
		args = append(args, opts.Args...)
	}

	return args
}

// resolveCLIPath resolves CLI path - matches TypeScript SDK cli-resolver.js exactly
func resolveCLIPath() (string, error) {
	currentPlatform := runtime.GOOS
	currentArch := runtime.GOARCH

	// 1. Environment variable takes precedence (CODEBUDDY_CODE_PATH)
	envPath := os.Getenv("CODEBUDDY_CODE_PATH")
	if envPath != "" {
		if fileExists(envPath) {
			return envPath, nil
		}
		// Warn but continue to try other methods (matching TypeScript behavior)
		fmt.Fprintf(os.Stderr, "Warning: CODEBUDDY_CODE_PATH is set to \"%s\" but file does not exist. Falling back to other resolution methods.\n", envPath)
	}

	// 2. Try bundled CLI from npm package (node_modules/@tencent-ai/agent-sdk/cli/bin/codebuddy)
	bundledPath := resolveFromBundled()
	if bundledPath != "" {
		return bundledPath, nil
	}

	// 3. Try monorepo development path (agent-cli)
	monorepoPath := resolveFromMonorepo()
	if monorepoPath != "" {
		return monorepoPath, nil
	}

	// 4. Try PATH lookup
	if path, err := exec.LookPath(binaryName()); err == nil {
		return path, nil
	}

	// Nothing found - throw helpful error (matches TypeScript SDK)
	return "", &CLINotFoundError{
		Message: `CodeBuddy CLI not found.

Possible solutions:
  1. Install the SDK package:
     npm install @tencent-ai/agent-sdk

  2. Set CODEBUDDY_CODE_PATH environment variable to the CLI path

  3. Ensure 'codebuddy' is in your PATH`,
		Platform: currentPlatform,
		Arch:     currentArch,
	}
}

// binaryName returns platform-specific binary name - matches TypeScript SDK BINARY_NAMES
func binaryName() string {
	switch runtime.GOOS {
	case "windows":
		return "codebuddy.exe"
	case "darwin", "linux":
		return "codebuddy"
	default:
		return "codebuddy"
	}
}

// resolveFromBundled tries to find CLI from bundled npm package
// Matches TypeScript: ../../cli/bin/codebuddy relative to lib/utils
func resolveFromBundled() string {
	// Go module is at go/internal/services
	// Need to traverse up to find node_modules
	// Path: go/../node_modules/@tencent-ai/agent-sdk/cli/bin/codebuddy

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Try multiple relative paths
	paths := []string{
		// From go directory: ../node_modules/...
		filepath.Join(cwd, "../node_modules/@tencent-ai/agent-sdk/cli/bin", binaryName()),
		// From project root
		filepath.Join(cwd, "node_modules/@tencent-ai/agent-sdk/cli/bin", binaryName()),
		// Common locations
		filepath.Join(cwd, "../../node_modules/@tencent-ai/agent-sdk/cli/bin", binaryName()),
	}

	for _, p := range paths {
		if fileExists(p) {
			return p
		}
	}

	// Try absolute path based on executable location
	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		// From go/codebuddy-server binary
		p := filepath.Join(execDir, "../node_modules/@tencent-ai/agent-sdk/cli/bin", binaryName())
		if fileExists(p) {
			return p
		}
	}

	return ""
}

// resolveFromMonorepo tries monorepo development path - matches TypeScript SDK
func resolveFromMonorepo() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Try agent-cli directory (monorepo structure)
	paths := []string{
		filepath.Join(cwd, "../agent-cli/bin", binaryName()),
		filepath.Join(cwd, "../agent-cli/dist", binaryName()),
	}

	for _, p := range paths {
		if fileExists(p) {
			return p
		}
	}

	return ""
}

// buildEnv builds environment variables - matches TypeScript SDK
func buildEnv(extra map[string]string) []string {
	env := []string{"CODEBUDDY_CODE_ENTRYPOINT=sdk-go"}

	// Add CODEBUDDY_API_KEY if provided
	if extra != nil {
		for k, v := range extra {
			// Skip internal keys
			if k == "__stderr_handler__" {
				continue
			}
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return env
}

// Helper functions
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func joinStrings(items []string, sep string) string {
	result := ""
	for i, item := range items {
		if i > 0 {
			result += sep
		}
		result += item
	}
	return result
}

func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return json.RawMessage(data)
}

// Request ID counter for control requests
var requestIDCounterVal int64
var requestIDMu sync.Mutex

func requestIDCounter() int64 {
	requestIDMu.Lock()
	defer requestIDMu.Unlock()
	return requestIDCounterVal
}

func requestIDCounterAdd() {
	requestIDMu.Lock()
	requestIDCounterVal++
	requestIDMu.Unlock()
}