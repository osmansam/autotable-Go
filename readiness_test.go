package main

import (
	"context"
	"errors"
	"github.com/gofiber/fiber/v2"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadinessReflectsDependencyFailures(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
	}{{"healthy", nil, 200}, {"database down", errors.New("private connection details"), 503}} {
		t.Run(test.name, func(t *testing.T) {
			app := fiber.New()
			calls := 0
			app.Get("/ready", readinessHandler(func(ctx context.Context) error {
				calls++
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("unbounded readiness probe")
				}
				return test.err
			}))
			response, err := app.Test(httptest.NewRequest("GET", "/ready", nil))
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != test.status || calls != 1 {
				t.Fatalf("status=%d calls=%d body=%s", response.StatusCode, calls, body)
			}
			if strings.Contains(string(body), "private connection details") {
				t.Fatal("readiness leaks connection details")
			}
		})
	}
}
