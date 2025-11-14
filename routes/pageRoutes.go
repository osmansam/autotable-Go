package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
)

func PageRoutes(baseUrl string, app *fiber.App) {
	pageGroup := app.Group(baseUrl)
	pageGroup.Post("/", controllers.CreatePage)
	pageGroup.Get("/", controllers.GetAllPages)
	pageGroup.Get("/:id", controllers.GetPage)
	pageGroup.Patch("/:id", controllers.UpdatePage)
	pageGroup.Delete("/:id", controllers.DeletePage)
}
