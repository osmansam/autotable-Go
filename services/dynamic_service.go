package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
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

type bulkUpdateItemResult struct {
	Success interface{}
	Failed  map[string]interface{}
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
