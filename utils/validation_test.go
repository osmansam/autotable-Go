package utils

import (
	"strings"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestValidateContainerModelFieldTypes(t *testing.T) {
	id := primitive.NewObjectID().Hex()
	valid := map[string]interface{}{
		"object":    map[string]interface{}{"name": "Ada"},
		"array":     []interface{}{map[string]interface{}{"name": "Ada"}},
		"mapArray":  []map[string]interface{}{{"name": "Ada"}},
		"objectId":  id,
		"objectIds": []interface{}{id},
		"sequence":  float64(3),
		"email":     "user@example.com",
		"float":     "2.5",
		"uuid":      "550e8400-e29b-41d4-a716-446655440000",
		"url":       "https://example.com",
		"ip":        "127.0.0.1",
		"enum":      "red",
		"enumField": "pending",
		"int":       float64(2),
		"bool":      true,
		"strings":   []interface{}{"a", "b"},
		"numbers":   []interface{}{1, 2.5},
		"date":      float64(1_700_000_000),
		"timeDate":  time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
	}
	fields := []models.Field{
		{Name: "object", Type: "object", Children: []models.Field{{Name: "name", Type: "string"}}},
		{Name: "array", Type: "array", Children: []models.Field{{Name: "name", Type: "string"}}},
		{Name: "mapArray", Type: "array", Children: []models.Field{{Name: "name", Type: "string"}}},
		{Name: "objectId", Type: "objectId"},
		{Name: "objectIds", Type: "objectIdArray", Tag: "minlength=1,maxlength=2"},
		{Name: "sequence", Type: "autoIncrementId"},
		{Name: "email", Type: "string", Tag: "email"},
		{Name: "float", Type: "float", Tag: "positive,min=1,max=3"},
		{Name: "uuid", Type: "uuid"},
		{Name: "url", Type: "url"},
		{Name: "ip", Type: "ip"},
		{Name: "enum", Type: "enum", Tag: `enum="red|blue"`},
		{Name: "enumField", Type: "enum", EnumList: []interface{}{"pending", "delivered"}},
		{Name: "int", Type: "int", Tag: "positive,min=1,max=3"},
		{Name: "bool", Type: "bool"},
		{Name: "strings", Type: "stringArray", Tag: "minlength=1,maxlength=3"},
		{Name: "numbers", Type: "numberArray", Tag: "minlength=1,maxlength=3"},
		{Name: "date", Type: "date"},
		{Name: "timeDate", Type: "date"},
	}
	if err := ValidateContainerModel(valid, models.ContainerModel{Fields: fields}); err != nil {
		t.Fatalf("ValidateContainerModel() error = %v", err)
	}
}

func TestValidateContainerModelErrors(t *testing.T) {
	tests := []struct {
		name  string
		field models.Field
		value interface{}
		want  string
	}{
		{name: "required", field: models.Field{Name: "value", Type: "string", Tag: "required"}, want: "required"},
		{name: "object id", field: models.Field{Name: "value", Type: "objectId"}, value: "bad", want: "valid ObjectId"},
		{name: "email", field: models.Field{Name: "value", Type: "string", Tag: "email"}, value: "bad", want: "valid email"},
		{name: "phone", field: models.Field{Name: "value", Type: "string", Tag: "phone"}, value: "bad", want: "valid phone"},
		{name: "url tag", field: models.Field{Name: "value", Type: "string", Tag: "url"}, value: "bad", want: "valid URL"},
		{name: "credit card", field: models.Field{Name: "value", Type: "string", Tag: "creditcard"}, value: "123", want: "credit card"},
		{name: "lowercase", field: models.Field{Name: "value", Type: "string", Tag: "lowercase"}, value: "BAD", want: "lowercase"},
		{name: "uppercase", field: models.Field{Name: "value", Type: "string", Tag: "uppercase"}, value: "bad", want: "uppercase"},
		{name: "pattern", field: models.Field{Name: "value", Type: "string", Tag: "pattern=^[a-z]+$"}, value: "123", want: "pattern"},
		{name: "positive int", field: models.Field{Name: "value", Type: "int", Tag: "positive"}, value: -1, want: "positive"},
		{name: "boolean", field: models.Field{Name: "value", Type: "bool"}, value: "true", want: "boolean"},
		{name: "date", field: models.Field{Name: "value", Type: "date"}, value: "bad", want: "valid date"},
		{name: "enum", field: models.Field{Name: "value", Type: "enum", EnumList: []interface{}{"red", "blue"}}, value: "green", want: "one of"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContainerModel(map[string]interface{}{"value": tt.value}, models.ContainerModel{Fields: []models.Field{tt.field}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateContainerModel() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidatePartialUpdateAndLoginCredential(t *testing.T) {
	container := models.ContainerModel{Fields: []models.Field{{Name: "email", Type: "string", IsLoginCredential: true}}}
	if err := ValidateContainerModel(map[string]interface{}{}, container); err == nil {
		t.Fatal("ValidateContainerModel() missing login credential error = nil")
	}
	if err := ValidatePartialUpdate(map[string]interface{}{}, container); err != nil {
		t.Fatalf("ValidatePartialUpdate(empty) error = %v", err)
	}
	if err := ValidatePartialUpdate(map[string]interface{}{"email": ""}, container); err == nil {
		t.Fatal("ValidatePartialUpdate(empty credential) error = nil")
	}
}

func TestValidationRuleHelpers(t *testing.T) {
	rules := extractValidationRules(`required,minlength=2,maxlength=5,enum="red|blue",min=1,max=4,minDate=2026-01-01,maxDate=2026-12-31,auto`)
	if rules["required"] != true || rules["auto"] != true || rules["minlength"] != 2 || rules["maxlength"] != 5 {
		t.Fatalf("extractValidationRules() = %#v", rules)
	}
	if !isValidHex("0123abcdefABCDEF") || isValidHex("xyz") {
		t.Fatal("isValidHex() returned incorrect result")
	}
	if err := validateDateConstraints("date", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), map[string]interface{}{"minDate": "2026-01-01"}); err == nil {
		t.Fatal("validateDateConstraints(min) error = nil")
	}
	if err := validateDateConstraints("date", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), map[string]interface{}{"maxDate": "2026-12-31"}); err == nil {
		t.Fatal("validateDateConstraints(max) error = nil")
	}
}

func TestValidateEnumList(t *testing.T) {
	tests := []struct {
		name    string
		field   models.Field
		value   interface{}
		wantErr bool
	}{
		{name: "empty allowed", field: models.Field{Name: "value", Type: "string", EnumList: []interface{}{"red"}}, value: ""},
		{name: "string allowed", field: models.Field{Name: "value", Type: "string", EnumList: []interface{}{"red", "blue"}}, value: "red"},
		{name: "string rejected", field: models.Field{Name: "value", Type: "string", EnumList: []interface{}{"red"}}, value: "blue", wantErr: true},
		{name: "number allowed", field: models.Field{Name: "value", Type: "int", EnumList: []interface{}{1, float64(2)}}, value: 2},
		{name: "number rejected", field: models.Field{Name: "value", Type: "int", EnumList: []interface{}{1}}, value: 2, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateEnumList(map[string]interface{}{"value": tt.value}, tt.field); (err != nil) != tt.wantErr {
				t.Fatalf("validateEnumList() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
