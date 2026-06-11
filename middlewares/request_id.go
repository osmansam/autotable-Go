package middlewares

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/osmansam/autotableGo/observability"
)

const requestIDHeader = "X-Request-ID"

func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		requestID := strings.TrimSpace(c.Get(requestIDHeader))
		if requestID == "" {
			requestID = uuid.NewString()
		}

		c.Locals(observability.LocalRequestID, requestID)
		c.Set(requestIDHeader, requestID)

		return c.Next()
	}
}
