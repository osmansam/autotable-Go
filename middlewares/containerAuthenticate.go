package middlewares

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/utils"
)

func ContainerAuthenticate(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	// Check if the Authorization header is in the expected format
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).SendString("No Authorization header provided")
	}
	// Expecting "Bearer <token>"
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return c.Status(fiber.StatusUnauthorized).SendString("Invalid Authorization header format")
	}
	// Extract the token
	token := parts[1]
	// Parse the token
	permissions, err := utils.ParseContainerToken(token)

	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}	
	c.Locals("permissions", permissions)
	return c.Next()
}
