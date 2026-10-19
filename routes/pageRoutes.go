package routes

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
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
	app.Get(baseUrl, middlewares.ConditionalAuthenticationForPages, middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.GetAllPagesPublic)

	// Admin page management routes for tenantPanel.
	adminBase := "/" + strings.TrimPrefix(adminPageBaseURL(baseUrl), "/")

	// Create page - requires project admin, developer, or editor role
	app.Post(adminBase+"/", strictManagementRouteHandlers("POST", adminBase+"/",
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.CreatePage,
	)...)

	// Get all pages - tenant access (any project member can view)
	app.Get(adminBase+"/", strictManagementRouteHandlers("GET", adminBase+"/", middlewares.SearchRateLimit(), controllers.GetAllPages)...)

	// Get single page - any project member can view
	app.Get(adminBase+"/:id", strictManagementRouteHandlers("GET", adminBase+"/:id", middlewares.SearchRateLimit(), controllers.GetPage)...)

	// Update page - requires project admin, developer, or editor role
	app.Patch(adminBase+"/:id", strictManagementRouteHandlers("PATCH", adminBase+"/:id",
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdatePage,
	)...)

	// Delete page - requires project admin role
	app.Delete(adminBase+"/:id", strictManagementRouteHandlers("DELETE", adminBase+"/:id",
		middlewares.WriteRateLimit(),
		controllers.DeletePage,
	)...)
}
