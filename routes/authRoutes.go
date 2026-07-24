package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

func AuthRoutes(baseUrl string, app *fiber.App) {
	authGroup := app.Group(baseUrl)
	authGroup.Get("/login-config", middlewares.PublicRateLimit(), controllers.GetProjectLoginConfig)
	authGroup.Post("/register", middlewares.DefaultBodySizeLimit(), middlewares.AuthRateLimit(), controllers.Register)
	authGroup.Post("/login", middlewares.DefaultBodySizeLimit(), middlewares.AuthRateLimit(), controllers.Login)
	// authGroup.Post("/refresh", controllers.Refresh)

	// Google OAuth routes
	authGroup.Get("/google/login", middlewares.PublicRateLimit(), controllers.GoogleLogin)
	authGroup.Get("/google/callback", middlewares.PublicRateLimit(), controllers.GoogleCallback)

	// Logout
	authGroup.Post("/logout", middlewares.DefaultBodySizeLimit(), middlewares.GeneralRateLimit(), controllers.Logout)
}
