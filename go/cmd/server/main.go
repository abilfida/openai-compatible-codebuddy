// go/cmd/server/main.go
package main

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"

	"github.com/abilfida/openai-compatible-codebuddy/internal/config"
	"github.com/abilfida/openai-compatible-codebuddy/internal/middleware"
	"github.com/abilfida/openai-compatible-codebuddy/internal/routes"
)

func main() {
	cfg := config.Load()

	app := fiber.New(fiber.Config{
		ErrorHandler: middleware.ErrorHandler,
	})

	// Middleware
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,OPTIONS",
		AllowHeaders: "Content-Type,X-CodeBuddy-Api-Key,Authorization",
	}))
	app.Use(middleware.Logger(cfg))

	// Routes
	routes.SetupHealthRoutes(app)
	routes.SetupModelsRoutes(app, cfg)
	routes.SetupChatRoutes(app, cfg)

	// 404 handler
	app.Use(func(c *fiber.Ctx) error {
		return c.Status(404).JSON(fiber.Map{
			"error": fiber.Map{
				"message": fmt.Sprintf("Not Found: %s %s", c.Method(), c.Path()),
				"type":    "invalid_request_error",
			},
		})
	})

	// Startup banner
	log.Printf(`
╔══════════════════════════════════════════╗
║  OpenAI Compatible API Server (Go)       ║
║  Powered by CodeBuddy CLI                ║
╚══════════════════════════════════════════╝

  → Listening on http://%s:%d
  → Default model: %s
  → Cache: %s

  Endpoints:
    POST /v1/chat/completions
    GET  /v1/models
    GET  /v1/models/:model
    GET  /health
`,
		cfg.Host, cfg.Port,
		cfg.DefaultModel,
		cacheStatus(cfg),
	)

	if err := app.Listen(fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)); err != nil {
		log.Fatal(err)
	}
}

func cacheStatus(cfg *config.Config) string {
	if cfg.Cache.Enabled {
		return fmt.Sprintf("enabled (TTL=%v, max=%d)", cfg.Cache.TTL, cfg.Cache.MaxSize)
	}
	return "disabled"
}