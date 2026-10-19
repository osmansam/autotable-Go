package routes

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

func SchemaInfoRoutes(baseUrl string, app *fiber.App) {
	base := "/" + strings.TrimPrefix(baseUrl, "/")
	app.Get(base+"/roles", strictManagementRouteHandlers("GET", base+"/roles",
		middlewares.GeneralRateLimit(),
		middlewares.SearchRateLimit(),
		controllers.GetRoleItems)...)

	app.Get(base+"/roles/:id", strictManagementRouteHandlers("GET", base+"/roles/:id",
		middlewares.GeneralRateLimit(),
		middlewares.SearchRateLimit(),
		controllers.GetRoleItemById)...)
}
