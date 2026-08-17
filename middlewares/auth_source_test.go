package middlewares

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/utils"
)

func TestResolveCredentialPrefersExplicitBearerOverCookie(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	app := fiber.New()
	app.Get("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
		credential, err := ResolveProjectCredential(c)
		if err != nil {
			return err
		}
		return c.SendString(string(credential.Source) + ":" + credential.Token)
	})
	req := httptest.NewRequest("GET", "/davinci/goblin", nil)
	req.Header.Set("Authorization", "Bearer explicit-token")
	req.AddCookie(&http.Cookie{Name: utils.ProjectAuthCookieName("davinci", "goblin", "access"), Value: "cookie-token"})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}
	if string(body) != "bearer:explicit-token" {
		t.Fatalf("body = %q", body)
	}
}

func TestResolveCredentialDoesNotFallBackFromMalformedBearer(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	app := fiber.New()
	app.Get("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
		_, err := ResolveProjectCredential(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).SendString(err.Error())
		}
		return c.SendStatus(fiber.StatusOK)
	})
	req := httptest.NewRequest("GET", "/davinci/goblin", nil)
	req.Header.Set("Authorization", "not-a-bearer-token")
	req.AddCookie(&http.Cookie{Name: utils.ProjectAuthCookieName("davinci", "goblin", "access"), Value: "valid-cookie"})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestTrustedOriginRequiresExactConfiguredOrigin(t *testing.T) {
	allowed := []string{"https://panel.autoapi.org", "https://tenant.autoapi.org"}
	tests := []struct {
		origin string
		want   bool
	}{
		{"https://panel.autoapi.org", true},
		{"https://panel.autoapi.org:444", false},
		{"http://panel.autoapi.org", false},
		{"https://evil-autoapi.org", false},
		{"null", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsTrustedOrigin(tt.origin, allowed); got != tt.want {
			t.Errorf("IsTrustedOrigin(%q) = %v, want %v", tt.origin, got, tt.want)
		}
	}
}

func TestCookieOriginMiddlewareUsesSelectedAuthSource(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(AuthSourceLocalKey, AuthSourceCookie)
		return c.Next()
	})
	app.Use(RequireTrustedCookieOrigin([]string{"https://panel.autoapi.org"}))
	app.Post("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })

	for _, origin := range []string{"", "null", "https://evil-autoapi.org"} {
		req := httptest.NewRequest("POST", "/", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test(%q) error = %v", origin, err)
		}
		if resp.StatusCode != fiber.StatusForbidden {
			t.Fatalf("origin %q status = %d, want 403", origin, resp.StatusCode)
		}
	}
}

func TestAuthenticateAcceptsProjectAccessCookie(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	tokens, err := utils.GenerateTokens("user", "admin", "tenant", "project", "davinci", "goblin")
	if err != nil {
		t.Fatalf("GenerateTokens() error = %v", err)
	}
	app := fiber.New()
	app.Get("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
		c.Locals("expectedTenantID", "tenant")
		c.Locals("expectedProjectID", "project")
		return Authenticate(c, false, nil, true)
	}, func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	req := httptest.NewRequest("GET", "/davinci/goblin", nil)
	req.AddCookie(&http.Cookie{Name: utils.ProjectAuthCookieName("davinci", "goblin", "access"), Value: tokens.AccessToken})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestAuthenticateRejectsCookieMutationWithoutTrustedOrigin(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	tokens, err := utils.GenerateTokens("user", "admin", "tenant", "project", "davinci", "goblin")
	if err != nil {
		t.Fatalf("GenerateTokens() error = %v", err)
	}
	app := fiber.New()
	app.Post("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
		return Authenticate(c, false, nil, true)
	}, func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	req := httptest.NewRequest("POST", "/davinci/goblin", nil)
	req.AddCookie(&http.Cookie{Name: utils.ProjectAuthCookieName("davinci", "goblin", "access"), Value: tokens.AccessToken})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestResolveProjectCredentialSelectsCookieForRouteSlugs(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	app := fiber.New()
	app.Get("/api/v1/:tenantSlug/:projectSlug/test", func(c *fiber.Ctx) error {
		credential, err := ResolveProjectCredential(c)
		if err != nil {
			return err
		}
		return c.SendString(credential.Token)
	})
	req := httptest.NewRequest("GET", "/api/v1/davinci/goblin/test", nil)
	req.AddCookie(&http.Cookie{Name: utils.ProjectAuthCookieName("davinci", "goblin", "access"), Value: "goblin-token"})
	req.AddCookie(&http.Cookie{Name: utils.ProjectAuthCookieName("davinci", "phoenix", "access"), Value: "phoenix-token"})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "goblin-token" {
		t.Fatalf("selected token = %q, want goblin-token", body)
	}
}
