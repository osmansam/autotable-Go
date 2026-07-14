package models

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func validRuntimePage() PageModel {
	filterID := "tfl_created_at"
	return PageModel{
		Name: "Runtime bindings",
		Variables: []PageVariableDefinition{{
			ID: "var_region", Key: "region", Type: RuntimeValueTypeString, InitialValue: "all",
		}},
		Sections: []Section{{
			Type: SectionTypeComponent,
			Component: &ComponentBlock{
				ID: "cmp_orders", StateKey: "ordersTable", Type: ComponentTypeTable,
				Table: &TableComponentConfig{FilterPanel: &TableFilterPanelConfig{
					Inputs: &[]ActionFormFieldConfig{{
						ID: filterID, FormKey: "createdAt", Type: "date", FormKeyType: "date",
					}},
				}},
				Outputs: []ComponentOutputDefinition{{
					ID: "out_created_at", Key: "createdAt",
					Type: RuntimeValueTypeString,
					Source: ComponentOutputSource{
						Kind: ComponentOutputSourceTableFilter, FilterID: filterID,
					},
				}, {
					ID: "out_selected_ids", Key: "selectedIds",
					Type:   RuntimeValueTypeStringArray,
					Source: ComponentOutputSource{Kind: ComponentOutputSourceTableSelectedIDs},
				}},
			},
		}, {
			Type: SectionTypeComponent,
			Component: &ComponentBlock{
				ID: "cmp_summary", StateKey: "ordersSummary", Type: ComponentTypeInfoBlocks,
				DataBinding: &DataBinding{
					Kind: BindingKindPipeline,
					Parameters: map[string]ParameterBinding{
						"after": {
							Source:      ParameterBindingSourceComponentOutput,
							ComponentID: "cmp_orders", OutputID: "out_created_at",
						},
						"status": {Source: ParameterBindingSourceStatic, Value: "open"},
					},
				},
			},
		}},
	}
}

func requireRuntimeValidationError(t *testing.T, page PageModel, substring string) {
	t.Helper()
	err := ValidatePageRuntimeConfig(&page)
	if err == nil || !strings.Contains(err.Error(), substring) {
		t.Fatalf("ValidatePageRuntimeConfig() error = %v, want substring %q", err, substring)
	}
}

func TestValidatePageRuntimeConfigAcceptsValidGraph(t *testing.T) {
	page := validRuntimePage()
	if err := ValidatePageRuntimeConfig(&page); err != nil {
		t.Fatalf("ValidatePageRuntimeConfig() error = %v", err)
	}
}

func TestValidatePageRuntimeConfigLeavesLegacyPageWithoutRuntimeConfigUnchanged(t *testing.T) {
	legacyFilter := ActionFormFieldConfig{ID: "legacy-filter", FormKey: "status", Type: "text"}
	page := PageModel{
		Name: "Legacy",
		Sections: []Section{{
			Component: &ComponentBlock{
				StateKey: "legacyAlias", Type: ComponentTypeTable,
				Table: &TableComponentConfig{FilterPanel: &TableFilterPanelConfig{
					Inputs: &[]ActionFormFieldConfig{legacyFilter},
				}},
			},
		}, {
			Component: &ComponentBlock{
				StateKey: "legacyAlias", Type: ComponentTypeTable,
				DataBinding: &DataBinding{Kind: BindingKindSchema},
				Table: &TableComponentConfig{FilterPanel: &TableFilterPanelConfig{
					Inputs: &[]ActionFormFieldConfig{legacyFilter},
				}},
			},
		}},
	}
	if err := ValidatePageRuntimeConfig(&page); err != nil {
		t.Fatalf("ValidatePageRuntimeConfig() error = %v", err)
	}
}

func TestValidatePageRuntimeConfigRejectsDuplicateOutputID(t *testing.T) {
	page := validRuntimePage()
	page.Sections[0].Component.Outputs = append(
		page.Sections[0].Component.Outputs,
		page.Sections[0].Component.Outputs[0],
	)
	requireRuntimeValidationError(t, page, "duplicate output id")
}

func TestValidatePageRuntimeConfigRejectsDuplicateComponentID(t *testing.T) {
	page := validRuntimePage()
	page.Sections[1].Component.ID = page.Sections[0].Component.ID
	requireRuntimeValidationError(t, page, "duplicate component id")
}

func TestValidatePageRuntimeConfigRejectsDuplicateComponentStateKey(t *testing.T) {
	page := validRuntimePage()
	page.Sections[1].Component.StateKey = page.Sections[0].Component.StateKey
	requireRuntimeValidationError(t, page, "duplicate component state key")
}

func TestValidatePageRuntimeConfigRejectsDuplicateFilterID(t *testing.T) {
	page := validRuntimePage()
	filter := (*page.Sections[0].Component.Table.FilterPanel.Inputs)[0]
	page.Sections[1].Component.Type = ComponentTypeTable
	page.Sections[1].Component.Table = &TableComponentConfig{
		FilterPanel: &TableFilterPanelConfig{Inputs: &[]ActionFormFieldConfig{filter}},
	}
	requireRuntimeValidationError(t, page, "duplicate filter id")
}

func TestValidatePageRuntimeConfigAcceptsUnreferencedLegacyFilterWithoutID(t *testing.T) {
	page := validRuntimePage()
	*page.Sections[0].Component.Table.FilterPanel.Inputs = append(
		*page.Sections[0].Component.Table.FilterPanel.Inputs,
		ActionFormFieldConfig{FormKey: "status", Type: "text"},
	)
	if err := ValidatePageRuntimeConfig(&page); err != nil {
		t.Fatalf("ValidatePageRuntimeConfig() error = %v", err)
	}
}

func TestValidatePageRuntimeConfigRejectsDuplicateOutputKey(t *testing.T) {
	page := validRuntimePage()
	page.Sections[0].Component.Outputs[1].Key = page.Sections[0].Component.Outputs[0].Key
	requireRuntimeValidationError(t, page, "duplicate output key")
}

func TestValidatePageRuntimeConfigRejectsMissingReferencedComponent(t *testing.T) {
	page := validRuntimePage()
	page.Sections[1].Component.DataBinding.Parameters["after"] = ParameterBinding{
		Source: ParameterBindingSourceComponentOutput, ComponentID: "cmp_missing", OutputID: "out_created_at",
	}
	requireRuntimeValidationError(t, page, "referenced component")
}

func TestValidatePageRuntimeConfigRejectsMissingReferencedOutput(t *testing.T) {
	page := validRuntimePage()
	page.Sections[1].Component.DataBinding.Parameters["after"] = ParameterBinding{
		Source: ParameterBindingSourceComponentOutput, ComponentID: "cmp_orders", OutputID: "out_missing",
	}
	requireRuntimeValidationError(t, page, "referenced output")
}

func TestValidatePageRuntimeConfigRejectsMissingReferencedFilter(t *testing.T) {
	page := validRuntimePage()
	page.Sections[0].Component.Outputs[0].Source.FilterID = "tfl_missing"
	requireRuntimeValidationError(t, page, "referenced filter")
}

func TestValidatePageRuntimeConfigRejectsInvalidDateRangeField(t *testing.T) {
	page := validRuntimePage()
	page.Sections[0].Component.Outputs[0].Type = RuntimeValueTypeDateRange
	binding := page.Sections[1].Component.DataBinding.Parameters["after"]
	binding.Field = "value"
	page.Sections[1].Component.DataBinding.Parameters["after"] = binding
	requireRuntimeValidationError(t, page, "invalid date-range field")
}

func TestValidateParameterBindingAcceptsApprovedDateRangeFields(t *testing.T) {
	component := &ComponentBlock{ID: "cmp_date"}
	graph := pageRuntimeGraph{
		components: map[string]*ComponentBlock{component.ID: component},
		outputs: map[string]map[string]ComponentOutputDefinition{
			component.ID: {
				"out_date": {ID: "out_date", Type: RuntimeValueTypeDateRange},
			},
		},
	}
	for _, field := range []string{"", "start", "end", "preset", "timezone"} {
		t.Run(field, func(t *testing.T) {
			err := validateParameterBinding(ParameterBinding{
				Source:      ParameterBindingSourceComponentOutput,
				ComponentID: component.ID, OutputID: "out_date", Field: field,
			}, graph)
			if err != nil {
				t.Fatalf("validateParameterBinding() error = %v", err)
			}
		})
	}
}

func TestValidatePageRuntimeConfigAcceptsPageFilterBindings(t *testing.T) {
	page := validRuntimePage()
	page.Filters = []PageFilterDefinition{{
		ID:           "pfl_region",
		Key:          "region",
		Label:        "Region",
		Type:         RuntimeValueTypeDate,
		DefaultValue: "2026-07-08T00:00:00.000Z",
		Placement:    PageFilterPlacement{Kind: PageFilterPlacementNavbar},
	}}
	page.Sections[1].Component.DataBinding.Parameters["region"] = ParameterBinding{
		Source:   ParameterBindingSourcePageFilter,
		FilterID: "pfl_region",
	}
	if err := ValidatePageRuntimeConfig(&page); err != nil {
		t.Fatalf("ValidatePageRuntimeConfig() error = %v", err)
	}
}

func TestValidatePageRuntimeConfigRejectsMissingPageFilterBinding(t *testing.T) {
	page := validRuntimePage()
	page.Sections[1].Component.DataBinding.Parameters["region"] = ParameterBinding{
		Source:   ParameterBindingSourcePageFilter,
		FilterID: "pfl_missing",
	}
	requireRuntimeValidationError(t, page, "unknown page filter")
}

func TestValidatePageRuntimeConfigRejectsCellFilterWithoutMatchingCell(t *testing.T) {
	page := validRuntimePage()
	page.Filters = []PageFilterDefinition{{
		ID:        "pfl_region",
		Key:       "region",
		Label:     "Region",
		Type:      RuntimeValueTypeString,
		Placement: PageFilterPlacement{Kind: PageFilterPlacementCell, CellID: "cell_missing"},
	}}
	requireRuntimeValidationError(t, page, "unknown cell")
}

func TestNormalizePageRuntimeConfigRepairsTenantPanelFilterPayload(t *testing.T) {
	page := validRuntimePage()
	page.Filters = []PageFilterDefinition{{
		ID:            "pfl_missing_cell",
		Key:           "createdAt",
		Label:         "Created At",
		Type:          RuntimeValueTypeDate,
		DefaultPreset: PageFilterDefaultPresetToday,
		Placement:     PageFilterPlacement{Kind: PageFilterPlacementCell, CellID: "cell_missing"},
	}, {
		ID:            "pfl_month",
		Key:           "month",
		Label:         "Month",
		Type:          RuntimeValueTypeMonthYear,
		DefaultPreset: PageFilterDefaultPresetToday,
		Placement:     PageFilterPlacement{Kind: PageFilterPlacementNavbar},
	}}

	NormalizePageRuntimeConfig(&page)

	if page.Filters[0].Placement.Kind != PageFilterPlacementNavbar || page.Filters[0].Placement.CellID != "" {
		t.Fatalf("stale cell placement was not normalized: %#v", page.Filters[0].Placement)
	}
	if page.Filters[1].DefaultPreset != PageFilterDefaultPresetCurrentMonthYear {
		t.Fatalf("monthYear preset = %q, want %q", page.Filters[1].DefaultPreset, PageFilterDefaultPresetCurrentMonthYear)
	}
	if err := ValidatePageRuntimeConfig(&page); err != nil {
		t.Fatalf("ValidatePageRuntimeConfig() error = %v", err)
	}
}

func TestValidatePageRuntimeConfigRejectsScalarFieldAccessor(t *testing.T) {
	page := validRuntimePage()
	binding := page.Sections[1].Component.DataBinding.Parameters["after"]
	binding.Field = "start"
	page.Sections[1].Component.DataBinding.Parameters["after"] = binding
	requireRuntimeValidationError(t, page, "scalar output")
}

func TestValidateParameterBindingAcceptsJSONSafeStaticValues(t *testing.T) {
	var jsonValue interface{}
	if err := json.Unmarshal([]byte(`{"nested":[null,true,2.5,"value"]}`), &jsonValue); err != nil {
		t.Fatal(err)
	}
	decimal, err := primitive.ParseDecimal128("123.45")
	if err != nil {
		t.Fatal(err)
	}
	bsonBytes, err := bson.Marshal(bson.D{{
		Key: "nested",
		Value: bson.M{
			"values": primitive.A{nil, int32(7), int64(8), 9.5, decimal},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var bsonValue interface{}
	if err := bson.Unmarshal(bsonBytes, &bsonValue); err != nil {
		t.Fatal(err)
	}

	values := []interface{}{
		nil,
		"string",
		true,
		int(1), int8(1), int16(1), int32(1), int64(1),
		uint(1), uint8(1), uint16(1), uint32(1), uint64(1), uintptr(1),
		float32(1.5), float64(1.5), json.Number("1.5"), decimal,
		[]string{"one", "two"},
		[]interface{}{nil, false, float64(3), map[string]interface{}{"key": "value"}},
		map[string]interface{}{"nested": []interface{}{1, "two"}},
		primitive.M{"nested": primitive.A{int32(1), "two"}},
		primitive.D{{Key: "nested", Value: primitive.A{int64(1), "two"}}},
		jsonValue,
		bsonValue,
	}
	for index, value := range values {
		if err := validateParameterBinding(ParameterBinding{
			Source: ParameterBindingSourceStatic,
			Value:  value,
		}, pageRuntimeGraph{}); err != nil {
			t.Fatalf("value %d (%T): validateParameterBinding() error = %v", index, value, err)
		}
	}
}

func TestValidateParameterBindingRejectsUnsafeStaticValues(t *testing.T) {
	decimalNaN, err := primitive.ParseDecimal128("NaN")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		value     interface{}
		substring string
	}{
		{name: "nan", value: math.NaN(), substring: "non-finite number"},
		{name: "positive infinity", value: math.Inf(1), substring: "non-finite number"},
		{name: "negative infinity", value: math.Inf(-1), substring: "non-finite number"},
		{name: "invalid json number", value: json.Number("not-a-number"), substring: "invalid JSON number"},
		{name: "non-finite bson decimal", value: decimalNaN, substring: "non-finite BSON decimal"},
		{name: "non-string object key", value: map[int]interface{}{1: "value"}, substring: "non-string object key"},
		{name: "unsupported type", value: complex(1, 2), substring: "unsupported static value type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateParameterBinding(ParameterBinding{
				Source: ParameterBindingSourceStatic,
				Value:  test.value,
			}, pageRuntimeGraph{})
			if err == nil || !strings.Contains(err.Error(), test.substring) {
				t.Fatalf("validateParameterBinding() error = %v, want substring %q", err, test.substring)
			}
		})
	}
}

func TestValidatePageRuntimeConfigRejectsUnsupportedBindingSource(t *testing.T) {
	page := validRuntimePage()
	page.Sections[1].Component.DataBinding.Parameters["after"] = ParameterBinding{
		Source: ParameterBindingSourcePageVariable, VariableID: "var_region",
	}
	requireRuntimeValidationError(t, page, "unsupported binding source")
}

func TestValidatePageRuntimeConfigRejectsDerivedBeforeValidatingMalformedInput(t *testing.T) {
	page := validRuntimePage()
	page.Sections[1].Component.DataBinding.Parameters["after"] = ParameterBinding{
		Source: ParameterBindingSourceDerived,
		Input: &ParameterBinding{
			Source: ParameterBindingSourceComponentOutput, ComponentID: "cmp_missing", OutputID: "out_missing",
		},
	}
	requireRuntimeValidationError(t, page, `unsupported binding source "derived"`)
}

func TestValidatePageRuntimeConfigValidatesParametersInNameOrder(t *testing.T) {
	page := validRuntimePage()
	page.Sections[1].Component.DataBinding.Parameters = map[string]ParameterBinding{
		"zeta":  {Source: ParameterBindingSourceSystem},
		"alpha": {Source: ParameterBindingSourcePageVariable, VariableID: "missing"},
	}
	for iteration := 0; iteration < 200; iteration++ {
		err := ValidatePageRuntimeConfig(&page)
		if err == nil || !strings.Contains(err.Error(), `parameter "alpha"`) {
			t.Fatalf("iteration %d: ValidatePageRuntimeConfig() error = %v, want alpha parameter first", iteration, err)
		}
	}
}

func TestValidatePageRuntimeConfigRejectsUnsupportedOutputSource(t *testing.T) {
	page := validRuntimePage()
	page.Sections[0].Component.Outputs[0].Source.Kind = "pipelineResult"
	requireRuntimeValidationError(t, page, "unsupported output source")
}

func TestValidatePageRuntimeConfigRejectsMismatchedOutputType(t *testing.T) {
	page := validRuntimePage()
	page.Sections[0].Component.Outputs[0].Type = RuntimeValueTypeNumber
	requireRuntimeValidationError(t, page, "output type")
}

func TestValidatePageRuntimeConfigRejectsDeletionOfReferencedDefinition(t *testing.T) {
	page := validRuntimePage()
	page.Sections[0].Component.Outputs = page.Sections[0].Component.Outputs[1:]
	requireRuntimeValidationError(t, page, "referenced output")
}
