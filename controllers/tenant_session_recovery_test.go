package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
)

func TestTenantSessionRequestsRecoveryOnlyForTenantRefreshCookie(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	t.Setenv("TENANT_JWT_SECRET", "tenant-session-test-secret")
	app := fiber.New()
	app.Get("/api/v1/tenant/auth/session", TenantSession)
	for _, tc := range []struct {
		name, cookie string
		status       int
	}{
		{"anonymous", "", http.StatusOK},
		{"tenant refresh remains", utils.AuthCookieName(utils.TokenScopeTenant, "refresh") + "=remaining-session", http.StatusUnauthorized},
		{"invalid access with tenant refresh", utils.AuthCookieName(utils.TokenScopeTenant, "access") + "=expired; " + utils.AuthCookieName(utils.TokenScopeTenant, "refresh") + "=remaining-session", http.StatusUnauthorized},
		{"application refresh is separate", utils.ProjectAuthCookieName("acme", "shop", "refresh") + "=application-session", http.StatusOK},
		{"project management refresh is separate", utils.ManagementProjectAuthCookieName("acme", "shop", "refresh") + "=management-session", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/tenant/auth/session", nil)
			req.Header.Set("Cookie", tc.cookie)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.status {
				t.Fatalf("status=%d want=%d", resp.StatusCode, tc.status)
			}
			if resp.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("session response must not be cached")
			}
			if len(resp.Cookies()) != 0 {
				t.Fatal("session check must not change cookies")
			}
		})
	}
}

func TestTenantSessionAcceptsOnlyTenantManagementAccessIdentity(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	t.Setenv("TENANT_JWT_SECRET", "tenant-session-test-secret")
	t.Setenv("JWT_SECRET", "application-session-test-secret")
	tenant, err := utils.GenerateTenantTokens("user-1", "user@example.com", "tenant-1", "", []string{models.TenantRoleOwner}, string(models.RoleScopeTenant))
	if err != nil {
		t.Fatal(err)
	}
	project, err := utils.GenerateTenantTokens("user-1", "user@example.com", "tenant-1", "project-1", []string{models.ProjectRoleAdmin}, string(models.RoleScopeProject))
	if err != nil {
		t.Fatal(err)
	}
	application, err := utils.GenerateTokens("app-user", "admin", "tenant-1", "project-1", "acme", "shop")
	if err != nil {
		t.Fatal(err)
	}
	missingUser, err := utils.GenerateTenantTokens("", "user@example.com", "tenant-1", "", []string{models.TenantRoleOwner}, string(models.RoleScopeTenant))
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Get("/api/v1/tenant/auth/session", TenantSession)
	for _, tc := range []struct {
		name, cookieName, token string
		authenticated           bool
	}{
		{"tenant access", utils.AuthCookieName(utils.TokenScopeTenant, "access"), tenant.AccessToken, true},
		{"project management cannot become tenant", utils.AuthCookieName(utils.TokenScopeTenant, "access"), project.AccessToken, false},
		{"application cannot become tenant", utils.AuthCookieName(utils.TokenScopeTenant, "access"), application.AccessToken, false},
		{"tenant refresh cannot become access", utils.AuthCookieName(utils.TokenScopeTenant, "access"), tenant.RefreshToken, false},
		{"missing identity rejected", utils.AuthCookieName(utils.TokenScopeTenant, "access"), missingUser.AccessToken, false},
		{"project cookie cannot authenticate tenant", utils.ManagementProjectAuthCookieName("acme", "shop", "access"), project.AccessToken, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/tenant/auth/session", nil)
			req.AddCookie(&http.Cookie{Name: tc.cookieName, Value: tc.token})
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var state struct {
				Authenticated bool `json:"authenticated"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
				t.Fatal(err)
			}
			if state.Authenticated != tc.authenticated {
				t.Fatalf("authenticated=%v want=%v", state.Authenticated, tc.authenticated)
			}
		})
	}
}
