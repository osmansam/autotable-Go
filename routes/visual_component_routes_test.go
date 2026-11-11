package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/services"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVisualComponentManagementRoutes(t *testing.T) {
	app := fiber.New()
	VisualComponentRoutes(app)
	for _, item := range []struct {
		method, path string
		permission   services.ManagementPermission
	}{
		{"GET", "/api/v1/:tenantSlug/:projectSlug/admin/visual-components", services.PermissionProjectConfigRead},
		{"POST", "/api/v1/:tenantSlug/:projectSlug/admin/visual-components", services.PermissionProjectPageWrite},
		{"PUT", "/api/v1/:tenantSlug/:projectSlug/admin/visual-components/:id", services.PermissionProjectPageWrite},
	} {
		contract, ok := approvedManagementContract(item.method, item.path)
		if !ok || contract.Permission != item.permission {
			t.Fatalf("missing permission contract: %s", item.path)
		}
		path := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(item.path, ":tenantSlug", "t"), ":projectSlug", "p"), ":id", "012345678901234567890123")
		response, err := app.Test(httptest.NewRequest(item.method, path, nil))
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 401 {
			t.Fatalf("%s %s status=%d", item.method, path, response.StatusCode)
		}
	}
}
