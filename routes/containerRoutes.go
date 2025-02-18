package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
)

// func ContainerRoutes(baseUrl string, app *fiber.App) {
// 	containerGroup := app.Group(baseUrl)
// 	containerGroup.Post("/", middlewares.ContainerAuthenticate("CreateContainer"), controllers.CreateContainer)
// 	containerGroup.Get("/", middlewares.ContainerAuthenticate("GetAllContainers"), controllers.GetAllContainers)
// 	containerGroup.Patch("/dynamicFunctions/:id", middlewares.ContainerAuthenticate("UpdateDynamicFunctions"), controllers.UpdateDynamicFunctions)
// 	containerGroup.Patch("/pipelines/:id", middlewares.ContainerAuthenticate("UpdatePipelines"), controllers.UpdatePipelines)
// 	containerGroup.Get("/:id", middlewares.ContainerAuthenticate("GetContainer"), controllers.GetContainer)
// 	containerGroup.Delete("/:id", middlewares.ContainerAuthenticate("DeleteContainer"), controllers.DeleteContainer)
// 	containerGroup.Patch("/:id", middlewares.ContainerAuthenticate("UpdateContainer"), controllers.UpdateContainer)
// }
func ContainerRoutes(baseUrl string, app *fiber.App) {
	containerGroup := app.Group(baseUrl)
	containerGroup.Post("/", controllers.CreateContainer)
	containerGroup.Post("/reset-redis", controllers.ResetRedis)
	containerGroup.Get("/", controllers.GetAllContainers)
	containerGroup.Patch("/dynamicFunctions/:id", controllers.UpdateDynamicFunctions)
	containerGroup.Patch("/pipelines/:id", controllers.UpdatePipelines)
	containerGroup.Get("/:id", controllers.GetContainer)
	containerGroup.Delete("/:id", controllers.DeleteContainer)
	containerGroup.Patch("/:id", controllers.UpdateContainer)
}