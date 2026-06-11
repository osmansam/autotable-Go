package middlewares

import (
	"time"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/observability"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func PrometheusMetrics() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Path() == "/metrics" {
			return c.Next()
		}

		start := time.Now()
		err := c.Next()
		statusCode := c.Response().StatusCode()
		if err != nil && statusCode < fiber.StatusBadRequest {
			statusCode = fiber.StatusInternalServerError
		}

		observability.RecordHTTPRequest(c.Method(), routePath(c), statusCode, time.Since(start))
		return err
	}
}

func PrometheusHandler() fiber.Handler {
	return adaptor.HTTPHandler(promhttp.HandlerFor(
		observability.Registry,
		promhttp.HandlerOpts{},
	))
}
