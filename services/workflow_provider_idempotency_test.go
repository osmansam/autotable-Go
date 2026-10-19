package services

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestWorkflowCallAPIProviderIdempotencySurvivesReplay(t *testing.T) {
	var keys []string
	service := &DynamicService{workflowHTTPTransport: workflowHTTPAuditTransport(func(request *http.Request) (*http.Response, error) {
		keys = append(keys, request.Header.Get("Idempotency-Key"))
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"success":true}`))}, nil
	})}
	step := models.DynamicWorkflowStep{Type: models.WorkflowStepTypeCallAPI, TimeoutSec: 1, Config: map[string]interface{}{
		"url": "https://8.8.8.8/charges", "idempotencyHeader": "Idempotency-Key", "headers": map[string]interface{}{"Idempotency-Key": "caller-value"},
	}}
	payload := workflowExecutionPayload{OutboxEventID: primitive.NewObjectID(), IdempotencyKey: "durable-event:step:one"}
	for attempt := 0; attempt < 3; attempt++ {
		if attempt == 2 {
			payload.IdempotencyKey = "durable-event:step:two"
		}
		if _, err := service.workflowCallAPI(context.Background(), step, payload); err != nil {
			t.Fatal(err)
		}
	}
	if len(keys) != 3 || keys[0] == "" || keys[0] == "caller-value" || keys[0] != keys[1] || keys[0] == keys[2] || len(keys[0]) > 255 {
		t.Fatalf("provider replay identities = %#v, want stable retries and distinct steps", keys)
	}
}

func TestWorkflowCallAPIProviderIdempotencyRequiresOptInAndDurableIdentity(t *testing.T) {
	requests := 0
	service := &DynamicService{workflowHTTPTransport: workflowHTTPAuditTransport(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.Header.Get("Idempotency-Key") != "" {
			t.Error("unconfigured provider request received a new header")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}
	step := models.DynamicWorkflowStep{Type: models.WorkflowStepTypeCallAPI, TimeoutSec: 1, Config: map[string]interface{}{"url": "https://8.8.8.8/charges"}}
	if _, err := service.workflowCallAPI(context.Background(), step, workflowExecutionPayload{OutboxEventID: primitive.NewObjectID(), IdempotencyKey: "key"}); err != nil {
		t.Fatal(err)
	}
	step.Config["idempotencyHeader"] = "Idempotency-Key"
	if _, err := service.workflowCallAPI(context.Background(), step, workflowExecutionPayload{}); err == nil || !strings.Contains(err.Error(), "durable") {
		t.Fatalf("idempotency enabled without replay identity: %v", err)
	}
	if requests != 1 {
		t.Fatalf("invalid identity reached provider: %d requests", requests)
	}
}

func TestWorkflowCallAPIIdempotencyHeaderValidation(t *testing.T) {
	for _, name := range []interface{}{"Authorization", "Proxy-Authorization", "Host", "Cookie", "Content-Type", "Content-Length", "Connection", "Transfer-Encoding", "X Bad", "X\r\nInjected", true} {
		step := models.DynamicWorkflowStep{Type: models.WorkflowStepTypeCallAPI, TimeoutSec: 1, Config: map[string]interface{}{"url": "https://example.com", "idempotencyHeader": name}}
		err := ValidateWorkflow(models.DynamicWorkflow{Name: "provider", Mode: models.WorkflowModeOutbox, Steps: []models.DynamicWorkflowStep{step}})
		if err == nil || !strings.Contains(err.Error(), "idempotencyHeader") {
			t.Errorf("invalid idempotency header %#v accepted: %v", name, err)
		}
	}
}

func TestWorkflowProviderIdempotencyPreservesCredentialHeaders(t *testing.T) {
	payload := workflowExecutionPayload{OutboxEventID: primitive.NewObjectID(), IdempotencyKey: "durable-step"}
	protected := map[string]string{"X-Api-Key": "secret", "Authorization": "Bearer token"}
	if _, err := workflowProviderIdempotencyHeaders(map[string]interface{}{"idempotencyHeader": "x-api-key"}, payload, protected); err == nil || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("idempotency header could replace a credential: %v", err)
	}
	headers, err := workflowProviderIdempotencyHeaders(map[string]interface{}{"idempotencyHeader": "x-provider-replay"}, payload, protected)
	if err != nil || headers["X-Provider-Replay"] == "" || headers["X-Api-Key"] != "secret" || headers["Authorization"] != "Bearer token" {
		t.Fatalf("provider headers = %#v, error = %v", headers, err)
	}
	if len(protected) != 2 {
		t.Fatal("provider headers mutated the credential snapshot")
	}
}

func TestWorkflowIdempotencyScopeSeparatorsCannotCollide(t *testing.T) {
	id := primitive.NewObjectID()
	payload := workflowExecutionPayload{TenantID: "tenant", ProjectID: "project", SchemaName: "orders", OutboxEventID: id}
	for _, explicit := range []string{"", "business:key"} {
		first := workflowStepIdempotencyKey(payload, "a:b", models.DynamicWorkflowStep{ID: "c", IdempotencyKey: explicit})
		second := workflowStepIdempotencyKey(payload, "a", models.DynamicWorkflowStep{ID: "b:c", IdempotencyKey: explicit})
		if first == second {
			t.Errorf("workflow/step scopes collided with explicit=%q: %s", explicit, first)
		}
	}
	firstProviderKey := ""
	for _, workflow := range []string{"a:b", "a"} {
		payload.WorkflowName = workflow
		// Even a historical ambiguous key cannot collapse provider identities
		// across distinct trusted workflow scopes.
		payload.IdempotencyKey = "same-legacy-key"
		headers, err := workflowProviderIdempotencyHeaders(map[string]interface{}{"idempotencyHeader": "Idempotency-Key"}, payload, nil)
		if err != nil {
			t.Fatal(err)
		}
		if workflow == "a:b" {
			firstProviderKey = headers["Idempotency-Key"]
		} else if headers["Idempotency-Key"] == firstProviderKey {
			t.Fatal("provider idempotency digest ignored trusted workflow scope")
		}
	}
}
