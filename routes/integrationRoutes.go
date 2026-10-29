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

func tenantExternalAPICredentialRoutes(app *fiber.App) {
	const base = "/api/v1/tenant/integrations/external-api-credentials"
	app.Get(base, strictManagementRouteHandlers("GET", base, middlewares.GeneralRateLimit(), controllers.ListTenantExternalAPICredentials)...)
	app.Post(base, strictManagementRouteHandlers("POST", base, middlewares.GeneralRateLimit(), middlewares.DefaultBodySizeLimit(), controllers.CreateTenantExternalAPICredential)...)
	app.Post(base+"/:id/revoke", strictManagementRouteHandlers("POST", base+"/:id/revoke", middlewares.GeneralRateLimit(), controllers.RevokeTenantExternalAPICredential)...)
	app.Post(base+"/:id/promote", strictManagementRouteHandlers("POST", base+"/:id/promote", middlewares.GeneralRateLimit(), controllers.PromoteExternalAPICredential)...)
	app.Put(base+"/:id/projects/:projectId", strictManagementRouteHandlers("PUT", base+"/:id/projects/:projectId", middlewares.GeneralRateLimit(), controllers.GrantExternalAPICredential)...)
	app.Delete(base+"/:id/projects/:projectId", strictManagementRouteHandlers("DELETE", base+"/:id/projects/:projectId", middlewares.GeneralRateLimit(), controllers.DisconnectExternalAPICredential)...)
}
