package middlewares

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/services"
)

func isProtectedAccountContainer(container *models.ContainerModel) bool {
	return container != nil && (container.IsAuthContainer || container.SchemaName == "role")
}

func accountOperationPermission(operation string) services.ManagementPermission {
	switch operation {
	case "GetAllDynamicModelItems", "GetDynamicModelItem", "GetAllDynamicModelItemsWithPagination", "HandleSearchDynamicModelItem", "HandleFilterDynamicModelItem", "GetItemsForSelection", "ExportDynamicModelItems":
		return services.PermissionProjectConfigRead
	default:
		// Workflows, APIs, and table sources may have side effects even when
		// invoked through GET. Future operations also fail closed to writers.
		return services.PermissionProjectSchemaWrite
	}
}

func accountOperationEnabled(c *fiber.Ctx, operation string, container *models.ContainerModel) bool {
	if usesPipelineAuthentication(operation, c.Query("sourceType")) {
		for _, pipeline := range container.Pipelines {
			if pipeline.Name == c.Query("pipelineName") {
				return pipeline.IsActive
			}
		}
		return false
	}
	if usesWorkflowAuthentication(operation, c.Query("sourceType")) {
		name := c.Params("workflowName", c.Query("workflowName"))
		for _, workflow := range container.Workflows {
			if workflow.Name == name {
				return workflow.IsActive
			}
		}
		return false
	}
	if operation == "ExecuteDynamicAPI" {
		for _, api := range container.DynamicApis {
			if api.Name == c.Query("apiName") {
				return api.IsActive
			}
		}
		return false
	}
	return true // Management account CRUD is independent of public data-route flags.
}

type liveAccountAuthorizer struct{}

func (liveAccountAuthorizer) Authorize(ctx context.Context, subject services.ManagementAuthorizationSubject, target services.ManagementAuthorizationTarget, permission services.ManagementPermission) error {
	return services.NewManagementAuthorizationService(repositories.NewManagementMembershipRepository()).Authorize(ctx, subject, target, permission)
}

func requireAccountManagement(c *fiber.Ctx, tenantID, projectID string, permission services.ManagementPermission, authorizer services.ManagementAuthorizer, operationEnabled bool) error {
	if err := authenticateManagementPrincipal(c); err != nil {
		return err
	}
	if c.Response().StatusCode() >= 400 {
		return nil
	}
	principal, ok := ManagementPrincipalFromContext(c)
	if !ok || principal.ProjectID != projectID || principal.TenantID != tenantID || projectID == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Management token does not match requested project"})
	}
	err := authorizer.Authorize(c.UserContext(), services.ManagementAuthorizationSubject{UserID: principal.UserID, TenantID: principal.TenantID, TokenRoles: principal.TokenRoles}, services.ManagementAuthorizationTarget{TenantID: tenantID, ProjectID: projectID}, permission)
	if errors.Is(err, services.ErrManagementAuthorizationUnavailable) {
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	if err != nil {
		return c.SendStatus(fiber.StatusForbidden)
	}
	if !operationEnabled {
		return c.SendStatus(fiber.StatusForbidden)
	}
	return c.Next()
}
