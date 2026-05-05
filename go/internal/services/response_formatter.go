// go/internal/services/response_formatter.go
package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/abilfida/openai-compatible-codebuddy/internal/types"
)

func FormatChatCompletion(text, model string, usage *types.CLIUsage) *types.ChatCompletionResponse {
	id := generateID()
	created := time.Now().Unix()

	return &types.ChatCompletionResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: created,
		Model:   model,
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.Message{
					Role:    "assistant",
					Content: text,
				},
				FinishReason: "stop",
			},
		},
		Usage: mapUsage(usage),
	}
}

func FormatSSEChunk(text, model string, finishReason string, includeRole bool) *types.ChatCompletionChunk {
	id := generateID()
	created := time.Now().Unix()

	delta := types.Delta{
		Content: text,
	}
	if includeRole {
		delta.Role = "assistant"
	}

	return &types.ChatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []types.ChunkChoice{
			{
				Index:        0,
				Delta:        delta,
				FinishReason: finishReason,
			},
		},
	}
}

func FormatModelList(models []types.CLIModelInfo) *types.ModelListResponse {
	data := make([]types.ModelData, len(models))
	for i, m := range models {
		data[i] = types.ModelData{
			ID:      m.ID,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "codebuddy",
		}
	}
	return &types.ModelListResponse{
		Object: "list",
		Data:   data,
	}
}

func FormatModel(id string) *types.ModelData {
	return &types.ModelData{
		ID:      id,
		Object:  "model",
		Created: time.Now().Unix(),
		OwnedBy: "codebuddy",
	}
}

func mapUsage(usage *types.CLIUsage) types.Usage {
	if usage == nil {
		return types.Usage{}
	}

	u := types.Usage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		TotalTokens:      usage.InputTokens + usage.OutputTokens,
	}

	if usage.CacheReadInputTokens > 0 {
		u.PromptTokensDetails = &types.PromptTokensDetails{
			CachedTokens: usage.CacheReadInputTokens,
		}
	}

	return u
}

func FormatSSE(chunk *types.ChatCompletionChunk) string {
	data, _ := json.Marshal(chunk)
	return fmt.Sprintf("data: %s\n\n", data)
}

func FormatSSEDone() string {
	return "data: [DONE]\n\n"
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "chatcmpl-" + hex.EncodeToString(b)
}