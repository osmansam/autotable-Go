package services

import (
	"context"
	"testing"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestRunWorkflowDefinitionResolvesNestedCurrentUserFields(t *testing.T) {
	userID := primitive.NewObjectID().Hex()
	service := &DynamicService{
		loadAccessUser: func(context.Context, string, string, string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"_id":   userID,
				"email": "ada@example.com",
				"profile": map[string]interface{}{
					"plan": "pro",
				},
			}, nil
		},
	}
	payload := workflowExecutionPayload{
		TenantID:   "tenant",
		ProjectID:  "project",
		SchemaName: "orders",
		UserID:     userID,
	}
	workflow := models.DynamicWorkflow{
		Name:     "current-user",
		Trigger:  models.WorkflowTriggerManual,
		Mode:     models.WorkflowModeTransactional,
		IsActive: true,
		Steps: []models.DynamicWorkflowStep{{
			Name:     "return-user",
			Type:     models.WorkflowStepTypeReturn,
			IsActive: true,
			Config: map[string]interface{}{
				"value": map[string]interface{}{
					"email": "{{user.email}}",
					"plan":  "{{user.profile.plan}}",
				},
			},
		}},
	}

	if err := service.runWorkflowDefinition(context.Background(), &payload, workflow); err != nil {
		t.Fatalf("runWorkflowDefinition() error = %v", err)
	}
	got, ok := payload.ReturnValue.(map[string]interface{})
	if !ok || got["email"] != "ada@example.com" || got["plan"] != "pro" {
		t.Fatalf("ReturnValue = %#v, want current user email and plan", payload.ReturnValue)
	}
}

func TestSanitizeAccessUserRemovesEveryHashedField(t *testing.T) {
	container := &models.ContainerModel{Fields: []models.Field{
		{Name: "email", Type: "string"},
		{Name: "password", Type: "string", IsHashed: true},
		{Name: "pin", Type: "string", IsHashed: true},
		{Name: "security", Type: "object", Children: []models.Field{
			{Name: "hint", Type: "string"},
			{Name: "recoveryCode", Type: "string", IsHashed: true},
		}},
	}}
	user := map[string]interface{}{
		"_id":      primitive.NewObjectID(),
		"email":    "ada@example.com",
		"password": "secret-hash",
		"pin":      "pin-hash",
		"security": map[string]interface{}{
			"hint":         "first pet",
			"recoveryCode": "recovery-hash",
		},
	}

	got := sanitizeAccessUser(container, user)
	if got["email"] != "ada@example.com" {
		t.Fatalf("email = %#v, want ada@example.com", got["email"])
	}
	if _, exists := got["password"]; exists {
		t.Fatal("password must not be exposed to workflow context")
	}
	if _, exists := got["pin"]; exists {
		t.Fatal("pin must not be exposed to workflow context")
	}
	security, ok := got["security"].(map[string]interface{})
	if !ok || security["hint"] != "first pet" {
		t.Fatalf("security = %#v, want non-hashed nested fields", got["security"])
	}
	if _, exists := security["recoveryCode"]; exists {
		t.Fatal("nested recoveryCode must not be exposed to workflow context")
	}
	if _, exists := user["password"]; !exists {
		t.Fatal("sanitizeAccessUser must not mutate the repository document")
	}
	originalSecurity := user["security"].(map[string]interface{})
	if originalSecurity["recoveryCode"] != "recovery-hash" {
		t.Fatal("sanitizeAccessUser must not mutate nested repository fields")
	}
}

func TestBuildWorkflowStepOutboxEventCarriesCurrentUserSnapshot(t *testing.T) {
	payload := workflowExecutionPayload{
		TenantID:     "tenant",
		ProjectID:    "project",
		SchemaName:   "orders",
		IdentityKind: AccessIdentityCaller,
		UserID:       primitive.NewObjectID().Hex(),
		UserRoles:    []string{"admin", "operator"},
		AuditUser:    &models.AuditUser{Roles: []string{"admin"}},
		CurrentUser: map[string]interface{}{
			"email": "ada@example.com",
		},
	}
	step := models.DynamicWorkflowStep{ID: "notify", Name: "notify", Type: models.WorkflowStepTypeCreateNotification}

	event := buildWorkflowStepOutboxEvent(payload, "personalized", step)
	if event.Payload.CurrentUser["email"] != "ada@example.com" {
		t.Fatalf("outbox currentUser = %#v, want sanitized user snapshot", event.Payload.CurrentUser)
	}
	if event.Payload.UserRole != "admin" {
		t.Fatalf("outbox userRole = %q, want admin", event.Payload.UserRole)
	}
	if event.Payload.IdentityKind != AccessIdentityCaller {
		t.Fatalf("outbox identityKind = %q, want caller", event.Payload.IdentityKind)
	}
	if len(event.Payload.UserRoles) != 2 || event.Payload.UserRoles[1] != "operator" {
		t.Fatalf("outbox userRoles = %#v, want caller roles", event.Payload.UserRoles)
	}
}

func TestWorkflowRecordAccessIdentityCannotBeOverriddenByVariables(t *testing.T) {
	payload := workflowExecutionPayload{
		IdentityKind: AccessIdentityCaller,
		UserID:       "trusted-user",
		UserRole:     "member",
		UserRoles:    []string{"member"},
		Variables: map[string]interface{}{
			"auth": map[string]interface{}{
				"identity": map[string]interface{}{"kind": AccessIdentitySystem},
				"user":     map[string]interface{}{"id": "attacker", "role": "admin"},
			},
		},
	}

	got := workflowAccessIdentity(payload)
	if got.IdentityKind != AccessIdentityCaller || got.UserID != "trusted-user" || got.UserRole != "member" {
		t.Fatalf("workflowAccessIdentity() = %#v, want trusted payload identity", got)
	}
}

func TestOutboxIdentityKindAndRolesRoundTrip(t *testing.T) {
	payload := workflowExecutionPayload{
		TenantID:     "tenant",
		ProjectID:    "project",
		SchemaName:   "orders",
		IdentityKind: AccessIdentityCaller,
		UserID:       "user-1",
		UserRole:     "member",
		UserRoles:    []string{"member", "editor"},
		CurrentUser:  map[string]interface{}{"email": "ada@example.com"},
	}
	event := buildWorkflowStepOutboxEvent(payload, "workflow", models.DynamicWorkflowStep{ID: "step", Type: models.WorkflowStepTypeReturn})
	restored := workflowPayloadFromOutboxEvent(&event)

	if restored.IdentityKind != AccessIdentityCaller || restored.UserID != "user-1" || restored.UserRole != "member" {
		t.Fatalf("restored identity = %#v, want persisted caller", restored)
	}
	if len(restored.UserRoles) != 2 || restored.UserRoles[1] != "editor" {
		t.Fatalf("restored roles = %#v, want persisted roles", restored.UserRoles)
	}
	if restored.CurrentUser["email"] != "ada@example.com" {
		t.Fatalf("restored current user = %#v", restored.CurrentUser)
	}
}

func TestWorkflowUserRoleUsesPersistedOutboxRole(t *testing.T) {
	payload := workflowExecutionPayload{UserRole: "admin"}

	if got := workflowUserRole(payload); got != "admin" {
		t.Fatalf("workflowUserRole() = %q, want admin", got)
	}
	if got, ok := workflowTemplateValue("user.role", payload); !ok || got != "admin" {
		t.Fatalf("workflowTemplateValue(user.role) = %#v, %v; want admin, true", got, ok)
	}
}

func TestRunWorkflowsDoesNotLoadUserWithoutMatchingWorkflow(t *testing.T) {
	loads := 0
	service := &DynamicService{
		loadAccessUser: func(context.Context, string, string, string) (map[string]interface{}, error) {
			loads++
			return map[string]interface{}{"email": "ada@example.com"}, nil
		},
	}
	payload := workflowExecutionPayload{
		UserID: primitive.NewObjectID().Hex(),
		Container: &models.ContainerModel{Workflows: []models.DynamicWorkflow{{
			Name:     "after-create-only",
			Trigger:  models.WorkflowTriggerAfterCreate,
			Mode:     models.WorkflowModeTransactional,
			IsActive: true,
		}}},
	}

	if err := service.runWorkflows(nil, payload, models.WorkflowTriggerBeforeCreate, models.WorkflowModeTransactional); err != nil {
		t.Fatalf("runWorkflows() error = %v", err)
	}
	if loads != 0 {
		t.Fatalf("current user loads = %d, want 0 when no workflow matches", loads)
	}
}

func TestWorkflowConditionReadsCurrentUserField(t *testing.T) {
	userID := primitive.NewObjectID().Hex()
	payload := workflowExecutionPayload{
		CurrentUser: map[string]interface{}{"accountType": "business"},
		UserID:      userID,
		UserRole:    "admin",
	}
	conditions := []models.WorkflowCondition{
		{Field: "user.accountType", Operator: models.WorkflowConditionEqual, Value: "business"},
		{Field: "user.id", Operator: models.WorkflowConditionEqual, Value: userID},
		{Field: "user._id", Operator: models.WorkflowConditionEqual, Value: userID},
		{Field: "user.role", Operator: models.WorkflowConditionEqual, Value: "admin"},
	}

	for _, condition := range conditions {
		if !workflowConditionMatches(condition, payload) {
			t.Fatalf("workflowConditionMatches(%s) = false, want true", condition.Field)
		}
	}
}
