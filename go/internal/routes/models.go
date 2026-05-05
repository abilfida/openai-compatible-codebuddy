// go/internal/routes/models.go
package routes

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/abilfida/openai-compatible-codebuddy/internal/config"
	"github.com/abilfida/openai-compatible-codebuddy/internal/services"
	"github.com/abilfida/openai-compatible-codebuddy/internal/types"
)

var (
	modelsCache     *types.ModelListResponse
	modelsCacheTime time.Time
	modelsCacheMu   sync.RWMutex
	modelsCacheTTL  = 5 * time.Minute
)

func ModelsHandler(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		apiKey := getAPIKey(c, cfg)

		modelsCacheMu.RLock()
		if modelsCache != nil && time.Since(modelsCacheTime) < modelsCacheTTL {
			modelsCacheMu.RUnlock()
			return c.JSON(modelsCache)
		}
		modelsCacheMu.RUnlock()

		models, err := fetchModels(cfg, apiKey)
		if err != nil {
			return c.Status(500).JSON(types.ErrorResponse{
				Error: types.ErrorDetail{
					Message: err.Error(),
					Type:    "server_error",
				},
			})
		}

		modelsCacheMu.Lock()
		modelsCache = models
		modelsCacheTime = time.Now()
		modelsCacheMu.Unlock()

		return c.JSON(models)
	}
}

func ModelHandler(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		modelID := c.Params("model")
		return c.JSON(services.FormatModel(modelID))
	}
}

func SetupModelsRoutes(app *fiber.App, cfg *config.Config) {
	app.Get("/v1/models", ModelsHandler(cfg))
	app.Get("/v1/models/:model", ModelHandler(cfg))
}

func fetchModels(cfg *config.Config, apiKey string) (*types.ModelListResponse, error) {
	opts := services.CLIOptions{
		Model:          cfg.DefaultModel,
		MaxTurns:       1,
		PermissionMode: "plan",
		Env:            map[string]string{"CODEBUDDY_API_KEY": apiKey},
	}

	cli, err := services.NewCLIProcess(opts)
	if err != nil {
		// Fallback: return default model
		return services.FormatModelList([]types.CLIModelInfo{
			{ID: cfg.DefaultModel},
		}), nil
	}
	defer cli.Close()

	var models []types.CLIModelInfo
	for msg := range cli.Messages() {
		if msg.Type == "control_request" {
			models = []types.CLIModelInfo{{ID: cfg.DefaultModel}}
			break
		}
	}

	return services.FormatModelList(models), nil
}

