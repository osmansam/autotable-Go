package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

func RuntimeSchemaRoutes(app *fiber.App) {
	app.Get("/api/v1/:tenantSlug/:projectSlug/runtime/schemas",
		middlewares.PublicRateLimit(), middlewares.ConditionalAuthenticationForPages,
		middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.GetRuntimeSchemas)
}
