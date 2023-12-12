package utils

import (
	"context"
	"fmt"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
)

// DeleteCacheForSchema deletes cache related to specific routes and pipelines for a given schema.
func DeleteCacheForSchema(ctx context.Context, schemaName string, container *models.ContainerModel) error {
    // Generate keys for routes
    keyGetAll, _ := GenerateRedisKey("GetAllDynamicModelItems", schemaName, container)
    keyGetItem, _ := GenerateRedisKey("GetDynamicModelItem", schemaName, container)

    // Delete the specific keys
    if _, err := configs.RedisClient.Del(ctx, keyGetAll).Result(); err != nil {
        return fmt.Errorf("failed to delete key %s: %v", keyGetAll, err)
    }

    if _, err := configs.RedisClient.Del(ctx, keyGetItem).Result(); err != nil {
        return fmt.Errorf("failed to delete key %s: %v", keyGetItem, err)
    }
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
