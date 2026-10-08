package services

import (
	"context"
	"net/http"
	"testing"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestCreateRecordAccessReappliesAssignmentAfterWorkflowMutation(t *testing.T) {
	userID := primitive.NewObjectID()
	attackerID := primitive.NewObjectID()
	container := accessTestContainer()
	policy := &models.RecordAccessPolicy{
		Assign: map[string]interface{}{"ownerId": "{{auth.user.id}}"},
		Any: []models.RecordAccessRule{{
			Field: "ownerId", Operator: "eq", Value: "{{auth.user.id}}",
		}},
	}
	ctx := callerAccessContext(userID.Hex())
	clientRecord := map[string]interface{}{"ownerId": attackerID, "status": "open"}

	beforeWorkflow, err := applyCreateRecordAccessAssignments(container, policy, ctx, clientRecord)
	if err != nil {
		t.Fatalf("applyCreateRecordAccessAssignments(before workflow) error = %v", err)
	}
	beforeWorkflow["ownerId"] = attackerID // simulate a before_create workflow spoof

	finalRecord, err := applyCreateRecordAccessAssignments(container, policy, ctx, beforeWorkflow)
	if err != nil {
		t.Fatalf("applyCreateRecordAccessAssignments(after workflow) error = %v", err)
	}
	if finalRecord["ownerId"] != userID {
		t.Fatalf("final ownerId = %#v, want %s", finalRecord["ownerId"], userID.Hex())
	}
	if err := authorizePreparedCreateRecords(container, policy, ctx, finalRecord); err != nil {
		t.Fatalf("authorizePreparedCreateRecords() error = %v", err)
	}
	if clientRecord["ownerId"] != attackerID {
		t.Fatal("trusted assignment mutated the parsed caller record")
	}
}

func TestCreateRecordAccessAllowsAssignmentOnlyPolicy(t *testing.T) {
	userID := primitive.NewObjectID()
	policy := &models.RecordAccessPolicy{Assign: map[string]interface{}{"ownerId": "{{auth.user.id}}"}}
	record, err := applyCreateRecordAccessAssignments(accessTestContainer(), policy, callerAccessContext(userID.Hex()), map[string]interface{}{})
	if err != nil {
		t.Fatalf("applyCreateRecordAccessAssignments() error = %v", err)
	}
	if record["ownerId"] != userID {
		t.Fatalf("ownerId = %#v, want %s", record["ownerId"], userID.Hex())
	}
	if err := authorizePreparedCreateRecords(accessTestContainer(), policy, callerAccessContext(userID.Hex()), record); err != nil {
		t.Fatalf("assignment-only authorize error = %v", err)
	}
}

func TestCreateRecordAccessDeniedReturnsForbidden(t *testing.T) {
	policy := &models.RecordAccessPolicy{Any: []models.RecordAccessRule{{
		Field: "status", Operator: "eq", Value: "approved",
	}}}

	err := authorizePreparedCreateRecords(
		accessTestContainer(),
		policy,
		callerAccessContext(primitive.NewObjectID().Hex()),
		map[string]interface{}{"status": "draft"},
	)
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Status != http.StatusForbidden {
		t.Fatalf("authorizePreparedCreateRecords() error = %#v, want 403 ServiceError", err)
	}
}

func TestCreateRecordAccessMissingAssignmentReturnsStructuredError(t *testing.T) {
	policy := &models.RecordAccessPolicy{Assign: map[string]interface{}{"ownerId": "{{auth.user.id}}"}}

	_, err := applyCreateRecordAccessAssignments(
		accessTestContainer(),
		policy,
		recordAccessContext{IdentityKind: accessIdentitySystem},
		map[string]interface{}{},
	)
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Status != http.StatusInternalServerError {
		t.Fatalf("applyCreateRecordAccessAssignments() error = %#v, want structured 500 ServiceError", err)
	}
}

func TestBulkCreateRecordAccessDenialRejectsAtomicBatch(t *testing.T) {
	policy := &models.RecordAccessPolicy{Any: []models.RecordAccessRule{{
		Field: "status", Operator: "eq", Value: "approved",
	}}}
	records := []map[string]interface{}{
		{"status": "approved"},
		{"status": "draft"},
	}

	err := authorizePreparedCreateRecords(accessTestContainer(), policy, callerAccessContext(primitive.NewObjectID().Hex()), records...)
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Status != http.StatusForbidden {
		t.Fatalf("authorizePreparedCreateRecords() error = %#v, want atomic batch denial", err)
	}
}

func TestCreateRecordAccessContextLoadsSanitizedCurrentUser(t *testing.T) {
	service := &DynamicService{loadAccessUser: func(context.Context, string, string, string) (map[string]interface{}, error) {
		return map[string]interface{}{"profile": map[string]interface{}{"departmentId": "dept-1"}}, nil
	}}
	identity := AccessIdentityInput{
		IdentityKind: AccessIdentityCaller,
		UserID:       primitive.NewObjectID().Hex(),
		UserRole:     "member",
		UserRoles:    []string{"member"},
	}

	ctx, err := service.buildRecordAccessContext(context.Background(), "tenant", "project", "orders", "create", identity)
	if err != nil {
		t.Fatalf("buildRecordAccessContext() error = %v", err)
	}
	if got, ok := lookupAccessPath(buildAccessAuthContext(ctx), "user.profile.departmentId"); !ok || got != "dept-1" {
		t.Fatalf("loaded auth user department = %#v, %v", got, ok)
	}
}
