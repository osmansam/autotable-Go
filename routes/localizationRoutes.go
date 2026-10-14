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
	group.Get("/editor/translations",
		middlewares.ProjectAuthentication,
		middlewares.SearchRateLimit(),
		controllers.GetEditorTranslations,
	)
	group.Post("/reconcile",
		middlewares.ProjectAuthentication,
		middlewares.WriteRateLimit(),
		controllers.ReconcileRuntimeProjectTranslations,
	)
	group.Post("/translations/generate",
		middlewares.ProjectAuthentication,
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.GenerateRuntimeProjectTranslations,
	)
	group.Get("/translation-jobs/:jobId",
		middlewares.ProjectAuthentication,
		middlewares.SearchRateLimit(),
		controllers.GetRuntimeTranslationJob,
	)
	group.Patch("/translations/:locale/:key",
		middlewares.ProjectAuthentication,
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdateRuntimeProjectTranslation,
	)
	group.Put(
		"/preference",
		middlewares.TenantAuthenticate,
		middlewares.RequireProjectScope,
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		preferenceHandler,
	)
}
