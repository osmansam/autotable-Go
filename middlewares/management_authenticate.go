package middlewares

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/services"
	"github.com/osmansam/autotableGo/utils"
)

const ManagementPrincipalLocalKey = "managementPrincipal"

type ManagementPrincipal = models.ManagementPrincipal

func ManagementPrincipalFromContext(c *fiber.Ctx) (ManagementPrincipal, bool) {
	principal, ok := c.Locals(ManagementPrincipalLocalKey).(ManagementPrincipal)
	return principal, ok
}

type ManagementTargetResolver func(*fiber.Ctx) (services.ManagementAuthorizationTarget, error)

func RequireManagementPermission(authorizer services.ManagementAuthorizer, permission services.ManagementPermission, targetResolver ManagementTargetResolver) fiber.Handler {
	return func(c *fiber.Ctx) error {
		principal, ok := ManagementPrincipalFromContext(c)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Management authentication required"})
		}
		target, err := targetResolver(c)
		if err != nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Invalid management authorization target"})
		}
		err = authorizer.Authorize(c.UserContext(), services.ManagementAuthorizationSubject{
			UserID: principal.UserID, TenantID: principal.TenantID, TokenRoles: append([]string(nil), principal.TokenRoles...),
		}, target, permission)
		if errors.Is(err, services.ErrManagementAuthorizationUnavailable) {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Management authorization is temporarily unavailable"})
		}
		if err != nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Management permission denied"})
		}
		return c.Next()
	}
}

func ManagementAuthenticate(c *fiber.Ctx) error {
	if err := authenticateManagementPrincipal(c); err != nil {
		return err
	}
	if c.Response().StatusCode() >= 400 {
		return nil
	}
	return c.Next()
}

// authenticateManagementPrincipal validates/populates identity without advancing
// the Fiber chain, allowing conditional management boundaries to authorize first.
func authenticateManagementPrincipal(c *fiber.Ctx) error {
	credential, err := ResolveManagementCredential(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing or invalid management credential"})
	}
	if credential.Source == AuthSourceCookie && !isSafeMethod(c.Method()) && !IsTrustedOrigin(c.Get(fiber.HeaderOrigin), configs.GetAppConfig().CorsWhitelist) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Untrusted request origin"})
	}

	claims, err := utils.ParseTenantToken(credential.Token)
	if err != nil || !validManagementClaims(claims) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid or expired management token"})
	}

	principal := ManagementPrincipal{
		UserID: claims.UserID, Email: claims.Email, TenantID: claims.TenantID, ProjectID: claims.ProjectID,
		Scope: claims.Scope, RoleScope: claims.RoleScope, TokenRoles: append([]string(nil), claims.Roles...),
		TokenID: claims.Id, FamilyID: claims.FamilyID,
	}
	c.Locals(ManagementPrincipalLocalKey, principal)
	// Temporary controller adapter; authorization must use ManagementPrincipal and live membership.
	c.Locals("tenantUserID", principal.UserID)
	c.Locals("email", principal.Email)
	c.Locals("tenantID", principal.TenantID)
	c.Locals("projectID", principal.ProjectID)
	c.Locals("roles", append([]string(nil), principal.TokenRoles...))
	c.Locals("roleScope", principal.RoleScope)
	return nil
}

func validManagementClaims(claims *utils.TenantTokenClaims) bool {
	if claims == nil || claims.UserID == "" || claims.TenantID == "" {
		return false
	}
	switch claims.Scope {
	case utils.TokenScopeTenant:
		return claims.RoleScope == string(models.RoleScopeTenant) && claims.ProjectID == ""
	case utils.TokenScopeProject:
		return claims.RoleScope == string(models.RoleScopeProject) && claims.ProjectID != ""
	default:
		return false
	}
}

func RequireManagementTenantScope(c *fiber.Ctx) error {
	principal, ok := ManagementPrincipalFromContext(c)
	if !ok || principal.Scope != utils.TokenScopeTenant || principal.ProjectID != "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Tenant management scope required"})
	}
	return c.Next()
}

func RequireManagementProjectScope(c *fiber.Ctx) error {
	principal, ok := ManagementPrincipalFromContext(c)
	if !ok || principal.Scope != utils.TokenScopeProject || principal.ProjectID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Project management scope required"})
	}
	return c.Next()
}

var resolveManagementProject = utils.GetTenantAndProjectFromSlugs

func BindManagementProject(c *fiber.Ctx) error {
	principal, ok := ManagementPrincipalFromContext(c)
	if !ok || principal.Scope != utils.TokenScopeProject || principal.TenantID == "" || principal.ProjectID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Project management scope required"})
	}

	tenantID, projectID, err := resolveManagementProject(c)
	if err != nil || tenantID != principal.TenantID || projectID != principal.ProjectID {
		// A resolver may populate legacy locals; rejected URL context must not survive.
		c.Locals("tenantID", principal.TenantID)
		c.Locals("projectID", principal.ProjectID)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Management token does not match requested project"})
	}
	return c.Next()
}

// OverrideManagementProjectResolver replaces the slug resolver used by
// BindManagementProject and returns a function that restores the previous one.
// It exists so route-level authorization tests can run without a database.
func OverrideManagementProjectResolver(resolver func(*fiber.Ctx) (string, string, error)) (restore func()) {
	previous := resolveManagementProject
	resolveManagementProject = resolver
	return func() { resolveManagementProject = previous }
}
