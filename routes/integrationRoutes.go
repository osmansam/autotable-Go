package routes

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

func IntegrationRoutes(baseUrl string, app *fiber.App) {
	base := "/" + strings.TrimPrefix(baseUrl, "/")
	app.Get(base+"/credentials", strictManagementRouteHandlers("GET", base+"/credentials", middlewares.GeneralRateLimit(), controllers.ListIntegrationCredentials)...)
	app.Post(base+"/credentials", strictManagementRouteHandlers("POST", base+"/credentials", middlewares.GeneralRateLimit(), middlewares.DefaultBodySizeLimit(), controllers.CreateIntegrationCredential)...)
	app.Post(base+"/credentials/:id/revoke", strictManagementRouteHandlers("POST", base+"/credentials/:id/revoke", middlewares.GeneralRateLimit(), middlewares.DefaultBodySizeLimit(), controllers.RevokeIntegrationCredential)...)

	app.Get(base+"/external-api-credentials", strictManagementRouteHandlers("GET", base+"/external-api-credentials", middlewares.GeneralRateLimit(), controllers.ListExternalAPICredentials)...)
	app.Post(base+"/external-api-credentials", strictManagementRouteHandlers("POST", base+"/external-api-credentials", middlewares.GeneralRateLimit(), middlewares.DefaultBodySizeLimit(), controllers.CreateExternalAPICredential)...)
	app.Post(base+"/external-api-credentials/:id/revoke", strictManagementRouteHandlers("POST", base+"/external-api-credentials/:id/revoke", middlewares.GeneralRateLimit(), middlewares.DefaultBodySizeLimit(), controllers.RevokeExternalAPICredential)...)
}
