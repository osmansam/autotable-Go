package models

import (
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
