package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
)

func TestRouteRegistration(t *testing.T) {
	app := fiber.New()
	AuthRoutes("/auth", app)
	ContainerRoutes("api/v1/:tenantSlug/:projectSlug/container", app)
	DynamicRoutes("/dynamic", app)
	SetupExcelRoutes(app, "api/v1")
	PageRoutes("api/v1/:tenantSlug/:projectSlug/page", app)
	ProjectRoutes(app)
	SchemaInfoRoutes("api/v1/:tenantSlug/:projectSlug", app)
	SwaggerRoutes(app, false)
	TenantAuthRoutes(app)
	oldAuditLogsConfigProvider := auditLogsConfigProvider
	auditLogsConfigProvider = func() (*models.AuditLogsConfig, error) {
		return &models.AuditLogsConfig{}, nil
	}
	t.Cleanup(func() { auditLogsConfigProvider = oldAuditLogsConfigProvider })
	AuditRoutes("api/v1/:tenantSlug/:projectSlug/audit-logs", app)

	registered := 0
	for _, route := range app.GetRoutes() {
		if route.Method != "HEAD" {
			registered++
		}
	}
	if registered < 40 {
		t.Fatalf("registered routes = %d, want at least 40", registered)
	}
}

func TestSwaggerRoutesDisabledInProduction(t *testing.T) {
	debugRoutes := []string{
		"GET /api/swagger",
		"GET /api/swagger.json",
		"GET /api/schemas",
	}

	productionApp := fiber.New()
	SwaggerRoutes(productionApp, true)
	productionRoutes := registeredRouteSet(productionApp)
	for _, route := range debugRoutes {
		if productionRoutes[route] {
			t.Errorf("production route %s is registered", route)
		}
	}

	developmentApp := fiber.New()
	SwaggerRoutes(developmentApp, false)
	developmentRoutes := registeredRouteSet(developmentApp)
	for _, route := range debugRoutes {
		if !developmentRoutes[route] {
			t.Errorf("development route %s is not registered", route)
		}
	}
}

func registeredRouteSet(app *fiber.App) map[string]bool {
	routes := make(map[string]bool)
	for _, route := range app.GetRoutes() {
		if route.Method == http.MethodHead {
			continue
		}
		routes[route.Method+" "+route.Path] = true
	}
	return routes
}

func TestDynamicArrayRoutesRegisteredInSafeOrder(t *testing.T) {
	app := fiber.New()
	DynamicRoutes("/dynamic", app)

	want := map[string]bool{
		"POST /dynamic/:schema/:id/array/:field":                false,
		"PATCH /dynamic/:schema/:id/array/:field/:rowIdentity":  false,
		"DELETE /dynamic/:schema/:id/array/:field/:rowIdentity": false,
		"PATCH /dynamic/:schema/:id/array/:field/reorder":       false,
	}
	reorderIndex := -1
	identityIndex := -1
	for index, route := range app.GetRoutes() {
		key := route.Method + " " + route.Path
		if _, exists := want[key]; exists {
			want[key] = true
		}
		if key == "PATCH /dynamic/:schema/:id/array/:field/reorder" {
			reorderIndex = index
		}
		if key == "PATCH /dynamic/:schema/:id/array/:field/:rowIdentity" {
			identityIndex = index
		}
	}
	for route, found := range want {
		if !found {
			t.Errorf("route %s is not registered", route)
		}
	}
	if reorderIndex == -1 || identityIndex == -1 || reorderIndex > identityIndex {
		t.Fatalf("reorder route index = %d, identity route index = %d; reorder must be first", reorderIndex, identityIndex)
	}
}

func TestMetadataRoutesRequireTenantAuthentication(t *testing.T) {
	tests := []struct {
		name     string
		register func(*fiber.App)
		path     string
	}{
		{name: "containers", register: func(app *fiber.App) { ContainerRoutes("/api/v1/:tenantSlug/:projectSlug/container", app) }, path: "/api/v1/t/p/container"},
		{name: "container tenant list", register: func(app *fiber.App) { ContainerRoutes("/api/v1/:tenantSlug/:projectSlug/container", app) }, path: "/api/v1/t/p/container/tenant"},
		{name: "container types", register: func(app *fiber.App) { ContainerRoutes("/api/v1/:tenantSlug/:projectSlug/container", app) }, path: "/api/v1/t/p/container/types"},
		{name: "admin pages list", register: func(app *fiber.App) { PageRoutes("/api/v1/:tenantSlug/:projectSlug/page", app) }, path: "/api/v1/t/p/admin/page/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			tt.register(app)

			resp, err := app.Test(httptest.NewRequest(http.MethodGet, tt.path, nil))
			if err != nil {
				t.Fatalf("app.Test() error = %v", err)
			}
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("%s status = %d, want %d", tt.path, resp.StatusCode, http.StatusUnauthorized)
			}
		})
	}
}

func TestPagePublicRouteRemoved(t *testing.T) {
	app := fiber.New()
	PageRoutes("/api/v1/:tenantSlug/:projectSlug/page", app)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/t/p/page/public", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("/page/public status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestProjectTemplateRouteRegisteredBeforeProjectIDRoute(t *testing.T) {
	app := fiber.New()
	ProjectRoutes(app)

	templateIndex := -1
	projectIDIndex := -1
	for index, route := range app.GetRoutes() {
		if route.Method != "GET" {
			continue
		}
		switch route.Path {
		case "/api/v1/tenant/projects/templates":
			templateIndex = index
		case "/api/v1/tenant/projects/:id":
			projectIDIndex = index
		}
	}

	if templateIndex == -1 {
		t.Fatal("GET /api/v1/tenant/projects/templates route is not registered")
	}
	if projectIDIndex == -1 {
		t.Fatal("GET /api/v1/tenant/projects/:id route is not registered")
	}
	if templateIndex > projectIDIndex {
		t.Fatalf("templates route index = %d, want before project id route index = %d", templateIndex, projectIDIndex)
	}
}

func testProjectJWT(t *testing.T, role string) string {
	t.Helper()
	t.Setenv("JWT_SECRET", "test-secret")
	claims := jwt.MapClaims{
		"authorized":   true,
		"user_id":      "user-id",
		"role":         role,
		"tenant_id":    "tenant-id",
		"project_id":   "project-id",
		"tenant_slug":  "tenant",
		"project_slug": "project",
		"token_type":   "access",
		"scope":        "project",
		"aud":          "autoapi-project",
		"jti":          "test-jti",
		"family_id":    "test-family",
		"exp":          time.Now().Add(time.Hour).Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	return token
}
