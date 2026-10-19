package routes

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

// SetupExcelRoutes sets up routes for Excel upload functionality
func SetupExcelRoutes(app *fiber.App, baseURL string) {
	base := "/" + strings.TrimPrefix(baseURL, "/") + "/:tenantSlug/:projectSlug/excel"
	app.Post(base+"/upload", strictManagementRouteHandlers("POST", base+"/upload", middlewares.GeneralRateLimit(), middlewares.UploadBodySizeLimit(), middlewares.UploadRateLimit(), controllers.UploadExcel)...)
	app.Post(base+"/upload-multiple", strictManagementRouteHandlers("POST", base+"/upload-multiple", middlewares.GeneralRateLimit(), middlewares.UploadBodySizeLimit(), middlewares.UploadRateLimit(), controllers.UploadMultipleExcel)...)
}
