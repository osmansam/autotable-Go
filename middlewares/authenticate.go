package middlewares

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
)

func Authenticate(c *fiber.Ctx ,isAuthorized bool,authorizeRole string) error {
	token := c.Get("Authorization") // Assuming the token is in the Authorization header
	userID, role, err := utils.ParseToken(token)
	c.Locals("userID", userID)
	c.Locals("userRole", role)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	if isAuthorized && role != authorizeRole {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
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
		c.Locals("containerModel", container)

		// Determine if route Dynamic func
		isDynamicFunc := routeName == "ExecuteDynamicCode"
		// Determine if routeName is for a standard route or a pipeline
		isPipeline := routeName == "GetPipeline"
		var isAuthenticated bool
		var isAuthorized bool
		var authorizeRole string
		if isPipeline {
			pipelineName := c.Query("pipelineName")
			for _, pipeline := range container.Pipelines {
				if pipeline.Name == pipelineName {
					isAuthenticated = pipeline.IsAuthenticated
					isAuthorized = pipeline.IsAuthorized
					authorizeRole = pipeline.AuthorizeRole
					break
				}
			}
		}else if isDynamicFunc {
			functionName := c.Query("functionName")
			for _, function := range container.DynamicFunctions {
				if function.Name == functionName {
					isAuthenticated = function.IsAuthenticated
					isAuthorized = function.IsAuthorized
					authorizeRole = function.AuthorizeRole
					c.Locals("dynamicFunction", function)
					break
				}
			}
		}else {
			var route models.RouteSpec
			switch routeName {
			case "CreateDynamicModelItem":
				route = container.Routes.CreateDynamicModelItem
			case "GetAllDynamicModelItems":
				route = container.Routes.GetAllDynamicModelItems
			case "HandleSearchDynamicModelItem":
				route = container.Routes.HandleSearchDynamicModelItem
			case "DeleteDynamicModelItem":
				route = container.Routes.DeleteDynamicModelItem
			case "UpdateDynamicModelItem":
				route = container.Routes.UpdateDynamicModelItem
			case "GetDynamicModelItem":
				route = container.Routes.GetDynamicModelItem
			case "GetAllDynamicModelItemsWithPagination":
				route = container.Routes.GetAllDynamicModelItemsWithPagination
			default:
				// If the route name does not match any known route, proceed without authentication
				return c.Next()
			}
			isAuthenticated = route.IsAuthenticated
			isAuthorized = route.IsAuthorized
			authorizeRole = route.AuthorizeRole
		}

		if isAuthenticated {
			return Authenticate(c, isAuthorized, authorizeRole)
		}

		return c.Next()
	}
}
// TODO: authorize role will become string array and adjusted in middleware