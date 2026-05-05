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
	"sync"

	"github.com/abilfida/openai-compatible-codebuddy/internal/types"
)

type CLIOptions struct {
	Model          string
	FallbackModel  string
	MaxTurns       int
	PermissionMode string
	AllowedTools   []string
	SystemPrompt   string
	IncludePartial bool
	Env            map[string]string
}

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

func NewCLIProcess(opts CLIOptions) (*CLIProcess, error) {
	ctx, cancel := context.WithCancel(context.Background())

	args := buildCLIArgs(opts)
	execPath := resolveCLIPath()
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
		ctx:     ctx,
		cancel:  cancel,
	}

	go p.readMessages()

	return p, nil
}

func (p *CLIProcess) SendUserMessage(prompt string, blocks []types.CLIContentBlock) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return fmt.Errorf("process closed")
	}

	msg := types.CLIUserMessage{
		Type:           "user",
		SessionID:      "",
		ParentToolUseID: "",
	}

	if len(blocks) > 0 {
		msg.Message = types.CLIInnerMessage{
			Role:    "user",
			Content: mustMarshal(blocks),
		}
	} else {
		msg.Message = types.CLIInnerMessage{
			Role:    "user",
			Content: mustMarshal(prompt),
		}
	}

	line := string(mustMarshal(msg)) + "\n"
	_, err := p.stdin.Write([]byte(line))
	return err
}

func (p *CLIProcess) Messages() <-chan types.CLIMessage {
	return p.msgChan
}

func (p *CLIProcess) Errors() <-chan error {
	return p.errChan
}

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

func (p *CLIProcess) readMessages() {
	for p.scanner.Scan() {
		line := p.scanner.Text()
		if line == "" {
			continue
		}

		var msg types.CLIMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // Skip non-JSON lines
		}

		p.msgChan <- msg
	}

	if err := p.scanner.Err(); err != nil {
		p.errChan <- err
	}
}

func buildCLIArgs(opts CLIOptions) []string {
	args := []string{
		"--output-format", "stream-json",
		"--verbose",
		"--input-format", "stream-json",
		"--setting-sources", "none",
	}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.FallbackModel != "" {
		args = append(args, "--fallback-model", opts.FallbackModel)
	}
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", opts.MaxTurns))
	}
	if opts.PermissionMode != "" {
		args = append(args, "--permission-mode", opts.PermissionMode)
	}
	if len(opts.AllowedTools) == 0 {
		args = append(args, "--allowedTools", "")
	} else {
		args = append(args, "--allowedTools", joinTools(opts.AllowedTools))
	}
	if opts.SystemPrompt != "" {
		args = append(args, "--system-prompt", opts.SystemPrompt)
	}
	if opts.IncludePartial {
		args = append(args, "--include-partial-messages")
	}

	return args
}

func resolveCLIPath() string {
	if path := os.Getenv("CODEBUDDY_CODE_PATH"); path != "" {
		return path
	}
	return "codebuddy"
}

func buildEnv(extra map[string]string) []string {
	env := []string{"CODEBUDDY_CODE_ENTRYPOINT=sdk-go"}
	for k, v := range extra {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

func joinTools(tools []string) string {
	result := ""
	for i, t := range tools {
		if i > 0 {
			result += ","
		}
		result += t
	}
	return result
}

func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return json.RawMessage(data)
}