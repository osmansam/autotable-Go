package middlewares

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
)

// Regression: a configured account read must reach normal route authorization,
// instead of unconditionally requiring a management credential.
func TestAccountRuntimeReadsHonorConfiguredPermissions(t *testing.T) {
	t.Setenv("JWT_SECRET", "account-read-test-secret")
	t.Setenv("TENANT_JWT_SECRET", "account-management-test-secret")
	t.Setenv("NODE_ENV", "development")
	member, err := utils.GenerateTokens("reader", "member", "tenant", "project", "acme", "test")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := utils.GenerateTokens("reader", "member", "other", "other", "acme", "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, schema := range []string{"auth", "role"} {
		for _, operation := range []string{"GetTableSource", "GetItemsForSelection", "GetAllDynamicModelItems", "GetDynamicModelItem", "GetAllDynamicModelItemsWithPagination"} {
			for _, tc := range []struct {
				name             string
				route            models.RouteSpec
				token            string
				cookie           bool
				managementCookie bool
				status           int
			}{
				{name: "public", route: models.RouteSpec{IsActive: true}, status: 204},
				{name: "disabled", status: 401},
				{name: "disabled with session", token: member.AccessToken, status: 403},
				{name: "authentication required", route: models.RouteSpec{IsActive: true, IsAuthenticated: true}, status: 401},
				{name: "application bearer", route: models.RouteSpec{IsActive: true, IsAuthenticated: true}, token: member.AccessToken, status: 204},
				{name: "application cookie", route: models.RouteSpec{IsActive: true, IsAuthorized: true, AuthorizeRole: []string{"member"}}, token: member.AccessToken, cookie: true, status: 204},
				{name: "wrong role", route: models.RouteSpec{IsActive: true, IsAuthorized: true, AuthorizeRole: []string{"admin"}}, token: member.AccessToken, status: 403},
				{name: "foreign project", route: models.RouteSpec{IsActive: true, IsAuthenticated: true}, token: foreign.AccessToken, status: 403},
				{name: "record access needs session", route: models.RouteSpec{IsActive: true, Access: &models.RecordAccessPolicy{}}, status: 401},
				{name: "public read retains cookie identity", route: models.RouteSpec{IsActive: true}, token: member.AccessToken, cookie: true, status: 204},
				{name: "invalid management cookie does not downgrade", route: models.RouteSpec{IsActive: true}, managementCookie: true, status: 401},
			} {
				t.Run(schema+"/"+operation+"/"+tc.name, func(t *testing.T) {
					container := &models.ContainerModel{SchemaName: schema, IsAuthContainer: schema == "auth"}
					container.Routes.GetAllDynamicModelItems = tc.route
					container.Routes.GetDynamicModelItem = tc.route
					container.Routes.GetAllDynamicModelItemsWithPagination = tc.route
					container.Routes.GetItemsForSelection = tc.route
					app := fiber.New()
					app.Get("/api/v1/:tenantSlug/:projectSlug/dynamic/table-source", func(c *fiber.Ctx) error {
						return conditionalAuthenticationForContainer(c, operation, "tenant", "project", container)
					}, func(c *fiber.Ctx) error {
						if tc.token == member.AccessToken && c.Locals("userID") != "reader" {
							t.Error("application cookie identity lost")
						}
						return c.SendStatus(204)
					})
					req := httptest.NewRequest("GET", "/api/v1/acme/test/dynamic/table-source?schemaName="+schema+"&sourceType=schema", nil)
					if tc.token != "" {
						if tc.cookie {
							req.Header.Set("Cookie", utils.ProjectAuthCookieName("acme", "test", "access")+"="+tc.token)
						} else {
							req.Header.Set("Authorization", "Bearer "+tc.token)
						}
					}
					if tc.managementCookie {
						req.Header.Set("Cookie", utils.ManagementProjectAuthCookieName("acme", "test", "access")+"=invalid")
					}
					resp, err := app.Test(req)
					if err != nil {
						t.Fatal(err)
					}
					resp.Body.Close()
					if resp.StatusCode != tc.status {
						t.Fatalf("status=%d want=%d", resp.StatusCode, tc.status)
					}
				})
			}
		}
	}
}

func TestAccountExecutionStillRequiresManagement(t *testing.T) {
	t.Setenv("JWT_SECRET", "account-read-test-secret")
	for _, operation := range []string{"CreateDynamicModelItem", "UpdateDynamicModelItem", "DeleteDynamicModelItem", "GetPipeline", "ExecuteWorkflow", "ExecuteDynamicAPI", "GetTableSource"} {
		app := fiber.New()
		app.Get("/", func(c *fiber.Ctx) error {
			return conditionalAuthenticationForContainer(c, operation, "tenant", "project", &models.ContainerModel{SchemaName: "auth", IsAuthContainer: true})
		}, func(c *fiber.Ctx) error { return c.SendStatus(204) })
		resp, err := app.Test(httptest.NewRequest("GET", "/?schemaName=auth&sourceType=workflow", nil))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Errorf("%s status=%d want401", operation, resp.StatusCode)
		}
	}
}

func TestPublicWritesAndExecutableSourcesDoNotGainAmbientCookieIdentity(t *testing.T) {
	t.Setenv("JWT_SECRET", "account-read-test-secret")
	t.Setenv("NODE_ENV", "development")
	tokens, err := utils.GenerateTokens("reader", "member", "tenant", "project", "acme", "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ method, operation, query string }{
		{"POST", "CreateDynamicModelItem", ""},
		{"GET", "GetTableSource", "?sourceType=workflow&workflowName=public"},
		{"GET", "GetTableSource", "?sourceType=pipeline&pipelineName=public"},
	} {
		t.Run(tc.operation+tc.query, func(t *testing.T) {
			container := &models.ContainerModel{SchemaName: "products", Routes: models.Routes{CreateDynamicModelItem: models.RouteSpec{IsActive: true}}, Workflows: []models.DynamicWorkflow{{Name: "public", IsActive: true}}, Pipelines: []models.PipelineStage{{Name: "public", IsActive: true}}}
			app := fiber.New()
			app.Add(tc.method, "/api/v1/:tenantSlug/:projectSlug/dynamic", func(c *fiber.Ctx) error {
				return conditionalAuthenticationForContainer(c, tc.operation, "tenant", "project", container)
			}, func(c *fiber.Ctx) error {
				if c.Locals("userID") != nil || c.Locals("userRole") != nil {
					t.Error("public write or executable source gained ambient cookie identity")
				}
				return c.SendStatus(204)
			})
			req := httptest.NewRequest(tc.method, "/api/v1/acme/test/dynamic"+tc.query, nil)
			req.Header.Set("Cookie", utils.ProjectAuthCookieName("acme", "test", "access")+"="+tokens.AccessToken)
			req.Header.Set("Origin", "https://untrusted.example")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != 204 {
				t.Fatalf("status=%d want204 anonymous public execution", resp.StatusCode)
			}
		})
	}
}
