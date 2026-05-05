// go/internal/routes/health.go
package routes

import (
	"github.com/gofiber/fiber/v2"
)

func HealthHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "ok",
	})
}

func SetupHealthRoutes(app *fiber.App) {
	app.Get("/health", HealthHandler)
}