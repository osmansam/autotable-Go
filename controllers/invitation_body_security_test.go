package controllers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/services"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCollaborationAcceptanceCannotRecoverAccount(t *testing.T) {
	for _, body := range []string{`{"mode":"existing","password":"replacement-password"}`, `{"mode":"existing","userId":"another-user"}`, `{"mode":"new","tenantId":"another-tenant","name":"Someone","password":"some-password"}`, `{"mode":"new","role":"tenant_owner"}`} {
		app := fiber.New()
		app.Post("/", AcceptProjectInvitation)
		request := httptest.NewRequest("POST", "/", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response, err := app.Test(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 400 {
			t.Fatalf("untrusted acceptance body reached account mutation: %d", response.StatusCode)
		}
	}
}

func TestCollaborationRecoveryErrorGuidesAdministrator(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error { return collaborationError(c, services.ErrCollaborationRecoveryRequired) })
	response, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 409 || !strings.Contains(string(body), "collaboration_recovery_required") || !strings.Contains(string(body), "administrator recovery") {
		t.Fatal("adminless operation did not point to recovery")
	}
}
