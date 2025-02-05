package utils

import (
	"context"
	"time"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
)

// GetAllContainerModels retrieves all container models from the database
func GetAllContainerModels() ([]models.ContainerModel, error) {
	var containers []models.ContainerModel

	// Setting up the MongoDB collection
	containerCollection := configs.GetCollection( "containers")

	// Context to manage timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
