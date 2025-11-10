package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
)

func SwaggerRoutes(app *fiber.App) {
	// Swagger UI route
	app.Get("/api/swagger", controllers.GetSwaggerUI)
	
	// Swagger JSON specification
	app.Get("/api/swagger.json", controllers.GenerateDynamicSwagger)
	
	// List all available schemas
	app.Get("/api/schemas", controllers.ListAllSchemas)
}