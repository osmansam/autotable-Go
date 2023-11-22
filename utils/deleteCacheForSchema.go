package utils

import (
	"context"
	"fmt"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
)

// DeleteCacheForSchema deletes cache related to specific routes for a given schema.
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

    return nil
}
