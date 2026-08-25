package models

import (
	"encoding/json"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func intPointer(value int) *int           { return &value }
func floatPointer(value float64) *float64 { return &value }

func validCalculatedOrderForm() FormComponentConfig {
	return FormComponentConfig{
		SchemaName: "davinciOrder",
		Fields: []FormFieldConfig{
			{ActionFormFieldConfig: ActionFormFieldConfig{
				FormKey: "productId", Type: "select", OptionsSource: "schema", SourceSchemaName: "product",
				SourceValueField: "_id", SourceLabelField: "name", SourceDataFields: []string{"price"},
				OptionDisplay: &SelectOptionDisplayConfig{LeftTemplate: "{{name}}", RightTemplate: "{{price}} ₺"},
			}},
			{ActionFormFieldConfig: ActionFormFieldConfig{FormKey: "quantity", Type: "number"}},
		},
		ObjectLists: []FormObjectListConfig{{
			Key:        "items",
			ItemFields: []string{"productId", "quantity"},
			MergeOnAdd: &FormObjectListMergeConfig{MatchField: "productId", QuantityField: "quantity"},
			FieldMappings: []FormFieldMappingConfig{{
				SourceFormKey: "productId", SourceField: "price", TargetField: "unitPrice", Required: true,
			}},
			ItemCalculations: []FormItemCalculationConfig{{
				Operation: "quantityDiscount", Inputs: []string{"unitPrice", "quantity"},
				OriginalTargetField: "originalLineTotal", TargetField: "lineTotal",
				MinimumQuantity: floatPointer(6), DiscountPercentage: floatPointer(30), Precision: intPointer(2),
			}},
			Display: &FormObjectListDisplayConfig{PriceComparison: &FormPriceComparisonConfig{
				OriginalField: "originalLineTotal", DiscountedField: "lineTotal", Currency: "TRY", Precision: intPointer(2),
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
	jsonCalculation := jsonForm.ObjectLists[0].ItemCalculations[0]
	jsonComparison := jsonForm.ObjectLists[0].Display.PriceComparison
	if jsonCalculation.OriginalTargetField != "originalLineTotal" || *jsonCalculation.MinimumQuantity != 6 || *jsonCalculation.DiscountPercentage != 30 || jsonComparison.OriginalField != "originalLineTotal" || jsonComparison.DiscountedField != "lineTotal" || jsonForm.ObjectLists[0].MergeOnAdd.MatchField != "productId" {
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
	bsonCalculation := bsonForm.ObjectLists[0].ItemCalculations[0]
	bsonComparison := bsonForm.ObjectLists[0].Display.PriceComparison
	if bsonCalculation.OriginalTargetField != "originalLineTotal" || *bsonCalculation.MinimumQuantity != 6 || *bsonCalculation.DiscountPercentage != 30 || bsonComparison.Currency != "TRY" || *bsonComparison.Precision != 2 || bsonForm.ObjectLists[0].MergeOnAdd.QuantityField != "quantity" {
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
		{name: "mapping source must be available", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].FieldMappings[0].SourceField = "taxRate" }, wantErr: "available source field"},
		{name: "blank dependency", mutate: func(form *FormComponentConfig) {
			form.Fields[0].SourceDataFields = append(form.Fields[0].SourceDataFields, " ")
		}, wantErr: "sourceDataFields"},
		{name: "malformed display template", mutate: func(form *FormComponentConfig) { form.Fields[0].OptionDisplay.RightTemplate = "{{price" }, wantErr: "template"},
		{name: "duplicate mapping target", mutate: func(form *FormComponentConfig) {
			form.ObjectLists[0].FieldMappings = append(form.ObjectLists[0].FieldMappings, form.ObjectLists[0].FieldMappings[0])
		}, wantErr: "duplicate item target"},
		{name: "merge match field required", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].MergeOnAdd.MatchField = "" }, wantErr: "mergeOnAdd requires matchField and quantityField"},
		{name: "merge match field unknown", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].MergeOnAdd.MatchField = "missing" }, wantErr: "mergeOnAdd matchField"},
		{name: "merge quantity field unknown", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].MergeOnAdd.QuantityField = "missing" }, wantErr: "mergeOnAdd quantityField"},
		{name: "unknown calculation input", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].ItemCalculations[0].Inputs[1] = "missing" }, wantErr: "unknown input"},
		{name: "wrong quantity discount arity", mutate: func(form *FormComponentConfig) {
			form.ObjectLists[0].ItemCalculations[0].Inputs = []string{"unitPrice"}
		}, wantErr: "exactly two inputs"},
		{name: "unsupported item operation", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].ItemCalculations[0].Operation = "divide" }, wantErr: "unsupported item calculation operation"},
		{name: "overwrite input", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].ItemCalculations[0].TargetField = "quantity" }, wantErr: "cannot overwrite an input"},
		{name: "missing original target", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].ItemCalculations[0].OriginalTargetField = "" }, wantErr: "originalTargetField"},
		{name: "matching outputs", mutate: func(form *FormComponentConfig) {
			form.ObjectLists[0].ItemCalculations[0].OriginalTargetField = "lineTotal"
		}, wantErr: "distinct output"},
		{name: "original target overwrites input", mutate: func(form *FormComponentConfig) {
			form.ObjectLists[0].ItemCalculations[0].OriginalTargetField = "unitPrice"
		}, wantErr: "cannot overwrite an input"},
		{name: "minimum quantity must be positive", mutate: func(form *FormComponentConfig) {
			form.ObjectLists[0].ItemCalculations[0].MinimumQuantity = floatPointer(0)
		}, wantErr: "minimumQuantity must be greater than 0"},
		{name: "discount percentage must be positive", mutate: func(form *FormComponentConfig) {
			form.ObjectLists[0].ItemCalculations[0].DiscountPercentage = floatPointer(0)
		}, wantErr: "discountPercentage must be greater than 0"},
		{name: "discount percentage maximum", mutate: func(form *FormComponentConfig) {
			form.ObjectLists[0].ItemCalculations[0].DiscountPercentage = floatPointer(101)
		}, wantErr: "discountPercentage must not exceed 100"},
		{name: "unknown comparison original", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].Display.PriceComparison.OriginalField = "missing" }, wantErr: "price comparison originalField"},
		{name: "unknown comparison discounted", mutate: func(form *FormComponentConfig) {
			form.ObjectLists[0].Display.PriceComparison.DiscountedField = "missing"
		}, wantErr: "price comparison discountedField"},
		{name: "partial price comparison", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].Display.PriceComparison.DiscountedField = "" }, wantErr: "requires originalField and discountedField"},
		{name: "invalid comparison currency", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].Display.PriceComparison.Currency = "try" }, wantErr: "three uppercase"},
		{name: "invalid comparison precision", mutate: func(form *FormComponentConfig) { form.ObjectLists[0].Display.PriceComparison.Precision = intPointer(7) }, wantErr: "precision must be between 0 and 6"},
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

func TestValidateFormCalculationConfigAllowsQualifiedAdditionalOptionInput(t *testing.T) {
	form := validCalculatedOrderForm()
	form.ObjectLists[0].FieldMappings = nil
	form.ObjectLists[0].ItemCalculations[0].Inputs = []string{"productId.price", "quantity"}

	if err := ValidateFormComponentConfig(&form); err != nil {
		t.Fatalf("ValidateFormComponentConfig() error = %v, want nil", err)
	}
}
