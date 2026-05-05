// go/internal/routes/chat.go
package routes

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/abilfida/openai-compatible-codebuddy/internal/config"
	"github.com/abilfida/openai-compatible-codebuddy/internal/services"
	"github.com/abilfida/openai-compatible-codebuddy/internal/types"
)

var responseCache *services.LRUCache[string, types.ChatCompletionResponse]

func ChatHandler(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		initCache(cfg)

		var req types.ChatCompletionRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(types.ErrorResponse{
				Error: types.ErrorDetail{
					Message: "Invalid request body",
					Type:    "invalid_request_error",
				},
			})
		}

		apiKey := getAPIKey(c, cfg)
		model := req.Model
		if model == "" {
			model = cfg.DefaultModel
		}

		converted := services.ConvertMessages(req.Messages)
		systemPrompt := converted.SystemPrompt
		if systemPrompt == "" {
			systemPrompt = services.DefaultSystemPrompt
		}

		// Check cache for non-streaming
		if !req.Stream && cfg.Cache.Enabled {
			cacheKey := buildCacheKey(model, req.Messages)
			resp, ok := responseCache.Get(cacheKey)
			if ok {
				c.Set("X-Cache", "HIT")
				return c.JSON(resp)
			}
		}

		if req.Stream {
			return handleStreamChat(c, cfg, req, model, systemPrompt, apiKey, converted)
		}

		return handleNonStreamChat(c, cfg, req, model, systemPrompt, apiKey, converted)
	}
}

func handleNonStreamChat(c *fiber.Ctx, cfg *config.Config, req types.ChatCompletionRequest,
	model, systemPrompt, apiKey string, converted services.ConvertedPrompt) error {

	opts := services.CLIOptions{
		Model:          model,
		FallbackModel:  cfg.FallbackModel,
		MaxTurns:       1,
		PermissionMode: "bypassPermissions",
		AllowedTools:   []string{},
		SystemPrompt:   systemPrompt,
		Env:            map[string]string{"CODEBUDDY_API_KEY": apiKey},
	}

	cli, err := services.NewCLIProcess(opts)
	if err != nil {
		return c.Status(500).JSON(types.ErrorResponse{
			Error: types.ErrorDetail{
				Message: err.Error(),
				Type:    "server_error",
			},
		})
	}
	defer cli.Close()

	if err := cli.SendUserMessage(converted.Prompt, nil); err != nil {
		return c.Status(500).JSON(types.ErrorResponse{
			Error: types.ErrorDetail{
				Message: err.Error(),
				Type:    "server_error",
			},
		})
	}

	var fullText string
	var usage *types.CLIUsage
	var actualModel = model

	for msg := range cli.Messages() {
		switch msg.Type {
		case "result":
			if msg.Result != nil {
				fullText = msg.Result.Text
			}
			if msg.Usage != nil {
				usage = msg.Usage
			}
		}
	}

	resp := services.FormatChatCompletion(fullText, actualModel, usage)

	// Cache result
	if cfg.Cache.Enabled {
		cacheKey := buildCacheKey(model, req.Messages)
		responseCache.Set(cacheKey, *resp)
	}

	c.Set("X-Cache", "MISS")
	return c.JSON(resp)
}

func handleStreamChat(c *fiber.Ctx, cfg *config.Config, req types.ChatCompletionRequest,
	model, systemPrompt, apiKey string, converted services.ConvertedPrompt) error {

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	opts := services.CLIOptions{
		Model:          model,
		FallbackModel:  cfg.FallbackModel,
		MaxTurns:       1,
		PermissionMode: "bypassPermissions",
		AllowedTools:   []string{},
		SystemPrompt:   systemPrompt,
		IncludePartial: true,
		Env:            map[string]string{"CODEBUDDY_API_KEY": apiKey},
	}

	cli, err := services.NewCLIProcess(opts)
	if err != nil {
		return c.Status(500).JSON(types.ErrorResponse{
			Error: types.ErrorDetail{
				Message: err.Error(),
				Type:    "server_error",
			},
		})
	}

	if err := cli.SendUserMessage(converted.Prompt, nil); err != nil {
		cli.Close()
		return c.Status(500).JSON(types.ErrorResponse{
			Error: types.ErrorDetail{
				Message: err.Error(),
				Type:    "server_error",
			},
		})
	}

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		sentRole := false
		actualModel := model

		for msg := range cli.Messages() {
			if msg.Type == "stream_event" && msg.Event != nil {
				switch msg.Event.Type {
				case "content_block_delta":
					if msg.Event.Delta != nil {
						chunk := services.FormatSSEChunk(
							msg.Event.Delta.Text,
							actualModel,
							"",
							!sentRole,
						)
						sentRole = true
						fmt.Fprintf(w, services.FormatSSE(chunk))
						w.Flush()
					}
				case "message_start":
					if msg.Event.Message != nil {
						actualModel = msg.Event.Message.Model
					}
				}
			}
		}

		// Send stop chunk
		stopChunk := services.FormatSSEChunk("", actualModel, "stop", false)
		fmt.Fprintf(w, services.FormatSSE(stopChunk))
		fmt.Fprintf(w, services.FormatSSEDone())
		w.Flush()

		cli.Close()
	})

	return nil
}

func SetupChatRoutes(app *fiber.App, cfg *config.Config) {
	app.Post("/v1/chat/completions", ChatHandler(cfg))
}

func initCache(cfg *config.Config) {
	if responseCache == nil && cfg.Cache.Enabled {
		responseCache = services.NewLRUCache[string, types.ChatCompletionResponse](
			cfg.Cache.MaxSize,
			cfg.Cache.TTL,
		)
	}
}

func buildCacheKey(model string, messages []types.ChatMessage) string {
	data, _ := json.Marshal(struct {
		Model    string
		Messages []types.ChatMessage
	}{Model: model, Messages: messages})
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func getAPIKey(c *fiber.Ctx, cfg *config.Config) string {
	// Check for X-CodeBuddy-Api-Key header first
	if key := c.Get("X-CodeBuddy-Api-Key"); key != "" {
		return key
	}
	// Fall back to Authorization header (Bearer token)
	auth := c.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	// Fall back to config default
	return cfg.CodeBuddy.APIKey
}