package controllers

import (
	"context"
	"encoding/json"
	"fmt"
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
			  return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
                Status:  http.StatusInternalServerError,
                Message: "Failed to fetch container model",
                Data:    err.Error(),
            })
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
            return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Error parsing form.",  Data: err.Error()})
        }
        itemMap = utils.ProcessFormFields(form.Value)

        // Process image fields
        for fieldName, files := range form.File {
            for _, file := range files {
                tempFilePath := "./temp/" + file.Filename
                if err := c.SaveFile(file, tempFilePath); err != nil {
                    return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Error saving temp file.",  Data: err.Error()})
                }
                imageURL, err := utils.UploadToCloudinary(tempFilePath)
                os.Remove(tempFilePath) // Clean up temp file
                if err != nil {
                    return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Error uploading to Cloudinary.",  Data: err.Error()})
                }
                fileURLs[fieldName] = imageURL
            }
        }
    } else {
        // Parse the provided item from request body
        if err := c.BodyParser(&itemMap); err != nil {
            return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Failed to parse body.",  Data: err.Error()})
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
			Status: http.StatusBadRequest, Message: "Validation failed.", Data: err.Error(),
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
    var currentCollection *mongo.Collection = configs.GetCollection( schemaName)

    // Checking for Unique fields
    for _, field := range container.Fields {
        if field.Unique {
            fieldValue, found := itemMap[field.Name]
            if !found {
                continue 
            }

            count, err := currentCollection.CountDocuments(ctx, bson.M{field.Name: fieldValue})
            if err != nil {
                return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
                    "status":  http.StatusInternalServerError,
                    "message": "Error checking existing field value.",
                    "data":    err.Error(),
                })
            }

            if count > 0 {
                return c.Status(http.StatusBadRequest).JSON(fiber.Map{
                    "status":  http.StatusBadRequest,
                    "message": fmt.Sprintf("A document with the same %s already exists.", field.Name),
                    "data":    nil,
                })
            }
        }
    }

    // Save the validated item into its associated collection
    result, err := currentCollection.InsertOne(ctx, itemMap)
    if err != nil {
        return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status: http.StatusInternalServerError, Message: "Failed to save item.", Data: err.Error(),
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
        Status: http.StatusCreated, Message: "Item successfully created.", Data: result,
    })
}
// get all items for a given collection
func GetAllDynamicModelItems(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schemaName := c.Query("schemaName")
	if schemaName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
            Status:  fiber.StatusBadRequest,
            Message: "Schema name is required",
            Data:    nil,
        })
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
              return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
                Status:  http.StatusInternalServerError,
                Message: "Failed to fetch container model",
                Data:    err.Error(),
            })
        }
    }

	redisKey, shouldCache := utils.GenerateRedisKey("GetAllDynamicModelItems", schemaName, container)
	if shouldCache {
		cachedData, err := configs.RedisClient.Get(ctx, redisKey).Result()
		
		if err == nil {
			var items []map[string]interface{}
			if err := json.Unmarshal([]byte(cachedData), &items); err == nil {
				// Data fetched from cache
				return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{
                    Status: fiber.StatusOK, Message: "Items successfully fetched.", Data: items,Source: utils.PointerToString("cache"),
                })
			}
		}

		// If not found in cache, fetch from database
		currentCollection := configs.GetCollection( schemaName)
		results, err := currentCollection.Find(ctx, bson.M{})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
                Status:  fiber.StatusInternalServerError,
                Message: "Failed to fetch items",
                Data:    err.Error(),
            })
		}
		defer results.Close(ctx)

		var items []map[string]interface{}
		for results.Next(ctx) {
			var singleItem map[string]interface{}
			if err = results.Decode(&singleItem); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
                    Status:  fiber.StatusInternalServerError,
                    Message: "Failed to decode item",
                    Data:    err.Error(),
                })
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
		return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{
            Status:  fiber.StatusOK,
            Message: "Items fetched successfully",
            Data:    items,
            Source: utils.PointerToString("database"),
        })
	}

	// If caching is not enabled, fetch from database
	currentCollection := configs.GetCollection( schemaName)
	results, err := currentCollection.Find(ctx, bson.M{})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status:  fiber.StatusInternalServerError,
            Message: "Failed to fetch items from the database",
            Data:    err.Error(),
        })
	}
	defer results.Close(ctx)

	var items []map[string]interface{}
	for results.Next(ctx) {
		var singleItem map[string]interface{}
		if err = results.Decode(&singleItem); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
                Status:  fiber.StatusInternalServerError,
                Message: "Failed to decode an item",
                Data:    err.Error(),
            })
		}
		items = append(items, singleItem)
	}

	// Data fetched from database
	return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{
        Status:  fiber.StatusOK,
        Message: "Items successfully fetched.",
        Data:    items,
        Source: utils.PointerToString("database"),
    })
}
// TODO: performance will be improved by adding a field in the container as usedSchemas (which will be updated when the new schema added with objectId of the currentSchema) and instead of getting all containers we will only check the neccessary containers and if the usedSchemas are empty we will not waste time with getting all containers

//delete an item from the collection
func DeleteDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Fetching the schema name from the query params
	schemaName := c.Query("schemaName")
    
    // Attempting to convert the ID from string to ObjectID
	deleteIdStr := c.Params("id")
	deleteId, err := primitive.ObjectIDFromHex(deleteIdStr)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Provided ID is not in the valid format.",
			Data:  err.Error(),
		})
	}


    //check if schema is used as objectId in other containers
    allContainers, err := utils.GetAllContainerModels()
        if err != nil {
            return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
                "status":  http.StatusInternalServerError,
                "message": "Failed to retrieve container models.",
                "data":    err.Error(),
            })
        }
	// First check if any reference prevents deletion
	for _, container := range allContainers {
		if container.SchemaName != schemaName { // Skip the current schema
			for _, field := range container.Fields {
				if field.Type == "objectId" && field.Name == schemaName { // Field referencing the schema as an objectId
					collection := configs.GetCollection(container.SchemaName)
					count, err := collection.CountDocuments(ctx, bson.M{field.Name: deleteId})
					if err != nil {
						return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
							"status":  http.StatusInternalServerError,
							"message": "Database error while checking references.",
							"data":    err.Error(),
						})
					}
					if count > 0 && !field.IsForceDelete {
						return c.Status(http.StatusBadRequest).JSON(fiber.Map{
							"status":  http.StatusBadRequest,
							"message": fmt.Sprintf("Cannot delete: This item is still referenced in schema '%s' and cannot be forcibly deleted.", container.SchemaName),
							"data":    nil,
						})
					}
				}
			}
		}
	}

	// If passed, proceed with deletion
	for _, container := range allContainers {
		if container.SchemaName != schemaName { // Skip the current schema
			for _, field := range container.Fields {
				if field.Type == "objectId" && field.Name == schemaName && field.IsForceDelete { // Field referencing the schema as an objectId
					collection := configs.GetCollection( container.SchemaName)
					_, delErr := collection.DeleteMany(ctx, bson.M{field.Name: deleteId})
					if delErr != nil {
						return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
							"status":  http.StatusInternalServerError,
							"message": "Failed to force delete referenced items.",
							"data":    delErr.Error(),
						})
					}
				}
			}
		}
	}
	var container *models.ContainerModel
    if storedContainer := c.Locals("containerModel"); storedContainer != nil {
        // Use the container model from the context if availabl
        container, _ = storedContainer.(*models.ContainerModel)
    } else {
        // Fetch container model if not available in context
        var err error
        container, err = utils.GetContainerModel(schemaName)
        if err != nil {
              return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
                Status:  http.StatusInternalServerError,
                Message: "Failed to fetch container model",
                Data:    err.Error(),
            })
        }
    }
	
	// Using the schema name to determine the appropriate collection
	var currentCollection *mongo.Collection = configs.GetCollection( schemaName)

	
	// Attempting to delete the item with the given ID from the specified collection
	result, err := currentCollection.DeleteOne(ctx, bson.M{"_id": deleteId})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete the item from the specified collection.",
			Data:  err.Error(),
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
		Data:   result,
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
            Data:    err.Error(),
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
			  return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
                Status:  http.StatusInternalServerError,
                Message: "Failed to fetch container model",
                Data:    err.Error(),
            })
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
            return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Error in multipart form",  Data: err.Error()})
        }
        updatedItemMap = utils.ProcessFormFields(form.Value)

        // Process image fields
        for fieldName, files := range form.File {
            for _, file := range files {
                tempFilePath := "./temp/" + file.Filename
                if err := c.SaveFile(file, tempFilePath); err != nil {
                    return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Error saving temp file",  Data: err.Error()})
                }
                imageURL, err := utils.UploadToCloudinary(tempFilePath)
                os.Remove(tempFilePath)
                if err != nil {
                    return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Error uploading to Cloudinary",  Data: err.Error()})
                }
                fileURLs[fieldName] = imageURL
            }
        }
    } else {
        // Handle JSON body
        if err := c.BodyParser(&updatedItemMap); err != nil {
            return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Failed to parse body",  Data: err.Error()})
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
    var currentCollection *mongo.Collection = configs.GetCollection( schemaName)
    var existingItem bson.M
    err = currentCollection.FindOne(ctx, bson.M{"_id": updateId}).Decode(&existingItem)
    if err != nil {
        return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Failed to fetch item", Data: err.Error()})
    }
    // Apply updates from updatedItemMap to existingItem
    for key, value := range updatedItemMap {
        existingItem[key] = value
    }

    // Validation
    err = utils.ValidateContainerModel(existingItem, *container)
    if err != nil {
        return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Validation failed",  Data: err.Error()})
    }
        // Checking for Unique fields
    for _, field := range container.Fields {
        if field.Unique {
            fieldValue, found := updatedItemMap[field.Name]
            if !found {
                continue 
            }

            count, err := currentCollection.CountDocuments(ctx, bson.M{field.Name: fieldValue})
            if err != nil {
                return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
                    "status":  http.StatusInternalServerError,
                    "message": "Error checking existing field value.",
                    "data":    err.Error(),
                })
            }

            if count > 0 {
                return c.Status(http.StatusBadRequest).JSON(fiber.Map{
                    "status":  http.StatusBadRequest,
                    "message": fmt.Sprintf("A document with the same %s already exists.", field.Name),
                    "data":    nil,
                })
            }
        }
    }

    // Update in collection
    updateResult, err := currentCollection.UpdateOne(ctx, bson.M{"_id": updateId}, bson.M{"$set": existingItem})
    if err != nil {
        return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Failed to update item",  Data: err.Error()})
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

    return c.Status(http.StatusOK).JSON(responses.GeneralResponse{Status: http.StatusOK, Message: "Item successfully updated", Data: updateResult})
}

// get an item from the database
func GetDynamicModelItem(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    schemaName := c.Query("schemaName")
    if schemaName == "" {
        return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
            Status:  fiber.StatusBadRequest,
            Message: "Schema name is required",
            Data:    nil,
        })
    }

    getIdStr := c.Params("id")
    getId, err := primitive.ObjectIDFromHex(getIdStr)
    if err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
            Status:  fiber.StatusBadRequest,
            Message: "Invalid ID",
            Data:    err.Error(),
        })
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
              return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
                Status:  http.StatusInternalServerError,
                Message: "Failed to fetch container model",
                Data:    err.Error(),
            })
        }
    }

    redisKey, shouldCache := utils.GenerateRedisKey("GetDynamicModelItem", schemaName, container)
    if shouldCache {
        cachedData, err := configs.RedisClient.Get(ctx, redisKey).Result()
        if err == nil {
            var item bson.M
            if err := json.Unmarshal([]byte(cachedData), &item); err == nil {
                // Data fetched from cache
                return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{Status: http.StatusOK, Message: "Item found", Data: item,Source: utils.PointerToString("cache")})
            }
        }
    }

    currentCollection := configs.GetCollection( schemaName)
    var result bson.M
    if err := currentCollection.FindOne(ctx, bson.M{"_id": getId}).Decode(&result); err != nil {
        return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{Status: http.StatusNotFound, Message: "Item not found", Data: nil})
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

    return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{Status: http.StatusOK, Message: "Item found", Data: result,Source: utils.PointerToString("database")})
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
			  return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
                Status:  http.StatusInternalServerError,
                Message: "Failed to fetch container model",
                Data:    err.Error(),
            })
		}
	}

	// Build the query filter
	var orQueries []bson.M
	for _, field := range container.Fields {
		switch field.Type {
		case "string":
			pattern := ".*" + searchKey + ".*"
			regex := primitive.Regex{Pattern: pattern, Options: "i"} // Case-insensitive
			orQueries = append(orQueries, bson.M{field.Name: regex})
		case "int", "float":
			if num, err := strconv.ParseFloat(searchKey, 64); err == nil { // Converts to float for both int and float comparisons
				orQueries = append(orQueries, bson.M{field.Name: num})
			}
		case "boolean":
			if boolVal, err := strconv.ParseBool(searchKey); err == nil {
				orQueries = append(orQueries, bson.M{field.Name: boolVal})
			}
		case "date":
			if dateVal, err := time.Parse(time.RFC3339, searchKey); err == nil { // Assumes ISO 8601 format; adjust as necessary
				orQueries = append(orQueries, bson.M{field.Name: dateVal})
			}
		case "objectId":
			if objId, err := primitive.ObjectIDFromHex(searchKey); err == nil {
				orQueries = append(orQueries, bson.M{field.Name: objId})
			}
		}
	}

	if len(orQueries) == 0 {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "No valid search queries could be constructed from the provided input.",
			Data:    nil,
		})
	}

	filter := bson.M{"$or": orQueries}

	// Using the schema name to determine the appropriate collection
	var currentCollection *mongo.Collection = configs.GetCollection( schemaName)
	results, err := currentCollection.Find(ctx, filter)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Error fetching search results.",
			Data:   err.Error(),
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
				Data:    err.Error(),
			})
		}
		items = append(items, item)
	}

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Search results fetched successfully.",
		Data:   items,
	})
}
// handleFilter for a given collection with dynamic query parameters and values.the fields needs to be send in  query like field=value.
func HandleFilterDynamicModelItem(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    schemaName := c.Query("schemaName")

    // Fetch the associated container model
    var container *models.ContainerModel
    var err error
    if storedContainer := c.Locals("containerModel"); storedContainer != nil {
        container, _ = storedContainer.(*models.ContainerModel)
    } else {
        container, err = utils.GetContainerModel(schemaName)
        if err != nil {
            return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
                "status":  http.StatusInternalServerError,
                "message": "Failed to fetch container model",
                "data":    err.Error(),
            })
        }
    }

    filter := bson.M{}
    for _, field := range container.Fields {
        queryValue := c.Query(field.Name)
        if queryValue == "" {
            continue
        }

        convertedValue, err := utils.ConvertQueryValueToFieldType(field.Name, field.Type, queryValue)
        if err != nil {
            return c.Status(http.StatusBadRequest).JSON(fiber.Map{
                "status":  http.StatusBadRequest,
                "message": err.Error(),
                "data":    nil,
            })
        }

        if convertedMap, ok := convertedValue.(bson.M); ok {
            if existingCondition, exists := filter[field.Name]; exists {
                existingMap, ok := existingCondition.(bson.M)
                if ok {
                    for key, val := range convertedMap {
                        existingMap[key] = val
                    }
                }
            } else {
                filter[field.Name] = convertedMap
            }
        } else {
            filter[field.Name] = convertedValue
        }
    }

    var currentCollection *mongo.Collection = configs.GetCollection(schemaName)
    results, err := currentCollection.Find(ctx, filter)
    if err != nil {
        return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
            "status":  http.StatusInternalServerError,
            "message": "Error fetching filter results.",
            "data":    err.Error(),
        })
    }
    var items []map[string]interface{}
    defer results.Close(ctx)
    for results.Next(ctx) {
        var item map[string]interface{}
        if err = results.Decode(&item); err != nil {
            return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
                "status":  http.StatusInternalServerError,
                "message": "Error decoding filter result.",
                "data":    err.Error(),
            })
        }
        items = append(items, item)
    }

    return c.Status(http.StatusOK).JSON(fiber.Map{
        "status":  http.StatusOK,
        "message": "Filter results fetched successfully.",
        "data":    items,
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
        return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
            Status:  http.StatusBadRequest,
            Message: "schemaName and pipelineName are required",
            Data:    nil,
        })
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
            return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
                Status:  http.StatusInternalServerError,
                Message: "Failed to fetch container model",
                Data:   err.Error(),
            })
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
    currentCollection := configs.GetCollection( schemaName)

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
                return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{
                    Status:  http.StatusOK,
                    Message: "Pipeline results fetched successfully",
                    Data:    items,
                    Source: utils.PointerToString("cache"),
                })
            }
        }
    }

    // Execute the dynamic pipeline
    items, err := utils.ExecuteDynamicPipeline(ctx, currentCollection, pipelineStage)
    if err != nil {
        return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status:  http.StatusInternalServerError,
            Message: "Failed to execute dynamic pipeline",
            Data:    err.Error(),
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
        Data:    items,
    })
}
// GetAllDynamicModelItemsWithPagination gets items from a collection with pagination.
func GetAllDynamicModelItemsWithPagination(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    schemaName := c.Query("schemaName")
    if schemaName == "" {
        return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
            Status:  http.StatusBadRequest,
            Message: "schemaName is required",
            Data:    nil,
        })
    }

    // Parse pagination query parameters
    pageStr := c.Query("page", "1") // Default to page 1
    limitStr := c.Query("limit", "10") // Default to 10 items per page

    page, err := strconv.Atoi(pageStr)
    if err != nil || page < 1 {
        return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
            Status:  http.StatusBadRequest,
            Message: "Invalid page number",
            Data:    nil,
        })
    }

    limit, err := strconv.Atoi(limitStr)
    if err != nil || limit < 1 {
        return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
            Status:  http.StatusBadRequest,
            Message: "Invalid limit",
            Data:    nil,
        })
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
            //   return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
            //     Status:  http.StatusInternalServerError,
            //     Message: "Failed to fetch container model",
            //     Data:    err.Error(),
            // })
    //     }
    // }


    // Calculate the number of documents to skip
    skip := (page - 1) * limit

    // Define find options for pagination
    findOptions := options.Find().SetSkip(int64(skip)).SetLimit(int64(limit))

    currentCollection := configs.GetCollection( schemaName)
    cursor, err := currentCollection.Find(ctx, bson.M{}, findOptions)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status:  http.StatusInternalServerError,
            Message: "Failed to fetch items",
            Data:    err.Error(),
        })
    }
    defer cursor.Close(ctx)

    var items []map[string]interface{}
    for cursor.Next(ctx) {
        var item map[string]interface{}
        if err := cursor.Decode(&item); err != nil {
            return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
                Status:  http.StatusInternalServerError,
                Message: "Failed to decode item",
                Data:    err.Error(),
            })
        }
        items = append(items, item)
    }

    // Get total count of documents in the collection
    totalItems, err := currentCollection.CountDocuments(ctx, bson.M{})
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status:  http.StatusInternalServerError,
            Message: "Failed to fetch total items",
            Data:    err.Error(),
        })
    }

    // Calculate total pages
    totalPages := int(totalItems) / limit
    if int(totalItems)%limit > 0 {
        totalPages++
    }
     result := map[string]interface{}{
            "items":       items,
            "totalPages":  totalPages,
            "totalItems":  totalItems,
            "currentPage": page,
    }
    // Return paginated result
    return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{
        Status:  http.StatusOK,
        Message: "Successfully fetched items",
        Data:    result,
    })
    }
// executeDynamicCode executes dynamic code from a request.
func ExecuteDynamicCode(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    schemaName := c.Query("schemaName")
    functionName := c.Query("functionName")

     // Serialize the current query parameters
    currentQuery := c.OriginalURL()
    // Fetch the associated container model from context
    var container *models.ContainerModel
    if storedContainer := c.Locals("containerModel"); storedContainer != nil {
        container, _ = storedContainer.(*models.ContainerModel)
    } else {
        var err error
        container, err = utils.GetContainerModel(schemaName)
        if err != nil {
              return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
                Status:  http.StatusInternalServerError,
                Message: "Failed to fetch container model",
                Data:    err.Error(),
            })
        }
    }

    pluginFileName := "temp_" + functionName + ".so"
    fileName := "temp_" + functionName + ".go"

    // generate new redis key
    redisKey, shouldCache := utils.GenerateDynamicFunctionRedisKey(schemaName, functionName, container)

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
            var result interface{}
            if err := json.Unmarshal([]byte(cachedData), &result); err == nil {
                return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{
                    Status:  http.StatusOK,
                    Message: "Function result fetched from cache",
                    Data:    result,
                    Source:  utils.PointerToString("cache"),
                })
            }
        }
    }
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
                    return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
                        Status:  http.StatusInternalServerError,
                        Message: "Failed to execute function",
                        Data:    err.Error(),
                    })
                }
                // Cache result if query hasn't changed and cache is available
                if shouldCache  {
                    var expiration time.Duration
                    if container.Redis.CacheTime > 0 {
                        expiration = time.Duration(container.Redis.CacheTime) * time.Minute
                    } else {
                        expiration = 24 * time.Hour // Default to 24 Hours
                    }
                    resultJSON, err := json.Marshal(result)
                    if err == nil {
                        configs.RedisClient.Set(ctx, redisKey, resultJSON, expiration)
                        configs.RedisClient.Set(ctx, redisKey+"-query", currentQuery, expiration)
                    }
                }
                return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{
                    Status:  http.StatusOK,
                    Message: "Function result fetched from plugin",
                    Data:    result,
                    Source:  utils.PointerToString("plugin"),
                })
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
        return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
            Status:  http.StatusBadRequest,
            Message: "Function not found",
            Data:    nil,
        })
    }

    // Write new code to file
    err = ioutil.WriteFile(fileName, []byte(dynamicFuncCode), 0644)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status:  http.StatusInternalServerError,
            Message: "Failed to write code to file",
            Data:    err.Error(),
        })
    }

    // Compile new code into plugin
    out, err := exec.Command("go", "build", "-buildmode=plugin", fileName).CombinedOutput()
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status:  http.StatusInternalServerError,
            Message: "Failed to compile code into plugin",
            Data:    string(out),
        })
    }

    // Load new plugin
    p, err = plugin.Open(pluginFileName)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status:  http.StatusInternalServerError,
            Message: "Failed to load new plugin",
            Data:    err.Error(),
        })
    }

    // Lookup function in new plugin
    f, err := p.Lookup(functionName)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status:  http.StatusInternalServerError,
            Message: "Failed to lookup function in new plugin",
            Data:    err.Error(),
        })
    }

    // Execute the function and cache the result if caching is enabled
    if executeFunc, ok := f.(func(*fiber.Ctx) (interface{}, error)); ok {
        result, err := executeFunc(c)
        if err != nil {
            return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
                Status:  http.StatusInternalServerError,
                Message: "Failed to execute function",
                Data:    err.Error(),
            })
        }

        // Cache the result
        if shouldCache {
            dataToCache, _ := json.Marshal(result)
            var expiration time.Duration
            if container.Redis.CacheTime > 0 {
                expiration = time.Duration(container.Redis.CacheTime) * time.Minute
            } else {
                expiration = 24 * time.Hour // Default to 24 Hours
            }
            configs.RedisClient.Set(ctx, redisKey, dataToCache, expiration)
            configs.RedisClient.Set(ctx, redisKey+"-query", currentQuery, expiration)
        }

        return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{
            Status:  http.StatusOK,
            Message: "Function result fetched from new plugin",
            Data:    result,
            Source:  utils.PointerToString("new plugin"),
        })
    } else {
        return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status:  http.StatusInternalServerError,
            Message: "Failed to execute function",
            Data:    nil,
        })
    }
}
// function to test pipeline before saving
func TestPipeline(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    schemaName := c.Query("schemaName")
    // Parse request body
    var requestBody models.TestPipelineRequestBody
    if err := c.BodyParser(&requestBody); err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "status":  fiber.StatusBadRequest,
            "message": "Invalid request body",
            "data":    err.Error(),
        })  
    }
    requestBody.PipelineStage.PipelineJSON = utils.ReplacePlaceholdersWithQueryParams(requestBody.PipelineStage.PipelineJSON, c)
    currentCollection := configs.GetCollection( schemaName)

    // Execute the dynamic pipeline
    items, err := utils.ExecuteDynamicPipeline(ctx, currentCollection, requestBody.PipelineStage)
    if err != nil {
        // Log the error; do not fail the server
        log.Printf("Error executing test pipeline: %v", err)
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
            "status":  fiber.StatusInternalServerError,
            "message": "Failed to execute test pipeline",
            "data":    err.Error(),
        })
    }

    // Return the results
    return c.Status(fiber.StatusOK).JSON(fiber.Map{
        "status":  fiber.StatusOK,
        "message": "Test pipeline executed successfully",
        "data":    items,
    })
}

// TODO:redis generate key and delete key will added into this function and then the route will be added and tested again
func ExecuteDynamicAPI(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    schemaName := c.Query("schemaName")
    apiName := c.Query("apiName")

    // Fetch the associated container model from context
    var container *models.ContainerModel
    if storedContainer := c.Locals("containerModel"); storedContainer != nil {
        container, _ = storedContainer.(*models.ContainerModel)
    } else {
        var err error
        container, err = utils.GetContainerModel(schemaName)
        if err != nil {
            return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
                Status:  http.StatusInternalServerError,
                Message: "Failed to fetch container model",
                Data:    err.Error(),
            })
        }
    }

    // Check if API is defined in container model
    var dynamicApi *models.DynamicApiModel
    apiExists := false
    for _, api := range container.DynamicApis {
        if api.Name == apiName {
            dynamicApi = &api
            apiExists = true
            break
        }
    }
    if !apiExists {
        return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
            Status:  http.StatusBadRequest,
            Message: "API not found",
            Data:    nil,
        })
    }

  // Validate dependencies if they exist
    var requestBody map[string]interface{}
    if err := c.BodyParser(&requestBody); err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "status":  fiber.StatusBadRequest,
            "message": "Invalid request body",
            "data":    err.Error(),
        })
    }

    if dynamicApi.Dependencies != nil && len(dynamicApi.Dependencies) > 0 {
        missingDependencies := []string{}
        for _, dependency := range dynamicApi.Dependencies {
            if value, ok := requestBody[dependency]; !ok || value == nil {
                missingDependencies = append(missingDependencies, dependency)
            }
        }
        if len(missingDependencies) > 0 {
            return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
                "status":  fiber.StatusBadRequest,
                "message": "Missing dependencies",
                "data":    missingDependencies,
            })
        }
    }

    // Generate Redis key for caching
    redisKey, shouldCache := utils.GenerateDynamicApiRedisKey(schemaName, apiName, container)
    // Attempt to fetch from cache if enabled
    if dynamicApi.IsRedisCached {
        cachedData, err := configs.RedisClient.Get(ctx, redisKey).Result()
        if err == nil {
            var result interface{}
            if err := json.Unmarshal([]byte(cachedData), &result); err == nil {
                return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{
                    Status:  http.StatusOK,
                    Message: "API result fetched from cache",
                    Data:    result,
                    Source:  utils.PointerToString("cache"),
                })
            }
        }
    }

    apiResultBytes, err := utils.ExecuteApiRequest(ctx, dynamicApi.Method, dynamicApi.Url, requestBody)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status:  http.StatusInternalServerError,
            Message: "Failed to execute API call",
            Data:    err.Error(),
        })
    }

    var apiResult interface{}
    if err := json.Unmarshal(apiResultBytes, &apiResult); err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status:  http.StatusInternalServerError,
            Message: "Failed to unmarshal API response",
            Data:    err.Error(),
        })
    }
    // Cache the result if enabled
    if shouldCache {
        var expiration time.Duration
        if dynamicApi.CacheTime > 0 {
            expiration = time.Duration(dynamicApi.CacheTime) * time.Minute
        } else {
            expiration = 24 * time.Hour // Default to 24 Hours
        }
        resultJSON, _ := json.Marshal(apiResult)
        configs.RedisClient.Set(ctx, redisKey, resultJSON, expiration)
    }

    return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{
        Status:  http.StatusOK,
        Message: "API result fetched",
        Data:    apiResult,
        Source:  utils.PointerToString("API call"),
    })
}
