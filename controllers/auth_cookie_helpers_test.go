package controllers

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/utils"
)

func TestSendScopedAuthResponseSetsCookiesWithoutReturningTokens(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	now := time.Now()
	app := fiber.New()
	app.Post("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
		return sendScopedAuthResponse(c, fiber.StatusOK, "Login successful", utils.TokenScopeProject, utils.AuthTokenPair{
			AccessToken: "access-secret", RefreshToken: "refresh-secret",
			ATExpires: now.Add(time.Hour).Unix(), RTExpires: now.Add(7 * 24 * time.Hour).Unix(),
		}, fiber.Map{"user": fiber.Map{"id": "user-1"}})
	})

	resp, err := app.Test(httptest.NewRequest("POST", "/davinci/goblin", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "access-secret") || strings.Contains(string(body), "refresh-secret") {
		t.Fatalf("response leaks tokens: %s", body)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, _ := decoded["data"].(map[string]interface{})
	if user, ok := data["user"].(map[string]interface{}); !ok || user["id"] != "user-1" {
		t.Fatalf("response data = %#v", data)
	}
	joined := strings.Join(resp.Header.Values("Set-Cookie"), "\n")
	if !strings.Contains(joined, utils.ProjectAuthCookieName("davinci", "goblin", "access")+"=access-secret") || !strings.Contains(joined, utils.ProjectAuthCookieName("davinci", "goblin", "refresh")+"=refresh-secret") {
		t.Fatalf("Set-Cookie = %q", joined)
	}
}

func TestSendAnonymousSessionResponseIsNotCacheable(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error { return sendAnonymousSession(c) })
	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != fiber.StatusOK || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Pragma") != "no-cache" {
		t.Fatalf("response status/headers = %d %#v", resp.StatusCode, resp.Header)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"authenticated":false`) {
		t.Fatalf("body = %s", body)
	}
}
