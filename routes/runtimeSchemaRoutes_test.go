package routes

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestRuntimeSchemaRouteIsSeparateFromManagementContainers(t *testing.T) {
	app := fiber.New()
	RuntimeSchemaRoutes(app)
	ContainerRoutes("/api/v1/:tenantSlug/:projectSlug/container", app)
	registered := registeredRouteSet(app)
	if !registered["GET /api/v1/:tenantSlug/:projectSlug/runtime/schemas"] {
		t.Fatal("runtime schema route is missing")
	}
	if registered["POST /api/v1/:tenantSlug/:projectSlug/runtime/schemas"] {
		t.Fatal("runtime metadata route must be read-only")
	}
	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/davinci/iyikafa/container", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("management containers unexpectedly accessible: %d", resp.StatusCode)
	}
}
