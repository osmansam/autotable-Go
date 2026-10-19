package routes

import (
	"github.com/gofiber/fiber/v2"
	"net/http/httptest"
	"testing"
)

func TestManagedAccountCreationRejectsAnonymous(t *testing.T) {
	app := fiber.New()
	AuthRoutes("api/v1/:tenantSlug/:projectSlug/auth", app)
	response, err := app.Test(httptest.NewRequest("POST", "/api/v1/tenant/project/admin/auth/users", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 401 {
		t.Fatalf("status=%d", response.StatusCode)
	}
}

func TestPipelineTestingRequiresManagementAuthentication(t *testing.T) {
	app := fiber.New()
	DynamicRoutes("api/v1/:tenantSlug/:projectSlug/dynamic", app)
	response, err := app.Test(httptest.NewRequest("GET", "/api/v1/tenant/project/dynamic/testPipeline?schemaName=records", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 401 {
		t.Fatalf("pipeline testing status=%d want 401", response.StatusCode)
	}
}
