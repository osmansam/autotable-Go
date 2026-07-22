package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

func IntegrationRoutes(baseUrl string, app *fiber.App) {
	group := app.Group(baseUrl)
	group.Use(middlewares.TenantAuthenticate)
	group.Use(middlewares.GeneralRateLimit())

	group.Get("/credentials", controllers.ListIntegrationCredentials)
	group.Post("/credentials", middlewares.DefaultBodySizeLimit(), controllers.CreateIntegrationCredential)
	group.Post("/credentials/:id/revoke", middlewares.DefaultBodySizeLimit(), controllers.RevokeIntegrationCredential)

	group.Get("/external-api-credentials", controllers.ListExternalAPICredentials)
	group.Post("/external-api-credentials", middlewares.DefaultBodySizeLimit(), controllers.CreateExternalAPICredential)
	group.Post("/external-api-credentials/:id/revoke", middlewares.DefaultBodySizeLimit(), controllers.RevokeExternalAPICredential)
}
