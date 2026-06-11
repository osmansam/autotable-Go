package middlewares

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/observability"
)

func RequestLogger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Path() == "/metrics" {
			return c.Next()
		}

		start := time.Now()
		err := c.Next()
		duration := time.Since(start)
		statusCode := c.Response().StatusCode()
		if err != nil && statusCode < fiber.StatusBadRequest {
			statusCode = fiber.StatusInternalServerError
		}

		attrs := []slog.Attr{
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.String("route", routePath(c)),
			slog.Int("status_code", statusCode),
			slog.Float64(observability.FieldDurationMS, float64(duration.Microseconds())/1000),
		}

		if err != nil {
			observability.Error(c, "http request completed with error", err, attrs...)
			return err
		}

		observability.Info(c, "http request completed", attrs...)
		return nil
	}
}

func routePath(c *fiber.Ctx) string {
	if c == nil || c.Route() == nil || c.Route().Path == "" {
		return "unknown"
	}
	return c.Route().Path
}
