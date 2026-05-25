package utils

import (
	"context"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
)

// GetContainerModel fetches container model for a project-specific collection
// This requires tenantID and projectID to access the correct containers metadata
func GetContainerModel(tenantID, projectID, schemaName string) (*models.ContainerModel, error) {
	return GetContainerModelWithContext(context.Background(), tenantID, projectID, schemaName)
}

func GetContainerModelWithContext(ctx context.Context, tenantID, projectID, schemaName string) (*models.ContainerModel, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Get project-specific container collection
	containerCollection := GetContainerCollectionForProject(tenantID, projectID)

	var containerModel models.ContainerModel
	err := containerCollection.FindOne(ctx, bson.M{"schemaName": schemaName}).Decode(&containerModel)
	if err != nil {
		return nil, err
	}

	// Sort fields by order before returning
	containerModel.SortFieldsByOrder()

	return &containerModel, nil
}

// GetContainerModelLegacy fetches container model from the legacy global containers collection
// Used by non-tenant routes (e.g., dynamic auth routes) that don't have tenant context
// DEPRECATED: This is for backward compatibility only. New code should use GetContainerModel with tenantID/projectID
func GetContainerModelLegacy(schemaName string) (*models.ContainerModel, error) {
	// For legacy routes without tenant context, use empty strings which will result in "containers" collection
	return GetContainerModel("", "", schemaName)
}
