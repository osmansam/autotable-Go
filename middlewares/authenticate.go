package middlewares

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/utils"
)

func Authenticate(c *fiber.Ctx) error {
	token := c.Get("Authorization")  // Assuming the token is in the Authorization header
	_, err := utils.ParseJWT(token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	return c.Next()
}
