package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

func LocalizationRoutes(app *fiber.App) {
	registerLocalizationRoutes(app, controllers.GetRuntimeLocaleSettings, controllers.GetRuntimeTranslations, controllers.SaveRuntimeLocalePreference)
}

func registerLocalizationRoutes(app *fiber.App, settingsHandler, translationsHandler, preferenceHandler fiber.Handler) {
	group := app.Group("/api/v1/:tenantSlug/:projectSlug/localization")
	group.Use(middlewares.GeneralRateLimit())
	group.Get("/settings", middlewares.SearchRateLimit(), settingsHandler)
	group.Get("/translations", middlewares.SearchRateLimit(), translationsHandler)
	// Application-user translation editor/reconcile/generate/job/write routes are
	// intentionally not registered: those operations are management-only and are
	// served by the protected /tenant/projects/:id localization routes.
	group.Put(
		"/preference",
		middlewares.ProjectAuthentication,
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		preferenceHandler,
	)
}
