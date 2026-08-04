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
}
