package middlewares

import (
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
)

// TenantAuthenticate middleware validates tenant user JWT tokens
// This is used for container and page routes (NOT for dynamic routes)
func TenantAuthenticate(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing Authorization header",
		})
	}

	// Extract token from "Bearer <token>"
	var token string
	if strings.HasPrefix(authHeader, "Bearer ") {
		token = authHeader[7:]
	} else {
		token = authHeader
	}

	// Parse tenant token
	claims, err := utils.ParseTenantToken(token)
	if err != nil {
		log.Printf("Error parsing tenant token: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid or expired token",
		})
	}

	// Store user context in locals for downstream handlers
	c.Locals("tenantUserID", claims.UserID)
	c.Locals("email", claims.Email)
	c.Locals("tenantID", claims.TenantID)
	c.Locals("projectID", claims.ProjectID)
	c.Locals("roles", claims.Roles)
	c.Locals("roleScope", claims.RoleScope)

	return c.Next()
}

// TenantAuthorize middleware checks if the user has required roles
// Use after TenantAuthenticate
func TenantAuthorize(requiredRoles []string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userRoles, ok := c.Locals("roles").([]string)
		if !ok || len(userRoles) == 0 {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "No project roles found. Switch to a project and use the returned project access token.",
			})
		}

		// Check if user has at least one of the required roles
		hasRole := false
		for _, userRole := range userRoles {
			for _, requiredRole := range requiredRoles {
				if userRole == requiredRole {
					hasRole = true
					break
				}
			}
			if hasRole {
				break
			}
		}

		if !hasRole {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error":         "Insufficient permissions",
				"requiredRoles": requiredRoles,
			})
		}

		return c.Next()
	}
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
