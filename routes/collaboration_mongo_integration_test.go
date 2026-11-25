//go:build integration

package routes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/internal/testmongo"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/services"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
)

type captureCollaborationMail struct{ messages []models.InvitationMessage }

func (c *captureCollaborationMail) Send(_ context.Context, message models.InvitationMessage) error {
	c.messages = append(c.messages, message)
	return nil
}
func mailFragment(t *testing.T, message models.InvitationMessage) string {
	t.Helper()
	for _, line := range strings.Split(message.TextBody, "\n") {
		if strings.HasPrefix(line, "https://") {
			u, err := url.Parse(line)
			if err == nil {
				return u.Fragment
			}
		}
	}
	t.Fatal("captured email has no link")
	return ""
}
func TestCollaborationMongoHTTPJourneys(t *testing.T) {
	t.Setenv("TENANT_JWT_SECRET", strings.Repeat("local-management-key", 3))
	t.Setenv("JWT_SECRET", strings.Repeat("local-runtime-key", 3))
	t.Setenv("NODE_ENV", "development")
	for _, key := range []string{"COLLABORATION_MEMBERS_ENABLED", "COLLABORATION_INVITATIONS_ENABLED", "COLLABORATION_RECOVERY_ENABLED"} {
		t.Setenv(key, "true")
	}
	for k, v := range map[string]string{"INVITATION_SMTP_HOST": "smtp.example.test", "INVITATION_SMTP_FROM": "test@example.test", "INVITATION_PANEL_URL": "https://panel.example.test", "INVITATION_DELIVERY_KEYS_JSON": `{"test":"` + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))) + `"}`, "INVITATION_DELIVERY_ACTIVE_KEY_ID": "test"} {
		t.Setenv(k, v)
	}
	appcfg := configs.GetAppConfig()
	previousOrigins := appcfg.CorsWhitelist
	appcfg.CorsWhitelist = []string{"https://panel.example.test"}
	defer func() { appcfg.CorsWhitelist = previousOrigins }()
	f := testmongo.New(t)
	defer controllers.OverrideCollaborationDatabase(f.DB)()
	sessionRepo := repositories.NewAuthSessionRepositoryWithCollection(f.DB.Collection("auth_sessions"))
	defer middlewares.OverrideManagementSessionValidator(func(ctx context.Context, p models.ManagementPrincipal) error {
		return sessionRepo.ValidateManagementFamily(ctx, p, time.Now())
	})()
	previousAuth := routeManagementAuthorizer
	routeManagementAuthorizer = services.NewManagementAuthorizationService(repositories.NewManagementMembershipRepositoryWithDatabase(f.DB))
	defer func() { routeManagementAuthorizer = previousAuth }()
	tenant, otherTenant, project, otherProject, thirdProject, owner := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	for _, item := range []struct {
		collection string
		doc        interface{}
	}{
		{"users", models.User{ID: owner, Email: "owner@example.test", NormalizedEmail: "owner@example.test", IsActive: true}},
		{"tenants", models.Tenant{ID: tenant, Name: "Target", Slug: "target", IsActive: true}}, {"tenants", models.Tenant{ID: otherTenant, Name: "Other", Slug: "other", IsActive: true}},
		{"projects", models.Project{ID: project, TenantID: tenant, TenantSlug: "target", Slug: "inventory", Name: "Inventory", IsActive: true}},
		{"projects", models.Project{ID: otherProject, TenantID: otherTenant, TenantSlug: "other", Slug: "private", IsActive: true}},
		{"projects", models.Project{ID: thirdProject, TenantID: tenant, TenantSlug: "target", Slug: "unassigned", IsActive: true}},
		{"tenant_memberships", models.TenantMembership{ID: primitive.NewObjectID(), UserID: owner, TenantID: tenant, Roles: []string{models.TenantRoleOwner}, Status: models.MembershipStatusActive}},
		{"project_memberships", models.ProjectMembership{ID: primitive.NewObjectID(), UserID: owner, TenantID: tenant, ProjectID: project, Roles: []string{models.ProjectRoleAdmin}, Status: models.MembershipStatusActive}},
		{"management_collaboration_migrations", bson.M{"_id": "schema", "ready": true, "version": 1}},
	} {
		if _, err := f.DB.Collection(item.collection).InsertOne(f.Ctx, item.doc); err != nil {
			t.Fatal(err)
		}
	}
	if err := repositories.EnsureCollaborationIndexes(f.Ctx, f.DB); err != nil {
		t.Fatal(err)
	}
	defer middlewares.OverrideManagementProjectResolver(func(c *fiber.Ctx) (string, string, error) {
		if c.Params("tenantSlug") == "other" {
			return otherTenant.Hex(), otherProject.Hex(), nil
		}
		if c.Params("projectSlug") == "unassigned" {
			return tenant.Hex(), thirdProject.Hex(), nil
		}
		return tenant.Hex(), project.Hex(), nil
	})()
	app := fiber.New()
	TenantAuthRoutes(app)
	ProjectCollaborationRoutes(app)
	// Probe actual strict route contracts/live middleware without writing unrelated schemas/pages.
	for _, route := range []struct{ method, path string }{{"GET", "/api/v1/:tenantSlug/:projectSlug/admin/page/"}, {"POST", "/api/v1/:tenantSlug/:projectSlug/admin/page/"}, {"POST", "/api/v1/:tenantSlug/:projectSlug/container/"}} {
		handlers := strictManagementRouteHandlers(route.method, route.path, func(c *fiber.Ctx) error { return c.SendStatus(204) })
		app.Add(route.method, route.path, handlers...)
	}
	call := func(method, path string, body interface{}, cookies []*http.Cookie, key string) (*http.Response, []byte) {
		t.Helper()
		var reader io.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			reader = strings.NewReader(string(encoded))
		}
		req := httptest.NewRequest(method, path, reader)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://panel.example.test")
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		response, err := app.Test(req, 10000)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(response.Body)
		response.Body.Close()
		return response, data
	}
	start := func(user primitive.ObjectID, tenantID primitive.ObjectID, roles []string) []*http.Cookie {
		t.Helper()
		tokens, err := utils.GenerateTenantTokens(user.Hex(), "account@example.test", tenantID.Hex(), "", roles, "tenant")
		if err != nil {
			t.Fatal(err)
		}
		claims, _ := utils.ParseTenantRefreshToken(tokens.RefreshToken)
		err = services.NewAuthSessionService(sessionRepo).Start(f.Ctx, services.AuthSessionStart{Identity: "management", FamilyID: claims.FamilyID, RefreshTokenID: claims.ID, Scope: claims.Scope, UserID: claims.UserID, TenantID: claims.TenantID, ExpiresAt: time.Unix(tokens.RTExpires, 0)})
		if err != nil {
			t.Fatal(err)
		}
		return []*http.Cookie{{Name: utils.AuthCookieName("tenant", "access"), Value: tokens.AccessToken}}
	}
	ownerCookies := start(owner, tenant, []string{models.TenantRoleOwner})
	cfg, keys, err := services.ValidateInvitationConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	captured := &captureCollaborationMail{}
	delivery := services.NewInvitationDeliveryWorker(f.DB, cfg, keys, captured)
	for index, role := range []string{models.ProjectRoleDeveloper, models.ProjectRoleEditor, models.ProjectRoleViewer} {
		t.Run(role, func(t *testing.T) {
			email := role + "@example.test"
			existingUser := primitive.NewObjectID()
			password := "existing-account-password"
			if index < 2 {
				hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
				doc := bson.M{"_id": existingUser, "email": email, "normalizedEmail": email, "password": string(hash), "isActive": true}
				if index == 1 {
					doc["emailVerifiedAt"] = time.Now()
				}
				if _, err := f.DB.Collection("users").InsertOne(f.Ctx, doc); err != nil {
					t.Fatal(err)
				}
				_, err = f.DB.Collection("tenant_memberships").InsertOne(f.Ctx, models.TenantMembership{ID: primitive.NewObjectID(), UserID: existingUser, TenantID: otherTenant, Roles: []string{models.TenantRoleOwner}, Status: models.MembershipStatusActive})
				if err != nil {
					t.Fatal(err)
				}
			}
			response, data := call("POST", "/api/v1/tenant/projects/"+project.Hex()+"/invitations", bson.M{"email": email, "role": role}, ownerCookies, "http-create-"+role)
			if response.StatusCode != 202 {
				t.Fatalf("create %d %s", response.StatusCode, data)
			}
			var invitation struct {
				Data models.InvitationSummary `json:"data"`
			}
			if err := json.Unmarshal(data, &invitation); err != nil || invitation.Data.Version != 1 || invitation.Data.ID.IsZero() {
				t.Fatal("invitation UI contract mismatch")
			}
			if ok, err := delivery.ProcessNext(f.Ctx, "http-mail", time.Now()); err != nil || !ok {
				t.Fatalf("capture delivery: %v", err)
			}
			raw := mailFragment(t, captured.messages[len(captured.messages)-1])
			response, data = call("POST", invitationAuthBase+"/exchange", bson.M{"token": raw}, nil, "")
			if response.StatusCode != 200 {
				t.Fatalf("exchange %d %s", response.StatusCode, data)
			}
			cookies := response.Cookies()
			if strings.Contains(string(data), raw) {
				t.Fatal("secret in preview")
			}
			if index < 2 {
				response, data = call("POST", invitationAuthBase+"/reauthenticate", bson.M{"email": email, "password": password}, cookies, "")
				if response.StatusCode != 200 {
					t.Fatalf("reauth %d %s", response.StatusCode, data)
				}
				cookies = response.Cookies()
				if index == 0 {
					response, data = call("POST", invitationAuthBase+"/accept", bson.M{"mode": "existing"}, cookies, "")
					if response.StatusCode != 409 {
						t.Fatal("unverified account accepted")
					}
					response, data = call("POST", invitationAuthBase+"/verification", bson.M{}, cookies, "")
					if response.StatusCode != 202 {
						t.Fatalf("challenge %d %s", response.StatusCode, data)
					}
					if _, err := delivery.ProcessNext(f.Ctx, "http-verification", time.Now()); err != nil {
						t.Fatal(err)
					}
					fragment := mailFragment(t, captured.messages[len(captured.messages)-1])
					if !strings.HasPrefix(fragment, "verification=") {
						t.Fatal("challenge reused invite link")
					}
					response, data = call("POST", invitationAuthBase+"/verify", bson.M{"code": strings.TrimPrefix(fragment, "verification=")}, cookies, "")
					if response.StatusCode != 200 {
						t.Fatalf("verify %d %s", response.StatusCode, data)
					}
				}
			}
			body := bson.M{"mode": "existing"}
			if index == 2 {
				body = bson.M{"mode": "new", "name": "New collaborator", "password": "new-account-password"}
			}
			response, data = call("POST", invitationAuthBase+"/accept", body, cookies, "")
			if response.StatusCode != 200 {
				t.Fatalf("accept %d %s", response.StatusCode, data)
			}
			var accepted struct {
				Data models.AcceptedProjectInvitation `json:"data"`
			}
			if err := json.Unmarshal(data, &accepted); err != nil {
				t.Fatal(err)
			}
			if accepted.Data.Tenant.ID != tenant || accepted.Data.Project.ID != project || len(accepted.Data.Roles) != 1 || accepted.Data.Roles[0] != role || len(accepted.Data.TenantRoles) != 1 || accepted.Data.TenantRoles[0] != models.TenantRoleMember {
				t.Fatal("acceptance scope/role envelope mismatch")
			}
			joinedCookies := response.Cookies()
			for _, probe := range []struct {
				method, path string
				want         int
			}{{"GET", "/target/inventory/admin/page/", 204}, {"POST", "/target/inventory/admin/page/", map[bool]int{true: 403, false: 204}[index == 2]}, {"POST", "/target/inventory/container/", map[bool]int{true: 204, false: 403}[index == 0]}, {"GET", "/target/inventory/tenant-auth/members", 403}, {"GET", "/target/unassigned/admin/page/", 403}, {"GET", "/other/private/admin/page/", 403}} {
				r, _ := call(probe.method, "/api/v1"+probe.path, bson.M{}, joinedCookies, "")
				if r.StatusCode != probe.want {
					t.Fatalf("%s %s got %d want %d", probe.method, probe.path, r.StatusCode, probe.want)
				}
			}
			// The same ObjectID in an application user collection still has no management identity.
			runtime, _ := utils.GenerateTokens(accepted.Data.User.ID.Hex(), "admin", tenant.Hex(), project.Hex(), "target", "inventory")
			request := httptest.NewRequest("GET", "/api/v1/target/inventory/tenant-auth/members", nil)
			request.Header.Set("Authorization", "Bearer "+runtime.AccessToken)
			r, err := app.Test(request)
			if err != nil {
				t.Fatal(err)
			}
			r.Body.Close()
			if r.StatusCode != 401 {
				t.Fatal("runtime identity accessed management")
			}
			targetPath := "/api/v1/tenant/projects/" + project.Hex() + "/members/" + accepted.Data.User.ID.Hex()
			r, data = call("DELETE", targetPath, bson.M{"expectedVersion": 1}, ownerCookies, "")
			if r.StatusCode != 200 {
				t.Fatalf("remove %d %s", r.StatusCode, data)
			}
			r, _ = call("GET", "/api/v1/target/inventory/admin/page/", nil, joinedCookies, "")
			if r.StatusCode != 403 {
				t.Fatal("removed member retained live configuration access")
			}
			events := services.NewManagementCollaborationWorker(f.DB, nil)
			for i := 0; i < 20; i++ {
				found, err := events.ProcessNext(f.Ctx, "http-events", time.Now())
				if err != nil {
					t.Fatal(err)
				}
				if !found {
					break
				}
			}
			r, _ = call("GET", "/api/v1/target/inventory/admin/page/", nil, joinedCookies, "")
			if r.StatusCode != 401 {
				t.Fatal("durable revocation did not invalidate stale project session")
			}
			if index < 2 {
				var preserved models.TenantMembership
				if err := f.DB.Collection("tenant_memberships").FindOne(f.Ctx, bson.M{"userId": existingUser, "tenantId": otherTenant}).Decode(&preserved); err != nil || preserved.Roles[0] != models.TenantRoleOwner {
					t.Fatal("other tenant roles changed")
				}
			}
		})
	}
}
