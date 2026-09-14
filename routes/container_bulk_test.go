package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// Moving the development route behind TenantAuthenticate must break this test.
func TestBulkContainerRouteIsPublicButSingleCreateStillRequiresAuth(t *testing.T) {
	app := fiber.New()
	ContainerRoutes("/api/v1/:tenantSlug/:projectSlug/container", app)
	for _, tt := range []struct {
		path string
		want int
	}{
		{"/api/v1/tenant/project/container/create-multiple", http.StatusBadRequest},
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
