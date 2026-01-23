package utils

import (
	"github.com/osmansam/autotableGo/models"
)

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
