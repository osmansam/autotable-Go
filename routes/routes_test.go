package routes

import (
	"testing"

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
