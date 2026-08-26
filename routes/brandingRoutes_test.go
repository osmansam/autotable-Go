package routes

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/utils"
)

func brandingStub(c *fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) }

func TestBrandingRuntimeRouteIsPublic(t *testing.T) {
	app := fiber.New()
	registerBrandingRoutes(app, brandingRouteHandlers{
		Runtime: brandingStub,
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/acme/inventory/branding", nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestBrandingUploadAcceptsImageWithinTwoMegabyteContract(t *testing.T) {
	tokens, err := utils.GenerateTokens("user", "admin", "507f1f77bcf86cd799439011", "507f1f77bcf86cd799439012", "acme", "inventory")
	if err != nil {
		t.Fatalf("GenerateTokens() error = %v", err)
	}

	app := fiber.New()
	registerBrandingRoutes(app, brandingRouteHandlers{UploadTenant: brandingStub})
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/tenant/branding/assets/logo",
		bytes.NewReader(make([]byte, 1536*1024)),
	)
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}
}

func TestBrandingManagementRoutesRequireAuthentication(t *testing.T) {
	app := fiber.New()
	registerBrandingRoutes(app, brandingRouteHandlers{
		Runtime:       brandingStub,
		GetTenant:     brandingStub,
		PatchTenant:   brandingStub,
		UploadTenant:  brandingStub,
		DeleteTenant:  brandingStub,
		GetProject:    brandingStub,
		PatchProject:  brandingStub,
		UploadProject: brandingStub,
		DeleteProject: brandingStub,
	})
	paths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/tenant/branding"},
		{http.MethodPatch, "/api/v1/tenant/branding"},
		{http.MethodPost, "/api/v1/tenant/branding/assets/logo"},
		{http.MethodDelete, "/api/v1/tenant/branding/assets/logo"},
		{http.MethodGet, "/api/v1/tenant/projects/507f1f77bcf86cd799439011/branding"},
		{http.MethodPatch, "/api/v1/tenant/projects/507f1f77bcf86cd799439011/branding"},
	}
	for _, item := range paths {
		request := httptest.NewRequest(item.method, item.path, nil)
		response, err := app.Test(request)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d", item.method, item.path, response.StatusCode)
		}
	}
}
