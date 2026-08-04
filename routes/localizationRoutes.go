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
	group.Put(
		"/preference",
		middlewares.TenantAuthenticate,
		middlewares.RequireProjectScope,
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		preferenceHandler,
	)
}
