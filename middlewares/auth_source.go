package middlewares

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/utils"
)

type AuthSource string

const (
	AuthSourceBearer   AuthSource = "bearer"
	AuthSourceCookie   AuthSource = "cookie"
	AuthSourceLocalKey            = "authSource"
)

type ResolvedCredential struct {
	Token  string
	Source AuthSource
}

var ErrMissingCredential = errors.New("authentication credential is missing")
var ErrMalformedAuthorization = errors.New("malformed Authorization header")

func ResolveProjectCredential(c *fiber.Ctx) (ResolvedCredential, error) {
	tenantSlug, projectSlug := ProjectSlugsFromRequest(c)
	return resolveCredential(c, utils.ProjectAuthCookieName(tenantSlug, projectSlug, "access"))
}

func ProjectSlugsFromRequest(c *fiber.Ctx) (string, string) {
	tenantSlug, projectSlug := c.Params("tenantSlug"), c.Params("projectSlug")
	if tenantSlug == "" {
		tenantSlug, _ = c.Locals("tenantSlug").(string)
	}
	if projectSlug == "" {
		projectSlug, _ = c.Locals("projectSlug").(string)
	}
	if tenantSlug == "" {
		tenantSlug = c.Query("tenantSlug")
	}
	if projectSlug == "" {
		projectSlug = c.Query("projectSlug")
	}
	return tenantSlug, projectSlug
}

func ResolveTenantCredential(c *fiber.Ctx) (ResolvedCredential, error) {
	if authHeader := strings.TrimSpace(c.Get(fiber.HeaderAuthorization)); authHeader != "" {
		return resolveCredential(c, utils.AuthCookieName(utils.TokenScopeTenant, "access"))
	}
	preferTenant := strings.HasPrefix(c.Path(), "/api/v1/tenant/")
	if !preferTenant {
		tenantSlug, projectSlug := ProjectSlugsFromRequest(c)
		if token := strings.TrimSpace(c.Cookies(utils.ProjectAuthCookieName(tenantSlug, projectSlug, "access"))); token != "" {
			credential := ResolvedCredential{Token: token, Source: AuthSourceCookie}
			c.Locals(AuthSourceLocalKey, credential.Source)
			return credential, nil
		}
	}
	if token := strings.TrimSpace(c.Cookies(utils.AuthCookieName(utils.TokenScopeTenant, "access"))); token != "" {
		credential := ResolvedCredential{Token: token, Source: AuthSourceCookie}
		c.Locals(AuthSourceLocalKey, credential.Source)
		return credential, nil
	}
	return ResolvedCredential{}, ErrMissingCredential
}

func resolveCredential(c *fiber.Ctx, cookieName string) (ResolvedCredential, error) {
	if authHeader := strings.TrimSpace(c.Get(fiber.HeaderAuthorization)); authHeader != "" {
		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) || strings.TrimSpace(strings.TrimPrefix(authHeader, prefix)) == "" {
			return ResolvedCredential{}, ErrMalformedAuthorization
		}
		credential := ResolvedCredential{Token: strings.TrimSpace(strings.TrimPrefix(authHeader, prefix)), Source: AuthSourceBearer}
		c.Locals(AuthSourceLocalKey, credential.Source)
		return credential, nil
	}

	if token := strings.TrimSpace(c.Cookies(cookieName)); token != "" {
		credential := ResolvedCredential{Token: token, Source: AuthSourceCookie}
		c.Locals(AuthSourceLocalKey, credential.Source)
		return credential, nil
	}
	return ResolvedCredential{}, ErrMissingCredential
}
