package services

import (
	"reflect"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestUniqueSchemaNames(t *testing.T) {
	tests := []struct {
		name        string
		schemaNames []string
		want        []string
	}{
		{
			name:        "nil input",
			schemaNames: nil,
			want:        nil,
		},
		{
			name:        "empty and whitespace-only names are removed",
			schemaNames: []string{"", " ", "\t"},
			want:        nil,
		},
		{
			name:        "names are trimmed deduplicated and kept in first-seen order",
			schemaNames: []string{" orders ", "customers", "orders", " customers ", "invoices"},
			want:        []string{"orders", "customers", "invoices"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := uniqueSchemaNames(tt.schemaNames); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("uniqueSchemaNames() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestWriteInvalidateSchemas(t *testing.T) {
	tests := []struct {
		name       string
		schemaName string
		container  *models.ContainerModel
		want       []string
	}{
		{
			name:       "nil container invalidates written schema",
			schemaName: "orders",
			want:       []string{"orders"},
		},
		{
			name:       "triggered caches are included and normalized",
			schemaName: " orders ",
			container: &models.ContainerModel{
				Redis: models.Redis{
					TriggeredRedisCaches: []string{"customers", "orders", "", " customers ", "reports"},
				},
			},
			want: []string{"orders", "customers", "reports"},
		},
		{
			name:       "blank written schema is removed",
			schemaName: " ",
			container: &models.ContainerModel{
				Redis: models.Redis{TriggeredRedisCaches: []string{"customers"}},
			},
			want: []string{"customers"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := writeInvalidateSchemas(tt.schemaName, tt.container); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("writeInvalidateSchemas() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestOutboxRetryDelay(t *testing.T) {
	tests := []struct {
		name     string
		attempts int
		want     time.Duration
	}{
		{name: "negative attempts use first retry", attempts: -1, want: time.Second},
		{name: "zero attempts use first retry", attempts: 0, want: time.Second},
		{name: "first retry", attempts: 1, want: time.Second},
		{name: "quadratic retry", attempts: 3, want: 9 * time.Second},
		{name: "largest uncapped retry", attempts: 7, want: 49 * time.Second},
		{name: "retry is capped at one minute", attempts: 8, want: time.Minute},
		{name: "large retry stays capped", attempts: 100, want: time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := outboxRetryDelay(tt.attempts); got != tt.want {
				t.Fatalf("outboxRetryDelay(%d) = %s, want %s", tt.attempts, got, tt.want)
			}
		})
	}
}

func TestAuditDocumentID(t *testing.T) {
	id := primitive.NewObjectID()

	tests := []struct {
		name string
		doc  interface{}
		want primitive.ObjectID
	}{
		{name: "nil document", doc: nil, want: primitive.NilObjectID},
		{name: "map object id", doc: map[string]interface{}{"_id": id}, want: id},
		{name: "map hex id", doc: map[string]interface{}{"_id": id.Hex()}, want: id},
		{name: "bson object id", doc: bson.M{"_id": id}, want: id},
		{name: "bson hex id", doc: bson.M{"_id": id.Hex()}, want: id},
		{name: "missing id", doc: map[string]interface{}{"name": "order"}, want: primitive.NilObjectID},
		{name: "invalid hex id", doc: map[string]interface{}{"_id": "invalid"}, want: primitive.NilObjectID},
		{name: "unsupported document type", doc: struct{}{}, want: primitive.NilObjectID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := auditDocumentID(tt.doc); got != tt.want {
				t.Fatalf("auditDocumentID() = %s, want %s", got.Hex(), tt.want.Hex())
			}
		})
	}
}

func TestAuditDocumentIDs(t *testing.T) {
	firstID := primitive.NewObjectID()
	secondID := primitive.NewObjectID()

	tests := []struct {
		name string
		docs []interface{}
		want []primitive.ObjectID
	}{
		{
			name: "nil input",
			want: nil,
		},
		{
			name: "single document",
			docs: []interface{}{map[string]interface{}{"_id": firstID}},
			want: []primitive.ObjectID{firstID},
		},
		{
			name: "bulk documents flatten deduplicate and ignore invalid ids",
			docs: []interface{}{
				[]interface{}{
					map[string]interface{}{"_id": firstID},
					bson.M{"_id": secondID.Hex()},
					map[string]interface{}{"_id": "invalid"},
				},
				map[string]interface{}{"_id": firstID.Hex()},
				nil,
			},
			want: []primitive.ObjectID{firstID, secondID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := auditDocumentIDs(tt.docs...); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("auditDocumentIDs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestBuildDynamicAuditLog(t *testing.T) {
	userID := primitive.NewObjectID()
	before := map[string]interface{}{"_id": primitive.NewObjectID(), "status": "new"}
	after := map[string]interface{}{"_id": before["_id"], "status": "done"}

	tests := []struct {
		name      string
		user      *models.AuditUser
		wantID    primitive.ObjectID
		wantEmail string
		wantRoles []string
	}{
		{name: "without user"},
		{
			name:      "with user",
			user:      &models.AuditUser{ID: userID, Email: "user@example.com", Roles: []string{"admin"}},
			wantID:    userID,
			wantEmail: "user@example.com",
			wantRoles: []string{"admin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeBuild := time.Now()
			got := buildDynamicAuditLog("tenant", "project", "orders", models.DynamicOutboxOperationUpdate, tt.user, before, after)
			afterBuild := time.Now()

			if got.TenantID != "tenant" || got.ProjectID != "project" || got.SchemaName != "orders" {
				t.Fatalf("scope = (%q, %q, %q), want (tenant, project, orders)", got.TenantID, got.ProjectID, got.SchemaName)
			}
			if got.Action != models.DynamicOutboxOperationUpdate {
				t.Fatalf("Action = %q, want %q", got.Action, models.DynamicOutboxOperationUpdate)
			}
			if !reflect.DeepEqual(got.Before, before) || !reflect.DeepEqual(got.After, after) {
				t.Fatal("Before and After must preserve the provided snapshots")
			}
			if got.UserID != tt.wantID || got.UserEmail != tt.wantEmail || !reflect.DeepEqual(got.Roles, tt.wantRoles) {
				t.Fatalf("user = (%s, %q, %#v), want (%s, %q, %#v)", got.UserID.Hex(), got.UserEmail, got.Roles, tt.wantID.Hex(), tt.wantEmail, tt.wantRoles)
			}
			if !reflect.DeepEqual(got.DocumentIDs, []primitive.ObjectID{before["_id"].(primitive.ObjectID)}) {
				t.Fatalf("DocumentIDs = %#v, want the deduplicated document id", got.DocumentIDs)
			}
			assertDateTimeBetween(t, got.Timestamp, beforeBuild, afterBuild)
		})
	}
}

func TestBuildDynamicPostWriteEvent(t *testing.T) {
	operations := []string{
		models.DynamicOutboxOperationCreate,
		models.DynamicOutboxOperationUpdate,
		models.DynamicOutboxOperationDelete,
		models.DynamicOutboxOperationBulkCreate,
		models.DynamicOutboxOperationBulkUpdate,
		models.DynamicOutboxOperationBulkDelete,
	}

	for _, operation := range operations {
		t.Run(operation, func(t *testing.T) {
			auditLog := &models.AuditLog{}
			container := &models.ContainerModel{
				Redis: models.Redis{TriggeredRedisCaches: []string{"customers", "orders", "customers"}},
			}
			beforeBuild := time.Now()
			got := buildDynamicPostWriteEvent("tenant", "project", "orders", operation, "user", container, auditLog)
			afterBuild := time.Now()

			if got.ID == primitive.NilObjectID {
				t.Fatal("ID must be generated")
			}
			if auditLog.EventID != got.ID {
				t.Fatalf("audit EventID = %s, want event ID %s", auditLog.EventID.Hex(), got.ID.Hex())
			}
			if got.TenantID != "tenant" || got.ProjectID != "project" || got.SchemaName != "orders" {
				t.Fatalf("scope = (%q, %q, %q), want (tenant, project, orders)", got.TenantID, got.ProjectID, got.SchemaName)
			}
			if got.Operation != operation {
				t.Fatalf("Operation = %q, want %q", got.Operation, operation)
			}
			if got.Status != models.DynamicOutboxStatusPending || got.Attempts != 0 || got.MaxAttempts != dynamicOutboxMaxAttempts {
				t.Fatalf("retry state = (%q, %d, %d), want (%q, 0, %d)", got.Status, got.Attempts, got.MaxAttempts, models.DynamicOutboxStatusPending, dynamicOutboxMaxAttempts)
			}
			if got.Payload.AuditLog != auditLog || got.Payload.UserID != "user" {
				t.Fatal("payload must preserve audit log and user ID")
			}
			if want := []string{"orders", "customers"}; !reflect.DeepEqual(got.Payload.InvalidateSchemas, want) {
				t.Fatalf("InvalidateSchemas = %#v, want %#v", got.Payload.InvalidateSchemas, want)
			}
			assertDateTimeBetween(t, got.NextAttemptAt, beforeBuild, afterBuild)
			assertDateTimeBetween(t, got.CreatedAt, beforeBuild, afterBuild)
			assertDateTimeBetween(t, got.UpdatedAt, beforeBuild, afterBuild)
		})
	}

	t.Run("nil audit log and container", func(t *testing.T) {
		got := buildDynamicPostWriteEvent("tenant", "project", "orders", models.DynamicOutboxOperationCreate, "user", nil, nil)
		if got.Payload.AuditLog != nil {
			t.Fatalf("AuditLog = %#v, want nil", got.Payload.AuditLog)
		}
		if want := []string{"orders"}; !reflect.DeepEqual(got.Payload.InvalidateSchemas, want) {
			t.Fatalf("InvalidateSchemas = %#v, want %#v", got.Payload.InvalidateSchemas, want)
		}
	})
}

func TestBuildWorkflowStepOutboxEvent(t *testing.T) {
	tests := []struct {
		name        string
		step        models.DynamicWorkflowStep
		wantRetries int
	}{
		{
			name:        "default max attempts",
			step:        models.DynamicWorkflowStep{ID: "step-id", Name: "step name", Type: models.WorkflowStepTypeCallAPI},
			wantRetries: dynamicOutboxMaxAttempts,
		},
		{
			name:        "retry count overrides default",
			step:        models.DynamicWorkflowStep{ID: "step-id", Type: models.WorkflowStepTypeCallAPI, RetryCount: 3},
			wantRetries: 3,
		},
		{
			name:        "max attempts overrides retry count",
			step:        models.DynamicWorkflowStep{ID: "step-id", Type: models.WorkflowStepTypeCallAPI, RetryCount: 3, MaxAttempts: 5},
			wantRetries: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recordID := primitive.NewObjectID()
			record := map[string]interface{}{"_id": recordID}
			oldRecord := map[string]interface{}{"status": "old"}
			outputs := map[string]interface{}{"previous": "value"}
			variables := map[string]interface{}{"region": "west"}
			loop := map[string]interface{}{"index": 1}
			tt.step.Name = "step name"
			tt.step.TimeoutSec = 15
			tt.step.TargetSchema = "target"
			tt.step.Config = map[string]interface{}{"url": "https://example.com"}
			tt.step.Steps = []models.DynamicWorkflowStep{{ID: "nested"}}
			tt.step.ElseSteps = []models.DynamicWorkflowStep{{ID: "else"}}
			tt.step.Branches = []models.WorkflowBranch{{Name: "branch"}}

			beforeBuild := time.Now()
			got := buildWorkflowStepOutboxEvent("tenant", "project", "orders", "user", "workflow", tt.step, record, oldRecord, outputs, variables, loop)
			afterBuild := time.Now()

			if got.ID == primitive.NilObjectID {
				t.Fatal("ID must be generated")
			}
			if got.Operation != models.DynamicOutboxOperationWorkflowStep || got.Status != models.DynamicOutboxStatusPending {
				t.Fatalf("event state = (%q, %q), want (%q, %q)", got.Operation, got.Status, models.DynamicOutboxOperationWorkflowStep, models.DynamicOutboxStatusPending)
			}
			if got.MaxAttempts != tt.wantRetries {
				t.Fatalf("MaxAttempts = %d, want %d", got.MaxAttempts, tt.wantRetries)
			}
			wantKey := "tenant:project:orders:workflow:step-id:" + recordID.Hex()
			if got.Payload.IdempotencyKey != wantKey {
				t.Fatalf("IdempotencyKey = %q, want %q", got.Payload.IdempotencyKey, wantKey)
			}
			if got.Payload.UserID != "user" || got.Payload.WorkflowName != "workflow" || got.Payload.StepID != tt.step.ID || got.Payload.StepName != tt.step.Name || got.Payload.StepType != tt.step.Type {
				t.Fatal("payload must preserve workflow step identity")
			}
			if got.Payload.StepTimeoutSec != 15 || got.Payload.TargetSchema != "target" {
				t.Fatal("payload must preserve timeout and target schema")
			}
			if !reflect.DeepEqual(got.Payload.Record, record) || !reflect.DeepEqual(got.Payload.OldRecord, oldRecord) {
				t.Fatal("payload must preserve record snapshots")
			}
			if !reflect.DeepEqual(got.Payload.Config, tt.step.Config) || !reflect.DeepEqual(got.Payload.Steps, tt.step.Steps) || !reflect.DeepEqual(got.Payload.ElseSteps, tt.step.ElseSteps) || !reflect.DeepEqual(got.Payload.Branches, tt.step.Branches) {
				t.Fatal("payload must preserve step execution configuration")
			}
			assertDateTimeBetween(t, got.NextAttemptAt, beforeBuild, afterBuild)
			assertDateTimeBetween(t, got.CreatedAt, beforeBuild, afterBuild)
			assertDateTimeBetween(t, got.UpdatedAt, beforeBuild, afterBuild)

			outputs["previous"] = "changed"
			variables["region"] = "changed"
			loop["index"] = 2
			if got.Payload.StepOutputs["previous"] != "value" || got.Payload.Variables["region"] != "west" || got.Payload.Loop["index"] != 1 {
				t.Fatal("workflow context maps must be copied")
			}
		})
	}

	t.Run("explicit idempotency key works without record id", func(t *testing.T) {
		step := models.DynamicWorkflowStep{
			ID:             "step-id",
			Type:           models.WorkflowStepTypeCallAPI,
			IdempotencyKey: " fixed-key ",
		}
		got := buildWorkflowStepOutboxEvent("tenant", "project", "orders", "user", "workflow", step, nil, nil, nil, nil, nil)
		if got.Payload.IdempotencyKey != "fixed-key" {
			t.Fatalf("IdempotencyKey = %q, want fixed-key", got.Payload.IdempotencyKey)
		}
		if got.Payload.StepOutputs != nil || got.Payload.Variables != nil || got.Payload.Loop != nil {
			t.Fatal("nil workflow context maps must remain nil")
		}
	})

	t.Run("missing record id leaves generated idempotency key empty", func(t *testing.T) {
		step := models.DynamicWorkflowStep{ID: "step-id", Type: models.WorkflowStepTypeCallAPI}
		got := buildWorkflowStepOutboxEvent("tenant", "project", "orders", "user", "workflow", step, map[string]interface{}{}, nil, nil, nil, nil)
		if got.Payload.IdempotencyKey != "" {
			t.Fatalf("IdempotencyKey = %q, want empty string", got.Payload.IdempotencyKey)
		}
	})
}

func assertDateTimeBetween(t *testing.T, got primitive.DateTime, before, after time.Time) {
	t.Helper()
	gotTime := got.Time()
	if gotTime.Before(before.Truncate(time.Millisecond)) || gotTime.After(after) {
		t.Fatalf("timestamp = %s, want between %s and %s", gotTime, before, after)
	}
}
