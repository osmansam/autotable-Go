package routes

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestLocalizationSettingsArePublicButPreferencesRemainProtected(t *testing.T) {
	app := fiber.New()
	registerLocalizationRoutes(
		app,
		func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) },
		func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) },
		func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) },
	)

	settingsResponse, err := app.Test(httptest.NewRequest("GET", "/api/v1/acme/store/localization/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	if settingsResponse.StatusCode != fiber.StatusOK {
		t.Fatalf("settings status = %d, want %d", settingsResponse.StatusCode, fiber.StatusOK)
	}

	translationsResponse, err := app.Test(httptest.NewRequest("GET", "/api/v1/acme/store/localization/translations?locale=tr", nil))
	if err != nil {
		t.Fatal(err)
	}
	if translationsResponse.StatusCode != fiber.StatusOK {
		t.Fatalf("translations status = %d, want %d", translationsResponse.StatusCode, fiber.StatusOK)
	}

	preferenceResponse, err := app.Test(httptest.NewRequest("PUT", "/api/v1/acme/store/localization/preference", nil))
	if err != nil {
		t.Fatal(err)
	}
	if preferenceResponse.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("preference status = %d, want %d", preferenceResponse.StatusCode, fiber.StatusUnauthorized)
	}

	for _, request := range []struct{ method, path string }{
		{"GET", "/api/v1/acme/store/localization/editor/translations?locale=tr"},
		{"PATCH", "/api/v1/acme/store/localization/translations/tr/ozmishow%3AsitePages%3Ahome%3Atitle"},
		{"POST", "/api/v1/acme/store/localization/reconcile"},
		{"POST", "/api/v1/acme/store/localization/translations/generate"},
		{"GET", "/api/v1/acme/store/localization/translation-jobs/507f1f77bcf86cd799439011"},
	} {
		response, err := app.Test(httptest.NewRequest(request.method, request.path, nil))
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != fiber.StatusNotFound {
			t.Fatalf("%s %s status = %d, want %d (application editor routes are disabled)", request.method, request.path, response.StatusCode, fiber.StatusNotFound)
		}
	}
}

func TestApplicationLocalizationEditorRoutesAreNotRegistered(t *testing.T) {
	app := fiber.New()
	registerLocalizationRoutes(app, okRoute, okRoute, okRoute)
	registered := registeredRouteSet(app)
	for _, route := range []string{
		"GET /api/v1/:tenantSlug/:projectSlug/localization/editor/translations",
		"POST /api/v1/:tenantSlug/:projectSlug/localization/reconcile",
		"POST /api/v1/:tenantSlug/:projectSlug/localization/translations/generate",
		"GET /api/v1/:tenantSlug/:projectSlug/localization/translation-jobs/:jobId",
		"PATCH /api/v1/:tenantSlug/:projectSlug/localization/translations/:locale/:key",
	} {
		if registered[route] {
			t.Errorf("application route %s must stay disabled", route)
		}
	}
	for _, route := range []string{
		"GET /api/v1/:tenantSlug/:projectSlug/localization/settings",
		"GET /api/v1/:tenantSlug/:projectSlug/localization/translations",
		"PUT /api/v1/:tenantSlug/:projectSlug/localization/preference",
	} {
		if !registered[route] {
			t.Errorf("runtime route %s must remain registered", route)
		}
	}
}

func okRoute(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }
