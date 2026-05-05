// go/internal/middleware/logger.go
package middleware

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/abilfida/openai-compatible-codebuddy/internal/config"
)

func Logger(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		latency := time.Since(start)

		status := c.Response().StatusCode()
		timestamp := start.Format(time.RFC3339)

		if cfg.Debug {
			log.Printf("[%s] %s %s %d %v\nbody=%s",
				timestamp, c.Method(), c.Path(), status, latency, string(c.Body()))
		} else {
			log.Printf("[%s] %s %s %d %v",
				timestamp, c.Method(), c.Path(), status, latency)
		}

		return err
	}
}