package controllers

import (
	"reflect"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestProjectTemplateVisibilityFilterIncludesTenantAndGlobalTemplates(t *testing.T) {
	tenantID := primitive.NewObjectID()

	got := projectTemplateVisibilityFilter(tenantID)
	want := bson.M{
		"isTemplate": true,
		"isActive":   true,
		"$or": bson.A{
			bson.M{"tenantId": tenantID, "templateScope": projectTemplateScopeTenant},
			bson.M{"templateScope": projectTemplateScopeGlobal},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projectTemplateVisibilityFilter() = %#v, want %#v", got, want)
	}
}

func TestProjectTemplateUpdateSetWritesTenantScopeOnly(t *testing.T) {
	includeItems := true
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	input := UpdateProjectTemplateInput{
		IsTemplate:           true,
		TemplateIncludeItems: &includeItems,
		TemplateDescription:  "Starter inventory app",
	}

	got := projectTemplateUpdateSet(input, now)
	want := bson.M{
		"isTemplate":           true,
		"templateScope":        projectTemplateScopeTenant,
		"templateIncludeItems": true,
		"templateDescription":  "Starter inventory app",
		"updatedAt":            now,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projectTemplateUpdateSet() = %#v, want %#v", got, want)
	}
}

func TestProjectTemplateVisibleToTenant(t *testing.T) {
	currentTenant := primitive.NewObjectID()
	otherTenant := primitive.NewObjectID()

	tests := []struct {
		name    string
		project models.Project
		want    bool
	}{
		{
			name: "tenant template in current tenant",
			project: models.Project{
				TenantID:      currentTenant,
				IsActive:      true,
				IsTemplate:    true,
				TemplateScope: projectTemplateScopeTenant,
			},
			want: true,
		},
		{
			name: "global template in other tenant",
			project: models.Project{
				TenantID:      otherTenant,
				IsActive:      true,
				IsTemplate:    true,
				TemplateScope: projectTemplateScopeGlobal,
			},
			want: true,
		},
		{
			name: "other tenant tenant-scoped template",
			project: models.Project{
				TenantID:      otherTenant,
				IsActive:      true,
				IsTemplate:    true,
				TemplateScope: projectTemplateScopeTenant,
			},
			want: false,
		},
		{
			name: "inactive global template",
			project: models.Project{
				TenantID:      otherTenant,
				IsActive:      false,
				IsTemplate:    true,
				TemplateScope: projectTemplateScopeGlobal,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := projectTemplateVisibleToTenant(tt.project, currentTenant); got != tt.want {
				t.Fatalf("projectTemplateVisibleToTenant() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveIncludeTemplateItems(t *testing.T) {
	template := models.Project{TemplateIncludeItems: true}
	if got := resolveIncludeTemplateItems(template, nil); !got {
		t.Fatal("resolveIncludeTemplateItems() = false, want template default true")
	}

	override := false
	if got := resolveIncludeTemplateItems(template, &override); got {
		t.Fatal("resolveIncludeTemplateItems() = true, want override false")
	}
}

func TestShouldCloneDynamicRecordsSkipsAuthContainer(t *testing.T) {
	if shouldCloneDynamicRecords(models.ContainerModel{SchemaName: "auth"}) {
		t.Fatal("shouldCloneDynamicRecords(auth schema) = true, want false")
	}
	if shouldCloneDynamicRecords(models.ContainerModel{SchemaName: "users", IsAuthContainer: true}) {
		t.Fatal("shouldCloneDynamicRecords(auth container) = true, want false")
	}
	if !shouldCloneDynamicRecords(models.ContainerModel{SchemaName: "orders"}) {
		t.Fatal("shouldCloneDynamicRecords(orders) = false, want true")
	}
}

func TestRemapCopiedObjectIDReferences(t *testing.T) {
	sourceParentID := primitive.NewObjectID()
	targetParentID := primitive.NewObjectID()
	externalID := primitive.NewObjectID()
	container := models.ContainerModel{
		SchemaName: "orders",
		Fields: []models.Field{
			{Name: "customer", Type: "objectId", ObjectSchemaName: "customers"},
			{Name: "approvers", Type: "objectIdArray", ObjectSchemaName: "users"},
			{Name: "notes", Type: "string", ObjectSchemaName: "notes"},
		},
	}
	idMap := projectCloneIDMap{
		"customers": {sourceParentID: targetParentID},
		"users":     {externalID: primitive.NewObjectID()},
	}
	doc := bson.M{
		"_id":       primitive.NewObjectID(),
		"customer":  sourceParentID,
		"approvers": bson.A{externalID, primitive.NewObjectID()},
		"notes":     sourceParentID,
	}

	got := remapCopiedObjectIDReferences(doc, container, idMap)

	if got["customer"] != targetParentID {
		t.Fatalf("customer = %#v, want %#v", got["customer"], targetParentID)
	}
	approvers, ok := got["approvers"].(bson.A)
	if !ok || approvers[0] == externalID || approvers[1] == nil {
		t.Fatalf("approvers were not remapped/preserved correctly: %#v", got["approvers"])
	}
	if got["notes"] != sourceParentID {
		t.Fatalf("notes = %#v, want unchanged %#v", got["notes"], sourceParentID)
	}
}
