package controllers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
)

func requireAuthMutationOrigin(c *fiber.Ctx) error {
	origin := c.Get(fiber.HeaderOrigin)
	allowed := []string{"https://tenant.autoapi.org", "https://panel.autoapi.org", "http://localhost:3001", "http://localhost:3005", "http://localhost:5173"}
	if origin == "" || strings.EqualFold(origin, "null") || !middlewares.IsTrustedOrigin(origin, allowed) {
		return c.Status(fiber.StatusForbidden).JSON(responses.GeneralResponse{Status: fiber.StatusForbidden, Message: "Untrusted request origin"})
	}
	return nil
}

func ProjectSession(c *fiber.Ctx) error {
	setAuthNoStoreHeaders(c)
	token := c.Cookies(utils.ProjectAuthCookieName(c.Params("tenantSlug"), c.Params("projectSlug"), "access"))
	userID, role, tenantID, projectID, tenantSlug, projectSlug, err := utils.ParseToken(token)
	if err != nil {
		return sendAnonymousSession(c)
	}
	return c.JSON(fiber.Map{"authenticated": true, "user": fiber.Map{"id": userID, "role": role}, "tenantId": tenantID, "projectId": projectID, "tenantSlug": tenantSlug, "projectSlug": projectSlug})
}

func ProjectRefresh(c *fiber.Ctx) error {
	if err := requireAuthMutationOrigin(c); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, tokens, err := rotateProjectCookieSession(c, ctx)
	if err != nil {
		utils.ClearProjectAuthCookies(c, c.Params("tenantSlug"), c.Params("projectSlug"))
		return refreshErrorResponse(c, err)
	}
	return sendScopedAuthResponse(c, http.StatusOK, "Session refreshed successfully", utils.TokenScopeProject, projectCookieTokens(tokens), nil)
}

func TenantSession(c *fiber.Ctx) error {
	setAuthNoStoreHeaders(c)
	token := c.Cookies(utils.AuthCookieName(utils.TokenScopeTenant, "access"))
	claims, err := utils.ParseTenantToken(token)
	if err != nil {
		return sendAnonymousSession(c)
	}
	return c.JSON(fiber.Map{"authenticated": true, "user": fiber.Map{"id": claims.UserID, "email": claims.Email}, "tenantId": claims.TenantID, "roles": claims.Roles})
}
