package middlewares

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/observability"
)

func RequestLogger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if shouldSkipObservability(c) {
			return c.Next()
		}

		start := time.Now()
		err := c.Next()
		duration := time.Since(start)
		statusCode := observedStatusCode(c, err)

		attrs := []slog.Attr{
			slog.String("method", stableString(c.Method())),
			slog.String("path", stableString(c.Path())),
			slog.String("route", routePath(c)),
			slog.String(observability.FieldStatus, strconv.Itoa(statusCode)),
			slog.Int("status_code", statusCode),
			slog.Float64(observability.FieldDurationMS, float64(duration.Microseconds())/1000),
		}

		if err != nil {
			if statusCode >= fiber.StatusInternalServerError {
				observability.Error(c, "http request completed with error", err, attrs...)
			} else {
				observability.Warn(c, "http request completed with client error", append(attrs, slog.String(observability.FieldError, err.Error()))...)
			}
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
	return stableString(c.Route().Path)
}
