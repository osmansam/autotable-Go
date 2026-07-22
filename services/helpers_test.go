package services

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/requests"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestServiceError(t *testing.T) {
	cause := errors.New("cause")
	err := &ServiceError{Message: "message", Err: cause}
	if err.Error() != "cause" || !errors.Is(err, cause) {
		t.Fatalf("ServiceError = %v", err)
	}
	if got := (&ServiceError{Message: "message"}).Error(); got != "message" {
		t.Fatalf("Error() = %q", got)
	}
}

func TestWorkflowModeHelpers(t *testing.T) {
	tests := []struct {
		workflowMode  string
		executionMode string
		want          bool
	}{
		{workflowMode: "", executionMode: models.WorkflowModeTransactional, want: true},
		{workflowMode: models.WorkflowModeOutbox, executionMode: models.WorkflowModeOutbox, want: true},
		{workflowMode: models.WorkflowModeHybrid, executionMode: models.WorkflowModeOutbox, want: true},
		{workflowMode: models.WorkflowModeTransactional, executionMode: models.WorkflowModeOutbox},
	}
	for _, tt := range tests {
		if got := workflowSupportsMode(tt.workflowMode, tt.executionMode); got != tt.want {
			t.Fatalf("workflowSupportsMode(%q, %q) = %v, want %v", tt.workflowMode, tt.executionMode, got, tt.want)
		}
	}
	if got := workflowStepExecutionMode(models.DynamicWorkflowStep{ExecutionMode: "custom"}, "ignored"); got != "custom" {
		t.Fatalf("workflowStepExecutionMode() = %q", got)
	}
	if got := workflowStepExecutionMode(models.DynamicWorkflowStep{}, models.WorkflowModeOutbox); got != models.WorkflowModeOutbox {
		t.Fatalf("workflowStepExecutionMode() = %q", got)
	}
	if got := workflowNestedExecutionMode(workflowExecutionPayload{}); got != models.WorkflowModeTransactional {
		t.Fatalf("workflowNestedExecutionMode() = %q", got)
	}
	if got := workflowTargetSchema(models.DynamicWorkflowStep{TargetSchema: "users"}, "orders"); got != "users" {
		t.Fatalf("workflowTargetSchema() = %q", got)
	}
}

func TestWorkflowRequiresTransaction(t *testing.T) {
	tests := []struct {
		name     string
		workflow models.DynamicWorkflow
		want     bool
	}{
		{name: "read only", workflow: models.DynamicWorkflow{Steps: []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeFindRecords, IsActive: true}}}},
		{name: "count read only", workflow: models.DynamicWorkflow{Steps: []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeCountRecords, IsActive: true}}}},
		{name: "pipeline read only", workflow: models.DynamicWorkflow{Steps: []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeRunPipeline, IsActive: true}}}},
		{name: "aggregate read only", workflow: models.DynamicWorkflow{Steps: []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeAggregate, IsActive: true}}}},
		{name: "distinct read only", workflow: models.DynamicWorkflow{Steps: []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeDistinct, IsActive: true}}}},
		{name: "explicit", workflow: models.DynamicWorkflow{RunInTransaction: true}, want: true},
		{name: "write", workflow: models.DynamicWorkflow{Steps: []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeCreateRecord, IsActive: true}}}, want: true},
		{name: "unset record", workflow: models.DynamicWorkflow{Steps: []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeUnsetRecord, IsActive: true}}}, want: true},
		{name: "remove array", workflow: models.DynamicWorkflow{Steps: []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeRemoveArray, IsActive: true}}}, want: true},
		{name: "outbox", workflow: models.DynamicWorkflow{Mode: models.WorkflowModeOutbox, Steps: []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeCallAPI, IsActive: true}}}, want: true},
		{name: "nested write", workflow: models.DynamicWorkflow{Steps: []models.DynamicWorkflowStep{{
			Type:     models.WorkflowStepTypeIf,
			IsActive: true,
			ElseSteps: []models.DynamicWorkflowStep{{
				Type:     models.WorkflowStepTypeAppendArray,
				IsActive: true,
			}},
		}}}, want: true},
		{name: "nested workflow", workflow: models.DynamicWorkflow{Steps: []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeExecuteWorkflow, IsActive: true}}}, want: true},
		{name: "inactive write", workflow: models.DynamicWorkflow{Steps: []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeDeleteRecord}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := workflowRequiresTransaction(tt.workflow); got != tt.want {
				t.Fatalf("workflowRequiresTransaction() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkflowArrayUpdateOperator(t *testing.T) {
	tests := []struct {
		name string
		step models.DynamicWorkflowStep
		want string
	}{
		{name: "append default", step: models.DynamicWorkflowStep{Type: models.WorkflowStepTypeAppendArray}, want: "$addToSet"},
		{name: "append push", step: models.DynamicWorkflowStep{Type: models.WorkflowStepTypeAppendArray, Config: map[string]interface{}{"mode": "push"}}, want: "$push"},
		{name: "remove alias", step: models.DynamicWorkflowStep{Type: models.WorkflowStepTypeRemoveArray}, want: "$pull"},
		{name: "add to set", step: models.DynamicWorkflowStep{Type: models.WorkflowStepTypeAddToSet}, want: "$addToSet"},
		{name: "push", step: models.DynamicWorkflowStep{Type: models.WorkflowStepTypePush}, want: "$push"},
		{name: "pull", step: models.DynamicWorkflowStep{Type: models.WorkflowStepTypePull}, want: "$pull"},
		{name: "pull all", step: models.DynamicWorkflowStep{Type: models.WorkflowStepTypePullAll}, want: "$pullAll"},
		{name: "set array", step: models.DynamicWorkflowStep{Type: models.WorkflowStepTypeSetArray}, want: "$set"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := workflowArrayUpdateOperator(tt.step); err != nil || got != tt.want {
				t.Fatalf("workflowArrayUpdateOperator() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
	if _, err := workflowArrayUpdateOperator(models.DynamicWorkflowStep{Type: "invalid"}); err == nil {
		t.Fatal("workflowArrayUpdateOperator(invalid) error = nil")
	}
	if !workflowArrayValue([]string{"a"}) || workflowArrayValue("a") {
		t.Fatal("workflowArrayValue() returned incorrect result")
	}
}

func TestWorkflowUpdateOperatorHelpers(t *testing.T) {
	valid := map[string]interface{}{
		"$set":      map[string]interface{}{"state": "open"},
		"$unset":    bson.M{"finishHour": ""},
		"$inc":      map[string]interface{}{"count": 1},
		"$addToSet": map[string]interface{}{"items": "id"},
		"$push":     map[string]interface{}{"events": "id"},
		"$pull":     map[string]interface{}{"items": "id"},
	}
	if err := validateWorkflowUpdateOperators(valid); err != nil {
		t.Fatalf("validateWorkflowUpdateOperators(valid) error = %v", err)
	}
	if err := validateWorkflowUpdateOperators(map[string]interface{}{"$rename": map[string]interface{}{"old": "new"}}); err == nil {
		t.Fatal("validateWorkflowUpdateOperators($rename) error = nil")
	}
	if err := validateWorkflowUpdateOperators(map[string]interface{}{"$set": "invalid"}); err == nil {
		t.Fatal("validateWorkflowUpdateOperators(invalid body) error = nil")
	}
	if err := validateWorkflowUpdateOperators(map[string]interface{}{"$set": map[string]interface{}{"$bad": true}}); err == nil {
		t.Fatal("validateWorkflowUpdateOperators(invalid field) error = nil")
	}

	tests := []struct {
		name   string
		config map[string]interface{}
		want   map[string]interface{}
	}{
		{name: "field", config: map[string]interface{}{"field": "finishHour"}, want: map[string]interface{}{"finishHour": ""}},
		{name: "fields", config: map[string]interface{}{"fields": []string{"finishHour", "closedAt"}}, want: map[string]interface{}{"finishHour": "", "closedAt": ""}},
		{name: "values", config: map[string]interface{}{"values": map[string]interface{}{"finishHour": ""}}, want: map[string]interface{}{"finishHour": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := workflowUnsetValues(tt.config); err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("workflowUnsetValues() = %#v, %v; want %#v", got, err, tt.want)
			}
		})
	}
	if _, err := workflowUnsetValues(map[string]interface{}{}); err == nil {
		t.Fatal("workflowUnsetValues(empty) error = nil")
	}
	if _, err := workflowUnsetValues(map[string]interface{}{"field": "$bad"}); err == nil {
		t.Fatal("workflowUnsetValues(invalid field) error = nil")
	}
}

func TestValidateWorkflowExecutionBounds(t *testing.T) {
	if err := validateWorkflowExecutionBounds(workflowExecutionPayload{}, models.DynamicWorkflow{}); err != nil {
		t.Fatalf("validateWorkflowExecutionBounds() error = %v", err)
	}
	if err := validateWorkflowExecutionBounds(workflowExecutionPayload{WorkflowDepth: maxWorkflowDepth + 1}, models.DynamicWorkflow{}); err == nil {
		t.Fatal("depth limit error = nil")
	}
	if err := validateWorkflowExecutionBounds(workflowExecutionPayload{}, models.DynamicWorkflow{Name: "large", Steps: make([]models.DynamicWorkflowStep, maxWorkflowSteps+1)}); err == nil {
		t.Fatal("step limit error = nil")
	}
}

func TestWorkflowContextWithTimeout(t *testing.T) {
	parent := t.Context()
	ctx, cancel := workflowContextWithTimeout(parent, 0)
	defer cancel()
	if ctx != parent {
		t.Fatal("zero timeout should return parent context")
	}
	ctx, cancel = workflowContextWithTimeout(parent, 1)
	defer cancel()
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("positive timeout should set deadline")
	}
}

func TestWorkflowObjectIDHelpers(t *testing.T) {
	id := primitive.NewObjectID()
	item := map[string]interface{}{"owner": id, "members": []interface{}{id, "keep"}}
	workflowStringifyObjectIDsForValidation(&models.ContainerModel{Fields: []models.Field{{Name: "owner", Type: "objectId"}, {Name: "members", Type: "objectIdArray"}}}, item)
	want := map[string]interface{}{"owner": id.Hex(), "members": []interface{}{id.Hex(), "keep"}}
	if !reflect.DeepEqual(item, want) {
		t.Fatalf("workflowStringifyObjectIDsForValidation() = %#v", item)
	}
	nested := map[string]interface{}{"_id": id.Hex(), "child": bson.M{"_id": id.Hex()}}
	normalizeWorkflowIDs(nested)
	if nested["_id"] != id || nested["child"].(bson.M)["_id"] != id {
		t.Fatalf("normalizeWorkflowIDs() = %#v", nested)
	}
	filter := map[string]interface{}{"_id": map[string]interface{}{"$in": []interface{}{id.Hex(), "keep"}}}
	normalizeWorkflowIDs(filter)
	inValues := filter["_id"].(map[string]interface{})["$in"].([]interface{})
	if inValues[0] != id || inValues[1] != "keep" {
		t.Fatalf("normalizeWorkflowIDs(_id.$in) = %#v", filter)
	}
	plainOperatorFilter := map[string]interface{}{"_id": map[string]interface{}{"in": []interface{}{id.Hex(), "keep"}}}
	normalizeWorkflowIDs(plainOperatorFilter)
	plainOperatorValues := plainOperatorFilter["_id"].(map[string]interface{})["$in"].([]interface{})
	if _, exists := plainOperatorFilter["_id"].(map[string]interface{})["in"]; exists || plainOperatorValues[0] != id || plainOperatorValues[1] != "keep" {
		t.Fatalf("normalizeWorkflowIDs(_id.in) = %#v", plainOperatorFilter)
	}
	arrayValues := []interface{}{
		map[string]interface{}{"_id": id.Hex()},
		bson.M{"_id": id.Hex()},
	}
	normalizeWorkflowIDs(arrayValues)
	if arrayValues[0].(map[string]interface{})["_id"] != id || arrayValues[1].(bson.M)["_id"] != id {
		t.Fatalf("normalizeWorkflowIDs(array) = %#v", arrayValues)
	}
	stockContainer := &models.ContainerModel{Fields: []models.Field{{Name: "product", Type: "objectId"}}}
	productFilter := map[string]interface{}{"product": id.Hex()}
	normalizeWorkflowFilterIDs(stockContainer, productFilter)
	if productFilter["product"] != id {
		t.Fatalf("normalizeWorkflowFilterIDs(product) = %#v", productFilter)
	}
	productInFilter := map[string]interface{}{"product": map[string]interface{}{"in": []interface{}{id.Hex(), "keep"}}}
	normalizeWorkflowFilterIDs(stockContainer, productInFilter)
	productInValues := productInFilter["product"].(map[string]interface{})["$in"].([]interface{})
	if _, exists := productInFilter["product"].(map[string]interface{})["in"]; exists || productInValues[0] != id || productInValues[1] != "keep" {
		t.Fatalf("normalizeWorkflowFilterIDs(product.in) = %#v", productInFilter)
	}
}

func TestWorkflowConfigurationHelpers(t *testing.T) {
	if got := workflowStringConfig(map[string]interface{}{"key": " value "}, "key", "fallback"); got != " value " {
		t.Fatalf("workflowStringConfig() = %q", got)
	}
	for _, tt := range []struct {
		value interface{}
		want  int
	}{{3, 3}, {int32(4), 4}, {int64(5), 5}, {float64(6), 6}, {" 7 ", 7}, {"bad", 9}} {
		if got := workflowIntConfig(map[string]interface{}{"key": tt.value}, "key", 9); got != tt.want {
			t.Fatalf("workflowIntConfig(%#v) = %d, want %d", tt.value, got, tt.want)
		}
	}
	if !workflowBoolConfig(map[string]interface{}{"key": " true "}, "key", false) || workflowBoolConfig(map[string]interface{}{"key": false}, "key", true) {
		t.Fatal("workflowBoolConfig() returned incorrect result")
	}
	if !workflowCountRecordsApplyRowAccess(models.DynamicWorkflowStep{}, workflowExecutionPayload{WorkflowTrigger: models.WorkflowTriggerManual}) {
		t.Fatal("manual count_records should apply row access by default")
	}
	if workflowCountRecordsApplyRowAccess(models.DynamicWorkflowStep{}, workflowExecutionPayload{WorkflowTrigger: models.WorkflowTriggerAfterCreate}) {
		t.Fatal("internal count_records should not apply row access by default")
	}
	if !workflowCountRecordsApplyRowAccess(models.DynamicWorkflowStep{Config: map[string]interface{}{"applyRowAccess": true}}, workflowExecutionPayload{WorkflowTrigger: models.WorkflowTriggerAfterCreate}) {
		t.Fatal("count_records applyRowAccess override was ignored")
	}
	pipelineJSON, err := workflowPipelineJSONConfig(models.DynamicWorkflowStep{
		Type: models.WorkflowStepTypeAggregate,
		Config: map[string]interface{}{"pipeline": []interface{}{
			map[string]interface{}{"$match": map[string]interface{}{"state": "{{record.state}}"}},
		}},
	}, workflowExecutionPayload{Record: map[string]interface{}{"state": "open"}})
	if err != nil || !strings.Contains(pipelineJSON, `"state":"open"`) {
		t.Fatalf("workflowPipelineJSONConfig() = %q, %v", pipelineJSON, err)
	}
	if !strings.Contains(pipelineJSON, `"$limit":500`) {
		t.Fatalf("workflowPipelineJSONConfig() did not append safety limit: %q", pipelineJSON)
	}
	if pipelineJSON, err := workflowPipelineJSONConfig(models.DynamicWorkflowStep{
		Type:   models.WorkflowStepTypeAggregate,
		Config: map[string]interface{}{"pipelineJson": `[{"$match":{"state":"{{record.state}}"}}]`},
	}, workflowExecutionPayload{Record: map[string]interface{}{"state": "closed"}}); err != nil || !strings.Contains(pipelineJSON, `"state":"closed"`) {
		t.Fatalf("workflowPipelineJSONConfig(pipelineJson) = %q, %v", pipelineJSON, err)
	}
	if _, err := workflowPipelineJSONConfig(models.DynamicWorkflowStep{
		Type:   models.WorkflowStepTypeAggregate,
		Config: map[string]interface{}{"pipeline": []interface{}{map[string]interface{}{"$out": "orders"}}},
	}, workflowExecutionPayload{}); err == nil {
		t.Fatal("workflowPipelineJSONConfig() allowed unsafe aggregate stage")
	}
	if _, err := workflowPipelineJSONConfig(models.DynamicWorkflowStep{
		Type:   models.WorkflowStepTypeAggregate,
		Config: map[string]interface{}{"pipeline": []interface{}{map[string]interface{}{"$limit": maxWorkflowAggregateLimit + 1}}},
	}, workflowExecutionPayload{}); err == nil {
		t.Fatal("workflowPipelineJSONConfig() allowed oversized aggregate limit")
	}
	if _, err := workflowPipelineJSONConfig(models.DynamicWorkflowStep{
		Type:   models.WorkflowStepTypeAggregate,
		Config: map[string]interface{}{"pipeline": []interface{}{map[string]interface{}{"$skip": -1}}},
	}, workflowExecutionPayload{}); err == nil {
		t.Fatal("workflowPipelineJSONConfig() allowed negative aggregate skip")
	}
	if _, err := workflowPipelineJSONConfig(models.DynamicWorkflowStep{
		Type:   models.WorkflowStepTypeAggregate,
		Config: map[string]interface{}{"pipeline": []interface{}{map[string]interface{}{"$skip": 1.5}}},
	}, workflowExecutionPayload{}); err == nil {
		t.Fatal("workflowPipelineJSONConfig() allowed fractional aggregate skip")
	}
	if _, err := workflowPipelineJSONConfig(models.DynamicWorkflowStep{
		Type:   models.WorkflowStepTypeAggregate,
		Config: map[string]interface{}{"pipeline": []interface{}{map[string]interface{}{"$skip": maxWorkflowAggregateSkip + 1}}},
	}, workflowExecutionPayload{}); err == nil {
		t.Fatal("workflowPipelineJSONConfig() allowed oversized aggregate skip")
	}
}

func TestWorkflowApplyOutputMappingsFlattensNestedFields(t *testing.T) {
	items := []map[string]interface{}{
		{
			"_id": "stock-1",
			"product": map[string]interface{}{
				"_id":   "product-1",
				"name":  "Arkham Horror",
				"price": 120,
			},
			"quantity": 3,
		},
	}

	got := workflowApplyOutputMappings(map[string]interface{}{
		"outputMappings": map[string]interface{}{
			"productId":    "product._id",
			"productName":  "product.name",
			"productPrice": "product.price",
		},
	}, items)

	if got[0]["productId"] != "product-1" || got[0]["productName"] != "Arkham Horror" || got[0]["productPrice"] != 120 {
		t.Fatalf("workflowApplyOutputMappings() = %#v", got[0])
	}
}

func TestWorkflowFindRecordsSearchConfigResolvesQueryValue(t *testing.T) {
	payload := workflowExecutionPayload{Query: map[string]interface{}{"search": "davinci"}}

	if got := workflowFindRecordsSearchKey(map[string]interface{}{
		"search": map[string]interface{}{
			"value":         "{{query.search}}",
			"fields":        []interface{}{"name", "category"},
			"mode":          "contains",
			"caseSensitive": false,
		},
	}, payload); got != "davinci" {
		t.Fatalf("workflowFindRecordsSearchKey(object) = %q", got)
	}

	if got := workflowFindRecordsSearchKey(map[string]interface{}{"search": "{{query.search}}"}, payload); got != "davinci" {
		t.Fatalf("workflowFindRecordsSearchKey(string) = %q", got)
	}

	if got := workflowFindRecordsSearchKey(map[string]interface{}{"searchKey": "{{query.search}}"}, payload); got != "davinci" {
		t.Fatalf("workflowFindRecordsSearchKey(searchKey) = %q", got)
	}

	emptyPayload := workflowExecutionPayload{Query: map[string]interface{}{}}
	if got := workflowFindRecordsSearchKey(map[string]interface{}{
		"search": map[string]interface{}{"value": "{{query.search}}"},
	}, emptyPayload); got != "" {
		t.Fatalf("workflowFindRecordsSearchKey(empty object) = %q, want empty", got)
	}
	if got := workflowFindRecordsSearchKey(map[string]interface{}{"search": "{{query.search}}"}, emptyPayload); got != "" {
		t.Fatalf("workflowFindRecordsSearchKey(empty string) = %q, want empty", got)
	}
}

func TestWorkflowConditions(t *testing.T) {
	payload := workflowExecutionPayload{
		Record:    map[string]interface{}{"amount": 20, "state": "done", "status": "open", "tags": []interface{}{"a"}, "nested": map[string]interface{}{"value": 3}, "name": "invoice-001", "empty": "", "dueDate": "2026-06-05", "quantity": 3, "unitPrice": 7, "total": 21, "discount": 4, "discountedTotal": 17},
		OldRecord: map[string]interface{}{"amount": 10, "state": "new"},
		UserID:    "user",
		AuditUser: &models.AuditUser{Roles: []string{"manager"}},
	}
	tests := []struct {
		name      string
		condition models.WorkflowCondition
		want      bool
	}{
		{name: "equal", condition: models.WorkflowCondition{Field: "state", Operator: "=", Value: "done"}, want: true},
		{name: "not equal", condition: models.WorkflowCondition{Field: "state", Operator: "!=", Value: "new"}, want: true},
		{name: "greater", condition: models.WorkflowCondition{Field: "amount", Operator: ">", Value: 10}, want: true},
		{name: "greater equal", condition: models.WorkflowCondition{Field: "amount", Operator: ">=", Value: 20}, want: true},
		{name: "less", condition: models.WorkflowCondition{Field: "amount", Operator: "<", Value: 10}},
		{name: "less equal", condition: models.WorkflowCondition{Field: "amount", Operator: "<=", Value: 20}, want: true},
		{name: "in", condition: models.WorkflowCondition{Field: "state", Operator: "in", Value: []interface{}{"new", "done"}}, want: true},
		{name: "not in", condition: models.WorkflowCondition{Field: "state", Operator: "not_in", Value: []interface{}{"new", "open"}}, want: true},
		{name: "contains string", condition: models.WorkflowCondition{Field: "name", Operator: "contains", Value: "invoice"}, want: true},
		{name: "contains array", condition: models.WorkflowCondition{Field: "tags", Operator: "contains", Value: "a"}, want: true},
		{name: "not contains", condition: models.WorkflowCondition{Field: "tags", Operator: "not_contains", Value: "b"}, want: true},
		{name: "starts with", condition: models.WorkflowCondition{Field: "name", Operator: "starts_with", Value: "invoice"}, want: true},
		{name: "ends with", condition: models.WorkflowCondition{Field: "name", Operator: "ends_with", Value: "001"}, want: true},
		{name: "is empty", condition: models.WorkflowCondition{Field: "empty", Operator: "is_empty"}, want: true},
		{name: "is not empty", condition: models.WorkflowCondition{Field: "name", Operator: "is_not_empty"}, want: true},
		{name: "between dates", condition: models.WorkflowCondition{Field: "dueDate", Operator: "between", Value: []interface{}{"2026-06-01", "2026-06-10"}}, want: true},
		{name: "expression multiply", condition: models.WorkflowCondition{Field: "total", Operator: "=", Value: "{{multiply(record.quantity, record.unitPrice)}}"}, want: true},
		{name: "expression subtract", condition: models.WorkflowCondition{Field: "discountedTotal", Operator: "=", Value: "{{subtract(record.total, record.discount)}}"}, want: true},
		{name: "and group", condition: models.WorkflowCondition{Operator: "and", Conditions: []models.WorkflowCondition{{Field: "status", Operator: "=", Value: "open"}, {Field: "user.role", Operator: "=", Value: "manager"}}}, want: true},
		{name: "or group", condition: models.WorkflowCondition{Operator: "or", Conditions: []models.WorkflowCondition{{Field: "status", Operator: "=", Value: "closed"}, {Field: "amount", Operator: "<", Value: 100}}}, want: true},
		{name: "exists", condition: models.WorkflowCondition{Field: "missing", Operator: "exists", Value: false}, want: true},
		{name: "changed", condition: models.WorkflowCondition{Field: "amount", Operator: "changed"}, want: true},
		{name: "changed to", condition: models.WorkflowCondition{Field: "state", Operator: "changed_to", Value: "done"}, want: true},
		{name: "changed from", condition: models.WorkflowCondition{Field: "state", Operator: "changed_from", Value: "new"}, want: true},
		{name: "unknown", condition: models.WorkflowCondition{Field: "state", Operator: "unknown"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := workflowConditionMatches(tt.condition, payload); got != tt.want {
				t.Fatalf("workflowConditionMatches() = %v, want %v", got, tt.want)
			}
		})
	}
	if !workflowConditionsMatch([]models.WorkflowCondition{{Field: "amount", Operator: ">", Value: 10}, {Field: "state", Value: "done"}}, payload) {
		t.Fatal("workflowConditionsMatch() = false")
	}
	if workflowConditionsMatch([]models.WorkflowCondition{{Operator: "or", Conditions: []models.WorkflowCondition{{Field: "status", Operator: "=", Value: "closed"}, {Field: "amount", Operator: "<", Value: 10}}}}, payload) {
		t.Fatal("workflowConditionsMatch(or false) = true")
	}
}

func TestWorkflowTemplatesAndPaths(t *testing.T) {
	payload := workflowExecutionPayload{
		SchemaName:   "orders",
		WorkflowName: "notify",
		UserID:       "user",
		Record:       map[string]interface{}{"amount": 3, "nested": map[string]interface{}{"name": "Ada"}},
		Query:        map[string]interface{}{"search": "coffee", "nested": map[string]interface{}{"term": "tea"}},
		Variables:    map[string]interface{}{"enabled": true},
	}
	if got := resolveWorkflowTemplateString("{{record.amount}}", payload); got != 3 {
		t.Fatalf("resolved scalar = %#v", got)
	}
	if got := resolveWorkflowTemplateString("order-{{record.amount}}-{{schemaName}}", payload); got != "order-3-orders" {
		t.Fatalf("resolved string = %#v", got)
	}
	if got := resolveWorkflowTemplates(map[string]interface{}{"enabled": "{{vars.enabled}}"}, payload); !reflect.DeepEqual(got, map[string]interface{}{"enabled": true}) {
		t.Fatalf("resolveWorkflowTemplates() = %#v", got)
	}
	if got := resolveWorkflowTemplateString("{{query.search}}", payload); got != "coffee" {
		t.Fatalf("resolved query scalar = %#v", got)
	}
	if got := resolveWorkflowTemplateString("{{query.nested.term}}", payload); got != "tea" {
		t.Fatalf("resolved nested query scalar = %#v", got)
	}
	if got := resolveWorkflowTemplateString("{{add(record.amount, 4)}}", payload); got != float64(7) {
		t.Fatalf("resolved add = %#v", got)
	}
	if got := resolveWorkflowTemplateString("{{subtract(record.amount, 1)}}", payload); got != float64(2) {
		t.Fatalf("resolved subtract = %#v", got)
	}
	if got, ok := workflowPathValue(payload.Record, "nested.name"); !ok || got != "Ada" {
		t.Fatalf("workflowPathValue() = %#v, %v", got, ok)
	}
}

func TestWorkflowFilterHelpers(t *testing.T) {
	container := &models.ContainerModel{Fields: []models.Field{{Name: "age", Type: "int"}, {Name: "secret", Type: "string", IsHashed: true}}}
	got, err := buildWorkflowFilterFromValues(container, map[string]interface{}{"age": []interface{}{1, 2}, "secret": "hidden", "unknown": "ignored"})
	want := bson.M{"age": bson.M{"$in": []interface{}{1, 2}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("buildWorkflowFilterFromValues() = %#v, %v; want %#v", got, err, want)
	}
	filter, configured, err := workflowFilterConfig(map[string]interface{}{"filters": map[string]interface{}{"age": "{{record.age}}"}}, workflowExecutionPayload{Record: map[string]interface{}{"age": 3}}, container)
	if err != nil || !configured || !reflect.DeepEqual(filter, bson.M{"age": 3}) {
		t.Fatalf("workflowFilterConfig() = %#v, %v, %v", filter, configured, err)
	}
}

func TestWorkflowMiscHelpers(t *testing.T) {
	container := &models.ContainerModel{Workflows: []models.DynamicWorkflow{{Name: "notify"}}}
	if _, ok := findWorkflow(container, "notify"); !ok {
		t.Fatal("findWorkflow() = false")
	}
	if workflowStepIdentifier(models.DynamicWorkflowStep{Name: "named"}) != "named" || workflowStepIdentifier(models.DynamicWorkflowStep{Type: "type", Order: 2}) != "type:2" {
		t.Fatal("workflowStepIdentifier() returned incorrect value")
	}
	if !workflowStepHasNonTransactionalSideEffect(models.DynamicWorkflowStep{Type: models.WorkflowStepTypeCallAPI}) {
		t.Fatal("workflowStepHasNonTransactionalSideEffect() = false")
	}
	if !workflowHasUpdateOperator(map[string]interface{}{"$set": bson.M{}}) || workflowHasUpdateOperator(map[string]interface{}{"name": "Ada"}) {
		t.Fatal("workflowHasUpdateOperator() returned incorrect value")
	}
	if workflowUserRole(workflowExecutionPayload{AuditUser: &models.AuditUser{Roles: []string{"admin"}}}) != "admin" {
		t.Fatal("workflowUserRole() returned incorrect value")
	}
}

func TestDynamicServiceLookupAndFormattingHelpers(t *testing.T) {
	id := primitive.NewObjectID()
	container := &models.ContainerModel{
		Fields:           []models.Field{{Name: "sequence", Type: "autoIncrementId"}, {Name: "email", Unique: true}},
		Pipelines:        []models.PipelineStage{{Name: "summary"}},
		DynamicFunctions: []models.DynamicFunction{{Name: "total", CodeJSON: "code"}},
		DynamicApis:      []models.DynamicApiModel{{Name: "status", Dependencies: []string{"id", "token"}}},
	}
	if got, err := buildGetDynamicItemFilter(container, "42"); err != nil || !reflect.DeepEqual(got, bson.M{"sequence": 42}) {
		t.Fatalf("buildGetDynamicItemFilter(sequence) = %#v, %v", got, err)
	}
	if got, err := buildGetDynamicItemFilter(container, id.Hex()); err != nil || !reflect.DeepEqual(got, bson.M{"_id": id}) {
		t.Fatalf("buildGetDynamicItemFilter(id) = %#v, %v", got, err)
	}
	if _, ok := findPipelineStage(container, "summary"); !ok {
		t.Fatal("findPipelineStage() = false")
	}
	if code, ok := findDynamicFunctionCode(container, "total"); !ok || code != "code" {
		t.Fatalf("findDynamicFunctionCode() = %q, %v", code, ok)
	}
	api, ok := findDynamicAPI(container, "status")
	if !ok || !reflect.DeepEqual(missingDynamicAPIDependencies(api, map[string]interface{}{"id": "1"}), []string{"token"}) {
		t.Fatal("dynamic API lookup or dependency detection failed")
	}
	if got := duplicateKeyFieldName(container, errors.New("duplicate key index: idx_email_unique")); got != "email" {
		t.Fatalf("duplicateKeyFieldName() = %q", got)
	}
}

func TestErrorMappingHelpers(t *testing.T) {
	limitErr := &requests.BatchLimitError{Operation: "bulk write", Requested: 2, Max: 1}
	if got := parseBulkCreateError(limitErr); got.Status != http.StatusBadRequest {
		t.Fatalf("parseBulkCreateError() = %#v", got)
	}
	if parseCreateErrorMessage(errors.New("multipart failure")) != "Error parsing form." || parseUpdateErrorMessage(errors.New("multipart failure")) != "Error in multipart form" {
		t.Fatal("multipart error mapping failed")
	}
	if pluginExecutionMessage(errors.New("load plugin: failure")) != "Failed to load new plugin" || pluginExecutionMessage(nil) != "Failed to execute function" {
		t.Fatal("pluginExecutionMessage() returned incorrect value")
	}
}

func TestExportHelpers(t *testing.T) {
	fields := []models.Field{{Name: "firstName"}, {Name: "tags", Type: "stringArray"}, {Name: "owner", PopulationSettings: &models.PopulationSettings{DisplayFields: []string{"name"}}}}
	container := &models.ContainerModel{Fields: fields}
	if got := exportColumnName(fields[0]); got != "First Name" {
		t.Fatalf("exportColumnName() = %q", got)
	}
	if got := exportCellValue(fields[1], []string{"a", "b"}); got != "a,b" {
		t.Fatalf("exportCellValue(array) = %#v", got)
	}
	if got := exportCellValue(fields[2], map[string]interface{}{"name": "Ada"}); got != "Ada" {
		t.Fatalf("exportCellValue(populated) = %#v", got)
	}
	if got := buildExportFilter(container, map[string]interface{}{"firstName": "Ada", "unknown": "ignored", "tags": ""}); !reflect.DeepEqual(got, bson.M{"firstName": "Ada"}) {
		t.Fatalf("buildExportFilter() = %#v", got)
	}
	if got := selectExportFields(container, []string{"tags"}); !reflect.DeepEqual(got, []models.Field{fields[1]}) {
		t.Fatalf("selectExportFields() = %#v", got)
	}
	if workbook, err := buildExportWorkbook(container, []map[string]interface{}{{"firstName": "Ada"}}, []string{"firstName"}); err != nil || len(workbook) == 0 {
		t.Fatalf("buildExportWorkbook() bytes = %d, error = %v", len(workbook), err)
	}
}

func TestCachedResponseAndBulkIDHelpers(t *testing.T) {
	items, ok := cachedResponseItems(fiber.Map{"items": []interface{}{map[string]interface{}{"id": "1"}, "ignored"}})
	if !ok || !reflect.DeepEqual(items, []map[string]interface{}{{"id": "1"}}) {
		t.Fatalf("cachedResponseItems() = %#v, %v", items, ok)
	}
	for _, tt := range []struct {
		item    map[string]interface{}
		wantID  string
		wantErr string
	}{
		{item: map[string]interface{}{"id": "1"}, wantID: "1"},
		{item: map[string]interface{}{"_id": "2"}, wantID: "2"},
		{item: map[string]interface{}{"id": 1}, wantErr: "Invalid id format, expected string"},
		{item: map[string]interface{}{}, wantErr: "Missing id field"},
	} {
		if id, err := extractBulkUpdateID(tt.item); id != tt.wantID || err != tt.wantErr {
			t.Fatalf("extractBulkUpdateID(%#v) = %q, %q", tt.item, id, err)
		}
	}
	if cacheValueFingerprint(nil) != "" || !strings.Contains(cacheValueFingerprint(map[string]interface{}{"id": 1}), `"id":1`) {
		t.Fatal("cacheValueFingerprint() returned incorrect value")
	}
	if cacheDuration(2) != 2*time.Minute {
		t.Fatal("cacheDuration() returned incorrect duration")
	}
}

func TestEnsureWorkflowPayloadMapsAndSlices(t *testing.T) {
	payload := workflowExecutionPayload{}
	ensureWorkflowPayloadMaps(&payload)
	if payload.Record != nil || payload.OldRecord != nil || payload.StepOutputs == nil || payload.Variables == nil || payload.Loop == nil {
		t.Fatalf("ensureWorkflowPayloadMaps() = %#v", payload)
	}
	for _, value := range []interface{}{
		[]interface{}{1},
		[]map[string]interface{}{{"id": "1"}},
		[]bson.M{{"id": "1"}},
	} {
		if got, ok := workflowSlice(value); !ok || len(got) != 1 {
			t.Fatalf("workflowSlice(%T) = %#v, %v", value, got, ok)
		}
	}
	if _, ok := workflowSlice("invalid"); ok {
		t.Fatal("workflowSlice(invalid) ok = true")
	}
}

func TestWorkflowBlockedOutboundIP(t *testing.T) {
	for _, input := range []string{"127.0.0.1", "10.0.0.1", "169.254.1.1", "0.0.0.0", "224.0.0.1"} {
		if !workflowBlockedOutboundIP(parseIP(input)) {
			t.Fatalf("workflowBlockedOutboundIP(%s) = false", input)
		}
	}
	if workflowBlockedOutboundIP(parseIP("8.8.8.8")) {
		t.Fatal("workflowBlockedOutboundIP(public) = true")
	}
}

func TestWorkflowSetVariableAndDefinition(t *testing.T) {
	service := &DynamicService{}
	payload := workflowExecutionPayload{
		Record:    map[string]interface{}{"amount": 3},
		Variables: map[string]interface{}{},
	}
	output, err := service.workflowSetVariable(models.DynamicWorkflowStep{Config: map[string]interface{}{
		"values": map[string]interface{}{"total": "{{record.amount}}"},
	}}, &payload)
	if err != nil || payload.Variables["total"] != 3 {
		t.Fatalf("workflowSetVariable() = %#v, %v; variables = %#v", output, err, payload.Variables)
	}

	workflow := models.DynamicWorkflow{
		Name: "ordered",
		Mode: models.WorkflowModeTransactional,
		Steps: []models.DynamicWorkflowStep{
			{Name: "second", Type: models.WorkflowStepTypeSetVariable, Order: 2, IsActive: true, Config: map[string]interface{}{"values": map[string]interface{}{"second": "{{vars.first}}"}}},
			{Name: "first", Type: models.WorkflowStepTypeSetVariable, Order: 1, IsActive: true, Config: map[string]interface{}{"values": map[string]interface{}{"first": "ready"}}},
		},
	}
	if err := service.runWorkflowDefinition(context.Background(), &payload, workflow); err != nil {
		t.Fatalf("runWorkflowDefinition() error = %v", err)
	}
	if payload.Variables["first"] != "ready" || payload.Variables["second"] != "ready" {
		t.Fatalf("runWorkflowDefinition() variables = %#v", payload.Variables)
	}
}

func TestWorkflowMutationPrimitivesAndExpressions(t *testing.T) {
	payload := workflowExecutionPayload{
		Record: map[string]interface{}{
			"quantity":     4,
			"unitPrice":    6,
			"discountNote": []interface{}{"first", "second"},
			"items":        []interface{}{"a", "b"},
		},
		Variables: map[string]interface{}{},
		Loop: map[string]interface{}{
			"ingredient": map[string]interface{}{"quantity": 3},
		},
		StepOutputs: map[string]interface{}{
			"discount": map[string]interface{}{"data": map[string]interface{}{"amount": 20}},
		},
	}

	if _, err := workflowSetRecord(models.DynamicWorkflowStep{
		Type: models.WorkflowStepTypeSetRecord,
		Config: map[string]interface{}{"values": map[string]interface{}{
			"status":       "{{default(record.status, \"pending\")}}",
			"createdAt":    "{{now}}",
			"discountNote": "{{join(record.discountNote, \",\")}}",
		}},
	}, &payload); err != nil {
		t.Fatalf("workflowSetRecord() error = %v", err)
	}
	if payload.Record["status"] != "pending" || payload.Record["discountNote"] != "first,second" {
		t.Fatalf("workflowSetRecord() record = %#v", payload.Record)
	}
	if _, ok := payload.Record["createdAt"].(time.Time); !ok {
		t.Fatalf("workflowSetRecord() createdAt = %#v", payload.Record["createdAt"])
	}

	if _, err := workflowTransform(models.DynamicWorkflowStep{
		Type: models.WorkflowStepTypeTransform,
		Config: map[string]interface{}{"values": map[string]interface{}{
			"discountPerUnit": "{{divide(steps.discount.data.amount, record.quantity)}}",
			"consumption":     "{{multiply(loop.ingredient.quantity, record.quantity)}}",
			"lastItem":        "{{last(record.items)}}",
			"itemCount":       "{{length(record.items)}}",
		}},
	}, &payload); err != nil {
		t.Fatalf("workflowTransform() error = %v", err)
	}
	if _, err := workflowTransform(models.DynamicWorkflowStep{
		Type: models.WorkflowStepTypeTransform,
		Config: map[string]interface{}{"values": map[string]interface{}{
			"discountAmount": "{{min(vars.discountPerUnit, record.unitPrice)}}",
		}},
	}, &payload); err != nil {
		t.Fatalf("workflowTransform(dependent) error = %v", err)
	}
	wantVariables := map[string]interface{}{
		"discountPerUnit": float64(5),
		"discountAmount":  float64(5),
		"consumption":     float64(12),
		"lastItem":        "b",
		"itemCount":       2,
	}
	if !reflect.DeepEqual(payload.Variables, wantVariables) {
		t.Fatalf("workflowTransform() variables = %#v, want %#v", payload.Variables, wantVariables)
	}
	payload.Record["price"] = 2.5
	if _, err := workflowEquation(models.DynamicWorkflowStep{
		Type: models.WorkflowStepTypeEquation,
		Config: map[string]interface{}{
			"target": "record",
			"values": map[string]interface{}{
				"total": "price * quantity",
			},
		},
	}, &payload); err != nil {
		t.Fatalf("workflowEquation(record) error = %v", err)
	}
	if payload.Record["total"] != float64(10) {
		t.Fatalf("workflowEquation(record) total = %#v", payload.Record["total"])
	}
	if _, err := workflowEquation(models.DynamicWorkflowStep{
		Type: models.WorkflowStepTypeEquation,
		Config: map[string]interface{}{
			"values": map[string]interface{}{
				"adjustedTotal": "{{vars.discountPerUnit}} * quantity",
			},
		},
	}, &payload); err != nil {
		t.Fatalf("workflowEquation(vars) error = %v", err)
	}
	if payload.Variables["adjustedTotal"] != float64(20) {
		t.Fatalf("workflowEquation(vars) adjustedTotal = %#v", payload.Variables["adjustedTotal"])
	}

	err := workflowFail(models.DynamicWorkflowStep{Config: map[string]interface{}{"message": "invalid quantity", "status": 422}}, payload)
	mapped := workflowExecutionServiceError(err, "fallback")
	if mapped.Status != 422 || mapped.Message != "invalid quantity" {
		t.Fatalf("workflowExecutionServiceError() = %#v", mapped)
	}
}

func TestWorkflowSetPath(t *testing.T) {
	target := map[string]interface{}{}
	if err := workflowSetPath(target, "customer.address.city", "Chicago"); err != nil {
		t.Fatalf("workflowSetPath() error = %v", err)
	}
	if got, ok := workflowPathValue(target, "customer.address.city"); !ok || got != "Chicago" {
		t.Fatalf("workflowPathValue() = %#v, %v", got, ok)
	}
	if err := workflowSetPath(target, "$bad", "value"); err == nil {
		t.Fatal("workflowSetPath($bad) error = nil")
	}
}

func TestWorkflowIfAndForEach(t *testing.T) {
	service := &DynamicService{}
	payload := workflowExecutionPayload{
		WorkflowName:  "branching",
		WorkflowMode:  models.WorkflowModeTransactional,
		ExecutionMode: models.WorkflowModeTransactional,
		Record:        map[string]interface{}{"state": "closed", "items": []interface{}{"a", "b"}},
		Variables:     map[string]interface{}{},
	}
	setBranch := func(value string) models.DynamicWorkflowStep {
		return models.DynamicWorkflowStep{
			Type:     models.WorkflowStepTypeSetVariable,
			IsActive: true,
			Config:   map[string]interface{}{"values": map[string]interface{}{"branch": value}},
		}
	}
	output, err := service.workflowIf(context.Background(), models.DynamicWorkflowStep{
		Conditions: []models.WorkflowCondition{{Field: "state", Value: "open"}},
		Steps:      []models.DynamicWorkflowStep{setBranch("if")},
		ElseSteps:  []models.DynamicWorkflowStep{setBranch("else")},
	}, &payload)
	if err != nil || !reflect.DeepEqual(output, map[string]interface{}{"branch": "else"}) || payload.Variables["branch"] != "else" {
		t.Fatalf("workflowIf() = %#v, %v; variables = %#v", output, err, payload.Variables)
	}

	output, err = service.workflowForEach(context.Background(), models.DynamicWorkflowStep{
		Config: map[string]interface{}{"items": "{{record.items}}"},
	}, &payload)
	if err != nil || !reflect.DeepEqual(output, map[string]interface{}{
		"count": 2,
		"items": []interface{}{map[string]interface{}{"index": 0}, map[string]interface{}{"index": 1}},
	}) {
		t.Fatalf("workflowForEach() = %#v, %v", output, err)
	}
	payload.Record["mongoItems"] = primitive.A{
		map[string]interface{}{"index": 0},
		map[string]interface{}{"index": 1},
	}
	output, err = service.workflowForEach(context.Background(), models.DynamicWorkflowStep{
		Config: map[string]interface{}{"items": "{{record.mongoItems}}"},
	}, &payload)
	if err != nil || !reflect.DeepEqual(output, map[string]interface{}{
		"count": 2,
		"items": []interface{}{map[string]interface{}{"index": 0}, map[string]interface{}{"index": 1}},
	}) {
		t.Fatalf("workflowForEach(primitive.A) = %#v, %v", output, err)
	}
	if _, err := service.workflowForEach(context.Background(), models.DynamicWorkflowStep{Config: map[string]interface{}{"items": "invalid"}}, &payload); err == nil {
		t.Fatal("workflowForEach(invalid items) error = nil")
	}
	if _, err := service.workflowForEach(context.Background(), models.DynamicWorkflowStep{Config: map[string]interface{}{"items": []interface{}{}, "maxItems": 0}}, &payload); err == nil {
		t.Fatal("workflowForEach(invalid maxItems) error = nil")
	}
}

func TestWorkflowStepProcessingErrors(t *testing.T) {
	service := &DynamicService{}
	payload := workflowExecutionPayload{Variables: map[string]interface{}{}}
	if _, err := service.processWorkflowStep(context.Background(), models.DynamicWorkflowStep{Type: "unknown"}, &payload); err == nil {
		t.Fatal("processWorkflowStep(unknown) error = nil")
	}
	if _, err := service.processWorkflowStep(context.Background(), models.DynamicWorkflowStep{Type: models.WorkflowStepTypeDynamicFunction}, &payload); err == nil {
		t.Fatal("processWorkflowStep(dynamic function) error = nil")
	}
	if _, err := service.processWorkflowStepForMode(context.Background(), models.DynamicWorkflowStep{Type: models.WorkflowStepTypeCallAPI}, &payload, models.WorkflowModeTransactional); err == nil {
		t.Fatal("processWorkflowStepForMode(side effect) error = nil")
	}
	err := service.runWorkflowStepList(context.Background(), &payload, "workflow", models.WorkflowModeTransactional, models.WorkflowModeTransactional, true, []models.DynamicWorkflowStep{
		{Name: "bad", Type: "unknown", IsActive: true},
	})
	if err == nil || !strings.Contains(err.Error(), "workflow workflow step bad failed") {
		t.Fatalf("runWorkflowStepList() error = %v", err)
	}
	if err := service.runWorkflowStepList(context.Background(), &payload, "workflow", models.WorkflowModeTransactional, models.WorkflowModeTransactional, true, []models.DynamicWorkflowStep{
		{Name: "ignored", Type: "unknown", IsActive: true, ContinueOnError: true},
	}); err != nil {
		t.Fatalf("runWorkflowStepList(continue) error = %v", err)
	}
}

func TestValidateWorkflowCallAPIURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "public IP", url: "https://8.8.8.8/path"},
		{name: "loopback", url: "http://127.0.0.1/path", wantErr: true},
		{name: "private IP", url: "http://10.0.0.1/path", wantErr: true},
		{name: "localhost", url: "http://localhost/path", wantErr: true},
		{name: "unsupported scheme", url: "ftp://8.8.8.8/path", wantErr: true},
		{name: "missing host", url: "https:///path", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateWorkflowCallAPIURL(context.Background(), tt.url); (err != nil) != tt.wantErr {
				t.Fatalf("validateWorkflowCallAPIURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestWorkflowCallAPIEarlyValidation(t *testing.T) {
	service := &DynamicService{}
	if _, err := service.workflowCallAPI(context.Background(), models.DynamicWorkflowStep{}, workflowExecutionPayload{}); err == nil {
		t.Fatal("workflowCallAPI(missing timeout) error = nil")
	}
	if _, err := service.workflowCallAPI(context.Background(), models.DynamicWorkflowStep{TimeoutSec: 1}, workflowExecutionPayload{}); err == nil {
		t.Fatal("workflowCallAPI(missing URL) error = nil")
	}
	if _, err := service.workflowCallAPI(context.Background(), models.DynamicWorkflowStep{
		TimeoutSec: 1,
		Config:     map[string]interface{}{"url": "http://127.0.0.1"},
	}, workflowExecutionPayload{}); err == nil {
		t.Fatal("workflowCallAPI(blocked URL) error = nil")
	}
}

func TestWorkflowResolvedStringConfigResolvesURLTemplates(t *testing.T) {
	payload := workflowExecutionPayload{
		StepOutputs: map[string]interface{}{
			"order": map[string]interface{}{
				"data": map[string]interface{}{
					"_id": "6a5987d76b8ce5ece558e176",
				},
			},
		},
	}

	got := workflowResolvedStringConfig(map[string]interface{}{
		"url": "https://api.example.com/order/{{steps.order.data._id}}/status",
	}, "url", payload)
	want := "https://api.example.com/order/6a5987d76b8ce5ece558e176/status"
	if got != want {
		t.Fatalf("workflowResolvedStringConfig(url) = %q, want %q", got, want)
	}
}

func TestWorkflowLogPreview(t *testing.T) {
	if got := workflowLogPreview([]byte("davinci response"), 100); got != "davinci response" {
		t.Fatalf("workflowLogPreview(short) = %q", got)
	}
	got := workflowLogPreview([]byte("1234567890"), 4)
	if got != "1234...(truncated 6 bytes)" {
		t.Fatalf("workflowLogPreview(truncated) = %q", got)
	}
}

func TestWorkflowCallAPIStatusError(t *testing.T) {
	if err := workflowCallAPIStatusError(http.StatusCreated, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("workflowCallAPIStatusError(201) error = %v", err)
	}
	err := workflowCallAPIStatusError(http.StatusBadRequest, []byte(`{"message":"invalid order"}`))
	if err == nil {
		t.Fatal("workflowCallAPIStatusError(400) error = nil")
	}
	if !strings.Contains(err.Error(), "status 400") || !strings.Contains(err.Error(), "invalid order") {
		t.Fatalf("workflowCallAPIStatusError(400) = %q, want status and response body", err.Error())
	}
}

func TestWorkflowExternalAPICredentialProtectedHeaders(t *testing.T) {
	credential := models.ExternalAPICredential{
		AuthType:       models.ExternalAPIAuthTypeBearer,
		AllowedDomains: []string{"api.example.com"},
	}

	headers, err := workflowExternalAPICredentialProtectedHeaders(credential, "token-value", "api.example.com", time.Now())
	if err != nil {
		t.Fatalf("workflowExternalAPICredentialProtectedHeaders() error = %v", err)
	}
	if got := headers["Authorization"]; got != "Bearer token-value" {
		t.Fatalf("Authorization = %q, want bearer token", got)
	}
}

func TestWorkflowExternalAPICredentialProtectedHeadersRejectsInvalidCredential(t *testing.T) {
	now := time.Now()
	revokedAt := now.Add(-time.Minute)
	tests := []struct {
		name       string
		credential models.ExternalAPICredential
		host       string
	}{
		{
			name:       "revoked",
			credential: models.ExternalAPICredential{AuthType: models.ExternalAPIAuthTypeBearer, AllowedDomains: []string{"api.example.com"}, RevokedAt: &revokedAt},
			host:       "api.example.com",
		},
		{
			name:       "expired",
			credential: models.ExternalAPICredential{AuthType: models.ExternalAPIAuthTypeBearer, AllowedDomains: []string{"api.example.com"}, ExpiresAt: now.Add(-time.Minute)},
			host:       "api.example.com",
		},
		{
			name:       "wrong domain",
			credential: models.ExternalAPICredential{AuthType: models.ExternalAPIAuthTypeBearer, AllowedDomains: []string{"api.example.com"}},
			host:       "evil.example.net",
		},
		{
			name:       "custom authorization header",
			credential: models.ExternalAPICredential{AuthType: models.ExternalAPIAuthTypeHeader, HeaderName: "Authorization", AllowedDomains: []string{"api.example.com"}},
			host:       "api.example.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := workflowExternalAPICredentialProtectedHeaders(tt.credential, "secret", tt.host, now); err == nil {
				t.Fatal("workflowExternalAPICredentialProtectedHeaders() error = nil")
			}
		})
	}
}

func TestWorkflowExternalAPICredentialAllowsSubdomainByParsedHost(t *testing.T) {
	credential := models.ExternalAPICredential{
		AuthType:       models.ExternalAPIAuthTypeHeader,
		HeaderName:     "X-API-Key",
		AllowedDomains: []string{"example.com"},
	}
	headers, err := workflowExternalAPICredentialProtectedHeaders(credential, "secret", "orders.example.com", time.Now())
	if err != nil {
		t.Fatalf("workflowExternalAPICredentialProtectedHeaders(subdomain) error = %v", err)
	}
	if got := headers["X-API-Key"]; got != "secret" {
		t.Fatalf("X-API-Key = %q, want secret", got)
	}
	if _, ok := headers["Authorization"]; ok {
		t.Fatalf("Authorization header was set for custom header auth: %#v", headers)
	}
}

func TestWorkflowMissingRequiredFieldPathsIncludesArrayIndexes(t *testing.T) {
	fields := []models.Field{
		{
			Name: "product",
			Type: "array",
			Children: []models.Field{
				{Name: "productId", Type: "objectId", Tag: "required"},
				{Name: "productDavinciId", Type: "int", Tag: "required"},
			},
		},
	}
	document := map[string]interface{}{
		"product": []interface{}{
			map[string]interface{}{"productId": "p1", "productDavinciId": 181},
			map[string]interface{}{"productId": "p2"},
		},
	}

	got := workflowMissingRequiredFieldPaths(fields, document, "")
	want := []string{"product[1].productDavinciId"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("workflowMissingRequiredFieldPaths() = %#v, want %#v", got, want)
	}
}

func TestWorkflowArrayFunctionWrapsResolvedValue(t *testing.T) {
	payload := workflowExecutionPayload{
		StepOutputs: map[string]interface{}{
			"createOrder": map[string]interface{}{"_id": "order-1", "status": "pending"},
		},
	}

	got := resolveWorkflowTemplates(map[string]interface{}{
		"orders": "{{array(steps.createOrder)}}",
	}, payload)
	want := map[string]interface{}{
		"orders": []interface{}{map[string]interface{}{"_id": "order-1", "status": "pending"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveWorkflowTemplates(array) = %#v, want %#v", got, want)
	}
}

func TestValidateWorkflowDefinitions(t *testing.T) {
	valid := models.DynamicWorkflow{
		Name:    "notify",
		Trigger: models.WorkflowTriggerAfterCreate,
		Mode:    models.WorkflowModeOutbox,
		Steps: []models.DynamicWorkflowStep{{
			Name:          "send",
			Type:          models.WorkflowStepTypeCallAPI,
			IsActive:      true,
			TimeoutSec:    1,
			ExecutionMode: models.WorkflowModeOutbox,
			Config:        map[string]interface{}{"url": "https://example.com"},
		}},
	}
	if err := ValidateWorkflow(valid); err != nil {
		t.Fatalf("ValidateWorkflow(valid) error = %v", err)
	}
	if err := ValidateWorkflow(models.DynamicWorkflow{
		Name:    "create-davinci-order",
		Trigger: models.WorkflowTriggerManual,
		Mode:    models.WorkflowModeHybrid,
		Steps: []models.DynamicWorkflowStep{
			{
				Name:          "createOrder",
				Type:          models.WorkflowStepTypeCreateRecord,
				IsActive:      true,
				ExecutionMode: models.WorkflowModeTransactional,
			},
			{
				Name:          "sendDavinciOrder",
				Type:          models.WorkflowStepTypeCallAPI,
				IsActive:      true,
				ExecutionMode: models.WorkflowModeOutbox,
				TimeoutSec:    10,
				Config: map[string]interface{}{
					"url": "https://example.com/orders",
					"body": map[string]interface{}{
						"orders": []interface{}{"{{steps.createOrder}}"},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("ValidateWorkflow(manual hybrid outbox dependency) error = %v", err)
	}
	if err := ValidateWorkflow(models.DynamicWorkflow{
		Name: "return-before-outbox",
		Mode: models.WorkflowModeHybrid,
		Steps: []models.DynamicWorkflowStep{
			{Name: "response", Type: models.WorkflowStepTypeReturn, Order: 1, IsActive: true, ExecutionMode: models.WorkflowModeTransactional},
			{Name: "notify", Type: models.WorkflowStepTypeAuditLog, Order: 2, IsActive: true, ExecutionMode: models.WorkflowModeOutbox},
		},
	}); err != nil {
		t.Fatalf("ValidateWorkflow(return before outbox) error = %v", err)
	}
	if err := ValidateWorkflow(models.DynamicWorkflow{
		Name: "unset-finish-hour",
		Steps: []models.DynamicWorkflowStep{{
			Name: "unset", Type: models.WorkflowStepTypeUnsetRecord, Config: map[string]interface{}{"field": "finishHour"},
		}},
	}); err != nil {
		t.Fatalf("ValidateWorkflow(unset record) error = %v", err)
	}
	if err := ValidateWorkflow(models.DynamicWorkflow{
		Name: "aggregate-totals",
		Steps: []models.DynamicWorkflowStep{{
			Name: "totals", Type: models.WorkflowStepTypeAggregate, Config: map[string]interface{}{"pipeline": []interface{}{map[string]interface{}{"$match": map[string]interface{}{"state": "open"}}}},
		}},
	}); err != nil {
		t.Fatalf("ValidateWorkflow(aggregate) error = %v", err)
	}
	if err := ValidateWorkflow(models.DynamicWorkflow{
		Name: "distinct-states",
		Steps: []models.DynamicWorkflowStep{{
			Name: "states", Type: models.WorkflowStepTypeDistinct, Config: map[string]interface{}{"field": "state"},
		}},
	}); err != nil {
		t.Fatalf("ValidateWorkflow(distinct) error = %v", err)
	}

	tests := []struct {
		name     string
		workflow models.DynamicWorkflow
	}{
		{
			name: "hybrid requires explicit step mode",
			workflow: models.DynamicWorkflow{
				Name: "hybrid", Mode: models.WorkflowModeHybrid,
				Steps: []models.DynamicWorkflowStep{{Name: "set", Type: models.WorkflowStepTypeSetVariable}},
			},
		},
		{
			name: "outbox rejects prior step dependency",
			workflow: models.DynamicWorkflow{
				Name: "dependent", Mode: models.WorkflowModeOutbox,
				Steps: []models.DynamicWorkflowStep{{Name: "set", Type: models.WorkflowStepTypeSetVariable, Config: map[string]interface{}{"values": map[string]interface{}{"id": "{{steps.create._id}}"}}}},
			},
		},
		{
			name: "changed requires update trigger",
			workflow: models.DynamicWorkflow{
				Name: "changed", Trigger: models.WorkflowTriggerAfterCreate,
				Conditions: []models.WorkflowCondition{{Field: "status", Operator: models.WorkflowConditionChanged}},
			},
		},
		{
			name: "side effect requires outbox",
			workflow: models.DynamicWorkflow{
				Name:  "api",
				Steps: []models.DynamicWorkflowStep{{Name: "send", Type: models.WorkflowStepTypeCallAPI, TimeoutSec: 1, Config: map[string]interface{}{"url": "https://example.com"}}},
			},
		},
		{
			name: "return step must exist",
			workflow: models.DynamicWorkflow{
				Name:       "missing-return",
				ReturnStep: "missing",
				Steps:      []models.DynamicWorkflowStep{{Name: "set", Type: models.WorkflowStepTypeSetVariable}},
			},
		},
		{
			name: "aggregate requires pipeline",
			workflow: models.DynamicWorkflow{
				Name:  "missing-pipeline",
				Steps: []models.DynamicWorkflowStep{{Name: "aggregate", Type: models.WorkflowStepTypeAggregate}},
			},
		},
		{
			name: "distinct requires field",
			workflow: models.DynamicWorkflow{
				Name:  "missing-distinct-field",
				Steps: []models.DynamicWorkflowStep{{Name: "distinct", Type: models.WorkflowStepTypeDistinct}},
			},
		},
		{
			name: "return step requires transactional mode",
			workflow: models.DynamicWorkflow{
				Name: "async-return",
				Mode: models.WorkflowModeOutbox,
				Steps: []models.DynamicWorkflowStep{{
					Name: "response", Type: models.WorkflowStepTypeReturn,
				}},
			},
		},
		{
			name: "unconditional return must be terminal",
			workflow: models.DynamicWorkflow{
				Name: "return-then-write",
				Steps: []models.DynamicWorkflowStep{
					{Name: "response", Type: models.WorkflowStepTypeReturn, Order: 1, IsActive: true},
					{Name: "write", Type: models.WorkflowStepTypeSetVariable, Order: 2, IsActive: true},
				},
			},
		},
		{
			name: "unsafe update operator rejected",
			workflow: models.DynamicWorkflow{
				Name: "unsafe-update",
				Steps: []models.DynamicWorkflowStep{{
					Name: "rename", Type: models.WorkflowStepTypeUpdateRecord,
					Config: map[string]interface{}{"update": map[string]interface{}{"$rename": map[string]interface{}{"old": "new"}}},
				}},
			},
		},
		{
			name: "unset record requires config",
			workflow: models.DynamicWorkflow{
				Name:  "invalid-unset",
				Steps: []models.DynamicWorkflowStep{{Name: "unset", Type: models.WorkflowStepTypeUnsetRecord}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateWorkflow(tt.workflow); err == nil {
				t.Fatal("ValidateWorkflow() error = nil")
			}
		})
	}

	if err := ValidateWorkflows([]models.DynamicWorkflow{{Name: "same"}, {Name: "same"}}); err == nil {
		t.Fatal("ValidateWorkflows(duplicate names) error = nil")
	}
}

func TestValidateWorkflowCronSchedule(t *testing.T) {
	valid := models.DynamicWorkflow{
		Name:     "daily",
		Trigger:  models.WorkflowTriggerCron,
		Schedule: "0 2 * * *",
		Timezone: "UTC",
		Mode:     models.WorkflowModeOutbox,
	}
	if err := ValidateWorkflow(valid); err != nil {
		t.Fatalf("ValidateWorkflow(valid cron) error = %v", err)
	}

	tests := []models.DynamicWorkflow{
		{Name: "missing schedule", Trigger: models.WorkflowTriggerCron},
		{Name: "bad schedule", Trigger: models.WorkflowTriggerCron, Schedule: "bad"},
		{Name: "bad timezone", Trigger: models.WorkflowTriggerCron, Schedule: "0 2 * * *", Timezone: "No/SuchZone"},
		{Name: "non cron with schedule", Trigger: models.WorkflowTriggerManual, Schedule: "0 2 * * *"},
		{Name: "non cron with timezone", Trigger: models.WorkflowTriggerManual, Timezone: "UTC"},
	}
	for _, workflow := range tests {
		t.Run(workflow.Name, func(t *testing.T) {
			if err := ValidateWorkflow(workflow); err == nil {
				t.Fatal("ValidateWorkflow() error = nil")
			}
		})
	}
}

func TestWorkflowCallAPIRestrictions(t *testing.T) {
	if method, err := workflowAllowedHTTPMethod(" patch "); err != nil || method != http.MethodPatch {
		t.Fatalf("workflowAllowedHTTPMethod() = %q, %v", method, err)
	}
	if _, err := workflowAllowedHTTPMethod("TRACE"); err == nil {
		t.Fatal("workflowAllowedHTTPMethod(TRACE) error = nil")
	}

	t.Setenv("WORKFLOW_CALL_API_ALLOWED_DOMAINS", "example.com, api.test")
	if !workflowCallAPIHostAllowed("orders.example.com") {
		t.Fatal("workflowCallAPIHostAllowed(subdomain) = false")
	}
	if workflowCallAPIHostAllowed("example.net") {
		t.Fatal("workflowCallAPIHostAllowed(unlisted) = true")
	}
	if err := validateWorkflowCallAPIURL(context.Background(), "https://8.8.8.8/path"); err == nil {
		t.Fatal("validateWorkflowCallAPIURL(unlisted host) error = nil")
	}
}

func TestDynamicServiceParserWrappers(t *testing.T) {
	service := NewDynamicService()
	container := &models.ContainerModel{Fields: []models.Field{{Name: "age", Type: "int"}}}
	app := fiber.New()
	app.Get("/query", func(c *fiber.Ctx) error {
		search, err := service.ParseSearchParams(c)
		if err != nil || search.SearchKey != "Ada" || search.Pager.Page != 2 {
			t.Fatalf("ParseSearchParams() = %#v, %v", search, err)
		}
		filter, err := service.ParseFilterParams(c, container)
		if err != nil || !reflect.DeepEqual(filter.Filter, bson.M{"age": 42}) {
			t.Fatalf("ParseFilterParams() = %#v, %v", filter, err)
		}
		paginated, err := service.ParsePaginatedItemsParams(c, container)
		if err != nil || paginated.QueryString == "" {
			t.Fatalf("ParsePaginatedItemsParams() = %#v, %v", paginated, err)
		}
		return nil
	})
	app.Get("/pipeline", func(c *fiber.Ctx) error {
		got := service.ParsePipelineParams(c)
		if got.SchemaName != "orders" || got.PipelineName != "summary" || got.CurrentQuery == "" {
			t.Fatalf("ParsePipelineParams() = %#v", got)
		}
		return nil
	})
	app.Post("/test-pipeline", func(c *fiber.Ctx) error {
		got, err := service.ParseTestPipeline(c)
		if err != nil || got.PipelineStage.Name != "summary" {
			t.Fatalf("ParseTestPipeline() = %#v, %v", got, err)
		}
		return nil
	})
	app.Post("/api", func(c *fiber.Ctx) error {
		got, err := service.ParseDynamicAPIRequest(c)
		if err != nil || got["name"] != "Ada" {
			t.Fatalf("ParseDynamicAPIRequest() = %#v, %v", got, err)
		}
		return nil
	})
	app.Post("/export", func(c *fiber.Ctx) error {
		got, err := service.ParseExportRequest(c)
		if err != nil || got.SchemaName != "orders" || !reflect.DeepEqual(got.Fields, []string{"name"}) {
			t.Fatalf("ParseExportRequest() = %#v, %v", got, err)
		}
		return nil
	})

	requests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/query?search=Ada&page=2&limit=5&age=42"},
		{method: http.MethodGet, path: "/pipeline?schemaName=orders&pipelineName=summary"},
		{method: http.MethodPost, path: "/test-pipeline", body: `{"pipelineStage":{"name":"summary"}}`},
		{method: http.MethodPost, path: "/api", body: `{"name":"Ada"}`},
		{method: http.MethodPost, path: "/export", body: `{"schemaName":"orders","fields":["name"]}`},
	}
	for _, tt := range requests {
		req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
		if tt.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if _, err := app.Test(req); err != nil {
			t.Fatalf("app.Test(%q) error = %v", tt.path, err)
		}
	}
}

func TestDynamicServiceEarlyValidationPaths(t *testing.T) {
	service := NewDynamicService()
	ctx := context.Background()
	container := &models.ContainerModel{SchemaName: "orders"}

	if _, err := service.GetAllDynamicItems(ctx, GetAllDynamicItemsInput{}); serviceErrorStatus(err) != http.StatusBadRequest {
		t.Fatalf("GetAllDynamicItems() error = %v", err)
	}
	if _, err := service.GetItemsForSelection(ctx, GetItemsForSelectionInput{}); serviceErrorStatus(err) != http.StatusBadRequest {
		t.Fatalf("GetItemsForSelection() error = %v", err)
	}
	if _, err := service.GetDynamicItem(ctx, GetDynamicItemInput{Container: container, ID: "invalid"}); serviceErrorStatus(err) != http.StatusInternalServerError {
		t.Fatalf("GetDynamicItem() error = %v", err)
	}
	if got, err := service.SearchDynamicItems(ctx, SearchDynamicItemsInput{Container: container, SearchKey: "missing"}); err != nil || !reflect.DeepEqual(got, []interface{}{}) {
		t.Fatalf("SearchDynamicItems() = %#v, %v", got, err)
	}
	if got, err := service.GetAllDynamicItemsWithPagination(ctx, GetPaginatedDynamicItemsInput{
		Container: container,
		SearchKey: "missing",
		Pager:     utils.Pager{Enabled: true, Page: 2, Limit: 10},
	}); err != nil || !reflect.DeepEqual(got, fiber.Map{"items": []interface{}{}, "totalItems": 0, "totalPages": 0, "currentPage": 2}) {
		t.Fatalf("GetAllDynamicItemsWithPagination() = %#v, %v", got, err)
	}
	if _, err := service.GetPipeline(ctx, GetPipelineInput{}); serviceErrorStatus(err) != http.StatusBadRequest {
		t.Fatalf("GetPipeline(missing params) error = %v", err)
	}
	if _, err := service.GetPipeline(ctx, GetPipelineInput{Schema: "orders", PipelineName: "missing", Container: container}); serviceErrorStatus(err) != http.StatusNotFound {
		t.Fatalf("GetPipeline(missing stage) error = %v", err)
	}
	if _, err := service.ExecuteDynamicCode(ctx, ExecuteDynamicCodeInput{Container: container, FunctionName: "missing"}); serviceErrorStatus(err) != http.StatusBadRequest {
		t.Fatalf("ExecuteDynamicCode() error = %v", err)
	}
	if _, err := service.ExecuteDynamicAPI(ctx, ExecuteDynamicAPIInput{Container: container, APIName: "missing"}); serviceErrorStatus(err) != http.StatusBadRequest {
		t.Fatalf("ExecuteDynamicAPI(missing API) error = %v", err)
	}
	apiContainer := &models.ContainerModel{SchemaName: "orders", DynamicApis: []models.DynamicApiModel{{Name: "notify", Dependencies: []string{"id"}}}}
	if _, err := service.ExecuteDynamicAPI(ctx, ExecuteDynamicAPIInput{Container: apiContainer, APIName: "notify", Body: map[string]interface{}{}}); serviceErrorStatus(err) != http.StatusBadRequest {
		t.Fatalf("ExecuteDynamicAPI(missing dependencies) error = %v", err)
	}
	if _, err := service.ExportDynamicItems(ctx, ExportDynamicItemsInput{}); serviceErrorStatus(err) != http.StatusBadRequest {
		t.Fatalf("ExportDynamicItems() error = %v", err)
	}
}

func TestExecuteWorkflowValidationAndSuccess(t *testing.T) {
	service := NewDynamicService()
	ctx := context.Background()
	transactionCalled := false
	service.runTransaction = func(ctx context.Context, fn func(mongo.SessionContext) error) error {
		transactionCalled = true
		return fn(mongo.NewSessionContext(ctx, nil))
	}
	container := &models.ContainerModel{
		SchemaName: "orders",
		Workflows: []models.DynamicWorkflow{
			{Name: "disabled"},
			{
				Name:             "active",
				IsActive:         true,
				Mode:             models.WorkflowModeTransactional,
				RunInTransaction: true,
				ReturnStep:       "set",
				Steps: []models.DynamicWorkflowStep{{
					Name:     "set",
					Type:     models.WorkflowStepTypeSetVariable,
					IsActive: true,
					Config:   map[string]interface{}{"values": map[string]interface{}{"state": "done"}},
				}},
			},
			{
				Name:       "read-only",
				IsActive:   true,
				Mode:       models.WorkflowModeTransactional,
				ReturnStep: "set",
				Steps: []models.DynamicWorkflowStep{{
					Name:     "set",
					Type:     models.WorkflowStepTypeSetVariable,
					IsActive: true,
					Config:   map[string]interface{}{"values": map[string]interface{}{"state": "read"}},
				}},
			},
		},
	}
	if _, err := service.ExecuteWorkflow(ctx, ExecuteWorkflowInput{Container: container, WorkflowName: "missing"}); serviceErrorStatus(err) != http.StatusNotFound {
		t.Fatalf("ExecuteWorkflow(missing) error = %v", err)
	}
	if _, err := service.ExecuteWorkflow(ctx, ExecuteWorkflowInput{Container: container, WorkflowName: "disabled"}); serviceErrorStatus(err) != http.StatusForbidden {
		t.Fatalf("ExecuteWorkflow(disabled) error = %v", err)
	}
	got, err := service.ExecuteWorkflow(ctx, ExecuteWorkflowInput{Container: container, WorkflowName: "active"})
	if err != nil || !transactionCalled || got.Message != "Workflow executed successfully" || got.Source != "workflow" || !reflect.DeepEqual(got.Data, map[string]interface{}{"state": "done"}) {
		t.Fatalf("ExecuteWorkflow(active) = %#v, %v", got, err)
	}
	transactionCalled = false
	got, err = service.ExecuteWorkflow(ctx, ExecuteWorkflowInput{Container: container, WorkflowName: "read-only"})
	if err != nil || transactionCalled || !reflect.DeepEqual(got.Data, map[string]interface{}{"state": "read"}) {
		t.Fatalf("ExecuteWorkflow(read-only) = %#v, %v; transactionCalled = %v", got, err, transactionCalled)
	}
}

func TestWorkflowReturnValueAndNestedIsolation(t *testing.T) {
	outputs := map[string]interface{}{"first": 1, "selected": 2}
	if got := workflowReturnValue(models.DynamicWorkflow{}, outputs); !reflect.DeepEqual(got, outputs) {
		t.Fatalf("workflowReturnValue(legacy) = %#v", got)
	}
	if got := workflowReturnValue(models.DynamicWorkflow{ReturnStep: "selected"}, outputs); got != 2 {
		t.Fatalf("workflowReturnValue(selected) = %#v", got)
	}

	child := models.DynamicWorkflow{
		Name:       "child",
		IsActive:   true,
		Mode:       models.WorkflowModeTransactional,
		ReturnStep: "childSet",
		Steps: []models.DynamicWorkflowStep{{
			Name:     "childSet",
			Type:     models.WorkflowStepTypeSetVariable,
			IsActive: true,
			Config:   map[string]interface{}{"values": map[string]interface{}{"child": "done"}},
		}},
	}
	container := &models.ContainerModel{SchemaName: "orders", Workflows: []models.DynamicWorkflow{child}}
	parentOutputs := map[string]interface{}{"parent": "kept"}
	got, err := (&DynamicService{}).workflowExecuteWorkflow(context.Background(), models.DynamicWorkflowStep{
		Config: map[string]interface{}{"workflowName": "child"},
	}, workflowExecutionPayload{
		SchemaName:  "orders",
		Container:   container,
		StepOutputs: parentOutputs,
		Variables:   map[string]interface{}{},
	})
	if err != nil || !reflect.DeepEqual(got, map[string]interface{}{"child": "done"}) {
		t.Fatalf("workflowExecuteWorkflow() = %#v, %v", got, err)
	}
	if !reflect.DeepEqual(parentOutputs, map[string]interface{}{"parent": "kept"}) {
		t.Fatalf("parent outputs leaked child steps: %#v", parentOutputs)
	}
}

func TestWorkflowReturnStepBuildsResponseAndStopsExecution(t *testing.T) {
	service := &DynamicService{}
	payload := workflowExecutionPayload{
		Record:    map[string]interface{}{"tableId": 12},
		Variables: map[string]interface{}{},
	}
	workflow := models.DynamicWorkflow{
		Name:     "table.addGameplay",
		IsActive: true,
		Mode:     models.WorkflowModeTransactional,
		Steps: []models.DynamicWorkflowStep{
			{
				Name:     "createGameplay",
				Type:     models.WorkflowStepTypeSetVariable,
				Order:    1,
				IsActive: true,
				Config:   map[string]interface{}{"values": map[string]interface{}{"gameplayId": 55}},
			},
			{
				Name:     "returnResponse",
				Type:     models.WorkflowStepTypeReturn,
				Order:    2,
				IsActive: true,
				Config: map[string]interface{}{"value": map[string]interface{}{
					"success":  true,
					"message":  "Gameplay added successfully",
					"tableId":  "{{record.tableId}}",
					"gameplay": "{{steps.createGameplay}}",
				}},
			},
		},
	}
	if err := service.runWorkflowDefinition(context.Background(), &payload, workflow); err != nil {
		t.Fatalf("runWorkflowDefinition() error = %v", err)
	}
	want := map[string]interface{}{
		"success":  true,
		"message":  "Gameplay added successfully",
		"tableId":  12,
		"gameplay": map[string]interface{}{"gameplayId": 55},
	}
	if got := workflowExecutionReturnValue(workflow, &payload); !reflect.DeepEqual(got, want) {
		t.Fatalf("workflowExecutionReturnValue() = %#v, want %#v", got, want)
	}
	if payload.Variables["ran"] != nil {
		t.Fatalf("return step did not stop execution: %#v", payload.Variables)
	}

	runtimePayload := workflowExecutionPayload{Variables: map[string]interface{}{}}
	if err := service.runWorkflowStepList(context.Background(), &runtimePayload, "legacy", models.WorkflowModeTransactional, models.WorkflowModeTransactional, true, []models.DynamicWorkflowStep{
		{Name: "response", Type: models.WorkflowStepTypeReturn, Order: 1, IsActive: true, Config: map[string]interface{}{"value": "done"}},
		{Name: "mustNotRun", Type: models.WorkflowStepTypeSetVariable, Order: 2, IsActive: true, Config: map[string]interface{}{"values": map[string]interface{}{"ran": true}}},
	}); err != nil {
		t.Fatalf("runWorkflowStepList() error = %v", err)
	}
	if runtimePayload.ReturnValue != "done" ||
		runtimePayload.Variables["ran"] != nil ||
		runtimePayload.WorkflowName != "legacy" ||
		runtimePayload.WorkflowMode != models.WorkflowModeTransactional ||
		runtimePayload.ExecutionMode != models.WorkflowModeTransactional ||
		!runtimePayload.StopOnError {
		t.Fatalf("runtime return guard failed: %#v", runtimePayload)
	}
}

func TestDynamicServiceNoRepositoryHelpers(t *testing.T) {
	service := NewDynamicService()
	ctx := context.Background()
	container := &models.ContainerModel{SchemaName: "orders", Fields: []models.Field{{Name: "sequence", Type: "autoIncrementId"}}}
	item := map[string]interface{}{"sequence": 12}
	if err := service.applyAutoIncrementFields(ctx, "orders", container, item); err != nil || item["sequence"] != 12 {
		t.Fatalf("applyAutoIncrementFields() item = %#v, error = %v", item, err)
	}
	items := []map[string]interface{}{{"sequence": 13}}
	if err := service.applyAutoIncrementFieldsForItems(ctx, "orders", container, items); err != nil || items[0]["sequence"] != 13 {
		t.Fatalf("applyAutoIncrementFieldsForItems() items = %#v, error = %v", items, err)
	}
	if err := service.ensureDeleteReferences(ctx, "tenant", "project", "orders", primitive.NewObjectID(), []models.ContainerModel{{SchemaName: "customers"}}); err != nil {
		t.Fatalf("ensureDeleteReferences() error = %v", err)
	}
	if err := service.forceDeleteReferences(ctx, "tenant", "project", "orders", primitive.NewObjectID(), []models.ContainerModel{{SchemaName: "customers"}}, true); err != nil {
		t.Fatalf("forceDeleteReferences() error = %v", err)
	}
}

func TestDynamicServiceSuppliedContainerResolvers(t *testing.T) {
	service := NewDynamicService()
	ctx := context.Background()
	container := &models.ContainerModel{SchemaName: "orders"}
	tests := []struct {
		name    string
		resolve func() (*models.ContainerModel, error)
	}{
		{name: "create", resolve: func() (*models.ContainerModel, error) {
			return service.resolveContainer(ctx, CreateDynamicItemInput{Container: container})
		}},
		{name: "create multiple", resolve: func() (*models.ContainerModel, error) {
			return service.resolveCreateMultipleContainer(ctx, CreateMultipleDynamicItemsInput{Container: container})
		}},
		{name: "update", resolve: func() (*models.ContainerModel, error) {
			return service.resolveUpdateContainer(ctx, UpdateDynamicItemInput{Container: container})
		}},
		{name: "update multiple", resolve: func() (*models.ContainerModel, error) {
			return service.resolveUpdateMultipleContainer(ctx, UpdateMultipleDynamicItemsInput{Container: container})
		}},
		{name: "delete", resolve: func() (*models.ContainerModel, error) {
			return service.resolveDeleteContainer(ctx, DeleteDynamicItemInput{Container: container})
		}},
		{name: "delete multiple", resolve: func() (*models.ContainerModel, error) {
			return service.resolveDeleteMultipleContainer(ctx, DeleteMultipleDynamicItemsInput{Container: container})
		}},
		{name: "get all", resolve: func() (*models.ContainerModel, error) {
			return service.resolveGetAllContainer(ctx, GetAllDynamicItemsInput{Container: container})
		}},
		{name: "get item", resolve: func() (*models.ContainerModel, error) {
			return service.resolveGetDynamicContainer(ctx, GetDynamicItemInput{Container: container})
		}},
		{name: "search", resolve: func() (*models.ContainerModel, error) {
			return service.resolveSearchContainer(ctx, SearchDynamicItemsInput{Container: container})
		}},
		{name: "filter", resolve: func() (*models.ContainerModel, error) {
			return service.resolveFilterContainer(ctx, FilterDynamicItemsInput{Container: container})
		}},
		{name: "paginated", resolve: func() (*models.ContainerModel, error) {
			return service.resolvePaginatedContainer(ctx, GetPaginatedDynamicItemsInput{Container: container})
		}},
		{name: "pipeline", resolve: func() (*models.ContainerModel, error) {
			return service.resolvePipelineContainer(ctx, GetPipelineInput{Container: container})
		}},
		{name: "dynamic code", resolve: func() (*models.ContainerModel, error) {
			return service.resolveDynamicCodeContainer(ctx, ExecuteDynamicCodeInput{Container: container})
		}},
		{name: "test pipeline", resolve: func() (*models.ContainerModel, error) {
			return service.resolveTestPipelineContainer(ctx, TestPipelineInput{Container: container})
		}},
		{name: "dynamic api", resolve: func() (*models.ContainerModel, error) {
			return service.resolveDynamicAPIContainer(ctx, ExecuteDynamicAPIInput{Container: container})
		}},
		{name: "workflow", resolve: func() (*models.ContainerModel, error) {
			return service.resolveWorkflowContainer(ctx, ExecuteWorkflowInput{Container: container})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.resolve()
			if err != nil || got != container {
				t.Fatalf("resolve() = %#v, %v", got, err)
			}
		})
	}
}

func TestWorkflowHandlerEarlyValidation(t *testing.T) {
	service := &DynamicService{}
	ctx := context.Background()
	container := &models.ContainerModel{SchemaName: "orders"}
	payload := workflowExecutionPayload{SchemaName: "orders", Container: container}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "create requires document", run: func() error {
			_, err := service.workflowCreateRecord(ctx, models.DynamicWorkflowStep{Config: map[string]interface{}{"document": "invalid"}}, payload)
			return err
		}},
		{name: "update requires filter", run: func() error {
			_, err := service.workflowUpdateRecord(ctx, models.DynamicWorkflowStep{Config: map[string]interface{}{}}, payload)
			return err
		}},
		{name: "delete requires filter", run: func() error {
			_, err := service.workflowDeleteRecord(ctx, models.DynamicWorkflowStep{Config: map[string]interface{}{}}, payload)
			return err
		}},
		{name: "get requires lookup", run: func() error {
			_, err := service.workflowGetRecord(ctx, models.DynamicWorkflowStep{Config: map[string]interface{}{}}, payload)
			return err
		}},
		{name: "find rejects limit", run: func() error {
			_, err := service.workflowFindRecords(ctx, models.DynamicWorkflowStep{Config: map[string]interface{}{"limit": 0}}, payload)
			return err
		}},
		{name: "nested workflow depth", run: func() error {
			_, err := service.workflowExecuteWorkflow(ctx, models.DynamicWorkflowStep{}, workflowExecutionPayload{WorkflowDepth: maxWorkflowDepth})
			return err
		}},
		{name: "nested workflow requires name", run: func() error {
			_, err := service.workflowExecuteWorkflow(ctx, models.DynamicWorkflowStep{}, payload)
			return err
		}},
		{name: "dynamic api requires name", run: func() error {
			_, err := service.workflowExecuteDynamicAPI(ctx, models.DynamicWorkflowStep{}, payload)
			return err
		}},
		{name: "pipeline requires config", run: func() error {
			_, err := service.workflowRunPipeline(ctx, models.DynamicWorkflowStep{}, payload)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("workflow handler error = nil")
			}
		})
	}
}

func TestWorkflowAndDynamicFunctionRedisCaching(t *testing.T) {
	server := miniredis.RunT(t)
	oldClient := configs.RedisClient
	configs.RedisClient = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = configs.RedisClient.Close()
		configs.RedisClient = oldClient
	})

	service := NewDynamicService()
	ctx := context.Background()
	output, err := service.workflowInvalidateCache(ctx, models.DynamicWorkflowStep{Config: map[string]interface{}{
		"schemas": []interface{}{"orders", "customers", "orders"},
	}}, workflowExecutionPayload{TenantID: "tenant", ProjectID: "project", SchemaName: "fallback"})
	if err != nil || !reflect.DeepEqual(output, map[string]interface{}{"schemas": []string{"orders", "customers"}}) {
		t.Fatalf("workflowInvalidateCache() = %#v, %v", output, err)
	}
	for _, schema := range []string{"orders", "customers"} {
		if got, err := configs.RedisClient.Get(ctx, utils.BuildSchemaCacheVersionKey("tenant", "project", schema)).Int64(); err != nil || got != 1 {
			t.Fatalf("schema %q version = %d, %v", schema, got, err)
		}
	}

	container := &models.ContainerModel{Redis: models.Redis{CacheTime: 1}}
	service.cacheDynamicFunctionResult(ctx, "function-key", "query=1", map[string]interface{}{"count": 2}, container, true)
	if got, err := configs.RedisClient.Get(ctx, "function-key-query").Result(); err != nil || got != "query=1" {
		t.Fatalf("dynamic function query cache = %q, %v", got, err)
	}
	if key, ok := schemaCacheKey(ctx, "tenant", "project", "orders", true, "route", "query"); !ok || key == "" {
		t.Fatalf("schemaCacheKey() = %q, %v", key, ok)
	}
	if key, ok := schemaCacheKey(ctx, "tenant", "project", "orders", false, "route", "query"); ok || key != "" {
		t.Fatalf("schemaCacheKey(disabled) = %q, %v", key, ok)
	}
}

func TestWorkflowNoOpWrappers(t *testing.T) {
	service := &DynamicService{}
	payload := workflowExecutionPayload{}
	if err := service.runTransactionalWorkflows(nil, payload, "create"); err != nil {
		t.Fatalf("runTransactionalWorkflows() error = %v", err)
	}
	if err := service.enqueueOutboxWorkflows(nil, payload, "create"); err != nil {
		t.Fatalf("enqueueOutboxWorkflows() error = %v", err)
	}
	if err := service.runWorkflows(nil, payload, "create", models.WorkflowModeTransactional); err != nil {
		t.Fatalf("runWorkflows() error = %v", err)
	}
}

func serviceErrorStatus(err error) int {
	var serviceErr *ServiceError
	if errors.As(err, &serviceErr) {
		return serviceErr.Status
	}
	return 0
}

func TestAdditionalServiceErrorMappings(t *testing.T) {
	if got := duplicateKeyServiceError(&models.ContainerModel{Fields: []models.Field{{Name: "email", Unique: true}}}, errors.New("idx_email_unique")); got.Status != http.StatusConflict || !strings.Contains(got.Message, "email") {
		t.Fatalf("duplicateKeyServiceError() = %#v", got)
	}
	for _, parse := range []func(error) *ServiceError{parseBulkUpdateError, parseBulkDeleteError} {
		if got := parse(&requests.BatchLimitError{Operation: "bulk", Requested: 2, Max: 1}); got.Status != http.StatusBadRequest {
			t.Fatalf("limit mapping = %#v", got)
		}
	}
}

func TestAdditionalExportFormatting(t *testing.T) {
	if got := formatExportArray([]interface{}{"a", 2}); got != "a,2" {
		t.Fatalf("formatExportArray(interface) = %#v", got)
	}
	if got := formatExportArray([]int{1, 2}); got != "1,2" {
		t.Fatalf("formatExportArray(int) = %#v", got)
	}
	if got := formatPopulatedValue([]interface{}{map[string]interface{}{"name": "Ada"}, bson.M{"name": "Lin"}}, []string{"name"}); got != "Ada, Lin" {
		t.Fatalf("formatPopulatedValue(array) = %q", got)
	}
}

func parseIP(value string) net.IP {
	return net.ParseIP(value)
}
