package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/utils"
)

func TestProjectSessionRequestsRecoveryWhenOnlyRefreshCookieRemains(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	app := fiber.New()
	app.Get("/api/v1/:tenantSlug/:projectSlug/auth/session", ProjectSession)
	for _, cookie := range []string{"", utils.ProjectAuthCookieName("davinci", "iyikafa", "refresh") + "=refresh-session"} {
		req := httptest.NewRequest("GET", "/api/v1/davinci/iyikafa/auth/session", nil)
		req.Header.Set("Cookie", cookie)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		want := fiber.StatusOK
		if cookie != "" {
			want = fiber.StatusUnauthorized
		}
		if resp.StatusCode != want {
			t.Errorf("refresh cookie present=%v status=%d want=%d", cookie != "", resp.StatusCode, want)
		}
		if resp.Header.Get("Cache-Control") != "no-store" {
			t.Error("session response is cacheable")
		}
	}
}

func TestProjectRefreshRotatesCookiesAndRestoresSession(t *testing.T) {
	t.Setenv("NODE_ENV", "production")
	t.Setenv("JWT_SECRET", "project-refresh-test-secret")
	cfg := configs.GetAppConfig()
	previousOrigins := cfg.CorsWhitelist
	cfg.CorsWhitelist = []string{"https://bayi.davinciboardgame.com"}
	previousRotate := rotateProjectAuthSession
	t.Cleanup(func() { cfg.CorsWhitelist = previousOrigins; rotateProjectAuthSession = previousRotate })
	tokens, err := utils.GenerateTokens("user-1", "member", "tenant-1", "project-1", "davinci", "iyikafa")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := utils.ParseProjectRefreshToken(tokens.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	reject := false
	calls := 0
	rotateProjectAuthSession = func(_ context.Context, familyID, presentedID, nextID string, expiresAt time.Time) error {
		calls++
		if familyID != initial.FamilyID || presentedID != initial.ID || nextID == "" || nextID == presentedID || !expiresAt.After(time.Now()) {
			t.Error("invalid refresh rotation")
		}
		if reject {
			return errors.New("revoked session")
		}
		return nil
	}
	app := fiber.New()
	app.Post("/api/v1/:tenantSlug/:projectSlug/auth/refresh", ProjectRefresh)
	app.Get("/api/v1/:tenantSlug/:projectSlug/auth/session", ProjectSession)
	refresh := func(cookie string) *http.Response {
		req := httptest.NewRequest("POST", "/api/v1/davinci/iyikafa/auth/refresh", nil)
		req.Header.Set("Origin", "https://bayi.davinciboardgame.com")
		req.AddCookie(&http.Cookie{Name: utils.ProjectAuthCookieName("davinci", "iyikafa", "refresh"), Value: cookie})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	resp := refresh(tokens.RefreshToken)
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("valid refresh status=%d", resp.StatusCode)
	}
	req := httptest.NewRequest("GET", "/api/v1/davinci/iyikafa/auth/session", nil)
	count := 0
	for _, cookie := range resp.Cookies() {
		if cookie.Value == "" {
			continue
		}
		count++
		if !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/api/v1/davinci/iyikafa" {
			t.Error("invalid project cookie attributes")
		}
		req.AddCookie(cookie)
	}
	if count != 2 {
		t.Fatalf("rotated cookies=%d", count)
	}
	session, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Body.Close()
	var state struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(session.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if session.StatusCode != 200 || !state.Authenticated {
		t.Fatal("rotated access cookie did not restore session")
	}
	reject = true
	denied := refresh(tokens.RefreshToken)
	denied.Body.Close()
	if denied.StatusCode != 401 {
		t.Fatalf("revoked refresh status=%d", denied.StatusCode)
	}
	for _, cookie := range denied.Cookies() {
		if cookie.Value != "" || !cookie.Expires.Before(time.Now()) {
			t.Error("rejected refresh retained a session cookie")
		}
	}
	invalid := refresh("invalid")
	invalid.Body.Close()
	if invalid.StatusCode != 401 || calls != 2 {
		t.Fatalf("invalid refresh status=%d rotation calls=%d", invalid.StatusCode, calls)
	}
}
