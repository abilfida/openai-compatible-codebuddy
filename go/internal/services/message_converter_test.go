// go/internal/services/message_converter_test.go
package services

import (
	"encoding/json"
	"testing"

	"github.com/abilfida/openai-compatible-codebuddy/internal/types"
)

func TestConvertMessagesSimple(t *testing.T) {
	messages := []types.ChatMessage{
		{Role: "user", Content: json.RawMessage(`"Hello"`)},
	}

	result := ConvertMessages(messages)

	if result.SystemPrompt != "" {
		t.Errorf("expected empty system prompt, got %s", result.SystemPrompt)
	}
	if result.Prompt != "Hello" {
		t.Errorf("expected Hello, got %s", result.Prompt)
	}
}

func TestConvertMessagesWithSystem(t *testing.T) {
	messages := []types.ChatMessage{
		{Role: "system", Content: json.RawMessage(`"You are helpful"`)},
		{Role: "user", Content: json.RawMessage(`"Hello"`)},
	}

	result := ConvertMessages(messages)

	if result.SystemPrompt != "You are helpful" {
		t.Errorf("expected 'You are helpful', got %s", result.SystemPrompt)
	}
	if result.Prompt != "Hello" {
		t.Errorf("expected Hello, got %s", result.Prompt)
	}
}

func TestConvertMessagesMultiTurn(t *testing.T) {
	messages := []types.ChatMessage{
		{Role: "user", Content: json.RawMessage(`"What is Go?"`)},
		{Role: "assistant", Content: json.RawMessage(`"Go is a language"`)},
		{Role: "user", Content: json.RawMessage(`"Tell me more"`)},
	}

	result := ConvertMessages(messages)

	expected := "[User]: What is Go?\n\n[Assistant]: Go is a language\n\n[User]: Tell me more"
	if result.Prompt != expected {
		t.Errorf("expected %s, got %s", expected, result.Prompt)
	}
}