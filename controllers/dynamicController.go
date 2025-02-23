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
	"strings"
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
    log.Printf("Creating item for schema: %s", schemaName)

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
			log.Printf("Failed to fetch container model for schema: %s, error: %v", schemaName, err)
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
            log.Printf("Error parsing form for schema: %s, error: %v", schemaName, err)
            return utils.SendErrorResponse(c, err, "Error parsing form.")
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
                    log.Printf("Error uploading to Cloudinary for schema: %s, error: %v", schemaName, err)
                    return utils.SendErrorResponse(c, err, "Error uploading to Cloudinary.")
                }
                fileURLs[fieldName] = imageURL
            }
        }
    } else {
        // Parse the provided item from request body
        if err := c.BodyParser(&itemMap); err != nil {
            log.Printf("Failed to parse body for schema: %s, error: %v", schemaName, err)
            return utils.SendErrorResponse(c, err, "Failed to parse body.")
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
		log.Printf("Validation failed for schema: %s, error: %v", schemaName, err)
		return utils.SendErrorResponse(c, err, "Validation failed.")
	}

    // Convert fields that should be ObjectId to ObjectId type
    for _, field := range container.Fields {
        if (field.Type == "objectId") {
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
                log.Printf("Error checking existing field value for schema: %s, error: %v", schemaName, err)
                return utils.SendErrorResponse(c, err, "Error checking existing field value.")
            }

            if count > 0 {
                log.Printf("Duplicate field value found for schema: %s, field: %s", schemaName, field.Name)
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
        log.Printf("Failed to save item for schema: %s, error: %v", schemaName, err)
        return utils.SendErrorResponse(c, err, "Failed to save item.")
    }
	// Delete the cache for this schema
if container.Redis.IsRedisCached {
    err = utils.DeleteCacheForSchema(ctx, schemaName, container)
    if err != nil {
        log.Printf("Failed to delete cache for schema: %s, error: %v", schemaName, err)
        return utils.SendErrorResponse(c, err, "Failed to delete the cache for the schema.")
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
    log.Printf("Item successfully created for schema: %s", schemaName)
    return c.Status(http.StatusCreated).JSON(responses.GeneralResponse{
        Status: http.StatusCreated, Message: "Item successfully created.", Data: result,
    })
}
func CreateMultipleDynamicModelItem(c *fiber.Ctx) error {
	// Set up a context with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schemaName := c.Query("schemaName")
	log.Printf("Creating multiple items for schema: %s", schemaName)

	// Fetch the container model (either from the context or by fetching it)
	var container *models.ContainerModel
	var err error
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		container, err = utils.GetContainerModel(schemaName)
		if err != nil {
			log.Printf("Failed to fetch container model for schema: %s, error: %v", schemaName, err)
			return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch container model",
				Data:    err.Error(),
			})
		}
	}

	// Check whether any field in the container is of type "image"
	hasImageField := false
	for _, field := range container.Fields {
		if field.Type == "image" {
			hasImageField = true
			break
		}
	}

	// This slice will hold the items to be inserted.
	var items []map[string]interface{}
	// fileURLs holds, for each image field, an array of URLs (one per item).
	fileURLs := make(map[string][]string)

	// Determine if the request is multipart.
	contentType := c.Get("Content-Type")
	if strings.Contains(contentType, "multipart/form-data") {
		// Expecting a multipart form with one field "items" (a JSON array)
		form, err := c.MultipartForm()
		if err != nil {
			log.Printf("Error parsing multipart form for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Error parsing multipart form.")
		}

		itemsJSON, exists := form.Value["items"]
		if !exists || len(itemsJSON) == 0 {
			return utils.SendErrorResponse(c, fmt.Errorf("missing items field"), "Missing items JSON field.")
		}

		// Parse the JSON array into our items slice.
		if err := json.Unmarshal([]byte(itemsJSON[0]), &items); err != nil {
			log.Printf("Error parsing items JSON for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Error parsing items JSON.")
		}

		// Process each image field.
		if hasImageField {
			// For every field declared as an image, expect that form.File contains a file for each item.
			for _, field := range container.Fields {
				if field.Type != "image" {
					continue
				}

				files, exists := form.File[field.Name]
				if !exists {
					// No files for this image field; you might decide to treat this as an error or simply skip.
					continue
				}

				if len(files) != len(items) {
					msg := fmt.Sprintf("Expected %d files for field '%s' but got %d", len(items), field.Name, len(files))
					log.Printf(msg)
					return utils.SendErrorResponse(c, fmt.Errorf(msg), msg)
				}

				// Loop over the files so that each item gets its corresponding image.
				for _, file := range files {
                    tempFilePath := "./temp/" + file.Filename
                    if err := c.SaveFile(file, tempFilePath); err != nil {
                        return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
                            Status:  http.StatusInternalServerError,
                            Message: "Error saving temp file.",
                            Data:    err.Error(),
                        })
                    }
					imageURL, err := utils.UploadToCloudinary(tempFilePath)
					os.Remove(tempFilePath) // Clean up the temp file.
					if err != nil {
						log.Printf("Error uploading image to Cloudinary for schema: %s, error: %v", schemaName, err)
						return utils.SendErrorResponse(c, err, "Error uploading image to Cloudinary.")
					}
					fileURLs[field.Name] = append(fileURLs[field.Name], imageURL)
				}
			}
		}
	} else {
		// Not a multipart request; parse the JSON array directly from the request body.
		if err := c.BodyParser(&items); err != nil {
			log.Printf("Failed to parse body for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Failed to parse request body.")
		}
	}

	// Build a set of allowed field names.
	allowedFields := make(map[string]struct{})
	for _, field := range container.Fields {
		allowedFields[field.Name] = struct{}{}
	}

	// For each item: filter out disallowed fields and, if needed, add the corresponding image URL.
	for i, item := range items {
		for key := range item {
			if _, ok := allowedFields[key]; !ok {
				delete(item, key)
			}
		}
		// If images were uploaded, replace the corresponding fields with the uploaded URLs.
		for fieldName, urls := range fileURLs {
			if i < len(urls) {
				item[fieldName] = urls[i]
			}
		}
		items[i] = item
	}

	// Validate each item against the container model.
	for i, item := range items {
		if err := utils.ValidateContainerModel(item, *container); err != nil {
			log.Printf("Validation failed for schema: %s on item index %d, error: %v", schemaName, i, err)
			return utils.SendErrorResponse(c, err, fmt.Sprintf("Validation failed for item at index %d.", i))
		}
	}

	// Convert any fields that should be ObjectIds.
	for i, item := range items {
		for _, field := range container.Fields {
			if field.Type == "objectId" {
				if strId, ok := item[field.Name].(string); ok {
					objId, err := primitive.ObjectIDFromHex(strId)
					if err == nil {
						item[field.Name] = objId
					}
				}
			}
		}
		items[i] = item
	}

	// Get the MongoDB collection for this schema.
	currentCollection := configs.GetCollection(schemaName)

	// Check for unique field constraints.
	// For each field that is marked as Unique, we check both within the request and against existing documents.
	for _, field := range container.Fields {
		if field.Unique {
			valueSet := make(map[interface{}]bool)
			var values []interface{}
			for i, item := range items {
				fieldValue, found := item[field.Name]
				if !found {
					continue
				}
				// Check duplicates within the same request.
				if _, exists := valueSet[fieldValue]; exists {
					msg := fmt.Sprintf("Duplicate value for unique field '%s' in item index %d", field.Name, i)
					log.Printf(msg)
					return c.Status(http.StatusBadRequest).JSON(fiber.Map{
						"status":  http.StatusBadRequest,
						"message": msg,
						"data":    nil,
					})
				}
				valueSet[fieldValue] = true
				values = append(values, fieldValue)
			}

			if len(values) > 0 {
				filter := bson.M{field.Name: bson.M{"$in": values}}
				count, err := currentCollection.CountDocuments(ctx, filter)
				if err != nil {
					log.Printf("Error checking unique field '%s' for schema: %s, error: %v", field.Name, schemaName, err)
					return utils.SendErrorResponse(c, err, fmt.Sprintf("Error checking unique field '%s'.", field.Name))
				}
				if count > 0 {
					msg := fmt.Sprintf("A document with the same '%s' already exists.", field.Name)
					log.Printf(msg)
					return c.Status(http.StatusBadRequest).JSON(fiber.Map{
						"status":  http.StatusBadRequest,
						"message": msg,
						"data":    nil,
					})
				}
			}
		}
	}

	// Convert the slice of items (map[string]interface{}) to a slice of interface{} for InsertMany.
	var docs []interface{}
	for _, item := range items {
		docs = append(docs, item)
	}

	// Insert all items using a bulk operation.
	result, err := currentCollection.InsertMany(ctx, docs)
	if err != nil {
		log.Printf("Failed to insert multiple items for schema: %s, error: %v", schemaName, err)
		return utils.SendErrorResponse(c, err, "Failed to insert multiple items.")
	}

	// Clear the cache for this schema.
	if container.Redis.IsRedisCached {
		err = utils.DeleteCacheForSchema(ctx, schemaName, container)
		if err != nil {
			log.Printf("Failed to delete cache for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Failed to delete the cache for the schema.")
		}

		// Also clear cache for each schema in TriggeredRedisCaches.
		for _, triggeredSchema := range container.Redis.TriggeredRedisCaches {
			err = utils.DeleteCacheForSchema(ctx, triggeredSchema, container)
			if err != nil {
				log.Printf("Error deleting cache for schema '%s': %v", triggeredSchema, err)
				// Continue with next triggered schema.
				continue
			}
		}
	}

	log.Printf("Multiple items successfully created for schema: %s", schemaName)
	return c.Status(http.StatusCreated).JSON(responses.GeneralResponse{
		Status:  http.StatusCreated,
		Message: "Multiple items successfully created.",
		Data:    result,
	})
}
// GetAllDynamicModelItems fetches all items for a given collection and performs population if needed.
func GetAllDynamicModelItems(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schemaName := c.Query("schemaName")
	log.Printf("Fetching all items for schema: %s", schemaName)
	if schemaName == "" {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Schema name is required", nil)
	}

	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		// Use the container model from the context if available.
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		var err error
		container, err = utils.GetContainerModel(schemaName)
		if err != nil {
			log.Printf("Failed to fetch container model for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Failed to fetch container model")
		}
	}

	redisKey, shouldCache := utils.GenerateRedisKey("GetAllDynamicModelItems", schemaName, container)
	if shouldCache {
		cachedData, err := configs.RedisClient.Get(ctx, redisKey).Result()
		if err == nil {
			var items []map[string]interface{}
			if err := json.Unmarshal([]byte(cachedData), &items); err == nil {
				log.Printf("Items successfully fetched from cache for schema: %s", schemaName)
				source := "cache"
				return utils.SendResponse(c, fiber.StatusOK, "Items successfully fetched.", fiber.Map{
					"items": items, "source": &source,
				})
			}
		}
		// Not found in cache: fetch from database.
		currentCollection := configs.GetCollection(schemaName)
		results, err := currentCollection.Find(ctx, bson.M{})
		if err != nil {
			log.Printf("Failed to fetch items from database for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Failed to fetch items")
		}
		defer results.Close(ctx)
		items, err := utils.DecodeCursor(results, ctx)
		if err != nil {
			return utils.SendErrorResponse(c, err, "Failed to decode items")
		}

		// Remove hashed fields from the items.
		for _, field := range container.Fields {
			if field.IsHashed {
				for i := range items {
					delete(items[i], field.Name)
				}
			}
		}

		// ----- Population logic begins -----
		if len(container.PopulationArray) > 0&& contains(container.PopulatedRoutes, "GetAllDynamicModelItems") {
			items, err = populateItems(ctx, container, items)
			if err != nil {
				log.Printf("Failed to populate items: %v", err)
				return utils.SendErrorResponse(c, err, "Failed to populate items")
			}
		}
		// ----- Population logic ends -----

		// Cache the populated data.
		dataToCache, _ := json.Marshal(items)
		var expiration time.Duration
		if container.Redis.CacheTime > 0 {
			expiration = time.Duration(container.Redis.CacheTime) * time.Minute
		} else {
			expiration = 0 // key will never expire.
		}
		configs.RedisClient.Set(ctx, redisKey, dataToCache, expiration)

		log.Printf("Items successfully fetched from database for schema: %s", schemaName)
		return utils.SendResponse(c, fiber.StatusOK, "Items successfully fetched.", items)
	}

	// If caching is not enabled, fetch directly from the database.
	currentCollection := configs.GetCollection(schemaName)
	results, err := currentCollection.Find(ctx, bson.M{})
	if err != nil {
		return utils.SendErrorResponse(c, err, "Failed to fetch items from the database")
	}
	defer results.Close(ctx)

	items, err := utils.DecodeCursor(results, ctx)
	if err != nil {
		return utils.SendErrorResponse(c, err, "Failed to decode an item")
	}

	// Remove hashed fields from the items.
	for _, field := range container.Fields {
		if field.IsHashed {
			for i := range items {
				delete(items[i], field.Name)
			}
		}
	}

	// ----- Population logic begins -----
	if len(container.PopulationArray) > 0&& contains(container.PopulatedRoutes, "GetAllDynamicModelItems") {
		items, err = populateItems(ctx, container, items)
		if err != nil {
			log.Printf("Failed to populate items: %v", err)
			return utils.SendErrorResponse(c, err, "Failed to populate items")
		}
	}
	// ----- Population logic ends -----

	log.Printf("Items successfully fetched from database for schema: %s", schemaName)
	return utils.SendResponse(c, fiber.StatusOK, "Items successfully fetched.", items)
}
// TODO: performance will be improved by adding a field in the container as usedSchemas (which will be updated when the new schema added with objectId of the currentSchema) and instead of getting all containers we will only check the neccessary containers and if the usedSchemas are empty we will not waste time with getting all containers

//delete an item from the collection
func DeleteDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Fetching the schema name from the query params
	schemaName := c.Query("schemaName")
	log.Printf("Deleting item for schema: %s", schemaName)
    
    // Attempting to convert the ID from string to ObjectID
	deleteIdStr := c.Params("id")
	deleteId, err := primitive.ObjectIDFromHex(deleteIdStr)
	if err != nil {
		log.Printf("Provided ID is not in the valid format for schema: %s, error: %v", schemaName, err)
		return utils.SendErrorResponse(c, err, "Provided ID is not in the valid format.")
	}

	// Define Redis Lock Key to prevent concurrent deletions of the same item
	lockKey := fmt.Sprintf("lock:delete:%s:%s", schemaName, deleteId.Hex())

	// Acquire Redis Lock (expires in 10 seconds)
	lockID, locked := utils.AcquireLock(lockKey, 10*time.Second)
	if !locked {
		log.Printf("Another process is already deleting this item for schema: %s", schemaName)
		return c.Status(http.StatusConflict).JSON(responses.GeneralResponse{
			Status:  http.StatusConflict,
			Message: "Another process is already deleting this item",
			Data:    nil,
		})
	}
	defer utils.ReleaseLock(lockKey, lockID) // Ensure lock is released after execution

    //check if schema is used as objectId in other containers
    allContainers, err := utils.GetAllContainerModels()
        if err != nil {
			log.Printf("Failed to retrieve container models for schema: %s, error: %v", schemaName, err)
            return utils.SendErrorResponse(c, err, "Failed to retrieve container models.")
        }
	// First check if any reference prevents deletion
	for _, container := range allContainers {
		if container.SchemaName != schemaName { // Skip the current schema
			for _, field := range container.Fields {
				if field.Type == "objectId" && field.Name == schemaName { // Field referencing the schema as an objectId
					collection := configs.GetCollection(container.SchemaName)
					count, err := collection.CountDocuments(ctx, bson.M{field.Name: deleteId})
					if err != nil {
						log.Printf("Database error while checking references for schema: %s, error: %v", schemaName, err)
						return utils.SendErrorResponse(c, err, "Database error while checking references.")
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
						log.Printf("Failed to force delete referenced items for schema: %s, error: %v", schemaName, delErr)
						return utils.SendErrorResponse(c, delErr, "Failed to force delete referenced items.")
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
			log.Printf("Failed to fetch container model for schema: %s, error: %v", schemaName, err)
              return utils.SendErrorResponse(c, err, "Failed to fetch container model")
        }
    }
	
	// Using the schema name to determine the appropriate collection
	var currentCollection *mongo.Collection = configs.GetCollection( schemaName)

	
	// Attempting to delete the item with the given ID from the specified collection
	result, err := currentCollection.DeleteOne(ctx, bson.M{"_id": deleteId})
	if err != nil {
		log.Printf("Failed to delete the item from the specified collection for schema: %s, error: %v", schemaName, err)
		return utils.SendErrorResponse(c, err, "Failed to delete the item from the specified collection.")
	}
	// Now attempting to delete the related cache
if container.Redis.IsRedisCached {
    err = utils.DeleteCacheForSchema(ctx, schemaName, container)
    if err != nil {
		log.Printf("Failed to delete cache for schema: %s, error: %v", schemaName, err)
        return utils.SendErrorResponse(c, err, "Failed to delete the cache for the schema.")
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
	log.Printf("Item successfully deleted for schema: %s", schemaName)
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Item successfully deleted from the specified collection.",
		Data:   result,
	})
}

func DeleteMultipleDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schemaName := c.Query("schemaName")
	log.Printf("Deleting multiple items for schema: %s", schemaName)

	// Parse the request body as a JSON array.
	var items []map[string]interface{}
	if err := c.BodyParser(&items); err != nil {
		log.Printf("Failed to parse request body for schema: %s, error: %v", schemaName, err)
		return utils.SendErrorResponse(c, err, "Failed to parse request body")
	}

	// Retrieve all container models (to check for references).
	allContainers, err := utils.GetAllContainerModels()
	if err != nil {
		log.Printf("Failed to retrieve container models for schema: %s, error: %v", schemaName, err)
		return utils.SendErrorResponse(c, err, "Failed to retrieve container models")
	}

	// Get the container model for the current schema.
	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		container, err = utils.GetContainerModel(schemaName)
		if err != nil {
			log.Printf("Failed to fetch container model for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Failed to fetch container model")
		}
	}

	// Get the collection for the current schema.
	currentCollection := configs.GetCollection(schemaName)

	// Slices to keep track of successful and failed deletions.
	var successfulDeletes []interface{}
	var failedDeletes []map[string]interface{}

	// Iterate over each deletion request.
	for _, item := range items {
		// Extract the ID from "id" or "_id".
		var idStr string
		if v, ok := item["id"]; ok {
			idStr, ok = v.(string)
			if !ok {
				failedDeletes = append(failedDeletes, map[string]interface{}{
					"item":  item,
					"error": "Invalid id format, expected string",
				})
				continue
			}
		} else if v, ok := item["_id"]; ok {
			idStr, ok = v.(string)
			if !ok {
				failedDeletes = append(failedDeletes, map[string]interface{}{
					"item":  item,
					"error": "Invalid _id format, expected string",
				})
				continue
			}
		} else {
			failedDeletes = append(failedDeletes, map[string]interface{}{
				"item":  item,
				"error": "Missing id field",
			})
			continue
		}

		// Convert the id string to an ObjectID.
		deleteId, err := primitive.ObjectIDFromHex(idStr)
		if err != nil {
			failedDeletes = append(failedDeletes, map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": "Provided ID is not in the valid format",
			})
			continue
		}

		// Acquire a Redis lock for this deletion to prevent concurrent deletions.
		lockKey := fmt.Sprintf("lock:delete:%s:%s", schemaName, deleteId.Hex())
		lockID, locked := utils.AcquireLock(lockKey, 10*time.Second)
		if !locked {
			failedDeletes = append(failedDeletes, map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": "Another process is already deleting this item",
			})
			continue
		}
		// Remember to release the lock manually (cannot use defer in a loop).

		// Check if this item is referenced in other containers.
		referencePreventDeletion := false
		for _, otherContainer := range allContainers {
			// Skip the current schema.
			if otherContainer.SchemaName == schemaName {
				continue
			}
			// Check each field in the other container.
			for _, field := range otherContainer.Fields {
				// If the field references the current schema.
				if field.Type == "objectId" && field.Name == schemaName {
					coll := configs.GetCollection(otherContainer.SchemaName)
					count, err := coll.CountDocuments(ctx, bson.M{field.Name: deleteId})
					if err != nil {
						utils.ReleaseLock(lockKey, lockID)
						failedDeletes = append(failedDeletes, map[string]interface{}{
							"id":    idStr,
							"item":  item,
							"error": "Database error while checking references: " + err.Error(),
						})
						referencePreventDeletion = true
						break
					}
					// If references exist and force deletion is not enabled, record an error.
					if count > 0 && !field.IsForceDelete {
						utils.ReleaseLock(lockKey, lockID)
						failedDeletes = append(failedDeletes, map[string]interface{}{
							"id":    idStr,
							"item":  item,
							"error": fmt.Sprintf("Cannot delete: This item is still referenced in schema '%s' and cannot be forcibly deleted", otherContainer.SchemaName),
						})
						referencePreventDeletion = true
						break
					}
				}
			}
			if referencePreventDeletion {
				break
			}
		}
		if referencePreventDeletion {
			continue
		}

		// For any references that allow force deletion, remove the referenced items.
		for _, otherContainer := range allContainers {
			if otherContainer.SchemaName == schemaName {
				continue
			}
			for _, field := range otherContainer.Fields {
				if field.Type == "objectId" && field.Name == schemaName && field.IsForceDelete {
					coll := configs.GetCollection(otherContainer.SchemaName)
					if _, err := coll.DeleteMany(ctx, bson.M{field.Name: deleteId}); err != nil {
						// Log error but continue with deletion.
						log.Printf("Failed to force delete referenced items for schema: %s, error: %v", schemaName, err)
					}
				}
			}
		}

		// Attempt deletion from the current collection.
		result, err := currentCollection.DeleteOne(ctx, bson.M{"_id": deleteId})
		// Release the lock immediately.
		utils.ReleaseLock(lockKey, lockID)
		if err != nil {
			failedDeletes = append(failedDeletes, map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": "Failed to delete item: " + err.Error(),
			})
			continue
		}
		if result.DeletedCount == 0 {
			failedDeletes = append(failedDeletes, map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": "No item found with the specified ID",
			})
			continue
		}

		// Record a successful deletion.
		successfulDeletes = append(successfulDeletes, map[string]interface{}{
			"id":     idStr,
			"result": result,
		})
	}

	// Clear the cache for this schema if caching is enabled.
	if container.Redis.IsRedisCached {
		if err = utils.DeleteCacheForSchema(ctx, schemaName, container); err != nil {
			log.Printf("Failed to delete cache for schema: %s, error: %v", schemaName, err)
		}
		for _, triggeredSchema := range container.Redis.TriggeredRedisCaches {
			if err = utils.DeleteCacheForSchema(ctx, triggeredSchema, container); err != nil {
				log.Printf("Error deleting cache for schema %s: %v", triggeredSchema, err)
			}
		}
	}

	// Prepare the final response containing both successes and failures.
	responseData := fiber.Map{
		"successful": successfulDeletes,
		"failed":     failedDeletes,
	}
	log.Printf("Multiple deletion process completed for schema: %s", schemaName)
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Multiple deletion process completed",
		Data:    responseData,
	})
}

//update an item in the collection
func UpdateDynamicModelItem(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    schemaName := c.Query("schemaName")
    log.Printf("Updating item for schema: %s", schemaName)
    updateIdStr := c.Params("id")
    updateId, err := primitive.ObjectIDFromHex(updateIdStr)
    if err != nil {
		log.Printf("Provided ID is not in the valid format for schema: %s, error: %v", schemaName, err)
        return utils.SendErrorResponse(c, err, "Provided ID is not in the valid format.")
    }
    // Define Redis Lock Key to prevent concurrent updates of the same item
    lockKey := fmt.Sprintf("lock:update:%s:%s", schemaName, updateId.Hex())

    // Acquire Redis Lock (expires in 10 seconds)
    lockID, locked := utils.AcquireLock(lockKey, 10*time.Second)
    if !locked {
		log.Printf("Another process is already updating this item for schema: %s", schemaName)
        return c.Status(http.StatusConflict).JSON(responses.GeneralResponse{
            Status:  http.StatusConflict,
            Message: "Another process is already updating this item",
            Data:    nil,
        })
    }
    defer utils.ReleaseLock(lockKey, lockID) // Ensure lock is released after execution

    // Fetch the associated container model
	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		// Use the container model from the context if available
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		// Fetch container model if not available in context
		container, err = utils.GetContainerModel(schemaName)
		if err != nil {
			log.Printf("Failed to fetch container model for schema: %s, error: %v", schemaName, err)
			  return utils.SendErrorResponse(c, err, "Failed to fetch container model")
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
			log.Printf("Error in multipart form for schema: %s, error: %v", schemaName, err)
            return utils.SendErrorResponse(c, err, "Error in multipart form")
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
					log.Printf("Error uploading to Cloudinary for schema: %s, error: %v", schemaName, err)
                    return utils.SendErrorResponse(c, err, "Error uploading to Cloudinary")
                }
                fileURLs[fieldName] = imageURL
            }
        }
    } else {
        // Handle JSON body
        if err := c.BodyParser(&updatedItemMap); err != nil {
			log.Printf("Failed to parse body for schema: %s, error: %v", schemaName, err)
            return utils.SendErrorResponse(c, err, "Failed to parse body")
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
		log.Printf("Failed to fetch item for schema: %s, error: %v", schemaName, err)
        return utils.SendErrorResponse(c, err, "Failed to fetch item")
    }
    // Apply updates from updatedItemMap to existingItem
    for key, value := range updatedItemMap {
        existingItem[key] = value
    }

    // Validation
    err = utils.ValidateContainerModel(existingItem, *container)
    if err != nil {
		log.Printf("Validation failed for schema: %s, error: %v", schemaName, err)
        return utils.SendErrorResponse(c, err, "Validation failed")
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
				log.Printf("Error checking existing field value for schema: %s, error: %v", schemaName, err)
                return utils.SendErrorResponse(c, err, "Error checking existing field value.")
            }

            if count > 0 {
				log.Printf("Duplicate field value found for schema: %s, field: %s", schemaName, field.Name)
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
		log.Printf("Failed to update item for schema: %s, error: %v", schemaName, err)
        return utils.SendErrorResponse(c, err, "Failed to update item")
    }

    if updateResult.MatchedCount == 0 {
		log.Printf("No item found with specified ID for schema: %s", schemaName)
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

	log.Printf("Item successfully updated for schema: %s", schemaName)
    return c.Status(http.StatusOK).JSON(responses.GeneralResponse{Status: http.StatusOK, Message: "Item successfully updated", Data: updateResult})
}
func UpdateMultipleDynamicModelItem(c *fiber.Ctx) error {
	// Set a timeout for the operation.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schemaName := c.Query("schemaName")
	log.Printf("Updating multiple items for schema: %s", schemaName)

	// Fetch container model from context or via a helper function.
	var container *models.ContainerModel
	var err error
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		container, err = utils.GetContainerModel(schemaName)
		if err != nil {
			log.Printf("Failed to fetch container model for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Failed to fetch container model")
		}
	}

	// Determine if any field is of type "image"
	hasImageField := false
	for _, field := range container.Fields {
		if field.Type == "image" {
			hasImageField = true
			break
		}
	}

	// items will hold the update objects.
	var items []map[string]interface{}
	// fileURLs holds, for each image field, a slice of URLs (one per item).
	fileURLs := make(map[string][]string)

	// Process input based on content type.
	contentType := c.Get("Content-Type")
	if strings.Contains(contentType, "multipart/form-data") {
		// Expect a multipart form with one field "items" containing the JSON array.
		form, err := c.MultipartForm()
		if err != nil {
			log.Printf("Error parsing multipart form for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Error parsing multipart form")
		}

		itemsJSON, exists := form.Value["items"]
		if !exists || len(itemsJSON) == 0 {
			return utils.SendErrorResponse(c, fmt.Errorf("missing items field"), "Missing items JSON field")
		}
		if err := json.Unmarshal([]byte(itemsJSON[0]), &items); err != nil {
			log.Printf("Error parsing items JSON for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Error parsing items JSON")
		}

		// Process image fields if any.
		if hasImageField {
			for _, field := range container.Fields {
				if field.Type != "image" {
					continue
				}
				files, exists := form.File[field.Name]
				if !exists {
					// No file for this image field; decide whether to skip or error out.
					continue
				}
				// Make sure we have one file per item.
				if len(files) != len(items) {
					msg := fmt.Sprintf("Expected %d files for field '%s' but got %d", len(items), field.Name, len(files))
					log.Printf(msg)
					return utils.SendErrorResponse(c, fmt.Errorf(msg), msg)
				}
				// Loop over the files so that each item gets its corresponding image.
				for _, file := range files {
                tempFilePath := "./temp/" + file.Filename
                if err := c.SaveFile(file, tempFilePath); err != nil {
                    return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Error saving temp file.",  Data: err.Error()})
                }
					imageURL, err := utils.UploadToCloudinary(tempFilePath)
					// Clean up the temp file.
					os.Remove(tempFilePath)
					if err != nil {
						log.Printf("Error uploading image to Cloudinary for schema: %s, error: %v", schemaName, err)
						return utils.SendErrorResponse(c, err, "Error uploading image to Cloudinary")
					}
					fileURLs[field.Name] = append(fileURLs[field.Name], imageURL)
				}
			}
		}
	} else {
		// Otherwise, assume a JSON array in the request body.
		if err := c.BodyParser(&items); err != nil {
			log.Printf("Failed to parse body for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Failed to parse request body")
		}
	}

	// Build a set of allowed field names from the container model.
	allowedFields := make(map[string]struct{})
	for _, field := range container.Fields {
		allowedFields[field.Name] = struct{}{}
	}

	// Prepare slices for successful and failed update results.
	var successfulUpdates []interface{}
	var failedUpdates []map[string]interface{}

	// Get the collection for this schema.
	currentCollection := configs.GetCollection(schemaName)

	// Process each update item individually.
	for index, item := range items {
		// Expect each update item to include an ID.
		var idStr string
		if v, ok := item["id"]; ok {
			idStr, ok = v.(string)
			if !ok {
				failedUpdates = append(failedUpdates, map[string]interface{}{
					"item":  item,
					"error": "Invalid id format, expected string",
				})
				continue
			}
		} else if v, ok := item["_id"]; ok {
			idStr, ok = v.(string)
			if !ok {
				failedUpdates = append(failedUpdates, map[string]interface{}{
					"item":  item,
					"error": "Invalid _id format, expected string",
				})
				continue
			}
		} else {
			failedUpdates = append(failedUpdates, map[string]interface{}{
				"item":  item,
				"error": "Missing id field",
			})
			continue
		}

		updateId, err := primitive.ObjectIDFromHex(idStr)
		if err != nil {
			failedUpdates = append(failedUpdates, map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": "Provided ID is not in the valid format",
			})
			continue
		}

		// Acquire a Redis lock for this item to prevent concurrent updates.
		lockKey := fmt.Sprintf("lock:update:%s:%s", schemaName, updateId.Hex())
		lockID, locked := utils.AcquireLock(lockKey, 10*time.Second)
		if !locked {
			failedUpdates = append(failedUpdates, map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": "Another process is already updating this item",
			})
			continue
		}
		// IMPORTANT: We cannot use defer inside a loop—release the lock as soon as the update is done.

		// Remove the id fields from the update map so they are not updated.
		delete(item, "id")
		delete(item, "_id")

		// Remove any fields that are not allowed.
		for key := range item {
			if _, exists := allowedFields[key]; !exists {
				delete(item, key)
			}
		}

		// If files were uploaded for image fields, update the corresponding fields.
		for fieldName, urls := range fileURLs {
			if index < len(urls) {
				item[fieldName] = urls[index]
			}
		}

		// Fetch the existing item from the database.
		var existingItem bson.M
		err = currentCollection.FindOne(ctx, bson.M{"_id": updateId}).Decode(&existingItem)
		if err != nil {
			utils.ReleaseLock(lockKey, lockID)
			failedUpdates = append(failedUpdates, map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": "Failed to fetch existing item: " + err.Error(),
			})
			continue
		}

		// Merge update fields into the existing document.
		for key, value := range item {
			existingItem[key] = value
		}

		// Validate the merged document.
		if err = utils.ValidateContainerModel(existingItem, *container); err != nil {
			utils.ReleaseLock(lockKey, lockID)
			failedUpdates = append(failedUpdates, map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": "Validation failed: " + err.Error(),
			})
			continue
		}

		// Check unique constraints for updated fields.
		for _, field := range container.Fields {
			if field.Unique {
				if fieldValue, found := item[field.Name]; found {
					// Exclude the current document from the uniqueness check.
					filter := bson.M{
						field.Name: fieldValue,
						"_id":      bson.M{"$ne": updateId},
					}
					count, err := currentCollection.CountDocuments(ctx, filter)
					if err != nil {
						utils.ReleaseLock(lockKey, lockID)
						failedUpdates = append(failedUpdates, map[string]interface{}{
							"id":    idStr,
							"item":  item,
							"error": "Error checking unique field: " + err.Error(),
						})
						continue
					}
					if count > 0 {
						utils.ReleaseLock(lockKey, lockID)
						failedUpdates = append(failedUpdates, map[string]interface{}{
							"id":    idStr,
							"item":  item,
							"error": fmt.Sprintf("A document with the same %s already exists", field.Name),
						})
						continue
					}
				}
			}
		}

		// Convert fields that should be ObjectId if needed.
		for _, field := range container.Fields {
			if field.Type == "objectId" {
				if strVal, ok := existingItem[field.Name].(string); ok {
					if objId, err := primitive.ObjectIDFromHex(strVal); err == nil {
						existingItem[field.Name] = objId
					}
				}
			}
		}

		// Perform the update.
		updateResult, err := currentCollection.UpdateOne(ctx, bson.M{"_id": updateId}, bson.M{"$set": existingItem})
		// Release the Redis lock.
		utils.ReleaseLock(lockKey, lockID)
		if err != nil {
			failedUpdates = append(failedUpdates, map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": "Failed to update item: " + err.Error(),
			})
			continue
		}
		if updateResult.MatchedCount == 0 {
			failedUpdates = append(failedUpdates, map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": "No matching item found to update",
			})
			continue
		}

		// Record a successful update.
		successfulUpdates = append(successfulUpdates, map[string]interface{}{
			"id":     idStr,
			"result": updateResult,
		})
	}

	// After processing all items, clear the cache for the schema.
	if container.Redis.IsRedisCached {
		err = utils.DeleteCacheForSchema(ctx, schemaName, container)
		if err != nil {
			log.Printf("Failed to delete cache for schema: %s, error: %v", schemaName, err)
		}
		for _, triggeredSchema := range container.Redis.TriggeredRedisCaches {
			err = utils.DeleteCacheForSchema(ctx, triggeredSchema, container)
			if err != nil {
				log.Printf("Error deleting cache for schema %s: %v", triggeredSchema, err)
			}
		}
	}

	// Prepare the final response containing both successes and failures.
	responseData := fiber.Map{
		"successful": successfulUpdates,
		"failed":     failedUpdates,
	}

	log.Printf("Multiple items update completed for schema: %s", schemaName)
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Multiple items update completed",
		Data:    responseData,
	})
}

func GetDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schemaName := c.Query("schemaName")
	log.Printf("Fetching item for schema: %s", schemaName)
	if schemaName == "" {
		log.Printf("Schema name is required")
		return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  fiber.StatusBadRequest,
			Message: "Schema name is required",
			Data:    nil,
		})
	}

	getIdStr := c.Params("id")
	getId, err := primitive.ObjectIDFromHex(getIdStr)
	if err != nil {
		log.Printf("Invalid ID for schema: %s, error: %v", schemaName, err)
		return utils.SendErrorResponse(c, err, "Invalid ID")
	}

	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		// Use the container model from the context if available
		container, _ = storedContainer.(*models.ContainerModel)
		log.Printf("Using container model from context for schema: %s", schemaName)
	} else {
		// Fetch container model if not available in context
		container, err = utils.GetContainerModel(schemaName)
		if err != nil {
			log.Printf("Failed to fetch container model for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Failed to fetch container model")
		}
		log.Printf("Fetched container model for schema: %s", schemaName)
	}

	redisKey, shouldCache := utils.GenerateRedisKey("GetDynamicModelItem", schemaName, container)
	if shouldCache {
		cachedData, err := configs.RedisClient.Get(ctx, redisKey).Result()
		if err == nil {
			var item bson.M
			if err := json.Unmarshal([]byte(cachedData), &item); err == nil {
				// Remove any fields marked as hashed.
				for _, field := range container.Fields {
					if field.IsHashed {
						delete(item, field.Name)
					}
				}
				// ----- Population logic begins -----
				if len(container.PopulationArray) > 0 && contains(container.PopulatedRoutes, "GetDynamicModelItem") {
					// Wrap the item in a slice to reuse the populateItems helper.
					items, err := populateItems(ctx, container, []map[string]interface{}{item})
					if err != nil {
						log.Printf("Population error on cached item: %v", err)
					} else if len(items) > 0 {
						item = items[0]
					}
				}
				// ----- Population logic ends -----
				log.Printf("Item successfully fetched from cache for schema: %s", schemaName)
				return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{
					Status:  http.StatusOK,
					Message: "Item found",
					Data:    item,
					Source:  utils.PointerToString("cache"),
				})
			}
		}
	}

	currentCollection := configs.GetCollection(schemaName)
	var result bson.M
	if err := currentCollection.FindOne(ctx, bson.M{"_id": getId}).Decode(&result); err != nil {
		log.Printf("Item not found for schema: %s, error: %v", schemaName, err)
		return utils.SendErrorResponse(c, err, "Item not found")
	}

	// Remove hashed fields from the retrieved result.
	for _, field := range container.Fields {
		if field.IsHashed {
			delete(result, field.Name)
		}
	}

	// ----- Population logic begins -----
	if len(container.PopulationArray) > 0 && contains(container.PopulatedRoutes, "GetDynamicModelItem") {
		// Wrap the result in a slice to reuse the populateItems helper.
		items, err := populateItems(ctx, container, []map[string]interface{}{result})
		if err != nil {
			log.Printf("Failed to populate item for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Failed to populate item")
		}
		if len(items) > 0 {
			result = items[0]
		}
	}
	// ----- Population logic ends -----

	if shouldCache {
		dataToCache, _ := json.Marshal(result)
		var expiration time.Duration
		if container.Redis.CacheTime > 0 {
			expiration = time.Duration(container.Redis.CacheTime) * time.Minute
		} else {
			expiration = 0 // the key will never expire.
		}
		configs.RedisClient.Set(ctx, redisKey, dataToCache, expiration)
		log.Printf("Item cached for schema: %s with key: %s", schemaName, redisKey)
	}

	log.Printf("Item successfully fetched from database for schema: %s", schemaName)
	return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Item found",
		Data:    result,
		Source:  utils.PointerToString("database"),
	})
}

// handleSearch for a given collection
func HandleSearchDynamicModelItem(c *fiber.Ctx) error {
	// Create a context with a timeout for database operations.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schemaName := c.Query("schemaName")
	searchKey := c.Query("searchKey")
	log.Printf("Searching items for schema: %s with search key: %s", schemaName, searchKey)

	// Fetch the associated container model.
	var container *models.ContainerModel
	var err error
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		container, err = utils.GetContainerModel(schemaName)
		if err != nil {
			log.Printf("Failed to fetch container model for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Failed to fetch container model")
		}
	}

	// Build the query filter using the container's fields, skipping hashed fields.
	var orQueries []bson.M
	for _, field := range container.Fields {
		// Skip fields that are marked as hashed.
		if field.IsHashed {
			continue
		}
		switch field.Type {
		case "string":
			pattern := ".*" + searchKey + ".*"
			regex := primitive.Regex{Pattern: pattern, Options: "i"} // Case-insensitive search.
			orQueries = append(orQueries, bson.M{field.Name: regex})
		case "int", "float":
			if num, err := strconv.ParseFloat(searchKey, 64); err == nil {
				orQueries = append(orQueries, bson.M{field.Name: num})
			}
		case "boolean":
			if boolVal, err := strconv.ParseBool(searchKey); err == nil {
				orQueries = append(orQueries, bson.M{field.Name: boolVal})
			}
		case "date":
			if dateVal, err := time.Parse(time.RFC3339, searchKey); err == nil {
				orQueries = append(orQueries, bson.M{field.Name: dateVal})
			}
		case "objectId":
			if objId, err := primitive.ObjectIDFromHex(searchKey); err == nil {
				orQueries = append(orQueries, bson.M{field.Name: objId})
			}
		}
	}

	if len(orQueries) == 0 {
		log.Printf("No valid search queries could be constructed for schema: %s with search key: %s", schemaName, searchKey)
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "No valid search queries could be constructed from the provided input.",
			Data:    nil,
		})
	}

	// Build the final filter using a $or operator.
	filter := bson.M{"$or": orQueries}

	// Prepare find options.
	findOptions := options.Find()

	// Apply sorting if both "sort" and "asc" are provided.
	sortField := c.Query("sort")
	ascStr := c.Query("asc")
	if sortField != "" && ascStr != "" {
		asc, err := strconv.ParseBool(ascStr)
		if err != nil {
			return utils.SendErrorResponse(c, err, "Invalid asc parameter; must be true or false")
		}
		order := 1
		if !asc {
			order = -1
		}
		findOptions.SetSort(bson.D{{Key: sortField, Value: order}})
	}

	// Get the current collection.
	currentCollection := configs.GetCollection(schemaName)

	// Check if pagination parameters are provided.
	pageStr := c.Query("page")
	limitStr := c.Query("limit")
	if pageStr != "" && limitStr != "" {
		// Parse and validate pagination parameters.
		page, err := strconv.Atoi(pageStr)
		if err != nil || page < 1 {
			return utils.SendErrorResponse(c, err, "Invalid page number")
		}
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			return utils.SendErrorResponse(c, err, "Invalid limit")
		}
		skip := (page - 1) * limit
		findOptions.SetSkip(int64(skip)).SetLimit(int64(limit))

		// Fetch the paginated and sorted search results.
		cursor, err := currentCollection.Find(ctx, filter, findOptions)
		if err != nil {
			log.Printf("Error fetching paginated search results for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Error fetching paginated search results")
		}
		defer cursor.Close(ctx)

		var items []map[string]interface{}
		for cursor.Next(ctx) {
			var item map[string]interface{}
			if err = cursor.Decode(&item); err != nil {
				log.Printf("Error decoding search result for schema: %s, error: %v", schemaName, err)
				return utils.SendErrorResponse(c, err, "Error decoding search result")
			}
			items = append(items, item)
		}

		// Remove hashed fields from the items.
		for _, field := range container.Fields {
			if field.IsHashed {
				for i := range items {
					delete(items[i], field.Name)
				}
			}
		}

		// ----- Population logic begins -----
		if len(container.PopulationArray) > 0&& contains(container.PopulatedRoutes, "HandleSearchDynamicModelItem") {
			items, err = populateItems(ctx, container, items)
			if err != nil {
				log.Printf("Failed to populate items: %v", err)
				return utils.SendErrorResponse(c, err, "Failed to populate items")
			}
		}
		// ----- Population logic ends -----

		// Retrieve total count of matching documents for pagination metadata.
		totalItems, err := currentCollection.CountDocuments(ctx, filter)
		if err != nil {
			return utils.SendErrorResponse(c, err, "Error counting search results")
		}
		totalPages := int(totalItems) / limit
		if int(totalItems)%limit > 0 {
			totalPages++
		}

		return utils.SendResponse(c, http.StatusOK, "Paginated search results fetched successfully", fiber.Map{
			"items":       items,
			"totalItems":  totalItems,
			"totalPages":  totalPages,
			"currentPage": page,
		})
	}

	// If pagination parameters are not provided, fetch all matching (and sorted) results.
	cursor, err := currentCollection.Find(ctx, filter, findOptions)
	if err != nil {
		log.Printf("Error fetching search results for schema: %s, error: %v", schemaName, err)
		return utils.SendErrorResponse(c, err, "Error fetching search results")
	}
	defer cursor.Close(ctx)

	var items []map[string]interface{}
	for cursor.Next(ctx) {
		var item map[string]interface{}
		if err = cursor.Decode(&item); err != nil {
			log.Printf("Error decoding search result for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Error decoding search result")
		}
		items = append(items, item)
	}

	// Remove hashed fields from the items.
	for _, field := range container.Fields {
		if field.IsHashed {
			for i := range items {
				delete(items[i], field.Name)
			}
		}
	}

	// ----- Population logic begins -----
	if len(container.PopulationArray) > 0&& contains(container.PopulatedRoutes, "HandleSearchDynamicModelItem") {
		items, err = populateItems(ctx, container, items)
		if err != nil {
			log.Printf("Failed to populate items: %v", err)
			return utils.SendErrorResponse(c, err, "Failed to populate items")
		}
	}
	// ----- Population logic ends -----

	log.Printf("Search results successfully fetched for schema: %s", schemaName)
	return utils.SendResponse(c, http.StatusOK, "Search results fetched successfully", items)
}

// HandleFilterDynamicModelItem filters items for a given collection using dynamic query parameters.
func HandleFilterDynamicModelItem(c *fiber.Ctx) error {
	// Create a context with a timeout for database operations.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schemaName := c.Query("schemaName")
	log.Printf("Filtering items for schema: %s", schemaName)

	// Fetch or retrieve the container model.
	var container *models.ContainerModel
	var err error
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		container, err = utils.GetContainerModel(schemaName)
		if err != nil {
			return utils.SendErrorResponse(c, err, "Failed to fetch container model")
		}
	}

	// Build the MongoDB filter from the container's fields.
	// Skip any field that is marked as hashed.
	filter := bson.M{}
	for _, field := range container.Fields {
		// Skip hashed fields for filtering.
		if field.IsHashed {
			continue
		}
		queryValue := c.Query(field.Name)
		if queryValue == "" {
			continue
		}

		convertedValue, err := utils.ConvertQueryValueToFieldType(field.Name, field.Type, queryValue)
		if err != nil {
			return utils.SendErrorResponse(c, err, err.Error())
		}

		// Merge condition if converted value is a map.
		if convertedMap, ok := convertedValue.(bson.M); ok {
			if existingCondition, exists := filter[field.Name]; exists {
				if existingMap, ok := existingCondition.(bson.M); ok {
					for key, val := range convertedMap {
						existingMap[key] = val
					}
					continue
				}
			}
		}
		filter[field.Name] = convertedValue
	}

	currentCollection := configs.GetCollection(schemaName)

	// Always create find options to support sorting even without pagination.
	findOptions := options.Find()

	// Apply sorting if both "sort" and "asc" are provided.
	sortField := c.Query("sort")
	ascStr := c.Query("asc")
	if sortField != "" && ascStr != "" {
		asc, err := strconv.ParseBool(ascStr)
		if err != nil {
			return utils.SendErrorResponse(c, err, "Invalid asc parameter; must be true or false")
		}
		order := 1
		if !asc {
			order = -1
		}
		findOptions.SetSort(bson.D{{Key: sortField, Value: order}})
	}

	// Check if pagination parameters are provided.
	pageStr := c.Query("page")
	limitStr := c.Query("limit")
	if pageStr != "" && limitStr != "" {
		// Validate and parse pagination parameters.
		page, err := strconv.Atoi(pageStr)
		if err != nil || page < 1 {
			return utils.SendErrorResponse(c, err, "Invalid page number")
		}
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			return utils.SendErrorResponse(c, err, "Invalid limit")
		}
		skip := (page - 1) * limit
		findOptions.SetSkip(int64(skip)).SetLimit(int64(limit))

		cursor, err := currentCollection.Find(ctx, filter, findOptions)
		if err != nil {
			log.Printf("Error fetching paginated filter results for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Error fetching paginated filter results")
		}
		defer cursor.Close(ctx)

		items, err := utils.DecodeCursor(cursor, ctx)
		if err != nil {
			return utils.SendErrorResponse(c, err, "Error decoding paginated filter results")
		}

		// Remove hashed fields from the items.
		for _, field := range container.Fields {
			if field.IsHashed {
				for i := range items {
					delete(items[i], field.Name)
				}
			}
		}

		// ----- Population logic begins -----
		if len(container.PopulationArray) > 0 && contains(container.PopulatedRoutes, "HandleFilterDynamicModelItem") {
			items, err = populateItems(ctx, container, items)
			if err != nil {
				log.Printf("Failed to populate items: %v", err)
				return utils.SendErrorResponse(c, err, "Failed to populate items")
			}
		}
		// ----- Population logic ends -----

		// Retrieve the total number of documents for pagination metadata.
		totalItems, err := currentCollection.CountDocuments(ctx, filter)
		if err != nil {
			return utils.SendErrorResponse(c, err, "Error counting filtered documents")
		}

		totalPages := int(totalItems) / limit
		if int(totalItems)%limit > 0 {
			totalPages++
		}

		return utils.SendResponse(c, http.StatusOK, "Paginated filter results fetched successfully", fiber.Map{
			"items":       items,
			"totalItems":  totalItems,
			"totalPages":  totalPages,
			"currentPage": page,
		})
	}

	// If pagination parameters are not provided, fetch all filtered (and sorted) results.
	cursor, err := currentCollection.Find(ctx, filter, findOptions)
	if err != nil {
		log.Printf("Error fetching filter results for schema: %s, error: %v", schemaName, err)
		return utils.SendErrorResponse(c, err, "Error fetching filter results")
	}
	defer cursor.Close(ctx)

	items, err := utils.DecodeCursor(cursor, ctx)
	if err != nil {
		return utils.SendErrorResponse(c, err, "Error decoding filter results")
	}

	// Remove hashed fields from the returned items.
	for _, field := range container.Fields {
		if field.IsHashed {
			for i := range items {
				delete(items[i], field.Name)
			}
		}
	}

	// ----- Population logic begins -----
	if len(container.PopulationArray) > 0 && contains(container.PopulatedRoutes, "HandleFilterDynamicModelItem") {
		items, err = populateItems(ctx, container, items)
		if err != nil {
			log.Printf("Failed to populate items: %v", err)
			return utils.SendErrorResponse(c, err, "Failed to populate items")
		}
	}
	// ----- Population logic ends -----

	return utils.SendResponse(c, http.StatusOK, "Filter results fetched successfully", items)
}

//get all item for given collection
func GetPipeline(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // Fetching the schema name and pipeline name from the query params
    schemaName := c.Query("schemaName")
    pipelineName := c.Query("pipelineName")
	log.Printf("Fetching pipeline for schema: %s with pipeline name: %s", schemaName, pipelineName)

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
            return utils.SendErrorResponse(c, err, "Failed to fetch container model")
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
        return utils.SendErrorResponse(c, err, "Failed to execute dynamic pipeline")
    }

    // Cache the new data and query if shouldCache is true
    if shouldCache {
        dataToCache, _ := json.Marshal(items)
        var expiration time.Duration
        if pipelineStage.CacheTime > 0 {
            expiration = time.Duration(pipelineStage.CacheTime) * time.Minute
        } else {
            expiration = 0 //the key will never expire.
        }
        configs.RedisClient.Set(ctx, redisKey, dataToCache, expiration)
        configs.RedisClient.Set(ctx, redisKey+"-query", currentQuery, expiration)
    }

    // Return the results
	log.Printf("Pipeline results successfully fetched for schema: %s with pipeline name: %s", schemaName, pipelineName)
    return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{
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
	log.Printf("Fetching all items with pagination for schema: %s", schemaName)
	if schemaName == "" {
		return utils.SendResponse(c, fiber.StatusBadRequest, "schemaName is required", nil)
	}
    
	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		// Use the container model from the context if available.
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		var err error
		container, err = utils.GetContainerModel(schemaName)
		if err != nil {
			log.Printf("Failed to fetch container model for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Failed to fetch container model")
		}
	}
	// Parse pagination query parameters.
	pageStr := c.Query("page", "1")  // Default to page 1
	limitStr := c.Query("limit", "10") // Default to 10 items per page

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		return utils.SendErrorResponse(c, err, "Invalid page number")
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		return utils.SendErrorResponse(c, err, "Invalid limit")
	}

	// Calculate the number of documents to skip
	skip := (page - 1) * limit

	// Define find options for pagination
	findOptions := options.Find().SetSkip(int64(skip)).SetLimit(int64(limit))

	// Check for sorting parameters. Only apply sorting if both "sort" and "asc" are provided.
	sortField := c.Query("sort")
	ascStr := c.Query("asc")
	if sortField != "" && ascStr != "" {
		asc, err := strconv.ParseBool(ascStr)
		if err != nil {
			return utils.SendErrorResponse(c, err, "Invalid asc parameter; must be true or false")
		}
		order := 1
		if !asc {
			order = -1
		}
		findOptions.SetSort(bson.D{{Key: sortField, Value: order}})
	}

	currentCollection := configs.GetCollection(schemaName)
	cursor, err := currentCollection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Printf("Failed to fetch items with pagination for schema: %s, error: %v", schemaName, err)
		return utils.SendErrorResponse(c, err, "Failed to fetch items")
	}
	defer cursor.Close(ctx)

	items, err := utils.DecodeCursor(cursor, ctx)
	if err != nil {
		log.Printf("Failed to decode items for schema: %s, error: %v", schemaName, err)
		return utils.SendErrorResponse(c, err, "Failed to decode items")
	}

	// Remove hashed fields from the items.
	for _, field := range container.Fields {
		if field.IsHashed {
			for i := range items {
				delete(items[i], field.Name)
			}
		}
	}

	// ----- Population logic begins -----
	if len(container.PopulationArray) > 0 && contains(container.PopulatedRoutes, "GetAllDynamicModelItemsWithPagination") {
		items, err = populateItems(ctx, container, items)
		if err != nil {
			log.Printf("Failed to populate items for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Failed to populate items")
		}
	}
	// ----- Population logic ends -----

	// Get total count of documents in the collection
	totalItems, err := currentCollection.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Printf("Failed to fetch total items for schema: %s, error: %v", schemaName, err)
		return utils.SendErrorResponse(c, err, "Failed to fetch total items")
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
	return utils.SendResponse(c, http.StatusOK, "Successfully fetched items", result)
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
              return utils.SendErrorResponse(c, err, "Failed to fetch container model")
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
                    return utils.SendErrorResponse(c, err, "Failed to execute function")
                }
                // Cache result if query hasn't changed and cache is available
                if shouldCache  {
                    var expiration time.Duration
                    if container.Redis.CacheTime > 0 {
                        expiration = time.Duration(container.Redis.CacheTime) * time.Minute
                    } else {
                        expiration = 0 //the key will never expire.
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
            Status:  fiber.StatusBadRequest,
            Message: "Function not found",
            Data:    nil,
        })
    }

    // Write new code to file
    err = ioutil.WriteFile(fileName, []byte(dynamicFuncCode), 0644)
    if err != nil {
        return utils.SendErrorResponse(c, err, "Failed to write code to file")
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
        return utils.SendErrorResponse(c, err, "Failed to load new plugin")
    }

    // Lookup function in new plugin
    f, err := p.Lookup(functionName)
    if err != nil {
        return utils.SendErrorResponse(c, err, "Failed to lookup function in new plugin")
    }

    // Execute the function and cache the result if caching is enabled
    if executeFunc, ok := f.(func(*fiber.Ctx) (interface{}, error)); ok {
        result, err := executeFunc(c)
        if err != nil {
            return utils.SendErrorResponse(c, err, "Failed to execute function")
        }

        // Cache the result
        if shouldCache {
            dataToCache, _ := json.Marshal(result)
            var expiration time.Duration
            if container.Redis.CacheTime > 0 {
                expiration = time.Duration(container.Redis.CacheTime) * time.Minute
            } else {
                expiration = 0 //the key will never expire.
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
        return utils.SendErrorResponse(c, err, "Failed to execute function")
    }
}

func TestPipeline(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    schemaName := c.Query("schemaName")
    // Parse request body
    var requestBody models.TestPipelineRequestBody
    if err := c.BodyParser(&requestBody); err != nil {
        return utils.SendErrorResponse(c, err, "Invalid request body")
    }
    requestBody.PipelineStage.PipelineJSON = utils.ReplacePlaceholdersWithQueryParams(requestBody.PipelineStage.PipelineJSON, c)
    currentCollection := configs.GetCollection( schemaName)

    // Execute the dynamic pipeline
    items, err := utils.ExecuteDynamicPipeline(ctx, currentCollection, requestBody.PipelineStage)
    if err != nil {
        // Log the error; do not fail the server
        log.Printf("Error executing test pipeline: %v", err)
        return utils.SendErrorResponse(c, err, "Failed to execute test pipeline")
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
            return utils.SendErrorResponse(c, err, "Failed to fetch container model")
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
            Status:  fiber.StatusBadRequest,
            Message: "API not found",
            Data:    nil,
        })
    }

  // Validate dependencies if they exist
    var requestBody map[string]interface{}
    if err := c.BodyParser(&requestBody); err != nil {
        return utils.SendErrorResponse(c, err, "Invalid request body")
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
        return utils.SendErrorResponse(c, err, "Failed to execute API call")
    }

    var apiResult interface{}
    if err := json.Unmarshal(apiResultBytes, &apiResult); err != nil {
        return utils.SendErrorResponse(c, err, "Failed to unmarshal API response")
    }
    // Cache the result if enabled
    if shouldCache {
        var expiration time.Duration
        if dynamicApi.CacheTime > 0 {
            expiration = time.Duration(dynamicApi.CacheTime) * time.Minute
        } else {
            expiration = 0 //the key will never expire.
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
func populateItems(ctx context.Context, container *models.ContainerModel, items []map[string]interface{}) ([]map[string]interface{}, error) {
	for _, pop := range container.PopulationArray {
		// Locate the field definition by matching the local field name.
		var targetField *models.Field
		for _, f := range container.Fields {
			// Here we assume that the PopulationArray's FieldName matches the local field name.
			if f.Name == pop.FieldName {
				targetField = &f
				break
			}
		}
		if targetField == nil {
			// Skip if the field definition is not found.
			continue
		}

		// Only perform population if the field type is "objectId".
		if targetField.Type == "objectId" {
			// Iterate over each item to perform the population.
			// Look for the ObjectID stored under the local field name.
			for i, item := range items {
				if idVal, exists := item[targetField.Name]; exists && idVal != nil {
					var objectId primitive.ObjectID
					switch v := idVal.(type) {
					case primitive.ObjectID:
						objectId = v
					case string:
						var err error
						objectId, err = primitive.ObjectIDFromHex(v)
						if err != nil {
							// Skip population if conversion fails.
							continue
						}
					default:
						// Skip if the value is not in an expected format.
						continue
					}

					// Fetch the populated document.
					// Use targetField.ObjectSchemaName as the collection name.
					populatedDoc, err := getPopulatedDocument(ctx, targetField.ObjectSchemaName, objectId, pop.PopulatedVariables)
					if err != nil {
						log.Printf("Failed to populate field %s for objectId %s: %v", targetField.Name, objectId.Hex(), err)
						continue
					}
					// Replace the ObjectID with the populated document under the local field name.
					items[i][targetField.Name] = populatedDoc
				}
			}
		}
		// If the field type is not "objectId", no changes are made.
	}
	return items, nil
}

func getPopulatedDocument(ctx context.Context, collectionName string, objectID primitive.ObjectID, fields []string) (map[string]interface{}, error) {
	// Retrieve the collection (assumes you have a function configs.GetCollection)
	coll := configs.GetCollection(collectionName)

	// Build the projection document.
	projection := bson.M{}
	for _, field := range fields {
		projection[field] = 1
	}

	// Find the document using the filter and projection.
	var result bson.M
	err := coll.FindOne(ctx, bson.M{"_id": objectID}, options.FindOne().SetProjection(projection)).Decode(&result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
// Helper function to check if a slice contains a given string.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
