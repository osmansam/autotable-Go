package controllers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/services"
	"github.com/osmansam/autotableGo/utils"
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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

	dynamicService := services.NewDynamicService()
	params, err := dynamicService.ParsePaginatedItemsParams(c, container)
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
	result, err := dynamicService.GetAllDynamicItemsWithPagination(ctx, services.GetPaginatedDynamicItemsInput{
		TenantID:    tenantID,
		ProjectID:   projectID,
		Schema:      container.SchemaName,
		QueryString: params.QueryString,
		Filter:      params.Filter,
		SearchKey:   params.SearchKey,
		UserID:      userID,
		UserRole:    userRole,
		Sort:        params.Sort,
		Pager:       params.Pager,
		Container:   container,
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

	return c.JSON(result)
}

// executeDynamicCode executes dynamic code from a request.
func ExecuteDynamicCode(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	schemaName := c.Query("schemaName")
	functionName := c.Query("functionName")
	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	}

	dynamicService := services.NewDynamicService()
	result, err := dynamicService.ExecuteDynamicCode(ctx, services.ExecuteDynamicCodeInput{
		TenantID:     tenantID,
		ProjectID:    projectID,
		Schema:       schemaName,
		FunctionName: functionName,
		CurrentQuery: c.OriginalURL(),
		Container:    container,
		FiberCtx:     c,
	})
	if err != nil {
		if serviceErr, ok := err.(*services.ServiceError); ok {
			return c.Status(serviceErr.Status).JSON(responses.GeneralResponse{
				Status:  serviceErr.Status,
				Message: serviceErr.Message,
				Data:    serviceErr.Data,
			})
		}
		return utils.SendErrorResponse(c, err, "Failed to execute function")
	}

	return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: result.Message,
		Data:    result.Data,
		Source:  utils.PointerToString(result.Source),
	})
}
func TestPipeline(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	schemaName := c.Query("schemaName")
	dynamicService := services.NewDynamicService()
	requestBody, err := dynamicService.ParseTestPipeline(c)
	if err != nil {
		return utils.SendErrorResponse(c, err, "Invalid request body")
	}

	userRole, _ := c.Locals("userRole").(string)
	items, err := dynamicService.TestPipeline(ctx, services.TestPipelineInput{
		TenantID:      tenantID,
		ProjectID:     projectID,
		Schema:        schemaName,
		UserRole:      userRole,
		PipelineStage: requestBody.PipelineStage,
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
		return utils.SendErrorResponse(c, err, "Failed to execute test pipeline")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":  fiber.StatusOK,
		"message": "Test pipeline executed and filtered successfully",
		"data":    items,
	})
}

// TODO:redis generate key and delete key will added into this function and then the route will be added and tested again
func ExecuteDynamicAPI(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	schemaName := c.Query("schemaName")
	apiName := c.Query("apiName")
	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		container, _ = storedContainer.(*models.ContainerModel)
	}

	dynamicService := services.NewDynamicService()
	requestBody, err := dynamicService.ParseDynamicAPIRequest(c)
	if err != nil {
		return utils.SendErrorResponse(c, err, "Invalid request body")
	}

	result, err := dynamicService.ExecuteDynamicAPI(ctx, services.ExecuteDynamicAPIInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		Schema:    schemaName,
		APIName:   apiName,
		Body:      requestBody,
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
		return utils.SendErrorResponse(c, err, "Failed to execute API call")
	}

	return c.Status(fiber.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: result.Message,
		Data:    result.Data,
		Source:  utils.PointerToString(result.Source),
	})
}

// ExportDynamicModelItems exports items to an Excel file based on selected fields and filters.
func ExportDynamicModelItems(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		log.Printf("Project context error: %v", err)
		return utils.SendErrorResponse(c, err, err.Error())
	}

	dynamicService := services.NewDynamicService()
	req, err := dynamicService.ParseExportRequest(c)
	if err != nil {
		return utils.SendErrorResponse(c, err, "Failed to parse request body")
	}

	result, err := dynamicService.ExportDynamicItems(ctx, services.ExportDynamicItemsInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		Request:   req,
	})
	if err != nil {
		if serviceErr, ok := err.(*services.ServiceError); ok {
			return c.Status(serviceErr.Status).JSON(responses.GeneralResponse{
				Status:  serviceErr.Status,
				Message: serviceErr.Message,
				Data:    serviceErr.Data,
			})
		}
		return utils.SendErrorResponse(c, err, "Failed to export items")
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", result.FileName))

	return c.Send(result.Content)
}
