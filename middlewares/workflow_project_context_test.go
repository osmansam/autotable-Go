package middlewares

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/services"
	"github.com/osmansam/autotableGo/utils"
)

func TestWorkflowCacheReusesAuthenticatedProjectResolution(t *testing.T) {
	t.Setenv("JWT_SECRET", "workflow-context-test-secret")
	t.Setenv("TENANT_JWT_SECRET", "workflow-context-management-secret")
	server := miniredis.RunT(t)
	previous := configs.RedisClient
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	configs.RedisClient = client
	t.Cleanup(func() { _ = client.Close(); configs.RedisClient = previous })
	server.Set("slug_mapping:acme:store", "tenant|project")
	checks := 0
	restore := utils.OverrideProjectActiveChecker(func(context.Context, string, string) (bool, error) { checks++; return true, nil })
	t.Cleanup(restore)
	member, err := utils.GenerateTokens("user", "member", "tenant", "project", "acme", "store")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := utils.GenerateTokens("user", "member", "tenant", "other-project", "acme", "other")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token   string
		authenticated bool
		status        int
	}{
		{name: "public cache", status: http.StatusOK},
		{name: "authentication still required", authenticated: true, status: http.StatusUnauthorized},
		{name: "valid project token", authenticated: true, token: member.AccessToken, status: http.StatusOK},
		{name: "foreign project denied", authenticated: true, token: foreign.AccessToken, status: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflow := models.DynamicWorkflow{Name: "loadPublicPage", IsActive: true, IsRedisCached: true, CacheTime: 1, IsAuthenticated: tc.authenticated,
				Steps: []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeReturn, IsActive: true, Config: map[string]interface{}{"value": map[string]interface{}{"page": "{{record.page}}"}}}},
			}
			container := &models.ContainerModel{SchemaName: "siteConfig", Workflows: []models.DynamicWorkflow{workflow}}
			app := fiber.New()
			app.Post("/:tenantSlug/:projectSlug/workflow/:workflowName", func(c *fiber.Ctx) error {
				tenant, project, err := utils.GetTenantAndProjectContext(c)
				if err != nil {
					return err
				}
				// Supply fixture metadata at the boundary where production fetches it.
				return conditionalAuthenticationForContainer(c, "ExecuteWorkflow", tenant, project, container)
			}, func(c *fiber.Ctx) error {
				tenant, project, err := utils.GetTenantAndProjectContext(c)
				if err != nil {
					return err
				}
				userID, _ := c.Locals("userID").(string)
				if tc.authenticated {
					if userID != "user" {
						t.Errorf("authenticated user = %q", userID)
					}
					// User-data loading is covered by service tests with fixture users.
					return c.JSON(fiber.Map{"source": "authenticated", "data": fiber.Map{"page": "home"}})
				}
				result, err := services.NewDynamicService().ExecuteWorkflow(c.UserContext(), services.ExecuteWorkflowInput{
					AccessIdentityInput: services.AccessIdentityInput{IdentityKind: services.AccessIdentityCaller, UserID: userID},
					TenantID:            tenant, ProjectID: project, Schema: "siteConfig", WorkflowName: "loadPublicPage", Container: container,
					Record: map[string]interface{}{"page": "home"},
				})
				if err != nil {
					return err
				}
				return c.JSON(fiber.Map{"source": result.Source, "data": result.Data})
			})
			startChecks := checks
			for attempt := 0; attempt < 2; attempt++ {
				request := httptest.NewRequest(http.MethodPost, "/acme/store/workflow/loadPublicPage?schemaName=siteConfig", strings.NewReader(`{"record":{"page":"home"}}`))
				request.Header.Set("Content-Type", "application/json")
				if tc.token != "" {
					request.Header.Set("Authorization", "Bearer "+tc.token)
				}
				response, err := app.Test(request)
				if err != nil {
					t.Fatal(err)
				}
				var result struct {
					Source string                 `json:"source"`
					Data   map[string]interface{} `json:"data"`
				}
				decodeErr := json.NewDecoder(response.Body).Decode(&result)
				response.Body.Close()
				if response.StatusCode != tc.status {
					t.Fatalf("status = %d, want %d", response.StatusCode, tc.status)
				}
				if tc.status == http.StatusOK {
					if decodeErr != nil {
						t.Fatal(decodeErr)
					}
					want := "workflow"
					if tc.authenticated {
						want = "authenticated"
					} else if attempt == 1 {
						want = "cache"
					}
					if result.Source != want || result.Data["page"] != "home" {
						t.Errorf("response = %+v, want %s and original data", result, want)
					}
				}
			}
			if got := checks - startChecks; got != 2 {
				t.Errorf("active checks = %d, want one for each HTTP request", got)
			}
		})
	}
}
