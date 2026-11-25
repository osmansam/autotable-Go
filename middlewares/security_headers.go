package middlewares

import "github.com/gofiber/fiber/v2"

// SecurityHeaders protects API responses without imposing a document-level CSP,
// frame policy, or same-origin resource policy on Swagger/customer integrations.
// HSTS belongs at the trusted TLS ingress, since the app also serves plain HTTP.
func SecurityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Set("X-XSS-Protection", "0")
		return c.Next()
	}
}
