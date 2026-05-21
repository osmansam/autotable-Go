package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
)

type DynamicCache struct{}

func NewDynamicCache() *DynamicCache {
	return &DynamicCache{}
}

func (d *DynamicCache) InvalidateCreateCaches(ctx context.Context, tenantID, projectID, schemaName string, container *models.ContainerModel) error {
	return d.invalidateWriteCaches(ctx, tenantID, projectID, schemaName, container)
}

func (d *DynamicCache) GetItems(ctx context.Context, key string) ([]map[string]interface{}, bool) {
	if !configs.RedisCircuitAllow() {
		return nil, false
	}

	data, err := configs.RedisClient.Get(ctx, key).Result()
	configs.RedisCircuitRecordResult(err)
	if err != nil {
		return nil, false
	}

	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(data), &items); err != nil {
		return nil, false
	}

	return items, true
}

func (d *DynamicCache) GetItem(ctx context.Context, key string) (map[string]interface{}, bool) {
	if !configs.RedisCircuitAllow() {
		return nil, false
	}

	data, err := configs.RedisClient.Get(ctx, key).Result()
	configs.RedisCircuitRecordResult(err)
	if err != nil {
		return nil, false
	}

	var item map[string]interface{}
	if err := json.Unmarshal([]byte(data), &item); err != nil {
		return nil, false
	}

	return item, true
}

func (d *DynamicCache) SetItems(ctx context.Context, key string, items []map[string]interface{}, ttlMinutes int) {
	_ = utils.SetCache(ctx, key, items, cacheTTL(ttlMinutes))
}

func (d *DynamicCache) GetResponse(ctx context.Context, key string) (fiber.Map, bool) {
	if !configs.RedisCircuitAllow() {
		return nil, false
	}

	data, err := configs.RedisClient.Get(ctx, key).Result()
	configs.RedisCircuitRecordResult(err)
	if err != nil {
		return nil, false
	}

	var response fiber.Map
	if err := json.Unmarshal([]byte(data), &response); err != nil {
		return nil, false
	}

	return response, true
}

func (d *DynamicCache) SetResponse(ctx context.Context, key string, response fiber.Map, ttlMinutes int) {
	_ = utils.SetCache(ctx, key, response, cacheTTL(ttlMinutes))
}

func (d *DynamicCache) GetValue(ctx context.Context, key string) (interface{}, bool) {
	if !configs.RedisCircuitAllow() {
		return nil, false
	}

	data, err := configs.RedisClient.Get(ctx, key).Result()
	configs.RedisCircuitRecordResult(err)
	if err != nil {
		return nil, false
	}

	var result interface{}
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil, false
	}

	return result, true
}

func (d *DynamicCache) SetValue(ctx context.Context, key string, value interface{}, ttl time.Duration) {
	_ = utils.SetCache(ctx, key, value, ttl)
}

func (d *DynamicCache) GetPipelineItems(ctx context.Context, key, currentQuery string) ([]map[string]interface{}, bool) {
	if !configs.RedisCircuitAllow() {
		return nil, false
	}

	storedQuery, err := configs.RedisClient.Get(ctx, key+"-query").Result()
	configs.RedisCircuitRecordResult(err)
	if err == nil && storedQuery != currentQuery {
		return nil, false
	}

	return d.GetItems(ctx, key)
}

func (d *DynamicCache) SetPipelineItems(ctx context.Context, key, currentQuery string, items []map[string]interface{}, cacheMinutes int) {
	payload, err := json.Marshal(items)
	if err != nil {
		return
	}

	expiration := cacheTTL(cacheMinutes)
	if !configs.RedisCircuitAllow() {
		return
	}
	err = configs.RedisClient.Set(ctx, key, payload, expiration).Err()
	configs.RedisCircuitRecordResult(err)
	if err != nil {
		return
	}
	err = configs.RedisClient.Set(ctx, key+"-query", currentQuery, expiration).Err()
	configs.RedisCircuitRecordResult(err)
}

func (d *DynamicCache) SetItem(ctx context.Context, key string, item map[string]interface{}, ttlMinutes int) {
	_ = utils.SetCache(ctx, key, item, cacheTTL(ttlMinutes))
}

func (d *DynamicCache) InvalidateUpdateCaches(ctx context.Context, tenantID, projectID, schemaName string, container *models.ContainerModel, onTriggeredSchema func(string)) error {
	if err := d.invalidateWriteCaches(ctx, tenantID, projectID, schemaName, container); err != nil {
		return err
	}

	if container.Redis.IsRedisCached && onTriggeredSchema != nil {
		for _, triggeredSchema := range container.Redis.TriggeredRedisCaches {
			onTriggeredSchema(triggeredSchema)
		}
	}

	return nil
}

func (d *DynamicCache) invalidateWriteCaches(ctx context.Context, tenantID, projectID, schemaName string, container *models.ContainerModel) error {
	if container == nil {
		return nil
	}

	return utils.InvalidateSchemaAndTriggeredCaches(ctx, tenantID, projectID, schemaName, container.Redis.TriggeredRedisCaches)
}

func cacheTTL(ttlMinutes int) time.Duration {
	if ttlMinutes > 0 {
		return time.Duration(ttlMinutes) * time.Minute
	}
	return utils.DefaultCacheTTLDuration()
}
