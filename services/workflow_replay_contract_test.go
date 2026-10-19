package services

import (
	"context"
	"strings"
	"testing"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestWorkflowRejectsUnsupportedDynamicFunctionDefinitions(t *testing.T) {
	for _, steps := range [][]models.DynamicWorkflowStep{
		{{Type: models.WorkflowStepTypeDynamicFunction, IsActive: true}},
		{{Type: models.WorkflowStepTypeIf, IsActive: true, Steps: []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeDynamicFunction, IsActive: true}}}},
	} {
		err := ValidateWorkflow(models.DynamicWorkflow{Name: "unsupported-function", Steps: steps})
		if err == nil || !strings.Contains(err.Error(), "unsupported type") {
			t.Fatalf("unsupported workflow function definition accepted: %v", err)
		}
	}
}

func TestWorkflowOutboxSnapshotVersionIsPersisted(t *testing.T) {
	event := buildWorkflowStepOutboxEvent(workflowExecutionPayload{}, "snapshot", models.DynamicWorkflowStep{Type: models.WorkflowStepTypeSetVariable})
	encoded, err := bson.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var stored bson.M
	if err := bson.Unmarshal(encoded, &stored); err != nil {
		t.Fatal(err)
	}
	if got := stored["payload"].(bson.M)["workflowSnapshotVersion"]; got != int32(1) {
		t.Fatalf("stored snapshot version = %#v, want 1", got)
	}
}

func TestWorkflowOutboxRejectsAmbiguousLegacySnapshotsBeforeExecution(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload models.DynamicOutboxPayload
		want    string
	}{
		{name: "missing root condition", payload: models.DynamicOutboxPayload{StepType: models.WorkflowStepTypeIf, Steps: []models.DynamicWorkflowStep{auditVariableStep("if", 0, map[string]interface{}{"branch": "if"})}, ElseSteps: []models.DynamicWorkflowStep{auditVariableStep("else", 0, map[string]interface{}{"branch": "else"})}}, want: "legacy"},
		{name: "missing query", payload: models.DynamicOutboxPayload{StepType: models.WorkflowStepTypeSetVariable, Config: map[string]interface{}{"values": bson.M{"region": "{{query.region}}"}}}, want: "query"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := processWorkflowOutboxStep(context.Background(), nil, &models.DynamicOutboxEvent{ID: primitive.NewObjectID(), Payload: test.payload})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("unsafe old snapshot executed: error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestWorkflowOutboxRejectsFutureSnapshotFormat(t *testing.T) {
	encoded, err := bson.Marshal(bson.M{"payload": bson.M{"workflowSnapshotVersion": 99, "stepType": models.WorkflowStepTypeSetVariable, "config": bson.M{"values": bson.M{"changed": true}}}})
	if err != nil {
		t.Fatal(err)
	}
	var event models.DynamicOutboxEvent
	if err := bson.Unmarshal(encoded, &event); err != nil {
		t.Fatal(err)
	}
	if err := processWorkflowOutboxStep(context.Background(), nil, &event); err == nil || !strings.Contains(err.Error(), "snapshot version") {
		t.Fatalf("future snapshot format was executed: %v", err)
	}
}

func TestWorkflowOutboxLegacySnapshotsWithCompleteDataRemainCompatible(t *testing.T) {
	for _, payload := range []models.DynamicOutboxPayload{
		{StepType: models.WorkflowStepTypeSetVariable, Config: map[string]interface{}{"values": map[string]interface{}{"safe": true}}},
		{StepType: models.WorkflowStepTypeIf, Conditions: []models.WorkflowCondition{{Field: "record.ready", Value: true}}, Record: map[string]interface{}{"ready": true}},
		{StepType: models.WorkflowStepTypeIf, Branches: []models.WorkflowBranch{{Name: "safe"}}},
		{StepType: models.WorkflowStepTypeSetVariable, Query: map[string]interface{}{"region": "west"}, Config: map[string]interface{}{"values": map[string]interface{}{"region": "{{query.region}}"}}},
	} {
		if err := processWorkflowOutboxStep(context.Background(), nil, &models.DynamicOutboxEvent{Payload: payload}); err != nil {
			t.Fatalf("compatible legacy snapshot rejected: %v", err)
		}
	}
}

func TestWorkflowOutboxRestoresLegacyAuditIdentityFromTrustedSnapshot(t *testing.T) {
	id := primitive.NewObjectID()
	event := &models.DynamicOutboxEvent{Payload: models.DynamicOutboxPayload{UserID: id.Hex(), UserRole: "editor", UserRoles: []string{"editor", "reviewer"}, CurrentUser: map[string]interface{}{"email": "ada@example.com", "displayName": "Ada"}}}
	payload := workflowPayloadFromOutboxEvent(event)
	if payload.AuditUser == nil || payload.AuditUser.ID != id || payload.AuditUser.Email != "ada@example.com" || payload.AuditUser.DisplayName != "Ada" || len(payload.AuditUser.Roles) != 2 {
		t.Fatalf("legacy trusted identity was lost: %#v", payload.AuditUser)
	}
	payload.AuditUser.Roles[0] = "changed"
	if event.Payload.UserRoles[0] != "editor" {
		t.Fatal("restored audit roles alias persisted identity")
	}
}

func TestWorkflowLegacyQueryDetectionUnderstandsTemplateBoundaries(t *testing.T) {
	for _, test := range []struct {
		name  string
		value interface{}
		want  bool
	}{
		{name: "direct", value: "{{query.region}}", want: true},
		{name: "default", value: "{{default(query.region, 'west')}}", want: true},
		{name: "nested function", value: "{{add(default(query.count, 1), 2)}}", want: true},
		{name: "quoted literal", value: "{{default('query.region', 'west')}}"},
		{name: "plain text", value: "query.region"},
		{name: "record references remain literal", value: "{{record.note}}"},
		{name: "malformed", value: "{{query.region"},
		{name: "typed BSON", value: primitive.A{bson.M{"region": "{{query.region}}"}}, want: true},
		{name: "condition field", value: models.WorkflowCondition{Field: "query.region", Value: "west"}, want: true},
		{name: "nested branch", value: models.WorkflowBranch{Conditions: []models.WorkflowCondition{{Field: "query.region", Value: "west"}}}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := workflowSnapshotReferencesQuery(test.value); got != test.want {
				t.Fatalf("query dependency = %v, want %v", got, test.want)
			}
		})
	}
}

func TestWorkflowNewSnapshotAllowsIntentionalOptionalInputs(t *testing.T) {
	for _, step := range []models.DynamicWorkflowStep{
		{Type: models.WorkflowStepTypeIf},
		{Type: models.WorkflowStepTypeSetVariable, Config: map[string]interface{}{"values": map[string]interface{}{"region": "{{default(query.region, 'west')}}"}}},
	} {
		event := buildWorkflowStepOutboxEvent(workflowExecutionPayload{}, "current", step)
		if err := validateWorkflowOutboxSnapshot(&event); err != nil {
			t.Fatalf("new complete snapshot rejected optional input: %v", err)
		}
	}
	if err := validateWorkflowOutboxSnapshot(nil); err == nil {
		t.Fatal("nil outbox event was accepted")
	}
}

func TestWorkflowLegacyAuditIdentityDoesNotInventCaller(t *testing.T) {
	for _, payload := range []models.DynamicOutboxPayload{
		{IdentityKind: AccessIdentitySystem, UserID: primitive.NewObjectID().Hex()},
		{UserID: "invalid"},
		{},
	} {
		if got := workflowSnapshotAuditUser(payload); got != nil {
			t.Fatalf("invented legacy audit caller: %#v", got)
		}
	}
}

func TestWorkflowLegacyDelegationRequiresCapturedQueryInputs(t *testing.T) {
	delegated := models.DynamicWorkflowStep{Type: models.WorkflowStepTypeExecuteWorkflow, IsActive: true, Config: map[string]interface{}{"workflowName": "child"}}
	for _, payload := range []models.DynamicOutboxPayload{
		{StepType: delegated.Type, Config: delegated.Config},
		{StepType: models.WorkflowStepTypeIf, Branches: []models.WorkflowBranch{{Steps: []models.DynamicWorkflowStep{delegated}}}},
	} {
		event := &models.DynamicOutboxEvent{Payload: payload}
		if err := validateWorkflowOutboxSnapshot(event); err == nil || !strings.Contains(err.Error(), "delegated") {
			t.Fatalf("ambiguous legacy delegated query inputs were accepted: %v", err)
		}
		event.Payload.Query = map[string]interface{}{"region": "west"}
		if err := validateWorkflowOutboxSnapshot(event); err != nil {
			t.Fatalf("complete delegated legacy snapshot was rejected: %v", err)
		}
	}
}
