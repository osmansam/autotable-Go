package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

type brandingRouteHandlers struct {
	Runtime       fiber.Handler
	GetTenant     fiber.Handler
	PatchTenant   fiber.Handler
	UploadTenant  fiber.Handler
	DeleteTenant  fiber.Handler
	GetProject    fiber.Handler
	PatchProject  fiber.Handler
	UploadProject fiber.Handler
	DeleteProject fiber.Handler
}

func BrandingRoutes(app *fiber.App) {
	registerBrandingRoutes(app, brandingRouteHandlers{
		Runtime:       controllers.GetRuntimeBranding,
		GetTenant:     controllers.GetTenantBranding,
		PatchTenant:   controllers.PatchTenantBranding,
		UploadTenant:  controllers.UploadTenantBrandingAsset,
		DeleteTenant:  controllers.DeleteTenantBrandingAsset,
		GetProject:    controllers.GetProjectBranding,
		PatchProject:  controllers.PatchProjectBranding,
		UploadProject: controllers.UploadProjectBrandingAsset,
		DeleteProject: controllers.DeleteProjectBrandingAsset,
	})
}

func registerBrandingRoutes(app *fiber.App, handlers brandingRouteHandlers) {
	app.Get(
		"/api/v1/:tenantSlug/:projectSlug/branding",
		middlewares.GeneralRateLimit(),
		middlewares.SearchRateLimit(),
		handlers.Runtime,
	)

	const tenant = "/api/v1/tenant/branding"
	app.Get(tenant+"/", strictManagementRouteHandlers("GET", tenant+"/", middlewares.GeneralRateLimit(), handlers.GetTenant)...)
	app.Patch(tenant+"/", strictManagementRouteHandlers("PATCH", tenant+"/", middlewares.GeneralRateLimit(), middlewares.DefaultBodySizeLimit(), middlewares.WriteRateLimit(), handlers.PatchTenant)...)
	app.Post(tenant+"/assets/:slot", strictManagementRouteHandlers("POST", tenant+"/assets/:slot", middlewares.GeneralRateLimit(), middlewares.UploadBodySizeLimit(), middlewares.WriteRateLimit(), handlers.UploadTenant)...)
	app.Delete(tenant+"/assets/:slot", strictManagementRouteHandlers("DELETE", tenant+"/assets/:slot", middlewares.GeneralRateLimit(), middlewares.WriteRateLimit(), handlers.DeleteTenant)...)

	const project = "/api/v1/tenant/projects/:id/branding"
	app.Get(project+"/", strictManagementRouteHandlers("GET", project+"/", middlewares.GeneralRateLimit(), handlers.GetProject)...)
	app.Patch(project+"/", strictManagementRouteHandlers("PATCH", project+"/", middlewares.GeneralRateLimit(), middlewares.DefaultBodySizeLimit(), middlewares.WriteRateLimit(), handlers.PatchProject)...)
	app.Post(project+"/assets/:slot", strictManagementRouteHandlers("POST", project+"/assets/:slot", middlewares.GeneralRateLimit(), middlewares.UploadBodySizeLimit(), middlewares.WriteRateLimit(), handlers.UploadProject)...)
	app.Delete(project+"/assets/:slot", strictManagementRouteHandlers("DELETE", project+"/assets/:slot", middlewares.GeneralRateLimit(), middlewares.WriteRateLimit(), handlers.DeleteProject)...)
}
