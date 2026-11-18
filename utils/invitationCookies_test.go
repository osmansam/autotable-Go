package utils

import (
	"github.com/gofiber/fiber/v2"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCollaborationContinuationCookieSecurity(t *testing.T) {
	t.Setenv("NODE_ENV", "production")
	app := fiber.New()
	app.Post("/", func(c *fiber.Ctx) error {
		SetInvitationContinuationCookie(c, "opaque-browser-handle", time.Now().Add(30*time.Minute))
		return c.SendStatus(204)
	})
	response, err := app.Test(httptest.NewRequest("POST", "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatal("continuation cookie missing")
	}
	cookie := cookies[0]
	if cookie.Name != "__Host-invitation_continuation" || !cookie.Secure || !cookie.HttpOnly || cookie.Domain != "" || cookie.Path != "/" || cookie.MaxAge > 1800 || cookie.SameSite != 2 {
		t.Fatalf("insecure continuation cookie: %#v", cookie)
	}
}
