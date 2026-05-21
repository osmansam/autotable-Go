package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/cache"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/events"
	"github.com/osmansam/autotableGo/files"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/requests"
	"github.com/osmansam/autotableGo/utils"
	"github.com/osmansam/autotableGo/validators"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type ServiceError struct {
	Status  int
	Message string
	Data    interface{}
	Err     error
}

func (e *ServiceError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

type CreateDynamicItemInput struct {
	TenantID  string
	ProjectID string
	Schema    string
	UserID    string
	User      *models.AuditUser
	Container *models.ContainerModel
	FiberCtx  *fiber.Ctx
}

type CreateMultipleDynamicItemsInput struct {
	TenantID  string
	ProjectID string
	Schema    string
	UserID    string
	User      *models.AuditUser
	Container *models.ContainerModel
	FiberCtx  *fiber.Ctx
}

type UpdateDynamicItemInput struct {
	TenantID  string
	ProjectID string
	Schema    string
	ID        string
	UserID    string
	User      *models.AuditUser
	Container *models.ContainerModel
	FiberCtx  *fiber.Ctx
}

type UpdateMultipleDynamicItemsInput struct {
	TenantID  string
	ProjectID string
	Schema    string
	UserID    string
	User      *models.AuditUser
	Container *models.ContainerModel
	FiberCtx  *fiber.Ctx
}

type DeleteDynamicItemInput struct {
	TenantID  string
	ProjectID string
	Schema    string
	ID        string
	UserID    string
	User      *models.AuditUser
	Container *models.ContainerModel
}

type DeleteMultipleDynamicItemsInput struct {
	TenantID  string
	ProjectID string
	Schema    string
	UserID    string
	User      *models.AuditUser
	Container *models.ContainerModel
	FiberCtx  *fiber.Ctx
}

type GetAllDynamicItemsInput struct {
	TenantID  string
	ProjectID string
	Schema    string
	UserRole  string
	Container *models.ContainerModel
}

type GetItemsForSelectionInput struct {
	TenantID  string
	ProjectID string
	Schema    string
	FieldName string
	UserRole  string
}

type GetDynamicItemInput struct {
	TenantID  string
	ProjectID string
	Schema    string
	ID        string
	UserRole  string
	Container *models.ContainerModel
}

type GetDynamicItemResult struct {
	Item      map[string]interface{}
	FromCache bool
}

type SearchDynamicItemsInput struct {
	TenantID  string
	ProjectID string
	Schema    string
	SearchKey string
	UserID    string
	UserRole  string
	Sort      bson.D
	Pager     utils.Pager
	Container *models.ContainerModel
}

type DynamicService struct {
	repository *repositories.DynamicRepository
	parser     *requests.DynamicRequestParser
	cache      *cache.DynamicCache
	events     *events.DynamicEvents
}

func NewDynamicService() *DynamicService {
	uploadService := files.NewUploadService()
	return &DynamicService{
		repository: repositories.NewDynamicRepository(),
		parser:     requests.NewDynamicRequestParser(uploadService),
		cache:      cache.NewDynamicCache(),
		events:     events.NewDynamicEvents(),
	}
}

func (s *DynamicService) ParseSearchParams(c *fiber.Ctx) (requests.SearchParams, error) {
	return s.parser.ParseSearchParams(c)
}

func (s *DynamicService) CreateDynamicItem(ctx context.Context, input CreateDynamicItemInput) (map[string]interface{}, error) {
	container, err := s.resolveContainer(input)
	if err != nil {
		log.Printf("Failed to fetch container model for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch container model",
			Data:    err.Error(),
			Err:     err,
		}
	}

	itemMap, err := s.parser.ParseCreateItem(input.FiberCtx, container)
	if err != nil {
		log.Printf("Failed to parse create request for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: parseCreateErrorMessage(err),
			Err:     err,
		}
	}

	if err := validators.PrepareCreateItem(input.TenantID, input.ProjectID, container, itemMap); err != nil {
		log.Printf("Validation/preparation failed for schema: %s, error: %v", input.Schema, err)
		status := http.StatusInternalServerError
		message := "Validation failed."
		var equationErr *validators.EquationFieldError
		if errors.As(err, &equationErr) {
			status = http.StatusBadRequest
			message = fmt.Sprintf("Error evaluating equation for field %s", equationErr.FieldName)
		}
		return nil, &ServiceError{
			Status:  status,
			Message: message,
			Data:    err.Error(),
			Err:     err,
		}
	}

	if err := s.ensureUniqueFields(ctx, input, container, itemMap); err != nil {
		return nil, err
	}

	if err := s.applyAutoIncrementFields(ctx, input.Schema, container, itemMap); err != nil {
		log.Printf("Failed to generate autoIncrement id for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate autoIncrement id",
			Err:     err,
		}
	}

	result, err := s.repository.Insert(ctx, input.TenantID, input.ProjectID, input.Schema, itemMap)
	if err != nil {
		log.Printf("Failed to save item for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to save item.",
			Err:     err,
		}
	}

	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		itemMap["_id"] = oid
	}

	if err := utils.LogCreateAction(ctx, input.TenantID, input.ProjectID, container, input.User, itemMap); err != nil {
		log.Printf("Failed to log create action: %v", err)
	}

	if err := s.cache.InvalidateCreateCaches(ctx, input.TenantID, input.ProjectID, input.Schema, container); err != nil {
		log.Printf("Failed to delete cache for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete the cache for the schema.",
			Err:     err,
		}
	}

	s.events.EmitInvalidate(input.Schema, input.UserID, input.TenantID, input.ProjectID)
	utils.StripHashed(container.Fields, []map[string]interface{}{itemMap})

	return itemMap, nil
}

func (s *DynamicService) CreateMultipleDynamicItems(ctx context.Context, input CreateMultipleDynamicItemsInput) (*mongo.InsertManyResult, error) {
	container, err := s.resolveCreateMultipleContainer(input)
	if err != nil {
		log.Printf("Failed to fetch container model for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch container model",
			Data:    err.Error(),
			Err:     err,
		}
	}

	items, err := s.parser.ParseCreateItems(input.FiberCtx, container, configs.GetMaxBulkWriteLimit())
	if err != nil {
		log.Printf("Failed to parse bulk create request for schema: %s, error: %v", input.Schema, err)
		return nil, parseBulkCreateError(err)
	}

	if err := validators.PrepareCreateItems(input.TenantID, input.ProjectID, container, items); err != nil {
		log.Printf("Validation/preparation failed for schema: %s, error: %v", input.Schema, err)
		status := http.StatusInternalServerError
		message := "Validation failed."
		var equationErr *validators.EquationItemFieldError
		if errors.As(err, &equationErr) {
			status = http.StatusBadRequest
			message = fmt.Sprintf("Error evaluating equation for field %s in item %d", equationErr.FieldName, equationErr.Index)
		} else if strings.HasPrefix(err.Error(), "validation failed for item at index ") {
			message = strings.TrimPrefix(strings.Split(err.Error(), ":")[0], "validation failed for item at index ")
			message = fmt.Sprintf("Validation failed for item at index %s.", message)
		}
		return nil, &ServiceError{
			Status:  status,
			Message: message,
			Data:    err.Error(),
			Err:     err,
		}
	}

	if err := s.applyAutoIncrementFieldsForItems(ctx, input.Schema, container, items); err != nil {
		log.Printf("Failed to generate autoIncrement id for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate autoIncrement id",
			Err:     err,
		}
	}

	if err := s.ensureUniqueCreateItems(ctx, input, container, items); err != nil {
		return nil, err
	}

	result, err := s.repository.InsertMany(ctx, input.TenantID, input.ProjectID, input.Schema, items)
	if err != nil {
		log.Printf("Failed to insert multiple items for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to insert multiple items.",
			Err:     err,
		}
	}

	bulkCreatedDocs := make([]interface{}, 0, len(result.InsertedIDs))
	for i, insertedID := range result.InsertedIDs {
		if oid, ok := insertedID.(primitive.ObjectID); ok && i < len(items) {
			items[i]["_id"] = oid
			bulkCreatedDocs = append(bulkCreatedDocs, items[i])
		}
	}

	if len(bulkCreatedDocs) > 0 {
		if err := utils.LogBulkCreateAction(ctx, input.TenantID, input.ProjectID, container, input.User, bulkCreatedDocs); err != nil {
			log.Printf("Failed to log bulk create: %v", err)
		}
	}

	if err := s.cache.InvalidateUpdateCaches(ctx, input.TenantID, input.ProjectID, input.Schema, container, func(triggeredSchema string) {
		s.events.EmitInvalidate(triggeredSchema, input.UserID, input.TenantID, input.ProjectID)
	}); err != nil {
		log.Printf("Failed to delete cache for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete the cache for the schema.",
			Err:     err,
		}
	}

	s.events.EmitInvalidate(input.Schema, input.UserID, input.TenantID, input.ProjectID)
	return result, nil
}

func (s *DynamicService) UpdateDynamicItem(ctx context.Context, input UpdateDynamicItemInput) (map[string]interface{}, error) {
	updateID, err := primitive.ObjectIDFromHex(input.ID)
	if err != nil {
		log.Printf("Provided ID is not in the valid format for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Provided ID is not in the valid format.",
			Err:     err,
		}
	}

	lockKey := fmt.Sprintf("lock:update:%s:%s", input.Schema, updateID.Hex())
	lockID, locked := utils.AcquireLock(lockKey, 10*time.Second)
	if !locked {
		log.Printf("Another process is already updating this item for schema: %s", input.Schema)
		return nil, &ServiceError{
			Status:  http.StatusConflict,
			Message: "Another process is already updating this item",
			Data:    nil,
		}
	}
	defer utils.ReleaseLock(lockKey, lockID)

	container, err := s.resolveUpdateContainer(input)
	if err != nil {
		log.Printf("Failed to fetch container model for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch container model",
			Err:     err,
		}
	}

	updatedItemMap, err := s.parser.ParseUpdateItem(input.FiberCtx, container)
	if err != nil {
		log.Printf("Failed to parse update request for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: parseUpdateErrorMessage(err),
			Err:     err,
		}
	}

	if err := validators.PrepareUpdateFields(container, updatedItemMap); err != nil {
		log.Printf("Validation failed for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Validation failed",
			Err:     err,
		}
	}

	existingItem, err := s.repository.FindByID(ctx, input.TenantID, input.ProjectID, input.Schema, updateID)
	if err != nil {
		log.Printf("Failed to fetch item for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch item",
			Err:     err,
		}
	}

	beforeDoc := make(map[string]interface{})
	for k, v := range existingItem {
		beforeDoc[k] = v
	}

	if err := validators.PrepareMergedUpdateItem(input.TenantID, input.ProjectID, container, existingItem, updatedItemMap); err != nil {
		log.Printf("Validation/preparation failed for schema: %s, error: %v", input.Schema, err)
		status := http.StatusInternalServerError
		message := "Validation failed"
		var equationErr *validators.EquationFieldError
		if errors.As(err, &equationErr) {
			status = http.StatusBadRequest
			message = fmt.Sprintf("Error evaluating equation for field %s", equationErr.FieldName)
		}
		return nil, &ServiceError{
			Status:  status,
			Message: message,
			Data:    err.Error(),
			Err:     err,
		}
	}

	if err := s.ensureUniqueUpdateFields(ctx, input, container, updatedItemMap, updateID); err != nil {
		return nil, err
	}

	updateResult, err := s.repository.UpdateByID(ctx, input.TenantID, input.ProjectID, input.Schema, updateID, existingItem)
	if err != nil {
		log.Printf("Failed to update item for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update item",
			Err:     err,
		}
	}

	if err := utils.LogUpdateAction(ctx, input.TenantID, input.ProjectID, container, input.User, beforeDoc, existingItem); err != nil {
		log.Printf("Failed to log update action: %v", err)
	}

	if updateResult.MatchedCount == 0 {
		log.Printf("No item found with specified ID for schema: %s", input.Schema)
		return nil, &ServiceError{
			Status:  http.StatusNotFound,
			Message: "No item found with specified ID",
			Data:    nil,
		}
	}

	if err := s.cache.InvalidateUpdateCaches(ctx, input.TenantID, input.ProjectID, input.Schema, container, func(triggeredSchema string) {
		s.events.EmitInvalidate(triggeredSchema, input.UserID, input.TenantID, input.ProjectID)
	}); err != nil {
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete the cache for the schema.",
			Data:    err.Error(),
			Err:     err,
		}
	}

	s.events.EmitInvalidate(input.Schema, input.UserID, input.TenantID, input.ProjectID)

	responseItem := make(map[string]interface{})
	for k, v := range existingItem {
		responseItem[k] = v
	}
	utils.StripHashed(container.Fields, []map[string]interface{}{responseItem})

	return responseItem, nil
}

func (s *DynamicService) UpdateMultipleDynamicItems(ctx context.Context, input UpdateMultipleDynamicItemsInput) (fiber.Map, error) {
	container, err := s.resolveUpdateMultipleContainer(input)
	if err != nil {
		log.Printf("Failed to fetch container model for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch container model",
			Err:     err,
		}
	}

	items, err := s.parser.ParseUpdateItems(input.FiberCtx, container, configs.GetMaxBulkUpdateLimit())
	if err != nil {
		log.Printf("Failed to parse bulk update request for schema: %s, error: %v", input.Schema, err)
		return nil, parseBulkUpdateError(err)
	}

	var successfulUpdates []interface{}
	var failedUpdates []map[string]interface{}
	var bulkBeforeDocs []interface{}
	var bulkAfterDocs []interface{}

	for _, item := range items {
		result, beforeDoc, afterDoc := s.updateOneFromBulk(ctx, input, container, item)
		if result.Failed != nil {
			failedUpdates = append(failedUpdates, result.Failed)
			continue
		}
		successfulUpdates = append(successfulUpdates, result.Success)
		bulkBeforeDocs = append(bulkBeforeDocs, beforeDoc)
		bulkAfterDocs = append(bulkAfterDocs, afterDoc)
	}

	if len(bulkAfterDocs) > 0 {
		if err := utils.LogBulkUpdateAction(ctx, input.TenantID, input.ProjectID, container, input.User, bulkBeforeDocs, bulkAfterDocs); err != nil {
			log.Printf("Failed to log bulk update: %v", err)
		}
	}

	if len(successfulUpdates) > 0 {
		if err := s.cache.InvalidateUpdateCaches(ctx, input.TenantID, input.ProjectID, input.Schema, container, func(triggeredSchema string) {
			s.events.EmitInvalidate(triggeredSchema, input.UserID, input.TenantID, input.ProjectID)
		}); err != nil {
			log.Printf("Failed to delete cache for schema: %s, error: %v", input.Schema, err)
		}
		s.events.EmitInvalidate(input.Schema, input.UserID, input.TenantID, input.ProjectID)
	}

	return fiber.Map{
		"successful": successfulUpdates,
		"failed":     failedUpdates,
	}, nil
}

func (s *DynamicService) DeleteDynamicItem(ctx context.Context, input DeleteDynamicItemInput) (map[string]interface{}, error) {
	deleteID, err := primitive.ObjectIDFromHex(input.ID)
	if err != nil {
		log.Printf("Provided ID is not in the valid format for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Provided ID is not in the valid format.",
			Err:     err,
		}
	}

	lockKey := fmt.Sprintf("lock:delete:%s:%s", input.Schema, deleteID.Hex())
	lockID, locked := utils.AcquireLock(lockKey, 10*time.Second)
	if !locked {
		log.Printf("Another process is already deleting this item for schema: %s", input.Schema)
		return nil, &ServiceError{
			Status:  http.StatusConflict,
			Message: "Another process is already deleting this item",
			Data:    nil,
		}
	}
	defer utils.ReleaseLock(lockKey, lockID)

	allContainers, err := s.repository.GetAllContainerModels()
	if err != nil {
		log.Printf("Failed to retrieve container models for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve container models.",
			Err:     err,
		}
	}

	if err := s.ensureDeleteReferences(ctx, input.TenantID, input.ProjectID, input.Schema, deleteID, allContainers); err != nil {
		return nil, err
	}

	if err := s.forceDeleteReferences(ctx, input.TenantID, input.ProjectID, input.Schema, deleteID, allContainers, true); err != nil {
		return nil, err
	}

	container, err := s.resolveDeleteContainer(input)
	if err != nil {
		log.Printf("Failed to fetch container model for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch container model",
			Err:     err,
		}
	}

	deletedDoc, findErr := s.repository.FindByID(ctx, input.TenantID, input.ProjectID, input.Schema, deleteID)
	if findErr != nil {
		log.Printf("Failed to fetch item before delete for schema: %s, error: %v", input.Schema, findErr)
	}

	if _, err := s.repository.DeleteByID(ctx, input.TenantID, input.ProjectID, input.Schema, deleteID); err != nil {
		log.Printf("Failed to delete the item from the specified collection for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete the item from the specified collection.",
			Err:     err,
		}
	}

	if deletedDoc != nil {
		if err := utils.LogDeleteAction(ctx, input.TenantID, input.ProjectID, container, input.User, deletedDoc); err != nil {
			log.Printf("Failed to log delete action: %v", err)
		}
	}

	if err := s.cache.InvalidateCreateCaches(ctx, input.TenantID, input.ProjectID, input.Schema, container); err != nil {
		log.Printf("Failed to delete cache for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete the cache for the schema.",
			Err:     err,
		}
	}

	s.events.EmitInvalidate(input.Schema, input.UserID, input.TenantID, input.ProjectID)

	responseItem := make(map[string]interface{})
	if deletedDoc != nil {
		for k, v := range deletedDoc {
			responseItem[k] = v
		}
		utils.StripHashed(container.Fields, []map[string]interface{}{responseItem})
	}

	return responseItem, nil
}

func (s *DynamicService) DeleteMultipleDynamicItems(ctx context.Context, input DeleteMultipleDynamicItemsInput) (fiber.Map, error) {
	items, err := s.parser.ParseDeleteItems(input.FiberCtx, configs.GetMaxBulkDeleteLimit())
	if err != nil {
		log.Printf("Failed to parse bulk delete request for schema: %s, error: %v", input.Schema, err)
		return nil, parseBulkDeleteError(err)
	}

	allContainers, err := s.repository.GetAllContainerModels()
	if err != nil {
		log.Printf("Failed to retrieve container models for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve container models",
			Err:     err,
		}
	}

	container, err := s.resolveDeleteMultipleContainer(input)
	if err != nil {
		log.Printf("Failed to fetch container model for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch container model",
			Err:     err,
		}
	}

	var successfulDeletes []interface{}
	var failedDeletes []map[string]interface{}
	var bulkDeletedDocs []interface{}

	for _, item := range items {
		result, deletedDoc := s.deleteOneFromBulk(ctx, input, allContainers, item)
		if result.Failed != nil {
			failedDeletes = append(failedDeletes, result.Failed)
			continue
		}
		successfulDeletes = append(successfulDeletes, result.Success)
		if deletedDoc != nil {
			bulkDeletedDocs = append(bulkDeletedDocs, deletedDoc)
		}
	}

	if len(successfulDeletes) > 0 {
		if err := s.cache.InvalidateUpdateCaches(ctx, input.TenantID, input.ProjectID, input.Schema, container, func(triggeredSchema string) {
			s.events.EmitInvalidate(triggeredSchema, input.UserID, input.TenantID, input.ProjectID)
		}); err != nil {
			log.Printf("Failed to delete cache for schema: %s, error: %v", input.Schema, err)
		}
		s.events.EmitInvalidate(input.Schema, input.UserID, input.TenantID, input.ProjectID)
	}

	if len(bulkDeletedDocs) > 0 {
		if err := utils.LogBulkDeleteAction(ctx, input.TenantID, input.ProjectID, container, input.User, bulkDeletedDocs); err != nil {
			log.Printf("Failed to log bulk delete: %v", err)
		}
	}

	return fiber.Map{
		"successful": successfulDeletes,
		"failed":     failedDeletes,
	}, nil
}

func (s *DynamicService) GetAllDynamicItems(ctx context.Context, input GetAllDynamicItemsInput) ([]map[string]interface{}, error) {
	container, err := s.resolveGetAllContainer(input)
	if err != nil {
		if input.Schema == "" {
			return nil, &ServiceError{
				Status:  http.StatusBadRequest,
				Message: "schemaName is required",
				Data:    nil,
				Err:     err,
			}
		}
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to load container model",
			Err:     err,
		}
	}

	schema := container.SchemaName
	redisKey, shouldCache := utils.GenerateRedisKey("GetAllDynamicModelItems", input.TenantID, input.ProjectID, schema, container)
	if shouldCache {
		if items, ok := s.cache.GetItems(ctx, redisKey); ok {
			log.Printf("Fetched items from cache for schema: %s", schema)
			if maxUnboundedRead := configs.GetMaxUnboundedReadLimit(); len(items) > maxUnboundedRead {
				log.Printf("Unbounded read limit exceeded from cache for schema=%s tenant=%s project=%s requested=%d max=%d", schema, input.TenantID, input.ProjectID, len(items), maxUnboundedRead)
				items = items[:maxUnboundedRead]
			}
			return utils.FilterDocuments(items, container.Fields, input.UserRole), nil
		}
	}

	maxUnboundedRead := configs.GetMaxUnboundedReadLimit()
	items, err := s.repository.FindAll(ctx, input.TenantID, input.ProjectID, schema, int64(maxUnboundedRead+1))
	if err != nil {
		log.Printf("DB query failed for schema %q: %v", schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch items",
			Err:     err,
		}
	}
	if len(items) > maxUnboundedRead {
		log.Printf("Unbounded read limit exceeded for schema=%s tenant=%s project=%s max=%d", schema, input.TenantID, input.ProjectID, maxUnboundedRead)
		items = items[:maxUnboundedRead]
	}

	utils.StripHashed(container.Fields, items)

	items, err = utils.PopulateIfNeeded(ctx, input.TenantID, input.ProjectID, container, items)
	if err != nil {
		log.Printf("Population failed for schema %q: %v", schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to populate items",
			Err:     err,
		}
	}

	items = utils.FilterDocuments(items, container.Fields, input.UserRole)
	if shouldCache {
		s.cache.SetItems(ctx, redisKey, items, container.Redis.CacheTime)
	}

	log.Printf("Fetched items from DB for schema: %s", schema)
	return items, nil
}

func (s *DynamicService) GetItemsForSelection(ctx context.Context, input GetItemsForSelectionInput) ([]map[string]interface{}, error) {
	if input.Schema == "" || input.FieldName == "" {
		return nil, &ServiceError{
			Status:  http.StatusBadRequest,
			Message: "schemaName and fieldName are required",
			Data:    nil,
		}
	}

	container, err := s.repository.GetContainerModel(input.TenantID, input.ProjectID, input.Schema)
	if err != nil {
		log.Printf("Failed to get container model for schema %s: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to load schema configuration",
			Err:     err,
		}
	}

	for _, field := range container.Fields {
		if field.Name != input.FieldName {
			continue
		}

		if len(field.AuthorizeRole) > 0 {
			authorized := false
			for _, role := range field.AuthorizeRole {
				if role == input.UserRole {
					authorized = true
					break
				}
			}
			if !authorized {
				return nil, &ServiceError{
					Status:  http.StatusForbidden,
					Message: "Access to this field is restricted",
					Data:    nil,
				}
			}
		}

		if field.IsHashed {
			log.Printf("Attempted to access hashed field %s in schema %s", input.FieldName, input.Schema)
			return nil, &ServiceError{
				Status:  http.StatusForbidden,
				Message: "Cannot access hashed fields",
				Data:    nil,
			}
		}
	}

	items, err := s.repository.FindForSelection(ctx, input.TenantID, input.ProjectID, input.Schema, input.FieldName)
	if err != nil {
		log.Printf("Failed to query collection %s: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch items",
			Err:     err,
		}
	}

	log.Printf("Successfully fetched %d items for selection", len(items))
	return utils.FilterDocuments(items, container.Fields, input.UserRole), nil
}

func (s *DynamicService) GetDynamicItem(ctx context.Context, input GetDynamicItemInput) (GetDynamicItemResult, error) {
	container, err := s.resolveGetDynamicContainer(input)
	if err != nil {
		if input.Schema == "" {
			return GetDynamicItemResult{}, &ServiceError{
				Status:  http.StatusBadRequest,
				Message: "schemaName is required",
				Data:    nil,
				Err:     err,
			}
		}
		return GetDynamicItemResult{}, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to load container model",
			Err:     err,
		}
	}

	filter, err := buildGetDynamicItemFilter(container, input.ID)
	if err != nil {
		return GetDynamicItemResult{}, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Invalid ID",
			Err:     err,
		}
	}

	redisKey, shouldCache := utils.GenerateRedisKey("GetDynamicModelItem", input.TenantID, input.ProjectID, container.SchemaName, container, input.ID)
	if shouldCache {
		if item, ok := s.cache.GetItem(ctx, redisKey); ok {
			utils.StripHashed(container.Fields, []map[string]interface{}{item})
			populated, _ := utils.PopulateIfNeeded(ctx, input.TenantID, input.ProjectID, container, []map[string]interface{}{item})
			if len(populated) > 0 {
				item = populated[0]
			}
			return GetDynamicItemResult{Item: item, FromCache: true}, nil
		}
	}

	rawDoc, err := s.repository.FindOne(ctx, input.TenantID, input.ProjectID, container.SchemaName, filter)
	if err != nil {
		return GetDynamicItemResult{}, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Item not found",
			Err:     err,
		}
	}

	item := make(map[string]interface{}, len(rawDoc))
	for k, v := range rawDoc {
		item[k] = v
	}

	utils.StripHashed(container.Fields, []map[string]interface{}{item})
	populated, err := utils.PopulateIfNeeded(ctx, input.TenantID, input.ProjectID, container, []map[string]interface{}{item})
	if err != nil {
		return GetDynamicItemResult{}, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to populate item",
			Err:     err,
		}
	}
	if len(populated) > 0 {
		item = populated[0]
	}

	filtered := utils.FilterDocuments([]map[string]interface{}{item}, container.Fields, input.UserRole)
	if len(filtered) > 0 {
		item = filtered[0]
	}

	if shouldCache {
		s.cache.SetItem(ctx, redisKey, item, container.Redis.CacheTime)
	}

	return GetDynamicItemResult{Item: item}, nil
}

func (s *DynamicService) SearchDynamicItems(ctx context.Context, input SearchDynamicItemsInput) (interface{}, error) {
	container, err := s.resolveSearchContainer(input)
	if err != nil {
		if input.Schema == "" {
			return nil, &ServiceError{
				Status:  http.StatusBadRequest,
				Message: "schemaName is required",
				Data:    nil,
				Err:     err,
			}
		}
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "failed to load container",
			Err:     err,
		}
	}

	orClauses, err := utils.BuildSearchWithReferences(ctx, container, input.SearchKey)
	if err != nil {
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "failed to build search filter",
			Err:     err,
		}
	}

	var filter bson.M
	if input.SearchKey == "" {
		filter = bson.M{}
	} else if len(orClauses) == 0 {
		return []interface{}{}, nil
	} else {
		filter = bson.M{"$or": orClauses}
	}

	userMap := map[string]interface{}{"id": input.UserID, "_id": input.UserID, "role": input.UserRole}
	rowAccessFilter, err := utils.GetRowAccessFilter(container, input.UserRole, userMap)
	if err != nil {
		log.Printf("Error building row access filter: %v", err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "row access error",
			Err:     err,
		}
	}
	if rowAccessFilter != nil {
		if len(filter) > 0 {
			filter = bson.M{"$and": []bson.M{filter, rowAccessFilter}}
		} else {
			filter = rowAccessFilter
		}
	}

	pager := input.Pager
	opts := utils.BuildFindOptions(input.Sort, pager)
	items, err := s.repository.Query(ctx, input.TenantID, input.ProjectID, input.Schema, filter, opts, &pager)
	if err != nil {
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "query failed",
			Err:     err,
		}
	}

	utils.StripHashed(container.Fields, items)
	items, err = utils.PopulateIfNeeded(ctx, input.TenantID, input.ProjectID, container, items)
	if err != nil {
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "population failed",
			Err:     err,
		}
	}

	items = utils.FilterDocuments(items, container.Fields, input.UserRole)
	if pager.Enabled {
		return fiber.Map{
			"items":       items,
			"totalItems":  pager.TotalItems,
			"totalPages":  pager.TotalPages,
			"currentPage": pager.Page,
		}, nil
	}

	return items, nil
}

type bulkUpdateItemResult struct {
	Success interface{}
	Failed  map[string]interface{}
}

type bulkDeleteItemResult struct {
	Success interface{}
	Failed  map[string]interface{}
}

func (s *DynamicService) deleteOneFromBulk(ctx context.Context, input DeleteMultipleDynamicItemsInput, allContainers []models.ContainerModel, item map[string]interface{}) (bulkDeleteItemResult, interface{}) {
	idStr, errMessage := extractBulkUpdateID(item)
	if errMessage != "" {
		return bulkDeleteItemResult{Failed: map[string]interface{}{
			"item":  item,
			"error": errMessage,
		}}, nil
	}

	deleteID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		return bulkDeleteItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "Provided ID is not in the valid format",
		}}, nil
	}

	lockKey := fmt.Sprintf("lock:delete:%s:%s", input.Schema, deleteID.Hex())
	lockID, locked := utils.AcquireLock(lockKey, 10*time.Second)
	if !locked {
		return bulkDeleteItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "Another process is already deleting this item",
		}}, nil
	}
	defer utils.ReleaseLock(lockKey, lockID)

	if err := s.ensureDeleteReferences(ctx, input.TenantID, input.ProjectID, input.Schema, deleteID, allContainers); err != nil {
		if serviceErr, ok := err.(*ServiceError); ok {
			return bulkDeleteItemResult{Failed: map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": serviceErr.Message,
			}}, nil
		}
		return bulkDeleteItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": err.Error(),
		}}, nil
	}

	if err := s.forceDeleteReferences(ctx, input.TenantID, input.ProjectID, input.Schema, deleteID, allContainers, false); err != nil {
		log.Printf("Failed to force delete referenced items for schema: %s, error: %v", input.Schema, err)
	}

	deletedDoc, findErr := s.repository.FindByID(ctx, input.TenantID, input.ProjectID, input.Schema, deleteID)
	if findErr != nil {
		log.Printf("Failed to fetch item before delete (multiple) for schema: %s, error: %v", input.Schema, findErr)
	}

	deleteResult, err := s.repository.DeleteByID(ctx, input.TenantID, input.ProjectID, input.Schema, deleteID)
	if err != nil {
		return bulkDeleteItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "Failed to delete item: " + err.Error(),
		}}, nil
	}
	if deleteResult.DeletedCount == 0 {
		return bulkDeleteItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "No item found with the specified ID",
		}}, nil
	}

	return bulkDeleteItemResult{Success: map[string]interface{}{
		"id":     idStr,
		"result": deleteResult,
	}}, deletedDoc
}

func (s *DynamicService) updateOneFromBulk(ctx context.Context, input UpdateMultipleDynamicItemsInput, container *models.ContainerModel, item map[string]interface{}) (bulkUpdateItemResult, interface{}, interface{}) {
	idStr, errMessage := extractBulkUpdateID(item)
	if errMessage != "" {
		return bulkUpdateItemResult{Failed: map[string]interface{}{
			"item":  item,
			"error": errMessage,
		}}, nil, nil
	}

	updateID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		return bulkUpdateItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "Provided ID is not in the valid format",
		}}, nil, nil
	}

	lockKey := fmt.Sprintf("lock:update:%s:%s", input.Schema, updateID.Hex())
	lockID, locked := utils.AcquireLock(lockKey, 10*time.Second)
	if !locked {
		return bulkUpdateItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "Another process is already updating this item",
		}}, nil, nil
	}
	defer utils.ReleaseLock(lockKey, lockID)

	delete(item, "id")
	delete(item, "_id")

	if err := validators.PrepareUpdateFields(container, item); err != nil {
		return bulkUpdateItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "Validation failed: " + err.Error(),
		}}, nil, nil
	}

	existingItem, err := s.repository.FindByID(ctx, input.TenantID, input.ProjectID, input.Schema, updateID)
	if err != nil {
		return bulkUpdateItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "Failed to fetch existing item: " + err.Error(),
		}}, nil, nil
	}

	beforeDoc := make(map[string]interface{})
	for k, v := range existingItem {
		beforeDoc[k] = v
	}

	if err := validators.PrepareMergedUpdateItem(input.TenantID, input.ProjectID, container, existingItem, item); err != nil {
		var equationErr *validators.EquationFieldError
		if errors.As(err, &equationErr) {
			return bulkUpdateItemResult{Failed: map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": fmt.Sprintf("Error evaluating equation for field %s: %v", equationErr.FieldName, equationErr.Err),
			}}, nil, nil
		}
		return bulkUpdateItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": err.Error(),
		}}, nil, nil
	}

	for _, field := range container.Fields {
		if !field.Unique {
			continue
		}
		fieldValue, found := item[field.Name]
		if !found {
			continue
		}
		count, err := s.repository.CountByFieldExcludingID(ctx, input.TenantID, input.ProjectID, input.Schema, field.Name, fieldValue, updateID)
		if err != nil {
			return bulkUpdateItemResult{Failed: map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": "Error checking unique field: " + err.Error(),
			}}, nil, nil
		}
		if count > 0 {
			return bulkUpdateItemResult{Failed: map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": fmt.Sprintf("A document with the same %s already exists", field.Name),
			}}, nil, nil
		}
	}

	updateResult, err := s.repository.UpdateByID(ctx, input.TenantID, input.ProjectID, input.Schema, updateID, existingItem)
	if err != nil {
		return bulkUpdateItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "Failed to update item: " + err.Error(),
		}}, nil, nil
	}
	if updateResult.MatchedCount == 0 {
		return bulkUpdateItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "No matching item found to update",
		}}, nil, nil
	}

	return bulkUpdateItemResult{Success: map[string]interface{}{
		"id":     idStr,
		"result": updateResult,
	}}, beforeDoc, existingItem
}

func (s *DynamicService) resolveContainer(input CreateDynamicItemInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveCreateMultipleContainer(input CreateMultipleDynamicItemsInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveUpdateContainer(input UpdateDynamicItemInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveUpdateMultipleContainer(input UpdateMultipleDynamicItemsInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveDeleteContainer(input DeleteDynamicItemInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveDeleteMultipleContainer(input DeleteMultipleDynamicItemsInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveGetAllContainer(input GetAllDynamicItemsInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	if input.Schema == "" {
		return nil, utils.ErrNoSchemaName
	}
	return s.repository.GetContainerModel(input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveGetDynamicContainer(input GetDynamicItemInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	if input.Schema == "" {
		return nil, utils.ErrNoSchemaName
	}
	return s.repository.GetContainerModel(input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveSearchContainer(input SearchDynamicItemsInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	if input.Schema == "" {
		return nil, utils.ErrNoSchemaName
	}
	return s.repository.GetContainerModel(input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) ensureUniqueFields(ctx context.Context, input CreateDynamicItemInput, container *models.ContainerModel, itemMap map[string]interface{}) error {
	for _, field := range container.Fields {
		if !field.Unique {
			continue
		}

		fieldValue, found := itemMap[field.Name]
		if !found {
			continue
		}

		count, err := s.repository.CountByField(ctx, input.TenantID, input.ProjectID, input.Schema, field.Name, fieldValue)
		if err != nil {
			log.Printf("Error checking existing field value for schema: %s, error: %v", input.Schema, err)
			return &ServiceError{
				Status:  http.StatusInternalServerError,
				Message: "Error checking existing field value.",
				Err:     err,
			}
		}

		if count > 0 {
			log.Printf("Duplicate field value found for schema: %s, field: %s", input.Schema, field.Name)
			return &ServiceError{
				Status:  http.StatusBadRequest,
				Message: fmt.Sprintf("A document with the same %s already exists.", field.Name),
				Data:    nil,
			}
		}
	}
	return nil
}

func (s *DynamicService) ensureUniqueUpdateFields(ctx context.Context, input UpdateDynamicItemInput, container *models.ContainerModel, updatedItemMap map[string]interface{}, updateID primitive.ObjectID) error {
	for _, field := range container.Fields {
		if !field.Unique {
			continue
		}

		fieldValue, found := updatedItemMap[field.Name]
		if !found {
			continue
		}

		count, err := s.repository.CountByFieldExcludingID(ctx, input.TenantID, input.ProjectID, input.Schema, field.Name, fieldValue, updateID)
		if err != nil {
			log.Printf("Error checking existing field value for schema: %s, error: %v", input.Schema, err)
			return &ServiceError{
				Status:  http.StatusInternalServerError,
				Message: "Error checking existing field value.",
				Err:     err,
			}
		}

		if count > 0 {
			log.Printf("Duplicate field value found for schema: %s, field: %s", input.Schema, field.Name)
			return &ServiceError{
				Status:  http.StatusBadRequest,
				Message: fmt.Sprintf("A document with the same %s already exists.", field.Name),
				Data:    nil,
			}
		}
	}
	return nil
}

func (s *DynamicService) ensureUniqueCreateItems(ctx context.Context, input CreateMultipleDynamicItemsInput, container *models.ContainerModel, items []map[string]interface{}) error {
	for _, field := range container.Fields {
		if !field.Unique {
			continue
		}

		valueSet := make(map[interface{}]bool)
		values := make([]interface{}, 0, len(items))
		for i, item := range items {
			fieldValue, found := item[field.Name]
			if !found {
				continue
			}
			if _, exists := valueSet[fieldValue]; exists {
				msg := fmt.Sprintf("Duplicate value for unique field '%s' in item index %d", field.Name, i)
				log.Printf("%s", msg)
				return &ServiceError{
					Status:  http.StatusBadRequest,
					Message: msg,
					Data:    nil,
				}
			}
			valueSet[fieldValue] = true
			values = append(values, fieldValue)
		}

		if len(values) == 0 {
			continue
		}

		count, err := s.repository.CountByFieldIn(ctx, input.TenantID, input.ProjectID, input.Schema, field.Name, values)
		if err != nil {
			log.Printf("Error checking unique field '%s' for schema: %s, error: %v", field.Name, input.Schema, err)
			return &ServiceError{
				Status:  http.StatusInternalServerError,
				Message: fmt.Sprintf("Error checking unique field '%s'.", field.Name),
				Err:     err,
			}
		}
		if count > 0 {
			msg := fmt.Sprintf("A document with the same '%s' already exists.", field.Name)
			log.Printf("%s", msg)
			return &ServiceError{
				Status:  http.StatusBadRequest,
				Message: msg,
				Data:    nil,
			}
		}
	}

	return nil
}

func (s *DynamicService) ensureDeleteReferences(ctx context.Context, tenantID, projectID, schemaName string, deleteID primitive.ObjectID, allContainers []models.ContainerModel) error {
	for _, container := range allContainers {
		if container.SchemaName == schemaName {
			continue
		}
		for _, field := range container.Fields {
			if field.Type != "objectId" || field.Name != schemaName {
				continue
			}
			count, err := s.repository.CountByField(ctx, tenantID, projectID, container.SchemaName, field.Name, deleteID)
			if err != nil {
				log.Printf("Database error while checking references for schema: %s, error: %v", schemaName, err)
				return &ServiceError{
					Status:  http.StatusInternalServerError,
					Message: "Database error while checking references.",
					Err:     err,
				}
			}
			if count > 0 && !field.IsForceDelete {
				return &ServiceError{
					Status:  http.StatusBadRequest,
					Message: fmt.Sprintf("Cannot delete: This item is still referenced in schema '%s' and cannot be forcibly deleted.", container.SchemaName),
					Data:    nil,
				}
			}
		}
	}
	return nil
}

func (s *DynamicService) forceDeleteReferences(ctx context.Context, tenantID, projectID, schemaName string, deleteID primitive.ObjectID, allContainers []models.ContainerModel, failOnError bool) error {
	for _, container := range allContainers {
		if container.SchemaName == schemaName {
			continue
		}
		for _, field := range container.Fields {
			if field.Type != "objectId" || field.Name != schemaName || !field.IsForceDelete {
				continue
			}
			if _, err := s.repository.DeleteManyByField(ctx, tenantID, projectID, container.SchemaName, field.Name, deleteID); err != nil {
				log.Printf("Failed to force delete referenced items for schema: %s, error: %v", schemaName, err)
				if failOnError {
					return &ServiceError{
						Status:  http.StatusInternalServerError,
						Message: "Failed to force delete referenced items.",
						Err:     err,
					}
				}
			}
		}
	}
	return nil
}

func (s *DynamicService) applyAutoIncrementFields(ctx context.Context, schemaName string, container *models.ContainerModel, itemMap map[string]interface{}) error {
	for _, field := range container.Fields {
		if field.Type != "autoIncrementId" {
			continue
		}
		if _, exists := itemMap[field.Name]; exists {
			continue
		}
		seq, err := s.repository.NextSequence(ctx, schemaName)
		if err != nil {
			return err
		}
		itemMap[field.Name] = seq
	}
	return nil
}

func (s *DynamicService) applyAutoIncrementFieldsForItems(ctx context.Context, schemaName string, container *models.ContainerModel, items []map[string]interface{}) error {
	for _, item := range items {
		for _, field := range container.Fields {
			if field.Type != "autoIncrementId" {
				continue
			}
			if val, exists := item[field.Name]; exists && fmt.Sprintf("%v", val) != "" {
				continue
			}
			seq, err := s.repository.NextSequence(ctx, schemaName)
			if err != nil {
				return err
			}
			item[field.Name] = seq
		}
	}
	return nil
}

func parseCreateErrorMessage(err error) string {
	if err == nil {
		return "Failed to parse body."
	}
	if strings.Contains(err.Error(), "multipart") {
		return "Error parsing form."
	}
	return "Failed to parse body."
}

func parseUpdateErrorMessage(err error) string {
	if err == nil {
		return "Failed to parse body"
	}
	if strings.Contains(err.Error(), "multipart") {
		return "Error in multipart form"
	}
	return "Failed to parse body"
}

func parseBulkCreateError(err error) *ServiceError {
	var limitErr *requests.BatchLimitError
	if errors.As(err, &limitErr) {
		return &ServiceError{
			Status:  http.StatusBadRequest,
			Message: limitErr.Error(),
			Data: fiber.Map{
				"requested": limitErr.Requested,
				"max":       limitErr.Max,
			},
			Err: err,
		}
	}

	message := "Failed to parse request body"
	if err != nil {
		if strings.Contains(err.Error(), "multipart") {
			message = "Error parsing multipart form."
		} else if err.Error() == "missing items field" {
			message = "Missing items JSON field."
		} else if strings.HasPrefix(err.Error(), "Expected ") {
			message = err.Error()
		}
	}

	return &ServiceError{
		Status:  http.StatusInternalServerError,
		Message: message,
		Err:     err,
	}
}

func parseBulkUpdateError(err error) *ServiceError {
	var limitErr *requests.BatchLimitError
	if errors.As(err, &limitErr) {
		return &ServiceError{
			Status:  http.StatusBadRequest,
			Message: limitErr.Error(),
			Data: fiber.Map{
				"requested": limitErr.Requested,
				"max":       limitErr.Max,
			},
			Err: err,
		}
	}

	message := "Failed to parse request body"
	if err != nil {
		if strings.Contains(err.Error(), "multipart") {
			message = "Error parsing multipart form"
		} else if err.Error() == "missing items field" {
			message = "Missing items JSON field"
		} else if strings.HasPrefix(err.Error(), "Expected ") {
			message = err.Error()
		}
	}

	return &ServiceError{
		Status:  http.StatusInternalServerError,
		Message: message,
		Err:     err,
	}
}

func parseBulkDeleteError(err error) *ServiceError {
	var limitErr *requests.BatchLimitError
	if errors.As(err, &limitErr) {
		return &ServiceError{
			Status:  http.StatusBadRequest,
			Message: limitErr.Error(),
			Data: fiber.Map{
				"requested": limitErr.Requested,
				"max":       limitErr.Max,
			},
			Err: err,
		}
	}

	return &ServiceError{
		Status:  http.StatusInternalServerError,
		Message: "Failed to parse request body",
		Err:     err,
	}
}

func buildGetDynamicItemFilter(container *models.ContainerModel, idParam string) (bson.M, error) {
	var autoIncrementField string
	for _, field := range container.Fields {
		if field.Type == "autoIncrementId" {
			autoIncrementField = field.Name
			break
		}
	}

	if autoIncrementField != "" {
		if idInt, err := strconv.Atoi(idParam); err == nil {
			return bson.M{autoIncrementField: idInt}, nil
		}
		if objID, err := primitive.ObjectIDFromHex(idParam); err == nil {
			return bson.M{"_id": objID}, nil
		}
		return nil, errors.New("invalid id format")
	}

	objID, err := primitive.ObjectIDFromHex(idParam)
	if err != nil {
		return nil, err
	}
	return bson.M{"_id": objID}, nil
}

func extractBulkUpdateID(item map[string]interface{}) (string, string) {
	if v, ok := item["id"]; ok {
		idStr, ok := v.(string)
		if !ok {
			return "", "Invalid id format, expected string"
		}
		return idStr, ""
	}
	if v, ok := item["_id"]; ok {
		idStr, ok := v.(string)
		if !ok {
			return "", "Invalid _id format, expected string"
		}
		return idStr, ""
	}
	return "", "Missing id field"
}
