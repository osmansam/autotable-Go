package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
)

func TestRuntimeSchemasRespectApplicationAccessAndHideManagementDefinitions(t *testing.T) {
	public := models.ContainerModel{SchemaName: "products", Fields: []models.Field{{Name: "name", Type: "string"}}}
	public.Routes.GetAllDynamicModelItems.IsActive = true
	private := models.ContainerModel{SchemaName: "orders", Fields: []models.Field{{Name: "total", Type: "number"}}}
	private.Routes.GetAllDynamicModelItems = models.RouteSpec{IsActive: true, IsAuthenticated: true, IsAuthorized: true, AuthorizeRole: []string{"member"}}
	public.Pipelines = []models.PipelineStage{{Name: "private-pipeline", PipelineJSON: "server-only-pipeline"}}
	public.DynamicApis = []models.DynamicApiModel{{Name: "private-api", Url: "https://secret-internal.example"}}
	public.Fields = append(public.Fields,
		models.Field{Name: "secretHash", IsHashed: true},
		models.Field{Name: "internal", IsAuthorized: true, AuthorizeRole: []string{"admin"}},
		models.Field{Name: "details", Type: "object", Children: []models.Field{{Name: "visible"}, {Name: "privateNested", IsAuthorized: true, AuthorizeRole: []string{"admin"}}}},
		models.Field{Name: "calculatedTotal", Type: "number", Equation: "server-only-equation"},
	)
	auth := public
	auth.SchemaName = "accounts"
	auth.IsAuthContainer = true
	auth.Routes = models.Routes{}
	roles := public
	roles.SchemaName = "role"
	roles.Routes = models.Routes{}
	all := []models.ContainerModel{public, private, auth, roles, {SchemaName: "disabled"}}
	metadata := buildRuntimeSchemas(all, "", nil)
	if len(metadata) != 1 || metadata[0].SchemaName != "products" {
		t.Fatalf("anonymous schemas = %#v", metadata)
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"server-only-pipeline", "private-pipeline", "private-api", "secret-internal", "secretHash", "internal", "privateNested", "authorizeRole", "rowAccess", "redis", "server-only-equation"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("runtime metadata leaks %q: %s", forbidden, encoded)
		}
	}
	if !strings.Contains(string(encoded), `"isComputed":true`) {
		t.Fatalf("computed field marker missing: %s", encoded)
	}
	if !strings.Contains(string(encoded), "visible") || !strings.Contains(string(encoded), "name") {
		t.Fatalf("display fields missing: %s", encoded)
	}
	if got := buildRuntimeSchemas(all, "user-1", []string{"other"}); len(got) != 1 {
		t.Fatalf("wrong-role schemas = %#v", got)
	}
	if got := buildRuntimeSchemas(all, "user-1", []string{"member"}); len(got) != 2 {
		t.Fatalf("member schemas = %#v", got)
	}
	if len(public.Fields[3].Children) != 2 {
		t.Fatal("projection mutated stored metadata")
	}
}

func TestRuntimeSchemaRecordAccessRequiresAuthentication(t *testing.T) {
	container := models.ContainerModel{SchemaName: "owned-orders", Fields: []models.Field{{Name: "owner"}}, Routes: models.Routes{
		GetAllDynamicModelItems: models.RouteSpec{IsActive: true, Access: &models.RecordAccessPolicy{}},
	}}
	if got := buildRuntimeSchemas([]models.ContainerModel{container}, "", nil); len(got) != 0 {
		t.Fatalf("anonymous record-access metadata = %#v", got)
	}
	if got := buildRuntimeSchemas([]models.ContainerModel{container}, "user-1", nil); len(got) != 1 {
		t.Fatalf("authenticated record-access metadata = %#v", got)
	}
}

func TestRuntimeSchemaHandlerUsesProjectIdentityAndReturnsNoStoreMetadata(t *testing.T) {
	previous := loadRuntimeSchemaContainers
	t.Cleanup(func() { loadRuntimeSchemaContainers = previous })
	calls := 0
	loadRuntimeSchemaContainers = func(_ context.Context, tenantID, projectID string) ([]models.ContainerModel, error) {
		calls++
		if tenantID != "tenant-1" || projectID != "project-1" {
			t.Errorf("unexpected scope %s/%s", tenantID, projectID)
		}
		return []models.ContainerModel{{SchemaName: "orders", Routes: models.Routes{GetAllDynamicModelItems: models.RouteSpec{IsActive: true, IsAuthenticated: true}}, Fields: []models.Field{{Name: "title"}}}}, nil
	}
	app := fiber.New()
	app.Get("/api/v1/:tenantSlug/:projectSlug/runtime/schemas", func(c *fiber.Ctx) error {
		c.Locals("expectedTenantID", "tenant-1")
		c.Locals("expectedProjectID", "project-1")
		if c.Get("X-Test-User") != "" {
			c.Locals("userID", "user-1")
			c.Locals("userRole", "member")
		}
		return c.Next()
	}, GetRuntimeSchemas)
	for _, authenticated := range []bool{false, true} {
		req := httptest.NewRequest("GET", "/api/v1/davinci/iyikafa/runtime/schemas", nil)
		if authenticated {
			req.Header.Set("X-Test-User", "yes")
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("response: %d %s", resp.StatusCode, body)
		}
		if strings.Contains(string(body), "orders") != authenticated {
			t.Fatalf("authenticated=%v metadata=%s", authenticated, body)
		}
	}
	if calls != 2 {
		t.Fatalf("loader calls = %d", calls)
	}
	for _, tc := range []struct{ authorization, cookie string }{
		{cookie: utils.ProjectAuthCookieName("davinci", "iyikafa", "access") + "=expired"},
		{cookie: utils.ProjectAuthCookieName("davinci", "iyikafa", "refresh") + "=refresh-only"},
		{authorization: "Bearer invalid"}, {authorization: "malformed"},
	} {
		req := httptest.NewRequest("GET", "/api/v1/davinci/iyikafa/runtime/schemas", nil)
		req.Header.Set("Authorization", tc.authorization)
		req.Header.Set("Cookie", tc.cookie)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("stale credential status = %d", resp.StatusCode)
		}
	}
	if calls != 2 {
		t.Fatal("stale credentials reached the metadata loader")
	}
	loadRuntimeSchemaContainers = func(context.Context, string, string) ([]models.ContainerModel, error) {
		return nil, errors.New("database-secret")
	}
	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/davinci/iyikafa/runtime/schemas", nil))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusInternalServerError || strings.Contains(string(body), "database-secret") {
		t.Fatalf("error response = %d %s", resp.StatusCode, body)
	}
}

func TestRuntimeAccountSchemasExposeOnlyConfiguredReadableMetadata(t *testing.T) {
	for _, schema := range []string{"auth", "role"} {
		for _, tc := range []struct {
			name, user   string
			roles        []string
			route        models.RouteSpec
			writableOnly bool
			want         int
		}{
			{name: "public selection", route: models.RouteSpec{IsActive: true}, want: 1},
			{name: "disabled"},
			{name: "private anonymous", route: models.RouteSpec{IsActive: true, IsAuthenticated: true}},
			{name: "allowed member", user: "user", roles: []string{"member"}, route: models.RouteSpec{IsActive: true, IsAuthorized: true, AuthorizeRole: []string{"member"}}, want: 1},
			{name: "wrong role", user: "user", roles: []string{"other"}, route: models.RouteSpec{IsActive: true, IsAuthorized: true, AuthorizeRole: []string{"member"}}},
			{name: "account writes do not grant runtime metadata", writableOnly: true, route: models.RouteSpec{IsActive: true}},
		} {
			t.Run(schema+"/"+tc.name, func(t *testing.T) {
				container := models.ContainerModel{SchemaName: schema, IsAuthContainer: schema == "auth", Fields: []models.Field{{Name: "name", Type: "string"}, {Name: "password", Type: "string", IsHashed: true}, {Name: "internal", IsAuthorized: true, AuthorizeRole: []string{"admin"}}}}
				if tc.writableOnly {
					container.Routes.CreateDynamicModelItem = tc.route
				} else {
					container.Routes.GetItemsForSelection = tc.route
				}
				got := buildRuntimeSchemas([]models.ContainerModel{container}, tc.user, tc.roles)
				if len(got) != tc.want {
					t.Fatalf("schemas=%d want=%d", len(got), tc.want)
				}
				if len(got) > 0 {
					body, err := json.Marshal(got)
					if err != nil {
						t.Fatal(err)
					}
					if strings.Contains(string(body), "password") || strings.Contains(string(body), "internal") || !strings.Contains(string(body), `"name":"name"`) {
						t.Fatalf("unsafe or incomplete metadata: %s", body)
					}
				}
			})
		}
	}
}
