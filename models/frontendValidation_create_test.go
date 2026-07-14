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
