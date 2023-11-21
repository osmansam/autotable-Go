package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
)

func DynamicRoutes(baseUrl string, app *fiber.App) {
	dynamicGroup := app.Group(baseUrl)
	dynamicGroup.Post("/", controllers.CreateDynamicModelItem)
	dynamicGroup.Get("/",controllers.GetAllDynamicModelItems)
	dynamicGroup.Get("/pipeline", controllers.GetPipeline)
	dynamicGroup.Get("/search", controllers.HandleSearchDynamicModelItem)
	dynamicGroup.Delete("/:id", controllers.DeleteDynamicModelItem)
	dynamicGroup.Patch("/:id", controllers.UpdateDynamicModelItem)
	dynamicGroup.Get("/:id", controllers.GetDynamicModelItem)
}