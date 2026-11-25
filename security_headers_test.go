package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	clientws "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	serverws "github.com/gofiber/websocket/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

func TestSecurityHeadersPreserveSwaggerAndCORS(t *testing.T) {
	app := fiber.New()
	app.Use(middlewares.SecurityHeaders())
	app.Use(corsFromConfig(&configs.Config{CorsWhitelist: []string{"https://panel.example.test"}}))
	app.Get("/api/swagger", controllers.GetSwaggerUI)
	app.Get("/api/data", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"ok": true}) })
	for _, path := range []string{"/api/swagger", "/api/data", "/missing"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Origin", "https://panel.example.test")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
			t.Fatalf("missing headers: %v", resp.Header)
		}
		if resp.Header.Get("Access-Control-Allow-Origin") != "https://panel.example.test" || resp.Header.Get("Access-Control-Allow-Credentials") != "true" {
			t.Fatalf("CORS broken: %v", resp.Header)
		}
		if resp.Header.Get("X-Frame-Options") != "" || resp.Header.Get("Cross-Origin-Resource-Policy") != "" {
			t.Fatal("customer embedding blocked")
		}
		if path == "/api/swagger" && (!strings.Contains(string(body), "https://unpkg.com/swagger-ui-dist") || resp.Header.Get("Content-Security-Policy") != "") {
			t.Fatal("Swagger script loading blocked")
		}
	}
	req := httptest.NewRequest("OPTIONS", "/api/data", nil)
	req.Header.Set("Origin", "https://panel.example.test")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Authorization, Idempotency-Key, X-AutoTable-Client")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 || resp.Header.Get("Access-Control-Allow-Origin") != "https://panel.example.test" {
		t.Fatalf("preflight broken: %v", resp)
	}
}

func TestSecurityHeadersPreserveWebsocketUpgrade(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(middlewares.SecurityHeaders())
	app.Use(middlewares.PanicRecovery())
	app.Get("/ws", serverws.New(func(c *serverws.Conn) { defer c.Close(); _ = c.WriteMessage(serverws.TextMessage, []byte("ready")) }))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- app.Listener(listener) }()
	defer func() { _ = app.ShutdownWithTimeout(time.Second); _ = listener.Close(); <-done }()
	conn, resp, err := clientws.DefaultDialer.Dial("ws://"+listener.Addr().String()+"/ws", http.Header{"Origin": []string{"https://panel.example.test"}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if resp.StatusCode != 101 {
		t.Fatalf("upgrade status=%d", resp.StatusCode)
	}
	_, message, err := conn.ReadMessage()
	if err != nil || string(message) != "ready" {
		t.Fatalf("websocket payload=%s error=%v", message, err)
	}
}
