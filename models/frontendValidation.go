package models

import (
	"fmt"
	"strings"
)

// ValidLinkTypes defines the allowed values for Frontend.LinkType
var ValidLinkTypes = []string{
	"external",
	"internal",
	"email",
	"phone",
	"file",
}

var ValidActionKinds = []string{
	"create",
	"edit",
	"delete",
	"update",
	"link",
}

var ValidActionModalTypes = []string{
	"",
	"none",
	"form",
	"confirm",
	"confirmation",
}

// ValidateFrontendLinkConfig validates the link configuration in a Frontend struct
// Returns an error if LinkType is invalid or configuration is inconsistent
func ValidateFrontendLinkConfig(f *Frontend) error {
	if f == nil {
		return nil
	}

	// If LinkType is empty, no validation needed
	if f.LinkType == "" {
		return nil
	}

	// Validate LinkType is one of the allowed values
	isValid := false
	for _, validType := range ValidLinkTypes {
		if f.LinkType == validType {
			isValid = true
			break
		}
	}

	if !isValid {
		return fmt.Errorf(
			"invalid linkType '%s': must be one of [%s]",
			f.LinkType,
			strings.Join(ValidLinkTypes, ", "),
		)
	}

	// Optional: warn if LinkType is set but LinkTemplate is empty
	// This is a soft validation - you may choose to enforce or just warn
	if f.LinkType != "" && f.LinkTemplate == "" {
		// You can choose to return an error here or just log a warning
		// For now, we'll allow it but you can uncomment the line below to enforce
		// return fmt.Errorf("linkTemplate is required when linkType is set")
	}

	return nil
}

func validateLinkType(linkType string) error {
	if linkType == "" {
		return nil
	}

	for _, validType := range ValidLinkTypes {
		if linkType == validType {
			return nil
		}
	}

	return fmt.Errorf(
		"invalid linkType '%s': must be one of [%s]",
		linkType,
		strings.Join(ValidLinkTypes, ", "),
	)
}

func validateActionKind(kind string) error {
	if kind == "" {
		return fmt.Errorf("action kind is required")
	}

	for _, validKind := range ValidActionKinds {
		if kind == validKind {
			return nil
		}
	}

	return fmt.Errorf(
		"invalid action kind '%s': must be one of [%s]",
		kind,
		strings.Join(ValidActionKinds, ", "),
	)
}

func validateActionModalType(modalType string) error {
	for _, validType := range ValidActionModalTypes {
		if modalType == validType {
			return nil
		}
	}

	return fmt.Errorf(
		"invalid action modalType '%s': must be one of [none, form, confirm, confirmation]",
		modalType,
	)
}

func ValidateActionConfig(action ActionConfig) error {
	if err := validateActionKind(action.Kind); err != nil {
		return err
	}
	if err := validateActionModalType(action.ModalType); err != nil {
		return err
	}
	if action.Kind == "link" && action.Path == "" && action.LinkTemplate == "" {
		return fmt.Errorf("link action '%s' requires path", action.Key)
	}
	return nil
}

func ValidateActionConfigs(actions []ActionConfig) error {
	for _, action := range actions {
		if err := ValidateActionConfig(action); err != nil {
			return fmt.Errorf("action '%s': %w", action.Key, err)
		}
	}
	return nil
}

func ValidateFilterPanelConfig(filterPanel *TableFilterPanelConfig) error {
	if filterPanel == nil || filterPanel.Inputs == nil {
		return nil
	}

	for index, input := range *filterPanel.Inputs {
		if input.FormKey == "" {
			return fmt.Errorf("filter input %d requires formKey", index)
		}
		if input.Type == "" {
			return fmt.Errorf("filter input '%s' requires type", input.FormKey)
		}
	}

	return nil
}

func ValidateFormComponentConfig(form *FormComponentConfig) error {
	if form == nil {
		return nil
	}
	if form.SchemaName == "" {
		return fmt.Errorf("form requires schemaName")
	}
	for index, field := range form.Fields {
		if field.FormKey == "" {
			return fmt.Errorf("form field %d requires formKey", index)
		}
		if field.Type == "" {
			return fmt.Errorf("form field '%s' requires type", field.FormKey)
		}
	}

	objectListKeys := map[string]bool{}
	for index, objectList := range form.ObjectLists {
		if objectList.Key == "" {
			return fmt.Errorf("object list %d requires key", index)
		}
		objectListKeys[objectList.Key] = true
		for actionIndex, action := range objectList.Actions {
			if err := validateFormObjectActionConfig(action); err != nil {
				return fmt.Errorf("object list '%s' action %d: %w", objectList.Key, actionIndex, err)
			}
		}
	}
	for _, objectList := range form.ObjectLists {
		if objectList.AddAction != nil {
			if err := validateFormActionConfig(*objectList.AddAction, objectListKeys); err != nil {
				return fmt.Errorf("object list '%s' addAction: %w", objectList.Key, err)
			}
		}
	}

	for index, action := range form.Actions {
		if err := validateFormActionConfig(action, objectListKeys); err != nil {
			return fmt.Errorf("form action %d: %w", index, err)
		}
	}
	if err := validateFormSubmitConfig(form.Submit, objectListKeys); err != nil {
		return err
	}
	return nil
}

func validateFormSubmitConfig(submit *FormSubmitConfig, objectListKeys map[string]bool) error {
	if submit == nil || submit.Mode == "" || submit.Mode == "create" {
		return nil
	}
	switch submit.Mode {
	case "createMany":
		if submit.BulkObjectListKey == "" {
			return fmt.Errorf("createMany submit requires bulkObjectListKey")
		}
		if !objectListKeys[submit.BulkObjectListKey] {
			return fmt.Errorf("bulkObjectListKey '%s' does not match a configured object list", submit.BulkObjectListKey)
		}
		return nil
	case "workflow":
		if submit.WorkflowSchema == "" {
			return fmt.Errorf("workflow submit requires workflowSchema")
		}
		if submit.WorkflowName == "" {
			return fmt.Errorf("workflow submit requires workflowName")
		}
		if submit.BulkObjectListKey != "" && submit.BulkObjectListKey != "items" && !objectListKeys[submit.BulkObjectListKey] {
			return fmt.Errorf("bulkObjectListKey '%s' does not match a configured object list", submit.BulkObjectListKey)
		}
		return nil
	default:
		return fmt.Errorf("invalid form submit mode '%s'", submit.Mode)
	}
}

func validateFormObjectActionConfig(action FormObjectActionConfig) error {
	if action.Position != "" && action.Position != "start" && action.Position != "end" {
		return fmt.Errorf("invalid object action position '%s'", action.Position)
	}
	switch action.Kind {
	case "editObject", "removeObject":
		return nil
	case "increment", "decrement":
		if action.Field == "" {
			return fmt.Errorf("%s action requires field", action.Kind)
		}
		return nil
	default:
		return fmt.Errorf("invalid object action kind '%s'", action.Kind)
	}
}

func validateFormActionConfig(action FormActionConfig, objectListKeys map[string]bool) error {
	if err := validateFormArea(action.Area); err != nil {
		return err
	}
	switch action.Kind {
	case "submit":
		return nil
	case "addObject":
		if action.TargetObjectList == "" {
			return fmt.Errorf("addObject action requires targetObjectList")
		}
		if !objectListKeys[action.TargetObjectList] {
			return fmt.Errorf("addObject targetObjectList '%s' does not match a configured object list", action.TargetObjectList)
		}
		return nil
	default:
		return fmt.Errorf("invalid form action kind '%s'", action.Kind)
	}
}

func validateFormArea(area string) error {
	switch area {
	case "", "top", "main", "bottom", "left", "right":
		return nil
	default:
		return fmt.Errorf("invalid form area '%s'", area)
	}
}

func ValidateTableComponentConfig(table *TableComponentConfig) error {
	if table == nil {
		return nil
	}

	for _, column := range table.Columns {
		if column.Link == nil {
			continue
		}
		if err := validateLinkType(column.Link.Type); err != nil {
			return fmt.Errorf("table column '%s': %w", column.Field, err)
		}
	}
	if table.NestedRows != nil && table.NestedRows.Enabled {
		if strings.TrimSpace(table.NestedRows.Field) == "" {
			return fmt.Errorf("table nestedRows requires field")
		}
		if len(table.NestedRows.Columns) == 0 {
			return fmt.Errorf("table nestedRows requires at least one column")
		}
		for index, column := range table.NestedRows.Columns {
			if strings.TrimSpace(column.Field) == "" {
				return fmt.Errorf("table nestedRows column %d requires field", index)
			}
		}
	}
	if err := ValidateActionConfigs(table.Actions); err != nil {
		return fmt.Errorf("table actions: %w", err)
	}
	if table.AddButton != nil {
		if err := ValidateActionConfig(*table.AddButton); err != nil {
			return fmt.Errorf("table addButton: %w", err)
		}
	}
	if table.BulkActions != nil {
		if table.BulkActions.Edit != nil {
			if err := ValidateActionConfig(*table.BulkActions.Edit); err != nil {
				return fmt.Errorf("table bulkActions.edit: %w", err)
			}
		}
		if table.BulkActions.Delete != nil {
			if err := ValidateActionConfig(*table.BulkActions.Delete); err != nil {
				return fmt.Errorf("table bulkActions.delete: %w", err)
			}
		}
	}
	if err := ValidateFilterPanelConfig(table.FilterPanel); err != nil {
		return fmt.Errorf("table filterPanel: %w", err)
	}

	return nil
}

func ValidateComponentTableConfig(component *ComponentBlock) error {
	if component == nil {
		return nil
	}

	if component.Type == ComponentTypeTable {
		if err := ValidateTableComponentConfig(component.Table); err != nil {
			return fmt.Errorf("component '%s': %w", component.ID, err)
		}
	}
	if component.Type == ComponentTypeForm {
		if err := ValidateFormComponentConfig(component.Form); err != nil {
			return fmt.Errorf("component '%s': %w", component.ID, err)
		}
	}

	for i := range component.Tabs {
		for j := range component.Tabs[i].Components {
			if err := ValidateComponentTableConfig(&component.Tabs[i].Components[j]); err != nil {
				return err
			}
		}
	}

	return nil
}

func ValidatePageTableConfig(page *PageModel) error {
	if page == nil {
		return nil
	}

	for i := range page.Sections {
		section := &page.Sections[i]
		if err := ValidateComponentTableConfig(section.Component); err != nil {
			return err
		}
		if section.Grid != nil {
			for j := range section.Grid.Cells {
				for k := range section.Grid.Cells[j].Components {
					if err := ValidateComponentTableConfig(&section.Grid.Cells[j].Components[k]); err != nil {
						return err
					}
				}
			}
		}
		if section.Tabs != nil {
			for j := range section.Tabs.Tabs {
				for k := range section.Tabs.Tabs[j].Sections {
					tabSection := &section.Tabs.Tabs[j].Sections[k]
					if err := ValidateComponentTableConfig(tabSection.Component); err != nil {
						return err
					}
					if tabSection.Grid != nil {
						for l := range tabSection.Grid.Cells {
							for m := range tabSection.Grid.Cells[l].Components {
								if err := ValidateComponentTableConfig(&tabSection.Grid.Cells[l].Components[m]); err != nil {
									return err
								}
							}
						}
					}
				}
			}
		}
		for j := range section.Cells {
			for k := range section.Cells[j].Components {
				if err := ValidateComponentTableConfig(&section.Cells[j].Components[k]); err != nil {
					return err
				}
			}
		}
	}

	if page.SubPage != nil {
		if err := ValidatePageTableConfig(page.SubPage); err != nil {
			return err
		}
	}

	return nil
}

// ValidateFieldFrontendConfig validates the frontend configuration for a single field
func ValidateFieldFrontendConfig(field *Field) error {
	if field == nil {
		return nil
	}

	if field.Frontend != nil {
		if err := ValidateFrontendLinkConfig(field.Frontend); err != nil {
			return fmt.Errorf("field '%s': %w", field.Name, err)
		}
		if err := ValidateActionConfigs(field.Frontend.Actions); err != nil {
			return fmt.Errorf("field '%s': frontend actions: %w", field.Name, err)
		}
	}

	// Recursively validate children fields
	for i := range field.Children {
		if err := ValidateFieldFrontendConfig(&field.Children[i]); err != nil {
			return err
		}
	}

	return nil
}

// ValidateContainerFrontendConfig validates all frontend configurations in a ContainerModel
// This should be called during container creation or update
func ValidateContainerFrontendConfig(container *ContainerModel) error {
	if container == nil {
		return nil
	}

	for i := range container.Fields {
		if err := ValidateFieldFrontendConfig(&container.Fields[i]); err != nil {
			return fmt.Errorf("container '%s': %w", container.SchemaName, err)
		}
	}
	if container.Frontend != nil {
		if err := ValidateActionConfigs(container.Frontend.Actions); err != nil {
			return fmt.Errorf("container '%s': frontend actions: %w", container.SchemaName, err)
		}
	}

	return nil
}

// ValidateAuthContainerGoogleLoginConfig validates auth-container requirements that
// only apply when Google login is enabled.
func ValidateAuthContainerGoogleLoginConfig(container *ContainerModel) error {
	if container == nil || !container.IsAuthContainer || !container.IsGoogleLoginActive {
		return nil
	}

	for _, field := range container.Fields {
		if strings.EqualFold(strings.TrimSpace(field.Name), "email") {
			return nil
		}
	}

	return fmt.Errorf("auth container must have an email field when Google login is active")
}

// Example integration function showing how to use validation during container creation
func ValidateAndCreateContainer(container *ContainerModel) error {
	// Validate frontend link configurations
	if err := ValidateContainerFrontendConfig(container); err != nil {
		return fmt.Errorf("frontend validation failed: %w", err)
	}
	if err := ValidateAuthContainerGoogleLoginConfig(container); err != nil {
		return err
	}

	// Additional validation logic would go here
	// (e.g., schema name validation, field type validation, etc.)

	// Proceed with container creation
	// db.Collection("containers").InsertOne(ctx, container)

	return nil
}

// Example integration function showing how to use validation during container update
func ValidateAndUpdateContainer(container *ContainerModel) error {
	// Validate frontend link configurations
	if err := ValidateContainerFrontendConfig(container); err != nil {
		return fmt.Errorf("frontend validation failed: %w", err)
	}
	if err := ValidateAuthContainerGoogleLoginConfig(container); err != nil {
		return err
	}

	// Additional validation logic would go here

	// Proceed with container update
	// db.Collection("containers").UpdateOne(ctx, filter, update)

	return nil
}
