package models

import (
	"encoding/json"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func intPointer(value int) *int { return &value }

func validCalculatedOrderForm() FormComponentConfig {
	return FormComponentConfig{
		SchemaName: "davinciOrder",
		Fields: []FormFieldConfig{
			{ActionFormFieldConfig: ActionFormFieldConfig{FormKey: "productId", Type: "select", OptionsSource: "schema", SourceSchemaName: "product"}},
			{ActionFormFieldConfig: ActionFormFieldConfig{FormKey: "quantity", Type: "number"}},
		},
		ObjectLists: []FormObjectListConfig{{
			Key:        "items",
			ItemFields: []string{"productId", "quantity"},
			FieldMappings: []FormFieldMappingConfig{{
				SourceFormKey: "productId", SourceField: "price", TargetField: "unitPrice", Required: true,
			}},
			ItemCalculations: []FormItemCalculationConfig{{
				Operation: "multiply", Inputs: []string{"unitPrice", "quantity"}, TargetField: "lineTotal", Precision: intPointer(2),
			}},
		}},
		Summaries: []FormSummaryConfig{
			{Key: "subtotal", Operation: "sum", ObjectListKey: "items", SourceField: "lineTotal", TargetField: "subtotal", Format: &FormValueFormatConfig{Style: "currency", Currency: "TRY", Precision: intPointer(2)}},
			{Key: "total", Operation: "copy", SourceField: "subtotal", TargetField: "total", Format: &FormValueFormatConfig{Style: "currency", Currency: "TRY", Precision: intPointer(2)}},
		},
	}
}

func TestPageModelFormCalculationJSONAndBSONRoundTrip(t *testing.T) {
	want := validCalculatedOrderForm()
	page := PageModel{Sections: []Section{{Cells: []GridCell{{Components: []ComponentBlock{{Type: ComponentTypeForm, Form: &want}}}}}}}

	jsonBytes, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	var jsonPage PageModel
	if err := json.Unmarshal(jsonBytes, &jsonPage); err != nil {
		t.Fatal(err)
	}
	jsonForm := jsonPage.Sections[0].Cells[0].Components[0].Form
	if jsonForm.ObjectLists[0].FieldMappings[0].TargetField != "unitPrice" || jsonForm.Summaries[1].Operation != "copy" {
		t.Fatalf("JSON calculation config was not preserved: %#v", jsonForm)
	}

	bsonBytes, err := bson.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	var bsonPage PageModel
	if err := bson.Unmarshal(bsonBytes, &bsonPage); err != nil {
		t.Fatal(err)
	}
	bsonForm := bsonPage.Sections[0].Cells[0].Components[0].Form
	if bsonForm.ObjectLists[0].ItemCalculations[0].TargetField != "lineTotal" || *bsonForm.Summaries[0].Format.Precision != 2 {
		t.Fatalf("BSON calculation config was not preserved: %#v", bsonForm)
	}
}

func TestValidateFormCalculationConfig(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*FormComponentConfig)
		wantErr string
	}{
		{name: "valid"},
		{name: "mapping source required", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].FieldMappings[0].SourceField = "" }, wantErr: "sourceFormKey, sourceField, and targetField"},
		{name: "mapping select must use schema", mutate: func(form *FormComponentConfig) { form.Fields[0].OptionsSource = "static" }, wantErr: "schema-backed select"},
		{name: "duplicate mapping target", mutate: func(form *FormComponentConfig) {
			form.ObjectLists[0].FieldMappings = append(form.ObjectLists[0].FieldMappings, form.ObjectLists[0].FieldMappings[0])
		}, wantErr: "duplicate item target"},
		{name: "unknown calculation input", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].ItemCalculations[0].Inputs[1] = "missing" }, wantErr: "unknown input"},
		{name: "wrong multiply arity", mutate: func(form *FormComponentConfig) {
			form.ObjectLists[0].ItemCalculations[0].Inputs = []string{"unitPrice"}
		}, wantErr: "exactly two inputs"},
		{name: "unsupported item operation", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].ItemCalculations[0].Operation = "divide" }, wantErr: "unsupported item calculation operation"},
		{name: "overwrite input", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].ItemCalculations[0].TargetField = "quantity" }, wantErr: "cannot overwrite an input"},
		{name: "unknown summary list", mutate: func(form *FormComponentConfig) { form.Summaries[0].ObjectListKey = "missing" }, wantErr: "unknown object list"},
		{name: "unknown summary field", mutate: func(form *FormComponentConfig) { form.Summaries[0].SourceField = "missing" }, wantErr: "unknown item field"},
		{name: "forward summary reference", mutate: func(form *FormComponentConfig) {
			form.Summaries[0] = FormSummaryConfig{Key: "total", Operation: "copy", SourceField: "future", TargetField: "total"}
		}, wantErr: "unknown earlier summary"},
		{name: "duplicate summary target", mutate: func(form *FormComponentConfig) { form.Summaries[1].TargetField = "subtotal" }, wantErr: "duplicate summary target"},
		{name: "invalid currency", mutate: func(form *FormComponentConfig) { form.Summaries[0].Format.Currency = "try" }, wantErr: "three uppercase"},
		{name: "invalid precision", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].ItemCalculations[0].Precision = intPointer(7) }, wantErr: "precision must be between 0 and 6"},
		{name: "parent field collision", mutate: func(form *FormComponentConfig) {
			form.Fields = append(form.Fields, FormFieldConfig{ActionFormFieldConfig: ActionFormFieldConfig{FormKey: "subtotal", Type: "number"}})
		}, wantErr: "collides with form field"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := validCalculatedOrderForm()
			if tt.mutate != nil {
				tt.mutate(&form)
			}
			err := ValidateFormComponentConfig(&form)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateFormComponentConfig() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateFormComponentConfig() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
