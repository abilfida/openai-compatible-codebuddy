// go/internal/services/message_converter.go
package services

import (
	"encoding/json"
	"strings"

	"github.com/abilfida/openai-compatible-codebuddy/internal/types"
)

type ConvertedPrompt struct {
	SystemPrompt  string
	Prompt        string
	ContentBlocks []types.CLIContentBlock
}

const DefaultSystemPrompt = "You are Claude, a model from Anthropic. You are accessed through Claude Code — Anthropic's official CLI for Claude. When asked about your identity, say you are Claude from Anthropic. Respond to user messages directly."

func ConvertMessages(messages []types.ChatMessage) ConvertedPrompt {
	var systemParts []string
	var conversationParts []string

	for _, msg := range messages {
		content := contentToString(msg.Content)

		switch msg.Role {
		case "system":
			systemParts = append(systemParts, content)
		case "user":
			conversationParts = append(conversationParts, content)
		case "assistant":
			conversationParts = append(conversationParts, "[Assistant]: "+content)
		}
	}

	result := ConvertedPrompt{
		SystemPrompt: strings.Join(systemParts, "\n\n"),
	}

	hasAssistant := false
	for _, msg := range messages {
		if msg.Role == "assistant" {
			hasAssistant = true
			break
		}
	}

	if !hasAssistant && len(conversationParts) == 1 {
		result.Prompt = conversationParts[0]
	} else {
		var parts []string
		for _, msg := range messages {
			if msg.Role == "system" {
				continue
			}
			content := contentToString(msg.Content)
			switch msg.Role {
			case "user":
				parts = append(parts, "[User]: "+content)
			case "assistant":
				parts = append(parts, "[Assistant]: "+content)
			}
		}
		result.Prompt = strings.Join(parts, "\n\n")
	}

	return result
}

func contentToString(content json.RawMessage) string {
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return s
	}

	var parts []types.ContentPart
	if err := json.Unmarshal(content, &parts); err == nil {
		var texts []string
		for _, p := range parts {
			if p.Type == "text" {
				texts = append(texts, p.Text)
			} else if p.Type == "image_url" {
				texts = append(texts, "[image]")
			}
		}
		return strings.Join(texts, "\n")
	}

	return string(content)
}