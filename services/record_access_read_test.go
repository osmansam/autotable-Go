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

func TestReadRecordAccessFilterComposesRequestRowAndPolicy(t *testing.T) {
	userID := primitive.NewObjectID()
	container := accessTestContainer()
	container.RowAccess = &models.RowAccessRule{Conditions: []models.Condition{{
		Field: "status", Operator: "=", Value: "visible", Roles: []string{"member"},
	}}}
	route := models.RouteSpec{Access: &models.RecordAccessPolicy{Any: []models.RecordAccessRule{{
		Field: "ownerId", Operator: "eq", Value: "{{auth.user.id}}",
	}}}}
	service := &DynamicService{loadAccessUser: func(context.Context, string, string, string) (map[string]interface{}, error) {
		return map[string]interface{}{"_id": userID}, nil
	}}
	identity := AccessIdentityInput{
		IdentityKind: AccessIdentityCaller,
		UserID:       userID.Hex(),
		UserRole:     "member",
		UserRoles:    []string{"member"},
	}

	got, err := service.composeReadRecordAccessFilter(
		context.Background(),
		container,
		route,
		"tenant",
		"project",
		"orders",
		"read",
		identity,
		bson.M{"category": "books"},
	)
	if err != nil {
		t.Fatalf("composeReadRecordAccessFilter() error = %v", err)
	}
	want := bson.M{"$and": []bson.M{
		{"category": "books"},
		{"status": "visible"},
		{"ownerId": userID},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("composeReadRecordAccessFilter() = %#v, want %#v", got, want)
	}
}

func TestReadRecordAccessMixedAnyRules(t *testing.T) {
	userID := primitive.NewObjectID()
	container := accessTestContainer()
	route := models.RouteSpec{Access: &models.RecordAccessPolicy{Any: []models.RecordAccessRule{
		{Field: "ownerId", Operator: "eq", Value: "{{auth.user.id}}"},
		{Context: "{{auth.user.role}}", Operator: "eq", Value: "admin"},
	}}}
	service := &DynamicService{loadAccessUser: func(context.Context, string, string, string) (map[string]interface{}, error) {
		return map[string]interface{}{"_id": userID}, nil
	}}

	member, err := service.composeReadRecordAccessFilter(
		context.Background(), container, route, "tenant", "project", "orders", "read",
		AccessIdentityInput{IdentityKind: AccessIdentityCaller, UserID: userID.Hex(), UserRole: "member"},
		bson.M{"status": "open"},
	)
	if err != nil {
		t.Fatalf("member compose error = %v", err)
	}
	wantMember := bson.M{"$and": []bson.M{{"status": "open"}, {"ownerId": userID}}}
	if !reflect.DeepEqual(member, wantMember) {
		t.Fatalf("member filter = %#v, want %#v", member, wantMember)
	}

	admin, err := service.composeReadRecordAccessFilter(
		context.Background(), container, route, "tenant", "project", "orders", "read",
		AccessIdentityInput{IdentityKind: AccessIdentityCaller, UserID: userID.Hex(), UserRole: "admin"},
		bson.M{"status": "open"},
	)
	if err != nil {
		t.Fatalf("admin compose error = %v", err)
	}
	if !reflect.DeepEqual(admin, bson.M{"status": "open"}) {
		t.Fatalf("admin filter = %#v, want base filter only", admin)
	}
}

func TestReadRecordAccessSystemIdentityFailsClosedWithoutExplicitRule(t *testing.T) {
	container := accessTestContainer()
	route := models.RouteSpec{Access: &models.RecordAccessPolicy{Any: []models.RecordAccessRule{
		{Field: "ownerId", Operator: "eq", Value: "{{auth.user.id}}"},
		{Context: "{{auth.user.role}}", Operator: "eq", Value: "admin"},
	}}}
	service := &DynamicService{}

	got, err := service.composeReadRecordAccessFilter(
		context.Background(), container, route, "tenant", "project", "orders", "read",
		AccessIdentityInput{IdentityKind: AccessIdentitySystem},
		bson.M{},
	)
	if err != nil {
		t.Fatalf("composeReadRecordAccessFilter() error = %v", err)
	}
	if !reflect.DeepEqual(got, recordAccessDenyAllFilter()) {
		t.Fatalf("system filter = %#v, want deny-all", got)
	}
}

func TestReadRecordAccessSelectsPolicyForEverySupportedRoute(t *testing.T) {
	policies := []*models.RecordAccessPolicy{
		{Any: []models.RecordAccessRule{{Field: "status", Operator: "eq", Value: "all"}}},
		{Any: []models.RecordAccessRule{{Field: "status", Operator: "eq", Value: "selection"}}},
		{Any: []models.RecordAccessRule{{Field: "status", Operator: "eq", Value: "one"}}},
		{Any: []models.RecordAccessRule{{Field: "status", Operator: "eq", Value: "search"}}},
		{Any: []models.RecordAccessRule{{Field: "status", Operator: "eq", Value: "filter"}}},
		{Any: []models.RecordAccessRule{{Field: "status", Operator: "eq", Value: "page"}}},
		{Any: []models.RecordAccessRule{{Field: "status", Operator: "eq", Value: "export"}}},
	}
	container := &models.ContainerModel{Routes: models.Routes{
		GetAllDynamicModelItems:               models.RouteSpec{Access: policies[0]},
		GetItemsForSelection:                  models.RouteSpec{Access: policies[1]},
		GetDynamicModelItem:                   models.RouteSpec{Access: policies[2]},
		HandleSearchDynamicModelItem:          models.RouteSpec{Access: policies[3]},
		HandleFilterDynamicModelItem:          models.RouteSpec{Access: policies[4]},
		GetAllDynamicModelItemsWithPagination: models.RouteSpec{Access: policies[5]},
		ExportDynamicModelItems:               models.RouteSpec{Access: policies[6]},
	}}
	operations := []readAccessOperation{
		readAccessGetAll,
		readAccessSelection,
		readAccessGetOne,
		readAccessSearch,
		readAccessFilter,
		readAccessPaginated,
		readAccessExport,
	}
	for index, operation := range operations {
		if got := readAccessRoute(container, operation).Access; got != policies[index] {
			t.Fatalf("readAccessRoute(%q).Access = %p, want %p", operation, got, policies[index])
		}
	}
}

func TestCacheRecordAccessPolicyDisablesSharedCache(t *testing.T) {
	if !recordAccessDisablesSharedCache(models.RouteSpec{Access: &models.RecordAccessPolicy{Any: []models.RecordAccessRule{{
		Field: "status", Operator: "eq", Value: "open",
	}}}}) {
		t.Fatal("recordAccessDisablesSharedCache() = false for protected route")
	}
	if recordAccessDisablesSharedCache(models.RouteSpec{}) {
		t.Fatal("recordAccessDisablesSharedCache() = true for unprotected route")
	}
}

func TestGetRecordAccessNoDocumentReturnsNotFound(t *testing.T) {
	err := recordAccessReadError(mongo.ErrNoDocuments, "Item not found")
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Status != http.StatusNotFound {
		t.Fatalf("recordAccessReadError() = %#v, want 404 ServiceError", err)
	}
}
