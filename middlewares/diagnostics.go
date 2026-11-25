package middlewares

import (
	"crypto/sha256"
	"crypto/subtle"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"strings"
)

func DiagnosticsAuth() fiber.Handler {
	token, err := configs.DiagnosticsToken()
	enabled := err == nil && token != ""
	expected := sha256.Sum256([]byte(token))
	return func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		if !enabled {
			return c.SendStatus(fiber.StatusNotFound)
		}
		presented, ok := strings.CutPrefix(c.Get(fiber.HeaderAuthorization), "Bearer ")
		digest := sha256.Sum256([]byte(presented))
		if !ok || subtle.ConstantTimeCompare(digest[:], expected[:]) != 1 {
			c.Set(fiber.HeaderWWWAuthenticate, "Bearer")
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		return c.Next()
	}
}
