package ozmishowratelimit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
)

var testSecret = strings.Repeat("service-", 5)
var testIdentity = strings.Repeat("a", 64)

func setup(t *testing.T) (*miniredis.Miniredis, *redis.Client, Config) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	return server, client, Config{Secret: testSecret, Namespace: "test", Policies: DefaultPolicies(), MaxConcurrent: 16}
}
func testApp(t *testing.T, cfg Config, client redis.Cmdable) *fiber.App {
	t.Helper()
	handler, err := NewHandler(cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.All(Path, handler.Handle)
	return app
}
func call(t *testing.T, app *fiber.App, body, token string) (int, Result) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := app.Test(req, 3000)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	bytes, _ := io.ReadAll(res.Body)
	var result Result
	_ = json.Unmarshal(bytes, &result)
	if strings.Contains(string(bytes), testSecret) {
		t.Fatal("response leaked credential")
	}
	if res.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("response must not be cached")
	}
	return res.StatusCode, result
}
func body(policy string) string {
	return `{"policy":"` + policy + `","identity":"` + testIdentity + `"}`
}

func TestServiceAuthAndRotation(t *testing.T) {
	server, client, cfg := setup(t)
	cfg.PreviousSecret = strings.Repeat("previous-", 5)
	app := testApp(t, cfg, client)
	for _, token := range []string{"", "normal-user-jwt", "wrong"} {
		if status, _ := call(t, app, body("login"), token); status != 401 {
			t.Fatalf("status %d", status)
		}
	}
	if len(server.Keys()) != 0 {
		t.Fatal("unauthenticated request accessed Redis")
	}
	for _, token := range []string{cfg.Secret, cfg.PreviousSecret} {
		if status, result := call(t, app, body("login"), token); status != 200 || !result.Allowed {
			t.Fatalf("%d %+v", status, result)
		}
	}
}
func TestStrictRequestContract(t *testing.T) {
	server, client, cfg := setup(t)
	app := testApp(t, cfg, client)
	for _, input := range []string{
		`null`, `[]`, `{}`, `{"policy":"unknown","identity":"` + testIdentity + `"}`,
		`{"policy":"login","identity":"raw-browser-ip"}`,
		`{"policy":"login","identity":"` + testIdentity + `","max":999999}`,
		`{"policy":"login","identity":"` + testIdentity + `","script":"FLUSHALL"}`,
		`{"policy":"login","policy":"admin","identity":"` + testIdentity + `"}`,
		body("login") + ` {}`, `{"policy":false,"identity":"` + testIdentity + `"}`,
	} {
		if status, _ := call(t, app, input, testSecret); status != 400 {
			t.Fatalf("input %s: %d", input, status)
		}
	}
	if status, _ := call(t, app, strings.Repeat("x", 1025), testSecret); status != 413 {
		t.Fatalf("oversize: %d", status)
	}
	if len(server.Keys()) != 0 {
		t.Fatal("invalid request accessed Redis")
	}
}
func TestBudgetsAreAtomicAcrossHandlerInstances(t *testing.T) {
	_, client, cfg := setup(t)
	cfg.MaxConcurrent = 128
	apps := []*fiber.App{testApp(t, cfg, client), testApp(t, cfg, client)}
	var allowed, denied atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 80; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			status, result := call(t, apps[i%2], body("login"), testSecret)
			if status == 200 && result.Allowed {
				allowed.Add(1)
			} else if status == 429 && !result.Allowed && result.RetryAfterSeconds > 0 {
				denied.Add(1)
			} else {
				t.Errorf("unexpected %d %+v", status, result)
			}
		}(i)
	}
	wg.Wait()
	if allowed.Load() != 10 || denied.Load() != 70 {
		t.Fatalf("allowed=%d denied=%d", allowed.Load(), denied.Load())
	}
}
func TestExpiryAndIndependentPolicies(t *testing.T) {
	server, client, cfg := setup(t)
	app := testApp(t, cfg, client)
	for i := 0; i < 10; i++ {
		call(t, app, body("login"), testSecret)
	}
	if status, _ := call(t, app, body("login"), testSecret); status != 429 {
		t.Fatal(status)
	}
	if status, _ := call(t, app, body("admin"), testSecret); status != 200 {
		t.Fatal(status)
	}
	server.FastForward(5 * time.Minute)
	if status, _ := call(t, app, body("login"), testSecret); status != 200 {
		t.Fatal(status)
	}
}
func TestRedisOOMAndOutageFailClosed(t *testing.T) {
	server, client, cfg := setup(t)
	app := testApp(t, cfg, client)
	server.SetError("OOM command not allowed when used memory > maxmemory")
	if status, result := call(t, app, body("login"), testSecret); status != 503 || result.Allowed {
		t.Fatalf("%d %+v", status, result)
	}
	server.SetError("")
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if status, result := call(t, app, body("login"), testSecret); status != 503 || result.Allowed {
		t.Fatalf("%d %+v", status, result)
	}
}
func TestCorruptNonexpiringCounterFailsClosed(t *testing.T) {
	server, client, cfg := setup(t)
	app := testApp(t, cfg, client)
	call(t, app, body("login"), testSecret)
	keys := server.Keys()
	if len(keys) != 1 {
		t.Fatal(keys)
	}
	client.Persist(context.Background(), keys[0])
	if status, _ := call(t, app, body("login"), testSecret); status != 503 {
		t.Fatal(status)
	}
}
func TestPoliciesPreserveCurrentBudgets(t *testing.T) {
	want := map[string]Policy{
		"login": {Max: 10, Window: 5 * time.Minute}, "admin": {Max: 600, Window: time.Minute}, "peer": {Max: 300, Window: time.Minute},
		"operation:translate": {Max: 3, Window: 10 * time.Minute}, "operation:publish": {Max: 30, Window: time.Minute},
		"maintenance:prewarm": {Max: 2, Window: time.Minute}, "maintenance:revalidate": {Max: 30, Window: time.Minute}, "prewarm-host": {Max: 1, Window: time.Minute},
	}
	got := DefaultPolicies()
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for name, policy := range want {
		if got[name] != policy {
			t.Fatalf("%s: %+v", name, got[name])
		}
	}
}

func TestOnlyCanonicalGatewayPathIsAccepted(t *testing.T) {
	_, client, cfg := setup(t)
	app := testApp(t, cfg, client)
	for _, path := range []string{Path + "/", strings.ToUpper(Path)} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body("login")))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testSecret)
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 404 {
			t.Fatalf("noncanonical path %s: %d", path, res.StatusCode)
		}
	}
}

func TestEncodedBodiesAreRejectedBeforeDecompression(t *testing.T) {
	server, client, cfg := setup(t)
	app := testApp(t, cfg, client)
	req := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(body("login")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Authorization", "Bearer "+testSecret)
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 415 {
		t.Fatalf("encoded body: %d", res.StatusCode)
	}
	if len(server.Keys()) != 0 {
		t.Fatal("encoded input reached Redis")
	}
}
