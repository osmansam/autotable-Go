package main

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/utils"
	"github.com/osmansam/autotableGo/ws"
	"net/http/httptest"
	"testing"
)

func TestWebsocketAuthenticationCarriesVerifiedIDs(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-key")
	tokens, err := utils.GenerateTokens("user", "admin", "tenant-a", "project-a", "acme", "orders")
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Use("/api/v1/:tenantSlug/:projectSlug/ws", projectWebsocketAuthentication(&configs.Config{CorsWhitelist: []string{"https://panel.example.test"}}))
	app.Get("/api/v1/:tenantSlug/:projectSlug/ws", func(c *fiber.Ctx) error {
		auth, ok := c.Locals(ws.AuthenticatedContextKey).(ws.AuthenticatedContext)
		if !ok || auth.TenantID != "tenant-a" || auth.ProjectID != "project-a" {
			return c.SendStatus(500)
		}
		return c.SendStatus(204)
	})
	for _, tc := range []struct {
		slug, token string
		status      int
	}{{"orders", tokens.AccessToken, 204}, {"other", tokens.AccessToken, 403}, {"orders", tokens.RefreshToken, 401}} {
		req := httptest.NewRequest("GET", "/api/v1/acme/"+tc.slug+"/ws", nil)
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Origin", "https://panel.example.test")
		req.Header.Set("Cookie", utils.ProjectAuthCookieName("acme", tc.slug, "access")+"="+tc.token)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Fatalf("route=%s status=%d want=%d", tc.slug, resp.StatusCode, tc.status)
		}
	}
}
