// routes.go
package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

func DynamicRoutes(baseUrl string, app *fiber.App) {
	dynamicGroup := app.Group(baseUrl)
	dynamicGroup.Post("/", middlewares.DefaultBodySizeLimit(), middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("CreateDynamicModelItem"), middlewares.GeneralRateLimit(), middlewares.WriteRateLimit(), controllers.CreateDynamicModelItem)
	dynamicGroup.Post("/multiple", middlewares.BulkWriteBodySizeLimit(), middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("CreateMultipleDynamicModelItem"), middlewares.GeneralRateLimit(), middlewares.BulkRateLimit(), controllers.CreateMultipleDynamicModelItem)
	dynamicGroup.Post("/export", middlewares.ExportBodySizeLimit(), middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("ExportDynamicModelItems"), middlewares.GeneralRateLimit(), middlewares.ExportRateLimit(), controllers.ExportDynamicModelItems)
	dynamicGroup.Get("/", middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("GetAllDynamicModelItems"), middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.GetAllDynamicModelItems)
	dynamicGroup.Get("/page", middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("GetAllDynamicModelItemsWithPagination"), middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.GetAllDynamicModelItemsWithPagination)
	dynamicGroup.Get("/testPipeline", middlewares.DefaultBodySizeLimit(), middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("TestPipeline"), middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.TestPipeline)
	dynamicGroup.Get("/pipeline", middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("GetPipeline"), middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.GetPipeline)
	dynamicGroup.Get("/search", middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("HandleSearchDynamicModelItem"), middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.HandleSearchDynamicModelItem)
	dynamicGroup.Get("/filter", middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("HandleFilterDynamicModelItem"), middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.HandleFilterDynamicModelItem)
	dynamicGroup.Get("/execute", middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("ExecuteDynamicCode"), middlewares.GeneralRateLimit(), middlewares.ExecuteRateLimit(), controllers.ExecuteDynamicCode)
	dynamicGroup.Get("/api", middlewares.DefaultBodySizeLimit(), middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("ExecuteDynamicAPI"), middlewares.GeneralRateLimit(), middlewares.ExecuteRateLimit(), controllers.ExecuteDynamicAPI)
	dynamicGroup.Patch("/multiple", middlewares.BulkUpdateBodySizeLimit(), middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("UpdateMultipleDynamicModelItem"), middlewares.GeneralRateLimit(), middlewares.BulkRateLimit(), controllers.UpdateMultipleDynamicModelItem)
	dynamicGroup.Delete("/multiple", middlewares.BulkDeleteBodySizeLimit(), middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("DeleteMultipleDynamicModelItem"), middlewares.GeneralRateLimit(), middlewares.BulkRateLimit(), controllers.DeleteMultipleDynamicModelItem)
	dynamicGroup.Delete("/:id", middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("DeleteDynamicModelItem"), middlewares.GeneralRateLimit(), middlewares.WriteRateLimit(), controllers.DeleteDynamicModelItem)
	dynamicGroup.Patch("/:id", middlewares.DefaultBodySizeLimit(), middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("UpdateDynamicModelItem"), middlewares.GeneralRateLimit(), middlewares.WriteRateLimit(), controllers.UpdateDynamicModelItem)
	dynamicGroup.Get("/selection", middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("GetItemsForSelection"), middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.GetItemsForSelection)
	dynamicGroup.Get("/:id", middlewares.PublicRateLimit(), middlewares.ConditionalAuthentication("GetDynamicModelItem"), middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.GetDynamicModelItem)
}

// func DynamicRoutes(baseUrl string, app *fiber.App) {
// 	dynamicGroup := app.Group(baseUrl)
// 	dynamicGroup.Post("/", controllers.CreateDynamicModelItem)
// 	dynamicGroup.Post("/export", controllers.ExportDynamicModelItems)
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
// 	dynamicGroup.Get("/selection", controllers.GetItemsForSelection)
// 	dynamicGroup.Get("/:id", controllers.GetDynamicModelItem)
// }
