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
	ContainerRoutes("/containers", app)
	DynamicRoutes("/dynamic", app)
	SetupExcelRoutes(app, "/api")
	PageRoutes("/pages", app)
	ProjectRoutes(app)
	SchemaInfoRoutes("/schema", app)
	SwaggerRoutes(app)
	TenantAuthRoutes(app)
	oldAuditLogsConfigProvider := auditLogsConfigProvider
	auditLogsConfigProvider = func() (*models.AuditLogsConfig, error) {
		return &models.AuditLogsConfig{}, nil
	}
	t.Cleanup(func() { auditLogsConfigProvider = oldAuditLogsConfigProvider })
	AuditRoutes("/audit", app)

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

func TestMetadataRoutesRequireTenantAuthentication(t *testing.T) {
	tests := []struct {
		name     string
		register func(*fiber.App)
		path     string
	}{
		{name: "containers", register: func(app *fiber.App) { ContainerRoutes("/containers", app) }, path: "/containers"},
		{name: "container tenant list", register: func(app *fiber.App) { ContainerRoutes("/containers", app) }, path: "/containers/tenant"},
		{name: "container types", register: func(app *fiber.App) { ContainerRoutes("/containers", app) }, path: "/containers/types"},
		{name: "runtime pages list", register: func(app *fiber.App) { PageRoutes("/pages", app) }, path: "/pages"},
		{name: "admin pages list", register: func(app *fiber.App) { PageRoutes("/pages", app) }, path: "/admin/pages"},
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
	PageRoutes("/pages", app)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/pages/public", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("/pages/public status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestAuditRoutesReadAuthorizationConfigPerRequest(t *testing.T) {
	app := fiber.New()
	auditConfig := &models.AuditLogsConfig{IsAuthorized: false, AuthorizeRole: []string{}}
	oldAuditLogsConfigProvider := auditLogsConfigProvider
	auditLogsConfigProvider = func() (*models.AuditLogsConfig, error) {
		return auditConfig, nil
	}
	t.Cleanup(func() { auditLogsConfigProvider = oldAuditLogsConfigProvider })
	AuditRoutes("/audit", app)

	token := testProjectJWT(t, models.ProjectRoleViewer)
	req := httptest.NewRequest(http.MethodGet, "/audit/missing", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode == http.StatusForbidden {
		t.Fatalf("first /audit/missing status = %d, want config without role authorization to pass middleware", resp.StatusCode)
	}

	auditConfig = &models.AuditLogsConfig{IsAuthorized: true, AuthorizeRole: []string{models.ProjectRoleAdmin}}
	req = httptest.NewRequest(http.MethodGet, "/audit/missing", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("second /audit/missing status = %d, want %d after config changed", resp.StatusCode, http.StatusForbidden)
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
	claims := jwt.MapClaims{
		"authorized":   true,
		"user_id":      "user-id",
		"role":         role,
		"tenant_id":    "tenant-id",
		"project_id":   "project-id",
		"tenant_slug":  "tenant",
		"project_slug": "project",
		"exp":          time.Now().Add(time.Hour).Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte{})
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	return token
}
