package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/cache"
	"github.com/osmansam/autotableGo/events"
	"github.com/osmansam/autotableGo/files"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/requests"
	"github.com/osmansam/autotableGo/utils"
	"github.com/osmansam/autotableGo/validators"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

func (s *DynamicService) resolveContainer(input CreateDynamicItemInput) (*models.ContainerModel, error) {
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

func parseCreateErrorMessage(err error) string {
	if err == nil {
		return "Failed to parse body."
	}
	if strings.Contains(err.Error(), "multipart") {
		return "Error parsing form."
	}
	return "Failed to parse body."
}
