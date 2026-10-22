package routes

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/utils"
)

// ContainerRoutes keeps mutations management-only; application sessions receive
// filtered runtime metadata from the three read-only compatibility routes.
func ContainerRoutes(baseUrl string, app *fiber.App) {
	base := "/" + strings.TrimPrefix(baseUrl, "/")
	app.Get(base, containerMetadataReadHandlers(base, controllers.GetRuntimeSchemas, controllers.GetAllContainers)...)

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

	app.Get(base+"/types", containerMetadataReadHandlers(base+"/types", controllers.GetRuntimeSchemaTypes, controllers.GetAllContainerTypes)...)

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

	app.Get(base+"/:id", containerMetadataReadHandlers(base+"/:id", controllers.GetRuntimeSchema, controllers.GetContainer)...)

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

func containerMetadataReadHandlers(path string, runtimeView, managementView fiber.Handler) []fiber.Handler {
	contract, ok := approvedManagementContract("GET", path)
	if !ok || !contract.RuntimeMetadata {
		panic("unapproved runtime container read: " + path)
	}
	return selectContainerReadHandlers(
		[]fiber.Handler{middlewares.PublicRateLimit(), middlewares.ProjectAuthentication, middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), runtimeView},
		strictManagementRouteHandlers("GET", path, middlewares.GeneralRateLimit(), middlewares.SearchRateLimit(), managementView),
	)
}

func selectContainerReadHandlers(runtimeHandlers, managementHandlers []fiber.Handler) []fiber.Handler {
	const runtimeKey = "containerRuntimeMetadata"
	handlers := []fiber.Handler{func(c *fiber.Ctx) error {
		useRuntime := false
		if strings.TrimSpace(c.Get(fiber.HeaderAuthorization)) != "" {
			// Both authentication domains accept Bearer headers. Validate the
			// application token before choosing its strictly filtered view.
			credential, err := middlewares.ResolveProjectCredential(c)
			if err == nil {
				_, _, _, _, _, _, err = utils.ParseToken(credential.Token)
				useRuntime = err == nil
			}
		} else if _, err := middlewares.ResolveManagementCredential(c); err != nil {
			_, appErr := middlewares.ResolveProjectCredential(c)
			tenant, project := middlewares.ProjectSlugsFromRequest(c)
			useRuntime = appErr == nil || c.Cookies(utils.ProjectAuthCookieName(tenant, project, "refresh")) != ""
		}
		c.Locals(runtimeKey, useRuntime)
		return c.Next()
	}}
	guard := func(handler fiber.Handler, runtime bool) fiber.Handler {
		return func(c *fiber.Ctx) error {
			selected, _ := c.Locals(runtimeKey).(bool)
			if selected == runtime {
				return handler(c)
			}
			return c.Next()
		}
	}
	for _, handler := range runtimeHandlers {
		handlers = append(handlers, guard(handler, true))
	}
	for _, handler := range managementHandlers {
		handlers = append(handlers, guard(handler, false))
	}
	return handlers
}
