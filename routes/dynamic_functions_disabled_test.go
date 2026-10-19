package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestDynamicFunctionEndpointsAreDisabled(t *testing.T) {
	app := fiber.New()
	DynamicRoutes("/dynamic", app)
	ContainerRoutes("/api/v1/:tenantSlug/:projectSlug/container", app)
	registered := registeredRouteSet(app)
	for _, endpoint := range []string{
		"GET /dynamic/execute",
		"PATCH /api/v1/:tenantSlug/:projectSlug/container/dynamicFunctions/:id",
	} {
		if registered[endpoint] {
			t.Errorf("disabled dynamic-function endpoint remains registered: %s", endpoint)
		}
	}
	for _, endpoint := range []string{
		"POST /dynamic/workflow/:workflowName",
		"GET /dynamic/pipeline",
		"GET /dynamic/api",
		"PATCH /api/v1/:tenantSlug/:projectSlug/container/workflows/:id",
		"PATCH /api/v1/:tenantSlug/:projectSlug/container/pipelines/:id",
		"PATCH /api/v1/:tenantSlug/:projectSlug/container/dynamicApis/:id",
	} {
		if !registered[endpoint] {
			t.Errorf("unrelated endpoint was disabled: %s", endpoint)
		}
	}
}

func TestDynamicFunctionUpdateEndpointIsUnavailable(t *testing.T) {
	app := fiber.New()
	ContainerRoutes("/api/v1/:tenantSlug/:projectSlug/container", app)
	response, err := app.Test(httptest.NewRequest(http.MethodPatch, "/api/v1/tenant/project/container/dynamicFunctions/507f1f77bcf86cd799439011", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("disabled function update status = %d, want 404", response.StatusCode)
	}
}
