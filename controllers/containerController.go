package controllers

import (
	"context"
	"net/http"
	"time"

	"github.com/go-playground/validator"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var containerCollection *mongo.Collection = configs.GetCollection(configs.DB, "containers")
var validate = validator.New()

// CreateContainer creates a container with the provided model name and schema fields
func CreateContainer(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var container models.ContainerModel

	// Parse the body into the container struct
	if err := c.BodyParser(&container); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse the request body. Ensure the provided JSON is valid.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Validate the parsed data using the validator library
	if validationErr := validate.Struct(&container); validationErr != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Validation error. Some required fields might be missing or have invalid values.",
			Data:    &fiber.Map{"data": validationErr.Error()},
		})
	}
	//check if the container is already in the database
		count, err := containerCollection.CountDocuments(ctx, bson.M{"schemaName": container.SchemaName})
	if err != nil {
		return &fiber.Error{
			Code:    http.StatusInternalServerError,
			Message: "Unable to query the container model from the database.",
		}
	}
	
	// If no matching schema is found
	if count != 0 {
		return &fiber.Error{
			Code:    http.StatusNotFound,
			Message: "The specified schema already exists in containers",
		}
	}

	newContainer := models.ContainerModel{
		SchemaName: container.SchemaName,
		Fields:     container.Fields,
		Routes: container.Routes,
		Redis: container.Redis,
		Pipelines: container.Pipelines,
	}

	// Insert the container into the database
	result, err := containerCollection.InsertOne(ctx, newContainer)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to insert the container into the database. Please try again later.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	return c.Status(http.StatusCreated).JSON(responses.GeneralResponse{
		Status:  http.StatusCreated,
		Message: "Container successfully created.",
		Data:    &fiber.Map{"data": result},
	})
}

// GetAllContainers retrieves all containers from the database
func GetAllContainers(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var containers []models.ContainerModel

	// Retrieve all containers from the database
	results, err := containerCollection.Find(ctx, bson.M{})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve containers from the database. Please try again later.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}
	defer results.Close(ctx)

	// Decode each retrieved container
	for results.Next(ctx) {
		var singleContainer models.ContainerModel
		if err = results.Decode(&singleContainer); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
				Status:  http.StatusInternalServerError,
				Message: "An error occurred while processing the retrieved containers. Please try again later.",
				Data:    &fiber.Map{"data": err.Error()},
			})
		}

		containers = append(containers, singleContainer)
	}

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Containers successfully retrieved.",
		Data:    &fiber.Map{"data": containers},
	})
}
// Delete a container
func DeleteContainer(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Convert string ID to ObjectID
	deleteIdStr := c.Params("id")
	deleteId, err := primitive.ObjectIDFromHex(deleteIdStr)

	// Handle errors from ID conversion
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid ID format provided. Please ensure the ID is a valid MongoDB ObjectID.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Attempt to delete the container from the database
	result, err := containerCollection.DeleteOne(ctx, bson.M{"_id": deleteId})

	// Handle errors from deletion process
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete the container from the database. Please try again later.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Successful deletion
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Container successfully deleted.",
		Data:    &fiber.Map{"data": result},
	})
}

// UpdateContainer updates an existing container's details based on the provided schema name and new details
func UpdateContainer(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var updatedContainer models.ContainerModel

	// Parse the body into the updatedContainer struct
	if err := c.BodyParser(&updatedContainer); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse the request body. Ensure the provided JSON is valid.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Validate the parsed data using the validator library
	if validationErr := validate.Struct(&updatedContainer); validationErr != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Validation error. Some required fields might be missing or have invalid values.",
			Data:    &fiber.Map{"data": validationErr.Error()},
		})
	}

	// Get the ID of the item to be updated from the params and attempt to convert it to ObjectID
	updateIdStr := c.Params("id")
	updateId, err := primitive.ObjectIDFromHex(updateIdStr)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Provided ID is not in the valid format.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}
  // Check if any other container has the same schemaName
    var existingContainer models.ContainerModel
    err = containerCollection.FindOne(ctx, bson.M{"schemaName": updatedContainer.SchemaName, "_id": bson.M{"$ne": updateId}}).Decode(&existingContainer)
    if err == nil {
        // A container with the same schemaName and a different ID exists
        return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
            Status: http.StatusBadRequest,
            Message: "Another container with the specified schema name already exists.",
            Data: nil,
        })
    }
    if err != mongo.ErrNoDocuments {
        // Handle database errors other than 'No Documents'
        return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status: http.StatusInternalServerError,
            Message: "Database error occurred while checking for existing schema name.",
            Data: &fiber.Map{"data": err.Error()},
        })
    }
	// Update the validated item in the collection
	updateResult, err := containerCollection.UpdateOne(ctx, bson.M{"_id": updateId}, bson.M{"$set": updatedContainer})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update the item in the database. Please try again later.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	if updateResult.MatchedCount == 0 {
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
			Status:  http.StatusNotFound,
			Message: "No container found with the specified schema name.",
			Data:    nil,
		})
	}

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Container successfully updated.",
		Data:    &fiber.Map{"data": updateResult},
	})
}

// GetContainer retrieves a single container from the database based on its ID
func GetContainer(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Retrieve container ID from URL parameters
	containerIdStr := c.Params("id")
	containerId, err := primitive.ObjectIDFromHex(containerIdStr)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Provided ID is not in the valid format.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Fetch the container with the provided ID from the database
	var container models.ContainerModel
	err = containerCollection.FindOne(ctx, bson.M{"_id": containerId}).Decode(&container)
	if err != nil {
		// If the item is not found, return a 404 status
		if err == mongo.ErrNoDocuments {
			return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
				Status:  http.StatusNotFound,
				Message: "No container found with the specified ID.",
				Data:    nil,
			})
		}
		// For other errors, return a 500 status
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve the container from the database. Please try again later.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Container successfully retrieved.",
		Data:    &fiber.Map{"data": container},
	})
}
