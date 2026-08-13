package middlewares

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestConditionalSchemaNameSupportsArrayRouteParameter(t *testing.T) {
	app := fiber.New()
	app.Get("/:schema", func(c *fiber.Ctx) error {
		return c.SendString(conditionalSchemaName(c))
	})
	request := httptest.NewRequest("GET", "/checklist", nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	if string(body) != "checklist" {
		t.Fatalf("conditionalSchemaName() = %q, want checklist", body)
	}
}
