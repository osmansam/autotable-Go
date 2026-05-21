package utils

import (
	"context"

	"github.com/osmansam/autotableGo/models"
)

// DeleteCacheForSchema invalidates a schema cache by incrementing its version.
// Old versioned cache keys are ignored and expire naturally by TTL.
func DeleteCacheForSchema(ctx context.Context, tenantID, projectID, schemaName string, container *models.ContainerModel) error {
	return IncrementSchemaCacheVersion(ctx, tenantID, projectID, schemaName)
}
