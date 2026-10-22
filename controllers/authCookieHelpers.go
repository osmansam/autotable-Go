package controllers

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/services"
	"github.com/osmansam/autotableGo/utils"
)

var errMissingRefreshCookie = errors.New("refresh cookie is missing")

var rotateProjectAuthSession = func(ctx context.Context, familyID, presentedID, nextID string, expiresAt time.Time) error {
	return services.NewAuthSessionService(repositories.NewAuthSessionRepository()).Rotate(ctx, familyID, presentedID, nextID, expiresAt)
}

var rotateManagementAuthSession = func(ctx context.Context, familyID, presentedID, nextID string, expiresAt time.Time) error {
	return services.NewAuthSessionService(repositories.NewAuthSessionRepository()).Rotate(ctx, familyID, presentedID, nextID, expiresAt)
}

var revokeManagementAuthSession = func(ctx context.Context, familyID string) error {
	return services.NewAuthSessionService(repositories.NewAuthSessionRepository()).RevokeFamily(ctx, familyID)
}

func sendScopedAuthResponse(c *fiber.Ctx, status int, message, scope string, tokens utils.AuthTokenPair, data interface{}) error {
	if scope == utils.TokenScopeProject {
		utils.SetProjectAuthCookies(c, c.Params("tenantSlug"), c.Params("projectSlug"), tokens)
	} else {
		utils.SetAuthCookies(c, scope, tokens)
	}
	setAuthNoStoreHeaders(c)
	return c.Status(status).JSON(responses.GeneralResponse{
		Status:  status,
		Message: message,
		Data:    data,
	})
}

func sendProjectAuthResponse(c *fiber.Ctx, status int, message, tenantSlug, projectSlug string, tokens utils.AuthTokenPair, data interface{}) error {
	utils.SetManagementProjectAuthCookies(c, tenantSlug, projectSlug, tokens)
	setAuthNoStoreHeaders(c)
	return c.Status(status).JSON(responses.GeneralResponse{Status: status, Message: message, Data: data})
}

func sendAnonymousSession(c *fiber.Ctx) error {
	setAuthNoStoreHeaders(c)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"authenticated": false})
}

func setAuthNoStoreHeaders(c *fiber.Ctx) {
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("Pragma", "no-cache")
}

func startProjectAuthSession(ctx context.Context, tokens *utils.TokenDetails) error {
	claims, err := utils.ParseProjectRefreshToken(tokens.RefreshToken)
	if err != nil {
		return err
	}
	return services.NewAuthSessionService(repositories.NewAuthSessionRepository()).Start(ctx, services.AuthSessionStart{
		FamilyID: claims.FamilyID, RefreshTokenID: claims.ID, Scope: utils.TokenScopeProject,
		UserID: claims.UserID, TenantID: claims.TenantID, ProjectID: claims.ProjectID,
		ExpiresAt: time.Unix(tokens.RTExpires, 0),
	})
}

func startTenantAuthSession(ctx context.Context, tokens *utils.TenantTokenDetails) error {
	claims, err := utils.ParseTenantRefreshToken(tokens.RefreshToken)
	if err != nil {
		return err
	}
	return services.NewAuthSessionService(repositories.NewAuthSessionRepository()).Start(ctx, services.AuthSessionStart{
		FamilyID: claims.FamilyID, RefreshTokenID: claims.Id, Scope: claims.Scope,
		UserID: claims.UserID, TenantID: claims.TenantID, ProjectID: claims.ProjectID,
		ExpiresAt: time.Unix(tokens.RTExpires, 0),
	})
}

func projectCookieTokens(tokens *utils.TokenDetails) utils.AuthTokenPair {
	return utils.AuthTokenPair{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken, ATExpires: tokens.ATExpires, RTExpires: tokens.RTExpires}
}

func tenantCookieTokens(tokens *utils.TenantTokenDetails) utils.AuthTokenPair {
	return utils.AuthTokenPair{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken, ATExpires: tokens.ATExpires, RTExpires: tokens.RTExpires}
}

func rotateTenantCookieSession(c *fiber.Ctx, ctx context.Context) (*utils.TenantTokenClaims, *utils.TenantTokenDetails, error) {
	token := c.Cookies(utils.AuthCookieName(utils.TokenScopeTenant, "refresh"))
	if token == "" {
		return nil, nil, errMissingRefreshCookie
	}
	claims, err := utils.ParseTenantRefreshToken(token)
	if err != nil {
		return nil, nil, err
	}
	if claims.Scope != utils.TokenScopeTenant || claims.ProjectID != "" || claims.RoleScope != string(models.RoleScopeTenant) {
		return nil, nil, errors.New("tenant refresh token has invalid scope")
	}
	snapshot, err := loadManagementMembership(ctx, models.ManagementAuthorizationSubject{UserID: claims.UserID, TenantID: claims.TenantID}, models.ManagementAuthorizationTarget{TenantID: claims.TenantID})
	if err != nil {
		return nil, nil, err
	}
	roles, err := services.CurrentManagementSessionRoles(snapshot, false)
	if err != nil {
		return nil, nil, err
	}
	tokens, err := utils.GenerateTenantTokensForFamily(claims.UserID, claims.Email, claims.TenantID, "", roles, string(models.RoleScopeTenant), claims.FamilyID)
	if err != nil {
		return nil, nil, err
	}
	next, err := utils.ParseTenantRefreshToken(tokens.RefreshToken)
	if err != nil {
		return nil, nil, err
	}
	err = rotateManagementAuthSession(ctx, claims.FamilyID, claims.Id, next.Id, time.Unix(tokens.RTExpires, 0))
	return claims, tokens, err
}

var loadManagementMembership = func(ctx context.Context, subject models.ManagementAuthorizationSubject, target models.ManagementAuthorizationTarget) (models.ManagementMembershipSnapshot, error) {
	return repositories.NewManagementMembershipRepository().LoadManagementMembership(ctx, subject, target)
}

var resolveManagementProjectForRefresh = utils.GetTenantAndProjectFromSlugs

func rotateManagementProjectCookieSession(c *fiber.Ctx, ctx context.Context) (*utils.TenantTokenClaims, *utils.TenantTokenDetails, error) {
	tenantSlug, projectSlug := c.Params("tenantSlug"), c.Params("projectSlug")
	token := c.Cookies(utils.ManagementProjectAuthCookieName(tenantSlug, projectSlug, "refresh"))
	if token == "" {
		return nil, nil, errMissingRefreshCookie
	}
	claims, err := utils.ParseTenantRefreshToken(token)
	if err != nil {
		return nil, nil, err
	}
	if claims.Scope != utils.TokenScopeProject || claims.ProjectID == "" || claims.RoleScope != string(models.RoleScopeProject) {
		return nil, nil, errors.New("project management refresh token has invalid scope")
	}
	tenantID, projectID, err := resolveManagementProjectForRefresh(c)
	if err != nil || tenantID != claims.TenantID || projectID != claims.ProjectID {
		return nil, nil, errors.New("project management refresh token does not match requested project")
	}
	snapshot, err := loadManagementMembership(ctx, models.ManagementAuthorizationSubject{UserID: claims.UserID, TenantID: claims.TenantID}, models.ManagementAuthorizationTarget{TenantID: tenantID, ProjectID: projectID})
	if err != nil {
		return nil, nil, err
	}
	roles, err := services.CurrentManagementSessionRoles(snapshot, true)
	if err != nil {
		return nil, nil, err
	}
	tokens, err := utils.GenerateTenantTokensForFamily(claims.UserID, claims.Email, tenantID, projectID, roles, string(models.RoleScopeProject), claims.FamilyID)
	if err != nil {
		return nil, nil, err
	}
	next, err := utils.ParseTenantRefreshToken(tokens.RefreshToken)
	if err != nil {
		return nil, nil, err
	}
	err = rotateManagementAuthSession(ctx, claims.FamilyID, claims.Id, next.Id, time.Unix(tokens.RTExpires, 0))
	return claims, tokens, err
}

func rotateProjectCookieSession(c *fiber.Ctx, ctx context.Context) (*utils.ProjectTokenClaims, *utils.TokenDetails, error) {
	tenantSlug, projectSlug := c.Params("tenantSlug"), c.Params("projectSlug")
	token := c.Cookies(utils.ProjectAuthCookieName(tenantSlug, projectSlug, "refresh"))
	if token == "" {
		return nil, nil, errMissingRefreshCookie
	}
	claims, err := utils.ParseProjectRefreshToken(token)
	if err != nil {
		return nil, nil, err
	}
	tokens, err := utils.GenerateTokensForFamily(claims.UserID, claims.Role, claims.TenantID, claims.ProjectID, claims.TenantSlug, claims.ProjectSlug, claims.DisplayName, claims.FamilyID)
	if err != nil {
		return nil, nil, err
	}
	next, err := utils.ParseProjectRefreshToken(tokens.RefreshToken)
	if err != nil {
		return nil, nil, err
	}
	err = rotateProjectAuthSession(ctx, claims.FamilyID, claims.ID, next.ID, time.Unix(tokens.RTExpires, 0))
	return claims, tokens, err
}

func refreshErrorResponse(c *fiber.Ctx, err error) error {
	setAuthNoStoreHeaders(c)
	if errors.Is(err, services.ErrRefreshConflict) {
		return c.Status(fiber.StatusConflict).JSON(responses.GeneralResponse{Status: fiber.StatusConflict, Message: "Session was refreshed by another request; retry"})
	}
	return c.Status(fiber.StatusUnauthorized).JSON(responses.GeneralResponse{Status: fiber.StatusUnauthorized, Message: "Session expired. Please log in again."})
}
