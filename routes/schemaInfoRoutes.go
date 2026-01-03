package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/models"
)


func SchemaInfoRoutes(baseUrl string, app *fiber.App) {
	roleGroup := app.Group(baseUrl)

	// Apply tenant authentication and project scope requirement to all routes
	roleGroup.Use(middlewares.TenantAuthenticate)
	roleGroup.Use(middlewares.RequireProjectScope)

	roleGroup.Get("/roles", 
		middlewares.TenantAuthorize([]string{models.ProjectRoleAdmin, models.ProjectRoleDeveloper}), 
		controllers.GetRoleItems)

	roleGroup.Get("/roles/:id", 
		middlewares.TenantAuthorize([]string{models.ProjectRoleAdmin, models.ProjectRoleDeveloper}), 
		controllers.GetRoleItemById)
}
