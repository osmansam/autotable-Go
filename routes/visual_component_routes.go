package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

func VisualComponentRoutes(app *fiber.App) {
	const base = "/api/v1/:tenantSlug/:projectSlug/admin/visual-components"
	app.Get(base, strictManagementRouteHandlers("GET", base, middlewares.SearchRateLimit(), controllers.VisualComponents.List)...)
	app.Post(base, strictManagementRouteHandlers("POST", base, middlewares.DefaultBodySizeLimit(), middlewares.WriteRateLimit(), controllers.VisualComponents.Save)...)
	app.Put(base+"/:id", strictManagementRouteHandlers("PUT", base+"/:id", middlewares.DefaultBodySizeLimit(), middlewares.WriteRateLimit(), controllers.VisualComponents.Save)...)
}
