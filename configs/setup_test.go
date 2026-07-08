package configs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func TestBulkOperationLimitDefaults(t *testing.T) {
	if MaxBulkWriteLimit != 3000 {
		t.Fatalf("MaxBulkWriteLimit = %d, want 3000", MaxBulkWriteLimit)
	}
	if MaxBulkUpdateLimit != 3000 {
		t.Fatalf("MaxBulkUpdateLimit = %d, want 3000", MaxBulkUpdateLimit)
	}
	if MaxBulkDeleteLimit != 3000 {
		t.Fatalf("MaxBulkDeleteLimit = %d, want 3000", MaxBulkDeleteLimit)
	}

	overrideAppConfigForTest(t, &Config{})
	if got := GetMaxBulkWriteLimit(); got != 3000 {
		t.Fatalf("GetMaxBulkWriteLimit() = %d, want 3000", got)
	}
	if got := GetMaxBulkUpdateLimit(); got != 3000 {
		t.Fatalf("GetMaxBulkUpdateLimit() = %d, want 3000", got)
	}
	if got := GetMaxBulkDeleteLimit(); got != 3000 {
		t.Fatalf("GetMaxBulkDeleteLimit() = %d, want 3000", got)
	}
}

func TestConfigGettersDefaultsAndOverrides(t *testing.T) {
	tests := []struct {
		name string
		set  func(*Config)
		get  func() interface{}
		want interface{}
	}{
		{name: "default page limit", get: func() interface{} { return GetDefaultPageLimit() }, want: DefaultPageLimit},
		{name: "custom page limit", set: func(c *Config) { c.Limits.DefaultPageLimit = 7 }, get: func() interface{} { return GetDefaultPageLimit() }, want: 7},
		{name: "negative default page limit falls back", set: func(c *Config) { c.Limits.DefaultPageLimit = -1 }, get: func() interface{} { return GetDefaultPageLimit() }, want: DefaultPageLimit},
		{name: "default max page limit", get: func() interface{} { return GetMaxPageLimit() }, want: MaxPageLimit},
		{name: "zero max page limit falls back", set: func(c *Config) { c.Limits.MaxPageLimit = 0 }, get: func() interface{} { return GetMaxPageLimit() }, want: MaxPageLimit},
		{name: "default max unbounded read limit", get: func() interface{} { return GetMaxUnboundedReadLimit() }, want: MaxUnboundedReadLimit},
		{name: "default max export limit", get: func() interface{} { return GetMaxExportLimit() }, want: MaxExportLimit},
		{name: "default max bulk update limit", get: func() interface{} { return GetMaxBulkUpdateLimit() }, want: MaxBulkUpdateLimit},
		{name: "default max bulk delete limit", get: func() interface{} { return GetMaxBulkDeleteLimit() }, want: MaxBulkDeleteLimit},
		{name: "default cache ttl", get: func() interface{} { return GetDefaultCacheTTL() }, want: time.Duration(DefaultCacheTTLMinutes) * time.Minute},
		{name: "custom cache ttl", set: func(c *Config) { c.Cache.DefaultCacheTTLMinutes = 3 }, get: func() interface{} { return GetDefaultCacheTTL() }, want: 3 * time.Minute},
		{name: "negative cache ttl falls back", set: func(c *Config) { c.Cache.DefaultCacheTTLMinutes = -5 }, get: func() interface{} { return GetDefaultCacheTTL() }, want: time.Duration(DefaultCacheTTLMinutes) * time.Minute},
		{name: "default cache fill lock ttl", get: func() interface{} { return GetCacheFillLockTTL() }, want: time.Duration(CacheFillLockTTLSeconds) * time.Second},
		{name: "default cache fill wait timeout", get: func() interface{} { return GetCacheFillWaitTimeout() }, want: time.Duration(CacheFillWaitTimeoutMilliseconds) * time.Millisecond},
		{name: "default cache fill poll interval", get: func() interface{} { return GetCacheFillPollInterval() }, want: time.Duration(CacheFillPollIntervalMilliseconds) * time.Millisecond},
		{name: "default outbox retention", get: func() interface{} { return GetOutboxDoneRetention() }, want: time.Duration(OutboxDoneRetentionHours) * time.Hour},
		{name: "default failed outbox retention", get: func() interface{} { return GetOutboxFailedRetention() }, want: time.Duration(OutboxFailedRetentionDays) * 24 * time.Hour},
		{name: "default audit retention", get: func() interface{} { return GetAuditLogRetentionSeconds() }, want: int32(AuditLogRetentionDays * 24 * 60 * 60)},
		{name: "custom bulk write limit", set: func(c *Config) { c.Limits.MaxBulkWriteLimit = 12 }, get: func() interface{} { return GetMaxBulkWriteLimit() }, want: 12},
		{name: "default redis pool size", get: func() interface{} { return GetRedisPoolSize() }, want: RedisPoolSize},
		{name: "zero redis pool size falls back", set: func(c *Config) { c.Redis.Pool.PoolSize = 0 }, get: func() interface{} { return GetRedisPoolSize() }, want: RedisPoolSize},
		{name: "default redis min idle", get: func() interface{} { return GetRedisMinIdleConnections() }, want: RedisMinIdleConnections},
		{name: "custom redis min idle clamps to pool", set: func(c *Config) { c.Redis.Pool.PoolSize = 2; c.Redis.Pool.MinIdleConnections = 5 }, get: func() interface{} { return GetRedisMinIdleConnections() }, want: 2},
		{name: "default redis pool timeout", get: func() interface{} { return GetRedisPoolTimeout() }, want: time.Duration(RedisPoolTimeoutSeconds) * time.Second},
		{name: "default redis idle timeout", get: func() interface{} { return GetRedisIdleTimeout() }, want: time.Duration(RedisIdleTimeoutSeconds) * time.Second},
		{name: "default redis idle frequency", get: func() interface{} { return GetRedisIdleCheckFrequency() }, want: time.Duration(RedisIdleCheckFrequencySeconds) * time.Second},
		{name: "default redis max age", get: func() interface{} { return GetRedisMaxConnectionAge() }, want: time.Duration(RedisMaxConnectionAgeSeconds) * time.Second},
		{name: "default mongo max pool", get: func() interface{} { return GetMongoMaxPoolSize() }, want: uint64(MongoMaxPoolSize)},
		{name: "zero mongo max pool falls back", set: func(c *Config) { c.Mongo.Pool.MaxPoolSize = 0 }, get: func() interface{} { return GetMongoMaxPoolSize() }, want: uint64(MongoMaxPoolSize)},
		{name: "custom mongo min pool clamps to max", set: func(c *Config) { c.Mongo.Pool.MinPoolSize = 8; c.Mongo.Pool.MaxPoolSize = 3 }, get: func() interface{} { return GetMongoMinPoolSize() }, want: uint64(3)},
		{name: "default mongo idle", get: func() interface{} { return GetMongoMaxConnectionIdleTime() }, want: time.Duration(MongoMaxConnectionIdleSeconds) * time.Second},
		{name: "default mongo connect timeout", get: func() interface{} { return GetMongoConnectTimeout() }, want: time.Duration(MongoConnectTimeoutSeconds) * time.Second},
		{name: "default body size", get: func() interface{} { return GetDefaultBodySizeLimit() }, want: DefaultBodySizeBytes},
		{name: "default bulk write body size", get: func() interface{} { return GetBulkWriteBodySizeLimit() }, want: BulkWriteBodySizeBytes},
		{name: "default bulk update body size", get: func() interface{} { return GetBulkUpdateBodySizeLimit() }, want: BulkUpdateBodySizeBytes},
		{name: "default bulk delete body size", get: func() interface{} { return GetBulkDeleteBodySizeLimit() }, want: BulkDeleteBodySizeBytes},
		{name: "default export body size", get: func() interface{} { return GetExportBodySizeLimit() }, want: ExportBodySizeBytes},
		{name: "default upload body size", get: func() interface{} { return GetUploadBodySizeLimit() }, want: UploadBodySizeBytes},
		{name: "max body size", set: func(c *Config) { c.Limits.BodySizeLimits.UploadBodySizeBytes = 99 }, get: func() interface{} { return GetMaxRequestBodySizeLimit() }, want: BulkWriteBodySizeBytes},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			if tt.set != nil {
				tt.set(cfg)
			}
			overrideAppConfigForTest(t, cfg)
			if got := tt.get(); got != tt.want {
				t.Fatalf("getter() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"limits":{"defaultPageLimit":7}}`), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil || cfg.Limits.DefaultPageLimit != 7 {
		t.Fatalf("LoadConfig() = %#v, %v", cfg, err)
	}
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("LoadConfig(missing) error = nil")
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"limits":`), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("LoadConfig(invalid JSON) error = nil")
	}
}

func TestConfigFileForEnv(t *testing.T) {
	t.Setenv("NODE_ENV", "")
	if got := configFileForEnv(); got != "configs/development.json" {
		t.Fatalf("configFileForEnv() = %q", got)
	}
	t.Setenv("NODE_ENV", "test")
	if got := configFileForEnv(); got != "configs/test.json" {
		t.Fatalf("configFileForEnv() = %q", got)
	}
}

func TestEnvMongoURI(t *testing.T) {
	t.Setenv("MONGO_URI_BASE", "")
	t.Setenv("COLLECTION_NAME", "")
	if got := EnvMongoURI(); got != "mongodb://localhost:27017/test?retryWrites=true&w=majority" {
		t.Fatalf("EnvMongoURI() = %q", got)
	}
	t.Setenv("MONGO_URI_BASE", "mongodb://localhost/")
	t.Setenv("COLLECTION_NAME", "db")
	t.Setenv("MONGO_URI_SUFFIX", "")
	if got := EnvMongoURI(); got != "mongodb://localhost/db?retryWrites=true&w=majority" {
		t.Fatalf("EnvMongoURI() = %q", got)
	}
}

func TestGetRedisPasswordPrecedence(t *testing.T) {
	cfg := &Config{}
	cfg.Redis.Password = "config-secret"
	overrideAppConfigForTest(t, cfg)

	t.Setenv("REDIS_PASSWORD", "")
	if got := GetRedisPassword(); got != "config-secret" {
		t.Fatalf("GetRedisPassword() = %q, want config password", got)
	}

	t.Setenv("REDIS_PASSWORD", "env-secret")
	if got := GetRedisPassword(); got != "env-secret" {
		t.Fatalf("GetRedisPassword() = %q, want environment password", got)
	}
}

func TestRedisCircuitBreaker(t *testing.T) {
	cfg := &Config{}
	cfg.Redis.CircuitBreaker.Enabled = true
	cfg.Redis.CircuitBreaker.FailureThreshold = 2
	cfg.Redis.CircuitBreaker.OpenDurationSeconds = 1
	overrideAppConfigForTest(t, cfg)
	resetRedisCircuit()

	if !RedisCircuitAllow() {
		t.Fatal("RedisCircuitAllow() = false before failures")
	}
	RedisCircuitRecordFailure(errors.New("first"))
	if !RedisCircuitAllow() {
		t.Fatal("RedisCircuitAllow() = false before threshold")
	}
	RedisCircuitRecordFailure(errors.New("second"))
	if RedisCircuitAllow() {
		t.Fatal("RedisCircuitAllow() = true while open")
	}

	redisCircuit.Lock()
	redisCircuit.openedUntil = time.Now().Add(-time.Second)
	redisCircuit.Unlock()
	if !RedisCircuitAllow() {
		t.Fatal("RedisCircuitAllow() = false for first half-open probe")
	}
	if RedisCircuitAllow() {
		t.Fatal("RedisCircuitAllow() = true for concurrent half-open probe")
	}
	RedisCircuitRecordResult(redis.Nil)
	if !RedisCircuitAllow() {
		t.Fatal("RedisCircuitAllow() = false after successful probe")
	}
}

func TestRedisCircuitBreakerDisabledAlwaysAllows(t *testing.T) {
	cfg := &Config{}
	cfg.Redis.CircuitBreaker.Enabled = false
	overrideAppConfigForTest(t, cfg)
	resetRedisCircuit()

	RedisCircuitRecordFailure(errors.New("failure"))
	if !RedisCircuitAllow() {
		t.Fatal("RedisCircuitAllow() = false when disabled")
	}
}

func TestRedisWrappers(t *testing.T) {
	server := miniredis.RunT(t)
	oldClient := RedisClient
	RedisClient = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = RedisClient.Close()
		RedisClient = oldClient
	})
	overrideAppConfigForTest(t, &Config{})
	ctx := context.Background()

	if err := RedisSetValue(ctx, "key", "value", time.Minute); err != nil {
		t.Fatalf("RedisSetValue() error = %v", err)
	}
	if got, err := RedisGetString(ctx, "key"); err != nil || got != "value" {
		t.Fatalf("RedisGetString() = %q, %v", got, err)
	}
	if _, err := RedisGetString(ctx, "missing"); err != redis.Nil {
		t.Fatalf("RedisGetString(missing) error = %v, want redis.Nil", err)
	}
	if err := RedisDelKeys(ctx); err != nil {
		t.Fatalf("RedisDelKeys(empty) error = %v", err)
	}
	if err := RedisDelKeys(ctx, "key"); err != nil {
		t.Fatalf("RedisDelKeys() error = %v", err)
	}
	if err := RedisFlushAll(ctx); err != nil {
		t.Fatalf("RedisFlushAll() error = %v", err)
	}
}

func TestRedisSetValueRedisDown(t *testing.T) {
	oldClient := RedisClient
	RedisClient = redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() {
		_ = RedisClient.Close()
		RedisClient = oldClient
	})
	overrideAppConfigForTest(t, &Config{})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := RedisSetValue(ctx, "key", "value", time.Minute); err == nil {
		t.Fatal("RedisSetValue() error = nil, want Redis unavailable error")
	}
}

func overrideAppConfigForTest(t *testing.T, cfg *Config) {
	t.Helper()
	oldConfig, oldOnce := appConfig, appConfigOnce
	appConfig = cfg
	appConfigOnce = sync.Once{}
	appConfigOnce.Do(func() {})
	t.Cleanup(func() {
		appConfig, appConfigOnce = oldConfig, oldOnce
		resetRedisCircuit()
	})
}

func resetRedisCircuit() {
	redisCircuit.Lock()
	defer redisCircuit.Unlock()
	redisCircuit.failures = 0
	redisCircuit.openedUntil = time.Time{}
	redisCircuit.halfOpenInFlight = false
}
