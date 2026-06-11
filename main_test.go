package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
)

func TestArithmeticHandlers(t *testing.T) {
	app := fiber.New()
	app.Get("/sum", func(c *fiber.Ctx) error {
		value, err := Sum(c)
		if err != nil {
			return err
		}
		return c.JSON(value)
	})
	app.Get("/multiple", func(c *fiber.Ctx) error {
		value, err := Multiple(c)
		if err != nil {
			return err
		}
		return c.JSON(value)
	})
	for path, want := range map[string]string{
		"/sum?a=2&b=3":      "5",
		"/multiple?a=2&b=3": "6",
	} {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatalf("app.Test(%s) error = %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != want {
			t.Fatalf("body = %q, want %q", body, want)
		}
	}
}

func TestArithmeticHandlerErrors(t *testing.T) {
	app := fiber.New()
	app.Get("/sum", func(c *fiber.Ctx) error {
		_, err := Sum(c)
		return err
	})
	app.Get("/multiple", func(c *fiber.Ctx) error {
		_, err := Multiple(c)
		return err
	})
	for _, path := range []string{"/sum?a=invalid&b=2", "/multiple?a=2&b=invalid"} {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatalf("app.Test(%q) error = %v", path, err)
		}
		if resp.StatusCode < http.StatusBadRequest {
			t.Fatalf("%s status = %d, want error status", path, resp.StatusCode)
		}
	}
}

func TestCorsFromConfig(t *testing.T) {
	app := fiber.New()
	app.Use(corsFromConfig(&configs.Config{CorsWhitelist: []string{" https://example.com ", ""}}))
	app.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	resp, err := app.Test(req)
	if err != nil || resp.Header.Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Fatalf("CORS response = %#v, %v", resp.Header, err)
	}
}

func TestIsProduction(t *testing.T) {
	t.Setenv("NODE_ENV", "production")
	if !isProduction() {
		t.Fatal("isProduction() = false")
	}
	t.Setenv("NODE_ENV", "development")
	if isProduction() {
		t.Fatal("isProduction() = true")
	}
}

func TestConfigurePprof(t *testing.T) {
	t.Setenv("NODE_ENV", "production")
	app := fiber.New()
	configurePprof(app, ":3000")
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	if err != nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("production pprof status = %v, error = %v", resp, err)
	}

	t.Setenv("NODE_ENV", "development")
	app = fiber.New()
	configurePprof(app, ":3000")
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	if err != nil || resp.StatusCode == http.StatusNotFound {
		t.Fatalf("development pprof status = %v, error = %v", resp, err)
	}
}
