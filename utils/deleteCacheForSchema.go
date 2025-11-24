package utils

import (
	"context"
	"fmt"
	"log"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
)

// DeleteCacheForSchema deletes cache related to specific routes and pipelines for a given schema.
func DeleteCacheForSchema(ctx context.Context, schemaName string, container *models.ContainerModel) error {
    // Generate keys for routes
    keyGetAll, _ := GenerateRedisKey("GetAllDynamicModelItems", schemaName, container)

    // Delete the specific keys
    if _, err := configs.RedisClient.Del(ctx, keyGetAll).Result(); err != nil {
        return fmt.Errorf("failed to delete key %s: %v", keyGetAll, err)
    }

    // Delete all item caches for this schema
    // Pattern: schema_<schemaName>_route_GetDynamicModelItem_*
    itemPattern := "schema_" + schemaName + "_route_GetDynamicModelItem_*"
    log.Printf("Scanning for item keys with pattern: %s", itemPattern)
    
    itemIter := configs.RedisClient.Scan(ctx, 0, itemPattern, 0).Iterator()
    deletedItemCount := 0
    for itemIter.Next(ctx) {
        key := itemIter.Val()
        if err := configs.RedisClient.Del(ctx, key).Err(); err != nil {
            log.Printf("Failed to delete item key %s: %v", key, err)
        } else {
            log.Printf("Deleted item key: %s", key)
            deletedItemCount++
        }
    }
    if err := itemIter.Err(); err != nil {
        return fmt.Errorf("failed to scan item cache keys: %v", err)
    }
    if deletedItemCount > 0 {
        log.Printf("Successfully deleted %d item cache keys for schema %s", deletedItemCount, schemaName)
    } else {
        log.Printf("No item cache keys found for schema %s", schemaName)
    }

    // Delete all pagination caches for this schema (they have query params in the key)
    // Pattern: schema_<schemaName>_route_GetAllDynamicModelItemsWithPagination_*
    paginationPattern := "schema_" + schemaName + "_route_GetAllDynamicModelItemsWithPagination_*"
    
    // Use SCAN instead of KEYS for better performance and reliability
    log.Printf("Scanning for pagination keys with pattern: %s", paginationPattern)

    iter := configs.RedisClient.Scan(ctx, 0, paginationPattern, 0).Iterator()
    deletedCount := 0
    for iter.Next(ctx) {
        key := iter.Val()
        if err := configs.RedisClient.Del(ctx, key).Err(); err != nil {
            log.Printf("Failed to delete key %s: %v", key, err)
        } else {
            log.Printf("Deleted pagination key: %s", key)
            deletedCount++
        }
    }
    if err := iter.Err(); err != nil {
        return fmt.Errorf("failed to scan pagination cache keys: %v", err)
    }

    if deletedCount > 0 {
        log.Printf("Successfully deleted %d pagination cache keys for schema %s", deletedCount, schemaName)
    } else {
        log.Printf("No pagination cache keys found for schema %s", schemaName)
    }

    // Delete count cache
    countKey := "count:" + schemaName
    configs.RedisClient.Del(ctx, countKey)
    fmt.Printf("Deleted count cache for schema %s\n", schemaName)

    var pipelineContainer *models.ContainerModel
    var err error
    pipelineContainer, err = GetContainerModel(schemaName)
        if err != nil {
           
         return fmt.Errorf(" Failed to fetch container model: %v",  err)
        }
    // Iterate through each pipeline and delete cache if IsRedisCached is true
    for _, pipeline := range pipelineContainer.Pipelines {
        if pipeline.IsRedisCached {
            pipelineKey, _ := GeneratePipelineRedisKey(schemaName, pipeline.Name, pipelineContainer)
            if _, err := configs.RedisClient.Del(ctx, pipelineKey).Result(); err != nil {
                return fmt.Errorf("failed to delete pipeline cache key %s: %v", pipelineKey, err)
            }
        }
    }
    // Iterate through each dynamic function and delete cache if IsRedisCached is true
    for _, function := range pipelineContainer.DynamicFunctions {
        if function.IsRedisCached {
            functionKey, _ := GenerateDynamicFunctionRedisKey(schemaName, function.Name, pipelineContainer)
            if _, err := configs.RedisClient.Del(ctx, functionKey).Result(); err != nil {
                return fmt.Errorf("failed to delete function cache key %s: %v", functionKey, err)
            }
        }
    }

    return nil
}

