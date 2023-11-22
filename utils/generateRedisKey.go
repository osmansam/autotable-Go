package utils

import (
	"github.com/osmansam/autotableGo/models"
)

// GenerateRedisKey generates a Redis key for caching based on route, schema names, and an optional ID.
func GenerateRedisKey(routeName, schemaName string, container *models.ContainerModel, id ...string) (string, bool) {
	var redisKey string
	var shouldCache bool

	// Check if caching should be applied based on the IsRedisCached flag
	switch routeName {
	case "GetAllDynamicModelItems":
		shouldCache = container.Routes.GetAllDynamicModelItems.IsRedisCached
	case "GetDynamicModelItem":
		shouldCache = container.Routes.GetDynamicModelItem.IsRedisCached
	}

	if shouldCache {
		redisKey = "route_" + routeName + "_schema_" + schemaName
		if len(id) > 0 && id[0] != "" {
			redisKey += "_id_" + id[0]
		}
	}

	return redisKey, shouldCache
}
