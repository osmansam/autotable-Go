package middlewares

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func shouldSkipObservability(c *fiber.Ctx) bool {
	switch c.Path() {
	case "/metrics", "/favicon.ico":
		return true
	default:
		return false
	}
}

func observedStatusCode(c *fiber.Ctx, err error) int {
	statusCode := c.Response().StatusCode()
	if err == nil {
		return statusCode
	}

	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return fiberErr.Code
	}

	if statusCode >= fiber.StatusBadRequest {
		return statusCode
	}

	return fiber.StatusInternalServerError
}

func stableString(value string) string {
	return strings.Clone(value)
}
