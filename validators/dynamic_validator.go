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
	if err := utils.ValidateContainerModel(itemMap, *container); err != nil {
		return err
	}

	convertObjectIDFields(container, itemMap)
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

func setCreateTimestamps(container *models.ContainerModel, itemMap map[string]interface{}) {
	now := time.Now().UTC().Format(time.RFC3339)
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
