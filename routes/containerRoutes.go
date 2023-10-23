package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
)

func ContainerRoutes(baseUrl string, app *fiber.App) {
	containerGroup := app.Group(baseUrl)
	containerGroup.Post("/", controllers.CreateContainer)
	containerGroup.Get("/", controllers.GetAllContainers)
	containerGroup.Delete("/:id", controllers.DeleteContainer)
	containerGroup.Patch("/:id", controllers.UpdateContainer)
}