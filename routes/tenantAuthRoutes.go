package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

// TenantAuthRoutes sets up all tenant authentication routes
func TenantAuthRoutes(app *fiber.App) {
	projectManagementAuth := app.Group("/api/v1/:tenantSlug/:projectSlug/tenant-auth")
	projectManagementAuth.Post("/refresh", middlewares.DefaultBodySizeLimit(), middlewares.PublicRateLimit(), controllers.ManagementProjectRefresh)
	projectManagementAuth.Post("/logout", middlewares.DefaultBodySizeLimit(), middlewares.PublicRateLimit(), controllers.ManagementProjectLogout)

	// Public routes (no authentication required)
	tenantAuth := app.Group("/api/v1/tenant/auth")

	// Register new user and create tenant
	tenantAuth.Post("/register", middlewares.DefaultBodySizeLimit(), middlewares.AuthRateLimit(), controllers.TenantRegister)

	// Login to tenant
	tenantAuth.Post("/login", middlewares.DefaultBodySizeLimit(), middlewares.AuthRateLimit(), controllers.TenantLogin)
	tenantAuth.Get("/session", middlewares.PublicRateLimit(), controllers.TenantSession)

	// Refresh access token
	tenantAuth.Post("/refresh", middlewares.DefaultBodySizeLimit(), middlewares.PublicRateLimit(), controllers.TenantRefreshToken)

	// Get current user info
	app.Get("/api/v1/tenant/auth/me", strictManagementRouteHandlers("GET", "/api/v1/tenant/auth/me", middlewares.GeneralRateLimit(), controllers.GetCurrentUser)...)

	// Switch to project context (requires tenant auth)
	app.Post("/api/v1/tenant/auth/switch-project", strictManagementRouteHandlers("POST", "/api/v1/tenant/auth/switch-project", middlewares.GeneralRateLimit(), middlewares.DefaultBodySizeLimit(), controllers.SwitchToProject)...)

	// Logout remains callable when the access token has expired so cookies can be cleared.
	tenantAuth.Post("/logout", middlewares.DefaultBodySizeLimit(), middlewares.PublicRateLimit(), controllers.TenantLogout)
}
