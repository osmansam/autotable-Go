package controllers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
)

func enforceAuthMutationOrigin(c *fiber.Ctx) bool {
	origin := c.Get(fiber.HeaderOrigin)
	allowed := configs.GetAppConfig().CorsWhitelist
	if origin == "" || strings.EqualFold(origin, "null") || !middlewares.IsTrustedOrigin(origin, allowed) {
		_ = c.Status(fiber.StatusForbidden).JSON(responses.GeneralResponse{Status: fiber.StatusForbidden, Message: "Untrusted request origin"})
		return false
	}
	return true
}

func ProjectSession(c *fiber.Ctx) error {
	setAuthNoStoreHeaders(c)
	token := c.Cookies(utils.ProjectAuthCookieName(c.Params("tenantSlug"), c.Params("projectSlug"), "access"))
	userID, role, tenantID, projectID, tenantSlug, projectSlug, err := utils.ParseToken(token)
	if err != nil {
		// A remaining refresh cookie lets the runtime client's 401 interceptor
		// recover an expired access session before deciding the visitor is anonymous.
		if c.Cookies(utils.ProjectAuthCookieName(c.Params("tenantSlug"), c.Params("projectSlug"), "refresh")) != "" {
			return refreshErrorResponse(c, err)
		}
		return sendAnonymousSession(c)
	}
	return c.JSON(fiber.Map{"authenticated": true, "user": fiber.Map{"id": userID, "role": role}, "tenantId": tenantID, "projectId": projectID, "tenantSlug": tenantSlug, "projectSlug": projectSlug})
}

func ProjectRefresh(c *fiber.Ctx) error {
	if !enforceAuthMutationOrigin(c) {
		return nil
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
	if err != nil || claims.UserID == "" || claims.TenantID == "" || claims.Scope != utils.TokenScopeTenant || claims.RoleScope != string(models.RoleScopeTenant) || claims.ProjectID != "" {
		// Signal recovery only for this tenant session. Application-user and
		// project-management cookies must never authenticate the tenant panel.
		if c.Cookies(utils.AuthCookieName(utils.TokenScopeTenant, "refresh")) != "" {
			return refreshErrorResponse(c, err)
		}
		return sendAnonymousSession(c)
	}
	return c.JSON(fiber.Map{"authenticated": true, "user": fiber.Map{"id": claims.UserID, "email": claims.Email}, "tenantId": claims.TenantID, "roles": claims.Roles})
}

func ManagementProjectRefresh(c *fiber.Ctx) error {
	if !enforceAuthMutationOrigin(c) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, tokens, err := rotateManagementProjectCookieSession(c, ctx)
	if err != nil {
		utils.ClearManagementProjectAuthCookies(c, c.Params("tenantSlug"), c.Params("projectSlug"))
		return refreshErrorResponse(c, err)
	}
	utils.SetManagementProjectAuthCookies(c, c.Params("tenantSlug"), c.Params("projectSlug"), tenantCookieTokens(tokens))
	setAuthNoStoreHeaders(c)
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{Status: http.StatusOK, Message: "Session refreshed successfully"})
}

func ManagementProjectLogout(c *fiber.Ctx) error {
	if !enforceAuthMutationOrigin(c) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	token := c.Cookies(utils.ManagementProjectAuthCookieName(c.Params("tenantSlug"), c.Params("projectSlug"), "refresh"))
	if claims, err := utils.ParseTenantRefreshToken(token); err == nil && claims.Scope == utils.TokenScopeProject {
		_ = revokeManagementAuthSession(ctx, claims.FamilyID)
	}
	utils.ClearManagementProjectAuthCookies(c, c.Params("tenantSlug"), c.Params("projectSlug"))
	setAuthNoStoreHeaders(c)
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{Status: http.StatusOK, Message: "Logged out successfully"})
}
