package routes

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/utils"
)

func TestContainerReadSelectsApplicationMetadataWithoutGrantingManagementAccess(t *testing.T) {
	t.Setenv("JWT_SECRET", "container-runtime-test-secret")
	t.Setenv("TENANT_JWT_SECRET", "container-management-test-secret")
	t.Setenv("NODE_ENV", "development")
	tokens, err := utils.GenerateTokens("user-1", "member", "tenant-1", "project-1", "davinci", "iyikafa")
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	runtimeAuth := func(c *fiber.Ctx) error {
		c.Locals("expectedTenantID", "tenant-1")
		c.Locals("expectedProjectID", "project-1")
		return middlewares.Authenticate(c, false, nil, true)
	}
	managementCalled := false
	app.Get("/api/v1/:tenantSlug/:projectSlug/container", selectContainerReadHandlers(
		[]fiber.Handler{runtimeAuth, func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"view": "runtime", "user": c.Locals("userID")}) }},
		[]fiber.Handler{middlewares.ManagementAuthenticate, func(c *fiber.Ctx) error { managementCalled = true; return c.SendStatus(200) }},
	)...)
	for _, bearer := range []bool{false, true} {
		req := httptest.NewRequest("GET", "/api/v1/davinci/iyikafa/container", nil)
		if bearer {
			req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
		} else {
			req.Header.Set("Cookie", utils.ProjectAuthCookieName("davinci", "iyikafa", "access")+"="+tokens.AccessToken)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("application metadata bearer=%v status=%d", bearer, resp.StatusCode)
		}
	}
	if managementCalled {
		t.Fatal("application session reached management controller")
	}
	for _, cookie := range []string{"", utils.ProjectAuthCookieName("davinci", "iyikafa", "access") + "=invalid", utils.ProjectAuthCookieName("davinci", "iyikafa", "refresh") + "=" + tokens.RefreshToken} {
		req := httptest.NewRequest("GET", "/api/v1/davinci/iyikafa/container", nil)
		req.Header.Set("Cookie", cookie)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Errorf("missing/stale access status=%d", resp.StatusCode)
		}
	}
	wrong, err := utils.GenerateTokens("user-1", "member", "other", "project-1", "davinci", "iyikafa")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/v1/davinci/iyikafa/container", nil)
	req.Header.Set("Authorization", "Bearer "+wrong.AccessToken)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Errorf("foreign project identity status=%d", resp.StatusCode)
	}
	if managementCalled {
		t.Fatal("unauthorized application session reached management controller")
	}
	// Existing tenantPanel sessions keep their management branch even when both apps are signed in.
	req = httptest.NewRequest("GET", "/api/v1/davinci/iyikafa/container", nil)
	req.Header.Set("Cookie", strings.Join([]string{utils.ProjectAuthCookieName("davinci", "iyikafa", "access") + "=" + tokens.AccessToken, utils.ManagementProjectAuthCookieName("davinci", "iyikafa", "access") + "=invalid-management"}, "; "))
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("invalid management session fell back to runtime: %d", resp.StatusCode)
	}
	manager, err := utils.GenerateTenantTokens("manager-1", "manager@example.test", "tenant-1", "project-1", []string{"project_developer"}, "project")
	if err != nil {
		t.Fatal(err)
	}
	for _, bearer := range []bool{false, true} {
		managementCalled = false
		req := httptest.NewRequest("GET", "/api/v1/davinci/iyikafa/container", nil)
		if bearer {
			req.Header.Set("Authorization", "Bearer "+manager.AccessToken)
		} else {
			req.Header.Set("Cookie", strings.Join([]string{utils.ProjectAuthCookieName("davinci", "iyikafa", "access") + "=" + tokens.AccessToken, utils.ManagementProjectAuthCookieName("davinci", "iyikafa", "access") + "=" + manager.AccessToken}, "; "))
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 || !managementCalled {
			t.Fatalf("valid management dispatch bearer=%v status=%d", bearer, resp.StatusCode)
		}
	}
}
