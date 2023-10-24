package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
)
func AuthRoutes(baseUrl string, app *fiber.App) {
	authGroup := app.Group(baseUrl)
	authGroup.Post("/register", controllers.Register)
	authGroup.Post("/login", controllers.Login)
	authGroup.Post("/refresh", controllers.Refresh)
	authGroup.Post("/logout", controllers.Logout)
}