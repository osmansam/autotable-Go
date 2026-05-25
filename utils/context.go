package utils

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
)

func RequestContextWithTimeout(c *fiber.Ctx, timeout time.Duration) (context.Context, context.CancelFunc) {
	baseCtx := c.UserContext()
	if baseCtx == nil {
		baseCtx = context.Background()
	}

	return context.WithTimeout(baseCtx, timeout)
}
