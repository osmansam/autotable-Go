package cache

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/observability"
	"github.com/osmansam/autotableGo/utils"
)

type DynamicCache struct{}

func NewDynamicCache() *DynamicCache {
	return &DynamicCache{}
}

func (d *DynamicCache) InvalidateCreateCaches(ctx context.Context, tenantID, projectID, schemaName string, container *models.ContainerModel) error {
	ctx, span := observability.StartSpan(ctx, "redis.cache", observability.CacheTraceAttrs("invalidate", schemaName)...)
	start := time.Now()
	err := d.invalidateWriteCaches(ctx, tenantID, projectID, schemaName, container)
	recordCacheResult("invalidate", schemaName, err)
	logCacheError(ctx, "invalidate", schemaName, start, err)
	observability.EndSpan(span, cacheStatus(err), err)
	return err
}

func (d *DynamicCache) GetItems(ctx context.Context, key string) ([]map[string]interface{}, bool) {
	schemaName := cacheSchemaName(key)
	ctx, span := observability.StartSpan(ctx, "redis.cache", observability.CacheTraceAttrs("get_items", schemaName)...)
	status := "hit"
	var spanErr error
	defer func() { observability.EndSpan(span, status, spanErr) }()
	if !configs.RedisCircuitAllow() {
		status = "skipped"
		observability.RecordCacheRequest("get_items", schemaName, "skipped")
		return nil, false
	}

	data, err := configs.RedisClient.Get(ctx, key).Result()
	configs.RedisCircuitRecordResult(err)
	if err != nil {
		status = "miss"
		observability.RecordCacheRequest("get_items", schemaName, "miss")
		return nil, false
	}

	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(data), &items); err != nil {
		status = "error"
		spanErr = err
		observability.RecordCacheRequest("get_items", schemaName, "error")
		logCacheError(ctx, "get_items", schemaName, time.Now(), err)
		return nil, false
	}

	observability.RecordCacheRequest("get_items", schemaName, "hit")
	return items, true
}

func (d *DynamicCache) GetItem(ctx context.Context, key string) (map[string]interface{}, bool) {
	schemaName := cacheSchemaName(key)
	ctx, span := observability.StartSpan(ctx, "redis.cache", observability.CacheTraceAttrs("get_item", schemaName)...)
	status := "hit"
	var spanErr error
	defer func() { observability.EndSpan(span, status, spanErr) }()
	if !configs.RedisCircuitAllow() {
		status = "skipped"
		observability.RecordCacheRequest("get_item", schemaName, "skipped")
		return nil, false
	}

	data, err := configs.RedisClient.Get(ctx, key).Result()
	configs.RedisCircuitRecordResult(err)
	if err != nil {
		status = "miss"
		observability.RecordCacheRequest("get_item", schemaName, "miss")
		return nil, false
	}

	var item map[string]interface{}
	if err := json.Unmarshal([]byte(data), &item); err != nil {
		status = "error"
		spanErr = err
		observability.RecordCacheRequest("get_item", schemaName, "error")
		logCacheError(ctx, "get_item", schemaName, time.Now(), err)
		return nil, false
	}

	observability.RecordCacheRequest("get_item", schemaName, "hit")
	return item, true
}

func (d *DynamicCache) SetItems(ctx context.Context, key string, items []map[string]interface{}, ttlMinutes int) {
	start := time.Now()
	schemaName := cacheSchemaName(key)
	ctx, span := observability.StartSpan(ctx, "redis.cache", observability.CacheTraceAttrs("set_items", schemaName)...)
	err := utils.SetCache(ctx, key, items, cacheTTL(ttlMinutes))
	recordCacheResult("set_items", schemaName, err)
	logCacheError(ctx, "set_items", schemaName, start, err)
	observability.EndSpan(span, cacheStatus(err), err)
}

func (d *DynamicCache) GetResponse(ctx context.Context, key string) (fiber.Map, bool) {
	schemaName := cacheSchemaName(key)
	ctx, span := observability.StartSpan(ctx, "redis.cache", observability.CacheTraceAttrs("get_response", schemaName)...)
	status := "hit"
	var spanErr error
	defer func() { observability.EndSpan(span, status, spanErr) }()
	if !configs.RedisCircuitAllow() {
		status = "skipped"
		observability.RecordCacheRequest("get_response", schemaName, "skipped")
		return nil, false
	}

	data, err := configs.RedisClient.Get(ctx, key).Result()
	configs.RedisCircuitRecordResult(err)
	if err != nil {
		status = "miss"
		observability.RecordCacheRequest("get_response", schemaName, "miss")
		return nil, false
	}

	var response fiber.Map
	if err := json.Unmarshal([]byte(data), &response); err != nil {
		status = "error"
		spanErr = err
		observability.RecordCacheRequest("get_response", schemaName, "error")
		logCacheError(ctx, "get_response", schemaName, time.Now(), err)
		return nil, false
	}

	observability.RecordCacheRequest("get_response", schemaName, "hit")
	return response, true
}

func (d *DynamicCache) SetResponse(ctx context.Context, key string, response fiber.Map, ttlMinutes int) {
	start := time.Now()
	schemaName := cacheSchemaName(key)
	ctx, span := observability.StartSpan(ctx, "redis.cache", observability.CacheTraceAttrs("set_response", schemaName)...)
	err := utils.SetCache(ctx, key, response, cacheTTL(ttlMinutes))
	recordCacheResult("set_response", schemaName, err)
	logCacheError(ctx, "set_response", schemaName, start, err)
	observability.EndSpan(span, cacheStatus(err), err)
}

func (d *DynamicCache) GetValue(ctx context.Context, key string) (interface{}, bool) {
	schemaName := cacheSchemaName(key)
	ctx, span := observability.StartSpan(ctx, "redis.cache", observability.CacheTraceAttrs("get_value", schemaName)...)
	status := "hit"
	var spanErr error
	defer func() { observability.EndSpan(span, status, spanErr) }()
	if !configs.RedisCircuitAllow() {
		status = "skipped"
		observability.RecordCacheRequest("get_value", schemaName, "skipped")
		return nil, false
	}

	data, err := configs.RedisClient.Get(ctx, key).Result()
	configs.RedisCircuitRecordResult(err)
	if err != nil {
		status = "miss"
		observability.RecordCacheRequest("get_value", schemaName, "miss")
		return nil, false
	}

	var result interface{}
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		status = "error"
		spanErr = err
		observability.RecordCacheRequest("get_value", schemaName, "error")
		logCacheError(ctx, "get_value", schemaName, time.Now(), err)
		return nil, false
	}

	observability.RecordCacheRequest("get_value", schemaName, "hit")
	return result, true
}

func (d *DynamicCache) SetValue(ctx context.Context, key string, value interface{}, ttl time.Duration) {
	start := time.Now()
	schemaName := cacheSchemaName(key)
	ctx, span := observability.StartSpan(ctx, "redis.cache", observability.CacheTraceAttrs("set_value", schemaName)...)
	err := utils.SetCache(ctx, key, value, ttl)
	recordCacheResult("set_value", schemaName, err)
	logCacheError(ctx, "set_value", schemaName, start, err)
	observability.EndSpan(span, cacheStatus(err), err)
}

func (d *DynamicCache) GetPipelineItems(ctx context.Context, key, currentQuery string) ([]map[string]interface{}, bool) {
	schemaName := cacheSchemaName(key)
	ctx, span := observability.StartSpan(ctx, "redis.cache", observability.CacheTraceAttrs("get_pipeline_items", schemaName)...)
	status := "hit"
	var spanErr error
	defer func() { observability.EndSpan(span, status, spanErr) }()
	if !configs.RedisCircuitAllow() {
		status = "skipped"
		observability.RecordCacheRequest("get_pipeline_items", schemaName, "skipped")
		return nil, false
	}

	storedQuery, err := configs.RedisClient.Get(ctx, key+"-query").Result()
	configs.RedisCircuitRecordResult(err)
	if err == nil && storedQuery != currentQuery {
		status = "miss"
		observability.RecordCacheRequest("get_pipeline_items", schemaName, "miss")
		return nil, false
	}

	items, ok := d.GetItems(ctx, key)
	if !ok {
		status = "miss"
	}
	return items, ok
}

func (d *DynamicCache) SetPipelineItems(ctx context.Context, key, currentQuery string, items []map[string]interface{}, cacheMinutes int) {
	schemaName := cacheSchemaName(key)
	ctx, span := observability.StartSpan(ctx, "redis.cache", observability.CacheTraceAttrs("set_pipeline_items", schemaName)...)
	status := "success"
	var spanErr error
	defer func() { observability.EndSpan(span, status, spanErr) }()
	payload, err := json.Marshal(items)
	if err != nil {
		status = "error"
		spanErr = err
		observability.RecordCacheRequest("set_pipeline_items", schemaName, "error")
		logCacheError(ctx, "set_pipeline_items", schemaName, time.Now(), err)
		return
	}

	start := time.Now()
	expiration := cacheTTL(cacheMinutes)
	if !configs.RedisCircuitAllow() {
		status = "skipped"
		observability.RecordCacheRequest("set_pipeline_items", schemaName, "skipped")
		return
	}
	err = configs.RedisClient.Set(ctx, key, payload, expiration).Err()
	configs.RedisCircuitRecordResult(err)
	if err != nil {
		status = "error"
		spanErr = err
		observability.RecordCacheRequest("set_pipeline_items", schemaName, "error")
		logCacheError(ctx, "set_pipeline_items", schemaName, start, err)
		return
	}
	err = configs.RedisClient.Set(ctx, key+"-query", currentQuery, expiration).Err()
	configs.RedisCircuitRecordResult(err)
	status = cacheStatus(err)
	spanErr = err
	recordCacheResult("set_pipeline_items", schemaName, err)
	logCacheError(ctx, "set_pipeline_items", schemaName, start, err)
}

func (d *DynamicCache) SetItem(ctx context.Context, key string, item map[string]interface{}, ttlMinutes int) {
	start := time.Now()
	schemaName := cacheSchemaName(key)
	ctx, span := observability.StartSpan(ctx, "redis.cache", observability.CacheTraceAttrs("set_item", schemaName)...)
	err := utils.SetCache(ctx, key, item, cacheTTL(ttlMinutes))
	recordCacheResult("set_item", schemaName, err)
	logCacheError(ctx, "set_item", schemaName, start, err)
	observability.EndSpan(span, cacheStatus(err), err)
}

func (d *DynamicCache) InvalidateUpdateCaches(ctx context.Context, tenantID, projectID, schemaName string, container *models.ContainerModel, onTriggeredSchema func(string)) error {
	ctx, span := observability.StartSpan(ctx, "redis.cache", observability.CacheTraceAttrs("invalidate", schemaName)...)
	start := time.Now()
	if err := d.invalidateWriteCaches(ctx, tenantID, projectID, schemaName, container); err != nil {
		recordCacheResult("invalidate", schemaName, err)
		logCacheError(ctx, "invalidate", schemaName, start, err)
		observability.EndSpan(span, "error", err)
		return err
	}

	if container.Redis.IsRedisCached && onTriggeredSchema != nil {
		for _, triggeredSchema := range container.Redis.TriggeredRedisCaches {
			onTriggeredSchema(triggeredSchema)
		}
	}

	recordCacheResult("invalidate", schemaName, nil)
	observability.EndSpan(span, "success", nil)
	return nil
}

func (d *DynamicCache) invalidateWriteCaches(ctx context.Context, tenantID, projectID, schemaName string, container *models.ContainerModel) error {
	if container == nil {
		return nil
	}

	return utils.InvalidateSchemaAndTriggeredCaches(ctx, tenantID, projectID, schemaName, container.Redis.TriggeredRedisCaches)
}

func recordCacheResult(operation, schemaName string, err error) {
	result := "success"
	if err != nil {
		result = "error"
	}
	observability.RecordCacheRequest(operation, schemaName, result)
}

func cacheStatus(err error) string {
	if err != nil {
		return "error"
	}
	return "success"
}

func logCacheError(ctx context.Context, operation, schemaName string, start time.Time, err error) {
	if err == nil {
		return
	}
	attrs := observability.OperationAttrs(operation, "error", time.Since(start))
	attrs = append(attrs, slog.String(observability.FieldSchemaName, schemaName))
	observability.ErrorCtx(ctx, "cache operation failed", err, attrs...)
}

func cacheSchemaName(key string) string {
	parts := strings.Split(key, ":")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "schema" {
			return parts[i+1]
		}
	}
	return ""
}

func cacheTTL(ttlMinutes int) time.Duration {
	if ttlMinutes > 0 {
		return time.Duration(ttlMinutes) * time.Minute
	}
	return utils.DefaultCacheTTLDuration()
}
