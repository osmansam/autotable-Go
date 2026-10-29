package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"net/http/httptest"
	"testing"
)

func TestTenantExternalCredentialRoutesRejectAnonymousAndProjectScopedTokens(t *testing.T) {
	t.Setenv("TENANT_JWT_SECRET", "test-tenant-secret")
	tokens, err := utils.GenerateTenantTokens("user", "manager@example.com", "507f1f77bcf86cd799439011", "507f1f77bcf86cd799439012", []string{models.ProjectRoleAdmin}, string(models.RoleScopeProject))
	if err != nil {
		t.Fatal(err)
	}
	old := routeManagementAuthorizer
	routeManagementAuthorizer = allowingRouteAuthorizer{}
	t.Cleanup(func() { routeManagementAuthorizer = old })
	app := fiber.New()
	ProjectRoutes(app)
	base := "/api/v1/tenant/integrations/external-api-credentials"
	for _, route := range []struct{ method, path string }{{"GET", base}, {"POST", base}, {"POST", base + "/507f1f77bcf86cd799439013/promote"}, {"POST", base + "/507f1f77bcf86cd799439013/revoke"}, {"PUT", base + "/507f1f77bcf86cd799439013/projects/507f1f77bcf86cd799439014"}, {"DELETE", base + "/507f1f77bcf86cd799439013/projects/507f1f77bcf86cd799439014"}, {"GET", "/api/v1/tenant/projects/templates/507f1f77bcf86cd799439015/external-api-requirements"}} {
		for _, authenticated := range []bool{false, true} {
			req := httptest.NewRequest(route.method, route.path, nil)
			if authenticated {
				req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
			}
			response, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			want := 401
			if authenticated {
				want = 403
			}
			if response.StatusCode != want {
				t.Fatalf("%s %s authenticated=%t status=%d want=%d", route.method, route.path, authenticated, response.StatusCode, want)
			}
		}
	}
}
