package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/utils"
)

var auditLogsConfigProvider = utils.GetAuditLogsConfig

func AuditRoutes(baseUrl string, app *fiber.App) {
	auditGroup := app.Group(baseUrl)

	// Fetch audit logs authorization config from database (returns default if not found)
	auditConfig, _ := auditLogsConfigProvider()

	isAuthorized := auditConfig.IsAuthorized
	authorizeRole := auditConfig.AuthorizeRole
	isActive := true // Audit logs are always active

	// Protect audit routes with authentication and authorization from database config
	auditGroup.Use(func(c *fiber.Ctx) error {
		return middlewares.Authenticate(c, isAuthorized, authorizeRole, isActive)
	})
	auditGroup.Use(middlewares.GeneralRateLimit())

	// GET /audit-logs - Retrieve audit logs with filtering, sorting, and pagination
	auditGroup.Get("/", middlewares.SearchRateLimit(), controllers.GetAuditLogs)
}
