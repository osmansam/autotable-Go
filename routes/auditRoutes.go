package routes

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
)

var auditLogsConfigProvider = utils.GetAuditLogsConfig

func AuditRoutes(baseUrl string, app *fiber.App) {
	auditGroup := app.Group(baseUrl)

	auditGroup.Get("/config",
		middlewares.TenantAuthenticate,
		middlewares.RequireProjectScope,
		controllers.GetAuditLogsConfig,
	)
	auditGroup.Patch("/config",
		middlewares.TenantAuthenticate,
		middlewares.RequireProjectScope,
		requireAuditConfigManager,
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdateAuditLogsConfig,
	)

	auditGroup.Use(middlewares.TenantAuthenticate)
	auditGroup.Use(middlewares.RequireProjectScope)
	auditGroup.Use(auditLogsAuthorization)
	auditGroup.Use(middlewares.GeneralRateLimit())

	// GET /audit-logs - Retrieve audit logs with filtering, sorting, and pagination
	auditGroup.Get("/", middlewares.SearchRateLimit(), controllers.GetAuditLogs)
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

func requireAuditConfigManager(c *fiber.Ctx) error {
	if hasAnyRole(c, []string{models.ProjectRoleAdmin, models.ProjectRoleDeveloper}) {
		return c.Next()
	}
	return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Insufficient permissions"})
}

func hasAnyRole(c *fiber.Ctx, allowedRoles []string) bool {
	if len(allowedRoles) == 0 {
		return false
	}
	userRoles, ok := c.Locals("roles").([]string)
	if !ok || len(userRoles) == 0 {
		return false
	}
	for _, userRole := range userRoles {
		for _, allowedRole := range allowedRoles {
			if userRole == allowedRole {
				return true
			}
		}
	}
	return false
}
