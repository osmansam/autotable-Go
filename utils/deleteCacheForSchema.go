package utils

import (
	"context"
	"fmt"
	"log"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
)

// DeleteCacheForSchema deletes cache related to specific routes and pipelines for a given schema.
// For tenant-authenticated routes, pass tenantID and projectID. For non-tenant routes (auth), pass empty strings.
func DeleteCacheForSchema(ctx context.Context, tenantID, projectID, schemaName string, container *models.ContainerModel) error {
    // Generate keys for routes with tenant/project context
    keyGetAll, _ := GenerateRedisKey("GetAllDynamicModelItems", tenantID, projectID, schemaName, container)

    // Delete the specific keys
    if _, err := configs.RedisClient.Del(ctx, keyGetAll).Result(); err != nil {
        return fmt.Errorf("failed to delete key %s: %v", keyGetAll, err)
    }

    // Delete all item caches for this schema
    // Build project-specific pattern
    var itemPattern string
    if tenantID != "" && projectID != "" {
        itemPattern = "tenant_" + tenantID + "_project_" + projectID + "_schema_" + schemaName + "_route_GetDynamicModelItem_*"
    } else {
        itemPattern = "schema_" + schemaName + "_route_GetDynamicModelItem_*"
    }
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
    // Build project-specific pattern
    var paginationPattern string
    if tenantID != "" && projectID != "" {
        paginationPattern = "tenant_" + tenantID + "_project_" + projectID + "_schema_" + schemaName + "_route_GetAllDynamicModelItemsWithPagination_*"
    } else {
        paginationPattern = "schema_" + schemaName + "_route_GetAllDynamicModelItemsWithPagination_*"
    }
    
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


    var pipelineContainer *models.ContainerModel
    var err error
    // Use provided container if available, otherwise fetch it
    if container != nil {
        pipelineContainer = container
    } else if tenantID != "" && projectID != "" {
        pipelineContainer, err = GetContainerModel(tenantID, projectID, schemaName)
        if err != nil {
            return fmt.Errorf(" Failed to fetch container model: %v",  err)
        }
    } else {
        // No tenant context - skip pipeline cache deletion
        return nil
    }
    // Iterate through each pipeline and delete cache if IsRedisCached is true
    for _, pipeline := range pipelineContainer.Pipelines {
        if pipeline.IsRedisCached {
            pipelineKey, _ := GeneratePipelineRedisKey(tenantID, projectID, schemaName, pipeline.Name, pipelineContainer)
            if _, err := configs.RedisClient.Del(ctx, pipelineKey).Result(); err != nil {
                return fmt.Errorf("failed to delete pipeline cache key %s: %v", pipelineKey, err)
            }
        }
    }
    // Iterate through each dynamic function and delete cache if IsRedisCached is true
    for _, function := range pipelineContainer.DynamicFunctions {
        if function.IsRedisCached {
            functionKey, _ := GenerateDynamicFunctionRedisKey(tenantID, projectID, schemaName, function.Name, pipelineContainer)
            if _, err := configs.RedisClient.Del(ctx, functionKey).Result(); err != nil {
                return fmt.Errorf("failed to delete function cache key %s: %v", functionKey, err)
            }
        }
    }

    return nil
}

