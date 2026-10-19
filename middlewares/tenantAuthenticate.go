package middlewares

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
)

// TenantAuthenticate is retained as a compatibility name for management-token
// authentication. It deliberately delegates to the strict management boundary
// and never accepts application-user tokens. New management routes must use an
// approved route contract rather than registering this middleware directly.
func TenantAuthenticate(c *fiber.Ctx) error {
	return ManagementAuthenticate(c)
}

// RequireProjectScope ensures the request is made within a project context
func RequireProjectScope(c *fiber.Ctx) error {
	roleScope, ok := c.Locals("roleScope").(string)
	if !ok || roleScope != string(models.RoleScopeProject) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Project scope required. Please switch to a project first.",
		})
	}

	projectID, ok := c.Locals("projectID").(string)
	if !ok || projectID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "No project context found",
		})
	}

	return c.Next()
}

// RequireTenantScope ensures the request is made within a tenant context
func RequireTenantScope(c *fiber.Ctx) error {
	roleScope, ok := c.Locals("roleScope").(string)
	if !ok || roleScope != string(models.RoleScopeTenant) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Tenant scope required",
		})
	}

	return c.Next()
}

// TenantOwnerOnly restricts access to tenant owners only
func TenantOwnerOnly(c *fiber.Ctx) error {
	userRoles, ok := c.Locals("roles").([]string)
	if !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "No roles found",
		})
	}

	isOwner := false
	for _, role := range userRoles {
		if role == models.TenantRoleOwner {
			isOwner = true
			break
		}
	}

	if !isOwner {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Tenant owner access required",
		})
	}

	return c.Next()
}

// ProjectAdminOnly restricts access to project admins only
func ProjectAdminOnly(c *fiber.Ctx) error {
	userRoles, ok := c.Locals("roles").([]string)
	if !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "No roles found",
		})
	}

	isAdmin := false
	for _, role := range userRoles {
		if role == models.ProjectRoleAdmin {
			isAdmin = true
			break
		}
	}

	if !isAdmin {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Project admin access required",
		})
	}

	return c.Next()
}
