package middlewares

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/utils"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPanicRecoverySanitizesResponseAndKeepsServing(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: func(c *fiber.Ctx, err error) error { return utils.SendErrorResponse(c, err, "Internal server error") }})
	app.Use(PanicRecovery())
	app.Get("/panic", func(c *fiber.Ctx) error { c.SendString("private-partial-body"); panic("private-credential") })
	app.Get("/ok", func(c *fiber.Ctx) error { return c.SendString("ok") })
	for _, path := range []string{"/panic", "/ok"} {
		resp, err := app.Test(httptest.NewRequest("GET", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if path == "/panic" && (resp.StatusCode != 500 || strings.Contains(string(body), "private")) {
			t.Fatalf("panic response %d %s", resp.StatusCode, body)
		}
		if path == "/ok" && (resp.StatusCode != 200 || string(body) != "ok") {
			t.Fatalf("subsequent request %d %s", resp.StatusCode, body)
		}
	}
}
