package middlewares

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
)

func Authenticate(c *fiber.Ctx, isAuthorized bool, authorizeRole []string, isActive bool) error {
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

	userID, role, tokenTenantID, tokenProjectID, _, _, err := utils.ParseToken(token)
	if err != nil {
		log.Printf("Error parsing token: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	// Get the expected tenant and project IDs from context
	expectedTenantID, _ := c.Locals("expectedTenantID").(string)
	expectedProjectID, _ := c.Locals("expectedProjectID").(string)

	// Validate tenant ID if available
	if expectedTenantID != "" && tokenTenantID != expectedTenantID {
		log.Printf("Token tenant_id (%s) doesn't match requested tenant (%s)", tokenTenantID, expectedTenantID)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Access denied - Token is not valid for this tenant"})
	}

	// Validate project ID - this is the critical check to prevent cross-project access
	if expectedProjectID != "" && tokenProjectID != expectedProjectID {
		log.Printf("Token project_id (%s) doesn't match requested project (%s)", tokenProjectID, expectedProjectID)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Access denied - Token is not valid for this project"})
	}

	c.Locals("userID", userID)
	c.Locals("userRole", role)
	c.Locals("tenantID", tokenTenantID)
	c.Locals("projectID", tokenProjectID)

	// Check if the account is active.
	if !isActive {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}

	// Check if the user's role is in the authorized roles array
	if isAuthorized {
		roleAuthorized := false
		for _, allowedRole := range authorizeRole {
			if role == allowedRole {
				roleAuthorized = true
				break
			}
		}
		if !roleAuthorized {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
		}
	}

	return c.Next()
}

// ConditionalAuthenticationForPages middleware allows optional authentication for page routes
// If a valid token is present, it extracts user context; otherwise proceeds as anonymous
func ConditionalAuthenticationForPages(c *fiber.Ctx) error {
	// Extract tenant and project context from URL slugs
	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get project context: " + err.Error()})
	}

	// Store expected tenant and project IDs in context
	c.Locals("expectedTenantID", tenantID)
	c.Locals("expectedProjectID", projectID)

	// Optional Authentication: If token is present, try to parse it to identify the user/role
	// This allows pages with IsAuthenticated/IsAuthorized to be filtered properly
	authHeader := c.Get("Authorization")

	if authHeader != "" {
		var token string
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		} else {
			token = authHeader
		}

		userID, role, tokenTenantID, tokenProjectID, _, _, err := utils.ParseToken(token)
		if err == nil {
			// Validate that token's project matches the requested project
			if tokenProjectID == projectID && tokenTenantID == tenantID {
				// Valid token for this project
				c.Locals("userID", userID)
				c.Locals("userRole", role)
				c.Locals("tenantID", tokenTenantID)
				c.Locals("projectID", tokenProjectID)
			}
		}
	}

	return c.Next()
}

func ProjectAuthentication(c *fiber.Ctx) error {
	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get project context: " + err.Error()})
	}
	c.Locals("expectedTenantID", tenantID)
	c.Locals("expectedProjectID", projectID)
	return Authenticate(c, false, nil, true)
}

func ConditionalAuthentication(routeName string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Extract tenant and project context from URL slugs (falls back to query params or JWT for backward compatibility)
		tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get project context: " + err.Error()})
		}

		schemaName := c.Query("schemaName")
		if schemaName == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Schema name is required"})
		}

		// Fetch container model based on tenant/project context
		container, err := utils.GetContainerModel(tenantID, projectID, schemaName)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch container model"})
		}
		c.Locals("containerModel", container)

		isDynamicFunc := routeName == "ExecuteDynamicCode"
		isPipeline := routeName == "GetPipeline"
		isExecuteApi := routeName == "ExecuteDynamicAPI"
		isExecuteWorkflow := routeName == "ExecuteWorkflow"
		var isAuthenticated bool
		var isAuthorized bool
		var isActive bool
		var authorizeRole []string
		if isPipeline {
			pipelineName := c.Query("pipelineName")
			for _, pipeline := range container.Pipelines {
				if pipeline.Name == pipelineName {
					isAuthenticated = pipeline.IsAuthenticated
					isAuthorized = pipeline.IsAuthorized
					isActive = pipeline.IsActive
					authorizeRole = pipeline.AuthorizeRole
					break
				}
			}
		} else if isDynamicFunc {
			functionName := c.Query("functionName")
			for _, function := range container.DynamicFunctions {
				if function.Name == functionName {
					isAuthenticated = function.IsAuthenticated
					isAuthorized = function.IsAuthorized
					isActive = function.IsActive
					authorizeRole = function.AuthorizeRole
					c.Locals("dynamicFunction", function)
					break
				}
			}
		} else if isExecuteApi {
			apiName := c.Query("apiName")
			for _, api := range container.DynamicApis {
				if api.Name == apiName {
					isAuthenticated = api.IsAuthenticated
					isAuthorized = api.IsAuthorized
					isActive = api.IsActive
					authorizeRole = api.AuthorizeRole
					c.Locals("apiName", api)
					break
				}
			}
		} else if isExecuteWorkflow {
			workflowName := c.Params("workflowName")
			if workflowName == "" {
				workflowName = c.Query("workflowName")
			}
			found := false
			for _, workflow := range container.Workflows {
				if workflow.Name == workflowName {
					isAuthenticated = workflow.IsAuthenticated
					isAuthorized = workflow.IsAuthorized
					isActive = workflow.IsActive
					authorizeRole = workflow.AuthorizeRole
					c.Locals("workflow", workflow)
					found = true
					break
				}
			}
			if !found {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Workflow not found"})
			}
		} else {
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
			case "ExportDynamicModelItems":
				route = container.Routes.ExportDynamicModelItems
			case "GetItemsForSelection":
				route = container.Routes.GetItemsForSelection
			default:
				// If the route name does not match any known route, proceed without authentication
				return c.Next()
			}
			isAuthenticated = route.IsAuthenticated
			isAuthorized = route.IsAuthorized
			isActive = route.IsActive
			authorizeRole = route.AuthorizeRole
		}

		if isAuthenticated || !isActive {
			// Store expected tenant and project IDs in context for validation
			c.Locals("expectedTenantID", tenantID)
			c.Locals("expectedProjectID", projectID)
			return Authenticate(c, isAuthorized, authorizeRole, isActive)
		}

		// Optional Authentication: If token is present, try to parse it to identify the user/role.
		// This allows Row Access rules to work even on public routes.
		authHeader := c.Get("Authorization")
		if authHeader != "" {
			var token string
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				token = authHeader[7:]
			} else {
				token = authHeader
			}

			userID, role, tokenTenantID, tokenProjectID, _, _, err := utils.ParseToken(token)
			if err == nil {
				// Validate that token's project matches the requested project
				if tokenProjectID != projectID {
					log.Printf("Optional auth: Token project_id (%s) doesn't match requested project (%s) - proceeding as anonymous", tokenProjectID, projectID)
					// Don't set user context - treat as anonymous
				} else if tokenTenantID != tenantID {
					log.Printf("Optional auth: Token tenant_id (%s) doesn't match requested tenant (%s) - proceeding as anonymous", tokenTenantID, tenantID)
					// Don't set user context - treat as anonymous
				} else {
					// Valid token for this project
					c.Locals("userID", userID)
					c.Locals("userRole", role)
					c.Locals("tenantID", tokenTenantID)
					c.Locals("projectID", tokenProjectID)
				}
			} else {
				log.Printf("Optional auth token parse failed: %v", err)
			}
			// If token is invalid, we ignore it and proceed as anonymous.
		}

		return c.Next()
	}
}
