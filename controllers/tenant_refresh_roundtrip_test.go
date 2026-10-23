package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
)

func TestTenantRefreshRestoresOnlyTenantSession(t *testing.T) {
	t.Setenv("NODE_ENV", "production")
	t.Setenv("TENANT_JWT_SECRET", "tenant-refresh-roundtrip-secret")
	config := configs.GetAppConfig()
	originalOrigins := config.CorsWhitelist
	originalMembership, originalRotation := loadManagementMembership, rotateManagementAuthSession
	t.Cleanup(func() {
		config.CorsWhitelist = originalOrigins
		loadManagementMembership, rotateManagementAuthSession = originalMembership, originalRotation
	})
	config.CorsWhitelist = []string{"https://tenant.autoapi.org"}
	tokens, err := utils.GenerateTenantTokens("tenant-user", "user@example.com", "tenant-1", "", []string{models.TenantRoleOwner}, string(models.RoleScopeTenant))
	if err != nil {
		t.Fatal(err)
	}
	initial, err := utils.ParseTenantRefreshToken(tokens.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	project, err := utils.GenerateTenantTokens("tenant-user", "user@example.com", "tenant-1", "project-1", []string{models.ProjectRoleAdmin}, string(models.RoleScopeProject))
	if err != nil {
		t.Fatal(err)
	}
	loadManagementMembership = func(_ context.Context, subject models.ManagementAuthorizationSubject, target models.ManagementAuthorizationTarget) (models.ManagementMembershipSnapshot, error) {
		if subject.UserID != "tenant-user" || target.TenantID != "tenant-1" || target.ProjectID != "" {
			t.Error("tenant refresh crossed identity scope")
		}
		return models.ManagementMembershipSnapshot{UserActive: true, TenantActive: true, TenantMembership: &models.TenantMembership{Status: models.MembershipStatusActive, Roles: []string{models.TenantRoleOwner}}}, nil
	}
	rotations := 0
	rotateManagementAuthSession = func(_ context.Context, family, presented, next string, expires time.Time) error {
		rotations++
		if family != initial.FamilyID || presented != initial.Id || next == "" || next == presented || !expires.After(time.Now()) {
			t.Error("invalid tenant rotation")
		}
		return nil
	}
	app := fiber.New()
	app.Post("/api/v1/tenant/auth/refresh", TenantRefreshToken)
	app.Get("/api/v1/tenant/auth/session", TenantSession)
	refresh := func(token string) *http.Response {
		req := httptest.NewRequest("POST", "/api/v1/tenant/auth/refresh", nil)
		req.Header.Set("Origin", "https://tenant.autoapi.org")
		req.AddCookie(&http.Cookie{Name: utils.AuthCookieName(utils.TokenScopeTenant, "refresh"), Value: token})
		req.AddCookie(&http.Cookie{Name: utils.ManagementProjectAuthCookieName("acme", "shop", "refresh"), Value: project.RefreshToken})
		req.AddCookie(&http.Cookie{Name: utils.ProjectAuthCookieName("acme", "shop", "refresh"), Value: "separate-application-refresh"})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	resp := refresh(tokens.RefreshToken)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("refresh status=%d", resp.StatusCode)
	}
	sessionRequest := httptest.NewRequest("GET", "/api/v1/tenant/auth/session", nil)
	if len(resp.Cookies()) != 2 {
		t.Fatalf("refresh changed %d cookies, want two tenant cookies", len(resp.Cookies()))
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name != utils.AuthCookieName(utils.TokenScopeTenant, "access") && cookie.Name != utils.AuthCookieName(utils.TokenScopeTenant, "refresh") {
			t.Fatal("changed another authentication namespace")
		}
		if !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatal("tenant cookie protections changed")
		}
		sessionRequest.AddCookie(cookie)
	}
	session, err := app.Test(sessionRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Body.Close()
	var state struct {
		Authenticated bool   `json:"authenticated"`
		TenantID      string `json:"tenantId"`
	}
	if err := json.NewDecoder(session.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if session.StatusCode != 200 || !state.Authenticated || state.TenantID != "tenant-1" {
		t.Fatalf("tenant session not restored: %#v", state)
	}
	denied := refresh(project.RefreshToken)
	denied.Body.Close()
	if denied.StatusCode != 401 || rotations != 1 {
		t.Fatalf("project refresh accepted as tenant: status=%d rotations=%d", denied.StatusCode, rotations)
	}
	for _, cookie := range denied.Cookies() {
		if cookie.Name != utils.AuthCookieName(utils.TokenScopeTenant, "access") && cookie.Name != utils.AuthCookieName(utils.TokenScopeTenant, "refresh") {
			t.Fatal("tenant refresh failure changed another session")
		}
	}
}
