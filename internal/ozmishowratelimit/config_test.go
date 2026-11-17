package ozmishowratelimit

import (
	"github.com/gofiber/fiber/v2"
	"strings"
	"testing"
)

func TestConfigOverridesAreServerSide(t *testing.T) {
	t.Setenv("OZMISHOW_RATE_LIMIT_SERVICE_SECRET", testSecret)
	t.Setenv("OZMISHOW_RATE_LIMIT_LOGIN_MAX", "7")
	cfg, err := ConfigFromEnv()
	if err != nil || cfg.Policies["login"].Max != 7 || cfg.Policies["admin"].Max != 600 {
		t.Fatalf("%+v %v", cfg, err)
	}
	t.Setenv("OZMISHOW_RATE_LIMIT_LOGIN_MAX", "-1")
	if _, err = ConfigFromEnv(); err == nil {
		t.Fatal("invalid override accepted")
	}
}
func TestMissingConfigDisablesOnlyGateway(t *testing.T) {
	t.Setenv("OZMISHOW_RATE_LIMIT_SERVICE_SECRET", "")
	app := fiber.New()
	close := Mount(app)
	defer close()
	app.Get("/health", func(c *fiber.Ctx) error { return c.SendStatus(200) })
	if status, _ := call(t, app, body("login"), testSecret); status != 503 {
		t.Fatal(status)
	}
	// Mount adds no global middleware or normal user-auth requirements.
	if len(app.GetRoutes()) < 2 {
		t.Fatal("gateway should not replace application routes")
	}
}
func TestSharedUserCredentialCannotConfigureService(t *testing.T) {
	t.Setenv("OZMISHOW_RATE_LIMIT_SERVICE_SECRET", testSecret)
	t.Setenv("JWT_SECRET", testSecret)
	t.Setenv("OZMISHOW_RATE_LIMIT_REDIS_ADDR", "127.0.0.1:1")
	t.Setenv("OZMISHOW_RATE_LIMIT_REDIS_PASSWORD", strings.Repeat("redis-password-", 4))
	app := fiber.New()
	close := Mount(app)
	defer close()
	if status, _ := call(t, app, body("login"), testSecret); status != 503 {
		t.Fatal(status)
	}
}
