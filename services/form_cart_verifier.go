package services

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"reflect"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func verifyAuthoritativeFormCart(
	form models.FormComponentConfig,
	record map[string]interface{},
	products map[string]map[string]interface{},
) (map[string]interface{}, error) {
	authoritativeInput := cloneWorkflowMap(record)
	for _, list := range form.ObjectLists {
		items, _ := workflowSlice(authoritativeInput[list.Key])
		for index, rawItem := range items {
			item, ok := rawItem.(map[string]interface{})
			if !ok {
				return nil, &ServiceError{Status: http.StatusUnprocessableEntity, Message: "FORM_INVALID_QUANTITY"}
			}
			for _, mapping := range list.FieldMappings {
				productID := fmt.Sprint(item[mapping.SourceFormKey])
				product, exists := products[productID]
				if !exists {
					return nil, &ServiceError{Status: http.StatusUnprocessableEntity, Message: "FORM_PRODUCT_NOT_FOUND", Data: map[string]interface{}{"itemIndex": index}}
				}
				value, exists := product[mapping.SourceField]
				if !exists || value == nil {
					return nil, &ServiceError{Status: http.StatusUnprocessableEntity, Message: "FORM_PRODUCT_PRICE_MISSING", Data: map[string]interface{}{"itemIndex": index}}
				}
				item[mapping.TargetField] = value
			}
		}
	}

	authoritative, err := EvaluateFormCart(form, authoritativeInput)
	if err != nil {
		return nil, &ServiceError{Status: http.StatusUnprocessableEntity, Message: err.Error(), Err: err}
	}
	staleIndexes := make([]int, 0)
	for _, list := range form.ObjectLists {
		clientItems, _ := workflowSlice(record[list.Key])
		serverItems, _ := workflowSlice(authoritative[list.Key])
		for index := range serverItems {
			if index >= len(clientItems) {
				staleIndexes = append(staleIndexes, index)
				continue
			}
			client, _ := clientItems[index].(map[string]interface{})
			server, _ := serverItems[index].(map[string]interface{})
			for _, mapping := range list.FieldMappings {
				if !formCartValuesEqual(client[mapping.TargetField], server[mapping.TargetField]) {
					staleIndexes = append(staleIndexes, index)
					break
				}
			}
			for _, calculation := range list.ItemCalculations {
				if !formCartValuesEqual(client[calculation.TargetField], server[calculation.TargetField]) {
					staleIndexes = append(staleIndexes, index)
					break
				}
			}
		}
	}
	for _, summary := range form.Summaries {
		if !formCartValuesEqual(record[summary.TargetField], authoritative[summary.TargetField]) {
			return nil, &ServiceError{Status: http.StatusConflict, Message: "FORM_STALE_PRICE", Data: map[string]interface{}{"itemIndexes": staleIndexes}}
		}
	}
	if len(staleIndexes) > 0 {
		return nil, &ServiceError{Status: http.StatusConflict, Message: "FORM_STALE_PRICE", Data: map[string]interface{}{"itemIndexes": compactFormCartIndexes(staleIndexes)}}
	}
	return authoritative, nil
}

func (s *DynamicService) verifyWorkflowFormCart(ctx context.Context, input ExecuteWorkflowInput) (map[string]interface{}, error) {
	if input.FormConfigRef == nil {
		return input.Record, nil
	}
	pageID, err := primitive.ObjectIDFromHex(input.FormConfigRef.PageID)
	if err != nil {
		return nil, &ServiceError{Status: http.StatusNotFound, Message: "FORM_CONFIG_NOT_FOUND"}
	}
	var page models.PageModel
	if err := utils.GetPageCollectionForProject(input.TenantID, input.ProjectID).FindOne(ctx, bson.M{"_id": pageID}).Decode(&page); err != nil {
		return nil, &ServiceError{Status: http.StatusNotFound, Message: "FORM_CONFIG_NOT_FOUND", Err: err}
	}
	component := findPageComponentByID(&page, input.FormConfigRef.ComponentID)
	if component == nil || component.Form == nil || component.Form.Submit == nil || component.Form.Submit.WorkflowName != input.WorkflowName || component.Form.Submit.WorkflowSchema != input.Schema {
		return nil, &ServiceError{Status: http.StatusNotFound, Message: "FORM_CONFIG_NOT_FOUND"}
	}

	products := make(map[string]map[string]interface{})
	for _, list := range component.Form.ObjectLists {
		items, _ := workflowSlice(input.Record[list.Key])
		for _, mapping := range list.FieldMappings {
			field := formFieldByKey(component.Form.Fields, mapping.SourceFormKey)
			if field == nil || field.SourceSchemaName == "" {
				return nil, &ServiceError{Status: http.StatusNotFound, Message: "FORM_CONFIG_NOT_FOUND"}
			}
			for _, rawItem := range items {
				item, _ := rawItem.(map[string]interface{})
				key := fmt.Sprint(item[mapping.SourceFormKey])
				if _, loaded := products[key]; loaded {
					continue
				}
				lookupID := interface{}(key)
				if objectID, objectErr := primitive.ObjectIDFromHex(key); objectErr == nil {
					lookupID = objectID
				}
				product, findErr := s.repository.FindByID(ctx, input.TenantID, input.ProjectID, field.SourceSchemaName, lookupID)
				if findErr != nil {
					continue
				}
				products[key] = map[string]interface{}(product)
			}
		}
	}
	return verifyAuthoritativeFormCart(*component.Form, input.Record, products)
}

func formFieldByKey(fields []models.FormFieldConfig, key string) *models.FormFieldConfig {
	for index := range fields {
		if fields[index].FormKey == key {
			return &fields[index]
		}
	}
	return nil
}

func findPageComponentByID(page *models.PageModel, id string) *models.ComponentBlock {
	var found *models.ComponentBlock
	var visitComponent func(*models.ComponentBlock)
	visitComponent = func(component *models.ComponentBlock) {
		if component == nil || found != nil {
			return
		}
		if component.ID == id {
			found = component
			return
		}
		for tabIndex := range component.Tabs {
			for componentIndex := range component.Tabs[tabIndex].Components {
				visitComponent(&component.Tabs[tabIndex].Components[componentIndex])
			}
		}
	}
	var visitSection func(*models.Section)
	visitSection = func(section *models.Section) {
		if section == nil || found != nil {
			return
		}
		visitComponent(section.Component)
		if section.Grid != nil {
			for cellIndex := range section.Grid.Cells {
				for componentIndex := range section.Grid.Cells[cellIndex].Components {
					visitComponent(&section.Grid.Cells[cellIndex].Components[componentIndex])
				}
			}
		}
		for cellIndex := range section.Cells {
			for componentIndex := range section.Cells[cellIndex].Components {
				visitComponent(&section.Cells[cellIndex].Components[componentIndex])
			}
		}
		if section.Tabs != nil {
			for tabIndex := range section.Tabs.Tabs {
				for childIndex := range section.Tabs.Tabs[tabIndex].Sections {
					visitSection(&section.Tabs.Tabs[tabIndex].Sections[childIndex])
				}
			}
		}
	}
	for index := range page.Sections {
		visitSection(&page.Sections[index])
	}
	return found
}

func formCartValuesEqual(left, right interface{}) bool {
	leftNumber, leftErr := formCartNumber(left)
	rightNumber, rightErr := formCartNumber(right)
	if leftErr == nil && rightErr == nil {
		return math.Abs(leftNumber-rightNumber) < 0.0000005
	}
	return reflect.DeepEqual(left, right)
}

func compactFormCartIndexes(values []int) []int {
	seen := make(map[int]bool)
	result := make([]int, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
