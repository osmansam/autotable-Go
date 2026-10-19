package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/services"
)

// The bulk importer must only be reachable through the strict management contract
// (M identity + project scope + project.schema.write).
func TestBulkContainerRouteIsNotPublic(t *testing.T) {
	app := fiber.New()
	ContainerRoutes("/api/v1/:tenantSlug/:projectSlug/container", app)
	bulkRoute := "POST /api/v1/:tenantSlug/:projectSlug/container/create-multiple"
	if !registeredRouteSet(app)[bulkRoute] {
		t.Fatalf("bulk importer route %s is not registered", bulkRoute)
	}
	c, ok := approvedManagementContract("POST", "/api/v1/:tenantSlug/:projectSlug/container/create-multiple")
	if !ok || c.Identity != IdentityManagement || c.Scope != ManagementScopeProject || c.Permission != services.PermissionProjectSchemaWrite {
		t.Fatalf("bulk importer contract = %+v, want management/project/project.schema.write", c)
	}

	for _, tt := range []struct {
		path string
		want int
	}{
		{"/api/v1/tenant/project/container/create-multiple", http.StatusUnauthorized},
		{"/api/v1/tenant/project/container/", http.StatusUnauthorized},
	} {
		req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(`[]`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tt.want {
			t.Errorf("%s: got %d, want %d", tt.path, resp.StatusCode, tt.want)
		}
	}
}
