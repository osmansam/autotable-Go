package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/requests"
	"github.com/osmansam/autotableGo/utils"
	"github.com/osmansam/autotableGo/validators"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type DynamicArrayMutationInput struct {
	TenantID    string
	ProjectID   string
	Schema      string
	ParentID    string
	ArrayField  string
	RowIdentity interface{}
	Request     requests.ArrayRowMutationRequest
	Reorder     requests.ArrayReorderRequest
	UserID      string
	User        *models.AuditUser
	Container   *models.ContainerModel
}

type DynamicArrayMutationResult struct {
	Parent map[string]interface{} `json:"parent"`
	Row    map[string]interface{} `json:"row,omitempty"`
}

type arrayMutationTransform func(*models.Field, []map[string]interface{}) ([]map[string]interface{}, map[string]interface{}, error)

func embeddedArrayField(container *models.ContainerModel, fieldName string) (*models.Field, error) {
	fieldName = strings.TrimSpace(fieldName)
	if container == nil || fieldName == "" {
		return nil, fmt.Errorf("array field is required")
	}
	for index := range container.Fields {
		field := &container.Fields[index]
		if field.Name != fieldName {
			continue
		}
		if field.Type != "array" || len(field.Children) == 0 {
			return nil, fmt.Errorf("field %q is not an embedded array", fieldName)
		}
		return field, nil
	}
	return nil, fmt.Errorf("array field %q does not exist", fieldName)
}

func embeddedChildField(arrayField *models.Field, childName string) (*models.Field, error) {
	childName = strings.TrimSpace(childName)
	if arrayField == nil || childName == "" {
		return nil, fmt.Errorf("child field is required")
	}
	for index := range arrayField.Children {
		if arrayField.Children[index].Name == childName {
			return &arrayField.Children[index], nil
		}
	}
	return nil, fmt.Errorf("child field %q does not exist", childName)
}

func embeddedIdentityField(arrayField *models.Field, childName string) (*models.Field, error) {
	field, err := embeddedChildField(arrayField, childName)
	if err != nil {
		return nil, err
	}
	switch field.Type {
	case "string", "enum", "int", "number", "float", "boolean", "objectId":
		return field, nil
	default:
		return nil, fmt.Errorf("child field %q cannot identify an array row", childName)
	}
}

func copyArrayRow(row map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(row))
	for key, value := range row {
		result[key] = value
	}
	return result
}

func copyArrayRows(rows []map[string]interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, len(rows))
	for index, row := range rows {
		result[index] = copyArrayRow(row)
	}
	return result
}

func prepareEmbeddedArrayRow(tenantID, projectID string, arrayField *models.Field, row map[string]interface{}) (map[string]interface{}, error) {
	if arrayField == nil {
		return nil, fmt.Errorf("array field is required")
	}
	prepared := copyArrayRow(row)
	childContainer := &models.ContainerModel{Fields: arrayField.Children}
	if err := validators.PrepareCreateItem(tenantID, projectID, childContainer, prepared); err != nil {
		return nil, err
	}
	if err := validators.PrepareMergedUpdateItem(tenantID, projectID, childContainer, prepared, nil); err != nil {
		return nil, err
	}
	return prepared, nil
}

func normalizeArrayRows(value interface{}) ([]map[string]interface{}, error) {
	switch rows := value.(type) {
	case nil:
		return []map[string]interface{}{}, nil
	case []map[string]interface{}:
		return copyArrayRows(rows), nil
	case []interface{}:
		result := make([]map[string]interface{}, 0, len(rows))
		for index, row := range rows {
			nested, err := normalizeArrayRow(row)
			if err != nil {
				return nil, fmt.Errorf("array row %d must be an object", index)
			}
			result = append(result, nested)
		}
		return result, nil
	case bson.A:
		return normalizeArrayRows([]interface{}(rows))
	default:
		return nil, fmt.Errorf("array value must contain objects")
	}
}

func normalizeArrayRow(value interface{}) (map[string]interface{}, error) {
	switch row := value.(type) {
	case map[string]interface{}:
		return copyArrayRow(row), nil
	case bson.M:
		return copyArrayRow(map[string]interface{}(row)), nil
	default:
		return nil, fmt.Errorf("array row must be an object")
	}
}

func arrayIdentityEqual(left, right interface{}) bool {
	if reflect.DeepEqual(left, right) {
		return true
	}
	return fmt.Sprint(left) == fmt.Sprint(right)
}

func matchingArrayRowIndexes(rows []map[string]interface{}, identityField string, identity interface{}) []int {
	matches := make([]int, 0, 1)
	for index, row := range rows {
		if value, exists := row[identityField]; exists && arrayIdentityEqual(value, identity) {
			matches = append(matches, index)
		}
	}
	return matches
}

func requireOneArrayRow(rows []map[string]interface{}, identityField string, identity interface{}) (int, error) {
	matches := matchingArrayRowIndexes(rows, identityField, identity)
	switch len(matches) {
	case 0:
		return -1, fmt.Errorf("array row was not found")
	case 1:
		return matches[0], nil
	default:
		return -1, fmt.Errorf("array row identity is ambiguous")
	}
}

func addArrayRow(rows []map[string]interface{}, identityField string, item map[string]interface{}) ([]map[string]interface{}, map[string]interface{}, error) {
	identity, exists := item[identityField]
	if !exists {
		return nil, nil, fmt.Errorf("array row requires identity field %q", identityField)
	}
	if len(matchingArrayRowIndexes(rows, identityField, identity)) != 0 {
		return nil, nil, fmt.Errorf("array row identity already exists")
	}
	changed := copyArrayRow(item)
	next := append(copyArrayRows(rows), changed)
	return next, changed, nil
}

func updateArrayRow(rows []map[string]interface{}, identityField string, identity interface{}, updates map[string]interface{}) ([]map[string]interface{}, map[string]interface{}, error) {
	index, err := requireOneArrayRow(rows, identityField, identity)
	if err != nil {
		return nil, nil, err
	}
	next := copyArrayRows(rows)
	changed := next[index]
	for key, value := range updates {
		changed[key] = value
	}
	newIdentity, exists := changed[identityField]
	if !exists {
		return nil, nil, fmt.Errorf("array row requires identity field %q", identityField)
	}
	for candidateIndex, candidate := range next {
		if candidateIndex != index && arrayIdentityEqual(candidate[identityField], newIdentity) {
			return nil, nil, fmt.Errorf("array row identity already exists")
		}
	}
	return next, changed, nil
}

func deleteArrayRow(rows []map[string]interface{}, identityField string, identity interface{}) ([]map[string]interface{}, map[string]interface{}, error) {
	index, err := requireOneArrayRow(rows, identityField, identity)
	if err != nil {
		return nil, nil, err
	}
	deleted := copyArrayRow(rows[index])
	next := copyArrayRows(rows[:index])
	next = append(next, copyArrayRows(rows[index+1:])...)
	return next, deleted, nil
}

func reorderArrayRows(rows []map[string]interface{}, identityField, orderField string, identities []interface{}) ([]map[string]interface{}, error) {
	if len(identities) != len(rows) {
		return nil, fmt.Errorf("reorder identities must include every array row")
	}
	next := make([]map[string]interface{}, 0, len(rows))
	used := make(map[int]struct{}, len(rows))
	for order, identity := range identities {
		index, err := requireOneArrayRow(rows, identityField, identity)
		if err != nil {
			return nil, err
		}
		if _, exists := used[index]; exists {
			return nil, fmt.Errorf("reorder identities contain a duplicate")
		}
		used[index] = struct{}{}
		row := copyArrayRow(rows[index])
		row[orderField] = order
		next = append(next, row)
	}
	return next, nil
}

func arrayMutationServiceError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	status := http.StatusBadRequest
	if strings.Contains(message, "not found") {
		status = http.StatusNotFound
	}
	if strings.Contains(message, "ambiguous") || strings.Contains(message, "already exists") || strings.Contains(message, "duplicate") {
		status = http.StatusConflict
	}
	return &ServiceError{Status: status, Message: message, Err: err}
}

func (s *DynamicService) resolveArrayMutationContainer(ctx context.Context, input DynamicArrayMutationInput) (*models.ContainerModel, error) {
	if input.Container != nil {
		return input.Container, nil
	}
	container, err := s.repository.GetContainerModel(ctx, input.TenantID, input.ProjectID, input.Schema)
	if err != nil {
		return nil, &ServiceError{Status: http.StatusNotFound, Message: "Schema was not found", Err: err}
	}
	return container, nil
}

func (s *DynamicService) mutateArrayRows(ctx context.Context, input DynamicArrayMutationInput, operation string, transform arrayMutationTransform) (*DynamicArrayMutationResult, error) {
	parentID, err := primitive.ObjectIDFromHex(input.ParentID)
	if err != nil {
		return nil, &ServiceError{Status: http.StatusBadRequest, Message: "Parent ID is not valid", Err: err}
	}

	lockKey := fmt.Sprintf("lock:update:%s:%s", input.Schema, parentID.Hex())
	lockID, locked := utils.AcquireLock(lockKey, 10*time.Second)
	if !locked {
		return nil, &ServiceError{Status: http.StatusConflict, Message: "Another process is already updating this item"}
	}
	defer utils.ReleaseLock(lockKey, lockID)

	container, err := s.resolveArrayMutationContainer(ctx, input)
	if err != nil {
		return nil, err
	}
	arrayField, err := embeddedArrayField(container, input.ArrayField)
	if err != nil {
		return nil, arrayMutationServiceError(err)
	}

	existingItem, err := s.repository.FindByID(ctx, input.TenantID, input.ProjectID, input.Schema, parentID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, &ServiceError{Status: http.StatusNotFound, Message: "Parent item was not found", Err: err}
		}
		return nil, &ServiceError{Status: http.StatusInternalServerError, Message: "Failed to fetch parent item", Err: err}
	}
	rows, err := normalizeArrayRows(existingItem[arrayField.Name])
	if err != nil {
		return nil, arrayMutationServiceError(err)
	}
	nextRows, changedRow, err := transform(arrayField, rows)
	if err != nil {
		return nil, arrayMutationServiceError(err)
	}

	beforeDoc := copyArrayRow(existingItem)
	nextParent := copyArrayRow(existingItem)
	nextParent[arrayField.Name] = nextRows
	mutationContext := map[string]interface{}{
		"operation":        operation,
		"arrayField":       arrayField.Name,
		"rowIdentityField": input.Request.RowIdentityField,
		"rowIdentity":      input.RowIdentity,
	}
	if operation == "reorder" {
		mutationContext["rowIdentityField"] = input.Reorder.RowIdentityField
		mutationContext["rowIdentity"] = input.Reorder.RowIdentities
	}

	var updateResult *mongo.UpdateResult
	err = s.repository.WithTransaction(ctx, func(txCtx mongo.SessionContext) error {
		workflowPayload := workflowExecutionPayload{
			TenantID:    input.TenantID,
			ProjectID:   input.ProjectID,
			SchemaName:  input.Schema,
			Record:      nextParent,
			OldRecord:   beforeDoc,
			StepOutputs: map[string]interface{}{"arrayMutation": mutationContext},
			UserID:      input.UserID,
			AuditUser:   input.User,
			Container:   container,
		}
		if err := s.runTransactionalWorkflows(txCtx, workflowPayload, models.WorkflowTriggerBeforeUpdate); err != nil {
			return err
		}
		updateResult, err = s.repository.UpdateByFilter(
			txCtx,
			input.TenantID,
			input.ProjectID,
			input.Schema,
			bson.M{"_id": parentID, arrayField.Name: existingItem[arrayField.Name]},
			bson.M{"$set": bson.M{arrayField.Name: nextRows}},
		)
		if err != nil {
			return err
		}
		if updateResult.MatchedCount == 0 {
			return &ServiceError{Status: http.StatusConflict, Message: "Parent array changed during the operation"}
		}
		if err := s.runTransactionalWorkflows(txCtx, workflowPayload, models.WorkflowTriggerAfterUpdate); err != nil {
			return err
		}
		if err := s.enqueueOutboxWorkflows(txCtx, workflowPayload, models.WorkflowTriggerAfterUpdate); err != nil {
			return err
		}
		return s.insertDynamicPostWrite(
			txCtx,
			input.TenantID,
			input.ProjectID,
			input.Schema,
			models.DynamicOutboxOperationUpdate,
			input.UserID,
			container,
			buildDynamicAuditLog(input.TenantID, input.ProjectID, container.SchemaName, models.DynamicOutboxOperationUpdate, input.User, beforeDoc, nextParent),
		)
	})
	if err != nil {
		var serviceErr *ServiceError
		if errors.As(err, &serviceErr) {
			return nil, serviceErr
		}
		return nil, workflowExecutionServiceError(err, "Failed to mutate embedded array")
	}

	responseParent := copyArrayRow(nextParent)
	utils.StripHashed(container.Fields, []map[string]interface{}{responseParent})
	return &DynamicArrayMutationResult{Parent: responseParent, Row: changedRow}, nil
}

func (s *DynamicService) AddArrayRow(ctx context.Context, input DynamicArrayMutationInput) (*DynamicArrayMutationResult, error) {
	return s.mutateArrayRows(ctx, input, "add", func(arrayField *models.Field, rows []map[string]interface{}) ([]map[string]interface{}, map[string]interface{}, error) {
		if _, err := embeddedIdentityField(arrayField, input.Request.RowIdentityField); err != nil {
			return nil, nil, err
		}
		prepared, err := prepareEmbeddedArrayRow(input.TenantID, input.ProjectID, arrayField, input.Request.Item)
		if err != nil {
			return nil, nil, err
		}
		return addArrayRow(rows, input.Request.RowIdentityField, prepared)
	})
}

func (s *DynamicService) UpdateArrayRow(ctx context.Context, input DynamicArrayMutationInput) (*DynamicArrayMutationResult, error) {
	return s.mutateArrayRows(ctx, input, "update", func(arrayField *models.Field, rows []map[string]interface{}) ([]map[string]interface{}, map[string]interface{}, error) {
		if _, err := embeddedIdentityField(arrayField, input.Request.RowIdentityField); err != nil {
			return nil, nil, err
		}
		_, merged, err := updateArrayRow(rows, input.Request.RowIdentityField, input.RowIdentity, input.Request.Updates)
		if err != nil {
			return nil, nil, err
		}
		prepared, err := prepareEmbeddedArrayRow(input.TenantID, input.ProjectID, arrayField, merged)
		if err != nil {
			return nil, nil, err
		}
		return updateArrayRow(rows, input.Request.RowIdentityField, input.RowIdentity, prepared)
	})
}

func (s *DynamicService) DeleteArrayRow(ctx context.Context, input DynamicArrayMutationInput) (*DynamicArrayMutationResult, error) {
	return s.mutateArrayRows(ctx, input, "delete", func(arrayField *models.Field, rows []map[string]interface{}) ([]map[string]interface{}, map[string]interface{}, error) {
		if _, err := embeddedIdentityField(arrayField, input.Request.RowIdentityField); err != nil {
			return nil, nil, err
		}
		return deleteArrayRow(rows, input.Request.RowIdentityField, input.RowIdentity)
	})
}

func (s *DynamicService) ReorderArrayRows(ctx context.Context, input DynamicArrayMutationInput) (*DynamicArrayMutationResult, error) {
	return s.mutateArrayRows(ctx, input, "reorder", func(arrayField *models.Field, rows []map[string]interface{}) ([]map[string]interface{}, map[string]interface{}, error) {
		if _, err := embeddedIdentityField(arrayField, input.Reorder.RowIdentityField); err != nil {
			return nil, nil, err
		}
		orderField, err := embeddedChildField(arrayField, input.Reorder.OrderField)
		if err != nil {
			return nil, nil, err
		}
		if orderField.Type != "int" && orderField.Type != "number" {
			return nil, nil, fmt.Errorf("order field %q must be numeric", input.Reorder.OrderField)
		}
		next, err := reorderArrayRows(rows, input.Reorder.RowIdentityField, input.Reorder.OrderField, input.Reorder.RowIdentities)
		return next, nil, err
	})
}
