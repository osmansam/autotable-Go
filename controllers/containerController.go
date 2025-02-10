package controllers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-playground/validator"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type DynamicFunctionsUpdate struct {
    DynamicFunctions []models.DynamicFunction `json:"DynamicFunctions"`
}
type PipelinesUpdate struct {
    Pipelines []models.PipelineStage `json:"Pipelines"`
}
var containerCollection *mongo.Collection = configs.GetCollection( "containers")
var validate = validator.New()

// CreateContainer creates a container with the provided model name and schema fields
func CreateContainer(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var container models.ContainerModel

	log.Println("Parsing request body for CreateContainer")
	if err := c.BodyParser(&container); err != nil {
		log.Printf("Failed to parse request body: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to parse the request body. Ensure the provided JSON is valid.")
	}

	log.Println("Validating parsed data for CreateContainer")
	if validationErr := validate.Struct(&container); validationErr != nil {
		log.Printf("Validation error: %v", validationErr)
		return utils.SendErrorResponse(c, validationErr, "Validation error. Some required fields might be missing or have invalid values.")
	}

	log.Println("Checking if container already exists in the database")
	count, err := containerCollection.CountDocuments(ctx, bson.M{"schemaName": container.SchemaName})
	if err != nil {
		log.Printf("Database query error: %v", err)
		return utils.SendErrorResponse(c, err, "Unable to query the container model from the database.")
	}

	if count != 0 {
		log.Println("Container already exists in the database")
		return &fiber.Error{
			Code:    http.StatusNotFound,
			Message: "The specified schema already exists in containers",
		}
	}

	newContainer := models.ContainerModel{
		SchemaName: container.SchemaName,
		Fields:     container.Fields,
		Routes:     container.Routes,
		Redis:      container.Redis,
		Pipelines:  container.Pipelines,
	}

	log.Println("Inserting new container into the database")
	result, err := containerCollection.InsertOne(ctx, newContainer)
	if err != nil {
		log.Printf("Failed to insert container: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to insert the container into the database. Please try again later.")
	}

	log.Println("Container successfully created")
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

	log.Println("Retrieving all containers from the database")
	results, err := containerCollection.Find(ctx, bson.M{})
	if err != nil {
		log.Printf("Failed to retrieve containers: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to retrieve containers from the database. Please try again later.")
	}
	defer results.Close(ctx)


	for results.Next(ctx) {
		var singleContainer models.ContainerModel
		if err = results.Decode(&singleContainer); err != nil {
			log.Printf("Error decoding container: %v", err)
			return utils.SendErrorResponse(c, err, "An error occurred while processing the retrieved containers. Please try again later.")
		}

		containers = append(containers, singleContainer)
	}
	if err != nil {
		return utils.SendResponse(c, http.StatusInternalServerError, "An error occurred while processing the retrieved containers. Please try again later.", err.Error())
	}

	log.Println("Containers successfully retrieved")
	return utils.SendResponse(c, http.StatusOK, "Containers successfully retrieved.", containers)
}

// Delete a container
func DeleteContainer(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	deleteIdStr := c.Params("id")
	deleteId, err := primitive.ObjectIDFromHex(deleteIdStr)
	if err != nil {
		log.Printf("Invalid ID format: %v", err)
		return utils.SendErrorResponse(c, err, "Invalid ID format provided. Please ensure the ID is a valid MongoDB ObjectID.")
	}

	log.Println("Attempting to delete container from the database")
	result, err := containerCollection.DeleteOne(ctx, bson.M{"_id": deleteId})
	if err != nil {
		log.Printf("Failed to delete container: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to delete the container from the database. Please try again later.")
	}

	log.Println("Container successfully deleted")
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

	log.Println("Parsing request body for UpdateContainer")
	if err := c.BodyParser(&updatedContainer); err != nil {
		log.Printf("Failed to parse request body: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to parse the request body. Ensure the provided JSON is valid.")
	}

	log.Println("Validating parsed data for UpdateContainer")
	if validationErr := validate.Struct(&updatedContainer); validationErr != nil {
		log.Printf("Validation error: %v", validationErr)
		return utils.SendErrorResponse(c, validationErr, "Validation error. Some required fields might be missing or have invalid values.")
	}

	updateIdStr := c.Params("id")
	updateId, err := primitive.ObjectIDFromHex(updateIdStr)
	if err != nil {
		log.Printf("Invalid ID format: %v", err)
		return utils.SendErrorResponse(c, err, "Provided ID is not in the valid format.")
	}

	log.Println("Checking for existing container with the same schema name")
	var existingContainer models.ContainerModel
	err = containerCollection.FindOne(ctx, bson.M{"schemaName": updatedContainer.SchemaName, "_id": bson.M{"$ne": updateId}}).Decode(&existingContainer)
	if err == nil {
		log.Println("Another container with the specified schema name already exists")
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Another container with the specified schema name already exists.",
			Data:    nil,
		})
	}
	if err != mongo.ErrNoDocuments {
		log.Printf("Database error: %v", err)
		return utils.SendErrorResponse(c, err, "Database error occurred while checking for existing schema name.")
	}

	updatedContainer.Pipelines = existingContainer.Pipelines
	updatedContainer.DynamicFunctions = existingContainer.DynamicFunctions

	log.Println("Updating container in the database")
	updateResult, err := containerCollection.UpdateOne(ctx, bson.M{"_id": updateId}, bson.M{"$set": updatedContainer})
	if err != nil {
		log.Printf("Failed to update container: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to update the item in the database. Please try again later.")
	}

	if updateResult.MatchedCount == 0 {
		log.Println("No container found with the specified schema name")
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
			Status:  http.StatusNotFound,
			Message: "No container found with the specified schema name.",
			Data:    nil,
		})
	}

	log.Println("Container successfully updated")
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Container successfully updated.",
		Data:    &fiber.Map{"data": updateResult},
	})
}

// UpdatePipelines updates the Pipelines of a specific container
func UpdatePipelines(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    containerIdStr := c.Params("id")
    containerId, err := primitive.ObjectIDFromHex(containerIdStr)
    if err != nil {
        log.Printf("Invalid container ID format: %v", err)
        return utils.SendErrorResponse(c, err, "Invalid container ID format")
    }

    log.Println("Parsing request body for UpdatePipelines")
    var update PipelinesUpdate
    if err := c.BodyParser(&update); err != nil {
        log.Printf("Failed to parse request body: %v", err)
        return utils.SendErrorResponse(c, err, "Failed to parse request body")
    }

    log.Println("Updating Pipelines in the container")
    updateResult, err := containerCollection.UpdateOne(
        ctx, 
        bson.M{"_id": containerId}, 
        bson.M{"$set": bson.M{"pipelines": update.Pipelines}},
    )
    if err != nil {
        log.Printf("Failed to update Pipelines: %v", err)
        return utils.SendErrorResponse(c, err, "Failed to update Pipelines")
    }

    if updateResult.MatchedCount == 0 {
        log.Println("No container found with the specified ID")
        return c.Status(http.StatusNotFound).JSON(fiber.Map{
            "status":  http.StatusNotFound,
            "message": "No container found with the specified ID",
        })
    }

    log.Println("Pipelines successfully updated")
    return c.Status(http.StatusOK).JSON(fiber.Map{
        "status":  http.StatusOK,
        "message": "Pipelines successfully updated",
        "data":    updateResult,
    })
}

// Update the DynamicFunctions in the container
func UpdateDynamicFunctions(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    containerIdStr := c.Params("id")
    containerId, err := primitive.ObjectIDFromHex(containerIdStr)
    if err != nil {
        log.Printf("Invalid container ID format: %v", err)
        return utils.SendErrorResponse(c, err, "Invalid container ID format")
    }

    log.Println("Parsing request body for UpdateDynamicFunctions")
    var update DynamicFunctionsUpdate
    if err := c.BodyParser(&update); err != nil {
        log.Printf("Failed to parse request body: %v", err)
        return utils.SendErrorResponse(c, err, "Failed to parse request body")
    }

    log.Println("Updating DynamicFunctions in the container")
    updateResult, err := containerCollection.UpdateOne(
        ctx, 
        bson.M{"_id": containerId}, 
        bson.M{"$set": bson.M{"dynamicFunctions": update.DynamicFunctions}},
    )
    if err != nil {
        log.Printf("Failed to update DynamicFunctions: %v", err)
        return utils.SendErrorResponse(c, err, "Failed to update DynamicFunctions")
    }

    if updateResult.MatchedCount == 0 {
        log.Println("No container found with the specified ID")
        return c.Status(http.StatusNotFound).JSON(fiber.Map{
            "status":  http.StatusNotFound,
            "message": "No container found with the specified ID",
        })
    }

    log.Println("DynamicFunctions successfully updated")
    return c.Status(http.StatusOK).JSON(fiber.Map{
        "status":  http.StatusOK,
        "message": "DynamicFunctions successfully updated",
        "data":    updateResult,
    })
}

// GetContainer retrieves a single container from the database based on its ID
func GetContainer(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	containerIdStr := c.Params("id")
	containerId, err := primitive.ObjectIDFromHex(containerIdStr)
	if err != nil {
		log.Printf("Invalid ID format: %v", err)
		return utils.SendErrorResponse(c, err, "Provided ID is not in the valid format.")
	}

	log.Println("Fetching container from the database")
	var container models.ContainerModel
	err = containerCollection.FindOne(ctx, bson.M{"_id": containerId}).Decode(&container)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Println("No container found with the specified ID")
			return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
				Status:  http.StatusNotFound,
				Message: "No container found with the specified ID.",
				Data:    nil,
			})
		}
		log.Printf("Failed to retrieve container: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to retrieve the container from the database. Please try again later.")
	}

	log.Println("Container successfully retrieved")
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Container successfully retrieved.",
		Data:    &fiber.Map{"data": container},
	})
}
