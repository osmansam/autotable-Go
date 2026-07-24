package routes

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/models"
)

func adminPageBaseURL(baseUrl string) string {
	if strings.HasSuffix(baseUrl, "/page") {
		return strings.TrimSuffix(baseUrl, "/page") + "/admin/page"
	}
	if strings.HasSuffix(baseUrl, "/pages") {
		return strings.TrimSuffix(baseUrl, "/pages") + "/admin/pages"
	}
	return "/admin" + baseUrl
}

// PageRoutes sets up all page management routes
func PageRoutes(baseUrl string, app *fiber.App) {
	// Runtime pages for the end-user app. Users see pages based on page-level
	// authentication and authorization settings.
	app.Get(baseUrl, middlewares.TenantAuthenticate, middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.GetAllPagesPublic)

	// Admin page management routes for tenantPanel.
	pageGroup := app.Group(adminPageBaseURL(baseUrl))
	pageGroup.Use(middlewares.TenantAuthenticate)
	pageGroup.Use(middlewares.GeneralRateLimit())
	pageGroup.Use(middlewares.RequireProjectScope)

	// Create page - requires project admin, developer, or editor role
	pageGroup.Post("/",
		middlewares.TenantAuthorize([]string{
			models.ProjectRoleAdmin,
			models.ProjectRoleDeveloper,
			models.ProjectRoleEditor,
		}),
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.CreatePage,
	)

	// Get all pages - tenant access (any project member can view)
	pageGroup.Get("/", middlewares.TenantAuthorize([]string{
		models.ProjectRoleAdmin,
		models.ProjectRoleDeveloper,
		models.ProjectRoleEditor,
	}), middlewares.SearchRateLimit(), controllers.GetAllPages)

	// Get single page - any project member can view
	pageGroup.Get("/:id",
		middlewares.TenantAuthorize([]string{
			models.ProjectRoleAdmin,
			models.ProjectRoleDeveloper,
			models.ProjectRoleEditor,
		}), middlewares.SearchRateLimit(), controllers.GetPage)

	// Update page - requires project admin, developer, or editor role
	pageGroup.Patch("/:id",
		// middlewares.TenantAuthorize([]string{
		// 	models.ProjectRoleAdmin,
		// 	models.ProjectRoleDeveloper,
		// 	models.ProjectRoleEditor,
		// }),
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdatePage,
	)

	// Delete page - requires project admin role
	pageGroup.Delete("/:id",
		middlewares.TenantAuthorize([]string{models.ProjectRoleAdmin}),
		middlewares.WriteRateLimit(),
		controllers.DeletePage,
	)
}
