package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
)

func ContainerRoutes(baseUrl string, app *fiber.App) {
	containerGroup := app.Group(baseUrl)
	containerGroup.Post("/", controllers.CreateContainer)
	containerGroup.Get("/", controllers.GetAllContainers)
	containerGroup.Patch("/dynamicFunctions/:id", controllers.UpdateDynamicFunctions)
	containerGroup.Patch("/pipelines/:id", controllers.UpdatePipelines)
	containerGroup.Get("/:id", controllers.GetContainer)
	containerGroup.Delete("/:id", controllers.DeleteContainer)
	containerGroup.Patch("/:id", controllers.UpdateContainer)
}