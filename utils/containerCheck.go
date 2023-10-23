package utils

import (
	"context"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// IsSchemaInContainers checks if a schema with the provided name exists in the containers collection.
func IsSchemaInContainers(ctx context.Context, containerCollection *mongo.Collection, schemaName string) *fiber.Error {
	// Count the documents matching the provided schemaName
	count, err := containerCollection.CountDocuments(ctx, bson.M{"schemaName": schemaName})
	if err != nil {
		return &fiber.Error{
			Code:    http.StatusInternalServerError,
			Message: "Unable to query the container model from the database.",
		}
	}
	
	// If no matching schema is found
	if count == 0 {
		return &fiber.Error{
			Code:    http.StatusNotFound,
			Message: "The specified schema does not exist in containers.",
		}
	}

	return nil
}
