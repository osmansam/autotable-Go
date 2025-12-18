package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/models"
)

// PageRoutes sets up all page management routes
// These routes require tenant authentication and project scope
func PageRoutes(baseUrl string, app *fiber.App) {
	// All page routes require tenant authentication and project scope
	pageGroup := app.Group(baseUrl)
	pageGroup.Use(middlewares.TenantAuthenticate)
	pageGroup.Use(middlewares.RequireProjectScope)
	
	// Create page - requires project admin, developer, or editor role
	pageGroup.Post("/", 
		middlewares.TenantAuthorize([]string{
			models.ProjectRoleAdmin, 
			models.ProjectRoleDeveloper,
			models.ProjectRoleEditor,
		}), 
		controllers.CreatePage,
	)
	
	// Get all pages - any project member can view
	pageGroup.Get("/", controllers.GetAllPages)
	
	// Get single page - any project member can view
	pageGroup.Get("/:id", controllers.GetPage)
	
	// Update page - requires project admin, developer, or editor role
	pageGroup.Patch("/:id", 
		middlewares.TenantAuthorize([]string{
			models.ProjectRoleAdmin, 
			models.ProjectRoleDeveloper,
			models.ProjectRoleEditor,
		}), 
		controllers.UpdatePage,
	)
	
	// Delete page - requires project admin role
	pageGroup.Delete("/:id", 
		middlewares.TenantAuthorize([]string{models.ProjectRoleAdmin}), 
		controllers.DeletePage,
	)
}
