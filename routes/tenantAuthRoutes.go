package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

// TenantAuthRoutes sets up all tenant authentication routes
func TenantAuthRoutes(app *fiber.App) {
	// Public routes (no authentication required)
	tenantAuth := app.Group("/api/tenant/auth")
	
	// Register new user and create tenant
	tenantAuth.Post("/register", controllers.TenantRegister)
	
	// Login to tenant
	tenantAuth.Post("/login", controllers.TenantLogin)
	
	// Refresh access token
	tenantAuth.Post("/refresh", controllers.TenantRefreshToken)

	// Protected routes (require authentication)
	tenantAuthProtected := app.Group("/api/tenant/auth")
	tenantAuthProtected.Use(middlewares.TenantAuthenticate)
	
	// Logout
	tenantAuthProtected.Post("/logout", controllers.TenantLogout)
	
	// Get current user info
	tenantAuthProtected.Get("/me", controllers.GetCurrentUser)
	
	// Switch to project context (requires tenant auth)
	tenantAuthProtected.Post("/switch-project", controllers.SwitchToProject)
}
