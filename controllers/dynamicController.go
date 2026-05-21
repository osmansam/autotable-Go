package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
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
	"github.com/xuri/excelize/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	}

	userRole, _ := c.Locals("userRole").(string)
	dynamicService := services.NewDynamicService()
	items, err := dynamicService.GetAllDynamicItems(ctx, services.GetAllDynamicItemsInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		Schema:    c.Query("schemaName"),
		UserRole:  userRole,
		Container: container,
	})
	if err != nil {
		if serviceErr, ok := err.(*services.ServiceError); ok {
			return c.Status(serviceErr.Status).JSON(responses.GeneralResponse{
				Status:  serviceErr.Status,
				Message: serviceErr.Message,
				Data:    serviceErr.Data,
			})
		}
		return utils.SendErrorResponse(c, err, "Failed to fetch items")
	}

	return c.JSON(items)
}

// GetItemsForSelection retrieves items with only _id and the specified fieldName for a schema
func GetItemsForSelection(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	schemaName := c.Query("schemaName")
	fieldName := c.Query("fieldName")
	log.Printf("Getting items for selection: schema=%s, field=%s (tenant: %s, project: %s)", schemaName, fieldName, tenantID, projectID)

	userRole, _ := c.Locals("userRole").(string)
	dynamicService := services.NewDynamicService()
	items, err := dynamicService.GetItemsForSelection(ctx, services.GetItemsForSelectionInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		Schema:    schemaName,
		FieldName: fieldName,
		UserRole:  userRole,
	})
	if err != nil {
		if serviceErr, ok := err.(*services.ServiceError); ok {
			return c.Status(serviceErr.Status).JSON(responses.GeneralResponse{
				Status:  serviceErr.Status,
				Message: serviceErr.Message,
				Data:    serviceErr.Data,
			})
		}
		return utils.SendErrorResponse(c, err, "Failed to fetch items")
	}

	return c.JSON(items)
}

// TODO: performance will be improved by adding a field in the container as usedSchemas (which will be updated when the new schema added with objectId of the currentSchema) and instead of getting all containers we will only check the neccessary containers and if the usedSchemas are empty we will not waste time with getting all containers

// delete an item from the collection
func DeleteDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, err := getProjectContext(c)
	userIDStr, _ := c.Locals("userID").(string)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	schemaName := c.Query("schemaName")
	log.Printf("Deleting item for schema: %s (tenant: %s, project: %s)", schemaName, tenantID, projectID)

	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	}

	dynamicService := services.NewDynamicService()
	responseItem, err := dynamicService.DeleteDynamicItem(ctx, services.DeleteDynamicItemInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		Schema:    schemaName,
		ID:        c.Params("id"),
		UserID:    userIDStr,
		User:      utils.GetUserFromContext(c),
		Container: container,
	})
	if err != nil {
		if serviceErr, ok := err.(*services.ServiceError); ok {
			return c.Status(serviceErr.Status).JSON(responses.GeneralResponse{
				Status:  serviceErr.Status,
				Message: serviceErr.Message,
				Data:    serviceErr.Data,
			})
		}
		return utils.SendErrorResponse(c, err, "Failed to delete the item from the specified collection.")
	}

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

	tenantID, projectID, err := getProjectContext(c)
	userIDStr, _ := c.Locals("userID").(string)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	schemaName := c.Query("schemaName")
	log.Printf("Deleting multiple items for schema: %s (tenant: %s, project: %s)", schemaName, tenantID, projectID)

	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	}

	dynamicService := services.NewDynamicService()
	responseData, err := dynamicService.DeleteMultipleDynamicItems(ctx, services.DeleteMultipleDynamicItemsInput{
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
		return utils.SendErrorResponse(c, err, "Failed to parse request body")
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, err := getProjectContext(c)
	userIDStr, _ := c.Locals("userID").(string)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	schemaName := c.Query("schemaName")
	log.Printf("Updating multiple items for schema: %s (tenant: %s, project: %s)", schemaName, tenantID, projectID)

	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	}

	dynamicService := services.NewDynamicService()
	responseData, err := dynamicService.UpdateMultipleDynamicItems(ctx, services.UpdateMultipleDynamicItemsInput{
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
		return utils.SendErrorResponse(c, err, "Failed to parse request body")
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

	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	}

	userRole, _ := c.Locals("userRole").(string)
	dynamicService := services.NewDynamicService()
	result, err := dynamicService.GetDynamicItem(ctx, services.GetDynamicItemInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		Schema:    c.Query("schemaName"),
		ID:        c.Params("id"),
		UserRole:  userRole,
		Container: container,
	})
	if err != nil {
		if serviceErr, ok := err.(*services.ServiceError); ok {
			return c.Status(serviceErr.Status).JSON(responses.GeneralResponse{
				Status:  serviceErr.Status,
				Message: serviceErr.Message,
				Data:    serviceErr.Data,
			})
		}
		return utils.SendErrorResponse(c, err, "Item not found")
	}

	if result.FromCache {
		source := "cache"
		return utils.SendResponse(c, http.StatusOK, "Item found", fiber.Map{
			"item":   result.Item,
			"source": &source,
		})
	}

	return c.JSON(result.Item)
}

// handleSearch for a given collection
func HandleSearchDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	}

	dynamicService := services.NewDynamicService()
	params, err := dynamicService.ParseSearchParams(c)
	if err != nil {
		if strings.HasPrefix(err.Error(), "invalid sort params") {
			return utils.SendErrorResponse(c, err, "invalid sort params")
		}
		return utils.SendErrorResponse(c, err, "invalid pagination params")
	}

	userRole, _ := c.Locals("userRole").(string)
	userID, _ := c.Locals("userID").(string)
	result, err := dynamicService.SearchDynamicItems(ctx, services.SearchDynamicItemsInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		Schema:    c.Query("schemaName"),
		SearchKey: params.SearchKey,
		UserID:    userID,
		UserRole:  userRole,
		Sort:      params.Sort,
		Pager:     params.Pager,
		Container: container,
	})
	if err != nil {
		if serviceErr, ok := err.(*services.ServiceError); ok {
			return c.Status(serviceErr.Status).JSON(responses.GeneralResponse{
				Status:  serviceErr.Status,
				Message: serviceErr.Message,
				Data:    serviceErr.Data,
			})
		}
		return utils.SendErrorResponse(c, err, "query failed")
	}

	return c.JSON(result)
}

// HandleFilterDynamicModelItem filters items for a given collection using dynamic query parameters.
func HandleFilterDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	}
	if container == nil {
		container, err = utils.FetchContainerModel(c)
		if err != nil {
			if err == utils.ErrNoSchemaName {
				return utils.SendResponse(c, fiber.StatusBadRequest, err.Error(), nil)
			}
			return utils.SendErrorResponse(c, err, "Failed to load container model")
		}
	}

	dynamicService := services.NewDynamicService()
	params, err := dynamicService.ParseFilterParams(c, container)
	if err != nil {
		if strings.HasPrefix(err.Error(), "invalid filter parameter") {
			return utils.SendErrorResponse(c, err, "Invalid filter parameter")
		}
		if strings.HasPrefix(err.Error(), "invalid sort parameters") {
			return utils.SendErrorResponse(c, err, "Invalid sort parameters")
		}
		return utils.SendErrorResponse(c, err, "Invalid pagination parameters")
	}

	userRole, _ := c.Locals("userRole").(string)
	userID, _ := c.Locals("userID").(string)
	result, err := dynamicService.FilterDynamicItems(ctx, services.FilterDynamicItemsInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		Schema:    container.SchemaName,
		Filter:    params.Filter,
		SearchKey: params.SearchKey,
		UserID:    userID,
		UserRole:  userRole,
		Sort:      params.Sort,
		Pager:     params.Pager,
		Container: container,
	})
	if err != nil {
		if serviceErr, ok := err.(*services.ServiceError); ok {
			return c.Status(serviceErr.Status).JSON(responses.GeneralResponse{
				Status:  serviceErr.Status,
				Message: serviceErr.Message,
				Data:    serviceErr.Data,
			})
		}
		return utils.SendErrorResponse(c, err, "Failed to fetch filtered items")
	}

	return c.JSON(result)
}

// get all item for given collection
func GetPipeline(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	}

	dynamicService := services.NewDynamicService()
	params := dynamicService.ParsePipelineParams(c)
	log.Printf("Fetching pipeline for schema: %s with pipeline name: %s (tenant: %s, project: %s)", params.SchemaName, params.PipelineName, tenantID, projectID)

	items, err := dynamicService.GetPipeline(ctx, services.GetPipelineInput{
		TenantID:     tenantID,
		ProjectID:    projectID,
		Schema:       params.SchemaName,
		PipelineName: params.PipelineName,
		CurrentQuery: params.CurrentQuery,
		Container:    container,
		PrepareStage: func(pipelineJSON string) string {
			return utils.ReplacePlaceholdersWithQueryParams(pipelineJSON, c)
		},
	})
	if err != nil {
		if serviceErr, ok := err.(*services.ServiceError); ok {
			return c.Status(serviceErr.Status).JSON(responses.GeneralResponse{
				Status:  serviceErr.Status,
				Message: serviceErr.Message,
				Data:    serviceErr.Data,
			})
		}
		return utils.SendErrorResponse(c, err, "Failed to execute dynamic pipeline")
	}

	return c.JSON(items)
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
