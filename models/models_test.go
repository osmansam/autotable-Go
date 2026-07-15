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

func TestAuthContainerGoogleLoginFlagRoundTrip(t *testing.T) {
	container := ContainerModel{
		SchemaName:          "auth",
		IsAuthContainer:     true,
		IsGoogleLoginActive: true,
		IsRegisterActive:    true,
		Fields:              []Field{{Name: "email", Type: "string"}},
		PopulatedRoutes:     []string{},
		DynamicFunctions:    []DynamicFunction{},
		DynamicApis:         []DynamicApiModel{},
		Workflows:           []DynamicWorkflow{},
		Pipelines:           []PipelineStage{},
		Indexes:             []Index{},
	}

	jsonData, err := json.Marshal(container)
	if err != nil {
		t.Fatalf("json.Marshal(ContainerModel) error = %v", err)
	}
	var jsonGot ContainerModel
	if err := json.Unmarshal(jsonData, &jsonGot); err != nil {
		t.Fatalf("json.Unmarshal(ContainerModel) error = %v", err)
	}
	if !jsonGot.IsGoogleLoginActive {
		t.Fatal("JSON IsGoogleLoginActive = false, want true")
	}

	bsonData, err := bson.Marshal(container)
	if err != nil {
		t.Fatalf("bson.Marshal(ContainerModel) error = %v", err)
	}
	var bsonGot ContainerModel
	if err := bson.Unmarshal(bsonData, &bsonGot); err != nil {
		t.Fatalf("bson.Unmarshal(ContainerModel) error = %v", err)
	}
	if !bsonGot.IsGoogleLoginActive {
		t.Fatal("BSON IsGoogleLoginActive = false, want true")
	}
}

func TestAuthContainerBooleanFalseValuesArePersisted(t *testing.T) {
	container := ContainerModel{
		SchemaName:          "auth",
		IsAuthContainer:     true,
		IsRegisterActive:    false,
		IsGoogleLoginActive: false,
	}

	data, err := bson.Marshal(container)
	if err != nil {
		t.Fatalf("bson.Marshal(ContainerModel) error = %v", err)
	}

	var raw bson.M
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("bson.Unmarshal(ContainerModel) error = %v", err)
	}

	if got, exists := raw["isRegisterActive"]; !exists || got != false {
		t.Fatalf("isRegisterActive BSON = %#v, exists %v; want false and present", got, exists)
	}
	if got, exists := raw["isGoogleLoginActive"]; !exists || got != false {
		t.Fatalf("isGoogleLoginActive BSON = %#v, exists %v; want false and present", got, exists)
	}
}

func TestValidateAuthContainerGoogleLoginConfig(t *testing.T) {
	tests := []struct {
		name      string
		container ContainerModel
		wantErr   bool
	}{
		{
			name: "allows auth container without email when google login is disabled",
			container: ContainerModel{
				SchemaName:          "auth",
				IsAuthContainer:     true,
				IsGoogleLoginActive: false,
				Fields: []Field{{
					Name:              "username",
					Type:              "string",
					IsLoginCredential: true,
				}},
			},
		},
		{
			name: "rejects google login without email field",
			container: ContainerModel{
				SchemaName:          "auth",
				IsAuthContainer:     true,
				IsGoogleLoginActive: true,
				Fields: []Field{{
					Name:              "username",
					Type:              "string",
					IsLoginCredential: true,
				}},
			},
			wantErr: true,
		},
		{
			name: "allows google login with email field",
			container: ContainerModel{
				SchemaName:          "auth",
				IsAuthContainer:     true,
				IsGoogleLoginActive: true,
				Fields: []Field{{
					Name: "email",
					Type: "string",
				}},
			},
		},
		{
			name: "ignores non auth containers",
			container: ContainerModel{
				SchemaName:          "customer",
				IsAuthContainer:     false,
				IsGoogleLoginActive: true,
				Fields: []Field{{
					Name: "name",
					Type: "string",
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAuthContainerGoogleLoginConfig(&tt.container)
			if tt.wantErr && err == nil {
				t.Fatal("ValidateAuthContainerGoogleLoginConfig error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateAuthContainerGoogleLoginConfig error = %v, want nil", err)
			}
		})
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

func TestPageRuntimeBindingRoundTrip(t *testing.T) {
	filterID := "tfl_created_at"
	page := PageModel{
		Name: "Sales",
		Variables: []PageVariableDefinition{{
			ID: "var_branch", Key: "selectedBranch",
			Type: RuntimeValueTypeString, InitialValue: "all",
		}},
		Sections: []Section{{
			Type: SectionTypeComponent,
			Component: &ComponentBlock{
				ID: "cmp_products", StateKey: "productTable", Type: ComponentTypeTable,
				Table: &TableComponentConfig{FilterPanel: &TableFilterPanelConfig{
					Inputs: &[]ActionFormFieldConfig{{
						ID: filterID, FormKey: "createdAt", Type: "date",
					}},
				}},
				Outputs: []ComponentOutputDefinition{{
					ID: "out_created_at", Key: "createdAtFilter",
					Type: RuntimeValueTypeString,
					Source: ComponentOutputSource{
						Kind: ComponentOutputSourceTableFilter, FilterID: filterID,
					},
				}},
			},
		}, {
			Type: SectionTypeComponent,
			Component: &ComponentBlock{
				ID: "cmp_summary", StateKey: "salesSummary",
				Type: ComponentTypeInfoBlocks,
				DataBinding: &DataBinding{
					Kind: BindingKindPipeline, SchemaName: "sales",
					PipelineName: "sales_summary",
					Parameters: map[string]ParameterBinding{
						"after": {
							Source:      ParameterBindingSourceComponentOutput,
							ComponentID: "cmp_products",
							OutputID:    "out_created_at",
						},
					},
				},
			},
		}},
	}

	roundTrips := []struct {
		name string
		run  func(PageModel) (PageModel, error)
	}{
		{
			name: "json",
			run: func(page PageModel) (PageModel, error) {
				data, err := json.Marshal(page)
				if err != nil {
					return PageModel{}, err
				}
				var got PageModel
				err = json.Unmarshal(data, &got)
				return got, err
			},
		},
		{
			name: "bson",
			run: func(page PageModel) (PageModel, error) {
				data, err := bson.Marshal(page)
				if err != nil {
					return PageModel{}, err
				}
				var got PageModel
				err = bson.Unmarshal(data, &got)
				return got, err
			},
		},
	}

	for _, roundTrip := range roundTrips {
		t.Run(roundTrip.name, func(t *testing.T) {
			got, err := roundTrip.run(page)
			if err != nil {
				t.Fatalf("round trip error = %v", err)
			}

			if got.Variables[0].ID != "var_branch" || got.Variables[0].InitialValue != "all" {
				t.Fatalf("Variables[0] = %#v, want preserved ID and initial value", got.Variables[0])
			}

			productComponent := got.Sections[0].Component
			if productComponent.ID != "cmp_products" || productComponent.StateKey != "productTable" {
				t.Fatalf("product component = %#v, want preserved IDs", productComponent)
			}
			if (*productComponent.Table.FilterPanel.Inputs)[0].ID != filterID {
				t.Fatalf("filter ID = %q, want %q", (*productComponent.Table.FilterPanel.Inputs)[0].ID, filterID)
			}
			if productComponent.Outputs[0].ID != "out_created_at" ||
				productComponent.Outputs[0].Source.Kind != ComponentOutputSourceTableFilter {
				t.Fatalf("Outputs[0] = %#v, want preserved ID and source kind", productComponent.Outputs[0])
			}

			summaryComponent := got.Sections[1].Component
			if summaryComponent.ID != "cmp_summary" || summaryComponent.StateKey != "salesSummary" {
				t.Fatalf("summary component = %#v, want preserved IDs", summaryComponent)
			}
			after := summaryComponent.DataBinding.Parameters["after"]
			if after.Source != ParameterBindingSourceComponentOutput ||
				after.ComponentID != "cmp_products" ||
				after.OutputID != "out_created_at" {
				t.Fatalf("Parameters[after] = %#v, want preserved component output binding", after)
			}
		})
	}
}

func TestPageFiltersRoundTrip(t *testing.T) {
	page := PageModel{
		Name: "Orders",
		Filters: []PageFilterDefinition{{
			ID:                 "pfl_status",
			Key:                "status",
			Label:              "Status",
			Type:               RuntimeValueTypeNumberArray,
			DefaultValue:       []interface{}{float64(10), float64(23)},
			DefaultPreset:      PageFilterDefaultPresetToday,
			ArraySerialization: PageFilterArraySerializationComma,
			Placement:          PageFilterPlacement{Kind: PageFilterPlacementCell, CellID: "cell-main"},
		}},
		Sections: []Section{{
			Type: SectionTypeGrid,
			Grid: &GridSection{Columns: 1, Cells: []GridCell{{
				ID: "cell-main",
				Components: []ComponentBlock{{
					ID:   "cmp_orders",
					Type: ComponentTypeTable,
					DataBinding: &DataBinding{Kind: BindingKindPipeline, Parameters: map[string]ParameterBinding{
						"status": {Source: ParameterBindingSourcePageFilter, FilterID: "pfl_status"},
					}},
				}},
			}}},
		}},
	}

	data, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("json.Marshal(PageModel) error = %v", err)
	}
	var got PageModel
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal(PageModel) error = %v", err)
	}
	if got.Filters[0].ID != "pfl_status" || got.Filters[0].Placement.CellID != "cell-main" {
		t.Fatalf("Filters[0] = %#v, want persisted page filter", got.Filters[0])
	}
	if got.Filters[0].ArraySerialization != PageFilterArraySerializationComma {
		t.Fatalf("Filters[0].ArraySerialization = %q, want comma", got.Filters[0].ArraySerialization)
	}
	if got.Filters[0].DefaultPreset != PageFilterDefaultPresetToday {
		t.Fatalf("Filters[0].DefaultPreset = %q, want today", got.Filters[0].DefaultPreset)
	}
	if got.Sections[0].Grid.Cells[0].Components[0].DataBinding.Parameters["status"].FilterID != "pfl_status" {
		t.Fatalf("pageFilter binding not preserved: %#v", got.Sections[0].Grid.Cells[0].Components[0].DataBinding.Parameters)
	}
}

func TestDistributionBlocksComponentPreservesBindingAndConfig(t *testing.T) {
	page := PageModel{
		Name: "Categories",
		Sections: []Section{{
			Type: SectionTypeComponent,
			Component: &ComponentBlock{
				ID:    "category-distribution",
				Type:  ComponentTypeDistributionBlocks,
				Title: `{{total > 0 ? "Kategori Dağılımı (adet)" : "Kategori yok"}}`,
				DataBinding: &DataBinding{
					Kind:         BindingKindWorkflow,
					SchemaName:   "products",
					WorkflowName: "categorySummary",
				},
				Props: map[string]interface{}{
					"distributionBlocks": DistributionBlocksConfig{
						Source: "workflow",
						Items: []DistributionBlockItemConfig{{
							Label:   `{{strategy > 10 ? "↑ Strateji" : "Strateji"}}`,
							Value:   "{{strategy}}",
							Percent: "{{strategyPercent}}",
							Color:   "#4f46e5",
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
	if component.Type != ComponentTypeDistributionBlocks {
		t.Fatalf("component.Type = %q, want %q", component.Type, ComponentTypeDistributionBlocks)
	}
	if component.DataBinding == nil || component.DataBinding.WorkflowName != "categorySummary" {
		t.Fatalf("DataBinding = %#v, want workflowName categorySummary", component.DataBinding)
	}
	config, ok := component.Props["distributionBlocks"].(map[string]interface{})
	if !ok {
		t.Fatalf("distributionBlocks config type = %T, want map[string]interface{}", component.Props["distributionBlocks"])
	}
	if config["source"] != "workflow" {
		t.Fatalf("distributionBlocks.source = %v, want workflow", config["source"])
	}
	items, ok := config["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("distributionBlocks.items = %#v, want one item", config["items"])
	}
	item, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatalf("distributionBlocks.items[0] = %T, want map[string]interface{}", items[0])
	}
	if item["percent"] != "{{strategyPercent}}" {
		t.Fatalf("distributionBlocks.items[0].percent = %v, want template", item["percent"])
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

func TestPageTableNestedRowsRoundTrip(t *testing.T) {
	page := PageModel{
		Name: "Orders",
		Sections: []Section{{
			Type: SectionTypeComponent,
			Component: &ComponentBlock{
				ID:   "orders-table",
				Type: ComponentTypeTable,
				Table: &TableComponentConfig{
					NestedRows: &TableNestedRowsConfig{
						Enabled: true,
						Field:   "product",
						Header:  "Products",
						Columns: []TableNestedRowColumnConfig{
							{Field: "productDavinciId", DisplayName: "Davinci ID", Type: "number"},
							{Field: "productId", DisplayName: "Product ID"},
							{Field: "quantity", DisplayName: "Quantity", Type: "number"},
						},
					},
				},
			},
		}},
	}

	if err := ValidatePageTableConfig(&page); err != nil {
		t.Fatalf("ValidatePageTableConfig() error = %v", err)
	}

	data, err := bson.Marshal(page)
	if err != nil {
		t.Fatalf("bson.Marshal() error = %v", err)
	}
	var got PageModel
	if err := bson.Unmarshal(data, &got); err != nil {
		t.Fatalf("bson.Unmarshal() error = %v", err)
	}

	nestedRows := got.Sections[0].Component.Table.NestedRows
	if nestedRows == nil || !nestedRows.Enabled || nestedRows.Field != "product" {
		t.Fatalf("NestedRows = %#v, want enabled product config", nestedRows)
	}
	if len(nestedRows.Columns) != 3 || nestedRows.Columns[0].Field != "productDavinciId" {
		t.Fatalf("NestedRows columns = %#v, want product child columns", nestedRows.Columns)
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

func TestFormComponentConfigRoundTrip(t *testing.T) {
	minimum := 1.0
	page := PageModel{Name: "Sales", Sections: []Section{{
		Type: SectionTypeComponent,
		Component: &ComponentBlock{ID: "sales-form", Type: ComponentTypeForm, Form: &FormComponentConfig{
			Title: "Sales Details", SchemaName: "sales",
			Fields: []FormFieldConfig{{ActionFormFieldConfig: ActionFormFieldConfig{
				FormKey: "saleDate", Type: "date", FormKeyType: "date", Label: "Sale Date",
			}, Area: "main", Order: 1, Width: "full"}},
			ObjectLists: []FormObjectListConfig{{
				Key: "items", Title: "Cart", Area: "right", Source: "embedded",
				ItemFields: []string{"product", "quantity", "note"},
				Display:    &FormObjectListDisplayConfig{PrimaryField: "product", SecondaryTemplate: "{{quantity}} items"},
				Actions:    []FormObjectActionConfig{{Kind: "editObject", Position: "start"}, {Kind: "decrement", Field: "quantity", Min: &minimum, Step: 1, Position: "end"}},
			}},
			Actions: []FormActionConfig{{Kind: "addObject", Area: "bottom", TargetObjectList: "items", SourceFields: []string{"product", "quantity", "note"}}, {Kind: "submit", Area: "bottom", ButtonName: "Save Sale"}},
			Submit:  &FormSubmitConfig{Mode: "workflow", WorkflowSchema: "sales", WorkflowName: "submitSale"},
		}},
	}}}

	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("json.Marshal(PageModel) error = %v", err)
	}
	var got PageModel
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("json.Unmarshal(PageModel) error = %v", err)
	}
	form := got.Sections[0].Component.Form
	if form == nil || form.SchemaName != "sales" {
		t.Fatalf("Component.Form = %#v, want sales form config", form)
	}
	if gotList := form.ObjectLists[0]; gotList.Key != "items" || gotList.Display.SecondaryTemplate != "{{quantity}} items" {
		t.Fatalf("form.ObjectLists[0] = %#v, want configured items list", gotList)
	}
	if form.Actions[0].Area != "bottom" || form.ObjectLists[0].Actions[0].Position != "start" {
		t.Fatalf("action placement was not preserved: %#v %#v", form.Actions[0], form.ObjectLists[0].Actions[0])
	}
	if form.Submit == nil || form.Submit.Mode != "workflow" || form.Submit.WorkflowName != "submitSale" {
		t.Fatalf("form.Submit = %#v, want workflow submit config", form.Submit)
	}

	bsonBytes, err := bson.Marshal(page)
	if err != nil {
		t.Fatalf("bson.Marshal(PageModel) error = %v", err)
	}
	var bsonGot PageModel
	if err := bson.Unmarshal(bsonBytes, &bsonGot); err != nil {
		t.Fatalf("bson.Unmarshal(PageModel) error = %v", err)
	}
	if bsonGot.Sections[0].Component.Form == nil {
		t.Fatal("BSON Component.Form = nil, want form config")
	}
}

func TestValidateFormComponentConfig(t *testing.T) {
	tests := []struct {
		name    string
		form    *FormComponentConfig
		wantErr string
	}{
		{name: "nil form"},
		{name: "valid form", form: &FormComponentConfig{
			SchemaName:  "sales",
			Fields:      []FormFieldConfig{{ActionFormFieldConfig: ActionFormFieldConfig{FormKey: "saleDate", Type: "date"}}},
			ObjectLists: []FormObjectListConfig{{Key: "items", Actions: []FormObjectActionConfig{{Kind: "editObject"}, {Kind: "increment", Field: "quantity"}}}},
			Actions:     []FormActionConfig{{Kind: "addObject", TargetObjectList: "items"}, {Kind: "submit"}},
		}},
		{name: "valid create submit", form: &FormComponentConfig{SchemaName: "sales", Submit: &FormSubmitConfig{Mode: "create"}}},
		{name: "valid create many submit", form: &FormComponentConfig{
			SchemaName: "sales", ObjectLists: []FormObjectListConfig{{Key: "items"}}, Submit: &FormSubmitConfig{Mode: "createMany", BulkObjectListKey: "items"},
		}},
		{name: "valid workflow submit", form: &FormComponentConfig{
			SchemaName: "sales", Submit: &FormSubmitConfig{Mode: "workflow", WorkflowSchema: "sales", WorkflowName: "submitSale"},
		}},
		{name: "valid workflow submit with implicit items body", form: &FormComponentConfig{
			SchemaName: "sales", Submit: &FormSubmitConfig{Mode: "workflow", WorkflowSchema: "sales", WorkflowName: "submitSale", BulkObjectListKey: "items"},
		}},
		{name: "invalid submit mode", form: &FormComponentConfig{
			SchemaName: "sales", Submit: &FormSubmitConfig{Mode: "archive"},
		}, wantErr: "invalid form submit mode 'archive'"},
		{name: "create many missing list", form: &FormComponentConfig{
			SchemaName: "sales", Submit: &FormSubmitConfig{Mode: "createMany"},
		}, wantErr: "createMany submit requires bulkObjectListKey"},
		{name: "create many unknown list", form: &FormComponentConfig{
			SchemaName: "sales", ObjectLists: []FormObjectListConfig{{Key: "items"}}, Submit: &FormSubmitConfig{Mode: "createMany", BulkObjectListKey: "lines"},
		}, wantErr: "bulkObjectListKey 'lines' does not match"},
		{name: "workflow missing schema", form: &FormComponentConfig{
			SchemaName: "sales", Submit: &FormSubmitConfig{Mode: "workflow", WorkflowName: "submitSale"},
		}, wantErr: "workflow submit requires workflowSchema"},
		{name: "workflow missing name", form: &FormComponentConfig{
			SchemaName: "sales", Submit: &FormSubmitConfig{Mode: "workflow", WorkflowSchema: "sales"},
		}, wantErr: "workflow submit requires workflowName"},
		{name: "missing schema", form: &FormComponentConfig{}, wantErr: "form requires schemaName"},
		{name: "missing field key", form: &FormComponentConfig{
			SchemaName: "sales", Fields: []FormFieldConfig{{ActionFormFieldConfig: ActionFormFieldConfig{Type: "text"}}},
		}, wantErr: "form field 0 requires formKey"},
		{name: "missing list key", form: &FormComponentConfig{
			SchemaName: "sales", ObjectLists: []FormObjectListConfig{{Source: "embedded"}},
		}, wantErr: "object list 0 requires key"},
		{name: "invalid item action", form: &FormComponentConfig{
			SchemaName: "sales", ObjectLists: []FormObjectListConfig{{Key: "items", Actions: []FormObjectActionConfig{{Kind: "archive"}}}},
		}, wantErr: "invalid object action kind 'archive'"},
		{name: "increment missing field", form: &FormComponentConfig{
			SchemaName: "sales", ObjectLists: []FormObjectListConfig{{Key: "items", Actions: []FormObjectActionConfig{{Kind: "increment"}}}},
		}, wantErr: "increment action requires field"},
		{name: "add object missing target", form: &FormComponentConfig{
			SchemaName: "sales", Actions: []FormActionConfig{{Kind: "addObject"}},
		}, wantErr: "addObject action requires targetObjectList"},
		{name: "add object unknown target", form: &FormComponentConfig{
			SchemaName: "sales", ObjectLists: []FormObjectListConfig{{Key: "items"}}, Actions: []FormActionConfig{{Kind: "addObject", TargetObjectList: "lines"}},
		}, wantErr: "does not match a configured object list"},
		{name: "nested add action missing target", form: &FormComponentConfig{
			SchemaName: "sales", ObjectLists: []FormObjectListConfig{{Key: "items", AddAction: &FormActionConfig{Kind: "addObject"}}},
		}, wantErr: "addObject action requires targetObjectList"},
		{name: "invalid form action area", form: &FormComponentConfig{
			SchemaName: "sales", Actions: []FormActionConfig{{Kind: "submit", Area: "center"}},
		}, wantErr: "invalid form area 'center'"},
		{name: "invalid item action position", form: &FormComponentConfig{
			SchemaName: "sales", ObjectLists: []FormObjectListConfig{{Key: "items", Actions: []FormObjectActionConfig{{Kind: "editObject", Position: "middle"}}}},
		}, wantErr: "invalid object action position 'middle'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFormComponentConfig(tt.form)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("ValidateFormComponentConfig() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("ValidateFormComponentConfig() error = %v, want containing %q", err, tt.wantErr)
			}
		})
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

func TestPageGroupOnlyFalseIsSerialized(t *testing.T) {
	page := PageModel{
		Name:        "Product Catalog",
		IsGroupOnly: false,
	}

	jsonData, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var jsonDocument map[string]interface{}
	if err := json.Unmarshal(jsonData, &jsonDocument); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if value, ok := jsonDocument["isGroupOnly"]; !ok || value != false {
		t.Fatalf("JSON isGroupOnly = %#v, present = %v; want false and present", value, ok)
	}

	bsonData, err := bson.Marshal(page)
	if err != nil {
		t.Fatalf("bson.Marshal() error = %v", err)
	}

	var bsonDocument bson.M
	if err := bson.Unmarshal(bsonData, &bsonDocument); err != nil {
		t.Fatalf("bson.Unmarshal() error = %v", err)
	}
	if value, ok := bsonDocument["isGroupOnly"]; !ok || value != false {
		t.Fatalf("BSON isGroupOnly = %#v, present = %v; want false and present", value, ok)
	}
}

func TestPageMainPageFlagRoundTrip(t *testing.T) {
	page := PageModel{
		Name:       "Sales Reports",
		IsMainPage: true,
	}

	jsonData, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var jsonGot PageModel
	if err := json.Unmarshal(jsonData, &jsonGot); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !jsonGot.IsMainPage {
		t.Fatal("JSON IsMainPage = false, want true")
	}

	bsonData, err := bson.Marshal(page)
	if err != nil {
		t.Fatalf("bson.Marshal() error = %v", err)
	}

	var bsonGot PageModel
	if err := bson.Unmarshal(bsonData, &bsonGot); err != nil {
		t.Fatalf("bson.Unmarshal() error = %v", err)
	}
	if !bsonGot.IsMainPage {
		t.Fatal("BSON IsMainPage = false, want true")
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
