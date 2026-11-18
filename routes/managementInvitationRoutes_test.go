package routes

import (
	"github.com/gofiber/fiber/v2"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCollaborationContinuationCSRFAndMismatch(t *testing.T) {
	t.Setenv("COLLABORATION_MEMBERS_ENABLED", "false")
	app := fiber.New()
	ManagementInvitationRoutes(app)
	for _, path := range []string{"exchange", "reauthenticate", "verification", "verify", "accept"} {
		request := httptest.NewRequest("POST", "/api/v1/tenant/auth/invitations/"+path, strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "https://untrusted.example.test")
		response, err := app.Test(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 403 {
			t.Fatalf("public mutation %s bypassed Origin check: %d", path, response.StatusCode)
		}
	}
}
