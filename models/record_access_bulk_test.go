package models

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestValidateBulkRoutePolicyUpdate(t *testing.T) {
	// This is the shape sent by the panel's bulk editor in one container update.
	const payload = `{
		"fields": [{"name": "ownerId", "type": "objectId"}],
		"routes": {
			"createDynamicModelItem": {"isAuthenticated": true, "access": {"assign": {"ownerId": "{{auth.user.id}}"}, "any": [{"field": "ownerId", "operator": "eq", "value": "{{auth.user.id}}"}]}},
			"createMultipleDynamicModelItem": {"isAuthenticated": true, "access": {"assign": {"ownerId": "{{auth.user.id}}"}, "any": [{"field": "ownerId", "operator": "eq", "value": "{{auth.user.id}}"}]}},
			"getAllDynamicModelItems": {"isAuthenticated": true, "access": {"any": [{"field": "ownerId", "operator": "eq", "value": "{{auth.user.id}}"}]}},
			"getAllDynamicModelItemsWithPagination": {"isAuthenticated": true, "access": {"any": [{"field": "ownerId", "operator": "eq", "value": "{{auth.user.id}}"}]}},
			"handleSearchDynamicModelItem": {"isAuthenticated": true, "access": {"any": [{"field": "ownerId", "operator": "eq", "value": "{{auth.user.id}}"}]}},
			"handleFilterDynamicModelItem": {"isAuthenticated": true, "access": {"any": [{"field": "ownerId", "operator": "eq", "value": "{{auth.user.id}}"}]}},
			"deleteDynamicModelItem": {"isAuthenticated": true, "access": {"any": [{"field": "ownerId", "operator": "eq", "value": "{{auth.user.id}}"}]}},
			"updateDynamicModelItem": {"isAuthenticated": true, "access": {"any": [{"field": "ownerId", "operator": "eq", "value": "{{auth.user.id}}"}]}},
			"updateMultipleDynamicModelItem": {"isAuthenticated": true, "access": {"any": [{"field": "ownerId", "operator": "eq", "value": "{{auth.user.id}}"}]}},
			"getDynamicModelItem": {"isAuthenticated": true, "access": {"any": [{"field": "ownerId", "operator": "eq", "value": "{{auth.user.id}}"}]}},
			"deleteMultipleDynamicModelItem": {"isAuthenticated": true, "access": {"any": [{"field": "ownerId", "operator": "eq", "value": "{{auth.user.id}}"}]}},
			"exportDynamicModelItems": {"isAuthenticated": true, "access": {"any": [{"field": "ownerId", "operator": "eq", "value": "{{auth.user.id}}"}]}},
			"getItemsForSelection": {"isAuthenticated": true, "access": {"any": [{"field": "ownerId", "operator": "eq", "value": "{{auth.user.id}}"}]}}
		}
	}`
	var container ContainerModel
	if err := json.Unmarshal([]byte(payload), &container); err != nil {
		t.Fatalf("decode bulk policy update: %v", err)
	}
	if err := ValidateRecordAccessPolicies(&container); err != nil {
		t.Fatalf("validate bulk policy update: %v", err)
	}

	wantRules := []RecordAccessRule{{Field: "ownerId", Operator: "eq", Value: "{{auth.user.id}}"}}
	for name, route := range map[string]RouteSpec{
		"create":      container.Routes.CreateDynamicModelItem,
		"bulk create": container.Routes.CreateMultipleDynamicModelItem,
		"list":        container.Routes.GetAllDynamicModelItems,
		"pagination":  container.Routes.GetAllDynamicModelItemsWithPagination,
		"search":      container.Routes.HandleSearchDynamicModelItem,
		"filter":      container.Routes.HandleFilterDynamicModelItem,
		"delete":      container.Routes.DeleteDynamicModelItem,
		"update":      container.Routes.UpdateDynamicModelItem,
		"bulk update": container.Routes.UpdateMultipleDynamicModelItem,
		"get":         container.Routes.GetDynamicModelItem,
		"bulk delete": container.Routes.DeleteMultipleDynamicModelItem,
		"export":      container.Routes.ExportDynamicModelItems,
		"selection":   container.Routes.GetItemsForSelection,
	} {
		if !route.IsAuthenticated || route.Access == nil || !reflect.DeepEqual(route.Access.Any, wantRules) {
			t.Fatalf("%s did not receive the shared authenticated policy: %#v", name, route)
		}
	}
	if got := container.Routes.CreateMultipleDynamicModelItem.Access.Assign["ownerId"]; got != "{{auth.user.id}}" {
		t.Fatalf("bulk create assignment = %#v", got)
	}
	if container.Routes.GetPipeline.Access != nil || container.Routes.TestPipeline.Access != nil {
		t.Fatal("bulk policy update affected unsupported pipeline routes")
	}

	// An invalid policy on any one target must reject the whole configuration.
	container.Routes.UpdateMultipleDynamicModelItem.Access.Assign = map[string]interface{}{"ownerId": "{{auth.user.id}}"}
	if err := ValidateRecordAccessPolicies(&container); err == nil || !strings.Contains(err.Error(), "updateMultipleDynamicModelItem") {
		t.Fatalf("invalid bulk update assignment error = %v", err)
	}
}
