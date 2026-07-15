package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestValidateWorkflowAcceptsQueryDynamicAPIAndJoinArrays(t *testing.T) {
	workflow := models.DynamicWorkflow{
		Name:    "products-with-external-prices",
		Trigger: models.WorkflowTriggerManual,
		Mode:    models.WorkflowModeTransactional,
		Steps: []models.DynamicWorkflowStep{
			{
				Name:     "external",
				Type:     "query_dynamic_api",
				Order:    1,
				IsActive: true,
				Config:   map[string]interface{}{"apiName": "menu-items"},
			},
			{
				Name:     "joined",
				Type:     "join_arrays",
				Order:    2,
				IsActive: true,
				Config: map[string]interface{}{
					"items":       []interface{}{},
					"lookupItems": []interface{}{},
					"localField":  "davinciId",
					"lookupField": "_id",
					"mappings":    map[string]interface{}{"davinciPrice": "price"},
				},
			},
		},
	}

	if err := ValidateWorkflow(workflow); err != nil {
		t.Fatalf("ValidateWorkflow() error = %v", err)
	}
}

func TestValidateWorkflowRejectsInvalidQueryAndJoinConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		step    models.DynamicWorkflowStep
		wantErr string
	}{
		{
			name:    "query API name",
			step:    models.DynamicWorkflowStep{Name: "query", Type: "query_dynamic_api", IsActive: true},
			wantErr: "apiName is required",
		},
		{
			name: "join items",
			step: models.DynamicWorkflowStep{
				Name:     "join",
				Type:     "join_arrays",
				IsActive: true,
				Config: map[string]interface{}{
					"lookupItems": []interface{}{},
					"localField":  "id",
					"lookupField": "_id",
					"mappings":    map[string]interface{}{"price": "price"},
				},
			},
			wantErr: "items is required",
		},
		{
			name: "join mapping source type",
			step: models.DynamicWorkflowStep{
				Name:     "join",
				Type:     "join_arrays",
				IsActive: true,
				Config: map[string]interface{}{
					"items":       []interface{}{},
					"lookupItems": []interface{}{},
					"localField":  "id",
					"lookupField": "_id",
					"mappings":    map[string]interface{}{"price": 123},
				},
			},
			wantErr: "non-empty destination and string source fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkflow(models.DynamicWorkflow{
				Name:    "invalid",
				Trigger: models.WorkflowTriggerManual,
				Mode:    models.WorkflowModeTransactional,
				Steps:   []models.DynamicWorkflowStep{tt.step},
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateWorkflow() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestWorkflowQueryDynamicAPIRejectsUnsafeConfigurations(t *testing.T) {
	tests := []struct {
		name    string
		api     models.DynamicApiModel
		wantErr string
	}{
		{
			name:    "inactive",
			api:     models.DynamicApiModel{Name: "menu-items", Url: "http://127.0.0.1:1", Method: http.MethodGet},
			wantErr: "API is disabled",
		},
		{
			name:    "non GET",
			api:     models.DynamicApiModel{Name: "menu-items", Url: "http://127.0.0.1:1", Method: http.MethodPost, IsActive: true},
			wantErr: "must use GET",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&DynamicService{}).workflowQueryDynamicAPI(
				context.Background(),
				models.DynamicWorkflowStep{Config: map[string]interface{}{"apiName": "menu-items"}},
				workflowExecutionPayload{
					SchemaName: "product",
					Container: &models.ContainerModel{
						SchemaName:  "product",
						DynamicApis: []models.DynamicApiModel{tt.api},
					},
				},
			)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("workflowQueryDynamicAPI() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestWorkflowQueryDynamicAPIReturnsConfiguredGETResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("request method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"_id":181,"price":250}]`))
	}))
	defer server.Close()

	container := &models.ContainerModel{
		SchemaName: "product",
		DynamicApis: []models.DynamicApiModel{{
			Name:     "menu-items",
			Url:      server.URL,
			Method:   http.MethodGet,
			IsActive: true,
		}},
	}
	output, err := (&DynamicService{}).workflowQueryDynamicAPI(
		context.Background(),
		models.DynamicWorkflowStep{Config: map[string]interface{}{"apiName": "menu-items"}},
		workflowExecutionPayload{
			TenantID:   "tenant",
			ProjectID:  "project",
			SchemaName: "product",
			Container:  container,
		},
	)
	if err != nil {
		t.Fatalf("workflowQueryDynamicAPI() error = %v", err)
	}

	want := map[string]interface{}{
		"message": "API result fetched",
		"data": []interface{}{
			map[string]interface{}{"_id": float64(181), "price": float64(250)},
		},
		"source": "API call",
	}
	if !reflect.DeepEqual(output, want) {
		t.Fatalf("workflowQueryDynamicAPI() = %#v, want %#v", output, want)
	}
}

func TestWorkflowJoinArraysEnrichesMatchesAndFallbacksWithoutMutation(t *testing.T) {
	items := []interface{}{
		map[string]interface{}{
			"name":      "Matched",
			"davinciId": int64(181),
			"pricing":   map[string]interface{}{"base": float64(100)},
		},
		map[string]interface{}{
			"name":      "Missing",
			"davinciId": int64(999),
			"pricing":   map[string]interface{}{"base": float64(80)},
		},
	}
	lookupItems := []interface{}{
		map[string]interface{}{"_id": float64(181), "price": float64(250)},
		map[string]interface{}{"_id": float64(181), "price": float64(999)},
	}
	payload := workflowExecutionPayload{
		StepOutputs: map[string]interface{}{
			"products": map[string]interface{}{"items": items},
			"external": map[string]interface{}{"data": lookupItems},
		},
	}

	output, err := workflowJoinArrays(models.DynamicWorkflowStep{
		Config: map[string]interface{}{
			"items":       "{{steps.products.items}}",
			"lookupItems": "{{steps.external.data}}",
			"localField":  "davinciId",
			"lookupField": "_id",
			"mappings": map[string]interface{}{
				"davinciPrice":   "price",
				"pricing.remote": "price",
			},
			"fallbacks": map[string]interface{}{
				"davinciPrice":   "-",
				"pricing.remote": "-",
			},
		},
	}, payload)
	if err != nil {
		t.Fatalf("workflowJoinArrays() error = %v", err)
	}

	want := map[string]interface{}{
		"count": 2,
		"items": []map[string]interface{}{
			{
				"name":         "Matched",
				"davinciId":    int64(181),
				"davinciPrice": float64(250),
				"pricing": map[string]interface{}{
					"base":   float64(100),
					"remote": float64(250),
				},
			},
			{
				"name":         "Missing",
				"davinciId":    int64(999),
				"davinciPrice": "-",
				"pricing": map[string]interface{}{
					"base":   float64(80),
					"remote": "-",
				},
			},
		},
	}
	if !reflect.DeepEqual(output, want) {
		t.Fatalf("workflowJoinArrays() = %#v, want %#v", output, want)
	}

	originalPricing := items[0].(map[string]interface{})["pricing"].(map[string]interface{})
	if _, mutated := originalPricing["remote"]; mutated {
		t.Fatalf("workflowJoinArrays() mutated input: %#v", items[0])
	}
}

func TestWorkflowJoinArraysMatchesHexStringToObjectID(t *testing.T) {
	productID := primitive.NewObjectID()
	output, err := workflowJoinArrays(models.DynamicWorkflowStep{
		Config: map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"productId": productID.Hex(), "quantity": 1},
			},
			"lookupItems": []interface{}{
				map[string]interface{}{"_id": productID, "davinciId": 123},
			},
			"localField":  "productId",
			"lookupField": "_id",
			"mappings":    map[string]interface{}{"productDavinciId": "davinciId"},
		},
	}, workflowExecutionPayload{})
	if err != nil {
		t.Fatalf("workflowJoinArrays() error = %v", err)
	}

	items := output.(map[string]interface{})["items"].([]map[string]interface{})
	if got := items[0]["productDavinciId"]; got != 123 {
		t.Fatalf("productDavinciId = %#v, want 123", got)
	}
}

func TestReadOnlyQueryJoinWorkflowReturnsTableSourceRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"_id":181,"price":250}]`))
	}))
	defer server.Close()

	workflow := models.DynamicWorkflow{
		Name:        "products-with-external-prices",
		Trigger:     models.WorkflowTriggerManual,
		Mode:        models.WorkflowModeTransactional,
		IsActive:    true,
		StopOnError: true,
		Steps: []models.DynamicWorkflowStep{
			{
				Name:     "products",
				Type:     models.WorkflowStepTypeSetVariable,
				Order:    1,
				IsActive: true,
				Config: map[string]interface{}{"values": map[string]interface{}{
					"products": []interface{}{
						map[string]interface{}{"name": "Sandwich", "davinciId": int64(181)},
						map[string]interface{}{"name": "Unknown", "davinciId": int64(999)},
					},
				}},
			},
			{
				Name:     "external",
				Type:     models.WorkflowStepTypeQueryDynamicAPI,
				Order:    2,
				IsActive: true,
				Config:   map[string]interface{}{"apiName": "menu-items"},
			},
			{
				Name:     "joined",
				Type:     models.WorkflowStepTypeJoinArrays,
				Order:    3,
				IsActive: true,
				Config: map[string]interface{}{
					"items":       "{{vars.products}}",
					"lookupItems": "{{steps.external.data}}",
					"localField":  "davinciId",
					"lookupField": "_id",
					"mappings":    map[string]interface{}{"davinciPrice": "price"},
					"fallbacks":   map[string]interface{}{"davinciPrice": "-"},
				},
			},
			{
				Name:     "result",
				Type:     models.WorkflowStepTypeReturn,
				Order:    4,
				IsActive: true,
				Config:   map[string]interface{}{"value": "{{steps.joined.items}}"},
			},
		},
	}
	if workflowRequiresTransaction(workflow) {
		t.Fatal("workflowRequiresTransaction() = true for read-only query/join workflow")
	}

	payload := workflowExecutionPayload{
		TenantID:     "tenant",
		ProjectID:    "project",
		SchemaName:   "product",
		WorkflowName: workflow.Name,
		Container: &models.ContainerModel{
			SchemaName: "product",
			DynamicApis: []models.DynamicApiModel{{
				Name:     "menu-items",
				Url:      server.URL,
				Method:   http.MethodGet,
				IsActive: true,
			}},
		},
	}
	if err := (&DynamicService{}).runWorkflowDefinition(context.Background(), &payload, workflow); err != nil {
		t.Fatalf("runWorkflowDefinition() error = %v", err)
	}

	rows, ok := tableSourceRows(workflowExecutionReturnValue(workflow, &payload))
	if !ok {
		t.Fatalf("tableSourceRows() rejected workflow output: %#v", payload.ReturnValue)
	}
	if len(rows) != 2 || rows[0]["davinciPrice"] != float64(250) || rows[1]["davinciPrice"] != "-" {
		t.Fatalf("enriched rows = %#v", rows)
	}
}
