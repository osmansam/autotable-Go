package services

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func mutationAccessFixture() (*models.ContainerModel, models.RouteSpec, primitive.ObjectID, primitive.ObjectID) {
	userID := primitive.NewObjectID()
	otherID := primitive.NewObjectID()
	container := accessTestContainer()
	container.RowAccess = &models.RowAccessRule{Conditions: []models.Condition{{
		Field: "status", Operator: "=", Value: "open", Roles: []string{"member"},
	}}}
	route := models.RouteSpec{Access: &models.RecordAccessPolicy{Any: []models.RecordAccessRule{
		{Field: "ownerId", Operator: "eq", Value: "{{auth.user.id}}"},
		{Context: "{{auth.user.role}}", Operator: "eq", Value: "admin"},
	}}}
	return container, route, userID, otherID
}

func TestUpdateRecordAccessSelectorCombinesIDRowAndPolicy(t *testing.T) {
	container, route, userID, _ := mutationAccessFixture()
	service := &DynamicService{loadAccessUser: func(context.Context, string, string, string) (map[string]interface{}, error) {
		return map[string]interface{}{"_id": userID}, nil
	}}
	identity := AccessIdentityInput{IdentityKind: AccessIdentityCaller, UserID: userID.Hex(), UserRole: "member"}
	accessCtx, err := service.buildMutationRecordAccessContext(context.Background(), route, "tenant", "project", "orders", "update", identity)
	if err != nil {
		t.Fatalf("buildMutationRecordAccessContext() error = %v", err)
	}
	recordID := primitive.NewObjectID()
	got, err := mutationRecordAccessSelector(container, route, identity, accessCtx, bson.M{"_id": recordID})
	if err != nil {
		t.Fatalf("mutationRecordAccessSelector() error = %v", err)
	}
	want := bson.M{"$and": []bson.M{
		{"_id": recordID},
		{"status": "open"},
		{"ownerId": userID},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mutationRecordAccessSelector() = %#v, want %#v", got, want)
	}
}

func TestUpdateRecordAccessRejectsOwnershipTransferAfterRequestOrWorkflow(t *testing.T) {
	container, route, userID, otherID := mutationAccessFixture()
	ctx := callerAccessContext(userID.Hex())

	for _, source := range []string{"request", "before_update workflow"} {
		t.Run(source, func(t *testing.T) {
			finalRecord := map[string]interface{}{"ownerId": otherID, "status": "open"}
			err := authorizeFinalUpdateRecord(container, route.Access, ctx, finalRecord)
			serviceErr, ok := err.(*ServiceError)
			if !ok || serviceErr.Status != http.StatusNotFound {
				t.Fatalf("authorizeFinalUpdateRecord() error = %#v, want not found", err)
			}
		})
	}
}

func TestUpdateRecordAccessAllowsAdminOwnershipTransfer(t *testing.T) {
	container, route, _, otherID := mutationAccessFixture()
	ctx := callerAccessContext(primitive.NewObjectID().Hex())
	ctx.UserRole = "admin"

	if err := authorizeFinalUpdateRecord(container, route.Access, ctx, map[string]interface{}{"ownerId": otherID}); err != nil {
		t.Fatalf("authorizeFinalUpdateRecord() admin error = %v", err)
	}
}

func TestUpdateRecordAccessConcurrentSelectorMissReturnsNotFound(t *testing.T) {
	err := updateResultNotFound(&mongo.UpdateResult{MatchedCount: 0})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Status != http.StatusNotFound {
		t.Fatalf("updateResultNotFound() = %#v, want 404", err)
	}
	if err := updateResultNotFound(&mongo.UpdateResult{MatchedCount: 1}); err != nil {
		t.Fatalf("updateResultNotFound(matched) error = %v", err)
	}
}

func TestDeleteRecordAccessUsesSameConstrainedSelector(t *testing.T) {
	container, route, userID, _ := mutationAccessFixture()
	identity := AccessIdentityInput{IdentityKind: AccessIdentityCaller, UserID: userID.Hex(), UserRole: "member"}
	ctx := callerAccessContext(userID.Hex())
	recordID := primitive.NewObjectID()

	got, err := mutationRecordAccessSelector(container, route, identity, ctx, bson.M{"_id": recordID})
	if err != nil {
		t.Fatalf("mutationRecordAccessSelector() error = %v", err)
	}
	want := bson.M{"$and": []bson.M{
		{"_id": recordID},
		{"status": "open"},
		{"ownerId": userID},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("delete selector = %#v, want %#v", got, want)
	}
}

func TestBulkUpdateDeleteRecordAccessKeepsUnauthorizedItemsNotFound(t *testing.T) {
	container, route, userID, otherID := mutationAccessFixture()
	ctx := callerAccessContext(userID.Hex())
	records := []map[string]interface{}{
		{"ownerId": userID, "status": "open"},
		{"ownerId": otherID, "status": "open"},
	}

	if err := authorizeFinalUpdateRecord(container, route.Access, ctx, records[0]); err != nil {
		t.Fatalf("authorized bulk item error = %v", err)
	}
	err := authorizeFinalUpdateRecord(container, route.Access, ctx, records[1])
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Status != http.StatusNotFound {
		t.Fatalf("unauthorized bulk item error = %#v, want not found", err)
	}
}
