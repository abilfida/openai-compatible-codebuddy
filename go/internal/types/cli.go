// go/internal/types/cli.go
package types

import "encoding/json"

// UserMessage sent to CLI via stdin
type CLIUserMessage struct {
	Type           string          `json:"type"`
	SessionID      string          `json:"session_id"`
	Message        CLIInnerMessage `json:"message"`
	ParentToolUseID string          `json:"parent_tool_use_id"`
}

type CLIInnerMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// Anthropic-style content blocks for multimodal
type CLIContentBlock struct {
	Type   string          `json:"type"` // "text" or "image"
	Text   string          `json:"text,omitempty"`
	Source *CLIImageSource `json:"source,omitempty"`
}

type CLIImageSource struct {
	Type       string `json:"type"` // "base64"
	MediaType  string `json:"media_type"`
	Data       string `json:"data"`
}

// Messages received from CLI via stdout
type CLIMessage struct {
	Type   string          `json:"type"`
	Event  *CLIStreamEvent `json:"event,omitempty"`
	Result *CLIResult      `json:"result,omitempty"`
	Usage  *CLIUsage       `json:"usage,omitempty"`
}

type CLIStreamEvent struct {
	Type    string          `json:"type"` // "content_block_delta", "message_start", "message_delta"
	Index   int             `json:"index,omitempty"`
	Delta   *CLITextDelta   `json:"delta,omitempty"`
	Message *CLIAssistantMsg `json:"message,omitempty"`
}

type CLITextDelta struct {
	Type string `json:"type"` // "text_delta"
	Text string `json:"text"`
}

type CLIAssistantMsg struct {
	ID      string            `json:"id"`
	Model   string            `json:"model"`
	Role    string            `json:"role"`
	Content []CLIContentBlock `json:"content"`
	Usage   *CLIUsage         `json:"usage,omitempty"`
}

type CLIResult struct {
	Text  string    `json:"result,omitempty"`
	Usage *CLIUsage `json:"usage,omitempty"`
}

type CLIUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

// Control request/response
type CLIControlRequest struct {
	Type      string             `json:"type"`
	RequestID string             `json:"request_id"`
	Request   CLIControlPayload  `json:"request"`
}

type CLIControlPayload struct {
	Subtype string `json:"subtype"` // "get_supported_models"
}

type CLIControlResponse struct {
	Type     string                `json:"type"`
	Response CLIControlResponseBody `json:"response"`
}

type CLIControlResponseBody struct {
	Subtype   string          `json:"subtype"`
	RequestID string          `json:"request_id"`
	Response  json.RawMessage `json:"response,omitempty"`
	Error     string          `json:"error,omitempty"`
}

type CLIModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}