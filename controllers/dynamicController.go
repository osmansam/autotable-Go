package controllers

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/osmansam/autotableGo/ws"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// convertFormFieldTypes converts string values from multipart forms to their appropriate types
func convertFormFieldTypes(itemMap map[string]interface{}, container *models.ContainerModel) {
	for _, field := range container.Fields {
		value, exists := itemMap[field.Name]
		if !exists {
			continue
		}
		
		// Only convert if the value is a string
		strValue, isString := value.(string)
		if !isString {
			// Handle array fields with nested objects
			if field.Type == "array" && field.Children != nil {
				if arrayValue, isArray := value.([]interface{}); isArray {
					for _, item := range arrayValue {
						if objMap, isMap := item.(map[string]interface{}); isMap {
							convertNestedFields(objMap, field.Children)
						}
					}
				}
			}
			// Handle object fields with nested children
			if field.Type == "object" && field.Children != nil {
				if objValue, isMap := value.(map[string]interface{}); isMap {
					convertNestedFields(objValue, field.Children)
				}
			}
			continue
		}
		
		switch field.Type {
		case "bool", "boolean":
			// Convert "true"/"false" strings to boolean
			if strValue == "true" {
				itemMap[field.Name] = true
			} else if strValue == "false" {
				itemMap[field.Name] = false
			}
		case "int":
			// Convert string to int
			if intValue, err := strconv.Atoi(strValue); err == nil {
				itemMap[field.Name] = intValue
			}
		case "float", "decimal":
			// Convert string to float64
			if floatValue, err := strconv.ParseFloat(strValue, 64); err == nil {
				itemMap[field.Name] = floatValue
			}
		case "stringArray":
			// Convert comma-separated string to string array
			if strValue != "" {
				parts := strings.Split(strValue, ",")
				strArray := make([]interface{}, len(parts))
				for i, part := range parts {
					strArray[i] = strings.TrimSpace(part)
				}
				itemMap[field.Name] = strArray
			} else {
				itemMap[field.Name] = []interface{}{}
			}
		case "numberArray", "intArray":
			// Convert comma-separated string to number array
			if strValue != "" {
				parts := strings.Split(strValue, ",")
				numArray := make([]interface{}, 0, len(parts))
				for _, part := range parts {
					part = strings.TrimSpace(part)
					// Try to parse as int first
					if intValue, err := strconv.Atoi(part); err == nil {
						numArray = append(numArray, intValue)
					} else if floatValue, err := strconv.ParseFloat(part, 64); err == nil {
						// If not an int, try as float
						numArray = append(numArray, floatValue)
					}
				}
				itemMap[field.Name] = numArray
			} else {
				itemMap[field.Name] = []interface{}{}
			}
		}
	}
}

// convertNestedFields converts string values in nested objects (for array fields)
func convertNestedFields(objMap map[string]interface{}, fields []models.Field) {
	for _, field := range fields {
		value, exists := objMap[field.Name]
		if !exists {
			continue
		}
		
		strValue, isString := value.(string)
		if !isString {
			continue
		}
		
		switch field.Type {
		case "bool", "boolean":
			if strValue == "true" {
				objMap[field.Name] = true
			} else if strValue == "false" {
				objMap[field.Name] = false
			}
		case "int":
			if intValue, err := strconv.Atoi(strValue); err == nil {
				objMap[field.Name] = intValue
			}
		case "float", "decimal":
			if floatValue, err := strconv.ParseFloat(strValue, 64); err == nil {
				objMap[field.Name] = floatValue
			}
		case "stringArray":
			if strValue != "" {
				parts := strings.Split(strValue, ",")
				strArray := make([]interface{}, len(parts))
				for i, part := range parts {
					strArray[i] = strings.TrimSpace(part)
				}
				objMap[field.Name] = strArray
			} else {
				objMap[field.Name] = []interface{}{}
			}
		case "numberArray", "intArray":
			if strValue != "" {
				parts := strings.Split(strValue, ",")
				numArray := make([]interface{}, 0, len(parts))
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if intValue, err := strconv.Atoi(part); err == nil {
						numArray = append(numArray, intValue)
					} else if floatValue, err := strconv.ParseFloat(part, 64); err == nil {
						numArray = append(numArray, floatValue)
					}
				}
				objMap[field.Name] = numArray
			} else {
				objMap[field.Name] = []interface{}{}
			}
		}
	}
}

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

    // Check if request is multipart form
    contentType := c.Get("Content-Type")
    if hasImageField || strings.Contains(contentType, "multipart/form-data") {
        // Parse multipart form for image fields or when explicitly sent as multipart
        form, err := c.MultipartForm()
        if err != nil {
            log.Printf("Error parsing form for schema: %s, error: %v", schemaName, err)
            return utils.SendErrorResponse(c, err, "Error parsing form.")
        }
        itemMap = utils.ProcessFormFields(form.Value)

        // Convert form field types (string to bool, int, float) based on schema
        convertFormFieldTypes(itemMap, container)

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

	// Automatically set createdAt and updatedAt if they are defined in the container schema.
	now := time.Now().UTC().Format(time.RFC3339)
	for _, field := range container.Fields {
		if field.Name == "createdAt" {
			// Only set createdAt if not already provided.
			if _, exists := itemMap["createdAt"]; !exists {
				itemMap["createdAt"] = now
			}
		}
		if field.Name == "updatedAt" {
			// Always set updatedAt at creation.
			itemMap["updatedAt"] = now
		}
	}

    // Calculate equation fields
    for _, field := range container.Fields {
        if field.Equation != "" {
            val, err := utils.EvaluateEquation(field.Equation, itemMap)
            if err != nil {
                log.Printf("Error evaluating equation for field %s: %v", field.Name, err)
                return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
                    Status:  http.StatusBadRequest,
                    Message: fmt.Sprintf("Error evaluating equation for field %s", field.Name),
                    Data:    err.Error(),
                })
            }
            itemMap[field.Name] = val
        }
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

	// Generate auto-increment id if defined and not provided
	for _, field := range container.Fields {
		if field.Type == "autoIncrementId" {
			if _, exists := itemMap[field.Name]; !exists {
				seq, err := utils.GetNextSequence(ctx, schemaName)
				if err != nil {
					log.Printf("Failed to generate autoIncrement id for schema: %s, error: %v", schemaName, err)
					return utils.SendErrorResponse(c, err, "Failed to generate autoIncrement id")
				}
				itemMap[field.Name] = seq
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
    // Emit WebSocket invalidate event for this schema
    ws.EmitInvalidate(schemaName)
    
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

	// Automatically set createdAt and updatedAt for each item, if defined in the container schema.
	now := time.Now().UTC().Format(time.RFC3339)
	for i, item := range items {
		for _, field := range container.Fields {
			if field.Name == "createdAt" {
				if _, exists := item["createdAt"]; !exists {
					item["createdAt"] = now
				}
			}
			if field.Name == "updatedAt" {
				item["updatedAt"] = now
			}
		}
		items[i] = item
	}


    // Calculate equation fields for each item
    for i, item := range items {
        for _, field := range container.Fields {
            if field.Equation != "" {
                val, err := utils.EvaluateEquation(field.Equation, item)
                if err != nil {
                    log.Printf("Error evaluating equation for field %s in item %d: %v", field.Name, i, err)
                    return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
                        Status:  http.StatusBadRequest,
                        Message: fmt.Sprintf("Error evaluating equation for field %s in item %d", field.Name, i),
                        Data:    err.Error(),
                    })
                }
                item[field.Name] = val
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

	// Generate auto-increment id for each item if defined and not provided or empty.
	for i, item := range items {
		for _, field := range container.Fields {
			if field.Type == "autoIncrementId" {
				// Check if the field is missing or its value is empty.
				if val, exists := item[field.Name]; !exists || fmt.Sprintf("%v", val) == "" {
					seq, err := utils.GetNextSequence(ctx, schemaName)
					if err != nil {
						log.Printf("Failed to generate autoIncrement id for schema: %s, error: %v", schemaName, err)
						return utils.SendErrorResponse(c, err, "Failed to generate autoIncrement id")
					}
					item[field.Name] = seq
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
			// Emit WebSocket invalidate event for triggered schema
			ws.EmitInvalidate(triggeredSchema)
		}
	}

	// Emit WebSocket invalidate event for this schema
	ws.EmitInvalidate(schemaName)
	
	log.Printf("Multiple items successfully created for schema: %s", schemaName)
	return c.Status(http.StatusCreated).JSON(responses.GeneralResponse{
		Status:  http.StatusCreated,
		Message: "Multiple items successfully created.",
		Data:    result,
	})
}

// GetAllDynamicModelItems fetches all items for a given collection and performs population if needed.
func GetAllDynamicModelItems(c *fiber.Ctx) error {
    // 1. Context with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // 2. Load container model (requires schemaName)
    container, err := utils.FetchContainerModel(c)
    if err != nil {
        if err == utils.ErrNoSchemaName {
            return utils.SendResponse(c, fiber.StatusBadRequest, err.Error(), nil)
        }
        return utils.SendErrorResponse(c, err, "Failed to load container model")
    }
    schema := container.SchemaName

    // 3. Determine caching
    redisKey, shouldCache := utils.GenerateRedisKey("GetAllDynamicModelItems", schema, container)
    if shouldCache {
        if data, err := configs.RedisClient.Get(ctx, redisKey).Result(); err == nil {
            var items []map[string]interface{}
            if json.Unmarshal([]byte(data), &items) == nil {
                log.Printf("Fetched items from cache for schema: %s", schema)
                return c.JSON(items)
            }
        }
    }

    // 4. Query database (no pagination)
    pager := utils.Pager{Enabled: false}
    opts := options.Find() // no skip/limit
    items, err := utils.QueryAndDecode(ctx, schema, bson.M{}, opts, &pager)
    if err != nil {
        log.Printf("DB query failed for schema %q: %v", schema, err)
        return utils.SendErrorResponse(c, err, "Failed to fetch items")
    }

    // 5. Strip hashed fields
    utils.StripHashed(container.Fields, items)

    // 6. Populate if needed
    items, err = utils.PopulateIfNeeded(ctx, container, "GetAllDynamicModelItems", items)
    if err != nil {
        log.Printf("Population failed for schema %q: %v", schema, err)
        return utils.SendErrorResponse(c, err, "Failed to populate items")
    }

    // 7. Cache the result if enabled
    if shouldCache {
        if payload, err := json.Marshal(items); err == nil {
            ttl := time.Duration(container.Redis.CacheTime) * time.Minute
            configs.RedisClient.Set(ctx, redisKey, payload, ttl)
        }
    }

    // 8. Return final response
    log.Printf("Fetched items from DB for schema: %s", schema)
    return c.JSON(items)
}

// GetItemsForSelection retrieves items with only _id and the specified fieldName for a schema
func GetItemsForSelection(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // Get schema name and field name from query params
    schemaName := c.Query("schemaName")
    fieldName := c.Query("fieldName")

    if schemaName == "" || fieldName == "" {
        return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
            Status:  http.StatusBadRequest,
            Message: "schemaName and fieldName are required",
            Data:    nil,
        })
    }

    log.Printf("Getting items for selection: schema=%s, field=%s", schemaName, fieldName)

    // Query the collection with projection for only _id and fieldName
    collection := configs.GetCollection(schemaName)
    projection := bson.M{
        "_id":     1,
        fieldName: 1,
    }

    cursor, err := collection.Find(ctx, bson.M{}, options.Find().SetProjection(projection))
    if err != nil {
        log.Printf("Failed to query collection %s: %v", schemaName, err)
        return utils.SendErrorResponse(c, err, "Failed to fetch items")
    }
    defer cursor.Close(ctx)

    var items []map[string]interface{}
    if err = cursor.All(ctx, &items); err != nil {
        log.Printf("Failed to decode items from collection %s: %v", schemaName, err)
        return utils.SendErrorResponse(c, err, "Failed to decode items")
    }

    log.Printf("Successfully fetched %d items for selection", len(items))
    return c.JSON(items)
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
	// Emit WebSocket invalidate event for this schema
	ws.EmitInvalidate(schemaName)
	
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
			// Emit WebSocket invalidate event for triggered schema
			ws.EmitInvalidate(triggeredSchema)
		}
	}

	// Prepare the final response containing both successes and failures.
	responseData := fiber.Map{
		"successful": successfulDeletes,
		"failed":     failedDeletes,
	}
	// Emit WebSocket invalidate event for this schema if there were any successful deletions
	if len(successfulDeletes) > 0 {
		ws.EmitInvalidate(schemaName)
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

    // Check if request is multipart form
    contentType := c.Get("Content-Type")
    if hasImageField || strings.Contains(contentType, "multipart/form-data") {
        // Handle multipart form data
        form, err := c.MultipartForm()
        if err != nil {
			log.Printf("Error in multipart form for schema: %s, error: %v", schemaName, err)
            return utils.SendErrorResponse(c, err, "Error in multipart form")
        }
        updatedItemMap = utils.ProcessFormFields(form.Value)

        // Convert form field types (string to bool, int, float) based on schema
        convertFormFieldTypes(updatedItemMap, container)

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
    
    // Automatically update the updatedAt field if defined in the container schema.
    now := time.Now().UTC().Format(time.RFC3339)
    for _, field := range container.Fields {
        if field.Name == "updatedAt" {
            updatedItemMap["updatedAt"] = now
        }
    }
    
    // Validate only the fields being updated (partial validation)
    err = utils.ValidatePartialUpdate(updatedItemMap, *container)
    if err != nil {
		log.Printf("Validation failed for schema: %s, error: %v", schemaName, err)
        return utils.SendErrorResponse(c, err, "Validation failed")
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

    // Calculate equation fields based on the merged existingItem
    for _, field := range container.Fields {
        if field.Equation != "" {
            // We need to convert bson.M (existingItem) to map[string]interface{} for the helper if it isn't already compatible
            // bson.M is map[string]interface{}, so it should be fine.
            val, err := utils.EvaluateEquation(field.Equation, existingItem)
            if err != nil {
                log.Printf("Error evaluating equation for field %s: %v", field.Name, err)
                 return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
                    Status:  http.StatusBadRequest,
                    Message: fmt.Sprintf("Error evaluating equation for field %s", field.Name),
                    Data:    err.Error(),
                })
            }
            existingItem[field.Name] = val
        }
    }
        // Checking for Unique fields
    for _, field := range container.Fields {
        if field.Unique {
            fieldValue, found := updatedItemMap[field.Name]
            if !found {
                continue 
            }

            // Exclude the current document from the uniqueness check
            filter := bson.M{
                field.Name: fieldValue,
                "_id": bson.M{"$ne": updateId}, // Exclude current document
            }
            count, err := currentCollection.CountDocuments(ctx, filter)
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
		// Emit WebSocket invalidate event for triggered schema
		ws.EmitInvalidate(triggeredSchema)
		}
	}

	// Emit WebSocket invalidate event for this schema
	ws.EmitInvalidate(schemaName)

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

		// Automatically update updatedAt for the update data.
		now := time.Now().UTC().Format(time.RFC3339)
		for _, field := range container.Fields {
			if field.Name == "updatedAt" {
				item["updatedAt"] = now
			}
		}

		// Validate only the fields being updated (partial validation)
		if err = utils.ValidatePartialUpdate(item, *container); err != nil {
			utils.ReleaseLock(lockKey, lockID)
			failedUpdates = append(failedUpdates, map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": "Validation failed: " + err.Error(),
			})
			continue
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

        // Calculate equation fields based on the merged existingItem
        equationError := false
        for _, field := range container.Fields {
            if field.Equation != "" {
                val, err := utils.EvaluateEquation(field.Equation, existingItem)
                if err != nil {
                    utils.ReleaseLock(lockKey, lockID)
                    failedUpdates = append(failedUpdates, map[string]interface{}{
                        "id":    idStr,
                        "item":  item,
                        "error": fmt.Sprintf("Error evaluating equation for field %s: %v", field.Name, err),
                    })
                    equationError = true
                    break
                }
                existingItem[field.Name] = val
            }
        }
        if equationError {
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
			// Emit WebSocket invalidate event for triggered schema
			ws.EmitInvalidate(triggeredSchema)
		}
	}

	// Prepare the final response containing both successes and failures.
	responseData := fiber.Map{
		"successful": successfulUpdates,
		"failed":     failedUpdates,
	}

	// Emit WebSocket invalidate event for this schema if there were any successful updates
	if len(successfulUpdates) > 0 {
		ws.EmitInvalidate(schemaName)
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

    // 1) Load and validate schemaName
    container, err := utils.FetchContainerModel(c)
    if err != nil {
        if err == utils.ErrNoSchemaName {
            return utils.SendResponse(c, fiber.StatusBadRequest, err.Error(), nil)
        }
        return utils.SendErrorResponse(c, err, "Failed to load container model")
    }
    schema := container.SchemaName

    // 2) Build the filter based on :id param
    idParam := c.Params("id")
    var filter bson.M

    // Check if there's an autoIncrementId field in this container
    var autoIncField string
    for _, f := range container.Fields {
        if f.Type == "autoIncrementId" {
            autoIncField = f.Name
            break
        }
    }

    if autoIncField != "" {
        // Try parse as integer first
        if idInt, parseErr := strconv.Atoi(idParam); parseErr == nil {
            filter = bson.M{autoIncField: idInt}
        } else if objID, parseErr := primitive.ObjectIDFromHex(idParam); parseErr == nil {
            // Fallback: valid ObjectID
            filter = bson.M{"_id": objID}
        } else {
            // Neither integer nor ObjectID
            return utils.SendErrorResponse(c, errors.New("invalid id format"), "Invalid ID")
        }
    } else {
        // No autoIncrementId → must be a Mongo ObjectID
        objID, parseErr := primitive.ObjectIDFromHex(idParam)
        if parseErr != nil {
            return utils.SendErrorResponse(c, parseErr, "Invalid ID")
        }
        filter = bson.M{"_id": objID}
    }

    // 3) Try cache
    redisKey, shouldCache := utils.GenerateRedisKey("GetDynamicModelItem", schema, container, idParam)
    if shouldCache {
        if raw, err := configs.RedisClient.Get(ctx, redisKey).Result(); err == nil {
            var item map[string]interface{}
            if json.Unmarshal([]byte(raw), &item) == nil {
                // strip hashed & populate
                utils.StripHashed(container.Fields, []map[string]interface{}{item})
                pop, _ := utils.PopulateIfNeeded(ctx, container, "GetDynamicModelItem", []map[string]interface{}{item})
                if len(pop) > 0 {
                    item = pop[0]
                }
                source := "cache"
                return utils.SendResponse(c, http.StatusOK, "Item found", fiber.Map{
                    "item":   item,
                    "source": &source,
                })
            }
        }
    }

    // 4) Fetch from Mongo
    coll := configs.GetCollection(schema)
    var rawDoc bson.M
    if err := coll.FindOne(ctx, filter).Decode(&rawDoc); err != nil {
        return utils.SendErrorResponse(c, err, "Item not found")
    }

    // convert to map[string]interface{}
    item := make(map[string]interface{}, len(rawDoc))
    for k, v := range rawDoc {
        item[k] = v
    }

    // 5) Strip hashed fields
    utils.StripHashed(container.Fields, []map[string]interface{}{item})

    // 6) Populate if needed
    pop, err := utils.PopulateIfNeeded(ctx, container, "GetDynamicModelItem", []map[string]interface{}{item})
    if err != nil {
        return utils.SendErrorResponse(c, err, "Failed to populate item")
    }
    if len(pop) > 0 {
        item = pop[0]
    }
    // 7) Cache the result
    if shouldCache {
        if blob, err := json.Marshal(item); err == nil {
            ttl := time.Duration(container.Redis.CacheTime) * time.Minute
            configs.RedisClient.Set(ctx, redisKey, blob, ttl)
        }
    }

    // 8) Return
    return c.JSON(item)
}
// handleSearch for a given collection
func HandleSearchDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1) load container (your original local-or-fetch logic is fine)
	container, err := utils.FetchContainerModel(c)
	if err != nil {
		if err == utils.ErrNoSchemaName {
			return utils.SendResponse(c, fiber.StatusBadRequest, err.Error(), nil)
		}
		return utils.SendErrorResponse(c, err, "failed to load container")
	}

	searchKey := c.Query("search")
	schemaName := c.Query("schemaName")

	// 2) build OR clauses with references
	orClauses, err := utils.BuildSearchWithReferences(ctx, container, searchKey)
	if err != nil {
		return utils.SendErrorResponse(c, err, "failed to build search filter")
	}

	// 3) finalize filter
	var filter bson.M
	if searchKey == "" {
		// behavior choice A: return all docs when search is empty
		filter = bson.M{}
	} else if len(orClauses) == 0 {
		// No searchable fields - return empty results
		return c.JSON([]interface{}{})
	} else {
		filter = bson.M{"$or": orClauses}
	}

	// 4) sort + pagination
	sortDoc, err := utils.ParseSort(c)
	if err != nil {
		return utils.SendErrorResponse(c, err, "invalid sort params")
	}
	pager, err := utils.ParsePager(c)
	if err != nil {
		return utils.SendErrorResponse(c, err, "invalid pagination params")
	}

	// 5) query
	opts := utils.BuildFindOptions(sortDoc, pager)
	items, err := utils.QueryAndDecode(ctx, schemaName, filter, opts, &pager)
	if err != nil {
		return utils.SendErrorResponse(c, err, "query failed")
	}

	// 6) strip hashed, populate
	utils.StripHashed(container.Fields, items)
	items, err = utils.PopulateIfNeeded(ctx, container, "HandleSearchDynamicModelItem", items)
	if err != nil {
		return utils.SendErrorResponse(c, err, "population failed")
	}

	// 7) response
	if pager.Enabled {
		return c.JSON(fiber.Map{
			"items":       items,
			"totalItems":  pager.TotalItems,
			"totalPages":  pager.TotalPages,
			"currentPage": pager.Page,
		})
	}
	return c.JSON(items)
}

// HandleFilterDynamicModelItem filters items for a given collection using dynamic query parameters.
func HandleFilterDynamicModelItem(c *fiber.Ctx) error {
    // 1) Context + load containerModel (ensures schemaName is present)
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    container, err := utils.FetchContainerModel(c)
    if err != nil {
        if err == utils.ErrNoSchemaName {
            return utils.SendResponse(c, fiber.StatusBadRequest, err.Error(), nil)
        }
        return utils.SendErrorResponse(c, err, "Failed to load container model")
    }

    // 2) Build the filter from query parameters
    filter, err := utils.BuildFilterFromQuery(c, container)
    if err != nil {
        return utils.SendErrorResponse(c, err, "Invalid filter parameter")
    }

    // 3) Add search functionality if search parameter is provided
    searchKey := c.Query("search")
    if searchKey != "" {
        // Build search OR clauses with references
        orClauses, err := utils.BuildSearchWithReferences(ctx, container, searchKey)
        if err != nil {
            return utils.SendErrorResponse(c, err, "Failed to build search filter")
        }
        
        if len(orClauses) > 0 {
            // Combine existing filter with search using $and
            if len(filter) > 0 {
                filter = bson.M{
                    "$and": []bson.M{
                        filter,
                        {"$or": orClauses},
                    },
                }
            } else {
                // No existing filter, just use search
                filter = bson.M{"$or": orClauses}
            }
        }
    }

    // 4) Parse sort & pagination
    sortDoc, err := utils.ParseSort(c)
    if err != nil {
        return utils.SendErrorResponse(c, err, "Invalid sort parameters")
    }
    pager, err := utils.ParsePager(c)
    if err != nil {
        return utils.SendErrorResponse(c, err, "Invalid pagination parameters")
    }

    // 5) Execute the query
    opts := utils.BuildFindOptions(sortDoc, pager)
    items, err := utils.QueryAndDecode(ctx, container.SchemaName, filter, opts, &pager)
    if err != nil {
        log.Printf("Filter query failed for schema %q: %v", container.SchemaName, err)
        return utils.SendErrorResponse(c, err, "Failed to fetch filtered items")
    }

    // 6) Strip out hashed fields
    utils.StripHashed(container.Fields, items)

    // 7) Populate references if configured
    items, err = utils.PopulateIfNeeded(ctx, container, "HandleFilterDynamicModelItem", items)
    if err != nil {
        return utils.SendErrorResponse(c, err, "Failed to populate items")
    }

    // 8) Send response (with pagination metadata if applicable)
    if pager.Enabled {
        return c.JSON(fiber.Map{
            "items":       items,
            "totalItems":  pager.TotalItems,
            "totalPages":  pager.TotalPages,
            "currentPage": pager.Page,
        })
    }
    return c.JSON(items)
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
                return c.JSON(items)
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
    return c.JSON(items)
}
// GetAllDynamicModelItemsWithPagination gets items from a collection with pagination.
func GetAllDynamicModelItemsWithPagination(c *fiber.Ctx) error {
    // 1. Context with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // 2. Load container (ensures schemaName present & fetches model)
    container, err := utils.FetchContainerModel(c)
    if err != nil {
        if err == utils.ErrNoSchemaName {
            return utils.SendResponse(c, fiber.StatusBadRequest, err.Error(), nil)
        }
        return utils.SendErrorResponse(c, err, "Failed to load container model")
    }

    // 2.6 Try Redis cache for paginated results
    // Use query string for cache key to differentiate between different page/limit/search params
    queryString := string(c.Request().URI().QueryString())
    redisKey, shouldCache := utils.GenerateRedisKey("GetAllDynamicModelItemsWithPagination", container.SchemaName, container, queryString)
    if shouldCache {
        if cachedData, err := configs.RedisClient.Get(ctx, redisKey).Result(); err == nil {
            var response fiber.Map
            if json.Unmarshal([]byte(cachedData), &response) == nil {
                log.Printf("Fetched paginated items from cache for schema: %s", container.SchemaName)
                return c.JSON(response)
            }
        }
    }

    // 3. Build filter from query parameters (field-based filtering)
    filter, err := utils.BuildFilterFromQuery(c, container)
    if err != nil {
        return utils.SendErrorResponse(c, err, "Invalid filter parameter")
    }

    // 4. Parse sort & pagination (needed for empty search results)
    sortDoc, err := utils.ParseSort(c)
    if err != nil {
        return utils.SendErrorResponse(c, err, "Invalid sort parameters")
    }
    pager, err := utils.ParsePager(c)
    if err != nil {
        return utils.SendErrorResponse(c, err, "Invalid pagination parameters")
    }

    // 5. Add search functionality if search parameter is provided
    searchKey := c.Query("search")
    if searchKey != "" {
        // Build search OR clauses with references
        orClauses, err := utils.BuildSearchWithReferences(ctx, container, searchKey)
        if err != nil {
            return utils.SendErrorResponse(c, err, "Failed to build search filter")
        }
        
        if len(orClauses) > 0 {
            // Combine existing filter with search using $and
            if len(filter) > 0 {
                filter = bson.M{
                    "$and": []bson.M{
                        filter,
                        {"$or": orClauses},
                    },
                }
            } else {
                // No existing filter, just use search
                filter = bson.M{"$or": orClauses}
            }
        } else {
            // No searchable fields - return empty results
            if pager.Enabled {
                return c.JSON(fiber.Map{
                    "items":       []interface{}{},
                    "totalItems":  0,
                    "totalPages":  0,
                    "currentPage": pager.Page,
                })
            }
            return c.JSON([]interface{}{})
        }
    }

    // 6. Query and decode results
    opts := utils.BuildFindOptions(sortDoc, pager)
    items, err := utils.QueryAndDecode(ctx, c.Query("schemaName"), filter, opts, &pager)
    if err != nil {
        log.Printf("Query failed for schema %q: %v", c.Query("schemaName"), err)
        return utils.SendErrorResponse(c, err, "Failed to fetch items")
    }

    // 7. Strip hashed fields
    utils.StripHashed(container.Fields, items)

    // 8. Populate if configured
    items, err = utils.PopulateIfNeeded(ctx, container, "GetAllDynamicModelItemsWithPagination", items)
    if err != nil {
        log.Printf("Population failed for schema %q: %v", c.Query("schemaName"), err)
        return utils.SendErrorResponse(c, err, "Failed to populate items")
    }

    // 9. Build response
    var response fiber.Map
    if pager.Enabled {
        response = fiber.Map{
            "items":       items,
            "totalItems":  pager.TotalItems,
            "totalPages":  pager.TotalPages,
            "currentPage": pager.Page,
        }
    } else {
        response = fiber.Map{"items": items}
    }

    // 10. Cache the response if enabled
    if shouldCache {
        if payload, err := json.Marshal(response); err == nil {
            ttl := time.Duration(container.Redis.CacheTime) * time.Minute
            configs.RedisClient.Set(ctx, redisKey, payload, ttl)
            log.Printf("Cached paginated items for schema: %s", container.SchemaName)
        }
    }

    // 11. Return response
    if pager.Enabled {
        return c.JSON(response)
    }
    return c.JSON(items)
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


// Helper function to check if a slice contains a given string.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}


