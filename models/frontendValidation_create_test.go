package models

import "testing"

func TestValidateTableComponentConfigAllowsCreateAction(t *testing.T) {
	table := &TableComponentConfig{
		AddButton: &ActionConfig{
			Kind:      "create",
			Label:     "Add",
			ModalType: "form",
		},
	}

	if err := ValidateTableComponentConfig(table); err != nil {
		t.Fatalf("ValidateTableComponentConfig() error = %v, want nil", err)
	}
}

func TestValidateTableComponentConfigRejectsInvalidBulkAction(t *testing.T) {
	table := &TableComponentConfig{
		BulkActions: &TableBulkActionsConfig{
			Edit: &ActionConfig{
				Kind:      "archive",
				Label:     "Archive Selected",
				ModalType: "form",
			},
		},
	}

	if err := ValidateTableComponentConfig(table); err == nil {
		t.Fatal("ValidateTableComponentConfig() error = nil, want invalid bulk action error")
	}
}

func TestValidateTableComponentConfigRejectsEnabledDragWithoutOrderField(t *testing.T) {
	table := &TableComponentConfig{
		Drag: &TableDragConfig{Enabled: true, OrderField: " "},
	}

	if err := ValidateTableComponentConfig(table); err == nil {
		t.Fatal("ValidateTableComponentConfig() error = nil, want missing drag order field error")
	}
}

func TestValidateTableComponentConfigRejectsInvalidToggleConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		table *TableComponentConfig
	}{
		{
			name: "duplicate toggle id",
			table: &TableComponentConfig{Toggles: []TableToggleConfig{
				{ID: "mode", Label: "First"},
				{ID: "mode", Label: "Second"},
			}},
		},
		{
			name: "set effect without field",
			table: &TableComponentConfig{Toggles: []TableToggleConfig{{
				ID: "mode", Request: &TableToggleRequestConfig{
					On: &ToggleRequestEffect{Type: "set", Value: true},
				},
			}}},
		},
		{
			name: "column references missing toggle",
			table: &TableComponentConfig{Columns: []TableColumnConfig{{
				Field: "active", VisibilityToggle: &ToggleBinding{ToggleID: "missing", When: true},
			}}},
		},
		{
			name: "boolean display references missing toggle",
			table: &TableComponentConfig{Columns: []TableColumnConfig{{
				Field: "active", BooleanDisplayToggle: &ToggleBinding{ToggleID: "missing", When: true},
			}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateTableComponentConfig(tt.table); err == nil {
				t.Fatal("ValidateTableComponentConfig() error = nil, want validation error")
			}
		})
	}
}

func TestValidateTableComponentConfigRejectsInvalidGeneratedRelationColumns(t *testing.T) {
	valid := GeneratedRelationColumnsConfig{
		ID: "locations", ArrayField: "locations", SourceSchemaName: "location",
		SourceIDField: "_id", SourceLabelField: "name", SourceLimit: 100,
	}
	tests := []struct {
		name   string
		groups []GeneratedRelationColumnsConfig
	}{
		{name: "empty id", groups: []GeneratedRelationColumnsConfig{{ArrayField: "locations", SourceSchemaName: "location", SourceLabelField: "name"}}},
		{name: "duplicate id", groups: []GeneratedRelationColumnsConfig{valid, valid}},
		{name: "missing array field", groups: []GeneratedRelationColumnsConfig{{ID: "locations", SourceSchemaName: "location", SourceLabelField: "name"}}},
		{name: "missing source schema", groups: []GeneratedRelationColumnsConfig{{ID: "locations", ArrayField: "locations", SourceLabelField: "name"}}},
		{name: "missing label field", groups: []GeneratedRelationColumnsConfig{{ID: "locations", ArrayField: "locations", SourceSchemaName: "location"}}},
		{name: "limit too high", groups: []GeneratedRelationColumnsConfig{{ID: "locations", ArrayField: "locations", SourceSchemaName: "location", SourceLabelField: "name", SourceLimit: 101}}},
		{name: "missing toggle", groups: []GeneratedRelationColumnsConfig{{ID: "locations", ArrayField: "locations", SourceSchemaName: "location", SourceLabelField: "name", BooleanEditToggle: &ToggleBinding{ToggleID: "missing", When: true}}}},
		{name: "missing visibility toggle", groups: []GeneratedRelationColumnsConfig{{ID: "locations", ArrayField: "locations", SourceSchemaName: "location", SourceLabelField: "name", VisibilityToggle: &ToggleBinding{ToggleID: "missing", When: true}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateTableComponentConfig(&TableComponentConfig{GeneratedRelationColumns: tt.groups}); err == nil {
				t.Fatal("ValidateTableComponentConfig() error = nil, want validation error")
			}
		})
	}
}
