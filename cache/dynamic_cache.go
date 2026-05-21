package cache

import (
	"context"
	"encoding/json"
	"log"
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
	data, err := configs.RedisClient.Get(ctx, key).Result()
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
	data, err := configs.RedisClient.Get(ctx, key).Result()
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
	payload, err := json.Marshal(items)
	if err != nil {
		return
	}
	ttl := time.Duration(ttlMinutes) * time.Minute
	configs.RedisClient.Set(ctx, key, payload, ttl)
}

func (d *DynamicCache) GetResponse(ctx context.Context, key string) (fiber.Map, bool) {
	data, err := configs.RedisClient.Get(ctx, key).Result()
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
	payload, err := json.Marshal(response)
	if err != nil {
		return
	}
	ttl := time.Duration(ttlMinutes) * time.Minute
	configs.RedisClient.Set(ctx, key, payload, ttl)
}

func (d *DynamicCache) GetPipelineItems(ctx context.Context, key, currentQuery string) ([]map[string]interface{}, bool) {
	storedQuery, err := configs.RedisClient.Get(ctx, key+"-query").Result()
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

	var expiration time.Duration
	if cacheMinutes > 0 {
		expiration = time.Duration(cacheMinutes) * time.Minute
	}

	configs.RedisClient.Set(ctx, key, payload, expiration)
	configs.RedisClient.Set(ctx, key+"-query", currentQuery, expiration)
}

func (d *DynamicCache) SetItem(ctx context.Context, key string, item map[string]interface{}, ttlMinutes int) {
	payload, err := json.Marshal(item)
	if err != nil {
		return
	}
	ttl := time.Duration(ttlMinutes) * time.Minute
	configs.RedisClient.Set(ctx, key, payload, ttl)
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
	if !container.Redis.IsRedisCached {
		return nil
	}

	if err := utils.DeleteCacheForSchema(ctx, tenantID, projectID, schemaName, container); err != nil {
		return err
	}

	for _, triggeredSchema := range container.Redis.TriggeredRedisCaches {
		if err := utils.DeleteCacheForSchema(ctx, tenantID, projectID, triggeredSchema, container); err != nil {
			log.Printf("Error deleting cache for schema %s: %v", triggeredSchema, err)
			continue
		}
	}

	return nil
}
