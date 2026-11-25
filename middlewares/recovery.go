package middlewares

import (
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/internal/panicguard"
)

func PanicRecovery() fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := panicguard.Run("http-request", c.Next)
		if errors.Is(err, panicguard.ErrPanic) {
			c.Response().ResetBody()
			return fiber.ErrInternalServerError
		}
		return err
	}
}
