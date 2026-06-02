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
}

func TestWorkflowConditions(t *testing.T) {
	payload := workflowExecutionPayload{
		Record:    map[string]interface{}{"amount": 20, "state": "done", "tags": []interface{}{"a"}, "nested": map[string]interface{}{"value": 3}},
		OldRecord: map[string]interface{}{"amount": 10, "state": "new"},
		UserID:    "user",
	}
	tests := []struct {
		name      string
		condition models.WorkflowCondition
		want      bool
	}{
		{name: "equal", condition: models.WorkflowCondition{Field: "state", Operator: "=", Value: "done"}, want: true},
		{name: "not equal", condition: models.WorkflowCondition{Field: "state", Operator: "!=", Value: "new"}, want: true},
		{name: "greater", condition: models.WorkflowCondition{Field: "amount", Operator: ">", Value: 10}, want: true},
		{name: "less", condition: models.WorkflowCondition{Field: "amount", Operator: "<", Value: 10}},
		{name: "in", condition: models.WorkflowCondition{Field: "state", Operator: "in", Value: []interface{}{"new", "done"}}, want: true},
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
}

func TestWorkflowTemplatesAndPaths(t *testing.T) {
	payload := workflowExecutionPayload{
		SchemaName:   "orders",
		WorkflowName: "notify",
		UserID:       "user",
		Record:       map[string]interface{}{"amount": 3, "nested": map[string]interface{}{"name": "Ada"}},
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
	}}, payload)
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
	if err := service.runWorkflowDefinition(context.Background(), payload, workflow); err != nil {
		t.Fatalf("runWorkflowDefinition() error = %v", err)
	}
	if payload.Variables["first"] != "ready" || payload.Variables["second"] != "ready" {
		t.Fatalf("runWorkflowDefinition() variables = %#v", payload.Variables)
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
	}, payload)
	if err != nil || !reflect.DeepEqual(output, map[string]interface{}{"branch": "else"}) || payload.Variables["branch"] != "else" {
		t.Fatalf("workflowIf() = %#v, %v; variables = %#v", output, err, payload.Variables)
	}

	output, err = service.workflowForEach(context.Background(), models.DynamicWorkflowStep{
		Config: map[string]interface{}{"items": "{{record.items}}"},
	}, payload)
	if err != nil || !reflect.DeepEqual(output, map[string]interface{}{
		"count": 2,
		"items": []interface{}{map[string]interface{}{"index": 0}, map[string]interface{}{"index": 1}},
	}) {
		t.Fatalf("workflowForEach() = %#v, %v", output, err)
	}
	if _, err := service.workflowForEach(context.Background(), models.DynamicWorkflowStep{Config: map[string]interface{}{"items": "invalid"}}, payload); err == nil {
		t.Fatal("workflowForEach(invalid items) error = nil")
	}
	if _, err := service.workflowForEach(context.Background(), models.DynamicWorkflowStep{Config: map[string]interface{}{"items": []interface{}{}, "maxItems": 0}}, payload); err == nil {
		t.Fatal("workflowForEach(invalid maxItems) error = nil")
	}
}

func TestWorkflowStepProcessingErrors(t *testing.T) {
	service := &DynamicService{}
	payload := workflowExecutionPayload{Variables: map[string]interface{}{}}
	if _, err := service.processWorkflowStep(context.Background(), models.DynamicWorkflowStep{Type: "unknown"}, payload); err == nil {
		t.Fatal("processWorkflowStep(unknown) error = nil")
	}
	if _, err := service.processWorkflowStep(context.Background(), models.DynamicWorkflowStep{Type: models.WorkflowStepTypeDynamicFunction}, payload); err == nil {
		t.Fatal("processWorkflowStep(dynamic function) error = nil")
	}
	if _, err := service.processWorkflowStepForMode(context.Background(), models.DynamicWorkflowStep{Type: models.WorkflowStepTypeCallAPI}, payload, models.WorkflowModeTransactional); err == nil {
		t.Fatal("processWorkflowStepForMode(side effect) error = nil")
	}
	err := service.runWorkflowStepList(context.Background(), payload, "workflow", models.WorkflowModeTransactional, models.WorkflowModeTransactional, true, []models.DynamicWorkflowStep{
		{Name: "bad", Type: "unknown", IsActive: true},
	})
	if err == nil || !strings.Contains(err.Error(), "workflow workflow step bad failed") {
		t.Fatalf("runWorkflowStepList() error = %v", err)
	}
	if err := service.runWorkflowStepList(context.Background(), payload, "workflow", models.WorkflowModeTransactional, models.WorkflowModeTransactional, true, []models.DynamicWorkflowStep{
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
	container := &models.ContainerModel{
		SchemaName: "orders",
		Workflows: []models.DynamicWorkflow{
			{Name: "disabled"},
			{
				Name:     "active",
				IsActive: true,
				Mode:     models.WorkflowModeTransactional,
				Steps: []models.DynamicWorkflowStep{{
					Name:     "set",
					Type:     models.WorkflowStepTypeSetVariable,
					IsActive: true,
					Config:   map[string]interface{}{"values": map[string]interface{}{"state": "done"}},
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
	if err != nil || got.Message != "Workflow executed successfully" || got.Source != "workflow" {
		t.Fatalf("ExecuteWorkflow(active) = %#v, %v", got, err)
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
