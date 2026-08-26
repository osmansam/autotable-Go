package services

import (
	"strings"
	"testing"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func checklistArrayContainer() *models.ContainerModel {
	return &models.ContainerModel{SchemaName: "checklist", Fields: []models.Field{
		{Name: "name", Type: "string", Tag: "required"},
		{Name: "duties", Type: "array", Children: []models.Field{
			{Name: "duty", Type: "string", Tag: "required", Unique: true},
			{Name: "description", Type: "string"},
			{Name: "locations", Type: "objectIdArray", ObjectSchemaName: "location"},
			{Name: "order", Type: "int", Tag: "required"},
		}},
	}}
}

func checklistDutyRows() []map[string]interface{} {
	return []map[string]interface{}{
		{"duty": "Open", "description": "Opening", "locations": []interface{}{"one"}, "order": 0},
		{"duty": "Clean", "description": "Closing", "locations": []interface{}{"two"}, "order": 1},
	}
}

func TestEmbeddedArrayFieldRequiresDeclaredArray(t *testing.T) {
	container := checklistArrayContainer()
	field, err := embeddedArrayField(container, "duties")
	if err != nil || field.Name != "duties" {
		t.Fatalf("embeddedArrayField(duties) = %#v, %v", field, err)
	}
	for _, name := range []string{"", "name", "missing"} {
		if _, err := embeddedArrayField(container, name); err == nil {
			t.Fatalf("embeddedArrayField(%q) error = nil", name)
		}
	}
}

func TestEmbeddedChildIdentityRequiresScalarChild(t *testing.T) {
	arrayField, _ := embeddedArrayField(checklistArrayContainer(), "duties")
	field, err := embeddedIdentityField(arrayField, "duty")
	if err != nil || field.Name != "duty" {
		t.Fatalf("embeddedIdentityField(duty) = %#v, %v", field, err)
	}
	for _, name := range []string{"", "locations", "missing"} {
		if _, err := embeddedIdentityField(arrayField, name); err == nil {
			t.Fatalf("embeddedIdentityField(%q) error = nil", name)
		}
	}
}

func TestAddArrayRowAppendsValidatedCopy(t *testing.T) {
	rows := checklistDutyRows()
	item := map[string]interface{}{"duty": "Count", "description": "Count cash", "order": 2}
	next, changed, err := addArrayRow(rows, "duty", item)
	if err != nil {
		t.Fatalf("addArrayRow() error = %v", err)
	}
	if len(next) != 3 || changed["duty"] != "Count" || len(rows) != 2 {
		t.Fatalf("addArrayRow() next = %#v, changed = %#v", next, changed)
	}
	if _, _, err := addArrayRow(rows, "duty", map[string]interface{}{"duty": "Open"}); err == nil {
		t.Fatal("addArrayRow(duplicate) error = nil")
	}
}

func TestUpdateArrayRowMergesOneMatchAndAllowsUniqueIdentityChange(t *testing.T) {
	rows := checklistDutyRows()
	next, changed, err := updateArrayRow(rows, "duty", "Clean", map[string]interface{}{
		"duty":      "Close",
		"locations": []interface{}{"one", "three"},
	})
	if err != nil {
		t.Fatalf("updateArrayRow() error = %v", err)
	}
	if changed["duty"] != "Close" || changed["description"] != "Closing" {
		t.Fatalf("changed = %#v, want merged row", changed)
	}
	if next[0]["duty"] != "Open" || next[1]["duty"] != "Close" {
		t.Fatalf("next = %#v", next)
	}
	if _, _, err := updateArrayRow(rows, "duty", "Clean", map[string]interface{}{"duty": "Open"}); err == nil {
		t.Fatal("updateArrayRow(duplicate replacement) error = nil")
	}
}

func TestArrayRowMutationsRejectMissingAndAmbiguousIdentity(t *testing.T) {
	rows := append(checklistDutyRows(), map[string]interface{}{"duty": "Open", "order": 2})
	for _, operation := range []func() error{
		func() error {
			_, _, err := updateArrayRow(rows, "duty", "Missing", map[string]interface{}{"order": 3})
			return err
		},
		func() error {
			_, _, err := updateArrayRow(rows, "duty", "Open", map[string]interface{}{"order": 3})
			return err
		},
		func() error { _, _, err := deleteArrayRow(rows, "duty", "Missing"); return err },
		func() error { _, _, err := deleteArrayRow(rows, "duty", "Open"); return err },
	} {
		if err := operation(); err == nil {
			t.Fatal("array row mutation error = nil")
		}
	}
}

func TestDeleteArrayRowRemovesExactlyOneMatch(t *testing.T) {
	next, deleted, err := deleteArrayRow(checklistDutyRows(), "duty", "Open")
	if err != nil {
		t.Fatalf("deleteArrayRow() error = %v", err)
	}
	if len(next) != 1 || next[0]["duty"] != "Clean" || deleted["duty"] != "Open" {
		t.Fatalf("deleteArrayRow() next = %#v, deleted = %#v", next, deleted)
	}
}

func TestDeleteArrayRowMatchesObjectIDWithHexIdentity(t *testing.T) {
	productID := primitive.NewObjectID()
	rows := []map[string]interface{}{
		{"product": productID},
		{"product": primitive.NewObjectID()},
	}

	next, deleted, err := deleteArrayRow(rows, "product", productID.Hex())
	if err != nil {
		t.Fatalf("deleteArrayRow() error = %v", err)
	}
	if len(next) != 1 || next[0]["product"] == productID || deleted["product"] != productID {
		t.Fatalf("deleteArrayRow() next = %#v, deleted = %#v", next, deleted)
	}
}

func TestReorderArrayRowsRequiresCompleteUniqueIdentitySet(t *testing.T) {
	next, err := reorderArrayRows(checklistDutyRows(), "duty", "order", []interface{}{"Clean", "Open"})
	if err != nil {
		t.Fatalf("reorderArrayRows() error = %v", err)
	}
	if next[0]["duty"] != "Clean" || next[0]["order"] != 0 || next[1]["order"] != 1 {
		t.Fatalf("reorderArrayRows() = %#v", next)
	}
	for _, identities := range [][]interface{}{{"Open"}, {"Open", "Missing"}, {"Open", "Open"}} {
		if _, err := reorderArrayRows(checklistDutyRows(), "duty", "order", identities); err == nil {
			t.Fatalf("reorderArrayRows(%#v) error = nil", identities)
		}
	}
}

func TestNormalizeArrayRowsRejectsNonObjects(t *testing.T) {
	rows, err := normalizeArrayRows(bson.A{bson.M{"duty": "Open"}})
	if err != nil || len(rows) != 1 || rows[0]["duty"] != "Open" {
		t.Fatalf("normalizeArrayRows(bson.A) = %#v, %v", rows, err)
	}
	if _, err := normalizeArrayRows([]interface{}{map[string]interface{}{"duty": "Open"}, "invalid"}); err == nil || !strings.Contains(err.Error(), "object") {
		t.Fatalf("normalizeArrayRows() error = %v", err)
	}
}

func TestPrepareEmbeddedArrayRowValidatesMergedChild(t *testing.T) {
	arrayField, _ := embeddedArrayField(checklistArrayContainer(), "duties")
	if _, err := prepareEmbeddedArrayRow("", "", arrayField, map[string]interface{}{"order": 0}); err == nil {
		t.Fatal("prepareEmbeddedArrayRow(missing required duty) error = nil")
	}
	locationID := primitive.NewObjectID()
	row, err := prepareEmbeddedArrayRow("", "", arrayField, map[string]interface{}{
		"duty":      "Open",
		"order":     0,
		"locations": []interface{}{locationID.Hex()},
		"ignored":   "value",
	})
	if err != nil {
		t.Fatalf("prepareEmbeddedArrayRow() error = %v", err)
	}
	if _, exists := row["ignored"]; exists {
		t.Fatalf("prepareEmbeddedArrayRow() retained unknown field: %#v", row)
	}
	locations, ok := row["locations"].([]primitive.ObjectID)
	if !ok || len(locations) != 1 || locations[0] != locationID {
		t.Fatalf("locations = %#v, want converted ObjectID", row["locations"])
	}
}
