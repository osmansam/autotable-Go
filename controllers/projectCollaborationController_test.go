package controllers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/models"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCollaborationBodyCannotSupplyAuthority(t *testing.T) {
	for _, body := range []string{`{"role":"project_viewer","expectedVersion":0,"authority":"tenant"}`, `{"role":"project_viewer"}`, `{"role":"project_viewer","expectedVersion":0.5}`, `{"role":"project_viewer","expectedVersion":null}`, `{"role":"project_viewer","expectedVersion":0} {"actor":"other"}`} {
		t.Run(body, func(t *testing.T) {
			app := fiber.New()
			app.Patch("/:id/members/:userId", func(c *fiber.Ctx) error {
				c.Locals(middlewares.ManagementPrincipalLocalKey, models.ManagementPrincipal{UserID: "user", TenantID: "tenant", Scope: "tenant"})
				return c.Next()
			}, ChangeProjectCollaboratorRole)
			request := httptest.NewRequest("PATCH", "/project/members/target", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response, err := app.Test(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != 400 {
				t.Fatalf("untrusted body reached service: %d", response.StatusCode)
			}
		})
	}
}
