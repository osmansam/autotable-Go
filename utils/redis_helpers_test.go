package utils

import (
	"context"
	"errors"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/responses"
)

func TestSchemaCacheVersionsAndSetCache(t *testing.T) {
	setupMiniRedis(t)
	ctx := context.Background()
	if got, err := GetSchemaCacheVersion(ctx, "tenant", "project", "orders"); err != nil || got != 1 {
		t.Fatalf("GetSchemaCacheVersion() = %d, %v", got, err)
	}
	if err := IncrementSchemaCacheVersion(ctx, "tenant", "project", "orders"); err != nil {
		t.Fatalf("IncrementSchemaCacheVersion() error = %v", err)
	}
	if got, err := GetSchemaCacheVersion(ctx, "tenant", "project", "orders"); err != nil || got != 2 {
		t.Fatalf("GetSchemaCacheVersion() = %d, %v", got, err)
	}
	if err := InvalidateSchemaAndTriggeredCaches(ctx, "tenant", "project", "orders", []string{"", "orders", "customers"}); err != nil {
		t.Fatalf("InvalidateSchemaAndTriggeredCaches() error = %v", err)
	}
	if err := SetCache(ctx, "cache", map[string]interface{}{"id": 1}, time.Minute); err != nil {
		t.Fatalf("SetCache() error = %v", err)
	}
	if _, err := configs.RedisClient.Get(ctx, "cache").Result(); err != nil {
		t.Fatalf("cached value error = %v", err)
	}
	if err := SetCache(ctx, "invalid", func() {}, time.Minute); err == nil {
		t.Fatal("SetCache(unmarshalable) error = nil")
	}
}

func TestDefaultCacheTTLDuration(t *testing.T) {
	if got := DefaultCacheTTLDuration(); got <= 0 {
		t.Fatalf("DefaultCacheTTLDuration() = %s", got)
	}
}

func TestGetTenantAndProjectFromSlugsCacheHit(t *testing.T) {
	setupMiniRedis(t)
	ctx := context.Background()
	if err := configs.RedisClient.Set(ctx, "slug_mapping:tenant:project", "tenant-id|project-id", time.Minute).Err(); err != nil {
		t.Fatalf("Redis Set() error = %v", err)
	}

	app := fiber.New()
	app.Get("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
		tenantID, projectID, err := GetTenantAndProjectFromSlugs(c)
		if err != nil || tenantID != "tenant-id" || projectID != "project-id" {
			t.Fatalf("GetTenantAndProjectFromSlugs() = %q, %q, %v", tenantID, projectID, err)
		}
		return nil
	})
	if _, err := app.Test(httptest.NewRequest("GET", "/tenant/project", nil)); err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
}

func TestCacheFillLockHelpers(t *testing.T) {
	setupMiniRedis(t)
	ctx := context.Background()
	lockID, locked := AcquireCacheFillLock(ctx, "cache")
	if !locked || lockID == "" {
		t.Fatalf("AcquireCacheFillLock() = %q, %v", lockID, locked)
	}
	if _, locked := AcquireCacheFillLock(ctx, "cache"); locked {
		t.Fatal("second AcquireCacheFillLock() locked = true")
	}
	ReleaseCacheFillLock(ctx, "cache", "wrong")
	if _, locked := AcquireCacheFillLock(ctx, "cache"); locked {
		t.Fatal("wrong release removed lock")
	}
	ReleaseCacheFillLock(ctx, "cache", lockID)
	if _, locked := AcquireCacheFillLock(ctx, "cache"); !locked {
		t.Fatal("correct release did not remove lock")
	}
	if !WaitForCacheFill(ctx, func() bool { return true }) {
		t.Fatal("WaitForCacheFill() = false")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if WaitForCacheFill(canceled, func() bool { return false }) {
		t.Fatal("WaitForCacheFill(canceled) = true")
	}
}

func TestIdempotencyLifecycle(t *testing.T) {
	setupMiniRedis(t)
	ctx := context.Background()
	key := "request"
	hash := "hash"
	begin, err := BeginIdempotentRequest(ctx, key, hash)
	if err != nil || begin.Status != IdempotencyOwned {
		t.Fatalf("BeginIdempotentRequest() = %#v, %v", begin, err)
	}
	begin, err = BeginIdempotentRequest(ctx, key, hash)
	if err != nil || begin.Status != IdempotencyProcessing {
		t.Fatalf("second BeginIdempotentRequest() = %#v, %v", begin, err)
	}
	if _, err := BeginIdempotentRequest(ctx, key, "other"); !errors.Is(err, ErrIdempotencyRequestMismatch) {
		t.Fatalf("mismatched BeginIdempotentRequest() error = %v", err)
	}
	result := IdempotencyResult{Status: 201, RequestHash: hash, Body: responses.GeneralResponse{Status: 201, Message: "created"}}
	if err := StoreIdempotentResult(ctx, key, result); err != nil {
		t.Fatalf("StoreIdempotentResult() error = %v", err)
	}
	got, err := GetIdempotentResult(ctx, key, hash)
	if err != nil || !reflect.DeepEqual(got, &result) {
		t.Fatalf("GetIdempotentResult() = %#v, %v", got, err)
	}
	got, err = WaitForIdempotentResult(ctx, key, hash)
	if err != nil || !reflect.DeepEqual(got, &result) {
		t.Fatalf("WaitForIdempotentResult() = %#v, %v", got, err)
	}
}

func setupMiniRedis(t *testing.T) {
	t.Helper()
	server := miniredis.RunT(t)
	oldClient := configs.RedisClient
	configs.RedisClient = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = configs.RedisClient.Close()
		configs.RedisClient = oldClient
	})
}
