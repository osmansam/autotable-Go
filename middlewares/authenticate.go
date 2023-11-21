package middlewares

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
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

func ConditionalAuthentication(routeName string) fiber.Handler {
    return func(c *fiber.Ctx) error {
        schemaName := c.Query("schemaName")
        if schemaName == "" {
            return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Schema name is required"})
        }

        // Fetch container model based on schema name
        container, err := utils.GetContainerModel(schemaName)
        if err != nil {
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch container model"})
        }

        // Check if route requires authentication
        var route models.AuthenticatedField
        switch routeName {
        case "CreateDynamicModelItem":
            route = container.Routes.CreateDynamicModelItem
        case "GetAllDynamicModelItems":
            route = container.Routes.GetAllDynamicModelItems
        case "GetPipeline":
            route = container.Routes.GetPipeline
        case "HandleSearchDynamicModelItem":
            route = container.Routes.HandleSearchDynamicModelItem
        case "DeleteDynamicModelItem":
            route = container.Routes.DeleteDynamicModelItem
        case "UpdateDynamicModelItem":
            route = container.Routes.UpdateDynamicModelItem
        case "GetDynamicModelItem":
            route = container.Routes.GetDynamicModelItem
        default:
            // If the route name does not match any known route, proceed without authentication
            return c.Next()
        }

        if route.IsAuthenticated {
            // Call the Authenticate middleware if IsAuthenticated is true
            return Authenticate(c)
        }

        // Proceed to the next middleware if no authentication is required
        return c.Next()
    }
}