package controllers

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"plugin"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// create an item for a given collection
func CreateDynamicModelItem(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    schemaName := c.Query("schemaName")

    // Fetch the associated container model
	var container *models.ContainerModel
	var err error

	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		// Use the container model from the context if available
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		// Fetch container model if not available in context
		container, err = utils.GetContainerModel(schemaName)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch container model"})
		}
	}

    // Check if there is an image field in the container
    hasImageField := false
    for _, field := range container.Fields {
        if field.Type == "image" {
            hasImageField = true
            break
        }
    }

    var itemMap map[string]interface{}
    fileURLs := make(map[string]string)

    if hasImageField {
        // Parse multipart form for image fields
        form, err := c.MultipartForm()
        if err != nil {
            return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Error parsing form.", Data: &fiber.Map{"data": err.Error()}})
        }
        itemMap = utils.ProcessFormFields(form.Value)

        // Process image fields
        for fieldName, files := range form.File {
            for _, file := range files {
                tempFilePath := "./temp/" + file.Filename
                if err := c.SaveFile(file, tempFilePath); err != nil {
                    return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Error saving temp file.", Data: &fiber.Map{"data": err.Error()}})
                }
                imageURL, err := utils.UploadToCloudinary(tempFilePath)
                os.Remove(tempFilePath) // Clean up temp file
                if err != nil {
                    return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Error uploading to Cloudinary.", Data: &fiber.Map{"data": err.Error()}})
                }
                fileURLs[fieldName] = imageURL
            }
        }
    } else {
        // Parse the provided item from request body
        if err := c.BodyParser(&itemMap); err != nil {
            return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Failed to parse body.", Data: &fiber.Map{"data": err.Error()}})
        }
    }
    // Create a set of allowed field names
    allowedFields := make(map[string]struct{})
    for _, field := range container.Fields {
        allowedFields[field.Name] = struct{}{}
    }

    // Filter out fields not in container.Fields
    for key := range itemMap {
        if _, exists := allowedFields[key]; !exists {
            delete(itemMap, key)
        }
    }
    // Replace file fields with URLs
    for fieldName, url := range fileURLs {
        itemMap[fieldName] = url
    }

    // Validation
   err = utils.ValidateContainerModel(itemMap, *container)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status: http.StatusBadRequest, Message: "Validation failed.", Data: &fiber.Map{"data": err.Error()},
		})
	}

    // Convert fields that should be ObjectId to ObjectId type
    for _, field := range container.Fields {
        if field.Type == "objectId" {
            if strId, ok := itemMap[field.Name].(string); ok {
                objId, err := primitive.ObjectIDFromHex(strId)
                if err == nil {
                    itemMap[field.Name] = objId
                }
            }
        }
    }

    // Get the associated collection for this schema
    var currentCollection *mongo.Collection = configs.GetCollection(configs.DB, schemaName)

    // Save the validated item into its associated collection
    result, err := currentCollection.InsertOne(ctx, itemMap)
    if err != nil {
        return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status: http.StatusInternalServerError, Message: "Failed to save item.", Data: &fiber.Map{"data": err.Error()},
        })
    }
	// Delete the cache for this schema
if container.Redis.IsRedisCached {
    err = utils.DeleteCacheForSchema(ctx, schemaName, container)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
            "status":  fiber.StatusInternalServerError,
            "message": "Failed to delete the cache for the schema.",
            "data":    err.Error(),
        })
    }

    // Additionally, delete cache for each schema mentioned in TriggeredRedisCaches
    for _, triggeredSchema := range container.Redis.TriggeredRedisCaches {
        err = utils.DeleteCacheForSchema(ctx, triggeredSchema, container)
        if err != nil {
        // Log the error and continue with the next iteration
        log.Printf("Error deleting cache for schema %s: %v", triggeredSchema, err)
        continue
    }
    }
}
    // Successfully saved the item
    return c.Status(http.StatusCreated).JSON(responses.GeneralResponse{
        Status: http.StatusCreated, Message: "Item successfully created.", Data: &fiber.Map{"data": result},
    })
}
// get all items for a given collection
func GetAllDynamicModelItems(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schemaName := c.Query("schemaName")
	if schemaName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Schema name is required"})
	}

  	var container *models.ContainerModel
    if storedContainer := c.Locals("containerModel"); storedContainer != nil {
        // Use the container model from the context if available
        container, _ = storedContainer.(*models.ContainerModel)
    } else {
        // Fetch container model if not available in context
        var err error
        container, err = utils.GetContainerModel(schemaName)
        if err != nil {
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch container model"})
        }
    }

	redisKey, shouldCache := utils.GenerateRedisKey("GetAllDynamicModelItems", schemaName, container)
	if shouldCache {
		cachedData, err := configs.RedisClient.Get(ctx, redisKey).Result()
		
		if err == nil {
			var items []map[string]interface{}
			if err := json.Unmarshal([]byte(cachedData), &items); err == nil {
				// Data fetched from cache
				return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": items, "source": "cache"})
			}
		}

		// If not found in cache, fetch from database
		currentCollection := configs.GetCollection(configs.DB, schemaName)
		results, err := currentCollection.Find(ctx, bson.M{})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch items from the database"})
		}
		defer results.Close(ctx)

		var items []map[string]interface{}
		for results.Next(ctx) {
			var singleItem map[string]interface{}
			if err = results.Decode(&singleItem); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to decode an item"})
			}
			items = append(items, singleItem)
		}

		// Cache the data fetched from database
		dataToCache, _ := json.Marshal(items)
		var expiration time.Duration
		if container.Redis.CacheTime > 0 {
			expiration = time.Duration(container.Redis.CacheTime) * time.Minute
		} else {
			expiration = 24 * time.Hour // Default to 24 Hours
		}

		configs.RedisClient.Set(ctx, redisKey, dataToCache, expiration)


		// Data fetched from database
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": items, "source": "database"})
	}

	// If caching is not enabled, fetch from database
	currentCollection := configs.GetCollection(configs.DB, schemaName)
	results, err := currentCollection.Find(ctx, bson.M{})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch items from the database"})
	}
	defer results.Close(ctx)

	var items []map[string]interface{}
	for results.Next(ctx) {
		var singleItem map[string]interface{}
		if err = results.Decode(&singleItem); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to decode an item"})
		}
		items = append(items, singleItem)
	}

	// Data fetched from database
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": items, "source": "database"})
}

//delete an item from the collection
func DeleteDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Fetching the schema name from the query params
	schemaName := c.Query("schemaName")

	var container *models.ContainerModel
    if storedContainer := c.Locals("containerModel"); storedContainer != nil {
        // Use the container model from the context if available
        container, _ = storedContainer.(*models.ContainerModel)
    } else {
        // Fetch container model if not available in context
        var err error
        container, err = utils.GetContainerModel(schemaName)
        if err != nil {
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch container model"})
        }
    }
	
	// Using the schema name to determine the appropriate collection
	var currentCollection *mongo.Collection = configs.GetCollection(configs.DB, schemaName)

	// Attempting to convert the ID from string to ObjectID
	deleteIdStr := c.Params("id")
	deleteId, err := primitive.ObjectIDFromHex(deleteIdStr)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Provided ID is not in the valid format.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Attempting to delete the item with the given ID from the specified collection
	result, err := currentCollection.DeleteOne(ctx, bson.M{"_id": deleteId})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete the item from the specified collection.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}
	// Now attempting to delete the related cache
if container.Redis.IsRedisCached {
    err = utils.DeleteCacheForSchema(ctx, schemaName, container)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
            "status":  fiber.StatusInternalServerError,
            "message": "Failed to delete the cache for the schema.",
            "data":    err.Error(),
        })
    }

    // Additionally, delete cache for each schema mentioned in TriggeredRedisCaches
    for _, triggeredSchema := range container.Redis.TriggeredRedisCaches {
        err = utils.DeleteCacheForSchema(ctx, triggeredSchema, container)
        if err != nil {
        // Log the error and continue with the next iteration
        log.Printf("Error deleting cache for schema %s: %v", triggeredSchema, err)
        continue
    }
    }
}
	// Successfully deleted the item
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Item successfully deleted from the specified collection.",
		Data:    &fiber.Map{"data": result},
	})
}
//update an item in the collection
func UpdateDynamicModelItem(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    schemaName := c.Query("schemaName")
    updateIdStr := c.Params("id")
    updateId, err := primitive.ObjectIDFromHex(updateIdStr)
    if err != nil {
        return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
            Status:  http.StatusBadRequest,
            Message: "Provided ID is not in the valid format.",
            Data:    &fiber.Map{"data": err.Error()},
        })
    }

    // Fetch the associated container model
	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		// Use the container model from the context if available
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		// Fetch container model if not available in context
		container, err = utils.GetContainerModel(schemaName)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch container model"})
		}
	}

    // Check if there is an image field in the container
    hasImageField := false
    for _, field := range container.Fields {
        if field.Type == "image" {
            hasImageField = true
            break
        }
    }

    var updatedItemMap map[string]interface{}
    fileURLs := make(map[string]string)

    if hasImageField  {
        // Handle multipart form data
        form, err := c.MultipartForm()
        if err != nil {
            return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Error in multipart form", Data: &fiber.Map{"data": err.Error()}})
        }
        updatedItemMap = utils.ProcessFormFields(form.Value)

        // Process image fields
        for fieldName, files := range form.File {
            for _, file := range files {
                tempFilePath := "./temp/" + file.Filename
                if err := c.SaveFile(file, tempFilePath); err != nil {
                    return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Error saving temp file", Data: &fiber.Map{"data": err.Error()}})
                }
                imageURL, err := utils.UploadToCloudinary(tempFilePath)
                os.Remove(tempFilePath)
                if err != nil {
                    return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Error uploading to Cloudinary", Data: &fiber.Map{"data": err.Error()}})
                }
                fileURLs[fieldName] = imageURL
            }
        }
    } else {
        // Handle JSON body
        if err := c.BodyParser(&updatedItemMap); err != nil {
            return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Failed to parse body", Data: &fiber.Map{"data": err.Error()}})
        }
    }
    // Create a set of allowed field names
    allowedFields := make(map[string]struct{})
    for _, field := range container.Fields {
        allowedFields[field.Name] = struct{}{}
    }

    // Filter out fields not in container.Fields
    for key := range updatedItemMap {
        if _, exists := allowedFields[key]; !exists {
            delete(updatedItemMap, key)
        }
    }
    // Replace file fields with URLs
    for fieldName, url := range fileURLs {
        updatedItemMap[fieldName] = url
    }
    // Fetch the existing item
    var currentCollection *mongo.Collection = configs.GetCollection(configs.DB, schemaName)
    var existingItem bson.M
    err = currentCollection.FindOne(ctx, bson.M{"_id": updateId}).Decode(&existingItem)
    if err != nil {
        return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Failed to fetch item", Data: &fiber.Map{"data": err.Error()}})
    }
    // Apply updates from updatedItemMap to existingItem
    for key, value := range updatedItemMap {
        existingItem[key] = value
    }

    // Validation
    err = utils.ValidateContainerModel(existingItem, *container)
    if err != nil {
        return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Validation failed", Data: &fiber.Map{"data": err.Error()}})
    }

    // Update in collection
    updateResult, err := currentCollection.UpdateOne(ctx, bson.M{"_id": updateId}, bson.M{"$set": existingItem})
    if err != nil {
        return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Failed to update item", Data: &fiber.Map{"data": err.Error()}})
    }

    if updateResult.MatchedCount == 0 {
        return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{Status: http.StatusNotFound, Message: "No item found with specified ID", Data: nil})
    }
	// Now attempting to delete the related cache
	if container.Redis.IsRedisCached {
		err = utils.DeleteCacheForSchema(ctx, schemaName, container)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"status":  fiber.StatusInternalServerError,
				"message": "Failed to delete the cache for the schema.",
				"data":    err.Error(),
			})
		}

		// Additionally, delete cache for each schema mentioned in TriggeredRedisCaches
		for _, triggeredSchema := range container.Redis.TriggeredRedisCaches {
			err = utils.DeleteCacheForSchema(ctx, triggeredSchema, container)
			if err != nil {
			// Log the error and continue with the next iteration
			log.Printf("Error deleting cache for schema %s: %v", triggeredSchema, err)
			continue
		}
		}
	}

    return c.Status(http.StatusOK).JSON(responses.GeneralResponse{Status: http.StatusOK, Message: "Item successfully updated", Data: &fiber.Map{"data": updateResult}})
}

// get an item from the database
func GetDynamicModelItem(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    schemaName := c.Query("schemaName")
    if schemaName == "" {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Schema name is required"})
    }

    getIdStr := c.Params("id")
    getId, err := primitive.ObjectIDFromHex(getIdStr)
    if err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid ID format"})
    }

   	var container *models.ContainerModel
    if storedContainer := c.Locals("containerModel"); storedContainer != nil {
        // Use the container model from the context if available
        container, _ = storedContainer.(*models.ContainerModel)
    } else {
        // Fetch container model if not available in context
        var err error
        container, err = utils.GetContainerModel(schemaName)
        if err != nil {
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch container model"})
        }
    }

    redisKey, shouldCache := utils.GenerateRedisKey("GetDynamicModelItem", schemaName, container, getIdStr)
    if shouldCache {
        cachedData, err := configs.RedisClient.Get(ctx, redisKey).Result()
        if err == nil {
            var item bson.M
            if err := json.Unmarshal([]byte(cachedData), &item); err == nil {
                // Data fetched from cache
                return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": item, "source": "cache"})
            }
        }
    }

    currentCollection := configs.GetCollection(configs.DB, schemaName)
    var result bson.M
    if err := currentCollection.FindOne(ctx, bson.M{"_id": getId}).Decode(&result); err != nil {
        return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Item not found"})
    }

    if shouldCache {
        dataToCache, _ := json.Marshal(result)
        var expiration time.Duration
		if container.Redis.CacheTime > 0 {
			expiration = time.Duration(container.Redis.CacheTime) * time.Minute
		} else {
			expiration = 24 * time.Hour // Default to 24 Hours
		}

		configs.RedisClient.Set(ctx, redisKey, dataToCache, expiration)
    }

    return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": result, "source": "database"})
}
// handleSearch for a given collection
func HandleSearchDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schemaName := c.Query("schemaName")
	searchKey := c.Query("searchKey")

	    // Fetch the associated container model
	var container *models.ContainerModel
	var err error
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		// Use the container model from the context if available
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		// Fetch container model if not available in context
		container, err = utils.GetContainerModel(schemaName)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch container model"})
		}
	}
	// Define the regex pattern to match anywhere in the string
	pattern := ".*" + searchKey + ".*"
	regex := primitive.Regex{Pattern: pattern, Options: "i"} // "i" is for case-insensitive

	// Build the query filter
	var orQueries []bson.M
	for _, field := range container.Fields {
		if field.Type == "string" {
			orQueries = append(orQueries, bson.M{field.Name: regex})
		}
	}
	filter := bson.M{"$or": orQueries}

	// Using the schema name to determine the appropriate collection
	var currentCollection *mongo.Collection = configs.GetCollection(configs.DB, schemaName)
	results, err := currentCollection.Find(ctx, filter)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Error fetching search results.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Reading from the db
	var items []map[string]interface{}
	defer results.Close(ctx)
	for results.Next(ctx) {
		var item map[string]interface{}
		if err = results.Decode(&item); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
				Status:  http.StatusInternalServerError,
				Message: "Error decoding search result.",
				Data:    &fiber.Map{"data": err.Error()},
			})
		}
		items = append(items, item)
	}

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Search results fetched successfully.",
		Data:    &fiber.Map{"data": items},
	})
}
//get all item for given collection
func GetPipeline(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // Fetching the schema name and pipeline name from the query params
    schemaName := c.Query("schemaName")
    pipelineName := c.Query("pipelineName")

    // Validate schemaName and pipelineName
    if schemaName == "" || pipelineName == "" {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Schema name and pipeline name are required"})
    }

    // Serialize the current query parameters
    currentQuery := c.OriginalURL()

    // Get the container model for the schema
    var container *models.ContainerModel
    if storedContainer := c.Locals("containerModel"); storedContainer != nil {
        container, _ = storedContainer.(*models.ContainerModel)
    } else {
        var err error
        container, err = utils.GetContainerModel(schemaName)
        if err != nil {
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch container model"})
        }
    }

    // Find the pipeline stage with the given name
    var pipelineStage models.PipelineStage
    found := false
    for _, stage := range container.Pipelines {
        if stage.Name == pipelineName {
            pipelineStage = stage
            found = true
            break
        }
    }
    if !found {
        return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
            Status:  http.StatusNotFound,
            Message: "Pipeline not found",
            Data:    nil,
        })
    }
    pipelineStage.PipelineJSON = utils.ReplacePlaceholdersWithQueryParams(pipelineStage.PipelineJSON, c)
    currentCollection := configs.GetCollection(configs.DB, schemaName)

    redisKey, shouldCache := utils.GeneratePipelineRedisKey(schemaName, pipelineName, container)

    // Check if query has changed
    queryChanged := false
    if shouldCache {
        storedQuery, err := configs.RedisClient.Get(ctx, redisKey+"-query").Result()
        if err == nil && storedQuery != currentQuery {
            queryChanged = true
        }
    }

    // Fetch from cache if query hasn't changed and cache is available
    if shouldCache && !queryChanged {
        cachedData, err := configs.RedisClient.Get(ctx, redisKey).Result()
        if err == nil {
            var items []bson.M
            if err := json.Unmarshal([]byte(cachedData), &items); err == nil {
                return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": items, "source": "cache"})
            }
        }
    }

    // Execute the dynamic pipeline
    items, err := utils.ExecuteDynamicPipeline(ctx, currentCollection, pipelineStage)
    if err != nil {
        return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status:  http.StatusInternalServerError,
            Message: "Failed to execute dynamic pipeline",
            Data:    &fiber.Map{"error": err.Error()},
        })
    }

    // Cache the new data and query if shouldCache is true
    if shouldCache {
        dataToCache, _ := json.Marshal(items)
        var expiration time.Duration
        if pipelineStage.CacheTime > 0 {
            expiration = time.Duration(pipelineStage.CacheTime) * time.Minute
        } else {
            expiration = 24 * time.Hour // Default to 24 Hours
        }
        configs.RedisClient.Set(ctx, redisKey, dataToCache, expiration)
        configs.RedisClient.Set(ctx, redisKey+"-query", currentQuery, expiration)
    }

    // Return the results
    return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
        Status:  http.StatusOK,
        Message: "Successfully fetched items using dynamic pipeline",
        Data:    &fiber.Map{"data": items, "source": "database"},
    })
}
// GetAllDynamicModelItemsWithPagination gets items from a collection with pagination.
func GetAllDynamicModelItemsWithPagination(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    schemaName := c.Query("schemaName")
    if schemaName == "" {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Schema name is required"})
    }

    // Parse pagination query parameters
    pageStr := c.Query("page", "1") // Default to page 1
    limitStr := c.Query("limit", "10") // Default to 10 items per page

    page, err := strconv.Atoi(pageStr)
    if err != nil || page < 1 {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid page number"})
    }

    limit, err := strconv.Atoi(limitStr)
    if err != nil || limit < 1 {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid limit number"})
    }

    // var container *models.ContainerModel
    // if storedContainer := c.Locals("containerModel"); storedContainer != nil {
    //     // Use the container model from the context if available
    //     container, _ = storedContainer.(*models.ContainerModel)
    // } else {
    //     // Fetch container model if not available in context
    //     var err error
    //     container, err = utils.GetContainerModel(schemaName)
    //     if err != nil {
    //         return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch container model"})
    //     }
    // }


    // Calculate the number of documents to skip
    skip := (page - 1) * limit

    // Define find options for pagination
    findOptions := options.Find().SetSkip(int64(skip)).SetLimit(int64(limit))

    currentCollection := configs.GetCollection(configs.DB, schemaName)
    cursor, err := currentCollection.Find(ctx, bson.M{}, findOptions)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch items from the database"})
    }
    defer cursor.Close(ctx)

    var items []map[string]interface{}
    for cursor.Next(ctx) {
        var item map[string]interface{}
        if err := cursor.Decode(&item); err != nil {
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to decode an item"})
        }
        items = append(items, item)
    }

    // Get total count of documents in the collection
    totalItems, err := currentCollection.CountDocuments(ctx, bson.M{})
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to count items"})
    }

    // Calculate total pages
    totalPages := int(totalItems) / limit
    if int(totalItems)%limit > 0 {
        totalPages++
    }

    // Return paginated result
    return c.Status(fiber.StatusOK).JSON(fiber.Map{
        "data":         items,
        "totalPages":   totalPages,
        "totalItems":   totalItems,
        "currentPage":  page,
    })
    }
// executeDynamicCode executes dynamic code from a request.
func ExecuteDynamicCode(c *fiber.Ctx) error {
    schemaName := c.Query("schemaName")
    functionName := c.Query("functionName")

    // Fetch the associated container model from context
    var container *models.ContainerModel
    if storedContainer := c.Locals("containerModel"); storedContainer != nil {
        container, _ = storedContainer.(*models.ContainerModel)
    } else {
        var err error
        container, err = utils.GetContainerModel(schemaName)
        if err != nil {
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch container model"})
        }
    }

    pluginFileName := "temp_" + functionName + ".so"
    fileName := "temp_" + functionName + ".go"

    // Check if plugin exists
    p, err := plugin.Open(pluginFileName)
    if err == nil {
        // Plugin exists, try to lookup the function
        f, err := p.Lookup(functionName)
        if err == nil {
            // Function found, execute it
            if executeFunc, ok := f.(func(*fiber.Ctx) (interface{}, error)); ok {
                result, err := executeFunc(c)
                if err != nil {
                    return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Function execution failed", "details": err.Error()})
                }
                return c.JSON(fiber.Map{"result": result})
            }
        }
    }

    // Function not found in existing plugin, fetch or generate new code
    var dynamicFuncCode string
    // Check if function is defined in container model
    functionExists := false
    for _, dynamicFunc := range container.DynamicFunctions {
        if dynamicFunc.Name == functionName {
            dynamicFuncCode = dynamicFunc.CodeJSON
            functionExists = true
            break
        }
    }
    if !functionExists {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Function not defined in container model"})
    }

    // Write new code to file
    err = ioutil.WriteFile(fileName, []byte(dynamicFuncCode), 0644)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to write code"})
    }

    // Compile new code into plugin
    out, err := exec.Command("go", "build", "-buildmode=plugin", fileName).CombinedOutput()
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to compile code", "output": string(out)})
    }

    // Load new plugin
    p, err = plugin.Open(pluginFileName)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to open plugin"})
    }

    // Lookup function in new plugin
    f, err := p.Lookup(functionName)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to find function in new code"})
    }

    // Execute new function
    if executeFunc, ok := f.(func(*fiber.Ctx) (interface{}, error)); ok {
        result, err := executeFunc(c)
        if err != nil {
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Function execution failed", "details": err.Error()})
        }
        return c.JSON(fiber.Map{"result": result})
    } else {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid function signature"})
    }
}
