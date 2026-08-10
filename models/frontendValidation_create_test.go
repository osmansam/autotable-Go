package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateTableComponentConfigDataMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		wantErr bool
	}{
		{name: "omitted"},
		{name: "paginated", mode: "paginated"},
		{name: "all", mode: "all"},
		{name: "unknown", mode: "future", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTableComponentConfig(&TableComponentConfig{DataMode: tt.mode})
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "invalid table dataMode") {
					t.Fatalf("ValidateTableComponentConfig() error = %v, want invalid table dataMode", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateTableComponentConfig() error = %v, want nil", err)
			}
		})
	}
}

func TestTableComponentConfigDataModeJSON(t *testing.T) {
	encoded, err := json.Marshal(TableComponentConfig{DataMode: "all"})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(encoded), `"dataMode":"all"`) {
		t.Fatalf("json.Marshal() = %s, want dataMode", encoded)
	}

	var decoded TableComponentConfig
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if decoded.DataMode != "all" {
		t.Fatalf("decoded.DataMode = %q, want all", decoded.DataMode)
	}
}

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
