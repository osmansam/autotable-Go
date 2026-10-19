package routes

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/utils"
)

var auditLogsConfigProvider = utils.GetAuditLogsConfig

func adminAuditBaseURL(baseUrl string) string {
	if strings.HasSuffix(baseUrl, "/audit-logs") {
		return strings.TrimSuffix(baseUrl, "/audit-logs") + "/admin/audit-logs"
	}
	return "/admin" + baseUrl
}

// AuditRoutes registers two separate identity domains:
//   - runtime application audit access on baseUrl, which accepts only
//     application (A) credentials and the configured application-role policy;
//   - management audit access on /admin/audit-logs, which accepts only strict
//     management (M) credentials with live project membership permissions.
func AuditRoutes(baseUrl string, app *fiber.App) {
	auditGroup := app.Group(baseUrl)

	auditGroup.Get("/config",
		middlewares.ProjectAuthentication,
		controllers.GetAuditLogsConfig,
	)

	auditGroup.Use(middlewares.ProjectAuthentication)
	auditGroup.Use(auditLogsAuthorization)
	auditGroup.Use(middlewares.GeneralRateLimit())

	// GET /audit-logs - Retrieve audit logs with filtering, sorting, and pagination
	auditGroup.Get("/", middlewares.SearchRateLimit(), controllers.GetAuditLogs)

	adminBase := "/" + strings.TrimPrefix(adminAuditBaseURL(baseUrl), "/")
	app.Get(adminBase+"/config", strictManagementRouteHandlers("GET", adminBase+"/config", controllers.GetAuditLogsConfig)...)
	app.Patch(adminBase+"/config", strictManagementRouteHandlers("PATCH", adminBase+"/config",
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdateAuditLogsConfig,
	)...)
	app.Get(adminBase+"/", strictManagementRouteHandlers("GET", adminBase+"/",
		middlewares.GeneralRateLimit(),
		middlewares.SearchRateLimit(),
		controllers.GetAuditLogs,
	)...)
}

func auditLogsAuthorization(c *fiber.Ctx) error {
	auditConfig, err := auditLogsConfigProvider()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch audit logs config"})
	}
	if auditConfig == nil || !auditConfig.IsAuthorized {
		return c.Next()
	}
	tenantID, _ := c.Locals("tenantID").(string)
	projectID, _ := c.Locals("projectID").(string)
	authorizeRoles := auditConfig.AuthorizeRole
	if tenantID != "" && projectID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		authorizeRoles = utils.ExpandRoleIdentifiers(ctx, tenantID, projectID, auditConfig.AuthorizeRole)
	}
	if hasAnyRole(c, authorizeRoles) {
		return c.Next()
	}
	return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
}

func hasAnyRole(c *fiber.Ctx, allowedRoles []string) bool {
	if len(allowedRoles) == 0 {
		return false
	}
	// Application identity carries a single dynamic role in "userRole".
	userRole, _ := c.Locals("userRole").(string)
	if userRole == "" {
		return false
	}
	userRoles := []string{userRole}
	for _, userRole := range userRoles {
		for _, allowedRole := range allowedRoles {
			if userRole == allowedRole {
				return true
			}
		}
	}
	return false
}
