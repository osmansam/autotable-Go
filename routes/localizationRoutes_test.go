package routes

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
		if response.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, want %d", request.method, request.path, response.StatusCode, fiber.StatusUnauthorized)
		}
	}
}

func TestLocalizationStaticRoutesPrecedeTranslationKeyMutation(t *testing.T) {
	app := fiber.New()
	registerLocalizationRoutes(app, okRoute, okRoute, okRoute)
	routes := app.GetRoutes(true)
	generateIndex, mutationIndex := -1, -1
	for index, route := range routes {
		if route.Method == "POST" && route.Path == "/api/v1/:tenantSlug/:projectSlug/localization/translations/generate" {
			generateIndex = index
		}
		if route.Method == "PATCH" && route.Path == "/api/v1/:tenantSlug/:projectSlug/localization/translations/:locale/:key" {
			mutationIndex = index
		}
	}
	if generateIndex < 0 || mutationIndex < 0 || generateIndex > mutationIndex {
		t.Fatalf("route order generate=%d mutation=%d", generateIndex, mutationIndex)
	}
}

func TestLocalizationEditorRoutesRequireAuthenticationButNotProjectRoleAuthorization(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	tenantID, projectID := primitive.NewObjectID(), primitive.NewObjectID()
	server := miniredis.RunT(t)
	oldClient := configs.RedisClient
	configs.RedisClient = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = configs.RedisClient.Close()
		configs.RedisClient = oldClient
	})
	if err := configs.RedisClient.Set(context.Background(), "slug_mapping:acme:store", tenantID.Hex()+"|"+projectID.Hex(), 0).Err(); err != nil {
		t.Fatal(err)
	}
	tokens, err := utils.GenerateTokens("viewer-user", models.ProjectRoleViewer, tenantID.Hex(), projectID.Hex(), "acme", "store")
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	registerLocalizationRoutes(app, okRoute, okRoute, okRoute)
	request := httptest.NewRequest("GET", "/api/v1/acme/store/localization/translation-jobs/not-an-object-id", nil)
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("viewer status = %d, want %d", response.StatusCode, fiber.StatusBadRequest)
	}
}

func okRoute(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }
