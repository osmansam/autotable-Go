package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

// ProjectRoutes sets up all project management routes
func ProjectRoutes(app *fiber.App) {
	const base = "/api/v1/tenant/projects"
	tenantExternalAPICredentialRoutes(app)

	app.Post(base+"/", strictManagementRouteHandlers("POST", base+"/",
		middlewares.GeneralRateLimit(),
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.CreateProject,
	)...)

	app.Get(base+"/", strictManagementRouteHandlers("GET", base+"/", middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.GetAllProjects)...)

	app.Get(base+"/templates/:id/external-api-requirements", strictManagementRouteHandlers("GET", base+"/templates/:id/external-api-requirements", middlewares.GeneralRateLimit(), controllers.GetTemplateExternalAPIRequirements)...)

	app.Get(base+"/templates", strictManagementRouteHandlers("GET", base+"/templates", middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.GetProjectTemplates)...)

	app.Patch(base+"/:id/locales", strictManagementRouteHandlers("PATCH", base+"/:id/locales", middlewares.GeneralRateLimit(),
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdateProjectLocales,
	)...)
	app.Get(base+"/:id/translations", strictManagementRouteHandlers("GET", base+"/:id/translations", middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.ListProjectTranslations)...)
	app.Post(base+"/:id/translations/generate", strictManagementRouteHandlers("POST", base+"/:id/translations/generate", middlewares.GeneralRateLimit(),
		middlewares.DefaultBodySizeLimit(), middlewares.WriteRateLimit(), controllers.GenerateProjectTranslations,
	)...)
	app.Get(base+"/:id/translation-jobs/:jobId", strictManagementRouteHandlers("GET", base+"/:id/translation-jobs/:jobId", middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.GetProjectTranslationJob)...)
	app.Patch(base+"/:id/translations/:locale/:key", strictManagementRouteHandlers("PATCH", base+"/:id/translations/:locale/:key", middlewares.GeneralRateLimit(),
		middlewares.DefaultBodySizeLimit(), middlewares.WriteRateLimit(), controllers.UpdateProjectTranslation,
	)...)

	app.Patch(base+"/:id/template", strictManagementRouteHandlers("PATCH", base+"/:id/template", middlewares.GeneralRateLimit(),
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdateProjectTemplate,
	)...)

	app.Get(base+"/:id", strictManagementRouteHandlers("GET", base+"/:id", middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.GetProject)...)

	app.Patch(base+"/:id", strictManagementRouteHandlers("PATCH", base+"/:id", middlewares.GeneralRateLimit(),
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdateProject,
	)...)

	app.Delete(base+"/:id", strictManagementRouteHandlers("DELETE", base+"/:id", middlewares.GeneralRateLimit(),
		middlewares.WriteRateLimit(),
		controllers.DeleteProject,
	)...)
}
