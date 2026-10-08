package services

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
)

func TestValidateWorkflowRejectsInvalidUpdateExcludeFields(t *testing.T) {
	for _, excludeFields := range []interface{}{
		"owner_user_id",
		[]interface{}{""},
		[]interface{}{"$owner_user_id"},
	} {
		workflow := models.DynamicWorkflow{
			Name:    "invalid-update-exclusions",
			Trigger: models.WorkflowTriggerManual,
			Mode:    models.WorkflowModeTransactional,
			Steps: []models.DynamicWorkflowStep{{
				Name:     "update_pet",
				Type:     models.WorkflowStepTypeUpdateRecord,
				IsActive: true,
				Config: map[string]interface{}{
					"filter":        map[string]interface{}{"_id": "{{record.petId}}"},
					"update":        "{{record.updates}}",
					"excludeFields": excludeFields,
				},
			}},
		}
		err := ValidateWorkflow(workflow)
		if err == nil || !strings.Contains(err.Error(), "excludeFields") {
			t.Fatalf("ValidateWorkflow(excludeFields=%#v) error = %v, want excludeFields validation", excludeFields, err)
		}
	}
}

func TestWorkflowUpdateExcludedFieldsAcceptsBSONArray(t *testing.T) {
	encoded, err := bson.Marshal(bson.M{
		"excludeFields": bson.A{"owner_user_id", "_id"},
	})
	if err != nil {
		t.Fatalf("marshal workflow config: %v", err)
	}

	var config map[string]interface{}
	if err := bson.Unmarshal(encoded, &config); err != nil {
		t.Fatalf("unmarshal workflow config: %v", err)
	}

	got, err := workflowUpdateExcludedFields(config)
	if err != nil {
		t.Fatalf("workflowUpdateExcludedFields(BSON array) error = %v", err)
	}
	want := []string{"owner_user_id", "_id"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("workflowUpdateExcludedFields(BSON array) = %#v, want %#v", got, want)
	}
}

func TestFilterWorkflowUpdateFieldsProtectsPlainAndOperatorUpdates(t *testing.T) {
	tests := []struct {
		name   string
		update map[string]interface{}
		want   map[string]interface{}
	}{
		{
			name: "plain update",
			update: map[string]interface{}{
				"name":          "Milo",
				"owner_user_id": "another-profile",
				"_id":           "another-pet",
			},
			want: map[string]interface{}{"name": "Milo"},
		},
		{
			name: "operator update",
			update: map[string]interface{}{
				"$set": map[string]interface{}{
					"name":                     "Milo",
					"owner_user_id":            "another-profile",
					"owner_user_id.unexpected": true,
				},
				"$unset": map[string]interface{}{
					"notes":         "",
					"owner_user_id": "",
				},
				"$inc": map[string]interface{}{"sync_version": 1},
			},
			want: map[string]interface{}{
				"$set":   map[string]interface{}{"name": "Milo"},
				"$unset": map[string]interface{}{"notes": ""},
				"$inc":   map[string]interface{}{"sync_version": 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := cloneWorkflowMap(tt.update)
			got, err := filterWorkflowUpdateFields(tt.update, []string{"owner_user_id", "_id"})
			if err != nil {
				t.Fatalf("filterWorkflowUpdateFields() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("filterWorkflowUpdateFields() = %#v, want %#v", got, tt.want)
			}
			if !reflect.DeepEqual(tt.update, original) {
				t.Fatalf("filterWorkflowUpdateFields() mutated input: %#v", tt.update)
			}
		})
	}
}

func TestFilterWorkflowUpdateFieldsRejectsEmptyResult(t *testing.T) {
	_, err := filterWorkflowUpdateFields(
		map[string]interface{}{"owner_user_id": "another-profile"},
		[]string{"owner_user_id"},
	)
	if err == nil || !strings.Contains(err.Error(), "no fields") {
		t.Fatalf("filterWorkflowUpdateFields() error = %v, want no fields error", err)
	}
	businessErr, ok := err.(*workflowBusinessError)
	if !ok || businessErr.Status != http.StatusBadRequest {
		t.Fatalf("filterWorkflowUpdateFields() error = %#v, want 400 business error", err)
	}
}

func TestPersonalizedPetUpdateWorkflowExampleIsValid(t *testing.T) {
	data, err := os.ReadFile("../docs/examples/miywo/update_pet_personalized.workflow.json")
	if err != nil {
		t.Fatalf("read personalized pet workflow example: %v", err)
	}
	var workflow models.DynamicWorkflow
	if err := json.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("decode personalized pet workflow example: %v", err)
	}
	if err := ValidateWorkflow(workflow); err != nil {
		t.Fatalf("ValidateWorkflow(personalized pet update) error = %v", err)
	}
	if workflow.Name != "update_pet_personalized" || !workflow.IsAuthenticated {
		t.Fatalf("workflow identity settings = (%q, %v), want authenticated update_pet_personalized", workflow.Name, workflow.IsAuthenticated)
	}

	steps := make(map[string]models.DynamicWorkflowStep, len(workflow.Steps))
	for _, step := range workflow.Steps {
		steps[step.Name] = step
	}
	petFilter, _ := steps["pet"].Config["filter"].(map[string]interface{})
	if petFilter["_id"] != "{{record.petId}}" {
		t.Fatalf("pet lookup filter = %#v, want requested pet id", petFilter)
	}
	profileFilter, _ := steps["current_profile"].Config["filter"].(map[string]interface{})
	if profileFilter["_id"] != "{{steps.pet.items.0.owner_user_id}}" || profileFilter["auth_id"] != "{{user._id}}" {
		t.Fatalf("profile filter = %#v, want pet owner and trusted workflow user id", profileFilter)
	}
	updateFilter, _ := steps["update_pet"].Config["filter"].(map[string]interface{})
	if updateFilter["owner_user_id"] != "{{steps.pet.items.0.owner_user_id}}" {
		t.Fatalf("pet owner filter = %#v, want original pet owner id", updateFilter)
	}
	updatedPetFilter, _ := steps["updated_pet"].Config["filter"].(map[string]interface{})
	if updatedPetFilter["owner_user_id"] != "{{steps.pet.items.0.owner_user_id}}" {
		t.Fatalf("updated pet filter = %#v, want original pet owner id", updatedPetFilter)
	}
	excluded, err := workflowUpdateExcludedFields(steps["update_pet"].Config)
	if err != nil {
		t.Fatalf("workflowUpdateExcludedFields(example) error = %v", err)
	}
	if !reflect.DeepEqual(excluded, []string{"owner_user_id", "_id"}) {
		t.Fatalf("excluded fields = %#v, want owner and immutable id", excluded)
	}
}

func TestPersonalizedPetDeleteWorkflowExampleIsValid(t *testing.T) {
	data, err := os.ReadFile("../docs/examples/miywo/delete_pet_personalized.workflow.json")
	if err != nil {
		t.Fatalf("read personalized pet delete workflow example: %v", err)
	}
	var workflow models.DynamicWorkflow
	if err := json.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("decode personalized pet delete workflow example: %v", err)
	}
	if err := ValidateWorkflow(workflow); err != nil {
		t.Fatalf("ValidateWorkflow(personalized pet delete) error = %v", err)
	}
	if workflow.Name != "delete_pet_personalized" || !workflow.IsAuthenticated {
		t.Fatalf("workflow identity settings = (%q, %v), want authenticated delete_pet_personalized", workflow.Name, workflow.IsAuthenticated)
	}

	steps := make(map[string]models.DynamicWorkflowStep, len(workflow.Steps))
	for _, step := range workflow.Steps {
		steps[step.Name] = step
	}
	petFilter, _ := steps["pet"].Config["filter"].(map[string]interface{})
	if petFilter["_id"] != "{{record.petId}}" {
		t.Fatalf("pet lookup filter = %#v, want requested pet id", petFilter)
	}
	profileFilter, _ := steps["current_profile"].Config["filter"].(map[string]interface{})
	if profileFilter["_id"] != "{{steps.pet.items.0.owner_user_id}}" || profileFilter["auth_id"] != "{{user._id}}" {
		t.Fatalf("profile filter = %#v, want pet owner and trusted workflow user id", profileFilter)
	}
	deleteFilter, _ := steps["delete_pet"].Config["filter"].(map[string]interface{})
	if deleteFilter["_id"] != "{{record.petId}}" || deleteFilter["owner_user_id"] != "{{steps.pet.items.0.owner_user_id}}" {
		t.Fatalf("delete filter = %#v, want requested pet and original owner", deleteFilter)
	}
	deletedGuard := steps["guard_deleted"].Conditions
	if len(deletedGuard) != 1 || deletedGuard[0].Field != "steps.delete_pet.deletedCount" || deletedGuard[0].Value != float64(0) {
		t.Fatalf("deleted guard = %#v, want zero deletedCount guard", deletedGuard)
	}
}
