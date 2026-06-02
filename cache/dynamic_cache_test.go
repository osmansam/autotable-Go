package cache

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
)

func TestNewDynamicCache(t *testing.T) {
	if NewDynamicCache() == nil {
		t.Fatal("NewDynamicCache() = nil")
	}
}

func TestCacheTTL(t *testing.T) {
	if got := cacheTTL(2); got != 2*time.Minute {
		t.Fatalf("cacheTTL(2) = %s", got)
	}
	if got := cacheTTL(0); got != utils.DefaultCacheTTLDuration() {
		t.Fatalf("cacheTTL(0) = %s", got)
	}
}

func TestInvalidateWriteCachesNilContainer(t *testing.T) {
	cache := NewDynamicCache()
	if err := cache.invalidateWriteCaches(context.Background(), "tenant", "project", "orders", nil); err != nil {
		t.Fatalf("invalidateWriteCaches() error = %v", err)
	}
	if err := cache.InvalidateCreateCaches(context.Background(), "tenant", "project", "orders", nil); err != nil {
		t.Fatalf("InvalidateCreateCaches() error = %v", err)
	}
}

func TestDynamicCacheRedisOperations(t *testing.T) {
	setupRedis(t)
	ctx := context.Background()
	cache := NewDynamicCache()

	items := []map[string]interface{}{{"id": "1"}}
	cache.SetItems(ctx, "items", items, 1)
	if got, ok := cache.GetItems(ctx, "items"); !ok || !reflect.DeepEqual(got, items) {
		t.Fatalf("GetItems() = %#v, %v", got, ok)
	}
	cache.SetItem(ctx, "item", items[0], 1)
	if got, ok := cache.GetItem(ctx, "item"); !ok || !reflect.DeepEqual(got, items[0]) {
		t.Fatalf("GetItem() = %#v, %v", got, ok)
	}
	cache.SetResponse(ctx, "response", fiber.Map{"status": "ok"}, 1)
	if got, ok := cache.GetResponse(ctx, "response"); !ok || got["status"] != "ok" {
		t.Fatalf("GetResponse() = %#v, %v", got, ok)
	}
	cache.SetValue(ctx, "value", map[string]interface{}{"count": 2}, time.Minute)
	if got, ok := cache.GetValue(ctx, "value"); !ok || !reflect.DeepEqual(got, map[string]interface{}{"count": float64(2)}) {
		t.Fatalf("GetValue() = %#v, %v", got, ok)
	}
}

func TestPipelineCacheAndInvalidation(t *testing.T) {
	setupRedis(t)
	ctx := context.Background()
	cache := NewDynamicCache()
	items := []map[string]interface{}{{"id": "1"}}
	cache.SetPipelineItems(ctx, "pipeline", "query=1", items, 1)
	if got, ok := cache.GetPipelineItems(ctx, "pipeline", "query=1"); !ok || !reflect.DeepEqual(got, items) {
		t.Fatalf("GetPipelineItems() = %#v, %v", got, ok)
	}
	if _, ok := cache.GetPipelineItems(ctx, "pipeline", "query=2"); ok {
		t.Fatal("GetPipelineItems(mismatched query) ok = true")
	}

	container := &models.ContainerModel{Redis: models.Redis{IsRedisCached: true, TriggeredRedisCaches: []string{"customers"}}}
	for _, schema := range []string{"orders", "customers"} {
		if got, err := utils.GetSchemaCacheVersion(ctx, "tenant", "project", schema); err != nil || got != 1 {
			t.Fatalf("initial schema %q version = %d, %v; want 1", schema, got, err)
		}
	}
	var triggered []string
	if err := cache.InvalidateUpdateCaches(ctx, "tenant", "project", "orders", container, func(schema string) {
		triggered = append(triggered, schema)
	}); err != nil {
		t.Fatalf("InvalidateUpdateCaches() error = %v", err)
	}
	if !reflect.DeepEqual(triggered, []string{"customers"}) {
		t.Fatalf("triggered = %#v", triggered)
	}
	for _, schema := range []string{"orders", "customers"} {
		versionKey := utils.BuildSchemaCacheVersionKey("tenant", "project", schema)
		if got, err := configs.RedisClient.Get(ctx, versionKey).Int64(); err != nil || got != 2 {
			t.Fatalf("schema %q version = %d, %v; want 2", schema, got, err)
		}
	}
}

func TestDynamicCacheMissesAndMalformedPayloads(t *testing.T) {
	setupRedis(t)
	ctx := context.Background()
	cache := NewDynamicCache()

	if _, ok := cache.GetItems(ctx, "missing"); ok {
		t.Fatal("GetItems(missing) ok = true")
	}
	if _, ok := cache.GetItem(ctx, "missing"); ok {
		t.Fatal("GetItem(missing) ok = true")
	}
	if _, ok := cache.GetResponse(ctx, "missing"); ok {
		t.Fatal("GetResponse(missing) ok = true")
	}
	if _, ok := cache.GetValue(ctx, "missing"); ok {
		t.Fatal("GetValue(missing) ok = true")
	}
	if _, ok := cache.GetPipelineItems(ctx, "missing", "query=1"); ok {
		t.Fatal("GetPipelineItems(missing) ok = true")
	}
	if err := configs.RedisClient.Set(ctx, "malformed", "{", time.Minute).Err(); err != nil {
		t.Fatalf("Set(malformed) error = %v", err)
	}
	if _, ok := cache.GetItems(ctx, "malformed"); ok {
		t.Fatal("GetItems(malformed) ok = true")
	}
	if _, ok := cache.GetItem(ctx, "malformed"); ok {
		t.Fatal("GetItem(malformed) ok = true")
	}
	if _, ok := cache.GetResponse(ctx, "malformed"); ok {
		t.Fatal("GetResponse(malformed) ok = true")
	}
	if _, ok := cache.GetValue(ctx, "malformed"); ok {
		t.Fatal("GetValue(malformed) ok = true")
	}
	if err := configs.RedisClient.Set(ctx, "malformed-pipeline", "{", time.Minute).Err(); err != nil {
		t.Fatalf("Set(malformed pipeline) error = %v", err)
	}
	if err := configs.RedisClient.Set(ctx, "malformed-pipeline-query", "query=1", time.Minute).Err(); err != nil {
		t.Fatalf("Set(malformed pipeline query) error = %v", err)
	}
	if _, ok := cache.GetPipelineItems(ctx, "malformed-pipeline", "query=1"); ok {
		t.Fatal("GetPipelineItems(malformed) ok = true")
	}
}

func TestDynamicCacheExpiration(t *testing.T) {
	server := setupRedis(t)
	ctx := context.Background()
	cache := NewDynamicCache()

	cache.SetItems(ctx, "items", []map[string]interface{}{{"id": "1"}}, 1)
	if _, ok := cache.GetItems(ctx, "items"); !ok {
		t.Fatal("GetItems() ok = false before expiration")
	}

	server.FastForward(time.Minute + time.Second)
	if _, ok := cache.GetItems(ctx, "items"); ok {
		t.Fatal("GetItems() ok = true after expiration")
	}
}

func TestDynamicCacheInvalidationMakesPreviousVersionUnreachable(t *testing.T) {
	setupRedis(t)
	ctx := context.Background()
	cache := NewDynamicCache()

	version, err := utils.GetSchemaCacheVersion(ctx, "tenant", "project", "orders")
	if err != nil || version != 1 {
		t.Fatalf("GetSchemaCacheVersion() = %d, %v; want 1", version, err)
	}
	oldKey := utils.BuildVersionedCacheKey("tenant", "project", "orders", version, "GetAllDynamicModelItems", utils.HashCacheQuery("query=1"))
	cache.SetItems(ctx, oldKey, []map[string]interface{}{{"id": "1"}}, 1)
	if _, ok := cache.GetItems(ctx, oldKey); !ok {
		t.Fatal("GetItems(old version) ok = false before invalidation")
	}

	if err := cache.InvalidateCreateCaches(ctx, "tenant", "project", "orders", &models.ContainerModel{}); err != nil {
		t.Fatalf("InvalidateCreateCaches() error = %v", err)
	}
	version, err = utils.GetSchemaCacheVersion(ctx, "tenant", "project", "orders")
	if err != nil || version != 2 {
		t.Fatalf("GetSchemaCacheVersion() = %d, %v; want 2", version, err)
	}
	currentKey := utils.BuildVersionedCacheKey("tenant", "project", "orders", version, "GetAllDynamicModelItems", utils.HashCacheQuery("query=1"))
	if _, ok := cache.GetItems(ctx, currentKey); ok {
		t.Fatal("GetItems(current version) returned stale version-1 data")
	}
	if _, ok := cache.GetItems(ctx, oldKey); !ok {
		t.Fatal("GetItems(old version) ok = false; expected physical key to remain isolated until TTL expiration")
	}
}

func setupRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server := miniredis.RunT(t)
	oldClient := configs.RedisClient
	configs.RedisClient = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = configs.RedisClient.Close()
		configs.RedisClient = oldClient
	})
	return server
}
