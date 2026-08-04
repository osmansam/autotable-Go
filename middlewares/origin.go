package middlewares

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func IsTrustedOrigin(origin string, allowed []string) bool {
	normalized, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}
	for _, candidate := range allowed {
		trusted, valid := normalizeOrigin(candidate)
		if valid && normalized == trusted {
			return true
		}
	}
	return false
}

func RequireTrustedCookieOrigin(allowed []string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if isSafeMethod(c.Method()) || c.Locals(AuthSourceLocalKey) != AuthSourceCookie {
			return c.Next()
		}
		if !IsTrustedOrigin(c.Get(fiber.HeaderOrigin), allowed) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Untrusted request origin"})
		}
		return c.Next()
	}
}

func RequireTrustedBrowserOrigin(allowed []string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if isSafeMethod(c.Method()) {
			return c.Next()
		}
		if !IsTrustedOrigin(c.Get(fiber.HeaderOrigin), allowed) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Untrusted request origin"})
		}
		return c.Next()
	}
}

func normalizeOrigin(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "null") {
		return "", false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), true
}

func isSafeMethod(method string) bool {
	return method == fiber.MethodGet || method == fiber.MethodHead || method == fiber.MethodOptions
}
