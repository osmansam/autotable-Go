package ws

import (
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	clientws "github.com/fasthttp/websocket"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	serverws "github.com/gofiber/websocket/v2"
	"github.com/osmansam/autotableGo/configs"
)

func TestAuthenticatedWebsocketRejectsCrossProjectMirrors(t *testing.T) {
	redisServer := miniredis.RunT(t)
	previous := configs.RedisClient
	configs.RedisClient = redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { configs.RedisClient.Close(); configs.RedisClient = previous }()
	redisServer.Set("slug_mapping:acme:orders", "tenant-a|project-a")
	redisServer.Set("slug_mapping:victim:private", "tenant-b|project-b")
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(AuthenticatedContextKey, AuthenticatedContext{TenantID: "tenant-a", ProjectID: "project-a", TenantSlug: "acme", ProjectSlug: "orders"})
		return c.Next()
	})
	app.Get("/ws", serverws.New(HandleWS))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- app.Listener(listener) }()
	defer func() { app.ShutdownWithTimeout(time.Second); listener.Close(); <-done }()
	for _, query := range []string{"?tenantSlug=victim&projectSlug=private", "?tenantId=tenant-b&projectId=project-b", "?tenantSlug=acme&projectSlug=orders&projectId=project-b"} {
		conn, _, err := clientws.DefaultDialer.Dial("ws://"+listener.Addr().String()+"/ws"+query, http.Header{})
		if err != nil {
			t.Fatal(err)
		}
		conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		_, body, err := conn.ReadMessage()
		conn.Close()
		if err != nil || !strings.Contains(string(body), "Invalid tenant or project") {
			t.Fatalf("cross-project context was not rejected: body=%s err=%v", body, err)
		}
	}
	conn, _, err := clientws.DefaultDialer.Dial("ws://"+listener.Addr().String()+"/ws?tenantSlug=acme&projectSlug=orders", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	deadline := time.Now().Add(time.Second)
	for {
		clientsMu.RLock()
		registered := false
		for _, info := range clients {
			if info.TenantID == "tenant-a" && info.ProjectID == "project-a" {
				registered = true
			}
		}
		clientsMu.RUnlock()
		if registered {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("matching authenticated client did not register")
		}
		time.Sleep(time.Millisecond)
	}
	broadcastEvent(Event{Type: "invalidate", Schema: "orders", TenantID: "tenant-a", ProjectID: "project-a"})
	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, body, err := conn.ReadMessage()
	if err != nil || !strings.Contains(string(body), "orders") {
		t.Fatalf("legitimate subscription failed: %s %v", body, err)
	}
}

func TestWebsocketEvictionUnblocksReaderBeforePoolReuse(t *testing.T) {
	redisServer := miniredis.RunT(t)
	previous := configs.RedisClient
	configs.RedisClient = redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { configs.RedisClient.Close(); configs.RedisClient = previous }()
	redisServer.Set("slug_mapping:acme:orders", "tenant-a|project-a")
	handled := make(chan struct{})
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(AuthenticatedContextKey, AuthenticatedContext{TenantID: "tenant-a", ProjectID: "project-a", TenantSlug: "acme", ProjectSlug: "orders"})
		return c.Next()
	})
	app.Get("/ws", serverws.New(func(c *serverws.Conn) { defer close(handled); HandleWS(c) }))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- app.Listener(listener) }()
	defer func() { app.ShutdownWithTimeout(time.Second); listener.Close(); <-done }()
	conn, _, err := clientws.DefaultDialer.Dial("ws://"+listener.Addr().String()+"/ws?tenantSlug=acme&projectSlug=orders", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var serverConn *serverws.Conn
	var info *ClientInfo
	deadline := time.Now().Add(time.Second)
	for info == nil {
		clientsMu.RLock()
		for c, item := range clients {
			if item.ProjectID == "project-a" {
				serverConn, info = c, item
			}
		}
		clientsMu.RUnlock()
		if time.Now().After(deadline) {
			t.Fatal("not registered")
		}
		time.Sleep(time.Millisecond)
	}
	unregisterClient(serverConn, info, "eviction-test")
	select {
	case <-handled:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("eviction left reader blocked; adapter cannot return pooled connection")
	}
}
