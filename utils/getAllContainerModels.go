package utils

import (
	"context"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
)

// GetAllContainerModels retrieves all container models from the database
func GetAllContainerModels() ([]models.ContainerModel, error) {
	return GetAllContainerModelsWithContext(context.Background())
}

func GetAllContainerModelsWithContext(ctx context.Context) ([]models.ContainerModel, error) {
	var containers []models.ContainerModel

	// Setting up the MongoDB collection
	containerCollection := globalCollectionProvider("containers")

	// Context to manage timeout
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Fetching all documents
	cursor, err := containerCollection.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	// Decoding documents into container models
	for cursor.Next(ctx) {
		var container models.ContainerModel
		err := cursor.Decode(&container)
		if err != nil {
			return nil, err
		}
		containers = append(containers, container)
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return containers, nil
}
