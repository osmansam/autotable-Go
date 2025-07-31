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
		shouldCache = container.Redis.IsRedisCached
	case "GetDynamicModelItem":
			shouldCache = container.Redis.IsRedisCached
	}
	if shouldCache {
		if len(id) > 0 && id[0] != "" {
			redisKey = "item_" + id[0] + "_schema_" + schemaName + "_route_" + routeName
		} else {
			redisKey = "schema_" + schemaName + "_route_" + routeName
		}
	}

	return redisKey, shouldCache
}

// generatePipelineRedisKey generates a Redis key for caching pipelines based on schema name and pipeline name.
func GeneratePipelineRedisKey(schemaName, pipelineName string, container *models.ContainerModel) (string, bool) {
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
		redisKey = "pipeline_" + pipelineName + "_schema_" + schemaName
	}

	return redisKey, shouldCache
}

// generateDynamicFunctionRedisKey generates a Redis key for caching dynamic functions based on schema name and function name.
func GenerateDynamicFunctionRedisKey(schemaName, functionName string, container *models.ContainerModel) (string, bool) {
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
		redisKey = "function_" + functionName + "_schema_" + schemaName
	}

	return redisKey, shouldCache
}

// generateDynamicApiRedisKey generates a Redis key for caching dynamic APIs based on schema name and API name.
func GenerateDynamicApiRedisKey(schemaName, apiName string, container *models.ContainerModel) (string, bool) {
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
        redisKey = "api_" + apiName + "_schema_" + schemaName
    }

    return redisKey, shouldCache
}
