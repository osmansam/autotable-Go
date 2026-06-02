package utils

import (
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
)

// GetProjectCollection returns a MongoDB collection for a specific project
// Collection naming pattern: tenant_{tenantId}_project_{projectId}_{collectionName}
func GetProjectCollection(tenantID, projectID, collectionName string) *mongo.Collection {
	fullCollectionName := fmt.Sprintf("tenant_%s_project_%s_%s", tenantID, projectID, collectionName)
	return globalCollectionProvider(fullCollectionName)
}

// GetContainerCollectionForProject returns the containers metadata collection for a project
// If tenantID and projectID are empty, returns the legacy global "containers" collection
func GetContainerCollectionForProject(tenantID, projectID string) *mongo.Collection {
	if tenantID == "" || projectID == "" {
		// Legacy mode: use global containers collection
		return globalCollectionProvider("containers")
	}
	return GetProjectCollection(tenantID, projectID, "containers")
}

// GetDynamicCollectionForProject returns a dynamic data collection for a project
// This is where actual data (like "users", "products") is stored
// If tenantID and projectID are empty, returns the legacy global collection
func GetDynamicCollectionForProject(tenantID, projectID, schemaName string) *mongo.Collection {
	if tenantID == "" || projectID == "" {
		// Legacy mode: use global collection
		return globalCollectionProvider(schemaName)
	}
	return GetProjectCollection(tenantID, projectID, schemaName)
}

// GetPageCollectionForProject returns the pages collection for a project
// If tenantID and projectID are empty, returns the legacy global "pages" collection
func GetPageCollectionForProject(tenantID, projectID string) *mongo.Collection {
	if tenantID == "" || projectID == "" {
		// Legacy mode: use global pages collection
		return globalCollectionProvider("pages")
	}
	return GetProjectCollection(tenantID, projectID, "pages")
}

// GetProjectCollectionName returns the full collection name for a project resource
func GetProjectCollectionName(tenantID, projectID, resourceName string) string {
	return fmt.Sprintf("tenant_%s_project_%s_%s", tenantID, projectID, resourceName)
}
