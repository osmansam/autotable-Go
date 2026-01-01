package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-playground/validator"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
	"github.com/osmansam/autotableGo/ws"
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
var validate = validator.New()

// ensureRoleSchemaExists checks if a "role" schema exists and creates it if not
func ensureRoleSchemaExists(ctx context.Context, containerCollection *mongo.Collection) error {
	// Check if role schema already exists
	count, err := containerCollection.CountDocuments(ctx, bson.M{"schemaName": "role"})
	if err != nil {
		log.Printf("Error checking for role schema: %v", err)
		return err
	}

	// If role schema already exists, nothing to do
	if count > 0 {
		log.Println("Role schema already exists, skipping creation")
		return nil
	}

	// Create the role schema
	log.Println("Creating role schema automatically")
	roleContainer := models.ContainerModel{
		SchemaName: "role",
		Fields: []models.Field{
			{
				Name: "name",
				Type: "string",
				Tag:  "required",
			},
		},
		Routes: models.Routes{
			CreateDynamicModelItem:                models.RouteSpec{IsActive: true, Method: "POST"},
			GetAllDynamicModelItems:               models.RouteSpec{IsActive: true, Method: "GET"},
			CreateMultipleDynamicModelItem:        models.RouteSpec{IsActive: true, Method: "POST"},
			GetAllDynamicModelItemsWithPagination: models.RouteSpec{IsActive: true, Method: "GET"},
			GetPipeline:                           models.RouteSpec{IsActive: true, Method: "GET"},
			TestPipeline:                          models.RouteSpec{IsActive: true, Method: "POST"},
			HandleSearchDynamicModelItem:          models.RouteSpec{IsActive: true, Method: "GET"},
			HandleFilterDynamicModelItem:          models.RouteSpec{IsActive: true, Method: "GET"},
			DeleteDynamicModelItem:                models.RouteSpec{IsActive: true, Method: "DELETE"},
			UpdateDynamicModelItem:                models.RouteSpec{IsActive: true, Method: "PUT"},
			UpdateMultipleDynamicModelItem:        models.RouteSpec{IsActive: true, Method: "PUT"},
			GetDynamicModelItem:                   models.RouteSpec{IsActive: true, Method: "GET"},
			DeleteMultipleDynamicModelItem:        models.RouteSpec{IsActive: true, Method: "DELETE"},
			ExportDynamicModelItems:               models.RouteSpec{IsActive: true, Method: "GET"},
			GetItemsForSelection:                  models.RouteSpec{IsActive: true, Method: "GET"},
		},
		Redis: models.Redis{
			IsRedisCached:        false,
			CacheTime:            0,
			TriggeredRedisCaches: []string{},
		},
		IsAuthContainer: false,
		PopulatedRoutes: []string{},
		Pipelines:       []models.PipelineStage{},
		DynamicFunctions: []models.DynamicFunction{},
		DynamicApis:     []models.DynamicApiModel{},
		Indexes:         []models.Index{},
	}

	// Insert the role container
	_, err = containerCollection.InsertOne(ctx, roleContainer)
	if err != nil {
		log.Printf("Failed to create role schema: %v", err)
		return err
	}

	log.Println("Role schema successfully created")
	return nil
}

// CreateContainer creates a container with the provided model name and schema fields
func CreateContainer(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Extract tenant and project context from URL slugs with JWT validation
	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil {
		log.Printf("Failed to get project context: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to get project context: "+err.Error())
	}
	if tenantID == "" || projectID == "" {
		log.Println("Missing tenant or project context")
		return utils.SendErrorResponse(c, nil, "Missing tenant or project context. Please ensure you are authenticated and have switched to a project.")
	}

	// Get project-specific container collection
	containerCollection := utils.GetContainerCollectionForProject(tenantID, projectID)

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

	// Check if the schema name is in the restricted schema names list
	for _, restrictedName := range models.RestrictedSchemaNames {
		if container.SchemaName == restrictedName {
			log.Println("Schema name is restricted and cannot be used")
			return utils.SendErrorResponse(c, nil, "The specified schema name is restricted and cannot be used.")
		}
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
	// Validate isAuthContainer requirements
	if container.IsAuthContainer {
		log.Println("Validating isAuthContainer requirements")
		
		// Check if another auth container already exists
		authCount, err := containerCollection.CountDocuments(ctx, bson.M{"isAuthContainer": true})
		if err != nil {
			log.Printf("Database error when checking for existing auth container: %v", err)
			return utils.SendErrorResponse(c, err, "Database error while checking for existing auth container.")
		}
		if authCount > 0 {
			log.Println("Another auth container already exists")
			return utils.SendErrorResponse(c, nil, "Only one container can have isAuthContainer set to true. An auth container already exists.")
		}
		
		// Validate that there's an email field with isLoginCredential true
		hasValidEmailField := false
		for _, field := range container.Fields {
			if field.Name == "email" && field.IsLoginCredential {
				hasValidEmailField = true
				break
			}
		}
		if !hasValidEmailField {
			log.Println("Auth container missing required email field with isLoginCredential")
			return utils.SendErrorResponse(c, nil, "Auth container must have a field named 'email' with isLoginCredential set to true.")
		}
	
		// Ensure role schema exists when creating an auth container
		if err := ensureRoleSchemaExists(ctx, containerCollection); err != nil {
			log.Printf("Failed to ensure role schema exists: %v", err)
			return utils.SendErrorResponse(c, err, "Failed to create role schema.")
		}
	}
	
	// Validate that objectId fields reference an existing container (not the one being created)
	for _, field := range container.Fields {
		if field.Type == "objectId" {
			// Ensure the field's name is not the same as the container being created.
			if field.ObjectSchemaName == container.SchemaName {
				log.Println("ObjectId field cannot reference the container being created")
				return utils.SendErrorResponse(c, nil, "Field with type 'objectId' must reference an already defined container name, not the one being created.")
			}
			// Optionally, verify that the referenced container exists in the database.
			count, err := containerCollection.CountDocuments(ctx, bson.M{"schemaName": field.ObjectSchemaName})
			if err != nil {
				log.Printf("Database error when verifying objectId reference: %v", err)
				return utils.SendErrorResponse(c, err, "Database error while verifying objectId reference.")
			}
			if count == 0 {
				log.Printf("Referenced container %s does not exist", field.ObjectSchemaName)
				return utils.SendErrorResponse(c, nil, "Field with type 'objectId' must reference a valid, existing container name.")
			}
		}
	}
	newContainer := models.ContainerModel{
		SchemaName:      container.SchemaName,
		Fields:          container.Fields,
		Routes:          container.Routes,
		Redis:           container.Redis,
		Pipelines:       container.Pipelines,
		IsAuthContainer: container.IsAuthContainer,
		PopulatedRoutes: container.PopulatedRoutes,
		DynamicApis:     container.DynamicApis,
		DynamicFunctions: container.DynamicFunctions,
		Indexes:         container.Indexes,
	}

	log.Println("Inserting new container into the database")
	result, err := containerCollection.InsertOne(ctx, newContainer)
	if err != nil {
		log.Printf("Failed to insert container: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to insert the container into the database. Please try again later.")
	}

	// Create indexes for the new container
	if err := utils.EnsureIndexes(ctx, &newContainer, tenantID, projectID); err != nil {
		log.Printf("Warning: Failed to create indexes for schema %s: %v", newContainer.SchemaName, err)
		// Don't fail the request, just log the warning
	}

	// Invalidate Redis cache for all containers (project-specific)
	configs.RedisClient.Del(ctx, fmt.Sprintf("containers:all:tenant_%s:project_%s", tenantID, projectID))
	log.Println("Invalidated containers cache after creation")

	// Emit WebSocket event for container change
	userIDStr, _ := c.Locals("userID").(string)
	ws.EmitContainerChanged(userIDStr)

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

	// Extract tenant and project context from URL slugs (falls back to query params or JWT for backward compatibility)
	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil || tenantID == "" || projectID == "" {
		log.Println("Missing tenant or project context")
		return utils.SendErrorResponse(c, err, "Missing tenant or project context.")
	}

	// Get project-specific container collection
	containerCollection := utils.GetContainerCollectionForProject(tenantID, projectID)

	// Try to get from Redis cache first (project-specific key)
	redisKey := fmt.Sprintf("containers:all:tenant_%s:project_%s", tenantID, projectID)
	if cachedData, err := configs.RedisClient.Get(ctx, redisKey).Result(); err == nil {
		var containers []models.ContainerModel
		if json.Unmarshal([]byte(cachedData), &containers) == nil {
			log.Println("Fetched all containers from Redis cache")
			return c.JSON(containers)
		}
	}

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

	// Cache the result in Redis (30 minutes TTL)
	if payload, err := json.Marshal(containers); err == nil {
		configs.RedisClient.Set(ctx, redisKey, payload, 30*time.Minute)
		log.Println("Cached all containers in Redis")
	}

	log.Println("Containers successfully retrieved from database")
	return c.JSON(containers)
}

// Delete a container
func DeleteContainer(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Extract tenant and project context from URL slugs with JWT validation
	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil {
		log.Printf("Failed to get project context: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to get project context: "+err.Error())
	}
	if tenantID == "" || projectID == "" {
		log.Println("Missing tenant or project context")
		return utils.SendErrorResponse(c, nil, "Missing tenant or project context.")
	}

	// Get project-specific container collection
	containerCollection := utils.GetContainerCollectionForProject(tenantID, projectID)

	deleteIdStr := c.Params("id")
	deleteId, err := primitive.ObjectIDFromHex(deleteIdStr)

	log.Println("Fetching container from the database")
	var container models.ContainerModel
	err = containerCollection.FindOne(ctx, bson.M{"_id": deleteId}).Decode(&container)
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

	// Drop indexes for the container (before dropping collection)
	projectCollectionName := utils.GetProjectCollectionName(tenantID, projectID, container.SchemaName)
	if err := utils.DropIndexes(ctx, projectCollectionName); err != nil {
		log.Printf("Warning: Failed to drop indexes for schema %s: %v", projectCollectionName, err)
	}

	// Drop the dynamic collection associated with the container.
	log.Printf("Dropping collection '%s' from the database", projectCollectionName)
	err = containerCollection.Database().Collection(projectCollectionName).Drop(ctx)
	if err != nil {
		log.Printf("Failed to drop collection '%s': %v", projectCollectionName, err)
		return utils.SendErrorResponse(c, err, "Container deleted but failed to drop the corresponding collection.")
	}

	// Invalidate Redis cache for all containers and this specific container (project-specific)
	configs.RedisClient.Del(ctx, fmt.Sprintf("containers:all:tenant_%s:project_%s", tenantID, projectID))
	configs.RedisClient.Del(ctx, fmt.Sprintf("container:%s:tenant_%s:project_%s", deleteIdStr, tenantID, projectID))
	log.Println("Invalidated containers cache after deletion")

	// Emit WebSocket event for container change
	userIDStr, _ := c.Locals("userID").(string)
	ws.EmitContainerChanged(userIDStr)

	log.Println("Container and its corresponding collection successfully deleted")
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

	// Extract tenant and project context from URL slugs with JWT validation
	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil {
		log.Printf("Failed to get project context: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to get project context: "+err.Error())
	}
	if tenantID == "" || projectID == "" {
		log.Println("Missing tenant or project context")
		return utils.SendErrorResponse(c, nil, "Missing tenant or project context.")
	}

	// Get project-specific container collection
	containerCollection := utils.GetContainerCollectionForProject(tenantID, projectID)

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

	// Validate isAuthContainer requirements
	if updatedContainer.IsAuthContainer {
		log.Println("Validating isAuthContainer requirements for update")
		
		// Check if another auth container already exists (excluding the current one being updated)
		authCount, err := containerCollection.CountDocuments(ctx, bson.M{"isAuthContainer": true, "_id": bson.M{"$ne": updateId}})
		if err != nil {
			log.Printf("Database error when checking for existing auth container: %v", err)
			return utils.SendErrorResponse(c, err, "Database error while checking for existing auth container.")
		}
		if authCount > 0 {
			log.Println("Another auth container already exists")
			return utils.SendErrorResponse(c, nil, "Only one container can have isAuthContainer set to true. An auth container already exists.")
		}
		
		// Validate that there's an email field with isLoginCredential true
		hasValidEmailField := false
		for _, field := range updatedContainer.Fields {
			if field.Name == "email" && field.IsLoginCredential {
				hasValidEmailField = true
				break
			}
		}
		if !hasValidEmailField {
			log.Println("Auth container missing required email field with isLoginCredential")
			return utils.SendErrorResponse(c, nil, "Auth container must have a field named 'email' with isLoginCredential set to true.")
		}
		
		// Ensure role schema exists when updating to an auth container
		if err := ensureRoleSchemaExists(ctx, containerCollection); err != nil {
			log.Printf("Failed to ensure role schema exists: %v", err)
			return utils.SendErrorResponse(c, err, "Failed to create role schema.")
		}
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

	// Rebuild indexes for the updated container
	if err := utils.RebuildIndexes(ctx, &updatedContainer, tenantID, projectID); err != nil {
		log.Printf("Warning: Failed to rebuild indexes for schema %s: %v", updatedContainer.SchemaName, err)
		// Don't fail the request, just log the warning
	}

	// Invalidate Redis cache for all containers and this specific container (project-specific)
	configs.RedisClient.Del(ctx, fmt.Sprintf("containers:all:tenant_%s:project_%s", tenantID, projectID))
	configs.RedisClient.Del(ctx, fmt.Sprintf("container:%s:tenant_%s:project_%s", updateIdStr, tenantID, projectID))
	log.Println("Invalidated containers cache after update")

	// Emit WebSocket event for container change
	userIDStr, _ := c.Locals("userID").(string)
	ws.EmitContainerChanged(userIDStr)

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

    // Extract tenant and project context from URL slugs with JWT validation
    tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
    if err != nil {
        log.Printf("Failed to get project context: %v", err)
        return utils.SendErrorResponse(c, err, "Failed to get project context: "+err.Error())
    }
    if tenantID == "" || projectID == "" {
        log.Println("Missing tenant or project context")
        return utils.SendErrorResponse(c, nil, "Missing tenant or project context.")
    }

    // Get project-specific container collection
    containerCollection := utils.GetContainerCollectionForProject(tenantID, projectID)

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

    // Invalidate Redis cache for all containers and this specific container (project-specific)
    configs.RedisClient.Del(ctx, fmt.Sprintf("containers:all:tenant_%s:project_%s", tenantID, projectID))
    configs.RedisClient.Del(ctx, fmt.Sprintf("container:%s:tenant_%s:project_%s", containerIdStr, tenantID, projectID))
    log.Println("Invalidated containers cache after pipeline update")

    // Emit WebSocket event for container change
    userIDStr, _ := c.Locals("userID").(string)
    ws.EmitContainerChanged(userIDStr)

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

    // Extract tenant and project context from URL slugs with JWT validation
    tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
    if err != nil {
        log.Printf("Failed to get project context: %v", err)
        return utils.SendErrorResponse(c, err, "Failed to get project context: "+err.Error())
    }
    if tenantID == "" || projectID == "" {
        log.Println("Missing tenant or project context")
        return utils.SendErrorResponse(c, nil, "Missing tenant or project context.")
    }

    // Get project-specific container collection
    containerCollection := utils.GetContainerCollectionForProject(tenantID, projectID)

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

    // Invalidate Redis cache for all containers and this specific container (project-specific)
    configs.RedisClient.Del(ctx, fmt.Sprintf("containers:all:tenant_%s:project_%s", tenantID, projectID))
    configs.RedisClient.Del(ctx, fmt.Sprintf("container:%s:tenant_%s:project_%s", containerIdStr, tenantID, projectID))
    log.Println("Invalidated containers cache after dynamic functions update")

    // Emit WebSocket event for container change
    userIDStr, _ := c.Locals("userID").(string)
    ws.EmitContainerChanged(userIDStr)

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

	// Extract tenant and project context from URL slugs with JWT validation
	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil {
		log.Printf("Failed to get project context: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to get project context: "+err.Error())
	}
	if tenantID == "" || projectID == "" {
		log.Println("Missing tenant or project context")
		return utils.SendErrorResponse(c, nil, "Missing tenant or project context.")
	}

	// Get project-specific container collection

	// Get project-specific container collection
	containerCollection := utils.GetContainerCollectionForProject(tenantID, projectID)

	containerIdStr := c.Params("id")
	containerId, err := primitive.ObjectIDFromHex(containerIdStr)
	if err != nil {
		log.Printf("Invalid ID format: %v", err)
		return utils.SendErrorResponse(c, err, "Provided ID is not in the valid format.")
	}

	// Try to get from Redis cache first (project-specific key)
	redisKey := fmt.Sprintf("container:%s:tenant_%s:project_%s", containerIdStr, tenantID, projectID)
	if cachedData, err := configs.RedisClient.Get(ctx, redisKey).Result(); err == nil {
		var container models.ContainerModel
		if json.Unmarshal([]byte(cachedData), &container) == nil {
			log.Printf("Fetched container %s from Redis cache", containerIdStr)
			return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
				Status:  http.StatusOK,
				Message: "Container successfully retrieved.",
				Data:    &fiber.Map{"data": container},
			})
		}
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

	// Cache the result in Redis (30 minutes TTL)
	if payload, err := json.Marshal(container); err == nil {
		configs.RedisClient.Set(ctx, redisKey, payload, 30*time.Minute)
		log.Printf("Cached container %s in Redis", containerIdStr)
	}

	log.Println("Container successfully retrieved from database")
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Container successfully retrieved",
		Data:    &fiber.Map{"data": container},
	})
}

// GetAllContainerTypes retrieves all containers and returns their dynamic field type definitions.
func GetAllContainerTypes(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // Extract tenant and project context from URL slugs with JWT validation
    tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
    if err != nil {
        log.Printf("Failed to get project context: %v", err)
        return utils.SendErrorResponse(c, err, "Failed to get project context: "+err.Error())
    }
    if tenantID == "" || projectID == "" {
        log.Println("Missing tenant or project context")
        return utils.SendErrorResponse(c, nil, "Missing tenant or project context.")
    }

    // Get project-specific container collection
    containerCollection := utils.GetContainerCollectionForProject(tenantID, projectID)

    // Retrieve all container documents from the collection.
    cursor, err := containerCollection.Find(ctx, bson.M{})
    if err != nil {
        log.Printf("Failed to retrieve containers: %v", err)
        return utils.SendErrorResponse(c, err, "Failed to retrieve containers from the database.")
    }
    defer cursor.Close(ctx)

    var containerTypes []models.ContainerTypes

    // Iterate over each container document.
    for cursor.Next(ctx) {
        var container models.ContainerModel
        if err = cursor.Decode(&container); err != nil {
            log.Printf("Error decoding container: %v", err)
            return utils.SendErrorResponse(c, err, "An error occurred while processing containers.")
        }

        // Build a map of field names to their type definitions.
        fieldTypes := make(map[string]string)
        for _, field := range container.Fields {
            fieldTypes[field.Name] = field.Type
        }

        containerTypes = append(containerTypes, models.ContainerTypes{
            ID:         container.ID.Hex(),
            SchemaName: container.SchemaName,
            FieldTypes: fieldTypes,
        })
    }

    // Check for any errors encountered during iteration.
    if err = cursor.Err(); err != nil {
        log.Printf("Cursor error: %v", err)
        return utils.SendErrorResponse(c, err, "An error occurred while iterating over containers.")
    }

    log.Println("Successfully retrieved container types")
    return utils.SendResponse(c, http.StatusOK, "Container types successfully retrieved.", containerTypes)
}

// ResetRedis resets the entire Redis cache
func ResetRedis(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Println("Resetting entire Redis cache")
	err := configs.RedisClient.FlushAll(ctx).Err()
	if err != nil {
		log.Printf("Failed to reset Redis cache: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to reset the Redis cache. Please try again later.")
	}

	log.Println("Redis cache successfully reset")
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Redis cache successfully reset.",
		Data:    nil,
	})
}
