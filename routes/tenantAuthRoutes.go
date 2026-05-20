package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

// TenantAuthRoutes sets up all tenant authentication routes
func TenantAuthRoutes(app *fiber.App) {
	// Public routes (no authentication required)
	tenantAuth := app.Group("/api/v1/tenant/auth")

	// Register new user and create tenant
	tenantAuth.Post("/register", middlewares.DefaultBodySizeLimit(), middlewares.AuthRateLimit(), controllers.TenantRegister)

	// Login to tenant
	tenantAuth.Post("/login", middlewares.DefaultBodySizeLimit(), middlewares.AuthRateLimit(), controllers.TenantLogin)

	// Refresh access token
	tenantAuth.Post("/refresh", middlewares.DefaultBodySizeLimit(), middlewares.PublicRateLimit(), controllers.TenantRefreshToken)

	// Protected routes (require authentication)
	tenantAuthProtected := app.Group("/api/v1/tenant/auth")
	tenantAuthProtected.Use(middlewares.TenantAuthenticate)
	tenantAuthProtected.Use(middlewares.GeneralRateLimit())

	// Logout
	tenantAuthProtected.Post("/logout", middlewares.DefaultBodySizeLimit(), controllers.TenantLogout)

	// Get current user info
	tenantAuthProtected.Get("/me", controllers.GetCurrentUser)

	// Switch to project context (requires tenant auth)
	tenantAuthProtected.Post("/switch-project", middlewares.DefaultBodySizeLimit(), controllers.SwitchToProject)
}
