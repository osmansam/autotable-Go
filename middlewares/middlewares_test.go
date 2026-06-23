package middlewares

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/observability"
	"github.com/osmansam/autotableGo/utils"
)

func TestRequestIDUsesIncomingHeader(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())
	app.Get("/", func(c *fiber.Ctx) error {
		if got := c.Locals("requestID"); got != "incoming-request-id" {
			t.Fatalf("requestID local = %v", got)
		}
		return c.SendStatus(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "incoming-request-id")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if got := resp.Header.Get("X-Request-ID"); got != "incoming-request-id" {
		t.Fatalf("X-Request-ID = %q", got)
	}
}

func TestRequestIDGeneratesMissingHeader(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())
	app.Get("/", func(c *fiber.Ctx) error {
		if got, _ := c.Locals("requestID").(string); got == "" {
			t.Fatal("requestID local is empty")
		}
		return c.SendStatus(http.StatusNoContent)
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if got := resp.Header.Get("X-Request-ID"); got == "" {
		t.Fatal("X-Request-ID header is empty")
	}
}

func TestPrometheusMetricsEndpoint(t *testing.T) {
	observability.RecordWorkflowExecution("tenant", "project", "workflow", "schema", "success", time.Millisecond)
	observability.SetWebsocketClientsConnected(1)

	app := fiber.New()
	app.Use(PrometheusMetrics())
	app.Get("/ok", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusNoContent)
	})
	app.Get("/metrics", PrometheusHandler())

	if _, err := app.Test(httptest.NewRequest(http.MethodGet, "/ok", nil)); err != nil {
		t.Fatalf("app.Test(/ok) error = %v", err)
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if err != nil {
		t.Fatalf("app.Test(/metrics) error = %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	text := string(body)
	for _, metricName := range []string{"http_requests_total", "http_request_duration_seconds", "workflow_executions_total", "websocket_clients_connected"} {
		if !strings.Contains(text, metricName) {
			t.Fatalf("metrics response missing %q", metricName)
		}
	}
	if strings.Contains(text, `route="/metrics"`) {
		t.Fatal("metrics scrape should not be recorded as an HTTP request metric")
	}
}

func TestObservedStatusCodeUsesFiberErrorCode(t *testing.T) {
	app := fiber.New()
	app.Use(PrometheusMetrics())
	app.Get("/metrics", PrometheusHandler())

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/missing", nil))
	if err != nil {
		t.Fatalf("app.Test(/missing) error = %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	metricsResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if err != nil {
		t.Fatalf("app.Test(/metrics) error = %v", err)
	}
	body, _ := io.ReadAll(metricsResp.Body)
	if !strings.Contains(string(body), `status="404"`) {
		t.Fatal("missing route should be recorded with status 404")
	}
}

func TestFaviconIsSkippedByObservability(t *testing.T) {
	app := fiber.New()
	app.Use(PrometheusMetrics())
	app.Get("/metrics", PrometheusHandler())

	if _, err := app.Test(httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)); err != nil {
		t.Fatalf("app.Test(/favicon.ico) error = %v", err)
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if err != nil {
		t.Fatalf("app.Test(/metrics) error = %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "/favicon.ico") {
		t.Fatal("favicon requests should not be recorded as HTTP request metrics")
	}
}

func TestBodySizeLimit(t *testing.T) {
	tests := []struct {
		name       string
		maxBytes   int
		body       string
		wantStatus int
	}{
		{name: "disabled", maxBytes: 0, body: "long body", wantStatus: http.StatusNoContent},
		{name: "within limit", maxBytes: 4, body: "1234", wantStatus: http.StatusNoContent},
		{name: "over limit", maxBytes: 3, body: "1234", wantStatus: http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Post("/", BodySizeLimit("test", tt.maxBytes), func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test() error = %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusRequestEntityTooLarge && resp.Header.Get("X-Body-Limit") != "3" {
				t.Fatalf("X-Body-Limit = %q", resp.Header.Get("X-Body-Limit"))
			}
		})
	}
}

func TestRateLimitIdentity(t *testing.T) {
	app := fiber.New()
	app.Get("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
		c.Locals("tenantUserID", "user-1")
		tests := []struct {
			subject RateLimitSubject
			want    string
			wantOK  bool
		}{
			{subject: RateLimitSubjectIP, want: "ip:0.0.0.0", wantOK: true},
			{subject: RateLimitSubjectUser, want: "user:tenant:tenant:project:project:user-1", wantOK: true},
			{subject: RateLimitSubjectUserOrIP, want: "user:tenant:tenant:project:project:user-1", wantOK: true},
			{subject: "unknown", want: "ip:0.0.0.0", wantOK: true},
		}
		for _, tt := range tests {
			if got, ok := rateLimitIdentity(c, tt.subject); got != tt.want || ok != tt.wantOK {
				t.Fatalf("rateLimitIdentity(%q) = %q, %v; want %q, %v", tt.subject, got, ok, tt.want, tt.wantOK)
			}
		}
		if !hasRateLimitUser(c) {
			t.Fatal("hasRateLimitUser() = false, want true")
		}
		return c.SendStatus(http.StatusNoContent)
	})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/tenant/project", nil))
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("app.Test() = %v, %v", resp, err)
	}
}

func TestRateLimitIdentityMissingUser(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		if got, ok := rateLimitIdentity(c, RateLimitSubjectUser); got != "" || ok {
			t.Fatalf("rateLimitIdentity() = %q, %v; want empty, false", got, ok)
		}
		if got, ok := rateLimitIdentity(c, RateLimitSubjectUserOrIP); got != "ip:global:0.0.0.0" || !ok {
			t.Fatalf("rateLimitIdentity() = %q, %v", got, ok)
		}
		if hasAuthorizationHeader(c) {
			t.Fatal("hasAuthorizationHeader() = true without header")
		}
		return nil
	})
	if _, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil)); err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer token")
	app = fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		if !hasAuthorizationHeader(c) {
			t.Fatal("hasAuthorizationHeader() = false with header")
		}
		return nil
	})
	if _, err := app.Test(req); err != nil {
		t.Fatalf("app.Test(with header) error = %v", err)
	}
}

func TestSanitizeRedisKeyPart(t *testing.T) {
	if got := sanitizeRedisKeyPart(" a\nb\rc\td "); got != "_a_b_c_d_" {
		t.Fatalf("sanitizeRedisKeyPart() = %q", got)
	}
}

func TestRateLimitSkipsWhenRedisIsNotInitialized(t *testing.T) {
	app := fiber.New()
	app.Get("/", RateLimit(RateLimitPolicy{Name: "test", Limit: 1}), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusNoContent)
	})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("app.Test() = %v, %v", resp, err)
	}
}

func TestRateLimitWithRedis(t *testing.T) {
	server := miniredis.RunT(t)
	oldClient := configs.RedisClient
	configs.RedisClient = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = configs.RedisClient.Close()
		configs.RedisClient = oldClient
	})

	app := fiber.New()
	app.Get("/", RateLimit(RateLimitPolicy{Name: "test", Limit: 1, Window: time.Minute, Subject: RateLimitSubjectIP}), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusNoContent)
	})
	first, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil || first.StatusCode != http.StatusNoContent || first.Header.Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("first response = %#v, error = %v", first, err)
	}
	second, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil || second.StatusCode != http.StatusTooManyRequests || second.Header.Get("Retry-After") == "" {
		t.Fatalf("second response = %#v, error = %v", second, err)
	}
}

func TestTenantAuthenticateRejectsMissingOrInvalidToken(t *testing.T) {
	app := fiber.New()
	app.Get("/", TenantAuthenticate, func(c *fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
	for _, auth := range []string{"", "invalid"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := app.Test(req)
		if err != nil || resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("TenantAuthenticate(%q) status = %v, error = %v", auth, resp, err)
		}
	}
}

func TestTenantAuthorizationMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		locals     map[string]interface{}
		middleware fiber.Handler
		wantStatus int
	}{
		{name: "authorize allows matching role", locals: map[string]interface{}{"roles": []string{models.ProjectRoleAdmin}}, middleware: TenantAuthorize([]string{models.ProjectRoleAdmin}), wantStatus: http.StatusNoContent},
		{name: "authorize temporarily allows missing roles", middleware: TenantAuthorize([]string{models.ProjectRoleAdmin}), wantStatus: http.StatusNoContent},
		{name: "project scope allows project", locals: map[string]interface{}{"roleScope": string(models.RoleScopeProject), "projectID": "project"}, middleware: RequireProjectScope, wantStatus: http.StatusNoContent},
		{name: "project scope rejects tenant", locals: map[string]interface{}{"roleScope": string(models.RoleScopeTenant)}, middleware: RequireProjectScope, wantStatus: http.StatusForbidden},
		{name: "tenant scope allows tenant", locals: map[string]interface{}{"roleScope": string(models.RoleScopeTenant)}, middleware: RequireTenantScope, wantStatus: http.StatusNoContent},
		{name: "owner allows owner", locals: map[string]interface{}{"roles": []string{models.TenantRoleOwner}}, middleware: TenantOwnerOnly, wantStatus: http.StatusNoContent},
		{name: "admin allows admin", locals: map[string]interface{}{"roles": []string{models.ProjectRoleAdmin}}, middleware: ProjectAdminOnly, wantStatus: http.StatusNoContent},
		{name: "admin rejects viewer", locals: map[string]interface{}{"roles": []string{models.ProjectRoleViewer}}, middleware: ProjectAdminOnly, wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/", func(c *fiber.Ctx) error {
				for key, value := range tt.locals {
					c.Locals(key, value)
				}
				return c.Next()
			}, tt.middleware, func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusNoContent)
			})
			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
			if err != nil || resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %v, error = %v; want %d", resp, err, tt.wantStatus)
			}
		})
	}
}

func TestAuthenticateRejectsMissingToken(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error { return Authenticate(c, false, nil, true) })
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %v, error = %v", resp, err)
	}
}

func TestAuthenticate(t *testing.T) {
	tokens, err := utils.GenerateTokens("user", "admin", "tenant", "project", "", "")
	if err != nil {
		t.Fatalf("GenerateTokens() error = %v", err)
	}
	tests := []struct {
		name       string
		token      string
		tenant     string
		project    string
		active     bool
		authorized bool
		roles      []string
		wantStatus int
	}{
		{name: "valid", token: tokens.AccessToken, active: true, wantStatus: http.StatusNoContent},
		{name: "invalid token", token: "invalid", active: true, wantStatus: http.StatusUnauthorized},
		{name: "tenant mismatch", token: tokens.AccessToken, tenant: "other", active: true, wantStatus: http.StatusForbidden},
		{name: "project mismatch", token: tokens.AccessToken, project: "other", active: true, wantStatus: http.StatusForbidden},
		{name: "inactive", token: tokens.AccessToken, wantStatus: http.StatusForbidden},
		{name: "authorized role", token: tokens.AccessToken, active: true, authorized: true, roles: []string{"admin"}, wantStatus: http.StatusNoContent},
		{name: "unauthorized role", token: tokens.AccessToken, active: true, authorized: true, roles: []string{"viewer"}, wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/", func(c *fiber.Ctx) error {
				c.Locals("expectedTenantID", tt.tenant)
				c.Locals("expectedProjectID", tt.project)
				return Authenticate(c, tt.authorized, tt.roles, tt.active)
			}, func(c *fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)
			resp, err := app.Test(req)
			if err != nil || resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %v, error = %v; want %d", resp, err, tt.wantStatus)
			}
		})
	}
}

func TestConditionalAuthenticationForPages(t *testing.T) {
	app := fiber.New()
	app.Get("/", ConditionalAuthenticationForPages, func(c *fiber.Ctx) error {
		if c.Locals("expectedTenantID") != "tenant" || c.Locals("expectedProjectID") != "project" {
			t.Fatalf("expected scope locals = %#v, %#v", c.Locals("expectedTenantID"), c.Locals("expectedProjectID"))
		}
		if c.Locals("userID") != nil {
			t.Fatalf("userID = %#v, want anonymous request", c.Locals("userID"))
		}
		return c.SendStatus(http.StatusNoContent)
	})
	for _, auth := range []string{"", "invalid"} {
		req := httptest.NewRequest(http.MethodGet, "/?tenantID=tenant&projectID=project", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := app.Test(req)
		if err != nil || resp.StatusCode != http.StatusNoContent {
			t.Fatalf("response = %#v, error = %v", resp, err)
		}
	}
}

func TestConditionalAuthenticationShortCircuits(t *testing.T) {
	app := fiber.New()
	app.Get("/", ConditionalAuthentication("GetAllDynamicModelItems"), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusNoContent)
	})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/?tenantID=tenant&projectID=project", nil))
	if err != nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("response = %#v, error = %v", resp, err)
	}
}

func TestMiddlewareConstructors(t *testing.T) {
	handlers := []fiber.Handler{
		DefaultBodySizeLimit(),
		BulkWriteBodySizeLimit(),
		BulkUpdateBodySizeLimit(),
		BulkDeleteBodySizeLimit(),
		ExportBodySizeLimit(),
		UploadBodySizeLimit(),
		GeneralRateLimit(),
		PublicRateLimit(),
		AuthRateLimit(),
		SearchRateLimit(),
		WriteRateLimit(),
		BulkRateLimit(),
		ExportRateLimit(),
		UploadRateLimit(),
		ExecuteRateLimit(),
	}
	for i, handler := range handlers {
		if handler == nil {
			t.Fatalf("handler %d = nil", i)
		}
	}
}
