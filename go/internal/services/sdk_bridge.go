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
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.Reader
	scanner    *bufio.Scanner
	msgChan    chan types.CLIMessage
	errChan    chan error
	done       chan struct{}      // Signal goroutine to stop
	wg         sync.WaitGroup      // Track goroutine completion
	mu         sync.Mutex
	closed     bool
	ctx        context.Context
	cancel     context.CancelFunc
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
		done:    make(chan struct{}),
		ctx:     ctx,
		cancel:  cancel,
	}

	// Start goroutine with WaitGroup tracking
	p.wg.Add(1)
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

// Messages returns the message channel - matches TypeScript SDK ProcessTransport.messages()
func (p *CLIProcess) Messages() <-chan types.CLIMessage {
	return p.msgChan
}

// Errors returns the error channel
func (p *CLIProcess) Errors() <-chan error {
	return p.errChan
}

// Close closes the process - FIXED to match TypeScript SDK ProcessTransport.close()
// Sequence: signal done → wait for goroutine → close channels → kill process
func (p *CLIProcess) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	// 1. Signal goroutine to stop (like TypeScript messageStream.done())
	close(p.done)

	// 2. Kill subprocess to unblock scanner (like TypeScript process.kill())
	p.cancel()

	// 3. Wait for goroutine to finish (prevents panic on closed channel)
	p.wg.Wait()

	// 4. Now safe to close channels (goroutine is done)
	close(p.msgChan)
	close(p.errChan)

	// 5. Wait for process to fully exit
	return p.cmd.Wait()
}

// IsReady checks if process is ready - matches TypeScript SDK ProcessTransport.isReady()
func (p *CLIProcess) IsReady() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.closed && p.cmd != nil
}

// readMessages reads from stdout and sends to channel
// Uses select with done channel to handle graceful shutdown
func (p *CLIProcess) readMessages() {
	defer p.wg.Done()

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
			continue
		}

		// Send to channel with select to check for done signal
		// This prevents panic when channel is closed during Close()
		select {
		case p.msgChan <- msg:
			// Message sent successfully
		case <-p.done:
			// Process is closing, stop reading
			return
		}
	}

	// Scanner finished (stdout EOF or error)
	if err := p.scanner.Err(); err != nil {
		select {
		case p.errChan <- err:
		case <-p.done:
			// Closing, don't send error
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
		fmt.Fprintf(os.Stderr, "Warning: CODEBUDDY_CODE_PATH is set to \"%s\" but file does not exist. Falling back to other resolution methods.\n", envPath)
	}

	// 2. Try bundled CLI from npm package
	bundledPath := resolveFromBundled()
	if bundledPath != "" {
		return bundledPath, nil
	}

	// 3. Try monorepo development path
	monorepoPath := resolveFromMonorepo()
	if monorepoPath != "" {
		return monorepoPath, nil
	}

	// 4. Try PATH lookup
	if path, err := exec.LookPath(binaryName()); err == nil {
		return path, nil
	}

	// Nothing found - throw helpful error
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

// binaryName returns platform-specific binary name
func binaryName() string {
	switch runtime.GOOS {
	case "windows":
		return "codebuddy.exe"
	default:
		return "codebuddy"
	}
}

// resolveFromBundled tries to find CLI from bundled npm package
func resolveFromBundled() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	paths := []string{
		filepath.Join(cwd, "../node_modules/@tencent-ai/agent-sdk/cli/bin", binaryName()),
		filepath.Join(cwd, "node_modules/@tencent-ai/agent-sdk/cli/bin", binaryName()),
		filepath.Join(cwd, "../../node_modules/@tencent-ai/agent-sdk/cli/bin", binaryName()),
	}

	for _, p := range paths {
		if fileExists(p) {
			return p
		}
	}

	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		p := filepath.Join(execDir, "../node_modules/@tencent-ai/agent-sdk/cli/bin", binaryName())
		if fileExists(p) {
			return p
		}
	}

	return ""
}

// resolveFromMonorepo tries monorepo development path
func resolveFromMonorepo() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

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

// buildEnv builds environment variables
func buildEnv(extra map[string]string) []string {
	env := []string{"CODEBUDDY_CODE_ENTRYPOINT=sdk-go"}

	if extra != nil {
		for k, v := range extra {
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