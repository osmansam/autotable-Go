package controllers

import (
	"bytes"
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
	"github.com/osmansam/autotableGo/services"
	"github.com/osmansam/autotableGo/utils"
	"github.com/osmansam/autotableGo/ws"
	"github.com/xuri/excelize/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// getProjectContext extracts tenantID and projectID from fiber context
// Returns an error if either is missing
func getProjectContext(c *fiber.Ctx) (tenantID, projectID string, err error) {
	// Use the new utility that extracts from URL slugs (or falls back to query/locals)
	tenantID, projectID, err = utils.GetTenantAndProjectContext(c)
	if err != nil {
		return "", "", err
	}

	if tenantID == "" || projectID == "" {
		return "", "", fmt.Errorf("missing tenant or project context - ensure you are authenticated and have switched to a project")
	}

	return tenantID, projectID, nil
}

func rejectBatchLimitExceeded(c *fiber.Ctx, operation, schemaName, tenantID, projectID string, requested, max int) error {
	log.Printf("%s limit exceeded for schema=%s tenant=%s project=%s requested=%d max=%d", operation, schemaName, tenantID, projectID, requested, max)
	return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
		Status:  http.StatusBadRequest,
		Message: fmt.Sprintf("%s limit exceeded. Maximum allowed items is %d.", operation, max),
		Data: fiber.Map{
			"requested": requested,
			"max":       max,
		},
	})
}

func parseLimitedItems(data []byte, max int) ([]map[string]interface{}, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))

	token, err := decoder.Token()
	if err != nil {
		return nil, false, err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '[' {
		return nil, false, errors.New("expected JSON array")
	}

	items := make([]map[string]interface{}, 0, max)
	for decoder.More() {
		if len(items) >= max {
			return items, true, nil
		}

		var item map[string]interface{}
		if err := decoder.Decode(&item); err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}

	token, err = decoder.Token()
	if err != nil {
		return nil, false, err
	}
	if delim, ok := token.(json.Delim); !ok || delim != ']' {
		return nil, false, errors.New("expected end of JSON array")
	}

	return items, false, nil
}

func parseLimitedBatchItems(c *fiber.Ctx, data []byte, operation, schemaName, tenantID, projectID string, max int) ([]map[string]interface{}, error) {
	items, exceeded, err := parseLimitedItems(data, max)
	if err != nil {
		log.Printf("Failed to parse %s items for schema: %s, error: %v", operation, schemaName, err)
		return nil, utils.SendErrorResponse(c, err, "Failed to parse request body")
	}
	if exceeded {
		return nil, rejectBatchLimitExceeded(c, operation, schemaName, tenantID, projectID, max+1, max)
	}
	return items, nil
}

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

	tenantID, projectID, err := getProjectContext(c)
	userIDStr, _ := c.Locals("userID").(string)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	schemaName := c.Query("schemaName")
	log.Printf("Creating item for schema: %s (tenant: %s, project: %s)", schemaName, tenantID, projectID)

	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	}

	dynamicService := services.NewDynamicService()
	itemMap, err := dynamicService.CreateDynamicItem(ctx, services.CreateDynamicItemInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		Schema:    schemaName,
		UserID:    userIDStr,
		User:      utils.GetUserFromContext(c),
		Container: container,
		FiberCtx:  c,
	})
	if err != nil {
		if serviceErr, ok := err.(*services.ServiceError); ok {
			return c.Status(serviceErr.Status).JSON(responses.GeneralResponse{
				Status:  serviceErr.Status,
				Message: serviceErr.Message,
				Data:    serviceErr.Data,
			})
		}
		return utils.SendErrorResponse(c, err, "Failed to save item.")
	}

	log.Printf("Item successfully created for schema: %s", schemaName)
	return c.Status(http.StatusCreated).JSON(responses.GeneralResponse{
		Status: http.StatusCreated, Message: "Item successfully created.", Data: itemMap,
	})
}
func CreateMultipleDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, err := getProjectContext(c)
	userIDStr, _ := c.Locals("userID").(string)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	schemaName := c.Query("schemaName")
	log.Printf("Creating multiple items for schema: %s (tenant: %s, project: %s)", schemaName, tenantID, projectID)

	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	}

	dynamicService := services.NewDynamicService()
	result, err := dynamicService.CreateMultipleDynamicItems(ctx, services.CreateMultipleDynamicItemsInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		Schema:    schemaName,
		UserID:    userIDStr,
		User:      utils.GetUserFromContext(c),
		Container: container,
		FiberCtx:  c,
	})
	if err != nil {
		if serviceErr, ok := err.(*services.ServiceError); ok {
			return c.Status(serviceErr.Status).JSON(responses.GeneralResponse{
				Status:  serviceErr.Status,
				Message: serviceErr.Message,
				Data:    serviceErr.Data,
			})
		}
		return utils.SendErrorResponse(c, err, "Failed to insert multiple items.")
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
	// 1. Context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Extract tenant and project context
	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

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
	redisKey, shouldCache := utils.GenerateRedisKey("GetAllDynamicModelItems", tenantID, projectID, schema, container)
	if shouldCache {
		if data, err := configs.RedisClient.Get(ctx, redisKey).Result(); err == nil {
			var items []map[string]interface{}
			if json.Unmarshal([]byte(data), &items) == nil {
				log.Printf("Fetched items from cache for schema: %s", schema)
				if maxUnboundedRead := configs.GetMaxUnboundedReadLimit(); len(items) > maxUnboundedRead {
					log.Printf("Unbounded read limit exceeded from cache for schema=%s tenant=%s project=%s requested=%d max=%d", schema, tenantID, projectID, len(items), maxUnboundedRead)
					items = items[:maxUnboundedRead]
				}
				// Filter fields based on user role before returning cached data
				userRole, _ := c.Locals("userRole").(string)
				items = utils.FilterDocuments(items, container.Fields, userRole)
				return c.JSON(items)
			}
		}
	}

	// 4. Query database with a configured safety cap for unbounded reads.
	maxUnboundedRead := configs.GetMaxUnboundedReadLimit()
	pager := utils.Pager{Enabled: false}
	opts := options.Find().SetLimit(int64(maxUnboundedRead + 1))
	items, err := utils.QueryAndDecode(ctx, tenantID, projectID, schema, bson.M{}, opts, &pager)
	if err != nil {
		log.Printf("DB query failed for schema %q: %v", schema, err)
		return utils.SendErrorResponse(c, err, "Failed to fetch items")
	}
	if len(items) > maxUnboundedRead {
		log.Printf("Unbounded read limit exceeded for schema=%s tenant=%s project=%s max=%d", schema, tenantID, projectID, maxUnboundedRead)
		items = items[:maxUnboundedRead]
	}

	// 5. Strip hashed fields
	utils.StripHashed(container.Fields, items)

	// 6. Populate if needed
	items, err = utils.PopulateIfNeeded(ctx, tenantID, projectID, container, items)
	if err != nil {
		log.Printf("Population failed for schema %q: %v", schema, err)
		return utils.SendErrorResponse(c, err, "Failed to populate items")
	}

	// 6.5. Filter fields based on user role authorization
	userRole, _ := c.Locals("userRole").(string)
	items = utils.FilterDocuments(items, container.Fields, userRole)

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

	// Extract tenant and project context
	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

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

	log.Printf("Getting items for selection: schema=%s, field=%s (tenant: %s, project: %s)", schemaName, fieldName, tenantID, projectID)

	// Load container model to check if field is hashed
	container, err := utils.GetContainerModel(tenantID, projectID, schemaName)
	if err != nil {
		log.Printf("Failed to get container model for schema %s: %v", schemaName, err)
		return utils.SendErrorResponse(c, err, "Failed to load schema configuration")
	}

	// Check if the requested field is hashed or restricted
	for _, field := range container.Fields {
		if field.Name == fieldName {
			// Check Authorization
			userRole, _ := c.Locals("userRole").(string)
			if len(field.AuthorizeRole) > 0 {
				authorized := false
				for _, r := range field.AuthorizeRole {
					if r == userRole {
						authorized = true
						break
					}
				}
				if !authorized {
					return c.Status(http.StatusForbidden).JSON(responses.GeneralResponse{
						Status:  http.StatusForbidden,
						Message: "Access to this field is restricted",
						Data:    nil,
					})
				}
			}

			if field.IsHashed {
				log.Printf("Attempted to access hashed field %s in schema %s", fieldName, schemaName)
				return c.Status(http.StatusForbidden).JSON(responses.GeneralResponse{
					Status:  http.StatusForbidden,
					Message: "Cannot access hashed fields",
					Data:    nil,
				})
			}
		}
	}

	// Query the project-specific collection with projection for only _id and fieldName
	collection := utils.GetDynamicCollectionForProject(tenantID, projectID, schemaName)
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
	// Filter fields based on user role authorization
	userRole, _ := c.Locals("userRole").(string)
	items = utils.FilterDocuments(items, container.Fields, userRole)

	return c.JSON(items)
}

// TODO: performance will be improved by adding a field in the container as usedSchemas (which will be updated when the new schema added with objectId of the currentSchema) and instead of getting all containers we will only check the neccessary containers and if the usedSchemas are empty we will not waste time with getting all containers

// delete an item from the collection
func DeleteDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Extract tenant and project context
	tenantID, projectID, err := getProjectContext(c)

	// Extract userID for WebSocket events
	userIDStr, _ := c.Locals("userID").(string)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	// Fetching the schema name from the query params
	schemaName := c.Query("schemaName")
	log.Printf("Deleting item for schema: %s (tenant: %s, project: %s)", schemaName, tenantID, projectID)

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
					collection := utils.GetDynamicCollectionForProject(tenantID, projectID, container.SchemaName)
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
					collection := utils.GetDynamicCollectionForProject(tenantID, projectID, container.SchemaName)
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
		container, err = utils.GetContainerModel(tenantID, projectID, schemaName)
		if err != nil {
			log.Printf("Failed to fetch container model for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Failed to fetch container model")
		}
	}

	// Using the project-specific collection for this schema
	var currentCollection *mongo.Collection = utils.GetDynamicCollectionForProject(tenantID, projectID, schemaName)

	// Fetch item before deletion for audit
	var deletedDoc bson.M
	if err := currentCollection.FindOne(ctx, bson.M{"_id": deleteId}).Decode(&deletedDoc); err != nil {
		log.Printf("Failed to fetch item before delete for schema: %s, error: %v", schemaName, err)
	}

	// Attempting to delete the item with the given ID from the specified collection
	_, err = currentCollection.DeleteOne(ctx, bson.M{"_id": deleteId})
	if err != nil {
		log.Printf("Failed to delete the item from the specified collection for schema: %s, error: %v", schemaName, err)
		return utils.SendErrorResponse(c, err, "Failed to delete the item from the specified collection.")
	}

	// Log audit
	if deletedDoc != nil {
		if err := utils.LogDeleteAction(ctx, tenantID, projectID, container, utils.GetUserFromContext(c), deletedDoc); err != nil {
			log.Printf("Failed to log delete action: %v", err)
		}
	}
	// Now attempting to delete the related cache
	if container.Redis.IsRedisCached {
		err = utils.DeleteCacheForSchema(ctx, tenantID, projectID, schemaName, container)
		if err != nil {
			log.Printf("Failed to delete cache for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Failed to delete the cache for the schema.")
		}

		// Additionally, delete cache for each schema mentioned in TriggeredRedisCaches
		for _, triggeredSchema := range container.Redis.TriggeredRedisCaches {
			err = utils.DeleteCacheForSchema(ctx, tenantID, projectID, triggeredSchema, container)
			if err != nil {
				// Log the error and continue with the next iteration
				log.Printf("Error deleting cache for schema %s: %v", triggeredSchema, err)
				continue
			}
		}
	}
	// Emit WebSocket invalidate event for this schema
	ws.EmitInvalidate(schemaName, userIDStr, tenantID, projectID)

	// Convert deletedDoc to map[string]interface{} for response
	responseItem := make(map[string]interface{})
	if deletedDoc != nil {
		for k, v := range deletedDoc {
			responseItem[k] = v
		}
		// Strip hashed fields from response
		utils.StripHashed(container.Fields, []map[string]interface{}{responseItem})
	}

	// Successfully deleted the item
	log.Printf("Item successfully deleted for schema: %s", schemaName)
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Item successfully deleted from the specified collection.",
		Data:    responseItem,
	})
}

func DeleteMultipleDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Extract tenant and project context
	tenantID, projectID, err := getProjectContext(c)

	// Extract userID for WebSocket events
	userIDStr, _ := c.Locals("userID").(string)

	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	schemaName := c.Query("schemaName")
	log.Printf("Deleting multiple items for schema: %s (tenant: %s, project: %s)", schemaName, tenantID, projectID)

	// Parse the request body as a JSON array.
	var items []map[string]interface{}
	maxBulkDelete := configs.GetMaxBulkDeleteLimit()
	items, err = parseLimitedBatchItems(c, c.Body(), "bulk delete", schemaName, tenantID, projectID, maxBulkDelete)
	if err != nil {
		return err
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
		container, err = utils.GetContainerModel(tenantID, projectID, schemaName)
		if err != nil {
			log.Printf("Failed to fetch container model for schema: %s, error: %v", schemaName, err)
			return utils.SendErrorResponse(c, err, "Failed to fetch container model")
		}
	}

	// Get the project-specific collection for the current schema.
	currentCollection := utils.GetDynamicCollectionForProject(tenantID, projectID, schemaName)

	// Slices to keep track of successful and failed deletions.
	var successfulDeletes []interface{}
	var failedDeletes []map[string]interface{}
	var bulkDeletedDocs []interface{}

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
					coll := utils.GetDynamicCollectionForProject(tenantID, projectID, otherContainer.SchemaName)
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
					coll := utils.GetDynamicCollectionForProject(tenantID, projectID, otherContainer.SchemaName)
					if _, err := coll.DeleteMany(ctx, bson.M{field.Name: deleteId}); err != nil {
						// Log error but continue with deletion.
						log.Printf("Failed to force delete referenced items for schema: %s, error: %v", schemaName, err)
					}
				}
			}
		}

		// Attempt deletion from the current collection.
		// Fetch audit doc
		var deletedDoc bson.M
		if err := currentCollection.FindOne(ctx, bson.M{"_id": deleteId}).Decode(&deletedDoc); err != nil {
			log.Printf("Failed to fetch item before delete (multiple) for schema: %s, error: %v", schemaName, err)
		}

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
		if deletedDoc != nil {
			bulkDeletedDocs = append(bulkDeletedDocs, deletedDoc)
		}

	}

	// Clear the cache for this schema if caching is enabled.
	if container.Redis.IsRedisCached {
		if err = utils.DeleteCacheForSchema(ctx, tenantID, projectID, schemaName, container); err != nil {
			log.Printf("Failed to delete cache for schema: %s, error: %v", schemaName, err)
		}
		for _, triggeredSchema := range container.Redis.TriggeredRedisCaches {
			if err = utils.DeleteCacheForSchema(ctx, tenantID, projectID, triggeredSchema, container); err != nil {
				log.Printf("Error deleting cache for schema %s: %v", triggeredSchema, err)
			}
			// Emit WebSocket invalidate event for triggered schema
			ws.EmitInvalidate(triggeredSchema, userIDStr, tenantID, projectID)
		}
	}

	// Prepare the final response containing both successes and failures.
	responseData := fiber.Map{
		"successful": successfulDeletes,
		"failed":     failedDeletes,
	}
	// Emit WebSocket invalidate event for this schema if there were any successful deletions
	if len(successfulDeletes) > 0 {
		ws.EmitInvalidate(schemaName, userIDStr, tenantID, projectID)
	}

	// Log Bulk Audit
	if len(bulkDeletedDocs) > 0 {
		if err := utils.LogBulkDeleteAction(ctx, tenantID, projectID, container, utils.GetUserFromContext(c), bulkDeletedDocs); err != nil {
			log.Printf("Failed to log bulk delete: %v", err)
		}
	}

	log.Printf("Multiple deletion process completed for schema: %s", schemaName)
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Multiple deletion process completed",
		Data:    responseData,
	})
}

// update an item in the collection
func UpdateDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, err := getProjectContext(c)
	userIDStr, _ := c.Locals("userID").(string)
	log.Printf("DEBUG: userID from Locals: '%s'", userIDStr)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	schemaName := c.Query("schemaName")
	log.Printf("Updating item for schema: %s (tenant: %s, project: %s)", schemaName, tenantID, projectID)

	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	}

	dynamicService := services.NewDynamicService()
	responseItem, err := dynamicService.UpdateDynamicItem(ctx, services.UpdateDynamicItemInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		Schema:    schemaName,
		ID:        c.Params("id"),
		UserID:    userIDStr,
		User:      utils.GetUserFromContext(c),
		Container: container,
		FiberCtx:  c,
	})
	if err != nil {
		if serviceErr, ok := err.(*services.ServiceError); ok {
			return c.Status(serviceErr.Status).JSON(responses.GeneralResponse{
				Status:  serviceErr.Status,
				Message: serviceErr.Message,
				Data:    serviceErr.Data,
			})
		}
		return utils.SendErrorResponse(c, err, "Failed to update item")
	}

	log.Printf("Item successfully updated for schema: %s", schemaName)
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{Status: http.StatusOK, Message: "Item successfully updated", Data: responseItem})
}
func UpdateMultipleDynamicModelItem(c *fiber.Ctx) error {
	// Set a timeout for the operation.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Extract tenant and project context
	tenantID, projectID, err := getProjectContext(c)

	// Extract userID for WebSocket events
	userIDStr, _ := c.Locals("userID").(string)

	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	schemaName := c.Query("schemaName")
	log.Printf("Updating multiple items for schema: %s (tenant: %s, project: %s)", schemaName, tenantID, projectID)

	// Fetch container model from context or via a helper function.
	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		container, err = utils.GetContainerModel(tenantID, projectID, schemaName)
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
		maxBulkUpdate := configs.GetMaxBulkUpdateLimit()
		items, err = parseLimitedBatchItems(c, []byte(itemsJSON[0]), "bulk update", schemaName, tenantID, projectID, maxBulkUpdate)
		if err != nil {
			return err
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
					log.Printf("%s", msg)
					return utils.SendErrorResponse(c, fmt.Errorf("%s", msg), msg)
				}
				// Loop over the files so that each item gets its corresponding image.
				for _, file := range files {
					tempFilePath := "./temp/" + file.Filename
					if err := c.SaveFile(file, tempFilePath); err != nil {
						return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Error saving temp file.", Data: err.Error()})
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
		maxBulkUpdate := configs.GetMaxBulkUpdateLimit()
		items, err = parseLimitedBatchItems(c, c.Body(), "bulk update", schemaName, tenantID, projectID, maxBulkUpdate)
		if err != nil {
			return err
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
	var bulkBeforeDocs []interface{}
	var bulkAfterDocs []interface{}

	// Get the project-specific collection for this schema.
	currentCollection := utils.GetDynamicCollectionForProject(tenantID, projectID, schemaName)

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

		// Clone for audit (Before state)
		beforeDoc := make(map[string]interface{})
		for k, v := range existingItem {
			beforeDoc[k] = v
		}

		// Merge update fields into the existing document.
		for key, value := range item {
			existingItem[key] = value
		}

		// Calculate equation fields based on the merged existingItem
		equationError := false
		for _, field := range container.Fields {
			if field.Equation != "" {
				eqCtx := &utils.EquationContext{
					TenantID:  tenantID,
					ProjectID: projectID,
					Data:      existingItem,
				}
				val, err := utils.EvaluateEquationWithContext(field.Equation, eqCtx)
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

		// Convert fields that should be ObjectId or ObjectIdArray to proper types
		for _, field := range container.Fields {
			if field.Type == "objectId" {
				if strVal, ok := existingItem[field.Name].(string); ok {
					if objId, err := primitive.ObjectIDFromHex(strVal); err == nil {
						existingItem[field.Name] = objId
					}
				}
			} else if field.Type == "objectIdArray" {
				// Handle objectIdArray conversion
				if val, exists := existingItem[field.Name]; exists && val != nil {
					// Try []interface{} first (common from JSON parsing)
					if arrInterface, ok := val.([]interface{}); ok {
						objIdArray := make([]primitive.ObjectID, 0, len(arrInterface))
						for _, item := range arrInterface {
							if strVal, ok := item.(string); ok {
								if objId, err := primitive.ObjectIDFromHex(strVal); err == nil {
									objIdArray = append(objIdArray, objId)
								}
							} else if objId, ok := item.(primitive.ObjectID); ok {
								objIdArray = append(objIdArray, objId)
							}
						}
						existingItem[field.Name] = objIdArray
					} else if arrString, ok := val.([]string); ok {
						// Handle []string
						objIdArray := make([]primitive.ObjectID, 0, len(arrString))
						for _, strVal := range arrString {
							if objId, err := primitive.ObjectIDFromHex(strVal); err == nil {
								objIdArray = append(objIdArray, objId)
							}
						}
						existingItem[field.Name] = objIdArray
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
		bulkBeforeDocs = append(bulkBeforeDocs, beforeDoc)
		bulkAfterDocs = append(bulkAfterDocs, existingItem)
	}

	// Log Bulk Audit
	if len(bulkAfterDocs) > 0 {
		if err := utils.LogBulkUpdateAction(ctx, tenantID, projectID, container, utils.GetUserFromContext(c), bulkBeforeDocs, bulkAfterDocs); err != nil {
			log.Printf("Failed to log bulk update: %v", err)
		}
	}

	// After processing all items, clear the cache for the schema.
	if container.Redis.IsRedisCached {
		err = utils.DeleteCacheForSchema(ctx, tenantID, projectID, schemaName, container)
		if err != nil {
			log.Printf("Failed to delete cache for schema: %s, error: %v", schemaName, err)
		}
		for _, triggeredSchema := range container.Redis.TriggeredRedisCaches {
			err = utils.DeleteCacheForSchema(ctx, tenantID, projectID, triggeredSchema, container)
			if err != nil {
				log.Printf("Error deleting cache for schema %s: %v", triggeredSchema, err)
			}
			// Emit WebSocket invalidate event for triggered schema
			ws.EmitInvalidate(triggeredSchema, userIDStr, tenantID, projectID)
		}
	}

	// Prepare the final response containing both successes and failures.
	responseData := fiber.Map{
		"successful": successfulUpdates,
		"failed":     failedUpdates,
	}

	// Emit WebSocket invalidate event for this schema if there were any successful updates
	if len(successfulUpdates) > 0 {
		ws.EmitInvalidate(schemaName, userIDStr, tenantID, projectID)
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

	// Extract tenant and project context
	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

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
	redisKey, shouldCache := utils.GenerateRedisKey("GetDynamicModelItem", tenantID, projectID, schema, container, idParam)
	if shouldCache {
		if raw, err := configs.RedisClient.Get(ctx, redisKey).Result(); err == nil {
			var item map[string]interface{}
			if json.Unmarshal([]byte(raw), &item) == nil {
				// strip hashed & populate
				utils.StripHashed(container.Fields, []map[string]interface{}{item})
				pop, _ := utils.PopulateIfNeeded(ctx, tenantID, projectID, container, []map[string]interface{}{item})
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

	// 4) Fetch from Mongo using project-specific collection
	coll := utils.GetDynamicCollectionForProject(tenantID, projectID, schema)
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
	pop, err := utils.PopulateIfNeeded(ctx, tenantID, projectID, container, []map[string]interface{}{item})
	if err != nil {
		return utils.SendErrorResponse(c, err, "Failed to populate item")
	}
	if len(pop) > 0 {
		item = pop[0]
	}

	// 6.5) Filter fields based on user role authorization
	userRole, _ := c.Locals("userRole").(string)
	filteredItems := utils.FilterDocuments([]map[string]interface{}{item}, container.Fields, userRole)
	if len(filteredItems) > 0 {
		item = filteredItems[0]
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

	// Extract tenant and project context
	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

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

	// Apply Row Access Logic
	userRole, _ := c.Locals("userRole").(string)
	// We need the full user object. Locals("user") might be set by middleware?
	// Assuming simple-auth middleware sets "user" or "userID".
	// If only userID is available, we might need to fetch user, or just pass map with ID if that's all needed.
	// For now, let's construct a minimal user map from locals if available, or fetch full user if needed.
	// Assuming "user" is NOT explicitly in locals as a map, but we have userID and userRole.
	// If row access relies on other user fields (e.g. email, companyId), we need to fetch the user.
	// Let's defer strict user fetching for performance unless necessary or use what's available.
	// Ideally Middleware should set "user" context.
	// Let's create a helper or assume middleware sets it.
	// HACK: Constructing a pseudo-user map from likely available claims.
	userID, _ := c.Locals("userID").(string)
	userMap := map[string]interface{}{"id": userID, "_id": userID, "role": userRole}
	// If strict fetching needed: userMap, _ = utils.GetUser(userID)

	rowAccessFilter, err := utils.GetRowAccessFilter(container, userRole, userMap)
	if err != nil {
		log.Printf("Error building row access filter: %v", err)
		return utils.SendErrorResponse(c, err, "row access error")
	}
	if rowAccessFilter != nil {
		if len(filter) > 0 {
			filter = bson.M{"$and": []bson.M{filter, rowAccessFilter}}
		} else {
			filter = rowAccessFilter
		}
	}

	// 5) query
	opts := utils.BuildFindOptions(sortDoc, pager)

	// Apply Row Access Logic
	items, err := utils.QueryAndDecode(ctx, tenantID, projectID, schemaName, filter, opts, &pager)
	if err != nil {
		return utils.SendErrorResponse(c, err, "query failed")
	}

	// 6) strip hashed, populate
	utils.StripHashed(container.Fields, items)
	items, err = utils.PopulateIfNeeded(ctx, tenantID, projectID, container, items)
	if err != nil {
		return utils.SendErrorResponse(c, err, "population failed")
	}

	// 6.5) Filter fields based on user role authorization
	// 6.5) Filter fields based on user role authorization
	items = utils.FilterDocuments(items, container.Fields, userRole)

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

	// Extract tenant and project context
	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

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

	// Apply Row Access Logic
	userRole, _ := c.Locals("userRole").(string)
	userID, _ := c.Locals("userID").(string)
	userMap := map[string]interface{}{"id": userID, "_id": userID, "role": userRole}

	rowAccessFilter, err := utils.GetRowAccessFilter(container, userRole, userMap)
	if err != nil {
		log.Printf("Error building row access filter: %v", err)
		return utils.SendErrorResponse(c, err, "row access error")
	}
	if rowAccessFilter != nil {
		if len(filter) > 0 {
			filter = bson.M{"$and": []bson.M{filter, rowAccessFilter}}
		} else {
			filter = rowAccessFilter
		}
	}

	items, err := utils.QueryAndDecode(ctx, tenantID, projectID, container.SchemaName, filter, opts, &pager)
	if err != nil {
		log.Printf("Filter query failed for schema %q: %v", container.SchemaName, err)
		return utils.SendErrorResponse(c, err, "Failed to fetch filtered items")
	}

	// 6) Strip out hashed fields
	utils.StripHashed(container.Fields, items)

	// 7) Populate references if configured
	items, err = utils.PopulateIfNeeded(ctx, tenantID, projectID, container, items)
	if err != nil {
		return utils.SendErrorResponse(c, err, "Failed to populate items")
	}

	// 7.5) Filter fields based on user role authorization
	// 7.5) Filter fields based on user role authorization
	items = utils.FilterDocuments(items, container.Fields, userRole)

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

// get all item for given collection
func GetPipeline(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Extract tenant and project context
	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	// Fetching the schema name and pipeline name from the query params
	schemaName := c.Query("schemaName")
	pipelineName := c.Query("pipelineName")
	log.Printf("Fetching pipeline for schema: %s with pipeline name: %s (tenant: %s, project: %s)", schemaName, pipelineName, tenantID, projectID)

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
		container, err = utils.GetContainerModel(tenantID, projectID, schemaName)
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
	currentCollection := utils.GetDynamicCollectionForProject(tenantID, projectID, schemaName)

	redisKey, shouldCache := utils.GeneratePipelineRedisKey(tenantID, projectID, schemaName, pipelineName, container)

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
			var items []map[string]interface{}
			if err := json.Unmarshal([]byte(cachedData), &items); err == nil {
				// Filter fields based on user role authorization
				// userRole, _ := c.Locals("userRole").(string)
				// items = utils.FilterDocuments(items, container.Fields, userRole)
				return c.JSON(items)
			}
		}
	}

	// Execute the dynamic pipeline
	items, err := utils.ExecuteDynamicPipeline(ctx, currentCollection, pipelineStage)
	if err != nil {
		return utils.SendErrorResponse(c, err, "Failed to execute dynamic pipeline")
	}

	// Convert []bson.M to []map[string]interface{} for filtering
	var resultItems []map[string]interface{}
	for _, doc := range items {
		resultItems = append(resultItems, map[string]interface{}(doc))
	}

	// Filter fields based on user role authorization
	// userRole, _ := c.Locals("userRole").(string)
	// resultItems = utils.FilterDocuments(resultItems, container.Fields, userRole)

	// Cache the new data and query if shouldCache is true
	if shouldCache {
		dataToCache, _ := json.Marshal(resultItems)
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
	log.Printf("Pipeline results successfully fetched and filtered for schema: %s with pipeline name: %s", schemaName, pipelineName)
	return c.JSON(resultItems)
}

// GetAllDynamicModelItemsWithPagination gets items from a collection with pagination.
func GetAllDynamicModelItemsWithPagination(c *fiber.Ctx) error {
	// 1. Context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Extract tenant and project context
	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	// 2. Load container (ensures schemaName present & fetches model)
	container, err := utils.FetchContainerModel(c)
	if err != nil {
		if err == utils.ErrNoSchemaName {
			return utils.SendResponse(c, fiber.StatusBadRequest, err.Error(), nil)
		}
		return utils.SendErrorResponse(c, err, "Failed to load container model")
	}

	// 2.6 Try Redis cache for paginated results (Skip if Row Access is potentially active - refined check below)
	queryString := string(c.Request().URI().QueryString())
	// Note: If Row Access is active, we should NOT return shared cache unless key includes user.
	// Checking row access first to decide on cache key would be better but requires more change.
	// For now, let's proceed and if we find row access is needed, we will invalidate attempting to use shared cache
	// or we append userID to key if row access rules exist.

	// Check if container has row access rules
	hasRowAccess := container.RowAccess != nil && len(container.RowAccess.Conditions) > 0

	userRole, _ := c.Locals("userRole").(string)
	userID, _ := c.Locals("userID").(string)

	var redisKey string
	var shouldCache bool

	if hasRowAccess {
		// Append userID to query string or key to ensure private cache
		redisKey, shouldCache = utils.GenerateRedisKey("GetAllDynamicModelItemsWithPagination", tenantID, projectID, container.SchemaName, container, queryString+"&_uid="+userID)
	} else {
		redisKey, shouldCache = utils.GenerateRedisKey("GetAllDynamicModelItemsWithPagination", tenantID, projectID, container.SchemaName, container, queryString)
	}

	if shouldCache {
		if cachedData, err := configs.RedisClient.Get(ctx, redisKey).Result(); err == nil {
			var response fiber.Map
			if json.Unmarshal([]byte(cachedData), &response) == nil {
				log.Printf("Fetched paginated items from cache for schema: %s", container.SchemaName)

				// Filter fields based on user role before returning cached data
				if itemsInterface, ok := response["items"].([]interface{}); ok {
					var items []map[string]interface{}
					for _, item := range itemsInterface {
						if itemMap, ok := item.(map[string]interface{}); ok {
							items = append(items, itemMap)
						}
					}

					items = utils.FilterDocuments(items, container.Fields, userRole)
					response["items"] = items
				}

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

	// Apply Row Access Logic (Last step of filtering)
	userMap := map[string]interface{}{"id": userID, "_id": userID, "role": userRole}
	rowAccessFilter, err := utils.GetRowAccessFilter(container, userRole, userMap)
	if err != nil {
		log.Printf("Error building row access filter: %v", err)
		return utils.SendErrorResponse(c, err, "row access error")
	}
	if rowAccessFilter != nil {
		if len(filter) > 0 {
			filter = bson.M{"$and": []bson.M{filter, rowAccessFilter}}
		} else {
			filter = rowAccessFilter
		}
	}

	// 6. Query and decode results
	opts := utils.BuildFindOptions(sortDoc, pager)
	items, err := utils.QueryAndDecode(ctx, tenantID, projectID, c.Query("schemaName"), filter, opts, &pager)
	if err != nil {
		log.Printf("Query failed for schema %q: %v", c.Query("schemaName"), err)
		return utils.SendErrorResponse(c, err, "Failed to fetch items")
	}

	// 7. Strip hashed fields
	utils.StripHashed(container.Fields, items)

	// 8. Populate if configured
	items, err = utils.PopulateIfNeeded(ctx, tenantID, projectID, container, items)
	if err != nil {
		log.Printf("Population failed for schema %q: %v", c.Query("schemaName"), err)
		return utils.SendErrorResponse(c, err, "Failed to populate items")
	}

	// 8.5. Filter fields based on user role authorization
	// 8.5. Filter fields based on user role authorization
	items = utils.FilterDocuments(items, container.Fields, userRole)

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

	// Extract tenant and project context
	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	schemaName := c.Query("schemaName")
	functionName := c.Query("functionName")

	// Serialize the current query parameters
	currentQuery := c.OriginalURL()
	// Fetch the associated container model from context
	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		container, err = utils.GetContainerModel(tenantID, projectID, schemaName)
		if err != nil {
			return utils.SendErrorResponse(c, err, "Failed to fetch container model")
		}
	}

	pluginFileName := "temp_" + functionName + ".so"
	fileName := "temp_" + functionName + ".go"

	// generate new redis key
	redisKey, shouldCache := utils.GenerateDynamicFunctionRedisKey(tenantID, projectID, schemaName, functionName, container)

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
				if shouldCache {
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

	// Extract tenant and project context
	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	schemaName := c.Query("schemaName")

	// Fetch the container model for filtering
	container, err := utils.GetContainerModel(tenantID, projectID, schemaName)
	if err != nil {
		return utils.SendErrorResponse(c, err, "Failed to fetch container model")
	}

	// Parse request body
	var requestBody models.TestPipelineRequestBody
	if err := c.BodyParser(&requestBody); err != nil {
		return utils.SendErrorResponse(c, err, "Invalid request body")
	}
	requestBody.PipelineStage.PipelineJSON = utils.ReplacePlaceholdersWithQueryParams(requestBody.PipelineStage.PipelineJSON, c)
	currentCollection := utils.GetDynamicCollectionForProject(tenantID, projectID, schemaName)

	// Execute the dynamic pipeline
	items, err := utils.ExecuteDynamicPipeline(ctx, currentCollection, requestBody.PipelineStage)
	if err != nil {
		// Log the error; do not fail the server
		log.Printf("Error executing test pipeline: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to execute test pipeline")
	}

	// Convert []bson.M to []map[string]interface{} for filtering
	var resultItems []map[string]interface{}
	for _, doc := range items {
		resultItems = append(resultItems, map[string]interface{}(doc))
	}

	// Filter fields based on user role authorization
	userRole, _ := c.Locals("userRole").(string)
	resultItems = utils.FilterDocuments(resultItems, container.Fields, userRole)

	// Return the results
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":  fiber.StatusOK,
		"message": "Test pipeline executed and filtered successfully",
		"data":    resultItems,
	})
}

// TODO:redis generate key and delete key will added into this function and then the route will be added and tested again
func ExecuteDynamicAPI(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Extract tenant and project context
	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	schemaName := c.Query("schemaName")
	apiName := c.Query("apiName")

	// Fetch the associated container model from context
	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		container, err = utils.GetContainerModel(tenantID, projectID, schemaName)
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
	redisKey, shouldCache := utils.GenerateDynamicApiRedisKey(tenantID, projectID, schemaName, apiName, container)
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

// formatPopulatedValue extracts and formats display fields from a populated object or array
func formatPopulatedValue(val interface{}, displayFields []string) string {
	// Handle populated object (single objectId)
	if populatedObj, ok := val.(map[string]interface{}); ok {
		var parts []string
		for _, displayField := range displayFields {
			if fieldVal, exists := populatedObj[displayField]; exists && fieldVal != nil {
				parts = append(parts, fmt.Sprintf("%v", fieldVal))
			}
		}
		return strings.Join(parts, " ")
	}

	// Handle bson.M (alternative map type from MongoDB) - single object
	if populatedObj, ok := val.(bson.M); ok {
		var parts []string
		for _, displayField := range displayFields {
			if fieldVal, exists := populatedObj[displayField]; exists && fieldVal != nil {
				parts = append(parts, fmt.Sprintf("%v", fieldVal))
			}
		}
		return strings.Join(parts, " ")
	}

	// Handle []map[string]interface{} (what MongoDB actually returns for populated arrays)
	if populatedArray, ok := val.([]map[string]interface{}); ok {
		var arrayParts []string
		for _, populatedObj := range populatedArray {
			var parts []string
			for _, displayField := range displayFields {
				if fieldVal, exists := populatedObj[displayField]; exists && fieldVal != nil {
					parts = append(parts, fmt.Sprintf("%v", fieldVal))
				}
			}
			if len(parts) > 0 {
				arrayParts = append(arrayParts, strings.Join(parts, " "))
			}
		}
		return strings.Join(arrayParts, ", ")
	}

	// Handle populated array (objectIdArray) - []interface{}
	if populatedArray, ok := val.([]interface{}); ok {
		var arrayParts []string
		for _, item := range populatedArray {
			// Try map[string]interface{}
			if populatedObj, ok := item.(map[string]interface{}); ok {
				var parts []string
				for _, displayField := range displayFields {
					if fieldVal, exists := populatedObj[displayField]; exists && fieldVal != nil {
						parts = append(parts, fmt.Sprintf("%v", fieldVal))
					}
				}
				if len(parts) > 0 {
					arrayParts = append(arrayParts, strings.Join(parts, " "))
				}
			} else if populatedObj, ok := item.(bson.M); ok {
				// Try bson.M
				var parts []string
				for _, displayField := range displayFields {
					if fieldVal, exists := populatedObj[displayField]; exists && fieldVal != nil {
						parts = append(parts, fmt.Sprintf("%v", fieldVal))
					}
				}
				if len(parts) > 0 {
					arrayParts = append(arrayParts, strings.Join(parts, " "))
				}
			}
		}
		return strings.Join(arrayParts, ", ")
	}

	// Handle primitive.A (MongoDB array type)
	if populatedArray, ok := val.(primitive.A); ok {
		var arrayParts []string
		for _, item := range populatedArray {
			// Try map[string]interface{}
			if populatedObj, ok := item.(map[string]interface{}); ok {
				var parts []string
				for _, displayField := range displayFields {
					if fieldVal, exists := populatedObj[displayField]; exists && fieldVal != nil {
						parts = append(parts, fmt.Sprintf("%v", fieldVal))
					}
				}
				if len(parts) > 0 {
					arrayParts = append(arrayParts, strings.Join(parts, " "))
				}
			} else if populatedObj, ok := item.(bson.M); ok {
				// Try bson.M
				var parts []string
				for _, displayField := range displayFields {
					if fieldVal, exists := populatedObj[displayField]; exists && fieldVal != nil {
						parts = append(parts, fmt.Sprintf("%v", fieldVal))
					}
				}
				if len(parts) > 0 {
					arrayParts = append(arrayParts, strings.Join(parts, " "))
				}
			}
		}
		return strings.Join(arrayParts, ", ")
	}

	// Fallback: return the value as-is
	return fmt.Sprintf("%v", val)
}

// ExportDynamicModelItems exports items to an Excel file based on selected fields and filters.
func ExportDynamicModelItems(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Extract tenant and project context
	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	// 1. Parse Request Body
	type ExportRequest struct {
		SchemaName string                 `json:"schemaName"`
		Fields     []string               `json:"fields"`
		Filters    map[string]interface{} `json:"filters"`
		Search     string                 `json:"search"`
		Page       int                    `json:"page"`
		Limit      int                    `json:"limit"`
	}

	var req ExportRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.SendErrorResponse(c, err, "Failed to parse request body")
	}

	if req.SchemaName == "" {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "schemaName is required",
			Data:    nil,
		})
	}

	// 2. Load Container Model
	container, err := utils.GetContainerModel(tenantID, projectID, req.SchemaName)
	if err != nil {
		return utils.SendErrorResponse(c, err, "Failed to fetch container model")
	}

	// 3. Build Filter
	// We need to adapt BuildFilterFromQuery to work with the map from the body
	// Since BuildFilterFromQuery uses c.QueryParser, we'll manually build the filter here
	// reusing logic similar to BuildFilterFromQuery but for the map.
	filter := bson.M{}

	// Apply filters from request
	if len(req.Filters) > 0 {
		// This is a simplified version. For full compatibility with complex filters
		// (like ranges, etc.) we might need to replicate BuildFilterFromQuery logic fully.
		// For now, assuming direct value matching or simple operators if passed in the map.
		// If the frontend sends the same structure as query params, we can iterate and build.
		for key, value := range req.Filters {
			// Skip empty string values (ignore them, don't filter for empty strings)
			if strVal, ok := value.(string); ok && strVal == "" {
				continue
			}

			// Check if field exists in container
			isValidField := false
			for _, f := range container.Fields {
				if f.Name == key {
					isValidField = true
					break
				}
			}
			if isValidField {
				filter[key] = value
			}
		}
	}

	// 4. Apply Search if provided
	if req.Search != "" {
		orClauses, err := utils.BuildSearchWithReferences(ctx, container, req.Search)
		if err != nil {
			return utils.SendErrorResponse(c, err, "Failed to build search filter")
		}
		if len(orClauses) > 0 {
			if len(filter) > 0 {
				filter = bson.M{"$and": []bson.M{filter, {"$or": orClauses}}}
			} else {
				filter = bson.M{"$or": orClauses}
			}
		}
	}

	// 5. Query Data with optional pagination from project-specific collection
	currentCollection := utils.GetDynamicCollectionForProject(tenantID, projectID, req.SchemaName)

	// Build find options with pagination if page and limit are provided
	maxExportLimit := configs.GetMaxExportLimit()
	findOpts := options.Find()
	if req.Limit > maxExportLimit {
		log.Printf("Export limit exceeded for schema=%s tenant=%s project=%s requested=%d max=%d", req.SchemaName, tenantID, projectID, req.Limit, maxExportLimit)
		req.Limit = maxExportLimit
	}
	if req.Page > 0 || req.Limit > 0 {
		page := req.Page
		if page < 1 {
			page = 1
		}
		limit := req.Limit
		if limit < 1 {
			limit = maxExportLimit
		}
		skip := int64((page - 1) * limit)
		findOpts.SetSkip(skip)
		findOpts.SetLimit(int64(limit))
	} else {
		findOpts.SetLimit(int64(maxExportLimit + 1))
	}

	cursor, err := currentCollection.Find(ctx, filter, findOpts)
	if err != nil {
		return utils.SendErrorResponse(c, err, "Failed to fetch items")
	}
	defer cursor.Close(ctx)

	var items []map[string]interface{}
	if err = cursor.All(ctx, &items); err != nil {
		return utils.SendErrorResponse(c, err, "Failed to decode items")
	}
	if len(items) > maxExportLimit {
		log.Printf("Export limit exceeded for schema=%s tenant=%s project=%s max=%d", req.SchemaName, tenantID, projectID, maxExportLimit)
		items = items[:maxExportLimit]
	}

	// 6. Populate and Strip Hashed
	utils.StripHashed(container.Fields, items)
	items, err = utils.PopulateIfNeeded(ctx, tenantID, projectID, container, items)
	if err != nil {
		return utils.SendErrorResponse(c, err, "Failed to populate items")
	}

	// 7. Generate Excel
	f := excelize.NewFile()
	sheetName := "Sheet1"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return utils.SendErrorResponse(c, err, "Failed to create sheet")
	}
	f.SetActiveSheet(index)

	// Determine columns to export
	var exportFields []models.Field
	if len(req.Fields) > 0 {
		// Filter container fields based on requested fields
		for _, reqField := range req.Fields {
			for _, field := range container.Fields {
				if field.Name == reqField {
					exportFields = append(exportFields, field)
					break
				}
			}
		}
	} else {
		// Default to all fields if none specified
		exportFields = container.Fields
	}

	// Write Headers
	for i, field := range exportFields {
		colName := field.Name
		// Column Naming Logic
		if field.Frontend != nil && field.Frontend.DisplayName != "" {
			colName = field.Frontend.DisplayName
		} else {
			// Capitalize first letter
			if len(colName) > 0 {
				colName = strings.ToUpper(colName[:1]) + colName[1:]
			}
			// CamelCase to Camel Case
			// A simple regex or loop to insert space before uppercase
			// Using a loop for simplicity and no extra regex dependency overhead if not needed
			var newColName strings.Builder
			for j, r := range colName {
				if j > 0 && r >= 'A' && r <= 'Z' {
					newColName.WriteRune(' ')
				}
				newColName.WriteRune(r)
			}
			colName = newColName.String()
		}

		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, colName)
	}

	// Write Data
	for i, item := range items {
		row := i + 2
		for j, field := range exportFields {
			cell, _ := excelize.CoordinatesToCellName(j+1, row)
			val, exists := item[field.Name]
			if exists {
				// Check if this field has population settings and displayFields
				if field.PopulationSettings != nil && len(field.PopulationSettings.DisplayFields) > 0 {
					// This is a populated objectId field
					displayValue := formatPopulatedValue(val, field.PopulationSettings.DisplayFields)
					f.SetCellValue(sheetName, cell, displayValue)
				} else {
					// Handle different types for string representation
					// Check if field is stringArray or intArray
					if field.Type == "stringArray" {
						// Convert to []string and join with commas
						if strArray, ok := val.([]interface{}); ok {
							var strValues []string
							for _, v := range strArray {
								strValues = append(strValues, fmt.Sprintf("%v", v))
							}
							f.SetCellValue(sheetName, cell, strings.Join(strValues, ","))
						} else if strArray, ok := val.([]string); ok {
							f.SetCellValue(sheetName, cell, strings.Join(strArray, ","))
						} else if strArray, ok := val.(primitive.A); ok {
							var strValues []string
							for _, v := range strArray {
								strValues = append(strValues, fmt.Sprintf("%v", v))
							}
							f.SetCellValue(sheetName, cell, strings.Join(strValues, ","))
						} else {
							f.SetCellValue(sheetName, cell, val)
						}
					} else if field.Type == "intArray" {
						// Convert to []int and join with commas
						if intArray, ok := val.([]interface{}); ok {
							var intValues []string
							for _, v := range intArray {
								intValues = append(intValues, fmt.Sprintf("%v", v))
							}
							f.SetCellValue(sheetName, cell, strings.Join(intValues, ","))
						} else if intArray, ok := val.([]int); ok {
							var intValues []string
							for _, v := range intArray {
								intValues = append(intValues, fmt.Sprintf("%d", v))
							}
							f.SetCellValue(sheetName, cell, strings.Join(intValues, ","))
						} else if intArray, ok := val.(primitive.A); ok {
							var intValues []string
							for _, v := range intArray {
								intValues = append(intValues, fmt.Sprintf("%v", v))
							}
							f.SetCellValue(sheetName, cell, strings.Join(intValues, ","))
						} else {
							f.SetCellValue(sheetName, cell, val)
						}
					} else {
						// For all other types, use default formatting
						f.SetCellValue(sheetName, cell, val)
					}
				}
			}
		}
	}

	// 8. Return File
	// Save to buffer
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return utils.SendErrorResponse(c, err, "Failed to write excel to buffer")
	}

	// Set headers for download
	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_export.xlsx", req.SchemaName))

	return c.Send(buffer.Bytes())
}
