package utils

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestProjectContextResolvesOncePerRequest(t *testing.T) {
	resolvers := []struct {
		name string
		run  func(*fiber.Ctx) (string, string, error)
	}{
		{"context", GetTenantAndProjectContext},
		{"slugs", GetTenantAndProjectFromSlugs},
	}
	for _, first := range resolvers {
		for _, second := range resolvers {
			t.Run(first.name+" then "+second.name, func(t *testing.T) {
				setupMiniRedis(t)
				seedRequestProject(t, "acme", "store", "tenant", "project")
				checks := 0
				restore := OverrideProjectActiveChecker(func(context.Context, string, string) (bool, error) {
					checks++
					return true, nil
				})
				defer restore()
				app := fiber.New()
				app.Get("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
					for _, resolve := range []func(*fiber.Ctx) (string, string, error){first.run, second.run} {
						tenant, project, err := resolve(c)
						if err != nil || tenant != "tenant" || project != "project" {
							t.Errorf("resolved = %q, %q, %v", tenant, project, err)
							return c.SendStatus(http.StatusInternalServerError)
						}
					}
					return c.SendStatus(http.StatusOK)
				})
				assertProjectRequestStatus(t, app, "/acme/store", http.StatusOK)
				if checks != 1 {
					t.Errorf("active-project database checks = %d, want one per request", checks)
				}
			})
		}
	}
}

func TestProjectContextRechecksAuthenticationAfterResolution(t *testing.T) {
	for _, field := range []string{"tenantID", "projectID"} {
		t.Run(field, func(t *testing.T) {
			setupMiniRedis(t)
			seedRequestProject(t, "acme", "store", "tenant", "project")
			app := fiber.New()
			app.Get("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
				if _, _, err := GetTenantAndProjectContext(c); err != nil {
					return err
				}
				// A later authentication middleware supplies a different token scope.
				c.Locals(field, "other")
				if _, _, err := GetTenantAndProjectContext(c); !errors.Is(err, ErrProjectMismatch) {
					t.Errorf("scope mismatch error = %v", err)
					return c.SendStatus(http.StatusInternalServerError)
				}
				if c.Locals(field) != "other" {
					t.Error("rejected URL context replaced authentication locals")
				}
				return c.SendStatus(http.StatusForbidden)
			})
			assertProjectRequestStatus(t, app, "/acme/store", http.StatusForbidden)
		})
	}
}

func TestProjectContextDoesNotReuseActivityAcrossRequests(t *testing.T) {
	setupMiniRedis(t)
	seedRequestProject(t, "acme", "store", "tenant", "project")
	checks := 0
	restore := OverrideProjectActiveChecker(func(context.Context, string, string) (bool, error) {
		checks++
		return checks == 1, nil
	})
	defer restore()
	app := fiber.New()
	app.Get("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
		if _, _, err := GetTenantAndProjectContext(c); errors.Is(err, ErrProjectNotFound) {
			return c.SendStatus(http.StatusNotFound)
		} else if err != nil {
			return err
		}
		return c.SendStatus(http.StatusOK)
	})
	assertProjectRequestStatus(t, app, "/acme/store", http.StatusOK)
	assertProjectRequestStatus(t, app, "/acme/store", http.StatusNotFound)
	if checks != 2 {
		t.Errorf("active checks = %d, want a fresh check for each request", checks)
	}
}

func TestProjectContextRetriesFailedResolution(t *testing.T) {
	setupMiniRedis(t)
	seedRequestProject(t, "acme", "store", "tenant", "project")
	checks := 0
	restore := OverrideProjectActiveChecker(func(context.Context, string, string) (bool, error) {
		checks++
		if checks == 1 {
			return false, errors.New("temporary database failure")
		}
		return true, nil
	})
	defer restore()
	app := fiber.New()
	app.Get("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
		if _, _, err := GetTenantAndProjectContext(c); err == nil {
			t.Error("first lookup should fail")
		}
		tenant, project, err := GetTenantAndProjectContext(c)
		if err != nil || tenant != "tenant" || project != "project" {
			t.Errorf("retry = %q, %q, %v", tenant, project, err)
			return c.SendStatus(http.StatusInternalServerError)
		}
		return c.SendStatus(http.StatusOK)
	})
	assertProjectRequestStatus(t, app, "/acme/store", http.StatusOK)
	if checks != 2 {
		t.Errorf("active checks = %d, want two attempts", checks)
	}
}

func TestProjectContextResolvesChangedURLSlugs(t *testing.T) {
	for _, path := range []string{"/other/store", "/acme/other"} {
		t.Run(path, func(t *testing.T) { testProjectContextReroute(t, path) })
	}
}

func testProjectContextReroute(t *testing.T, path string) {
	setupMiniRedis(t)
	seedRequestProject(t, "acme", "store", "tenant", "project")
	seedRequestProject(t, "other", "store", "other-tenant", "other-project")
	seedRequestProject(t, "acme", "other", "tenant", "other-project")
	checks := 0
	restore := OverrideProjectActiveChecker(func(context.Context, string, string) (bool, error) { checks++; return true, nil })
	defer restore()
	app := fiber.New()
	app.Get("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
		if c.Locals("rerouted") == nil {
			if _, _, err := GetTenantAndProjectContext(c); err != nil {
				return err
			}
			c.Locals("rerouted", true)
			c.Path(path)
			return c.RestartRouting()
		}
		// Keep the first authenticated scope; the changed URL must be rejected.
		if _, _, err := GetTenantAndProjectContext(c); !errors.Is(err, ErrProjectMismatch) {
			t.Errorf("rerouted scope mismatch error = %v", err)
			return c.SendStatus(http.StatusInternalServerError)
		}
		return c.SendStatus(http.StatusForbidden)
	})
	assertProjectRequestStatus(t, app, "/acme/store", http.StatusForbidden)
	if checks != 2 {
		t.Errorf("active checks = %d, want a lookup for each URL", checks)
	}
}

func TestProjectContextDoesNotTrustUnverifiedLocals(t *testing.T) {
	setupMiniRedis(t)
	seedRequestProject(t, "acme", "store", "tenant", "project")
	checks := 0
	restore := OverrideProjectActiveChecker(func(context.Context, string, string) (bool, error) { checks++; return false, nil })
	defer restore()
	app := fiber.New()
	app.Get("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
		c.Locals("tenantID", "tenant")
		c.Locals("projectID", "project")
		c.Locals("tenantSlug", "acme")
		c.Locals("projectSlug", "store")
		if _, _, err := GetTenantAndProjectContext(c); !errors.Is(err, ErrProjectNotFound) {
			t.Errorf("unverified inactive project error = %v", err)
			return c.SendStatus(http.StatusInternalServerError)
		}
		return c.SendStatus(http.StatusNotFound)
	})
	assertProjectRequestStatus(t, app, "/acme/store", http.StatusNotFound)
	if checks != 1 {
		t.Errorf("active checks = %d, want a live check despite matching locals", checks)
	}
}

func TestProjectContextReusesColdSlugLookup(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()
	mt.Run("one database lookup across both entry points", func(mt *mtest.T) {
		setupMiniRedis(mt.T)
		previous := globalCollectionProvider
		globalCollectionProvider = func(string) *mongo.Collection { return mt.Coll }
		defer func() { globalCollectionProvider = previous }()
		tenantID, projectID := primitive.NewObjectID(), primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{
			{Key: "_id", Value: projectID}, {Key: "tenantId", Value: tenantID}, {Key: "isActive", Value: true},
		}))
		checks := 0
		restore := OverrideProjectActiveChecker(func(context.Context, string, string) (bool, error) { checks++; return true, nil })
		defer restore()
		app := fiber.New()
		app.Get("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
			for _, resolve := range []func(*fiber.Ctx) (string, string, error){GetTenantAndProjectFromSlugs, GetTenantAndProjectContext} {
				tenant, project, err := resolve(c)
				if err != nil || tenant != tenantID.Hex() || project != projectID.Hex() {
					mt.Errorf("cold lookup = %q, %q, %v", tenant, project, err)
					return c.SendStatus(http.StatusInternalServerError)
				}
			}
			return c.SendStatus(http.StatusOK)
		})
		assertProjectRequestStatus(mt.T, app, "/acme/store", http.StatusOK)
		if events := mt.GetAllStartedEvents(); len(events) != 1 || events[0].CommandName != "find" {
			mt.Errorf("database commands = %d, want one find", len(events))
		}
		if checks != 0 {
			mt.Errorf("additional active checks = %d, want none after cold lookup", checks)
		}
	})
}

func BenchmarkProjectContextMiddlewareAndHandler(b *testing.B) {
	server := miniredis.RunT(b)
	previous := configs.RedisClient
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	configs.RedisClient = client
	server.Set("slug_mapping:acme:store", "tenant|project")
	checks := 0
	restore := OverrideProjectActiveChecker(func(context.Context, string, string) (bool, error) {
		checks++
		time.Sleep(3 * time.Millisecond) // Simulate the same database latency in both versions.
		return true, nil
	})
	b.Cleanup(func() { restore(); _ = client.Close(); configs.RedisClient = previous })
	app := fiber.New()
	resolve := func(c *fiber.Ctx) error {
		if _, _, err := GetTenantAndProjectContext(c); err != nil {
			return err
		}
		return c.Next()
	}
	app.Get("/:tenantSlug/:projectSlug", resolve, func(c *fiber.Ctx) error {
		if _, _, err := GetTenantAndProjectContext(c); err != nil {
			return err
		}
		return c.SendStatus(http.StatusOK)
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, "/acme/store", nil))
		if err != nil {
			b.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			b.Fatalf("status = %d", response.StatusCode)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(checks)/float64(b.N), "project-lookups/op")
}

func seedRequestProject(t *testing.T, tenantSlug, projectSlug, tenantID, projectID string) {
	t.Helper()
	if err := configs.RedisClient.Set(context.Background(), "slug_mapping:"+tenantSlug+":"+projectSlug, tenantID+"|"+projectID, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
}

func assertProjectRequestStatus(t *testing.T, app *fiber.App, path string, status int) {
	t.Helper()
	response, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		t.Fatalf("request %s status = %d, want %d", path, response.StatusCode, status)
	}
}
