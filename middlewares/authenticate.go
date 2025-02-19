package middlewares

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
)

func Authenticate(c *fiber.Ctx, isAuthorized bool, authorizeRole string, isActive bool) error {
    authHeader := c.Get("Authorization")
    if authHeader == "" {
        return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing Authorization header"})
    }

    var token string
    if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
        token = authHeader[7:]
    } else {
        token = authHeader // fallback if no "Bearer " prefix
    }
    userID, role, err := utils.ParseToken(token)
    if err != nil {
        log.Printf("Error parsing token: %v", err)
        return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
    }
    c.Locals("userID", userID)
    c.Locals("userRole", role)
    // Check if the account is active.
    if !isActive {
        return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
    }

    // Check if the role is authorized.
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

		isDynamicFunc := routeName == "ExecuteDynamicCode"
		isPipeline := routeName == "GetPipeline"
		isExecuteApi := routeName == "ExecuteDynamicAPI"
		var isAuthenticated bool
		var isAuthorized bool
		var isActive bool
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
		}else if isExecuteApi {
			apiName := c.Query("apiName")
			for _, api := range container.DynamicApis {
				if api.Name == apiName {
					isAuthenticated = api.IsAuthenticated
					isAuthorized = api.IsAuthorized
					authorizeRole = api.AuthorizeRole
					c.Locals("apiName", api)
					break
				}
			}
		}else {
			var route models.RouteSpec
			switch routeName {
			case "CreateDynamicModelItem":
				route = container.Routes.CreateDynamicModelItem
			case "CreateMultipleDynamicModelItem":
				route = container.Routes.CreateMultipleDynamicModelItem
			case "GetAllDynamicModelItems":
				route = container.Routes.GetAllDynamicModelItems
			case "HandleSearchDynamicModelItem":
				route = container.Routes.HandleSearchDynamicModelItem
			case "HandleFilterDynamicModelItem":
				route = container.Routes.HandleFilterDynamicModelItem
			case "DeleteDynamicModelItem":
				route = container.Routes.DeleteDynamicModelItem
			case "DeleteMultipleDynamicModelItem":
				route = container.Routes.DeleteMultipleDynamicModelItem
			case "UpdateDynamicModelItem":
				route = container.Routes.UpdateDynamicModelItem
			case "UpdateMultipleDynamicModelItem":
				route = container.Routes.UpdateMultipleDynamicModelItem
			case "GetDynamicModelItem":
				route = container.Routes.GetDynamicModelItem
			case "GetAllDynamicModelItemsWithPagination":
				route = container.Routes.GetAllDynamicModelItemsWithPagination
			case "TestPipeline":
				route = container.Routes.TestPipeline
			default:
				// If the route name does not match any known route, proceed without authentication
				return c.Next()
			}
			isAuthenticated = route.IsAuthenticated
			isAuthorized = route.IsAuthorized
			isActive= route.IsActive
			authorizeRole = route.AuthorizeRole
		}

		if isAuthenticated||!isActive {
			return Authenticate(c, isAuthorized, authorizeRole,isActive)
		}

		return c.Next()
	}
}
// TODO: authorize role will become string array and adjusted in middleware