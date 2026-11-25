package main

import (
	"github.com/gofiber/fiber/v2"
	"net/http/httptest"
	"testing"
)

const diagnosticsTestKey = "3eb8c1704d93fa5670286de591be7c06d2a84ef9137b60c5489d25ae36f107bc"

func TestDiagnosticsRequireDedicatedBearerToken(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	t.Setenv("ENABLE_PPROF", "true")
	for _, token := range []string{"", diagnosticsTestKey} {
		t.Run(token[:min(4, len(token))], func(t *testing.T) {
			t.Setenv("OBSERVABILITY_TOKEN", token)
			app := fiber.New()
			configureMetrics(app)
			configurePprof(app, ":3000")
			for _, path := range []string{"/metrics", "/METRICS/", "/debug/pprof/", "/debug/pprof/cmdline", "/debug/pprof/goroutine?debug=1"} {
				for _, auth := range []string{"", "Bearer wrong", "Bearer " + diagnosticsTestKey} {
					req := httptest.NewRequest("GET", path, nil)
					req.Header.Set("Authorization", auth)
					req.Header.Set("X-Forwarded-For", "127.0.0.1")
					req.Header.Set("X-Real-IP", "127.0.0.1")
					resp, err := app.Test(req)
					if err != nil {
						t.Fatal(err)
					}
					resp.Body.Close()
					want := 401
					if token == "" {
						want = 404
					} else if auth == "Bearer "+diagnosticsTestKey {
						want = 200
					}
					if resp.StatusCode != want {
						t.Fatalf("%s configured=%v authorized=%v status=%d want=%d", path, token != "", auth == "Bearer "+diagnosticsTestKey, resp.StatusCode, want)
					}
				}
			}
		})
	}
	t.Setenv("NODE_ENV", "production")
	t.Setenv("OBSERVABILITY_TOKEN", diagnosticsTestKey)
	app := fiber.New()
	configurePprof(app, ":3000")
	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	req.Header.Set("Authorization", "Bearer "+diagnosticsTestKey)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal("production profiling enabled")
	}
}
