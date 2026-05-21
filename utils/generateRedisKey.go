package utils

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
)

const DefaultCacheTTL = 10 * time.Minute

func DefaultCacheTTLDuration() time.Duration {
	return configs.GetDefaultCacheTTL()
}

func BuildSchemaCacheVersionKey(tenantID, projectID, schemaName string) string {
	return fmt.Sprintf("tenant:%s:project:%s:schema:%s:version", tenantID, projectID, schemaName)
}

func GetSchemaCacheVersion(ctx context.Context, tenantID, projectID, schemaName string) (int64, error) {
	versionKey := BuildSchemaCacheVersionKey(tenantID, projectID, schemaName)
	version, err := configs.RedisClient.Get(ctx, versionKey).Int64()
	if err == nil {
		return version, nil
	}
	if err != redis.Nil {
		return 0, err
	}

	created, err := configs.RedisClient.SetNX(ctx, versionKey, int64(1), 0).Result()
	if err != nil {
		return 0, err
	}
	if created {
		return 1, nil
	}

	return configs.RedisClient.Get(ctx, versionKey).Int64()
}

func IncrementSchemaCacheVersion(ctx context.Context, tenantID, projectID, schemaName string) error {
	versionKey := BuildSchemaCacheVersionKey(tenantID, projectID, schemaName)
	return configs.RedisClient.Incr(ctx, versionKey).Err()
}

func BuildVersionedCacheKey(tenantID string, projectID string, schemaName string, version int64, routeName string, queryHash string) string {
	return fmt.Sprintf("tenant:%s:project:%s:schema:%s:v%d:route:%s:query:%s", tenantID, projectID, schemaName, version, routeName, queryHash)
}

func HashCacheQuery(query string) string {
	sum := sha256.Sum256([]byte(query))
	return hex.EncodeToString(sum[:])
}

func SetCache(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if ttl <= 0 {
		ttl = DefaultCacheTTLDuration()
	}
	return configs.RedisClient.Set(ctx, key, payload, ttl).Err()
}

func InvalidateSchemaAndTriggeredCaches(ctx context.Context, tenantID string, projectID string, schemaName string, triggeredSchemas []string) error {
	if err := IncrementSchemaCacheVersion(ctx, tenantID, projectID, schemaName); err != nil {
		return err
	}

	for _, triggeredSchema := range triggeredSchemas {
		if triggeredSchema == "" || triggeredSchema == schemaName {
			continue
		}
		if err := IncrementSchemaCacheVersion(ctx, tenantID, projectID, triggeredSchema); err != nil {
			log.Printf("failed to increment cache version for triggered schema %s: %v", triggeredSchema, err)
			continue
		}
	}

	return nil
}

// GenerateRedisKey generates a Redis key for caching based on route, schema names, tenant/project context, and an optional ID or URL.
// For non-tenant routes (auth), pass empty strings for tenantID and projectID.
func GenerateRedisKey(routeName, tenantID, projectID, schemaName string, container *models.ContainerModel, id ...string) (string, bool) {
	var redisKey string
	var shouldCache bool

	// Check if caching should be applied based on the IsRedisCached flag
	switch routeName {
	case "GetAllDynamicModelItems":
		shouldCache = container.Redis.IsRedisCached
	case "GetAllDynamicModelItemsWithPagination":
		shouldCache = container.Redis.IsRedisCached
	case "GetDynamicModelItem":
		shouldCache = container.Redis.IsRedisCached
	}

	if shouldCache {
		// Build project-specific prefix if tenant/project are provided
		var prefix string
		if tenantID != "" && projectID != "" {
			prefix = "tenant_" + tenantID + "_project_" + projectID + "_"
		} else {
			prefix = "" // Legacy global cache
		}

		if len(id) > 0 && id[0] != "" {
			// For pagination, id[0] contains the full URL with query params
			// For single items, id[0] contains the item ID
			redisKey = prefix + "schema_" + schemaName + "_route_" + routeName + "_" + id[0]
		} else {
			redisKey = prefix + "schema_" + schemaName + "_route_" + routeName
		}
	}

	return redisKey, shouldCache
}

// GeneratePipelineRedisKey generates a Redis key for caching pipelines based on tenant/project context, schema name and pipeline name.
// For non-tenant routes, pass empty strings for tenantID and projectID.
func GeneratePipelineRedisKey(tenantID, projectID, schemaName, pipelineName string, container *models.ContainerModel) (string, bool) {
	var redisKey string
	var shouldCache bool

	// Check if caching should be applied based on the IsRedisCached flag for the specified pipeline
	for _, pipeline := range container.Pipelines {
		if pipeline.Name == pipelineName && pipeline.IsRedisCached {
			shouldCache = true
			break
		}
	}

	if shouldCache {
		// Build project-specific prefix if tenant/project are provided
		var prefix string
		if tenantID != "" && projectID != "" {
			prefix = "tenant_" + tenantID + "_project_" + projectID + "_"
		} else {
			prefix = "" // Legacy global cache
		}
		redisKey = prefix + "pipeline_" + pipelineName + "_schema_" + schemaName
	}

	return redisKey, shouldCache
}

// GenerateDynamicFunctionRedisKey generates a Redis key for caching dynamic functions based on tenant/project context, schema name and function name.
// For non-tenant routes, pass empty strings for tenantID and projectID.
func GenerateDynamicFunctionRedisKey(tenantID, projectID, schemaName, functionName string, container *models.ContainerModel) (string, bool) {
	var redisKey string
	var shouldCache bool

	// Check if caching should be applied based on the IsRedisCached flag for the specified function
	for _, function := range container.DynamicFunctions {
		if function.Name == functionName && function.IsRedisCached {
			shouldCache = true
			break
		}
	}

	if shouldCache {
		// Build project-specific prefix if tenant/project are provided
		var prefix string
		if tenantID != "" && projectID != "" {
			prefix = "tenant_" + tenantID + "_project_" + projectID + "_"
		} else {
			prefix = "" // Legacy global cache
		}
		redisKey = prefix + "function_" + functionName + "_schema_" + schemaName
	}

	return redisKey, shouldCache
}

// GenerateDynamicApiRedisKey generates a Redis key for caching dynamic APIs based on tenant/project context, schema name and API name.
// For non-tenant routes, pass empty strings for tenantID and projectID.
func GenerateDynamicApiRedisKey(tenantID, projectID, schemaName, apiName string, container *models.ContainerModel) (string, bool) {
	var redisKey string
	var shouldCache bool

	// Check if caching should be applied based on the IsRedisCached flag for the specified API
	for _, api := range container.DynamicApis {
		if api.Name == apiName && api.IsRedisCached {
			shouldCache = true
			break
		}
	}

	if shouldCache {
		// Build project-specific prefix if tenant/project are provided
		var prefix string
		if tenantID != "" && projectID != "" {
			prefix = "tenant_" + tenantID + "_project_" + projectID + "_"
		} else {
			prefix = "" // Legacy global cache
		}
		redisKey = prefix + "api_" + apiName + "_schema_" + schemaName
	}

	return redisKey, shouldCache
}
