package middlewares

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func IsTrustedOrigin(origin string, allowed []string) bool {
	normalized, ok := normalizeOrigin(origin)
	if !ok || strings.Contains(normalized, "*") {
		return false
	}
	for _, candidate := range allowed {
		trusted, valid := normalizeOrigin(candidate)
		if !valid {
			continue
		}
		if normalized == trusted {
			return true
		}
		if strings.Count(trusted, "*") != 1 {
			continue
		}
		pattern, _ := url.Parse(trusted)
		actual, _ := url.Parse(normalized)
		if !strings.HasPrefix(pattern.Hostname(), "*.") || actual.Scheme != pattern.Scheme || actual.Port() != pattern.Port() {
			continue
		}
		suffix := strings.TrimPrefix(pattern.Hostname(), "*")
		if !strings.HasSuffix(actual.Hostname(), suffix) {
			continue
		}
		prefix := strings.TrimSuffix(actual.Hostname(), suffix)
		if validSubdomainLabels(prefix) {
			return true
		}
	}
	return false
}

// Wildcards match one or more complete DNS labels, never the base domain.
func validSubdomainLabels(value string) bool {
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
				return false
			}
		}
	}
	return true
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
