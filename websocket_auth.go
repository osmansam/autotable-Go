package main

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/utils"
	"github.com/osmansam/autotableGo/ws"
)

func projectWebsocketAuthentication(appConfig *configs.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !websocket.IsWebSocketUpgrade(c) {
			return fiber.ErrUpgradeRequired
		}
		if !middlewares.IsTrustedOrigin(c.Get(fiber.HeaderOrigin), appConfig.CorsWhitelist) {
			return c.SendStatus(fiber.StatusForbidden)
		}
		requestedTenant := c.Params("tenantSlug")
		requestedProject := c.Params("projectSlug")
		token := c.Cookies(utils.ProjectAuthCookieName(requestedTenant, requestedProject, "access"))
		_, _, tenantID, projectID, tenantSlug, projectSlug, err := utils.ParseToken(token)
		if err != nil || tenantSlug == "" || projectSlug == "" {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		if requestedTenant != "" && (requestedTenant != tenantSlug || requestedProject != projectSlug) {
			return c.SendStatus(fiber.StatusForbidden)
		}
		c.Locals(ws.AuthenticatedContextKey, ws.AuthenticatedContext{TenantID: tenantID, ProjectID: projectID, TenantSlug: tenantSlug, ProjectSlug: projectSlug})
		c.Locals("tenantSlug", tenantSlug)
		c.Locals("projectSlug", projectSlug)
		return c.Next()
	}
}
