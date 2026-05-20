package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

// SetupExcelRoutes sets up routes for Excel upload functionality
func SetupExcelRoutes(app *fiber.App, baseURL string) {
	// Excel routes under tenant/project context
	excelGroup := app.Group(baseURL + "/:tenantSlug/:projectSlug/excel")

	// Require tenant authentication and project scope for Excel upload
	excelGroup.Use(middlewares.TenantAuthenticate)
	excelGroup.Use(middlewares.GeneralRateLimit())
	excelGroup.Use(middlewares.RequireProjectScope)

	excelGroup.Post("/upload", middlewares.UploadRateLimit(), controllers.UploadExcel)
	excelGroup.Post("/upload-multiple", middlewares.UploadRateLimit(), controllers.UploadMultipleExcel)
}
