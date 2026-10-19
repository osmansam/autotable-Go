package services

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const workflowSnapshotVersion = 1

// Old snapshots omitted some execution inputs. Refuse ambiguous jobs rather
// than choosing a branch or sending a request with silently missing values.
func validateWorkflowOutboxSnapshot(event *models.DynamicOutboxEvent) error {
	if event == nil {
		return fmt.Errorf("workflow outbox event is required")
	}
	version := event.Payload.WorkflowSnapshotVersion
	if version < 0 || version > workflowSnapshotVersion {
		return fmt.Errorf("unsupported workflow snapshot version %d", version)
	}
	if version == workflowSnapshotVersion {
		return nil
	}
	if event.Payload.StepType == models.WorkflowStepTypeIf && len(event.Payload.Branches) == 0 && len(event.Payload.Conditions) == 0 {
		return fmt.Errorf("legacy workflow snapshot may be missing its root condition; requeue from the original workflow inputs")
	}
	if event.Payload.Query == nil {
		step := workflowStepFromOutboxEvent(event)
		if workflowCapturedStepDelegates(step) {
			return fmt.Errorf("legacy delegated workflow snapshot is missing query inputs; requeue from the original workflow inputs")
		}
		if workflowSnapshotReferencesQuery(step) {
			return fmt.Errorf("legacy workflow snapshot is missing query inputs; requeue from the original workflow inputs")
		}
	}
	return nil
}

func workflowCapturedStepDelegates(step models.DynamicWorkflowStep) bool {
	if !step.IsActive {
		return false
	}
	if step.Type == models.WorkflowStepTypeExecuteWorkflow {
		return true
	}
	for _, children := range [][]models.DynamicWorkflowStep{step.Steps, step.ElseSteps} {
		for _, child := range children {
			if workflowCapturedStepDelegates(child) {
				return true
			}
		}
	}
	for _, branch := range step.Branches {
		for _, child := range branch.Steps {
			if workflowCapturedStepDelegates(child) {
				return true
			}
		}
	}
	return false
}

func workflowSnapshotReferencesQuery(value interface{}) bool {
	switch typed := value.(type) {
	case string:
		for {
			start := strings.Index(typed, "{{")
			if start < 0 {
				return false
			}
			typed = typed[start+2:]
			end := strings.Index(typed, "}}")
			if end < 0 {
				return false
			}
			if workflowExpressionReferencesQuery(typed[:end]) {
				return true
			}
			typed = typed[end+2:]
		}
	case models.WorkflowCondition:
		return strings.HasPrefix(typed.Field, "query.") || workflowSnapshotReferencesQuery(typed.Value) || workflowSnapshotReferencesQuery(typed.Conditions)
	}
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return false
	}
	switch reflected.Kind() {
	case reflect.Interface, reflect.Pointer:
		return !reflected.IsNil() && workflowSnapshotReferencesQuery(reflected.Elem().Interface())
	case reflect.Map:
		iterator := reflected.MapRange()
		for iterator.Next() {
			if workflowSnapshotReferencesQuery(iterator.Value().Interface()) {
				return true
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < reflected.Len(); index++ {
			if workflowSnapshotReferencesQuery(reflected.Index(index).Interface()) {
				return true
			}
		}
	case reflect.Struct:
		for index := 0; index < reflected.NumField(); index++ {
			if reflected.Field(index).CanInterface() && workflowSnapshotReferencesQuery(reflected.Field(index).Interface()) {
				return true
			}
		}
	}
	return false
}

func workflowExpressionReferencesQuery(expression string) bool {
	expression = strings.TrimSpace(expression)
	if len(expression) >= 2 && ((expression[0] == '\'' && expression[len(expression)-1] == '\'') || (expression[0] == '"' && expression[len(expression)-1] == '"')) {
		return false
	}
	if strings.HasPrefix(expression, "query.") {
		return true
	}
	if _, args, ok := workflowFunctionCall(expression); ok {
		for _, arg := range args {
			if workflowExpressionReferencesQuery(arg) {
				return true
			}
		}
	}
	return false
}

func workflowSnapshotAuditUser(payload models.DynamicOutboxPayload) *models.AuditUser {
	if payload.AuditUser != nil {
		return cloneWorkflowSnapshot(payload.AuditUser)
	}
	if payload.IdentityKind == AccessIdentitySystem || payload.UserID == "" {
		return nil
	}
	id, err := primitive.ObjectIDFromHex(payload.UserID)
	if err != nil {
		return nil
	}
	roles := append([]string(nil), payload.UserRoles...)
	if len(roles) == 0 && payload.UserRole != "" {
		roles = []string{payload.UserRole}
	}
	email, _ := payload.CurrentUser["email"].(string)
	displayName, _ := payload.CurrentUser["displayName"].(string)
	return &models.AuditUser{ID: id, Email: email, DisplayName: displayName, Roles: roles}
}
