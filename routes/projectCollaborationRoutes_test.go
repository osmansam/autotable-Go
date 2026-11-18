package routes

import (
	"context"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/services"
	"github.com/osmansam/autotableGo/utils"
	"net/http/httptest"
	"testing"
)

type collaborationRoutePolicy struct{ tenantRoles, projectRoles []string }

func (p collaborationRoutePolicy) Authorize(_ context.Context, actor services.ManagementAuthorizationSubject, target services.ManagementAuthorizationTarget, permission services.ManagementPermission) error {
	return services.AuthorizeManagementSnapshot(actor, target, permission, models.ManagementMembershipSnapshot{UserActive: true, TenantActive: true, ProjectActive: true, TenantMembership: &models.TenantMembership{Status: models.MembershipStatusActive, Roles: p.tenantRoles}, ProjectMembership: &models.ProjectMembership{Status: models.MembershipStatusActive, Roles: p.projectRoles}})
}
func TestCollaborationMemberRouteMatrix(t *testing.T) {
	t.Setenv("TENANT_JWT_SECRET", "collaboration-contract-test-secret")
	t.Setenv("COLLABORATION_MEMBERS_ENABLED", "false")
	allowManagementSessions(t)
	t.Cleanup(middlewares.OverrideManagementProjectResolver(func(*fiber.Ctx) (string, string, error) { return "tenant", "project", nil }))
	previous := routeManagementAuthorizer
	t.Cleanup(func() { routeManagementAuthorizer = previous })
	for _, tc := range []struct {
		scope, role string
		allowed     bool
	}{
		{"tenant", models.TenantRoleOwner, true}, {"tenant", models.TenantRoleAdmin, true}, {"tenant", models.TenantRoleBilling, false}, {"tenant", models.TenantRoleAuditor, false}, {"tenant", models.TenantRoleMember, false},
		{"project", models.ProjectRoleAdmin, true}, {"project", models.ProjectRoleDeveloper, false}, {"project", models.ProjectRoleEditor, false}, {"project", models.ProjectRoleViewer, false}, {"project", models.ProjectRoleSupport, false},
	} {
		t.Run(tc.scope+"/"+tc.role, func(t *testing.T) {
			tenantRoles, projectRoles := []string{models.TenantRoleMember}, []string{tc.role}
			projectID := "project"
			base := "/api/v1/acme/shop/tenant-auth/members"
			if tc.scope == "tenant" {
				tenantRoles = []string{tc.role}
				projectRoles = nil
				projectID = ""
				base = "/api/v1/tenant/projects/project/members"
			}
			routeManagementAuthorizer = collaborationRoutePolicy{tenantRoles, projectRoles}
			tokens, err := utils.GenerateTenantTokens("user", "user@example.test", "tenant", projectID, []string{models.ProjectRoleAdmin}, tc.scope)
			if err != nil {
				t.Fatal(err)
			}
			app := fiber.New()
			ProjectCollaborationRoutes(app)
			for _, method := range []string{"GET", "PATCH", "DELETE"} {
				path := base
				if method != "GET" {
					path += "/target-user"
				}
				req := httptest.NewRequest(method, path, nil)
				req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
				response, err := app.Test(req)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				want := 403
				if tc.allowed {
					want = 404
				}
				if response.StatusCode != want {
					t.Fatalf("%s status=%d want=%d", method, response.StatusCode, want)
				}
			}
		})
	}
}
func TestCollaborationFeaturesFailClosed(t *testing.T) {
	t.Setenv("COLLABORATION_MEMBERS_ENABLED", "false")
	app := fiber.New()
	ProjectCollaborationRoutes(app)
	for _, path := range []string{"/api/v1/tenant/projects/project/members", "/api/v1/acme/shop/tenant-auth/members"} {
		response, err := app.Test(httptest.NewRequest("GET", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 401 {
			t.Fatalf("anonymous request status=%d", response.StatusCode)
		}
	}
}

func TestCollaborationRuntimeAndWrongProjectRejected(t *testing.T) {
	t.Setenv("TENANT_JWT_SECRET", "collaboration-contract-test-secret")
	t.Setenv("JWT_SECRET", "application-contract-test-secret")
	t.Setenv("COLLABORATION_MEMBERS_ENABLED", "false")
	allowManagementSessions(t)
	t.Cleanup(middlewares.OverrideManagementProjectResolver(func(*fiber.Ctx) (string, string, error) { return "tenant", "project", nil }))
	previous := routeManagementAuthorizer
	t.Cleanup(func() { routeManagementAuthorizer = previous })
	routeManagementAuthorizer = collaborationRoutePolicy{[]string{models.TenantRoleMember}, []string{models.ProjectRoleAdmin}}
	app := fiber.New()
	ProjectCollaborationRoutes(app)
	runtime, _ := utils.GenerateTokens("user", "admin", "tenant", "project", "acme", "shop")
	wrongProject, _ := utils.GenerateTenantTokens("user", "user@example.test", "tenant", "other-project", []string{models.ProjectRoleAdmin}, "project")
	for _, tc := range []struct {
		token  string
		status int
	}{{runtime.AccessToken, 401}, {wrongProject.RefreshToken, 401}, {wrongProject.AccessToken, 403}} {
		request := httptest.NewRequest("GET", "/api/v1/acme/shop/tenant-auth/members", nil)
		request.Header.Set("Authorization", "Bearer "+tc.token)
		response, err := app.Test(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != tc.status {
			t.Fatalf("credential boundary status=%d want=%d", response.StatusCode, tc.status)
		}
	}
}
