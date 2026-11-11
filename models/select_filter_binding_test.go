package models

import (
	"encoding/json"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestActionFormFieldConfigSelectFilterBindingsRoundTrip(t *testing.T) {
	original := FormFieldConfig{
		ActionFormFieldConfig: ActionFormFieldConfig{
			FormKey:          "countList",
			Type:             "select",
			OptionsSource:    "schema",
			SourceSchemaName: "count_lists",
			SourceRequestFilters: map[string]interface{}{
				"active": true,
			},
			SourceFilterBindings: []SelectFilterBinding{
				{
					ID:       "rule_record",
					Field:    "location",
					Operator: "eq",
					Value: SelectFilterValue{
						Source:   "record",
						FieldKey: "locationSelect",
						Path:     []string{"_id"},
					},
				},
				{
					ID:       "rule_field",
					Field:    "tags",
					Operator: "in",
					Value: SelectFilterValue{
						Source:   "field",
						FieldKey: "tagSelect",
					},
				},
				{
					ID:       "rule_false",
					Field:    "archived",
					Operator: "eq",
					Value: SelectFilterValue{
						Source: "literal",
						Value:  false,
					},
				},
				{
					ID:       "rule_zero",
					Field:    "tier",
					Operator: "in",
					Value: SelectFilterValue{
						Source: "literal",
						Value:  []interface{}{0, "base"},
					},
				},
			},
		},
		Area:  "main",
		Order: 1,
	}

	jsonBytes, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var fromJSON FormFieldConfig
	if err := json.Unmarshal(jsonBytes, &fromJSON); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(fromJSON.SourceFilterBindings) != 4 {
		t.Fatalf("JSON roundtrip bindings len = %d, want 4", len(fromJSON.SourceFilterBindings))
	}
	if !reflect.DeepEqual(fromJSON.SourceFilterBindings[0], original.SourceFilterBindings[0]) {
		t.Fatalf("JSON record binding = %#v, want %#v", fromJSON.SourceFilterBindings[0], original.SourceFilterBindings[0])
	}
	if fromJSON.SourceFilterBindings[2].Value.Value != false {
		t.Fatalf("JSON false literal = %#v, want false", fromJSON.SourceFilterBindings[2].Value.Value)
	}

	bsonBytes, err := bson.Marshal(original)
	if err != nil {
		t.Fatalf("bson.Marshal() error = %v", err)
	}
	var fromBSON FormFieldConfig
	if err := bson.Unmarshal(bsonBytes, &fromBSON); err != nil {
		t.Fatalf("bson.Unmarshal() error = %v", err)
	}
	if len(fromBSON.SourceFilterBindings) != 4 {
		t.Fatalf("BSON roundtrip bindings len = %d, want 4", len(fromBSON.SourceFilterBindings))
	}
	if !reflect.DeepEqual(fromBSON.SourceFilterBindings[0], original.SourceFilterBindings[0]) {
		t.Fatalf("BSON record binding = %#v, want %#v", fromBSON.SourceFilterBindings[0], original.SourceFilterBindings[0])
	}
	if fromBSON.SourceFilterBindings[2].Value.Value != false {
		t.Fatalf("BSON false literal = %#v, want false", fromBSON.SourceFilterBindings[2].Value.Value)
	}
}
