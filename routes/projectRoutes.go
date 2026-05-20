package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/models"
)

// ProjectRoutes sets up all project management routes
func ProjectRoutes(app *fiber.App) {
	// All project routes require tenant authentication and tenant scope
	projectGroup := app.Group("/api/v1/tenant/projects")
	projectGroup.Use(middlewares.TenantAuthenticate)
	projectGroup.Use(middlewares.GeneralRateLimit())
	projectGroup.Use(middlewares.RequireTenantScope)

	// Create project - requires tenant admin or owner
	projectGroup.Post("/",
		middlewares.TenantAuthorize([]string{
			models.TenantRoleOwner,
			models.TenantRoleAdmin,
		}),
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.CreateProject,
	)

	// List all projects in tenant - any tenant member can view
	projectGroup.Get("/", middlewares.SearchRateLimit(), controllers.GetAllProjects)

	// Get single project - any tenant member can view
	projectGroup.Get("/:id", middlewares.SearchRateLimit(), controllers.GetProject)

	// Update project - requires tenant admin or owner
	projectGroup.Patch("/:id",
		middlewares.TenantAuthorize([]string{
			models.TenantRoleOwner,
			models.TenantRoleAdmin,
		}),
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdateProject,
	)

	// Delete project - requires tenant owner only
	projectGroup.Delete("/:id",
		middlewares.TenantOwnerOnly,
		middlewares.WriteRateLimit(),
		controllers.DeleteProject,
	)
}
