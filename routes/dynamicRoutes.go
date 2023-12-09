// routes.go
package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

func DynamicRoutes(baseUrl string, app *fiber.App) {
	dynamicGroup := app.Group(baseUrl)
	dynamicGroup.Post("/", middlewares.ConditionalAuthentication("CreateDynamicModelItem"), controllers.CreateDynamicModelItem)
	dynamicGroup.Get("/", middlewares.ConditionalAuthentication("GetAllDynamicModelItems"), controllers.GetAllDynamicModelItems)
	dynamicGroup.Get("/page",middlewares.ConditionalAuthentication("GetAllDynamicModelItemsWithPagination"),controllers.GetAllDynamicModelItemsWithPagination)
	dynamicGroup.Get("/pipeline", middlewares.ConditionalAuthentication("GetPipeline"), controllers.GetPipeline)
	dynamicGroup.Get("/search", middlewares.ConditionalAuthentication("HandleSearchDynamicModelItem"), controllers.HandleSearchDynamicModelItem)
	dynamicGroup.Get("/execute",controllers.ExecuteDynamicCode)
	dynamicGroup.Delete("/:id", middlewares.ConditionalAuthentication("DeleteDynamicModelItem"), controllers.DeleteDynamicModelItem)
	dynamicGroup.Patch("/:id", middlewares.ConditionalAuthentication("UpdateDynamicModelItem"), controllers.UpdateDynamicModelItem)
	dynamicGroup.Get("/:id", middlewares.ConditionalAuthentication("GetDynamicModelItem"), controllers.GetDynamicModelItem)
}
