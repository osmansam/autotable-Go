package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/models"
)

// PageRoutes sets up all page management routes
func PageRoutes(baseUrl string, app *fiber.App) {
	// Public route with conditional authentication based on page settings
	// Users will see pages based on IsAuthenticated/IsAuthorized settings
	// Apply conditional auth middleware to extract user role if token is present
	app.Get(baseUrl+"/public", middlewares.ConditionalAuthenticationForPages, controllers.GetAllPagesPublic)
	
	// Tenant-authenticated routes (project scope required)
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
	
	// Get all pages - tenant access (any project member can view)
	pageGroup.Get("/",middlewares.TenantAuthorize([]string{
			models.ProjectRoleAdmin, 
			models.ProjectRoleDeveloper,
			models.ProjectRoleEditor,
		}),  controllers.GetAllPages)
	
	// Get single page - any project member can view
	pageGroup.Get("/:id",
	middlewares.TenantAuthorize([]string{
			models.ProjectRoleAdmin, 
			models.ProjectRoleDeveloper,
			models.ProjectRoleEditor,
		}),  controllers.GetPage)
	
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
