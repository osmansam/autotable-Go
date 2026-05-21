package cache

import (
	"context"
	"log"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
)

type DynamicCache struct{}

func NewDynamicCache() *DynamicCache {
	return &DynamicCache{}
}

func (d *DynamicCache) InvalidateCreateCaches(ctx context.Context, tenantID, projectID, schemaName string, container *models.ContainerModel) error {
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
