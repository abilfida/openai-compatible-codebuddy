// go/internal/middleware/error_handler.go
package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/abilfida/openai-compatible-codebuddy/internal/types"
)

func ErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Internal server error"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}

	return c.Status(code).JSON(types.ErrorResponse{
		Error: types.ErrorDetail{
			Message: message,
			Type:    "invalid_request_error",
		},
	})
}