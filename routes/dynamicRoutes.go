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
	dynamicGroup.Post("/multiple", middlewares.ConditionalAuthentication("CreateMultipleDynamicModelItem"), controllers.CreateMultipleDynamicModelItem)
	dynamicGroup.Get("/", middlewares.ConditionalAuthentication("GetAllDynamicModelItems"), controllers.GetAllDynamicModelItems)
	dynamicGroup.Get("/page",middlewares.ConditionalAuthentication("GetAllDynamicModelItemsWithPagination"),controllers.GetAllDynamicModelItemsWithPagination)
	dynamicGroup.Get("testPipeline",middlewares.ConditionalAuthentication("TestPipeline"),controllers.TestPipeline)
	dynamicGroup.Get("/pipeline", middlewares.ConditionalAuthentication("GetPipeline"), controllers.GetPipeline)
	dynamicGroup.Get("/search", middlewares.ConditionalAuthentication("HandleSearchDynamicModelItem"), controllers.HandleSearchDynamicModelItem)
	dynamicGroup.Get("/filter", middlewares.ConditionalAuthentication("HandleFilterDynamicModelItem"), controllers.HandleFilterDynamicModelItem)
	dynamicGroup.Get("/execute",middlewares.ConditionalAuthentication("ExecuteDynamicCode"),controllers.ExecuteDynamicCode)
	dynamicGroup.Get("/api", middlewares.ConditionalAuthentication("ExecuteDynamicAPI"),controllers.ExecuteDynamicAPI)
	dynamicGroup.Patch("/multiple", middlewares.ConditionalAuthentication("UpdateMultipleDynamicModelItem"), controllers.UpdateMultipleDynamicModelItem)
	dynamicGroup.Delete("/multiple", middlewares.ConditionalAuthentication("DeleteMultipleDynamicModelItem"), controllers.DeleteMultipleDynamicModelItem)
	dynamicGroup.Delete("/:id", middlewares.ConditionalAuthentication("DeleteDynamicModelItem"), controllers.DeleteDynamicModelItem)
	dynamicGroup.Patch("/:id", middlewares.ConditionalAuthentication("UpdateDynamicModelItem"), controllers.UpdateDynamicModelItem)
	dynamicGroup.Get("/:id", middlewares.ConditionalAuthentication("GetDynamicModelItem"), controllers.GetDynamicModelItem)
}

// func DynamicRoutes(baseUrl string, app *fiber.App) {
// 	dynamicGroup := app.Group(baseUrl)
// 	dynamicGroup.Post("/", controllers.CreateDynamicModelItem)
// 	dynamicGroup.Get("/", controllers.GetAllDynamicModelItems)
// 	dynamicGroup.Post("/multiple", controllers.CreateMultipleDynamicModelItem)
// 	dynamicGroup.Get("/page", controllers.GetAllDynamicModelItemsWithPagination)
// 	dynamicGroup.Get("/testPipeline", controllers.TestPipeline)
// 	dynamicGroup.Get("/pipeline", controllers.GetPipeline)
// 	dynamicGroup.Get("/search", controllers.HandleSearchDynamicModelItem)
// 	dynamicGroup.Get("/filter", controllers.HandleFilterDynamicModelItem)
// 	dynamicGroup.Get("/execute", controllers.ExecuteDynamicCode)
// 	dynamicGroup.Get("/api", controllers.ExecuteDynamicAPI)
// 	dynamicGroup.Patch("/multiple", controllers.UpdateMultipleDynamicModelItem)
// 	dynamicGroup.Delete("/multiple", controllers.DeleteMultipleDynamicModelItem)
// 	dynamicGroup.Delete("/:id", controllers.DeleteDynamicModelItem)
// 	dynamicGroup.Patch("/:id", controllers.UpdateDynamicModelItem)
// 	dynamicGroup.Get("/:id", controllers.GetDynamicModelItem)
// }
