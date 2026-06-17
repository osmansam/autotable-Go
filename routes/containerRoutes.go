package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/models"
)

// ContainerRoutes sets up all container management routes
// These routes require tenant authentication and project scope
func ContainerRoutes(baseUrl string, app *fiber.App) {
	// All container routes require tenant authentication and project scope
	app.Get(baseUrl, middlewares.PublicRateLimit(), controllers.GetAllContainers)
	containerGroup := app.Group(baseUrl)
	containerGroup.Use(middlewares.TenantAuthenticate)
	containerGroup.Use(middlewares.GeneralRateLimit())
	containerGroup.Use(middlewares.RequireProjectScope)

	// Create container - requires project admin or developer role
	containerGroup.Post("/",
		// TODO: Re-enable once project role definitions/assignment are complete.
		// middlewares.TenantAuthorize([]string{
		// 	models.ProjectRoleAdmin,
		// 	models.ProjectRoleDeveloper,
		// }),
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.CreateContainer,
	)

	// Reset Redis cache - requires project admin role
	containerGroup.Post("/reset-redis",
		middlewares.TenantAuthorize([]string{models.ProjectRoleAdmin}),
		middlewares.WriteRateLimit(),
		controllers.ResetRedis,
	)

	// Get all containers-tenantlevel - any project member can view
	containerGroup.Get("/tenant", middlewares.SearchRateLimit(), controllers.GetAllContainers)

	// Get container types - any project member can view
	containerGroup.Get("/types", middlewares.SearchRateLimit(), controllers.GetAllContainerTypes)

	// Update dynamic functions - requires project admin or developer role
	containerGroup.Patch("/dynamicFunctions/:id",
		middlewares.TenantAuthorize([]string{
			models.ProjectRoleAdmin,
			models.ProjectRoleDeveloper,
		}),
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdateDynamicFunctions,
	)

	// Update pipelines - requires project admin or developer role
	containerGroup.Patch("/pipelines/:id",
		middlewares.TenantAuthorize([]string{
			models.ProjectRoleAdmin,
			models.ProjectRoleDeveloper,
		}),
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdatePipelines,
	)

	// Update workflows - requires project admin or developer role
	containerGroup.Patch("/workflows/:id",
		middlewares.TenantAuthorize([]string{
			models.ProjectRoleAdmin,
			models.ProjectRoleDeveloper,
		}),
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdateWorkflows,
	)

	// Get single container - any project member can view
	containerGroup.Get("/:id", middlewares.SearchRateLimit(), controllers.GetContainer)

	// Delete container - requires project admin role
	containerGroup.Delete("/:id",
		middlewares.TenantAuthorize([]string{models.ProjectRoleAdmin}),
		middlewares.WriteRateLimit(),
		controllers.DeleteContainer,
	)

	// Update container - requires project admin or developer role
	containerGroup.Patch("/:id",
		middlewares.TenantAuthorize([]string{
			models.ProjectRoleAdmin,
			models.ProjectRoleDeveloper,
		}),
		middlewares.DefaultBodySizeLimit(),
		middlewares.WriteRateLimit(),
		controllers.UpdateContainer,
	)
}
