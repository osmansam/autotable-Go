//go:build integration

package controllers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/internal/testmongo"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestCollaborationHTTPAcceptanceJourney(t *testing.T) {
	t.Setenv("TENANT_JWT_SECRET", strings.Repeat("local-test-key-", 4))
	t.Setenv("NODE_ENV", "development")
	f := testmongo.New(t)
	defer OverrideCollaborationDatabase(f.DB)()
	actor, tenant, project := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	docs := map[string]interface{}{
		"users":               models.User{ID: actor, Email: "admin@example.test", NormalizedEmail: "admin@example.test", IsActive: true},
		"tenants":             models.Tenant{ID: tenant, Name: "Acme", Slug: "acme", IsActive: true},
		"projects":            models.Project{ID: project, TenantID: tenant, Name: "Inventory", Slug: "inventory", TenantSlug: "acme", IsActive: true},
		"tenant_memberships":  models.TenantMembership{ID: primitive.NewObjectID(), TenantID: tenant, UserID: actor, Roles: []string{models.TenantRoleMember}, Status: models.MembershipStatusActive},
		"project_memberships": models.ProjectMembership{ID: primitive.NewObjectID(), TenantID: tenant, ProjectID: project, UserID: actor, Roles: []string{models.ProjectRoleAdmin}, Status: models.MembershipStatusActive},
	}
	for name, doc := range docs {
		if _, err := f.DB.Collection(name).InsertOne(f.Ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	if err := repositories.EnsureCollaborationIndexes(f.Ctx, f.DB); err != nil {
		t.Fatal(err)
	}
	raw, hash, err := utils.GenerateInvitationToken()
	if err != nil {
		t.Fatal(err)
	}
	invite := models.ManagementInvitation{ID: primitive.NewObjectID(), TenantID: tenant, ProjectID: project, InvitedBy: actor, Authority: models.CollaborationAuthorityProject, NormalizedEmail: "new@example.test", Role: models.ProjectRoleViewer, TokenHash: hash, TokenVersion: 1, Status: models.InvitationPending, ExpiresAt: time.Now().Add(time.Hour)}
	if _, err := f.DB.Collection("management_invitations").InsertOne(f.Ctx, invite); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Post("/exchange", ExchangeProjectInvitation)
	app.Get("/continuation", ResumeProjectInvitation)
	app.Post("/accept", AcceptProjectInvitation)
	call := func(method, path, body string, cookies []*http.Cookie) (*http.Response, string) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for _, c := range cookies {
			req.AddCookie(c)
		}
		response, err := app.Test(req, 10000)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(response.Body)
		response.Body.Close()
		return response, string(data)
	}
	response, body := call("POST", "/exchange", `{"token":"`+raw+`"}`, nil)
	if response.StatusCode != 200 {
		t.Fatalf("exchange %d %s", response.StatusCode, body)
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || strings.Contains(body, raw) || strings.Contains(body, cookies[0].Value) {
		t.Fatal("secret crossed JSON boundary or cookie missing")
	}
	response, _ = call("GET", "/continuation", "", nil)
	if response.StatusCode != 410 {
		t.Fatal("another browser resumed continuation")
	}
	response, body = call("GET", "/continuation", "", cookies)
	if response.StatusCode != 200 || !strings.Contains(body, "Inventory") {
		t.Fatal("reload failed")
	}
	ownerTokens, err := utils.GenerateTenantTokens(actor.Hex(), "admin@example.test", tenant.Hex(), "", []string{models.TenantRoleMember}, "tenant")
	if err != nil {
		t.Fatal(err)
	}
	if err := startManagementSessionInDatabase(f.Ctx, f.DB, ownerTokens); err != nil {
		t.Fatal(err)
	}
	ambient := append(append([]*http.Cookie{}, cookies...), &http.Cookie{Name: utils.AuthCookieName("tenant", "refresh"), Value: ownerTokens.RefreshToken}, &http.Cookie{Name: utils.AuthCookieName("tenant", "access"), Value: "expired-access"})
	response, body = call("GET", "/continuation", "", ambient)
	if response.StatusCode != 200 || !strings.Contains(body, "switch_account") {
		t.Fatal("refreshable account was treated as anonymous")
	}
	response, body = call("POST", "/accept", `{"mode":"new","name":"New collaborator","password":"local-account-password"}`, ambient)
	if response.StatusCode != 409 {
		t.Fatal("refreshable account was silently replaced")
	}
	response, body = call("POST", "/accept", `{"mode":"new","name":"New collaborator","password":"local-account-password"}`, cookies)
	if response.StatusCode != 200 {
		t.Fatalf("accept %d %s", response.StatusCode, body)
	}
	var result struct {
		Data models.AcceptedProjectInvitation `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data.TenantRoles) != 1 || result.Data.TenantRoles[0] != models.TenantRoleMember || len(result.Data.Roles) != 1 || result.Data.Roles[0] != models.ProjectRoleViewer {
		t.Fatal("wrong grant scope")
	}
	if strings.Contains(body, "local-account-password") || strings.Contains(body, "tokenHash") {
		t.Fatal("credentials in response")
	}
	sessions, _ := f.DB.Collection("auth_sessions").CountDocuments(f.Ctx, bson.M{"userId": result.Data.User.ID.Hex(), "identity": "management"})
	if sessions != 2 {
		t.Fatalf("target tenant/project sessions=%d", sessions)
	}
	cookiesSet := 0
	for _, c := range response.Cookies() {
		if c.Value != "" {
			cookiesSet++
		}
	}
	if cookiesSet < 4 {
		t.Fatal("target session cookies missing")
	}
	response, _ = call("POST", "/accept", `{"mode":"new","name":"Replay","password":"local-account-password"}`, cookies)
	if response.StatusCode != 410 {
		t.Fatal("acceptance replay issued session")
	}
	after, _ := f.DB.Collection("auth_sessions").CountDocuments(f.Ctx, bson.M{"userId": result.Data.User.ID.Hex()})
	if after != 2 {
		t.Fatal("replay created sessions")
	}
}

func TestCollaborationProjectReactivationHTTP(t *testing.T) {
	f := testmongo.New(t)
	defer OverrideCollaborationDatabase(f.DB)()
	user, tenant, project := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	for collection, doc := range map[string]interface{}{"users": models.User{ID: user, IsActive: true}, "tenants": models.Tenant{ID: tenant, IsActive: true}, "projects": models.Project{ID: project, TenantID: tenant, IsActive: false}, "tenant_memberships": models.TenantMembership{ID: primitive.NewObjectID(), TenantID: tenant, UserID: user, Roles: []string{models.TenantRoleOwner}, Status: models.MembershipStatusActive}} {
		if _, err := f.DB.Collection(collection).InsertOne(f.Ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	app := fiber.New()
	app.Patch("/:id", func(c *fiber.Ctx) error {
		c.Locals("tenantID", tenant.Hex())
		c.Locals("tenantUserID", user.Hex())
		return UpdateProject(c)
	})
	req := httptest.NewRequest("PATCH", "/"+project.Hex(), strings.NewReader(`{"isActive":true}`))
	req.Header.Set("Content-Type", "application/json")
	response, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != 409 || !strings.Contains(string(data), "collaboration_recovery_required") {
		t.Fatalf("adminless HTTP reactivation allowed: %d", response.StatusCode)
	}
	var stored models.Project
	_ = f.DB.Collection("projects").FindOne(f.Ctx, bson.M{"_id": project}).Decode(&stored)
	if stored.IsActive {
		t.Fatal("failed reactivation changed project")
	}
}
