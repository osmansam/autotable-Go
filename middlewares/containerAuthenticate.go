package middlewares

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/utils"
)

func ContainerAuthenticate(routeName string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		// Check for Bearer token format
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			return c.Status(fiber.StatusUnauthorized).SendString("Invalid or missing Authorization header")
		}
		// Extract the token
		token := strings.TrimPrefix(authHeader, "Bearer ")
		// Parse the token
		permissions, err := utils.ParseContainerToken(token)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized: Invalid token")
		}

		// Check if the routeName is in the permissions
		hasAccess := false
		for _, perm := range permissions {
			if perm == routeName {
				hasAccess = true
				break
			}
		}
		if !hasAccess {
			return c.Status(fiber.StatusForbidden).SendString("Forbidden: You don't have permission to access this route")
		}

		return c.Next()
	}
}