package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"plugin"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/cache"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/events"
	"github.com/osmansam/autotableGo/files"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/observability"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/requests"
	"github.com/osmansam/autotableGo/utils"
	"github.com/osmansam/autotableGo/validators"
	"github.com/xuri/excelize/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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

func (e *ServiceError) Unwrap() error {
	return e.Err
}

func workflowExecutionServiceError(err error, fallbackMessage string) *ServiceError {
	var serviceErr *ServiceError
	if errors.As(err, &serviceErr) {
		return serviceErr
	}
	var businessErr *workflowBusinessError
	if errors.As(err, &businessErr) {
		status := businessErr.Status
		if status < http.StatusBadRequest || status > 599 {
			status = http.StatusBadRequest
		}
		return &ServiceError{
			Status:  status,
			Message: businessErr.Message,
			Err:     err,
		}
	}
	return &ServiceError{
		Status:  http.StatusInternalServerError,
		Message: fallbackMessage,
		Err:     err,
	}
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
	TenantID   string
	ProjectID  string
	Schema     string
	FieldName  string
	ValueField string
	UserRole   string
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

type FilterDynamicItemsInput struct {
	TenantID  string
	ProjectID string
	Schema    string
	Filter    bson.M
	SearchKey string
	UserID    string
	UserRole  string
	Sort      bson.D
	Pager     utils.Pager
	Container *models.ContainerModel
}

type GetPaginatedDynamicItemsInput struct {
	TenantID    string
	ProjectID   string
	Schema      string
	QueryString string
	Filter      bson.M
	SearchKey   string
	UserID      string
	UserRole    string
	Sort        bson.D
	Pager       utils.Pager
	Container   *models.ContainerModel
}

type GetTableSourceInput struct {
	TenantID     string
	ProjectID    string
	SourceType   string
	Schema       string
	PipelineName string
	WorkflowName string
	QueryString  string
	Filter       bson.M
	SearchKey    string
	UserID       string
	UserRole     string
	Sort         bson.D
	Pager        utils.Pager
	Fields       []string
	Params       map[string]interface{}
	Container    *models.ContainerModel
	PrepareStage func(string) string
	AuditUser    *models.AuditUser
}

type GetPipelineInput struct {
	TenantID     string
	ProjectID    string
	Schema       string
	PipelineName string
	CurrentQuery string
	Container    *models.ContainerModel
	PrepareStage func(string) string
}

type ExecuteDynamicCodeInput struct {
	TenantID     string
	ProjectID    string
	Schema       string
	FunctionName string
	CurrentQuery string
	Container    *models.ContainerModel
	FiberCtx     *fiber.Ctx
}

type DynamicExecutionResult struct {
	Message    string
	Data       interface{}
	Source     string
	pagination *workflowTableSourcePagination
}

type TestPipelineInput struct {
	TenantID      string
	ProjectID     string
	Schema        string
	UserRole      string
	PipelineStage models.PipelineStage
	Container     *models.ContainerModel
	PrepareStage  func(string) string
}

type ExecuteDynamicAPIInput struct {
	TenantID  string
	ProjectID string
	Schema    string
	APIName   string
	Body      map[string]interface{}
	Container *models.ContainerModel
}

type ExecuteWorkflowInput struct {
	TenantID     string
	ProjectID    string
	Schema       string
	WorkflowName string
	Record       map[string]interface{}
	OldRecord    map[string]interface{}
	StepOutputs  map[string]interface{}
	UserID       string
	AuditUser    *models.AuditUser
	Container    *models.ContainerModel
	Pager        *utils.Pager
}

type ExportDynamicItemsInput struct {
	TenantID  string
	ProjectID string
	Request   requests.ExportRequest
}

type ExportDynamicItemsResult struct {
	FileName string
	Content  []byte
}

type DynamicService struct {
	repository     *repositories.DynamicRepository
	parser         *requests.DynamicRequestParser
	cache          *cache.DynamicCache
	events         *events.DynamicEvents
	runTransaction func(context.Context, func(mongo.SessionContext) error) error
}

func NewDynamicService() *DynamicService {
	uploadService := files.NewUploadService()
	repository := repositories.NewDynamicRepository()
	return &DynamicService{
		repository:     repository,
		parser:         requests.NewDynamicRequestParser(uploadService),
		cache:          cache.NewDynamicCache(),
		events:         events.NewDynamicEvents(),
		runTransaction: repository.WithTransaction,
	}
}

func (s *DynamicService) ParseSearchParams(c *fiber.Ctx) (requests.SearchParams, error) {
	return s.parser.ParseSearchParams(c)
}

func (s *DynamicService) ParseFilterParams(c *fiber.Ctx, container *models.ContainerModel) (requests.FilterParams, error) {
	return s.parser.ParseFilterParams(c, container)
}

func (s *DynamicService) ParsePaginatedItemsParams(c *fiber.Ctx, container *models.ContainerModel) (requests.PaginatedItemsParams, error) {
	return s.parser.ParsePaginatedItemsParams(c, container)
}

func (s *DynamicService) ParsePipelineParams(c *fiber.Ctx) requests.PipelineParams {
	return s.parser.ParsePipelineParams(c)
}

func (s *DynamicService) ParseTestPipeline(c *fiber.Ctx) (models.TestPipelineRequestBody, error) {
	return s.parser.ParseTestPipeline(c)
}

func (s *DynamicService) ParseDynamicAPIRequest(c *fiber.Ctx) (map[string]interface{}, error) {
	return s.parser.ParseDynamicAPIRequest(c)
}

func (s *DynamicService) ParseExportRequest(c *fiber.Ctx) (requests.ExportRequest, error) {
	return s.parser.ParseExportRequest(c)
}

func (s *DynamicService) CreateDynamicItem(ctx context.Context, input CreateDynamicItemInput) (map[string]interface{}, error) {
	container, err := s.resolveContainer(ctx, input)
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

	var result *mongo.InsertOneResult
	if err := s.repository.WithTransaction(ctx, func(txCtx mongo.SessionContext) error {
		workflowPayload := workflowExecutionPayload{
			TenantID:    input.TenantID,
			ProjectID:   input.ProjectID,
			SchemaName:  input.Schema,
			Record:      itemMap,
			StepOutputs: map[string]interface{}{},
			UserID:      input.UserID,
			AuditUser:   input.User,
			Container:   container,
		}
		if err := s.runTransactionalWorkflows(txCtx, workflowPayload, models.WorkflowTriggerBeforeCreate); err != nil {
			return err
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
			return &ServiceError{
				Status:  status,
				Message: message,
				Data:    err.Error(),
				Err:     err,
			}
		}
		if err := s.applyAutoIncrementFields(txCtx, input.Schema, container, itemMap); err != nil {
			log.Printf("Failed to generate autoIncrement id for schema: %s, error: %v", input.Schema, err)
			return &ServiceError{
				Status:  http.StatusInternalServerError,
				Message: "Failed to generate autoIncrement id",
				Err:     err,
			}
		}

		var insertErr error
		result, insertErr = s.repository.Insert(txCtx, input.TenantID, input.ProjectID, input.Schema, itemMap)
		if insertErr != nil {
			return insertErr
		}

		if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
			itemMap["_id"] = oid
		}

		workflowPayload.Record = itemMap
		if err := s.runTransactionalWorkflows(txCtx, workflowPayload, models.WorkflowTriggerAfterCreate); err != nil {
			return err
		}
		if err := s.enqueueOutboxWorkflows(txCtx, workflowPayload, models.WorkflowTriggerAfterCreate); err != nil {
			return err
		}

		return s.insertDynamicPostWrite(txCtx, input.TenantID, input.ProjectID, input.Schema, models.DynamicOutboxOperationCreate, input.UserID, container,
			buildDynamicAuditLog(input.TenantID, input.ProjectID, container.SchemaName, models.DynamicOutboxOperationCreate, input.User, nil, itemMap))
	}); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, duplicateKeyServiceError(container, err)
		}
		log.Printf("Failed to save item for schema: %s, error: %v", input.Schema, err)
		return nil, workflowExecutionServiceError(err, "Failed to save item.")
	}

	utils.StripHashed(container.Fields, []map[string]interface{}{itemMap})

	return itemMap, nil
}

func (s *DynamicService) CreateMultipleDynamicItems(ctx context.Context, input CreateMultipleDynamicItemsInput) (*mongo.InsertManyResult, error) {
	container, err := s.resolveCreateMultipleContainer(ctx, input)
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

	var result *mongo.InsertManyResult
	var bulkCreatedDocs []interface{}
	if err := s.repository.WithTransaction(ctx, func(txCtx mongo.SessionContext) error {
		for _, item := range items {
			workflowPayload := workflowExecutionPayload{
				TenantID:    input.TenantID,
				ProjectID:   input.ProjectID,
				SchemaName:  input.Schema,
				Record:      item,
				StepOutputs: map[string]interface{}{},
				UserID:      input.UserID,
				AuditUser:   input.User,
				Container:   container,
			}
			if err := s.runTransactionalWorkflows(txCtx, workflowPayload, models.WorkflowTriggerBeforeCreate); err != nil {
				return err
			}
		}
		if err := validators.PrepareCreateItems(input.TenantID, input.ProjectID, container, items); err != nil {
			return err
		}
		if err := s.applyAutoIncrementFieldsForItems(txCtx, input.Schema, container, items); err != nil {
			return err
		}

		var insertErr error
		result, insertErr = s.repository.InsertMany(txCtx, input.TenantID, input.ProjectID, input.Schema, items)
		if insertErr != nil {
			return insertErr
		}

		bulkCreatedDocs = make([]interface{}, 0, len(result.InsertedIDs))
		for i, insertedID := range result.InsertedIDs {
			if oid, ok := insertedID.(primitive.ObjectID); ok && i < len(items) {
				items[i]["_id"] = oid
				bulkCreatedDocs = append(bulkCreatedDocs, items[i])
			}
		}
		for _, item := range items {
			workflowPayload := workflowExecutionPayload{
				TenantID:    input.TenantID,
				ProjectID:   input.ProjectID,
				SchemaName:  input.Schema,
				Record:      item,
				StepOutputs: map[string]interface{}{},
				UserID:      input.UserID,
				AuditUser:   input.User,
				Container:   container,
			}
			if err := s.runTransactionalWorkflows(txCtx, workflowPayload, models.WorkflowTriggerAfterCreate); err != nil {
				return err
			}
			if err := s.enqueueOutboxWorkflows(txCtx, workflowPayload, models.WorkflowTriggerAfterCreate); err != nil {
				return err
			}
		}

		if len(bulkCreatedDocs) == 0 {
			return nil
		}
		return s.insertDynamicPostWrite(txCtx, input.TenantID, input.ProjectID, input.Schema, models.DynamicOutboxOperationBulkCreate, input.UserID, container,
			buildDynamicAuditLog(input.TenantID, input.ProjectID, container.SchemaName, models.DynamicOutboxOperationBulkCreate, input.User, nil, bulkCreatedDocs))
	}); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, duplicateKeyServiceError(container, err)
		}
		var equationErr *validators.EquationItemFieldError
		if errors.As(err, &equationErr) {
			return nil, &ServiceError{
				Status:  http.StatusBadRequest,
				Message: fmt.Sprintf("Error evaluating equation for field %s in item %d", equationErr.FieldName, equationErr.Index),
				Data:    err.Error(),
				Err:     err,
			}
		}
		log.Printf("Failed to insert multiple items for schema: %s, error: %v", input.Schema, err)
		return nil, workflowExecutionServiceError(err, "Failed to insert multiple items.")
	}
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

	container, err := s.resolveUpdateContainer(ctx, input)
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

	var updateResult *mongo.UpdateResult
	if err := s.repository.WithTransaction(ctx, func(txCtx mongo.SessionContext) error {
		workflowPayload := workflowExecutionPayload{
			TenantID:    input.TenantID,
			ProjectID:   input.ProjectID,
			SchemaName:  input.Schema,
			Record:      existingItem,
			OldRecord:   beforeDoc,
			StepOutputs: map[string]interface{}{},
			UserID:      input.UserID,
			AuditUser:   input.User,
			Container:   container,
		}
		if err := s.runTransactionalWorkflows(txCtx, workflowPayload, models.WorkflowTriggerBeforeUpdate); err != nil {
			return err
		}

		var updateErr error
		updateResult, updateErr = s.repository.UpdateByID(txCtx, input.TenantID, input.ProjectID, input.Schema, updateID, existingItem)
		if updateErr != nil {
			return updateErr
		}
		if updateResult.MatchedCount == 0 {
			return nil
		}
		if err := s.runTransactionalWorkflows(txCtx, workflowPayload, models.WorkflowTriggerAfterUpdate); err != nil {
			return err
		}
		if err := s.enqueueOutboxWorkflows(txCtx, workflowPayload, models.WorkflowTriggerAfterUpdate); err != nil {
			return err
		}
		return s.insertDynamicPostWrite(txCtx, input.TenantID, input.ProjectID, input.Schema, models.DynamicOutboxOperationUpdate, input.UserID, container,
			buildDynamicAuditLog(input.TenantID, input.ProjectID, container.SchemaName, models.DynamicOutboxOperationUpdate, input.User, beforeDoc, existingItem))
	}); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, duplicateKeyServiceError(container, err)
		}
		log.Printf("Failed to update item for schema: %s, error: %v", input.Schema, err)
		return nil, workflowExecutionServiceError(err, "Failed to update item")
	}

	if updateResult == nil || updateResult.MatchedCount == 0 {
		log.Printf("No item found with specified ID for schema: %s", input.Schema)
		return nil, &ServiceError{
			Status:  http.StatusNotFound,
			Message: "No item found with specified ID",
			Data:    nil,
		}
	}

	responseItem := make(map[string]interface{})
	for k, v := range existingItem {
		responseItem[k] = v
	}
	utils.StripHashed(container.Fields, []map[string]interface{}{responseItem})

	return responseItem, nil
}

func (s *DynamicService) UpdateMultipleDynamicItems(ctx context.Context, input UpdateMultipleDynamicItemsInput) (fiber.Map, error) {
	container, err := s.resolveUpdateMultipleContainer(ctx, input)
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

	if err := s.repository.WithTransaction(ctx, func(txCtx mongo.SessionContext) error {
		localSuccessfulUpdates := make([]interface{}, 0, len(items))
		localFailedUpdates := make([]map[string]interface{}, 0)
		localBulkBeforeDocs := make([]interface{}, 0, len(items))
		localBulkAfterDocs := make([]interface{}, 0, len(items))

		for _, item := range items {
			result, beforeDoc, afterDoc, workflowErr := s.updateOneFromBulk(txCtx, input, container, item)
			if workflowErr != nil {
				return workflowErr
			}
			if result.Failed != nil {
				localFailedUpdates = append(localFailedUpdates, result.Failed)
				continue
			}
			localSuccessfulUpdates = append(localSuccessfulUpdates, result.Success)
			localBulkBeforeDocs = append(localBulkBeforeDocs, beforeDoc)
			localBulkAfterDocs = append(localBulkAfterDocs, afterDoc)
		}

		if len(localSuccessfulUpdates) == 0 {
			failedUpdates = localFailedUpdates
			return nil
		}
		if err := s.insertDynamicPostWrite(txCtx, input.TenantID, input.ProjectID, input.Schema, models.DynamicOutboxOperationBulkUpdate, input.UserID, container,
			buildDynamicAuditLog(input.TenantID, input.ProjectID, container.SchemaName, models.DynamicOutboxOperationBulkUpdate, input.User, localBulkBeforeDocs, localBulkAfterDocs)); err != nil {
			return err
		}

		successfulUpdates = localSuccessfulUpdates
		failedUpdates = localFailedUpdates
		return nil
	}); err != nil {
		log.Printf("Failed to update multiple items for schema: %s, error: %v", input.Schema, err)
		return nil, workflowExecutionServiceError(err, "Failed to update multiple items")
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

	allContainers, err := s.repository.GetAllContainerModels(ctx)
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

	container, err := s.resolveDeleteContainer(ctx, input)
	if err != nil {
		log.Printf("Failed to fetch container model for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch container model",
			Err:     err,
		}
	}

	var deletedDoc bson.M
	if err := s.repository.WithTransaction(ctx, func(txCtx mongo.SessionContext) error {
		if err := s.forceDeleteReferences(txCtx, input.TenantID, input.ProjectID, input.Schema, deleteID, allContainers, true); err != nil {
			return err
		}

		var findErr error
		deletedDoc, findErr = s.repository.FindByID(txCtx, input.TenantID, input.ProjectID, input.Schema, deleteID)
		if findErr != nil {
			log.Printf("Failed to fetch item before delete for schema: %s, error: %v", input.Schema, findErr)
		}

		var workflowRecord map[string]interface{}
		if deletedDoc != nil {
			workflowRecord = map[string]interface{}{}
			for k, v := range deletedDoc {
				workflowRecord[k] = v
			}
		}
		workflowPayload := workflowExecutionPayload{
			TenantID:    input.TenantID,
			ProjectID:   input.ProjectID,
			SchemaName:  input.Schema,
			Record:      workflowRecord,
			OldRecord:   workflowRecord,
			StepOutputs: map[string]interface{}{},
			UserID:      input.UserID,
			AuditUser:   input.User,
			Container:   container,
		}
		if err := s.runTransactionalWorkflows(txCtx, workflowPayload, models.WorkflowTriggerBeforeDelete); err != nil {
			return err
		}

		if _, err := s.repository.DeleteByID(txCtx, input.TenantID, input.ProjectID, input.Schema, deleteID); err != nil {
			return err
		}

		if err := s.runTransactionalWorkflows(txCtx, workflowPayload, models.WorkflowTriggerAfterDelete); err != nil {
			return err
		}
		if err := s.enqueueOutboxWorkflows(txCtx, workflowPayload, models.WorkflowTriggerAfterDelete); err != nil {
			return err
		}

		var auditLog *models.AuditLog
		if deletedDoc != nil {
			auditLog = buildDynamicAuditLog(input.TenantID, input.ProjectID, container.SchemaName, models.DynamicOutboxOperationDelete, input.User, deletedDoc, nil)
		}
		return s.insertDynamicPostWrite(txCtx, input.TenantID, input.ProjectID, input.Schema, models.DynamicOutboxOperationDelete, input.UserID, container, auditLog)
	}); err != nil {
		log.Printf("Failed to delete the item from the specified collection for schema: %s, error: %v", input.Schema, err)
		return nil, workflowExecutionServiceError(err, "Failed to delete the item from the specified collection.")
	}

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

	allContainers, err := s.repository.GetAllContainerModels(ctx)
	if err != nil {
		log.Printf("Failed to retrieve container models for schema: %s, error: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve container models",
			Err:     err,
		}
	}

	container, err := s.resolveDeleteMultipleContainer(ctx, input)
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

	if err := s.repository.WithTransaction(ctx, func(txCtx mongo.SessionContext) error {
		localSuccessfulDeletes := make([]interface{}, 0, len(items))
		localFailedDeletes := make([]map[string]interface{}, 0)
		localBulkDeletedDocs := make([]interface{}, 0, len(items))

		for _, item := range items {
			result, deletedDoc, workflowErr := s.deleteOneFromBulk(txCtx, input, container, allContainers, item)
			if workflowErr != nil {
				return workflowErr
			}
			if result.Failed != nil {
				localFailedDeletes = append(localFailedDeletes, result.Failed)
				continue
			}
			localSuccessfulDeletes = append(localSuccessfulDeletes, result.Success)
			if deletedDoc != nil {
				localBulkDeletedDocs = append(localBulkDeletedDocs, deletedDoc)
			}
		}

		if len(localSuccessfulDeletes) == 0 {
			failedDeletes = localFailedDeletes
			return nil
		}
		if err := s.insertDynamicPostWrite(txCtx, input.TenantID, input.ProjectID, input.Schema, models.DynamicOutboxOperationBulkDelete, input.UserID, container,
			buildDynamicAuditLog(input.TenantID, input.ProjectID, container.SchemaName, models.DynamicOutboxOperationBulkDelete, input.User, localBulkDeletedDocs, nil)); err != nil {
			return err
		}

		successfulDeletes = localSuccessfulDeletes
		failedDeletes = localFailedDeletes
		return nil
	}); err != nil {
		log.Printf("Failed to delete multiple items for schema: %s, error: %v", input.Schema, err)
		return nil, workflowExecutionServiceError(err, "Failed to delete multiple items")
	}

	return fiber.Map{
		"successful": successfulDeletes,
		"failed":     failedDeletes,
	}, nil
}

func (s *DynamicService) GetAllDynamicItems(ctx context.Context, input GetAllDynamicItemsInput) ([]map[string]interface{}, error) {
	container, err := s.resolveGetAllContainer(ctx, input)
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
	_, shouldCache := utils.GenerateRedisKey("GetAllDynamicModelItems", input.TenantID, input.ProjectID, schema, container)
	redisKey, shouldCache := schemaCacheKey(ctx, input.TenantID, input.ProjectID, schema, shouldCache, "GetAllDynamicModelItems", "all")
	if shouldCache {
		if items, ok := s.cache.GetItems(ctx, redisKey); ok {
			log.Printf("Fetched items from cache for schema: %s", schema)
			if maxUnboundedRead := configs.GetMaxUnboundedReadLimit(); len(items) > maxUnboundedRead {
				log.Printf("Unbounded read limit exceeded from cache for schema=%s tenant=%s project=%s requested=%d max=%d", schema, input.TenantID, input.ProjectID, len(items), maxUnboundedRead)
				items = items[:maxUnboundedRead]
			}
			return utils.FilterDocuments(items, container.Fields, input.UserRole), nil
		}

		lockID, locked := utils.AcquireCacheFillLock(ctx, redisKey)
		if locked {
			defer utils.ReleaseCacheFillLock(ctx, redisKey, lockID)
		} else {
			var cachedItems []map[string]interface{}
			if utils.WaitForCacheFill(ctx, func() bool {
				var ok bool
				cachedItems, ok = s.cache.GetItems(ctx, redisKey)
				return ok
			}) {
				log.Printf("Fetched items from cache after wait for schema: %s", schema)
				if maxUnboundedRead := configs.GetMaxUnboundedReadLimit(); len(cachedItems) > maxUnboundedRead {
					cachedItems = cachedItems[:maxUnboundedRead]
				}
				return utils.FilterDocuments(cachedItems, container.Fields, input.UserRole), nil
			}
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

func validateSelectionFields(container *models.ContainerModel, fieldNames []string, userRole string) error {
	requestedFields := map[string]struct{}{}
	for _, fieldName := range fieldNames {
		if fieldName == "" || fieldName == "_id" {
			continue
		}
		requestedFields[fieldName] = struct{}{}
	}

	for _, field := range container.Fields {
		if _, ok := requestedFields[field.Name]; !ok {
			continue
		}

		if len(field.AuthorizeRole) > 0 {
			authorized := false
			for _, role := range field.AuthorizeRole {
				if role == userRole {
					authorized = true
					break
				}
			}
			if !authorized {
				return &ServiceError{
					Status:  http.StatusForbidden,
					Message: "Access to this field is restricted",
					Data:    nil,
				}
			}
		}

		if field.IsHashed {
			log.Printf("Attempted to access hashed field %s in schema %s", field.Name, container.SchemaName)
			return &ServiceError{
				Status:  http.StatusForbidden,
				Message: "Cannot access hashed fields",
				Data:    nil,
			}
		}
	}

	return nil
}

func (s *DynamicService) GetItemsForSelection(ctx context.Context, input GetItemsForSelectionInput) ([]map[string]interface{}, error) {
	if input.Schema == "" || input.FieldName == "" {
		return nil, &ServiceError{
			Status:  http.StatusBadRequest,
			Message: "schemaName and fieldName are required",
			Data:    nil,
		}
	}

	container, err := s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
	if err != nil {
		log.Printf("Failed to get container model for schema %s: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to load schema configuration",
			Err:     err,
		}
	}

	requestedFields := map[string]struct{}{
		input.FieldName: {},
	}
	if input.ValueField != "" && input.ValueField != "_id" {
		requestedFields[input.ValueField] = struct{}{}
	}

	for _, field := range container.Fields {
		if _, ok := requestedFields[field.Name]; !ok {
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
			log.Printf("Attempted to access hashed field %s in schema %s", field.Name, input.Schema)
			return nil, &ServiceError{
				Status:  http.StatusForbidden,
				Message: "Cannot access hashed fields",
				Data:    nil,
			}
		}
	}

	items, err := s.repository.FindForSelection(ctx, input.TenantID, input.ProjectID, input.Schema, input.FieldName, input.ValueField)
	if err != nil {
		log.Printf("Failed to query collection %s: %v", input.Schema, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch items",
			Err:     err,
		}
	}

	return utils.FilterDocuments(items, container.Fields, input.UserRole), nil
}

func (s *DynamicService) GetDynamicItem(ctx context.Context, input GetDynamicItemInput) (GetDynamicItemResult, error) {
	container, err := s.resolveGetDynamicContainer(ctx, input)
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

	_, shouldCache := utils.GenerateRedisKey("GetDynamicModelItem", input.TenantID, input.ProjectID, container.SchemaName, container, input.ID)
	redisKey, shouldCache := schemaCacheKey(ctx, input.TenantID, input.ProjectID, container.SchemaName, shouldCache, "GetDynamicModelItem", input.ID)
	if shouldCache {
		if item, ok := s.cache.GetItem(ctx, redisKey); ok {
			utils.StripHashed(container.Fields, []map[string]interface{}{item})
			populated, _ := utils.PopulateIfNeeded(ctx, input.TenantID, input.ProjectID, container, []map[string]interface{}{item})
			if len(populated) > 0 {
				item = populated[0]
			}
			return GetDynamicItemResult{Item: item, FromCache: true}, nil
		}

		lockID, locked := utils.AcquireCacheFillLock(ctx, redisKey)
		if locked {
			defer utils.ReleaseCacheFillLock(ctx, redisKey, lockID)
		} else {
			var cachedItem map[string]interface{}
			if utils.WaitForCacheFill(ctx, func() bool {
				var ok bool
				cachedItem, ok = s.cache.GetItem(ctx, redisKey)
				return ok
			}) {
				utils.StripHashed(container.Fields, []map[string]interface{}{cachedItem})
				populated, _ := utils.PopulateIfNeeded(ctx, input.TenantID, input.ProjectID, container, []map[string]interface{}{cachedItem})
				if len(populated) > 0 {
					cachedItem = populated[0]
				}
				return GetDynamicItemResult{Item: cachedItem, FromCache: true}, nil
			}
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
	container, err := s.resolveSearchContainer(ctx, input)
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

func (s *DynamicService) FilterDynamicItems(ctx context.Context, input FilterDynamicItemsInput) (interface{}, error) {
	container, err := s.resolveFilterContainer(ctx, input)
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

	filter := input.Filter
	if input.SearchKey != "" {
		orClauses, err := utils.BuildSearchWithReferences(ctx, container, input.SearchKey)
		if err != nil {
			return nil, &ServiceError{
				Status:  http.StatusInternalServerError,
				Message: "Failed to build search filter",
				Err:     err,
			}
		}

		if len(orClauses) > 0 {
			if len(filter) > 0 {
				filter = bson.M{
					"$and": []bson.M{
						filter,
						{"$or": orClauses},
					},
				}
			} else {
				filter = bson.M{"$or": orClauses}
			}
		}
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
	items, err := s.repository.Query(ctx, input.TenantID, input.ProjectID, container.SchemaName, filter, opts, &pager)
	if err != nil {
		log.Printf("Filter query failed for schema %q: %v", container.SchemaName, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch filtered items",
			Err:     err,
		}
	}

	utils.StripHashed(container.Fields, items)

	items, err = utils.PopulateIfNeeded(ctx, input.TenantID, input.ProjectID, container, items)
	if err != nil {
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to populate items",
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

func (s *DynamicService) GetAllDynamicItemsWithPagination(ctx context.Context, input GetPaginatedDynamicItemsInput) (interface{}, error) {
	container, err := s.resolvePaginatedContainer(ctx, input)
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

	hasRowAccess := container.RowAccess != nil && len(container.RowAccess.Conditions) > 0
	cacheQuery := input.QueryString
	if hasRowAccess {
		cacheQuery += "&_uid=" + input.UserID
	}

	_, shouldCache := utils.GenerateRedisKey("GetAllDynamicModelItemsWithPagination", input.TenantID, input.ProjectID, container.SchemaName, container, cacheQuery)
	redisKey, shouldCache := schemaCacheKey(ctx, input.TenantID, input.ProjectID, container.SchemaName, shouldCache, "GetAllDynamicModelItemsWithPagination", cacheQuery)
	if shouldCache {
		if response, ok := s.cache.GetResponse(ctx, redisKey); ok {
			log.Printf("Fetched paginated items from cache for schema: %s", container.SchemaName)
			if items, ok := cachedResponseItems(response); ok {
				response["items"] = utils.FilterDocuments(items, container.Fields, input.UserRole)
			}
			return response, nil
		}

		lockID, locked := utils.AcquireCacheFillLock(ctx, redisKey)
		if locked {
			defer utils.ReleaseCacheFillLock(ctx, redisKey, lockID)
		} else {
			var cachedResponse fiber.Map
			if utils.WaitForCacheFill(ctx, func() bool {
				var ok bool
				cachedResponse, ok = s.cache.GetResponse(ctx, redisKey)
				return ok
			}) {
				log.Printf("Fetched paginated items from cache after wait for schema: %s", container.SchemaName)
				if items, ok := cachedResponseItems(cachedResponse); ok {
					cachedResponse["items"] = utils.FilterDocuments(items, container.Fields, input.UserRole)
				}
				return cachedResponse, nil
			}
		}
	}

	filter := input.Filter
	pager := input.Pager
	if input.SearchKey != "" {
		orClauses, err := utils.BuildSearchWithReferences(ctx, container, input.SearchKey)
		if err != nil {
			return nil, &ServiceError{
				Status:  http.StatusInternalServerError,
				Message: "Failed to build search filter",
				Err:     err,
			}
		}

		if len(orClauses) > 0 {
			if len(filter) > 0 {
				filter = bson.M{
					"$and": []bson.M{
						filter,
						{"$or": orClauses},
					},
				}
			} else {
				filter = bson.M{"$or": orClauses}
			}
		} else if pager.Enabled {
			return fiber.Map{
				"items":       []interface{}{},
				"totalItems":  0,
				"totalPages":  0,
				"currentPage": pager.Page,
			}, nil
		} else {
			return []interface{}{}, nil
		}
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

	opts := utils.BuildFindOptions(input.Sort, pager)
	items, err := s.repository.Query(ctx, input.TenantID, input.ProjectID, container.SchemaName, filter, opts, &pager)
	if err != nil {
		log.Printf("Query failed for schema %q: %v", container.SchemaName, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch items",
			Err:     err,
		}
	}

	utils.StripHashed(container.Fields, items)

	items, err = utils.PopulateIfNeeded(ctx, input.TenantID, input.ProjectID, container, items)
	if err != nil {
		log.Printf("Population failed for schema %q: %v", container.SchemaName, err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to populate items",
			Err:     err,
		}
	}

	items = utils.FilterDocuments(items, container.Fields, input.UserRole)
	response := fiber.Map{"items": items}
	if pager.Enabled {
		response = fiber.Map{
			"items":       items,
			"totalItems":  pager.TotalItems,
			"totalPages":  pager.TotalPages,
			"currentPage": pager.Page,
		}
	}

	if shouldCache {
		s.cache.SetResponse(ctx, redisKey, response, container.Redis.CacheTime)
		log.Printf("Cached paginated items for schema: %s", container.SchemaName)
	}

	if pager.Enabled {
		return response, nil
	}
	return items, nil
}

func (s *DynamicService) GetPipeline(ctx context.Context, input GetPipelineInput) ([]map[string]interface{}, error) {
	start := time.Now()
	status := "success"
	defer func() {
		duration := time.Since(start)
		observability.RecordPipelineExecution(input.PipelineName, input.Schema, status, duration)
		attrs := append(observability.PipelineAttrs(input.TenantID, input.ProjectID, input.Schema, input.PipelineName),
			observability.OperationAttrs("pipeline_execute", status, duration)...)
		observability.InfoCtx(ctx, "pipeline execution completed", attrs...)
	}()

	if input.Schema == "" || input.PipelineName == "" {
		status = "error"
		return nil, &ServiceError{
			Status:  http.StatusBadRequest,
			Message: "schemaName and pipelineName are required",
			Data:    nil,
		}
	}

	container, err := s.resolvePipelineContainer(ctx, input)
	if err != nil {
		status = "error"
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch container model",
			Err:     err,
		}
	}

	pipelineStage, found := findPipelineStage(container, input.PipelineName)
	if !found {
		status = "error"
		return nil, &ServiceError{
			Status:  http.StatusNotFound,
			Message: "Pipeline not found",
			Data:    nil,
		}
	}

	if input.PrepareStage != nil {
		pipelineStage.PipelineJSON = input.PrepareStage(pipelineStage.PipelineJSON)
	}

	_, shouldCache := utils.GeneratePipelineRedisKey(input.TenantID, input.ProjectID, input.Schema, input.PipelineName, container)
	redisKey, shouldCache := schemaCacheKey(ctx, input.TenantID, input.ProjectID, input.Schema, shouldCache, "pipeline_"+input.PipelineName, input.CurrentQuery)
	if shouldCache {
		if items, ok := s.cache.GetPipelineItems(ctx, redisKey, input.CurrentQuery); ok {
			status = "cache_hit"
			return items, nil
		}

		lockID, locked := utils.AcquireCacheFillLock(ctx, redisKey)
		if locked {
			defer utils.ReleaseCacheFillLock(ctx, redisKey, lockID)
		} else {
			var cachedItems []map[string]interface{}
			if utils.WaitForCacheFill(ctx, func() bool {
				var ok bool
				cachedItems, ok = s.cache.GetPipelineItems(ctx, redisKey, input.CurrentQuery)
				return ok
			}) {
				status = "cache_hit"
				return cachedItems, nil
			}
		}
	}

	resultItems, err := s.repository.ExecutePipeline(ctx, input.TenantID, input.ProjectID, input.Schema, pipelineStage)
	if err != nil {
		status = "error"
		log.Printf("Error executing dynamic pipeline %q for schema %q: %v; pipelineJson=%s", input.PipelineName, input.Schema, err, pipelineStage.PipelineJSON)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to execute dynamic pipeline",
			Err:     err,
		}
	}

	if shouldCache {
		s.cache.SetPipelineItems(ctx, redisKey, input.CurrentQuery, resultItems, pipelineStage.CacheTime)
	}

	return resultItems, nil
}

func (s *DynamicService) GetTableSource(ctx context.Context, input GetTableSourceInput) (fiber.Map, error) {
	sourceType := strings.TrimSpace(input.SourceType)
	if sourceType == "" {
		sourceType = string(models.BindingKindSchema)
	}

	switch models.BindingKind(sourceType) {
	case models.BindingKindSchema:
		result, err := s.GetAllDynamicItemsWithPagination(ctx, GetPaginatedDynamicItemsInput{
			TenantID:    input.TenantID,
			ProjectID:   input.ProjectID,
			Schema:      input.Schema,
			QueryString: input.QueryString,
			Filter:      input.Filter,
			SearchKey:   input.SearchKey,
			UserID:      input.UserID,
			UserRole:    input.UserRole,
			Sort:        input.Sort,
			Pager:       input.Pager,
			Container:   input.Container,
		})
		if err != nil {
			return nil, err
		}
		return projectTableSourceResponse(normalizeTableSourceResponse(result, input.Pager), input.Fields), nil
	case models.BindingKindPipeline:
		items, err := s.GetPipeline(ctx, GetPipelineInput{
			TenantID:     input.TenantID,
			ProjectID:    input.ProjectID,
			Schema:       input.Schema,
			PipelineName: input.PipelineName,
			CurrentQuery: input.QueryString,
			Container:    input.Container,
			PrepareStage: input.PrepareStage,
		})
		if err != nil {
			return nil, err
		}
		items = projectTableSourceItems(items, input.Fields)
		return paginateTableSourceItems(items, input.Pager), nil
	case models.BindingKindWorkflow:
		result, err := s.ExecuteWorkflow(ctx, ExecuteWorkflowInput{
			TenantID:     input.TenantID,
			ProjectID:    input.ProjectID,
			Schema:       input.Schema,
			WorkflowName: input.WorkflowName,
			Record:       input.Params,
			StepOutputs:  map[string]interface{}{},
			UserID:       input.UserID,
			AuditUser:    input.AuditUser,
			Container:    input.Container,
			Pager:        &input.Pager,
		})
		if err != nil {
			return nil, err
		}
		items, ok := tableSourceRows(result.Data)
		if !ok {
			return nil, &ServiceError{
				Status:  http.StatusBadRequest,
				Message: "Workflow table source must return an array of objects",
				Data:    nil,
			}
		}
		items = projectTableSourceItems(items, input.Fields)
		return workflowTableSourceResponse(items, input.Pager, result.pagination), nil
	default:
		return nil, &ServiceError{
			Status:  http.StatusBadRequest,
			Message: "sourceType must be one of schema, pipeline, workflow",
			Data:    nil,
		}
	}
}

func projectTableSourceResponse(response fiber.Map, fields []string) fiber.Map {
	if len(fields) == 0 {
		return response
	}
	if items, ok := tableSourceRows(response["items"]); ok {
		response["items"] = projectTableSourceItems(items, fields)
	}
	return response
}

func projectTableSourceItems(items []map[string]interface{}, fields []string) []map[string]interface{} {
	if len(fields) == 0 {
		return items
	}

	allowed := make(map[string]struct{}, len(fields)+1)
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		allowed[field] = struct{}{}
	}
	if len(allowed) == 0 {
		return items
	}
	allowed["_id"] = struct{}{}

	projected := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		next := make(map[string]interface{}, len(allowed))
		for key, value := range item {
			if _, ok := allowed[key]; ok {
				next[key] = value
			}
		}
		projected = append(projected, next)
	}
	return projected
}

func normalizeTableSourceResponse(result interface{}, pager utils.Pager) fiber.Map {
	switch response := result.(type) {
	case fiber.Map:
		return response
	case map[string]interface{}:
		return fiber.Map(response)
	default:
		items, _ := tableSourceRows(result)
		return paginateTableSourceItems(items, pager)
	}
}

func paginateTableSourceItems(items []map[string]interface{}, pager utils.Pager) fiber.Map {
	totalItems := int64(len(items))
	totalPages := 1
	currentPage := 1

	if pager.Enabled {
		currentPage = pager.Page
		if pager.Limit > 0 {
			totalPages = int((totalItems + int64(pager.Limit) - 1) / int64(pager.Limit))
		}
		start := int(pager.Skip)
		if start > len(items) {
			start = len(items)
		}
		end := start + pager.Limit
		if end > len(items) {
			end = len(items)
		}
		items = items[start:end]
	}

	if totalItems == 0 {
		totalPages = 0
	}

	return fiber.Map{
		"items":       items,
		"totalItems":  totalItems,
		"totalPages":  totalPages,
		"currentPage": currentPage,
	}
}

func workflowTableSourceResponse(items []map[string]interface{}, pager utils.Pager, pagination *workflowTableSourcePagination) fiber.Map {
	if pagination == nil || !pagination.Applied {
		return paginateTableSourceItems(items, pager)
	}
	return fiber.Map{
		"items":       items,
		"totalItems":  pagination.Pager.TotalItems,
		"totalPages":  pagination.Pager.TotalPages,
		"currentPage": pagination.Pager.Page,
	}
}

func tableSourceRows(value interface{}) ([]map[string]interface{}, bool) {
	switch rows := value.(type) {
	case nil:
		return []map[string]interface{}{}, true
	case fiber.Map:
		return tableSourceRows(rows["items"])
	case map[string]interface{}:
		return tableSourceRows(rows["items"])
	case bson.M:
		return tableSourceRows(rows["items"])
	case []map[string]interface{}:
		return rows, true
	case []bson.M:
		items := make([]map[string]interface{}, 0, len(rows))
		for _, row := range rows {
			items = append(items, map[string]interface{}(row))
		}
		return items, true
	case []interface{}:
		items := make([]map[string]interface{}, 0, len(rows))
		for _, row := range rows {
			item, ok := tableSourceRow(row)
			if !ok {
				return nil, false
			}
			items = append(items, item)
		}
		return items, true
	default:
		return nil, false
	}
}

func tableSourceRow(value interface{}) (map[string]interface{}, bool) {
	switch row := value.(type) {
	case map[string]interface{}:
		return row, true
	case bson.M:
		return map[string]interface{}(row), true
	default:
		return nil, false
	}
}

func (s *DynamicService) ExecuteDynamicCode(ctx context.Context, input ExecuteDynamicCodeInput) (DynamicExecutionResult, error) {
	container, err := s.resolveDynamicCodeContainer(ctx, input)
	if err != nil {
		return DynamicExecutionResult{}, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch container model",
			Err:     err,
		}
	}

	pluginFileName := "temp_" + input.FunctionName + ".so"
	fileName := "temp_" + input.FunctionName + ".go"
	_, shouldCache := utils.GenerateDynamicFunctionRedisKey(input.TenantID, input.ProjectID, input.Schema, input.FunctionName, container)
	redisKey, shouldCache := schemaCacheKey(ctx, input.TenantID, input.ProjectID, input.Schema, shouldCache, "function_"+input.FunctionName, input.CurrentQuery)

	if shouldCache {
		if result, ok := s.cache.GetValue(ctx, redisKey); ok {
			return DynamicExecutionResult{
				Message: "Function result fetched from cache",
				Data:    result,
				Source:  "cache",
			}, nil
		}

		lockID, locked := utils.AcquireCacheFillLock(ctx, redisKey)
		if locked {
			defer utils.ReleaseCacheFillLock(ctx, redisKey, lockID)
		} else {
			var cachedResult interface{}
			if utils.WaitForCacheFill(ctx, func() bool {
				var ok bool
				cachedResult, ok = s.cache.GetValue(ctx, redisKey)
				return ok
			}) {
				return DynamicExecutionResult{
					Message: "Function result fetched from cache",
					Data:    cachedResult,
					Source:  "cache",
				}, nil
			}
		}
	}

	if result, ok, err := s.executePluginFunction(pluginFileName, input.FunctionName, input.FiberCtx, false); ok {
		if err != nil {
			return DynamicExecutionResult{}, &ServiceError{Status: http.StatusInternalServerError, Message: "Failed to execute function", Err: err}
		}
		s.cacheDynamicFunctionResult(ctx, redisKey, input.CurrentQuery, result, container, shouldCache)
		return DynamicExecutionResult{Message: "Function result fetched from plugin", Data: result, Source: "plugin"}, nil
	}

	dynamicFuncCode, found := findDynamicFunctionCode(container, input.FunctionName)
	if !found {
		return DynamicExecutionResult{}, &ServiceError{
			Status:  http.StatusBadRequest,
			Message: "Function not found",
			Data:    nil,
		}
	}

	if err := os.WriteFile(fileName, []byte(dynamicFuncCode), 0644); err != nil {
		return DynamicExecutionResult{}, &ServiceError{Status: http.StatusInternalServerError, Message: "Failed to write code to file", Err: err}
	}

	out, err := exec.Command("go", "build", "-buildmode=plugin", fileName).CombinedOutput()
	if err != nil {
		return DynamicExecutionResult{}, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to compile code into plugin",
			Data:    string(out),
			Err:     err,
		}
	}

	result, ok, err := s.executePluginFunction(pluginFileName, input.FunctionName, input.FiberCtx, true)
	if err != nil {
		return DynamicExecutionResult{}, &ServiceError{Status: http.StatusInternalServerError, Message: pluginExecutionMessage(err), Err: err}
	}
	if !ok {
		return DynamicExecutionResult{}, &ServiceError{Status: http.StatusInternalServerError, Message: "Failed to execute function", Err: errors.New("function has incompatible signature")}
	}

	s.cacheDynamicFunctionResult(ctx, redisKey, input.CurrentQuery, result, container, shouldCache)
	return DynamicExecutionResult{Message: "Function result fetched from new plugin", Data: result, Source: "new plugin"}, nil
}

func (s *DynamicService) TestPipeline(ctx context.Context, input TestPipelineInput) ([]map[string]interface{}, error) {
	container, err := s.resolveTestPipelineContainer(ctx, input)
	if err != nil {
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch container model",
			Err:     err,
		}
	}

	stage := input.PipelineStage
	if input.PrepareStage != nil {
		stage.PipelineJSON = input.PrepareStage(stage.PipelineJSON)
	}

	resultItems, err := s.repository.ExecutePipeline(ctx, input.TenantID, input.ProjectID, input.Schema, stage)
	if err != nil {
		log.Printf("Error executing test pipeline: %v", err)
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to execute test pipeline",
			Err:     err,
		}
	}

	return utils.FilterDocuments(resultItems, container.Fields, input.UserRole), nil
}

func (s *DynamicService) ExecuteDynamicAPI(ctx context.Context, input ExecuteDynamicAPIInput) (DynamicExecutionResult, error) {
	container, err := s.resolveDynamicAPIContainer(ctx, input)
	if err != nil {
		return DynamicExecutionResult{}, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch container model",
			Err:     err,
		}
	}

	dynamicAPI, found := findDynamicAPI(container, input.APIName)
	if !found {
		return DynamicExecutionResult{}, &ServiceError{
			Status:  http.StatusBadRequest,
			Message: "API not found",
			Data:    nil,
		}
	}

	if missing := missingDynamicAPIDependencies(dynamicAPI, input.Body); len(missing) > 0 {
		return DynamicExecutionResult{}, &ServiceError{
			Status:  http.StatusBadRequest,
			Message: "Missing dependencies",
			Data:    missing,
		}
	}

	apiCacheQuery := cacheValueFingerprint(input.Body)
	_, shouldCache := utils.GenerateDynamicApiRedisKey(input.TenantID, input.ProjectID, input.Schema, input.APIName, container)
	redisKey, shouldCache := schemaCacheKey(ctx, input.TenantID, input.ProjectID, input.Schema, shouldCache, "api_"+input.APIName, apiCacheQuery)
	if shouldCache {
		if result, ok := s.cache.GetValue(ctx, redisKey); ok {
			return DynamicExecutionResult{Message: "API result fetched from cache", Data: result, Source: "cache"}, nil
		}

		lockID, locked := utils.AcquireCacheFillLock(ctx, redisKey)
		if locked {
			defer utils.ReleaseCacheFillLock(ctx, redisKey, lockID)
		} else {
			var cachedResult interface{}
			if utils.WaitForCacheFill(ctx, func() bool {
				var ok bool
				cachedResult, ok = s.cache.GetValue(ctx, redisKey)
				return ok
			}) {
				return DynamicExecutionResult{Message: "API result fetched from cache", Data: cachedResult, Source: "cache"}, nil
			}
		}
	}

	apiResultBytes, err := utils.ExecuteApiRequest(ctx, dynamicAPI.Method, dynamicAPI.Url, input.Body)
	if err != nil {
		return DynamicExecutionResult{}, &ServiceError{Status: http.StatusInternalServerError, Message: "Failed to execute API call", Err: err}
	}

	var apiResult interface{}
	if err := json.Unmarshal(apiResultBytes, &apiResult); err != nil {
		return DynamicExecutionResult{}, &ServiceError{Status: http.StatusInternalServerError, Message: "Failed to unmarshal API response", Err: err}
	}

	if shouldCache {
		s.cache.SetValue(ctx, redisKey, apiResult, cacheDuration(dynamicAPI.CacheTime))
	}

	return DynamicExecutionResult{Message: "API result fetched", Data: apiResult, Source: "API call"}, nil
}

func (s *DynamicService) ExecuteWorkflow(ctx context.Context, input ExecuteWorkflowInput) (DynamicExecutionResult, error) {
	container, err := s.resolveWorkflowContainer(ctx, input)
	if err != nil {
		return DynamicExecutionResult{}, &ServiceError{Status: http.StatusInternalServerError, Message: "Failed to fetch container model", Err: err}
	}

	workflow, found := findWorkflow(container, input.WorkflowName)
	if !found {
		return DynamicExecutionResult{}, &ServiceError{Status: http.StatusNotFound, Message: "Workflow not found", Data: nil}
	}
	if !workflow.IsActive {
		return DynamicExecutionResult{}, &ServiceError{Status: http.StatusForbidden, Message: "Workflow is disabled", Data: nil}
	}

	var pagination *workflowTableSourcePagination
	if input.Pager != nil && input.Pager.Enabled {
		pagination = &workflowTableSourcePagination{Pager: *input.Pager}
	}
	payload := workflowExecutionPayload{
		TenantID:     input.TenantID,
		ProjectID:    input.ProjectID,
		SchemaName:   input.Schema,
		WorkflowName: workflow.Name,
		Record:       cloneWorkflowMap(input.Record),
		OldRecord:    cloneWorkflowMap(input.OldRecord),
		StepOutputs:  cloneWorkflowMap(input.StepOutputs),
		UserID:       input.UserID,
		AuditUser:    input.AuditUser,
		Container:    container,
		Pagination:   pagination,
	}
	if payload.Record == nil {
		payload.Record = map[string]interface{}{}
	}
	if payload.OldRecord == nil {
		payload.OldRecord = map[string]interface{}{}
	}
	if payload.StepOutputs == nil {
		payload.StepOutputs = map[string]interface{}{}
	}

	runWorkflow := func(workflowCtx context.Context) error {
		return s.runWorkflowDefinition(workflowCtx, &payload, workflow)
	}
	if workflowRequiresTransaction(workflow) {
		err = s.runTransaction(ctx, func(txCtx mongo.SessionContext) error {
			return runWorkflow(txCtx)
		})
	} else {
		err = runWorkflow(ctx)
	}
	if err != nil {
		return DynamicExecutionResult{}, workflowExecutionServiceError(err, "Failed to execute workflow")
	}

	return DynamicExecutionResult{
		Message:    "Workflow executed successfully",
		Data:       workflowExecutionReturnValue(workflow, &payload),
		Source:     "workflow",
		pagination: pagination,
	}, nil
}

func (s *DynamicService) ExportDynamicItems(ctx context.Context, input ExportDynamicItemsInput) (ExportDynamicItemsResult, error) {
	req := input.Request
	if req.SchemaName == "" {
		return ExportDynamicItemsResult{}, &ServiceError{
			Status:  http.StatusBadRequest,
			Message: "schemaName is required",
			Data:    nil,
		}
	}

	container, err := s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, req.SchemaName)
	if err != nil {
		return ExportDynamicItemsResult{}, &ServiceError{Status: http.StatusInternalServerError, Message: "Failed to fetch container model", Err: err}
	}

	filter := buildExportFilter(container, req.Filters)
	if req.Search != "" {
		orClauses, err := utils.BuildSearchWithReferences(ctx, container, req.Search)
		if err != nil {
			return ExportDynamicItemsResult{}, &ServiceError{Status: http.StatusInternalServerError, Message: "Failed to build search filter", Err: err}
		}
		if len(orClauses) > 0 {
			if len(filter) > 0 {
				filter = bson.M{"$and": []bson.M{filter, {"$or": orClauses}}}
			} else {
				filter = bson.M{"$or": orClauses}
			}
		}
	}

	maxExportLimit := configs.GetMaxExportLimit()
	findOpts := options.Find()
	if req.Limit > maxExportLimit {
		log.Printf("Export limit exceeded for schema=%s tenant=%s project=%s requested=%d max=%d", req.SchemaName, input.TenantID, input.ProjectID, req.Limit, maxExportLimit)
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
		findOpts.SetSkip(int64((page - 1) * limit))
		findOpts.SetLimit(int64(limit))
	} else {
		findOpts.SetLimit(int64(maxExportLimit + 1))
	}

	pager := utils.Pager{Enabled: false}
	items, err := s.repository.Query(ctx, input.TenantID, input.ProjectID, req.SchemaName, filter, findOpts, &pager)
	if err != nil {
		return ExportDynamicItemsResult{}, &ServiceError{Status: http.StatusInternalServerError, Message: "Failed to fetch items", Err: err}
	}
	if len(items) > maxExportLimit {
		log.Printf("Export limit exceeded for schema=%s tenant=%s project=%s max=%d", req.SchemaName, input.TenantID, input.ProjectID, maxExportLimit)
		items = items[:maxExportLimit]
	}

	utils.StripHashed(container.Fields, items)
	items, err = utils.PopulateIfNeeded(ctx, input.TenantID, input.ProjectID, container, items)
	if err != nil {
		return ExportDynamicItemsResult{}, &ServiceError{Status: http.StatusInternalServerError, Message: "Failed to populate items", Err: err}
	}

	content, err := buildExportWorkbook(container, items, req.Fields)
	if err != nil {
		return ExportDynamicItemsResult{}, &ServiceError{Status: http.StatusInternalServerError, Message: err.Error(), Err: err}
	}

	return ExportDynamicItemsResult{
		FileName: fmt.Sprintf("%s_export.xlsx", req.SchemaName),
		Content:  content,
	}, nil
}

type bulkUpdateItemResult struct {
	Success interface{}
	Failed  map[string]interface{}
}

type bulkDeleteItemResult struct {
	Success interface{}
	Failed  map[string]interface{}
}

func (s *DynamicService) deleteOneFromBulk(ctx mongo.SessionContext, input DeleteMultipleDynamicItemsInput, container *models.ContainerModel, allContainers []models.ContainerModel, item map[string]interface{}) (bulkDeleteItemResult, interface{}, error) {
	idStr, errMessage := extractBulkUpdateID(item)
	if errMessage != "" {
		return bulkDeleteItemResult{Failed: map[string]interface{}{
			"item":  item,
			"error": errMessage,
		}}, nil, nil
	}

	deleteID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		return bulkDeleteItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "Provided ID is not in the valid format",
		}}, nil, nil
	}

	lockKey := fmt.Sprintf("lock:delete:%s:%s", input.Schema, deleteID.Hex())
	lockID, locked := utils.AcquireLock(lockKey, 10*time.Second)
	if !locked {
		return bulkDeleteItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "Another process is already deleting this item",
		}}, nil, nil
	}
	defer utils.ReleaseLock(lockKey, lockID)

	if err := s.ensureDeleteReferences(ctx, input.TenantID, input.ProjectID, input.Schema, deleteID, allContainers); err != nil {
		if serviceErr, ok := err.(*ServiceError); ok {
			return bulkDeleteItemResult{Failed: map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": serviceErr.Message,
			}}, nil, nil
		}
		return bulkDeleteItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": err.Error(),
		}}, nil, nil
	}

	if err := s.forceDeleteReferences(ctx, input.TenantID, input.ProjectID, input.Schema, deleteID, allContainers, false); err != nil {
		log.Printf("Failed to force delete referenced items for schema: %s, error: %v", input.Schema, err)
	}

	deletedDoc, findErr := s.repository.FindByID(ctx, input.TenantID, input.ProjectID, input.Schema, deleteID)
	if findErr != nil {
		log.Printf("Failed to fetch item before delete (multiple) for schema: %s, error: %v", input.Schema, findErr)
	}

	var workflowRecord map[string]interface{}
	if deletedDoc != nil {
		workflowRecord = map[string]interface{}{}
		for k, v := range deletedDoc {
			workflowRecord[k] = v
		}
	}
	workflowPayload := workflowExecutionPayload{
		TenantID:    input.TenantID,
		ProjectID:   input.ProjectID,
		SchemaName:  input.Schema,
		Record:      workflowRecord,
		OldRecord:   workflowRecord,
		StepOutputs: map[string]interface{}{},
		UserID:      input.UserID,
		AuditUser:   input.User,
		Container:   container,
	}
	if err := s.runTransactionalWorkflows(ctx, workflowPayload, models.WorkflowTriggerBeforeDelete); err != nil {
		return bulkDeleteItemResult{}, nil, err
	}

	deleteResult, err := s.repository.DeleteByID(ctx, input.TenantID, input.ProjectID, input.Schema, deleteID)
	if err != nil {
		return bulkDeleteItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "Failed to delete item: " + err.Error(),
		}}, nil, nil
	}
	if deleteResult.DeletedCount == 0 {
		return bulkDeleteItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "No item found with the specified ID",
		}}, nil, nil
	}

	if err := s.runTransactionalWorkflows(ctx, workflowPayload, models.WorkflowTriggerAfterDelete); err != nil {
		return bulkDeleteItemResult{}, nil, err
	}
	if err := s.enqueueOutboxWorkflows(ctx, workflowPayload, models.WorkflowTriggerAfterDelete); err != nil {
		return bulkDeleteItemResult{}, nil, err
	}

	return bulkDeleteItemResult{Success: map[string]interface{}{
		"id":     idStr,
		"result": deleteResult,
	}}, deletedDoc, nil
}

func (s *DynamicService) updateOneFromBulk(ctx mongo.SessionContext, input UpdateMultipleDynamicItemsInput, container *models.ContainerModel, item map[string]interface{}) (bulkUpdateItemResult, interface{}, interface{}, error) {
	idStr, errMessage := extractBulkUpdateID(item)
	if errMessage != "" {
		return bulkUpdateItemResult{Failed: map[string]interface{}{
			"item":  item,
			"error": errMessage,
		}}, nil, nil, nil
	}

	updateID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		return bulkUpdateItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "Provided ID is not in the valid format",
		}}, nil, nil, nil
	}

	lockKey := fmt.Sprintf("lock:update:%s:%s", input.Schema, updateID.Hex())
	lockID, locked := utils.AcquireLock(lockKey, 10*time.Second)
	if !locked {
		return bulkUpdateItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "Another process is already updating this item",
		}}, nil, nil, nil
	}
	defer utils.ReleaseLock(lockKey, lockID)

	delete(item, "id")
	delete(item, "_id")

	if err := validators.PrepareUpdateFields(container, item); err != nil {
		return bulkUpdateItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "Validation failed: " + err.Error(),
		}}, nil, nil, nil
	}

	existingItem, err := s.repository.FindByID(ctx, input.TenantID, input.ProjectID, input.Schema, updateID)
	if err != nil {
		return bulkUpdateItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "Failed to fetch existing item: " + err.Error(),
		}}, nil, nil, nil
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
			}}, nil, nil, nil
		}
		return bulkUpdateItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": err.Error(),
		}}, nil, nil, nil
	}

	workflowPayload := workflowExecutionPayload{
		TenantID:    input.TenantID,
		ProjectID:   input.ProjectID,
		SchemaName:  input.Schema,
		Record:      existingItem,
		OldRecord:   beforeDoc,
		StepOutputs: map[string]interface{}{},
		UserID:      input.UserID,
		AuditUser:   input.User,
		Container:   container,
	}
	if err := s.runTransactionalWorkflows(ctx, workflowPayload, models.WorkflowTriggerBeforeUpdate); err != nil {
		return bulkUpdateItemResult{}, nil, nil, err
	}

	updateResult, err := s.repository.UpdateByID(ctx, input.TenantID, input.ProjectID, input.Schema, updateID, existingItem)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			serviceErr := duplicateKeyServiceError(container, err)
			return bulkUpdateItemResult{Failed: map[string]interface{}{
				"id":    idStr,
				"item":  item,
				"error": serviceErr.Message,
			}}, nil, nil, nil
		}
		return bulkUpdateItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "Failed to update item: " + err.Error(),
		}}, nil, nil, nil
	}
	if updateResult.MatchedCount == 0 {
		return bulkUpdateItemResult{Failed: map[string]interface{}{
			"id":    idStr,
			"item":  item,
			"error": "No matching item found to update",
		}}, nil, nil, nil
	}

	if err := s.runTransactionalWorkflows(ctx, workflowPayload, models.WorkflowTriggerAfterUpdate); err != nil {
		return bulkUpdateItemResult{}, nil, nil, err
	}
	if err := s.enqueueOutboxWorkflows(ctx, workflowPayload, models.WorkflowTriggerAfterUpdate); err != nil {
		return bulkUpdateItemResult{}, nil, nil, err
	}

	return bulkUpdateItemResult{Success: map[string]interface{}{
		"id":     idStr,
		"result": updateResult,
	}}, beforeDoc, existingItem, nil
}

func (s *DynamicService) resolveContainer(ctx context.Context, input CreateDynamicItemInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveCreateMultipleContainer(ctx context.Context, input CreateMultipleDynamicItemsInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveUpdateContainer(ctx context.Context, input UpdateDynamicItemInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveUpdateMultipleContainer(ctx context.Context, input UpdateMultipleDynamicItemsInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveDeleteContainer(ctx context.Context, input DeleteDynamicItemInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveDeleteMultipleContainer(ctx context.Context, input DeleteMultipleDynamicItemsInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveGetAllContainer(ctx context.Context, input GetAllDynamicItemsInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	if input.Schema == "" {
		return nil, utils.ErrNoSchemaName
	}
	return s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveGetDynamicContainer(ctx context.Context, input GetDynamicItemInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	if input.Schema == "" {
		return nil, utils.ErrNoSchemaName
	}
	return s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveSearchContainer(ctx context.Context, input SearchDynamicItemsInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	if input.Schema == "" {
		return nil, utils.ErrNoSchemaName
	}
	return s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveFilterContainer(ctx context.Context, input FilterDynamicItemsInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	if input.Schema == "" {
		return nil, utils.ErrNoSchemaName
	}
	return s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolvePaginatedContainer(ctx context.Context, input GetPaginatedDynamicItemsInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	if input.Schema == "" {
		return nil, utils.ErrNoSchemaName
	}
	return s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolvePipelineContainer(ctx context.Context, input GetPipelineInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveDynamicCodeContainer(ctx context.Context, input ExecuteDynamicCodeInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveTestPipelineContainer(ctx context.Context, input TestPipelineInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveDynamicAPIContainer(ctx context.Context, input ExecuteDynamicAPIInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
}

func (s *DynamicService) resolveWorkflowContainer(ctx context.Context, input ExecuteWorkflowInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	return s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
}

func duplicateKeyServiceError(container *models.ContainerModel, err error) *ServiceError {
	fieldName := duplicateKeyFieldName(container, err)
	message := "A document with the same unique field already exists."
	if fieldName != "" {
		message = fmt.Sprintf("A document with the same %s already exists.", fieldName)
	}

	return &ServiceError{
		Status:  http.StatusConflict,
		Message: message,
		Data:    nil,
		Err:     err,
	}
}

func duplicateKeyFieldName(container *models.ContainerModel, err error) string {
	if container == nil || err == nil {
		return ""
	}

	errText := strings.ToLower(err.Error())
	for _, field := range container.Fields {
		if !field.Unique {
			continue
		}
		fieldName := strings.ToLower(field.Name)
		indexName := strings.ToLower(fmt.Sprintf("idx_%s_unique", field.Name))
		if strings.Contains(errText, indexName) ||
			strings.Contains(errText, fieldName+"_1") ||
			strings.Contains(errText, fieldName+":") {
			return field.Name
		}
	}

	return ""
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
			result, err := s.repository.DeleteManyByField(ctx, tenantID, projectID, container.SchemaName, field.Name, deleteID)
			if err != nil {
				log.Printf("Failed to force delete referenced items for schema: %s, error: %v", schemaName, err)
				if failOnError {
					return &ServiceError{
						Status:  http.StatusInternalServerError,
						Message: "Failed to force delete referenced items.",
						Err:     err,
					}
				}
				continue
			}
			if result != nil && result.DeletedCount > 0 {
				if err := utils.InvalidateSchemaAndTriggeredCaches(ctx, tenantID, projectID, container.SchemaName, container.Redis.TriggeredRedisCaches); err != nil {
					log.Printf("Failed to invalidate force-deleted referenced schema cache for schema: %s, error: %v", container.SchemaName, err)
					if failOnError {
						return &ServiceError{
							Status:  http.StatusInternalServerError,
							Message: "Failed to invalidate force-deleted referenced schema cache.",
							Err:     err,
						}
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

func findPipelineStage(container *models.ContainerModel, pipelineName string) (models.PipelineStage, bool) {
	for _, stage := range container.Pipelines {
		if stage.Name == pipelineName {
			return stage, true
		}
	}
	return models.PipelineStage{}, false
}

func findDynamicFunctionCode(container *models.ContainerModel, functionName string) (string, bool) {
	for _, dynamicFunc := range container.DynamicFunctions {
		if dynamicFunc.Name == functionName {
			return dynamicFunc.CodeJSON, true
		}
	}
	return "", false
}

func findDynamicAPI(container *models.ContainerModel, apiName string) (models.DynamicApiModel, bool) {
	for _, api := range container.DynamicApis {
		if api.Name == apiName {
			return api, true
		}
	}
	return models.DynamicApiModel{}, false
}

func missingDynamicAPIDependencies(dynamicAPI models.DynamicApiModel, requestBody map[string]interface{}) []string {
	if len(dynamicAPI.Dependencies) == 0 {
		return nil
	}

	missing := make([]string, 0)
	for _, dependency := range dynamicAPI.Dependencies {
		if value, ok := requestBody[dependency]; !ok || value == nil {
			missing = append(missing, dependency)
		}
	}
	return missing
}

func (s *DynamicService) executePluginFunction(pluginFileName, functionName string, fiberCtx *fiber.Ctx, required bool) (interface{}, bool, error) {
	loadedPlugin, err := plugin.Open(pluginFileName)
	if err != nil {
		if required {
			return nil, false, fmt.Errorf("load plugin: %w", err)
		}
		return nil, false, nil
	}

	function, err := loadedPlugin.Lookup(functionName)
	if err != nil {
		if required {
			return nil, false, fmt.Errorf("lookup function: %w", err)
		}
		return nil, false, nil
	}

	executeFunc, ok := function.(func(*fiber.Ctx) (interface{}, error))
	if !ok {
		return nil, true, errors.New("function has incompatible signature")
	}

	result, err := executeFunc(fiberCtx)
	return result, true, err
}

func (s *DynamicService) cacheDynamicFunctionResult(ctx context.Context, redisKey, currentQuery string, result interface{}, container *models.ContainerModel, shouldCache bool) {
	if !shouldCache {
		return
	}
	expiration := cacheDuration(container.Redis.CacheTime)
	s.cache.SetValue(ctx, redisKey, result, expiration)
	if !configs.RedisCircuitAllow() {
		return
	}
	err := configs.RedisClient.Set(ctx, redisKey+"-query", currentQuery, expiration).Err()
	configs.RedisCircuitRecordResult(err)
}

func schemaCacheKey(ctx context.Context, tenantID, projectID, schemaName string, shouldCache bool, routeName, queryValue string) (string, bool) {
	if !shouldCache {
		return "", false
	}

	version, err := utils.GetSchemaCacheVersion(ctx, tenantID, projectID, schemaName)
	if err != nil {
		log.Printf("Failed to get cache version for schema=%s tenant=%s project=%s: %v", schemaName, tenantID, projectID, err)
		return "", false
	}

	return utils.BuildVersionedCacheKey(tenantID, projectID, schemaName, version, routeName, utils.HashCacheQuery(queryValue)), true
}

func cacheValueFingerprint(value interface{}) string {
	if value == nil {
		return ""
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(payload)
}

func cacheDuration(cacheMinutes int) time.Duration {
	if cacheMinutes > 0 {
		return time.Duration(cacheMinutes) * time.Minute
	}
	return utils.DefaultCacheTTLDuration()
}

func pluginExecutionMessage(err error) string {
	if err == nil {
		return "Failed to execute function"
	}
	if strings.Contains(err.Error(), "load plugin") {
		return "Failed to load new plugin"
	}
	if strings.Contains(err.Error(), "lookup function") {
		return "Failed to lookup function in new plugin"
	}
	return "Failed to execute function"
}

func cachedResponseItems(response fiber.Map) ([]map[string]interface{}, bool) {
	itemsInterface, ok := response["items"].([]interface{})
	if !ok {
		return nil, false
	}

	items := make([]map[string]interface{}, 0, len(itemsInterface))
	for _, item := range itemsInterface {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		items = append(items, itemMap)
	}

	return items, true
}

func buildExportFilter(container *models.ContainerModel, filters map[string]interface{}) bson.M {
	filter := bson.M{}
	if len(filters) == 0 {
		return filter
	}

	for key, value := range filters {
		if strVal, ok := value.(string); ok && strVal == "" {
			continue
		}
		if containerHasField(container, key) {
			filter[key] = value
		}
	}
	return filter
}

func containerHasField(container *models.ContainerModel, fieldName string) bool {
	for _, field := range container.Fields {
		if field.Name == fieldName {
			return true
		}
	}
	return false
}

func buildExportWorkbook(container *models.ContainerModel, items []map[string]interface{}, requestedFields []string) ([]byte, error) {
	workbook := excelize.NewFile()
	sheetName := "Sheet1"
	index, err := workbook.NewSheet(sheetName)
	if err != nil {
		return nil, errors.New("Failed to create sheet")
	}
	workbook.SetActiveSheet(index)

	exportFields := selectExportFields(container, requestedFields)
	for i, field := range exportFields {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		workbook.SetCellValue(sheetName, cell, exportColumnName(field))
	}

	for i, item := range items {
		row := i + 2
		for j, field := range exportFields {
			cell, _ := excelize.CoordinatesToCellName(j+1, row)
			if val, exists := item[field.Name]; exists {
				workbook.SetCellValue(sheetName, cell, exportCellValue(field, val))
			}
		}
	}

	buffer := bytes.NewBuffer(nil)
	if _, err := workbook.WriteTo(buffer); err != nil {
		return nil, errors.New("Failed to write excel to buffer")
	}
	return buffer.Bytes(), nil
}

func selectExportFields(container *models.ContainerModel, requestedFields []string) []models.Field {
	if len(requestedFields) == 0 {
		return container.Fields
	}

	exportFields := make([]models.Field, 0, len(requestedFields))
	for _, reqField := range requestedFields {
		for _, field := range container.Fields {
			if field.Name == reqField {
				exportFields = append(exportFields, field)
				break
			}
		}
	}
	return exportFields
}

func exportColumnName(field models.Field) string {
	colName := field.Name
	if field.Frontend != nil && field.Frontend.DisplayName != "" {
		return field.Frontend.DisplayName
	}
	if len(colName) > 0 {
		colName = strings.ToUpper(colName[:1]) + colName[1:]
	}
	var newColName strings.Builder
	for j, r := range colName {
		if j > 0 && r >= 'A' && r <= 'Z' {
			newColName.WriteRune(' ')
		}
		newColName.WriteRune(r)
	}
	return newColName.String()
}

func exportCellValue(field models.Field, val interface{}) interface{} {
	if field.PopulationSettings != nil && len(field.PopulationSettings.DisplayFields) > 0 {
		return formatPopulatedValue(val, field.PopulationSettings.DisplayFields)
	}
	if field.Type == "stringArray" || field.Type == "intArray" {
		return formatExportArray(val)
	}
	return val
}

func formatExportArray(val interface{}) interface{} {
	switch array := val.(type) {
	case []interface{}:
		values := make([]string, 0, len(array))
		for _, v := range array {
			values = append(values, fmt.Sprintf("%v", v))
		}
		return strings.Join(values, ",")
	case []string:
		return strings.Join(array, ",")
	case []int:
		values := make([]string, 0, len(array))
		for _, v := range array {
			values = append(values, fmt.Sprintf("%d", v))
		}
		return strings.Join(values, ",")
	case primitive.A:
		values := make([]string, 0, len(array))
		for _, v := range array {
			values = append(values, fmt.Sprintf("%v", v))
		}
		return strings.Join(values, ",")
	default:
		return val
	}
}

func formatPopulatedValue(val interface{}, displayFields []string) string {
	if populatedObj, ok := val.(map[string]interface{}); ok {
		return joinDisplayFields(populatedObj, displayFields)
	}
	if populatedObj, ok := val.(bson.M); ok {
		return joinDisplayFields(populatedObj, displayFields)
	}
	if populatedArray, ok := val.([]map[string]interface{}); ok {
		arrayParts := make([]string, 0, len(populatedArray))
		for _, populatedObj := range populatedArray {
			if displayValue := joinDisplayFields(populatedObj, displayFields); displayValue != "" {
				arrayParts = append(arrayParts, displayValue)
			}
		}
		return strings.Join(arrayParts, ", ")
	}
	if populatedArray, ok := val.([]interface{}); ok {
		return joinDisplayFieldArray(populatedArray, displayFields)
	}
	if populatedArray, ok := val.(primitive.A); ok {
		return joinDisplayFieldArray([]interface{}(populatedArray), displayFields)
	}
	return fmt.Sprintf("%v", val)
}

func joinDisplayFieldArray(populatedArray []interface{}, displayFields []string) string {
	arrayParts := make([]string, 0, len(populatedArray))
	for _, item := range populatedArray {
		if populatedObj, ok := item.(map[string]interface{}); ok {
			if displayValue := joinDisplayFields(populatedObj, displayFields); displayValue != "" {
				arrayParts = append(arrayParts, displayValue)
			}
		} else if populatedObj, ok := item.(bson.M); ok {
			if displayValue := joinDisplayFields(populatedObj, displayFields); displayValue != "" {
				arrayParts = append(arrayParts, displayValue)
			}
		}
	}
	return strings.Join(arrayParts, ", ")
}

func joinDisplayFields(populatedObj map[string]interface{}, displayFields []string) string {
	parts := make([]string, 0, len(displayFields))
	for _, displayField := range displayFields {
		if fieldVal, exists := populatedObj[displayField]; exists && fieldVal != nil {
			parts = append(parts, fmt.Sprintf("%v", fieldVal))
		}
	}
	return strings.Join(parts, " ")
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
