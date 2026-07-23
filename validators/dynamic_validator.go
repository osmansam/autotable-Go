package validators

import (
	"fmt"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type EquationFieldError struct {
	FieldName string
	Err       error
}

type EquationItemFieldError struct {
	FieldName string
	Index     int
	Err       error
}

func (e *EquationItemFieldError) Error() string {
	return fmt.Sprintf("Error evaluating equation for field %s in item %d: %v", e.FieldName, e.Index, e.Err)
}

func (e *EquationItemFieldError) Unwrap() error {
	return e.Err
}

func (e *EquationFieldError) Error() string {
	return fmt.Sprintf("Error evaluating equation for field %s: %v", e.FieldName, e.Err)
}

func (e *EquationFieldError) Unwrap() error {
	return e.Err
}

func PrepareCreateItem(tenantID, projectID string, container *models.ContainerModel, itemMap map[string]interface{}) error {
	keepAllowedFields(container, itemMap)
	setCreateTimestamps(container, itemMap)

	if err := evaluateEquationFields(tenantID, projectID, container, itemMap); err != nil {
		return err
	}
	convertDateFields(container.Fields, itemMap)
	if err := utils.ValidateContainerModel(itemMap, *container); err != nil {
		return err
	}

	convertObjectIDFields(container, itemMap)
	return nil
}

func PrepareCreateItems(tenantID, projectID string, container *models.ContainerModel, items []map[string]interface{}) error {
	now := time.Now().UTC().Format(time.RFC3339)
	for i, item := range items {
		keepAllowedFields(container, item)
		setCreateTimestampsAt(container, item, now)

		if err := evaluateEquationFieldsForItem(tenantID, projectID, container, item, i); err != nil {
			return err
		}

		convertDateFields(container.Fields, item)
		if err := utils.ValidateContainerModel(item, *container); err != nil {
			return fmt.Errorf("validation failed for item at index %d: %w", i, err)
		}

		convertObjectIDFields(container, item)
		items[i] = item
	}

	return nil
}

func PrepareUpdateFields(container *models.ContainerModel, updatedItemMap map[string]interface{}) error {
	keepAllowedFields(container, updatedItemMap)
	setUpdateTimestamp(container, updatedItemMap)
	convertDateFields(container.Fields, updatedItemMap)
	return utils.ValidatePartialUpdate(updatedItemMap, *container)
}

func PrepareMergedUpdateItem(tenantID, projectID string, container *models.ContainerModel, existingItem map[string]interface{}, updatedItemMap map[string]interface{}) error {
	for key, value := range updatedItemMap {
		existingItem[key] = value
	}

	if err := evaluateEquationFields(tenantID, projectID, container, existingItem); err != nil {
		return err
	}

	convertObjectIDFields(container, existingItem)
	convertObjectIDArrayFields(container, existingItem)
	return nil
}

func keepAllowedFields(container *models.ContainerModel, itemMap map[string]interface{}) {
	allowedFields := make(map[string]struct{})
	for _, field := range container.Fields {
		allowedFields[field.Name] = struct{}{}
	}
	for key := range itemMap {
		if _, exists := allowedFields[key]; !exists {
			delete(itemMap, key)
		}
	}
}

func convertDateFields(fields []models.Field, itemMap map[string]interface{}) {
	for _, field := range fields {
		value, exists := itemMap[field.Name]
		if !exists {
			continue
		}

		switch field.Type {
		case "date":
			if converted, ok := parseDateValue(value); ok {
				itemMap[field.Name] = converted
			}
		case "object":
			if nested, ok := value.(map[string]interface{}); ok {
				convertDateFields(field.Children, nested)
			}
		case "array":
			if rows, ok := value.([]interface{}); ok {
				for _, row := range rows {
					if nested, ok := row.(map[string]interface{}); ok {
						convertDateFields(field.Children, nested)
					}
				}
			}
		}
	}
}

func parseDateValue(value interface{}) (time.Time, bool) {
	switch v := value.(type) {
	case time.Time:
		return v, true
	case string:
		if parsed, err := time.Parse(time.RFC3339, v); err == nil {
			return parsed.UTC(), true
		}
		if parsed, err := time.Parse("2006-01-02", v); err == nil {
			return parsed.UTC(), true
		}
	case int64:
		return time.Unix(v, 0).UTC(), true
	case int:
		return time.Unix(int64(v), 0).UTC(), true
	case float64:
		return time.Unix(int64(v), 0).UTC(), true
	}
	return time.Time{}, false
}

func setUpdateTimestamp(container *models.ContainerModel, itemMap map[string]interface{}) {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, field := range container.Fields {
		if field.Name == "updatedAt" {
			itemMap["updatedAt"] = now
		}
	}
}

func setCreateTimestamps(container *models.ContainerModel, itemMap map[string]interface{}) {
	setCreateTimestampsAt(container, itemMap, time.Now().UTC().Format(time.RFC3339))
}

func setCreateTimestampsAt(container *models.ContainerModel, itemMap map[string]interface{}, now string) {
	for _, field := range container.Fields {
		if field.Name == "createdAt" {
			if _, exists := itemMap["createdAt"]; !exists {
				itemMap["createdAt"] = now
			}
		}
		if field.Name == "updatedAt" {
			itemMap["updatedAt"] = now
		}
	}
}

func evaluateEquationFieldsForItem(tenantID, projectID string, container *models.ContainerModel, itemMap map[string]interface{}, index int) error {
	for _, field := range container.Fields {
		if field.Equation == "" {
			continue
		}
		ctx := &utils.EquationContext{
			TenantID:  tenantID,
			ProjectID: projectID,
			Data:      itemMap,
		}
		val, err := utils.EvaluateEquationWithContext(field.Equation, ctx)
		if err != nil {
			return &EquationItemFieldError{FieldName: field.Name, Index: index, Err: err}
		}
		itemMap[field.Name] = val
	}
	return nil
}

func evaluateEquationFields(tenantID, projectID string, container *models.ContainerModel, itemMap map[string]interface{}) error {
	for _, field := range container.Fields {
		if field.Equation == "" {
			continue
		}
		ctx := &utils.EquationContext{
			TenantID:  tenantID,
			ProjectID: projectID,
			Data:      itemMap,
		}
		val, err := utils.EvaluateEquationWithContext(field.Equation, ctx)
		if err != nil {
			return &EquationFieldError{FieldName: field.Name, Err: err}
		}
		itemMap[field.Name] = val
	}
	return nil
}

func convertObjectIDFields(container *models.ContainerModel, itemMap map[string]interface{}) {
	for _, field := range container.Fields {
		if field.Type != "objectId" {
			continue
		}
		if strID, ok := itemMap[field.Name].(string); ok {
			if objID, err := primitive.ObjectIDFromHex(strID); err == nil {
				itemMap[field.Name] = objID
			}
		}
	}
}

func convertObjectIDArrayFields(container *models.ContainerModel, itemMap map[string]interface{}) {
	for _, field := range container.Fields {
		if field.Type != "objectIdArray" {
			continue
		}

		val, exists := itemMap[field.Name]
		if !exists || val == nil {
			continue
		}

		if arrInterface, ok := val.([]interface{}); ok {
			objIDArray := make([]primitive.ObjectID, 0, len(arrInterface))
			for _, item := range arrInterface {
				if strVal, ok := item.(string); ok {
					if objID, err := primitive.ObjectIDFromHex(strVal); err == nil {
						objIDArray = append(objIDArray, objID)
					}
				} else if objID, ok := item.(primitive.ObjectID); ok {
					objIDArray = append(objIDArray, objID)
				}
			}
			itemMap[field.Name] = objIDArray
			continue
		}

		if arrString, ok := val.([]string); ok {
			objIDArray := make([]primitive.ObjectID, 0, len(arrString))
			for _, strVal := range arrString {
				if objID, err := primitive.ObjectIDFromHex(strVal); err == nil {
					objIDArray = append(objIDArray, objID)
				}
			}
			itemMap[field.Name] = objIDArray
		}
	}
}
