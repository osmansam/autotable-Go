package routes

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

// ContainerRoutes sets up all container management routes.
func ContainerRoutes(baseUrl string, app *fiber.App) {
	base := "/" + strings.TrimPrefix(baseUrl, "/")
	app.Get(base, strictManagementRouteHandlers("GET", base, middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), controllers.GetAllContainers)...)

	app.Post(base+"/", strictManagementRouteHandlers("POST", base+"/",
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.CreateContainer,
	)...)
	app.Post(base+"/create-multiple", strictManagementRouteHandlers("POST", base+"/create-multiple",
		middlewares.BulkWriteBodySizeLimit(), middlewares.BulkRateLimit(), controllers.CreateMultipleContainers,
	)...)

	app.Post(base+"/reset-redis", strictManagementRouteHandlers("POST", base+"/reset-redis",
		middlewares.WriteRateLimit(),
		controllers.ResetRedis,
	)...)

	app.Get(base+"/tenant", strictManagementRouteHandlers("GET", base+"/tenant", middlewares.SearchRateLimit(), controllers.GetAllContainers)...)

	app.Get(base+"/types", strictManagementRouteHandlers("GET", base+"/types", middlewares.SearchRateLimit(), controllers.GetAllContainerTypes)...)

	// Dynamic functions are paused; retain the update route for future use.
	// app.Patch(base+"/dynamicFunctions/:id", strictManagementRouteHandlers("PATCH", base+"/dynamicFunctions/:id",
	// 	middlewares.DefaultBodySizeLimit(),
	// 	middlewares.WriteRateLimit(),
	// 	controllers.UpdateDynamicFunctions,
	// )...)

	app.Patch(base+"/pipelines/:id", strictManagementRouteHandlers("PATCH", base+"/pipelines/:id",
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdatePipelines,
	)...)

	app.Patch(base+"/workflows/:id", strictManagementRouteHandlers("PATCH", base+"/workflows/:id",
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdateWorkflows,
	)...)

	app.Patch(base+"/dynamicApis/:id", strictManagementRouteHandlers("PATCH", base+"/dynamicApis/:id",
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdateDynamicApis,
	)...)

	app.Get(base+"/:id", strictManagementRouteHandlers("GET", base+"/:id", middlewares.SearchRateLimit(), controllers.GetContainer)...)

	app.Delete(base+"/:id", strictManagementRouteHandlers("DELETE", base+"/:id",
		middlewares.WriteRateLimit(),
		controllers.DeleteContainer,
	)...)

	app.Patch(base+"/:id", strictManagementRouteHandlers("PATCH", base+"/:id",
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdateContainer,
	)...)
}
