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
	return d.invalidateWriteCaches(ctx, tenantID, projectID, schemaName, container)
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
