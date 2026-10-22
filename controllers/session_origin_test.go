package controllers

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
)

func TestProjectRefreshUsesConfiguredTrustedOrigins(t *testing.T) {
	cfg := configs.GetAppConfig()
	previous := cfg.CorsWhitelist
	cfg.CorsWhitelist = []string{"https://bayi.davinciboardgame.com"}
	t.Cleanup(func() { cfg.CorsWhitelist = previous })
	app := fiber.New()
	app.Post("/api/v1/:tenantSlug/:projectSlug/auth/refresh", ProjectRefresh)
	for _, tc := range []struct {
		origin string
		status int
	}{
		{"https://bayi.davinciboardgame.com", fiber.StatusUnauthorized}, // Trusted origin, missing refresh cookie.
		{"https://tenant.autoapi.org", fiber.StatusForbidden},           // Not configured in this deployment.
		{"https://attacker.example", fiber.StatusForbidden},
		{"https://bayi.davinciboardgame.com.attacker.example", fiber.StatusForbidden},
		{"", fiber.StatusForbidden}, {"null", fiber.StatusForbidden},
	} {
		req := httptest.NewRequest("POST", "/api/v1/davinci/iyikafa/auth/refresh", nil)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Errorf("origin %q status = %d, want %d", tc.origin, resp.StatusCode, tc.status)
		}
	}
}
