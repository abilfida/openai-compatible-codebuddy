// go/internal/services/response_formatter_test.go
package services

import (
	"testing"

	"github.com/abilfida/openai-compatible-codebuddy/internal/types"
)

func TestFormatChatCompletion(t *testing.T) {
	usage := &types.CLIUsage{
		InputTokens:  10,
		OutputTokens: 5,
	}

	resp := FormatChatCompletion("Hello!", "deepseek-v3.1", usage)

	if resp.Model != "deepseek-v3.1" {
		t.Errorf("expected deepseek-v3.1, got %s", resp.Model)
	}
	if len(resp.Choices) != 1 {
		t.Errorf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "Hello!" {
		t.Errorf("expected Hello!, got %s", resp.Choices[0].Message.Content)
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
}

func TestFormatSSEChunk(t *testing.T) {
	chunk := FormatSSEChunk("Hi", "deepseek-v3.1", "", true)

	if chunk.Object != "chat.completion.chunk" {
		t.Errorf("expected chat.completion.chunk, got %s", chunk.Object)
	}
	if len(chunk.Choices) != 1 {
		t.Errorf("expected 1 choice, got %d", len(chunk.Choices))
	}
	if chunk.Choices[0].Delta.Role != "assistant" {
		t.Errorf("expected assistant role, got %s", chunk.Choices[0].Delta.Role)
	}
}