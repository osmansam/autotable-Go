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

	tenant := app.Group("/api/v1/tenant/branding")
	tenant.Use(middlewares.TenantAuthenticate)
	tenant.Use(middlewares.GeneralRateLimit())
	tenant.Get("/", handlers.GetTenant)
	tenant.Patch("/", middlewares.DefaultBodySizeLimit(), middlewares.WriteRateLimit(), handlers.PatchTenant)
	tenant.Post("/assets/:slot", middlewares.DefaultBodySizeLimit(), middlewares.WriteRateLimit(), handlers.UploadTenant)
	tenant.Delete("/assets/:slot", middlewares.WriteRateLimit(), handlers.DeleteTenant)

	project := app.Group("/api/v1/tenant/projects/:id/branding")
	project.Use(middlewares.TenantAuthenticate)
	project.Use(middlewares.GeneralRateLimit())
	project.Get("/", handlers.GetProject)
	project.Patch("/", middlewares.DefaultBodySizeLimit(), middlewares.WriteRateLimit(), handlers.PatchProject)
	project.Post("/assets/:slot", middlewares.DefaultBodySizeLimit(), middlewares.WriteRateLimit(), handlers.UploadProject)
	project.Delete("/assets/:slot", middlewares.WriteRateLimit(), handlers.DeleteProject)
}
