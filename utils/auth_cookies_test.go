package utils

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func TestSetAuthCookiesUsesHardenedProductionAttributes(t *testing.T) {
	t.Setenv("NODE_ENV", "production")
	now := time.Now()
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		SetProjectAuthCookies(c, "davinci", "goblin", AuthTokenPair{
			AccessToken:  "access-secret",
			RefreshToken: "refresh-secret",
			ATExpires:    now.Add(30 * time.Minute).Unix(),
			RTExpires:    now.Add(7 * 24 * time.Hour).Unix(),
		})
		return c.SendStatus(fiber.StatusNoContent)
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	cookies := resp.Header.Values("Set-Cookie")
	joined := strings.Join(cookies, "\n")
	for _, want := range []string{
		ProjectAuthCookieName("davinci", "goblin", "access") + "=access-secret",
		ProjectAuthCookieName("davinci", "goblin", "refresh") + "=refresh-secret",
		"path=/",
		"HttpOnly",
		"secure",
		"SameSite=Lax",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("Set-Cookie headers %q do not contain %q", joined, want)
		}
	}
}

func TestAuthCookiesUseDevelopmentNamesAndMatchingClearAttributes(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	app := fiber.New()
	app.Get("/set", func(c *fiber.Ctx) error {
		now := time.Now()
		SetAuthCookies(c, TokenScopeTenant, AuthTokenPair{
			AccessToken:  "access",
			RefreshToken: "refresh",
			ATExpires:    now.Add(time.Hour).Unix(),
			RTExpires:    now.Add(7 * 24 * time.Hour).Unix(),
		})
		return c.SendStatus(fiber.StatusNoContent)
	})
	app.Get("/clear", func(c *fiber.Ctx) error {
		ClearAuthCookies(c, TokenScopeTenant)
		return c.SendStatus(fiber.StatusNoContent)
	})

	setResp, err := app.Test(httptest.NewRequest("GET", "/set", nil))
	if err != nil {
		t.Fatalf("set request error = %v", err)
	}
	setHeaders := strings.Join(setResp.Header.Values("Set-Cookie"), "\n")
	if !strings.Contains(setHeaders, "tenant_access_token=access") || strings.Contains(setHeaders, "__Host-") || strings.Contains(setHeaders, "Secure") {
		t.Fatalf("development Set-Cookie headers = %q", setHeaders)
	}

	clearResp, err := app.Test(httptest.NewRequest("GET", "/clear", nil))
	if err != nil {
		t.Fatalf("clear request error = %v", err)
	}
	clearHeaders := strings.Join(clearResp.Header.Values("Set-Cookie"), "\n")
	for _, want := range []string{"tenant_access_token=", "tenant_refresh_token=", "path=/", "HttpOnly", "SameSite=Lax"} {
		if !strings.Contains(clearHeaders, want) {
			t.Fatalf("clear Set-Cookie headers %q do not contain %q", clearHeaders, want)
		}
	}
}

func TestAccessCookieMaxAgeDoesNotExceedTokenExpiration(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	now := time.Now()
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		SetProjectAuthCookies(c, "davinci", "goblin", AuthTokenPair{
			AccessToken:  "access",
			RefreshToken: "refresh",
			ATExpires:    now.Add(90 * time.Second).Unix(),
			RTExpires:    now.Add(7 * 24 * time.Hour).Unix(),
		})
		return c.SendStatus(fiber.StatusNoContent)
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == ProjectAuthCookieName("davinci", "goblin", "access") && cookie.Path == ProjectAuthCookiePath("davinci", "goblin") && (cookie.MaxAge <= 0 || cookie.MaxAge > 90) {
			t.Fatalf("access cookie MaxAge = %d, want 1..90", cookie.MaxAge)
		}
	}
}

func TestProjectAuthCookieNamesAreStableAndIsolatedByProject(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	first := ProjectAuthCookieName("davinci", "goblin", "access")
	second := ProjectAuthCookieName("davinci", "phoenix", "access")
	if first == second {
		t.Fatalf("project cookie names collide: %q", first)
	}
	if first != ProjectAuthCookieName("DAVINCI", "GOBLIN", "access") {
		t.Fatalf("project cookie name is not normalized and stable")
	}
}

func TestProjectAuthCookiePathScopesCookiesToOneProjectAPI(t *testing.T) {
	if got := ProjectAuthCookiePath("davinci", "goblin"); got != "/api/v1/davinci/goblin" {
		t.Fatalf("ProjectAuthCookiePath() = %q", got)
	}
	if ProjectAuthCookiePath("davinci", "goblin") == ProjectAuthCookiePath("davinci", "phoenix") {
		t.Fatal("different projects share a cookie path")
	}
}

func TestSetProjectAuthCookiesDoesNotOverwriteAnotherProject(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		now := time.Now()
		SetProjectAuthCookies(c, "davinci", "goblin", AuthTokenPair{AccessToken: "goblin", RefreshToken: "goblin-r", ATExpires: now.Add(time.Hour).Unix(), RTExpires: now.Add(7 * 24 * time.Hour).Unix()})
		SetProjectAuthCookies(c, "davinci", "phoenix", AuthTokenPair{AccessToken: "phoenix", RefreshToken: "phoenix-r", ATExpires: now.Add(time.Hour).Unix(), RTExpires: now.Add(7 * 24 * time.Hour).Unix()})
		return c.SendStatus(fiber.StatusNoContent)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	activeCookies := 0
	for _, cookie := range resp.Cookies() {
		if cookie.Path == "/" {
			continue
		}
		activeCookies++
		wantPath := "/api/v1/davinci/goblin"
		if cookie.Value == "phoenix" || cookie.Value == "phoenix-r" {
			wantPath = "/api/v1/davinci/phoenix"
		}
		if cookie.Path != wantPath {
			t.Fatalf("cookie %q path = %q, want %q", cookie.Name, cookie.Path, wantPath)
		}
	}
	if activeCookies != 4 {
		t.Fatalf("active cookie count = %d, want 4 distinct project cookies; headers=%q", activeCookies, resp.Header.Values("Set-Cookie"))
	}
}

func TestSetProjectAuthCookiesExpiresLegacyRootCookies(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		now := time.Now()
		SetProjectAuthCookies(c, "davinci", "goblin", AuthTokenPair{AccessToken: "a", RefreshToken: "r", ATExpires: now.Add(time.Hour).Unix(), RTExpires: now.Add(7 * 24 * time.Hour).Unix()})
		return c.SendStatus(fiber.StatusNoContent)
	})
	resp, _ := app.Test(httptest.NewRequest("GET", "/", nil))
	legacyClears := 0
	for _, cookie := range resp.Cookies() {
		if cookie.Path == "/" && cookie.Expires.Before(time.Now()) {
			legacyClears++
		}
	}
	if legacyClears != 2 {
		t.Fatalf("legacy root cookie clears = %d, want 2", legacyClears)
	}
}
