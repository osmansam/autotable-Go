package models

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestContainerModelSortFieldsByOrder(t *testing.T) {
	container := &ContainerModel{Fields: []Field{
		{Name: "third", Order: 30},
		{Name: "first", Order: 10},
		{Name: "second", Order: 20},
	}}

	container.SortFieldsByOrder()

	for i, want := range []string{"first", "second", "third"} {
		if got := container.Fields[i].Name; got != want {
			t.Fatalf("Fields[%d].Name = %q, want %q", i, got, want)
		}
	}
}

func TestInfoBlocksComponentPreservesBindingAndBlockConfig(t *testing.T) {
	page := PageModel{
		Name: "Inventory",
		Sections: []Section{{
			Type: SectionTypeComponent,
			Component: &ComponentBlock{
				ID:    "stock-summary",
				Type:  ComponentTypeInfoBlocks,
				Title: "Stock summary",
				DataBinding: &DataBinding{
					Kind:         BindingKindWorkflow,
					SchemaName:   "products",
					WorkflowName: "stockSummary",
				},
				Props: map[string]interface{}{
					"infoBlocks": InfoBlocksConfig{
						Source: "workflow",
						Items: []InfoBlockItemConfig{{
							Title:  "Critical",
							Value:  "{{quantity}}",
							Footer: "urun",
							Color:  "#ef4444",
							TitleColorRules: []InfoBlockColorRule{{
								Condition: "{{quantity}} > 4",
								Color:     "#16a34a",
							}},
							FooterColorRules: []InfoBlockColorRule{{
								Condition: "default",
								Color:     "#dc2626",
							}},
						}},
					},
				},
			},
		}},
	}

	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("json.Marshal(PageModel) error = %v", err)
	}

	var got PageModel
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("json.Unmarshal(PageModel) error = %v", err)
	}

	component := got.Sections[0].Component
	if component.Type != ComponentTypeInfoBlocks {
		t.Fatalf("component.Type = %q, want %q", component.Type, ComponentTypeInfoBlocks)
	}
	if component.DataBinding == nil || component.DataBinding.WorkflowName != "stockSummary" {
		t.Fatalf("DataBinding = %#v, want workflowName stockSummary", component.DataBinding)
	}
	config, ok := component.Props["infoBlocks"].(map[string]interface{})
	if !ok {
		t.Fatalf("infoBlocks config type = %T, want map[string]interface{}", component.Props["infoBlocks"])
	}
	if config["source"] != "workflow" {
		t.Fatalf("infoBlocks.source = %v, want workflow", config["source"])
	}
	items, ok := config["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("infoBlocks.items = %#v, want one item", config["items"])
	}
	item, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatalf("infoBlocks.items[0] = %T, want map[string]interface{}", items[0])
	}
	if _, ok := item["titleColorRules"].([]interface{}); !ok {
		t.Fatalf("titleColorRules = %#v, want serialized color rules", item["titleColorRules"])
	}
	if _, ok := item["footerColorRules"].([]interface{}); !ok {
		t.Fatalf("footerColorRules = %#v, want serialized color rules", item["footerColorRules"])
	}
}

func TestValidateFrontendLinkConfig(t *testing.T) {
	tests := []struct {
		name     string
		frontend *Frontend
		wantErr  string
	}{
		{name: "nil config"},
		{name: "empty link type", frontend: &Frontend{}},
		{name: "valid type", frontend: &Frontend{LinkType: "email", LinkTemplate: "mailto:{{value}}"}},
		{name: "valid type without template is allowed", frontend: &Frontend{LinkType: "file"}},
		{name: "invalid type", frontend: &Frontend{LinkType: "javascript"}, wantErr: "invalid linkType 'javascript'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFrontendLinkConfig(tt.frontend)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("ValidateFrontendLinkConfig() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("ValidateFrontendLinkConfig() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateContainerFrontendConfig(t *testing.T) {
	tests := []struct {
		name      string
		container *ContainerModel
		wantErr   string
	}{
		{name: "nil container"},
		{
			name: "nested valid fields",
			container: &ContainerModel{SchemaName: "orders", Fields: []Field{{
				Name:     "customer",
				Children: []Field{{Name: "email", Frontend: &Frontend{LinkType: "email"}}},
			}}},
		},
		{
			name: "nested invalid field includes context",
			container: &ContainerModel{SchemaName: "orders", Fields: []Field{{
				Name:     "customer",
				Children: []Field{{Name: "website", Frontend: &Frontend{LinkType: "bad"}}},
			}}},
			wantErr: "container 'orders': field 'website': invalid linkType 'bad'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContainerFrontendConfig(tt.container)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("ValidateContainerFrontendConfig() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("ValidateContainerFrontendConfig() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAndCreateOrUpdateContainer(t *testing.T) {
	invalid := &ContainerModel{Fields: []Field{{Name: "link", Frontend: &Frontend{LinkType: "bad"}}}}
	for name, validate := range map[string]func(*ContainerModel) error{
		"create": ValidateAndCreateContainer,
		"update": ValidateAndUpdateContainer,
	} {
		t.Run(name+" accepts valid container", func(t *testing.T) {
			if err := validate(&ContainerModel{}); err != nil {
				t.Fatalf("validate() error = %v", err)
			}
		})
		t.Run(name+" wraps validation errors", func(t *testing.T) {
			if err := validate(invalid); err == nil || !strings.Contains(err.Error(), "frontend validation failed") {
				t.Fatalf("validate() error = %v, want wrapped frontend validation error", err)
			}
		})
	}
}

func TestValidatePageTableConfig(t *testing.T) {
	valid := &PageModel{
		Name: "Orders",
		Sections: []Section{{
			Type: SectionTypeComponent,
			Component: &ComponentBlock{
				ID:   "orders-table",
				Type: ComponentTypeTable,
				Table: &TableComponentConfig{Columns: []TableColumnConfig{{
					Field: "email",
					Link: &TableLinkConfig{
						Template: "mailto:{{value}}",
						Type:     "email",
					},
				}}},
			},
		}},
	}
	if err := ValidatePageTableConfig(valid); err != nil {
		t.Fatalf("ValidatePageTableConfig() error = %v", err)
	}

	invalid := &PageModel{
		Name: "Orders",
		Sections: []Section{{
			Type: SectionTypeComponent,
			Component: &ComponentBlock{
				ID:   "orders-table",
				Type: ComponentTypeTable,
				Table: &TableComponentConfig{Columns: []TableColumnConfig{{
					Field: "website",
					Link:  &TableLinkConfig{Type: "javascript"},
				}}},
			},
		}},
	}
	if err := ValidatePageTableConfig(invalid); err == nil || !strings.Contains(err.Error(), "component 'orders-table': table column 'website': invalid linkType 'javascript'") {
		t.Fatalf("ValidatePageTableConfig() error = %v, want invalid table link type", err)
	}
}

func TestPageTableComputedLabelColumnRoundTrip(t *testing.T) {
	page := PageModel{
		Name: "Inventory",
		Sections: []Section{{
			Type: SectionTypeComponent,
			Component: &ComponentBlock{
				ID:   "inventory-table",
				Type: ComponentTypeTable,
				Table: &TableComponentConfig{Columns: []TableColumnConfig{{
					Field:         "stockLevel",
					Type:          "computedLabel",
					DisplayName:   "Stock Level",
					FallbackValue: "unknown",
					ComputedLabelRules: []TableComputedLabelRule{
						{Condition: "stock == 1", Value: "critical"},
						{Condition: "stock > 1 && stock < 4", Value: "low"},
						{Condition: "stock > 3", Value: "enough"},
					},
				}}},
			},
		}},
	}

	data, err := bson.Marshal(page)
	if err != nil {
		t.Fatalf("bson.Marshal() error = %v", err)
	}

	var got PageModel
	if err := bson.Unmarshal(data, &got); err != nil {
		t.Fatalf("bson.Unmarshal() error = %v", err)
	}

	column := got.Sections[0].Component.Table.Columns[0]
	if column.Type != "computedLabel" {
		t.Fatalf("Type = %q, want computedLabel", column.Type)
	}
	if column.FallbackValue != "unknown" {
		t.Fatalf("FallbackValue = %q, want unknown", column.FallbackValue)
	}
	if len(column.ComputedLabelRules) != 3 {
		t.Fatalf("ComputedLabelRules length = %d, want 3", len(column.ComputedLabelRules))
	}
	if column.ComputedLabelRules[0].Condition != "stock == 1" || column.ComputedLabelRules[0].Value != "critical" {
		t.Fatalf("ComputedLabelRules[0] = %#v, want critical stock rule", column.ComputedLabelRules[0])
	}
}

func TestPageTableProgressBarColumnRoundTrip(t *testing.T) {
	showValue := true
	page := PageModel{
		Name: "Inventory",
		Sections: []Section{{
			Type: SectionTypeComponent,
			Component: &ComponentBlock{
				ID:   "inventory-table",
				Type: ComponentTypeTable,
				Table: &TableComponentConfig{Columns: []TableColumnConfig{{
					Field:       "stockProgress",
					Type:        "progressBar",
					DisplayName: "Stock",
					ProgressBar: &TableProgressBarConfig{
						SourceField: "stock",
						Max:         8,
						Color:       "#4d9f24",
						TrackColor:  "#e7e5df",
						Height:      12,
						Width:       260,
						ShowValue:   &showValue,
						ColorRules: []TableProgressBarColorRule{
							{Condition: "stock < 2", Color: "#ef4444"},
							{Condition: "stock > 1 && stock < 4", Color: "#f59e0b"},
							{Condition: "stock > 3", Color: "#4d9f24"},
						},
					},
				}}},
			},
		}},
	}

	data, err := bson.Marshal(page)
	if err != nil {
		t.Fatalf("bson.Marshal() error = %v", err)
	}

	var got PageModel
	if err := bson.Unmarshal(data, &got); err != nil {
		t.Fatalf("bson.Unmarshal() error = %v", err)
	}

	progressBar := got.Sections[0].Component.Table.Columns[0].ProgressBar
	if progressBar == nil {
		t.Fatal("ProgressBar = nil, want persisted config")
	}
	if progressBar.SourceField != "stock" || progressBar.Max != 8 {
		t.Fatalf("ProgressBar = %#v, want stock source with max 8", progressBar)
	}
	if progressBar.ShowValue == nil || !*progressBar.ShowValue {
		t.Fatalf("ShowValue = %#v, want true", progressBar.ShowValue)
	}
	if len(progressBar.ColorRules) != 3 {
		t.Fatalf("ColorRules length = %d, want 3", len(progressBar.ColorRules))
	}
}

func TestPageTableActionFormFieldInvalidateKeysRoundTrip(t *testing.T) {
	isNumberButtonsActive := true
	isDisabled := true
	formFields := []ActionFormFieldConfig{{
		FormKey:               "location",
		Type:                  "number",
		InvalidateKeys:        []string{"product", "variant"},
		IsNumberButtonsActive: &isNumberButtonsActive,
		IsDisabled:            &isDisabled,
	}}
	page := PageModel{
		Name: "Orders",
		Sections: []Section{{
			Type: SectionTypeComponent,
			Component: &ComponentBlock{
				ID:   "orders-table",
				Type: ComponentTypeTable,
				Table: &TableComponentConfig{
					Actions: []ActionConfig{{
						Kind:       "update",
						ButtonName: "Save stock",
						FormFields: &formFields,
					}},
				},
			},
		}},
	}

	data, err := bson.Marshal(page)
	if err != nil {
		t.Fatalf("bson.Marshal() error = %v", err)
	}

	var got PageModel
	if err := bson.Unmarshal(data, &got); err != nil {
		t.Fatalf("bson.Unmarshal() error = %v", err)
	}

	gotFields := got.Sections[0].Component.Table.Actions[0].FormFields
	if gotFields == nil {
		t.Fatal("FormFields = nil, want persisted form fields")
	}
	if !reflect.DeepEqual((*gotFields)[0].InvalidateKeys, []string{"product", "variant"}) {
		t.Fatalf("InvalidateKeys = %#v, want %#v", (*gotFields)[0].InvalidateKeys, []string{"product", "variant"})
	}
	if (*gotFields)[0].IsNumberButtonsActive == nil || !*(*gotFields)[0].IsNumberButtonsActive {
		t.Fatalf("IsNumberButtonsActive = %#v, want true", (*gotFields)[0].IsNumberButtonsActive)
	}
	if (*gotFields)[0].IsDisabled == nil || !*(*gotFields)[0].IsDisabled {
		t.Fatalf("IsDisabled = %#v, want true", (*gotFields)[0].IsDisabled)
	}
	if gotButtonName := got.Sections[0].Component.Table.Actions[0].ButtonName; gotButtonName != "Save stock" {
		t.Fatalf("ButtonName = %q, want %q", gotButtonName, "Save stock")
	}
}

func TestPageTableFilterPanelInputsRoundTrip(t *testing.T) {
	isMultiple := true
	inputs := []ActionFormFieldConfig{{
		FormKey:           "status",
		Type:              "select",
		FormKeyType:       "stringArray",
		Label:             "Status",
		Placeholder:       "Status",
		IsMultiple:        &isMultiple,
		OptionsSource:     "static",
		StaticOptionsJson: `[{"value":"active","label":"Active"}]`,
	}}
	page := PageModel{
		Name: "Orders",
		Sections: []Section{{
			Type: SectionTypeComponent,
			Component: &ComponentBlock{
				ID:   "orders-table",
				Type: ComponentTypeTable,
				Table: &TableComponentConfig{
					FilterPanel: &TableFilterPanelConfig{
						Inputs: &inputs,
					},
				},
			},
		}},
	}

	data, err := bson.Marshal(page)
	if err != nil {
		t.Fatalf("bson.Marshal() error = %v", err)
	}

	var got PageModel
	if err := bson.Unmarshal(data, &got); err != nil {
		t.Fatalf("bson.Unmarshal() error = %v", err)
	}

	gotInputs := got.Sections[0].Component.Table.FilterPanel.Inputs
	if gotInputs == nil {
		t.Fatal("FilterPanel.Inputs = nil, want persisted filter inputs")
	}
	if (*gotInputs)[0].FormKey != "status" {
		t.Fatalf("FormKey = %q, want status", (*gotInputs)[0].FormKey)
	}
	if (*gotInputs)[0].IsMultiple == nil || !*(*gotInputs)[0].IsMultiple {
		t.Fatalf("IsMultiple = %#v, want true", (*gotInputs)[0].IsMultiple)
	}
	if (*gotInputs)[0].StaticOptionsJson == "" {
		t.Fatal("StaticOptionsJson = empty, want persisted static options")
	}
}

func TestPageRouteParamSlugAndBindingParamsRoundTrip(t *testing.T) {
	isOnSidebar := false
	page := PageModel{
		Name:        "Count Detail",
		Slug:        "count/:id",
		IsOnSidebar: &isOnSidebar,
		Sections: []Section{{
			Type: SectionTypeComponent,
			Component: &ComponentBlock{
				ID:    "count-summary",
				Type:  ComponentTypeBarChart,
				Title: "Count Summary",
				DataBinding: &DataBinding{
					Kind:         BindingKindPipeline,
					SchemaName:   "account_counts",
					PipelineName: "count_summary",
					Params: map[string]interface{}{
						"countList": "{{route.id}}",
					},
				},
			},
		}},
	}

	data, err := bson.Marshal(page)
	if err != nil {
		t.Fatalf("bson.Marshal() error = %v", err)
	}

	var got PageModel
	if err := bson.Unmarshal(data, &got); err != nil {
		t.Fatalf("bson.Unmarshal() error = %v", err)
	}

	if got.Slug != "count/:id" {
		t.Fatalf("Slug = %q, want %q", got.Slug, "count/:id")
	}
	if got.IsOnSidebar == nil || *got.IsOnSidebar {
		t.Fatalf("IsOnSidebar = %#v, want false", got.IsOnSidebar)
	}

	gotBinding := got.Sections[0].Component.DataBinding
	if gotBinding == nil {
		t.Fatal("DataBinding = nil, want persisted binding")
	}
	if gotBinding.Params["countList"] != "{{route.id}}" {
		t.Fatalf("Params[countList] = %#v, want {{route.id}}", gotBinding.Params["countList"])
	}
}

func TestFrontendLinkExamplesAreValid(t *testing.T) {
	fields := []Field{
		ExampleExternalLink(),
		ExampleInternalLink(),
		ExampleEmailLink(),
		ExamplePhoneLink(),
		ExampleFileLink(),
		ExampleRowFieldLink(),
	}
	for _, field := range fields {
		t.Run(field.Name, func(t *testing.T) {
			if err := ValidateFieldFrontendConfig(&field); err != nil {
				t.Fatalf("ValidateFieldFrontendConfig() error = %v", err)
			}
		})
	}
	if err := ValidateContainerFrontendConfig(ptrContainer(ExampleCompleteContainer())); err != nil {
		t.Fatalf("ExampleCompleteContainer() error = %v", err)
	}
}

func ptrContainer(container ContainerModel) *ContainerModel {
	return &container
}
