package ozmishowratelimit

import (
	"errors"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
)

type Config struct {
	Secret, PreviousSecret, Namespace string
	Policies                          map[string]Policy
	MaxConcurrent                     int
}

var namespacePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func validSecret(value string) bool {
	return len(value) >= 32 && len(value) <= 512 && !strings.ContainsAny(value, "\r\n\t ")
}
func (c Config) validate() error {
	if !validSecret(c.Secret) {
		return errors.New("configure a dedicated service secret of at least 32 characters")
	}
	if c.PreviousSecret != "" && !validSecret(c.PreviousSecret) {
		return errors.New("invalid previous service secret")
	}
	if !namespacePattern.MatchString(c.Namespace) {
		return errors.New("invalid server namespace")
	}
	if c.MaxConcurrent < 1 || c.MaxConcurrent > 128 {
		return errors.New("invalid concurrency limit")
	}
	defaults := DefaultPolicies()
	if len(c.Policies) != len(defaults) {
		return errors.New("invalid server policies")
	}
	for name, policy := range c.Policies {
		original, ok := defaults[name]
		if !ok || original.Window != policy.Window || policy.Max < 1 || policy.Max > 1_000_000 {
			return errors.New("invalid server policy")
		}
	}
	return nil
}

func ConfigFromEnv() (Config, error) {
	c := Config{Secret: os.Getenv("OZMISHOW_RATE_LIMIT_SERVICE_SECRET"), PreviousSecret: os.Getenv("OZMISHOW_RATE_LIMIT_PREVIOUS_SECRET"), Namespace: strings.TrimSpace(os.Getenv("OZMISHOW_RATE_LIMIT_NAMESPACE")), Policies: DefaultPolicies(), MaxConcurrent: 16}
	if c.Namespace == "" {
		c.Namespace = "production"
	}
	for name, env := range map[string]string{"login": "OZMISHOW_RATE_LIMIT_LOGIN_MAX", "admin": "OZMISHOW_RATE_LIMIT_ADMIN_MAX", "peer": "OZMISHOW_RATE_LIMIT_PEER_MAX"} {
		if value := os.Getenv(env); value != "" {
			max, err := strconv.Atoi(value)
			if err != nil {
				return c, errors.New("invalid server policy override")
			}
			policy := c.Policies[name]
			policy.Max = max
			c.Policies[name] = policy
		}
	}
	return c, c.validate()
}

// Mount owns a dedicated client. Missing configuration disables only this route.
// It never uses AutoTable's shared Redis client, user authentication, or circuit breaker.
func Mount(app *fiber.App) func() {
	config, err := ConfigFromEnv()
	addr := strings.TrimSpace(os.Getenv("OZMISHOW_RATE_LIMIT_REDIS_ADDR"))
	password := os.Getenv("OZMISHOW_RATE_LIMIT_REDIS_PASSWORD")
	if err == nil && (addr == "" || !validSecret(password) || password == config.Secret || password == config.PreviousSecret) {
		err = errors.New("configure a dedicated Redis address and separate strong Redis password")
	}
	for _, name := range []string{"JWT_SECRET", "TENANT_JWT_SECRET", "CONTAINER_JWT_SECRET", "EXTERNAL_API_CREDENTIAL_KEY"} {
		if value := os.Getenv(name); value != "" && (config.Secret == value || config.PreviousSecret == value) {
			err = errors.New("service credentials must be separate from AutoTable credentials")
		}
	}
	if err != nil {
		log.Printf("Ozmishow rate-limit endpoint disabled: %v", err)
		app.All(Path, func(c *fiber.Ctx) error {
			if c.Path() != Path {
				return failure(c, 404, "Not found")
			}
			return failure(c, 503, "Rate limit service unavailable")
		})
		return func() {}
	}
	client := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: 0, PoolSize: 8, MinIdleConns: 0, PoolTimeout: 250 * time.Millisecond, DialTimeout: 500 * time.Millisecond, ReadTimeout: 750 * time.Millisecond, WriteTimeout: 750 * time.Millisecond, MaxRetries: -1, IdleTimeout: time.Minute})
	handler, _ := NewHandler(config, client) // configuration validated above
	app.All(Path, handler.Handle)
	return func() { _ = client.Close() }
}
