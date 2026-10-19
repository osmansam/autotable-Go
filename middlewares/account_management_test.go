package middlewares

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/services"
	"github.com/osmansam/autotableGo/utils"
)

type accountTestAuthorizer func(context.Context, services.ManagementAuthorizationSubject, services.ManagementAuthorizationTarget, services.ManagementPermission) error

func (f accountTestAuthorizer) Authorize(ctx context.Context, s services.ManagementAuthorizationSubject, target services.ManagementAuthorizationTarget, p services.ManagementPermission) error {
	return f(ctx, s, target, p)
}

func TestAccountOperationsRequireWritePermissionUnlessExplicitlyReadOnly(t *testing.T) {
	for _, operation := range []string{"ExecuteWorkflow", "ExecuteDynamicAPI", "GetTableSource", "GetPipeline", "CreateDynamicModelItem", "unknown future operation"} {
		if permission := accountOperationPermission(operation); permission != services.PermissionProjectSchemaWrite {
			t.Errorf("%s permits execution with read permission: %s", operation, permission)
		}
	}
	for _, operation := range []string{"GetAllDynamicModelItems", "GetDynamicModelItem", "ExportDynamicModelItems"} {
		if permission := accountOperationPermission(operation); permission != services.PermissionProjectConfigRead {
			t.Errorf("%s cannot be read by management readers: %s", operation, permission)
		}
	}
}

func TestAccountExecutionPreservesActivationChecks(t *testing.T) {
	for _, test := range []struct {
		operation, url string
		active         bool
	}{
		{"ExecuteDynamicAPI", "/?apiName=selected", false},
		{"ExecuteDynamicAPI", "/?apiName=selected", true},
		{"GetPipeline", "/?pipelineName=selected", false},
		{"GetPipeline", "/?pipelineName=selected", true},
		{"GetTableSource", "/?sourceType=workflow&workflowName=selected", false},
		{"GetTableSource", "/?sourceType=pipeline&pipelineName=selected", false},
		{"ExecuteWorkflow", "/?workflowName=selected", false},
	} {
		t.Run(test.operation+test.url+fmt.Sprint(test.active), func(t *testing.T) {
			container := &models.ContainerModel{DynamicApis: []models.DynamicApiModel{{Name: "selected", IsActive: test.active}}, Pipelines: []models.PipelineStage{{Name: "selected", IsActive: test.active}}, Workflows: []models.DynamicWorkflow{{Name: "selected", IsActive: test.active}}}
			app := fiber.New()
			app.Get("/", func(c *fiber.Ctx) error {
				if accountOperationEnabled(c, test.operation, container) {
					return c.SendStatus(204)
				}
				return c.SendStatus(403)
			})
			response, err := app.Test(httptest.NewRequest("GET", test.url, nil))
			if err != nil {
				t.Fatal(err)
			}
			if (response.StatusCode == 204) != test.active {
				t.Fatalf("status=%d active=%v", response.StatusCode, test.active)
			}
		})
	}
}

func TestAccountManagementBoundary(t *testing.T) {
	t.Setenv("JWT_SECRET", "project-test-secret")
	t.Setenv("TENANT_JWT_SECRET", "management-test-secret")
	management, err := utils.GenerateTenantTokens("operator", "a@b.com", "tenant", "project", []string{models.ProjectRoleAdmin}, string(models.RoleScopeProject))
	if err != nil {
		t.Fatal(err)
	}
	endUser, err := utils.GenerateTokens("person", "admin", "tenant", "project", "tenant", "project")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, token string
		denied      bool
		disabled    bool
		status      int
	}{
		{name: "anonymous", status: 401}, {name: "application token", token: endUser.AccessToken, status: 401},
		{name: "management", token: management.AccessToken, status: 204}, {name: "revoked membership", token: management.AccessToken, denied: true, status: 403},
		{name: "disabled operation", token: management.AccessToken, disabled: true, status: 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			authorizer := accountTestAuthorizer(func(_ context.Context, _ services.ManagementAuthorizationSubject, target services.ManagementAuthorizationTarget, p services.ManagementPermission) error {
				if target.TenantID != "tenant" || target.ProjectID != "project" || p != services.PermissionProjectSchemaWrite {
					t.Fatalf("wrong target/permission: %#v %s", target, p)
				}
				if test.denied {
					return services.ErrManagementForbidden
				}
				return nil
			})
			app := fiber.New()
			app.Post("/", func(c *fiber.Ctx) error {
				return requireAccountManagement(c, "tenant", "project", services.PermissionProjectSchemaWrite, authorizer, !test.disabled)
			}, func(c *fiber.Ctx) error { return c.SendStatus(204) })
			request := httptest.NewRequest("POST", "/", nil)
			if test.token != "" {
				request.Header.Set("Authorization", "Bearer "+test.token)
			}
			response, err := app.Test(request)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != test.status {
				t.Fatalf("status=%d want %d", response.StatusCode, test.status)
			}
		})
	}
	if !isProtectedAccountContainer(&models.ContainerModel{IsAuthContainer: true}) || !isProtectedAccountContainer(&models.ContainerModel{SchemaName: "role"}) || isProtectedAccountContainer(&models.ContainerModel{SchemaName: "products"}) {
		t.Fatal("incorrect protected collection selection")
	}
}
