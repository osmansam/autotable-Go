package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	TokenScopeTenant  = "tenant"
	TokenScopeProject = "project"
)

type AuthTokenPair struct {
	AccessToken  string
	RefreshToken string
	ATExpires    int64
	RTExpires    int64
}

func AuthCookieName(scope, kind string) string {
	name := scope + "_" + kind + "_token"
	if isProductionCookieEnvironment() {
		return "__Host-" + name
	}
	return name
}

func ProjectAuthCookieName(tenantSlug, projectSlug, kind string) string {
	name := projectAuthCookieBaseName(tenantSlug, projectSlug, kind)
	if isProductionCookieEnvironment() {
		return "__Secure-" + name
	}
	return name
}

func projectAuthCookieBaseName(tenantSlug, projectSlug, kind string) string {
	identity := strings.ToLower(strings.TrimSpace(tenantSlug)) + "/" + strings.ToLower(strings.TrimSpace(projectSlug))
	digest := sha256.Sum256([]byte(identity))
	return "project_v2_" + hex.EncodeToString(digest[:]) + "_" + kind + "_token"
}

func legacyProjectAuthCookieName(tenantSlug, projectSlug, kind string) string {
	identity := strings.ToLower(strings.TrimSpace(tenantSlug)) + "/" + strings.ToLower(strings.TrimSpace(projectSlug))
	digest := sha256.Sum256([]byte(identity))
	name := "project_" + hex.EncodeToString(digest[:]) + "_" + kind + "_token"
	if isProductionCookieEnvironment() {
		return "__Host-" + name
	}
	return name
}

func ProjectAuthCookiePath(tenantSlug, projectSlug string) string {
	return "/api/v1/" + strings.ToLower(strings.TrimSpace(tenantSlug)) + "/" + strings.ToLower(strings.TrimSpace(projectSlug))
}

func SetProjectAuthCookies(c *fiber.Ctx, tenantSlug, projectSlug string, tokens AuthTokenPair) {
	path := ProjectAuthCookiePath(tenantSlug, projectSlug)
	setAuthCookieWithPath(c, ProjectAuthCookieName(tenantSlug, projectSlug, "access"), tokens.AccessToken, tokens.ATExpires, path)
	setAuthCookieWithPath(c, ProjectAuthCookieName(tenantSlug, projectSlug, "refresh"), tokens.RefreshToken, tokens.RTExpires, path)
	clearAuthCookieWithPath(c, legacyProjectAuthCookieName(tenantSlug, projectSlug, "access"), "/")
	clearAuthCookieWithPath(c, legacyProjectAuthCookieName(tenantSlug, projectSlug, "refresh"), "/")
}

func ClearProjectAuthCookies(c *fiber.Ctx, tenantSlug, projectSlug string) {
	path := ProjectAuthCookiePath(tenantSlug, projectSlug)
	clearAuthCookieWithPath(c, ProjectAuthCookieName(tenantSlug, projectSlug, "access"), path)
	clearAuthCookieWithPath(c, ProjectAuthCookieName(tenantSlug, projectSlug, "refresh"), path)
}

func SetAuthCookies(c *fiber.Ctx, scope string, tokens AuthTokenPair) {
	setAuthCookie(c, AuthCookieName(scope, "access"), tokens.AccessToken, tokens.ATExpires)
	setAuthCookie(c, AuthCookieName(scope, "refresh"), tokens.RefreshToken, tokens.RTExpires)
}

func ClearAuthCookies(c *fiber.Ctx, scope string) {
	clearAuthCookie(c, AuthCookieName(scope, "access"))
	clearAuthCookie(c, AuthCookieName(scope, "refresh"))
}

func setAuthCookie(c *fiber.Ctx, name, value string, expiresAt int64) {
	setAuthCookieWithPath(c, name, value, expiresAt, "/")
}

func setAuthCookieWithPath(c *fiber.Ctx, name, value string, expiresAt int64, path string) {
	expires := time.Unix(expiresAt, 0)
	maxAge := int(time.Until(expires).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		Expires:  expires,
		MaxAge:   maxAge,
		Secure:   isProductionCookieEnvironment(),
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode,
	})
}

func clearAuthCookie(c *fiber.Ctx, name string) {
	clearAuthCookieWithPath(c, name, "/")
}

func clearAuthCookieWithPath(c *fiber.Ctx, name, path string) {
	c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    "",
		Path:     path,
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
		Secure:   isProductionCookieEnvironment(),
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode,
	})
}

func isProductionCookieEnvironment() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("NODE_ENV")), "production")
}
