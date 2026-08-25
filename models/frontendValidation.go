package models

import (
	"fmt"
	"math"
	"regexp"
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
	for key := range action.ConstantValues {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("constantValues requires non-empty keys")
		}
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
		if err := validateSelectOptionConfig(field); err != nil {
			return fmt.Errorf("form field '%s': %w", field.FormKey, err)
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
	if err := validateFormCalculationConfig(*form); err != nil {
		return err
	}
	return nil
}

var formCurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
var selectOptionTemplatePattern = regexp.MustCompile(`{{\s*([A-Za-z_][A-Za-z0-9_.]*)\s*}}`)

func selectOptionTemplateFields(template string) ([]string, error) {
	fields := []string{}
	for _, match := range selectOptionTemplatePattern.FindAllStringSubmatch(template, -1) {
		fields = append(fields, match[1])
	}
	remainder := selectOptionTemplatePattern.ReplaceAllString(template, "")
	if strings.Contains(remainder, "{{") || strings.Contains(remainder, "}}") {
		return nil, fmt.Errorf("option display template has malformed field reference")
	}
	return fields, nil
}

func validateSelectOptionConfig(field FormFieldConfig) error {
	for _, sourceField := range field.SourceDataFields {
		if strings.TrimSpace(sourceField) == "" {
			return fmt.Errorf("sourceDataFields cannot contain blank fields")
		}
	}
	if field.OptionDisplay == nil {
		return nil
	}
	if _, err := selectOptionTemplateFields(field.OptionDisplay.LeftTemplate); err != nil {
		return err
	}
	if _, err := selectOptionTemplateFields(field.OptionDisplay.RightTemplate); err != nil {
		return err
	}
	return nil
}

func effectiveSelectSourceFields(field FormFieldConfig) map[string]bool {
	available := map[string]bool{"_id": true}
	for _, sourceField := range []string{field.SourceValueField, field.SourceLabelField} {
		if sourceField != "" {
			available[sourceField] = true
		}
	}
	for _, sourceField := range field.SourceDataFields {
		available[strings.TrimSpace(sourceField)] = true
	}
	if field.OptionDisplay != nil {
		for _, template := range []string{field.OptionDisplay.LeftTemplate, field.OptionDisplay.RightTemplate} {
			fields, _ := selectOptionTemplateFields(template)
			for _, sourceField := range fields {
				available[sourceField] = true
			}
		}
	}
	return available
}

func validateFormCalculationConfig(form FormComponentConfig) error {
	fields := make(map[string]FormFieldConfig, len(form.Fields))
	parentFields := make(map[string]bool, len(form.Fields))
	for _, field := range form.Fields {
		fields[field.FormKey] = field
		parentFields[field.FormKey] = true
	}

	lists := make(map[string]map[string]bool, len(form.ObjectLists))
	for _, list := range form.ObjectLists {
		available := make(map[string]bool)
		for _, field := range list.ItemFields {
			available[field] = true
		}
		for _, field := range form.Fields {
			if field.Type != "select" || field.OptionsSource != "schema" {
				continue
			}
			for _, sourceField := range field.SourceDataFields {
				sourceField = strings.TrimSpace(sourceField)
				if sourceField != "" {
					available[field.FormKey+"."+sourceField] = true
				}
			}
		}
		for index, mapping := range list.FieldMappings {
			if mapping.SourceFormKey == "" || mapping.SourceField == "" || mapping.TargetField == "" {
				return fmt.Errorf("object list '%s' field mapping %d requires sourceFormKey, sourceField, and targetField", list.Key, index)
			}
			source, ok := fields[mapping.SourceFormKey]
			if !ok || source.Type != "select" || source.OptionsSource != "schema" || source.SourceSchemaName == "" {
				return fmt.Errorf("object list '%s' field mapping %d sourceFormKey must reference a schema-backed select", list.Key, index)
			}
			if !effectiveSelectSourceFields(source)[mapping.SourceField] {
				return fmt.Errorf("object list '%s' field mapping %d must reference an available source field", list.Key, index)
			}
			if available[mapping.TargetField] {
				return fmt.Errorf("object list '%s' has duplicate item target '%s'", list.Key, mapping.TargetField)
			}
			available[mapping.TargetField] = true
		}
		for index, calculation := range list.ItemCalculations {
			if calculation.Operation != "multiply" && calculation.Operation != "quantityDiscount" {
				return fmt.Errorf("object list '%s' item calculation %d has unsupported item calculation operation '%s'", list.Key, index, calculation.Operation)
			}
			if len(calculation.Inputs) != 2 {
				return fmt.Errorf("object list '%s' %s calculation %d requires exactly two inputs", list.Key, calculation.Operation, index)
			}
			for _, input := range calculation.Inputs {
				if !available[input] {
					return fmt.Errorf("object list '%s' item calculation %d references unknown input '%s'", list.Key, index, input)
				}
				if calculation.TargetField == input || calculation.OriginalTargetField == input {
					return fmt.Errorf("object list '%s' item calculation %d cannot overwrite an input field", list.Key, index)
				}
			}
			if calculation.Operation == "quantityDiscount" {
				if calculation.OriginalTargetField == "" {
					return fmt.Errorf("object list '%s' quantity discount calculation %d requires originalTargetField", list.Key, index)
				}
				if calculation.OriginalTargetField == calculation.TargetField {
					return fmt.Errorf("object list '%s' quantity discount calculation %d requires distinct output fields", list.Key, index)
				}
				if available[calculation.OriginalTargetField] {
					return fmt.Errorf("object list '%s' has duplicate item target '%s'", list.Key, calculation.OriginalTargetField)
				}
				if calculation.MinimumQuantity == nil || math.IsNaN(*calculation.MinimumQuantity) || math.IsInf(*calculation.MinimumQuantity, 0) || *calculation.MinimumQuantity <= 0 {
					return fmt.Errorf("object list '%s' quantity discount calculation %d minimumQuantity must be greater than 0", list.Key, index)
				}
				if calculation.DiscountPercentage == nil || math.IsNaN(*calculation.DiscountPercentage) || math.IsInf(*calculation.DiscountPercentage, 0) || *calculation.DiscountPercentage <= 0 {
					return fmt.Errorf("object list '%s' quantity discount calculation %d discountPercentage must be greater than 0", list.Key, index)
				}
				if *calculation.DiscountPercentage > 100 {
					return fmt.Errorf("object list '%s' quantity discount calculation %d discountPercentage must not exceed 100", list.Key, index)
				}
			}
			if calculation.TargetField == "" || available[calculation.TargetField] {
				return fmt.Errorf("object list '%s' has duplicate item target '%s'", list.Key, calculation.TargetField)
			}
			if err := validateFormPrecision(calculation.Precision); err != nil {
				return fmt.Errorf("object list '%s' item calculation %d: %w", list.Key, index, err)
			}
			if calculation.Operation == "quantityDiscount" {
				available[calculation.OriginalTargetField] = true
			}
			available[calculation.TargetField] = true
		}
		if list.Display != nil && list.Display.PriceComparison != nil {
			comparison := list.Display.PriceComparison
			if comparison.OriginalField == "" || comparison.DiscountedField == "" {
				return fmt.Errorf("object list '%s' price comparison requires originalField and discountedField", list.Key)
			}
			if !available[comparison.OriginalField] {
				return fmt.Errorf("object list '%s' price comparison originalField references unknown item field '%s'", list.Key, comparison.OriginalField)
			}
			if !available[comparison.DiscountedField] {
				return fmt.Errorf("object list '%s' price comparison discountedField references unknown item field '%s'", list.Key, comparison.DiscountedField)
			}
			if comparison.Currency != "" && !formCurrencyPattern.MatchString(comparison.Currency) {
				return fmt.Errorf("object list '%s' price comparison currency must be three uppercase ASCII letters", list.Key)
			}
			if err := validateFormPrecision(comparison.Precision); err != nil {
				return fmt.Errorf("object list '%s' price comparison: %w", list.Key, err)
			}
		}
		lists[list.Key] = available
	}

	availableSummaries := make(map[string]bool)
	summaryTargets := make(map[string]bool)
	for index, summary := range form.Summaries {
		if summary.Key == "" || summary.TargetField == "" {
			return fmt.Errorf("form summary %d requires key and targetField", index)
		}
		if summaryTargets[summary.TargetField] {
			return fmt.Errorf("form summary %d has duplicate summary target '%s'", index, summary.TargetField)
		}
		if parentFields[summary.TargetField] {
			return fmt.Errorf("form summary target '%s' collides with form field", summary.TargetField)
		}
		switch summary.Operation {
		case "sum":
			itemFields, ok := lists[summary.ObjectListKey]
			if !ok {
				return fmt.Errorf("form summary %d references unknown object list '%s'", index, summary.ObjectListKey)
			}
			if !itemFields[summary.SourceField] {
				return fmt.Errorf("form summary %d references unknown item field '%s'", index, summary.SourceField)
			}
		case "copy":
			if !availableSummaries[summary.SourceField] {
				return fmt.Errorf("form summary %d references unknown earlier summary '%s'", index, summary.SourceField)
			}
		default:
			return fmt.Errorf("form summary %d has unsupported summary operation '%s'", index, summary.Operation)
		}
		if summary.Format != nil {
			if summary.Format.Currency != "" && !formCurrencyPattern.MatchString(summary.Format.Currency) {
				return fmt.Errorf("form summary %d currency must be three uppercase ASCII letters", index)
			}
			if err := validateFormPrecision(summary.Format.Precision); err != nil {
				return fmt.Errorf("form summary %d: %w", index, err)
			}
		}
		summaryTargets[summary.TargetField] = true
		availableSummaries[summary.TargetField] = true
	}
	return nil
}

func validateFormPrecision(precision *int) error {
	if precision != nil && (*precision < 0 || *precision > 6) {
		return fmt.Errorf("precision must be between 0 and 6")
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

func validateToggleRequestEffect(effect *ToggleRequestEffect) error {
	if effect == nil {
		return nil
	}
	switch effect.Type {
	case "set":
		if strings.TrimSpace(effect.Field) == "" {
			return fmt.Errorf("set effect requires field")
		}
	case "omit":
		if strings.TrimSpace(effect.Field) != "" || effect.Value != nil {
			return fmt.Errorf("omit effect cannot define field or value")
		}
	default:
		return fmt.Errorf("invalid request effect type '%s'", effect.Type)
	}
	return nil
}

func validateTableToggles(table *TableComponentConfig) error {
	toggleIDs := make(map[string]bool, len(table.Toggles))
	for index, toggle := range table.Toggles {
		id := strings.TrimSpace(toggle.ID)
		if id == "" {
			return fmt.Errorf("table toggle %d requires id", index)
		}
		if toggleIDs[id] {
			return fmt.Errorf("table toggle id '%s' must be unique", id)
		}
		toggleIDs[id] = true
		if toggle.Request != nil {
			if err := validateToggleRequestEffect(toggle.Request.On); err != nil {
				return fmt.Errorf("table toggle '%s' on: %w", id, err)
			}
			if err := validateToggleRequestEffect(toggle.Request.Off); err != nil {
				return fmt.Errorf("table toggle '%s' off: %w", id, err)
			}
		}
	}

	for _, column := range table.Columns {
		bindings := []struct {
			name    string
			binding *ToggleBinding
		}{
			{name: "visibilityToggle", binding: column.VisibilityToggle},
			{name: "booleanEditToggle", binding: column.BooleanEditToggle},
			{name: "booleanDisplayToggle", binding: column.BooleanDisplayToggle},
		}
		for _, candidate := range bindings {
			if candidate.binding == nil {
				continue
			}
			if !toggleIDs[strings.TrimSpace(candidate.binding.ToggleID)] {
				return fmt.Errorf("table column '%s' %s references unknown toggle '%s'", column.Field, candidate.name, candidate.binding.ToggleID)
			}
		}
	}
	groupIDs := make(map[string]bool, len(table.GeneratedRelationColumns))
	for index, group := range table.GeneratedRelationColumns {
		id := strings.TrimSpace(group.ID)
		if id == "" {
			return fmt.Errorf("generated relation column group %d requires id", index)
		}
		if groupIDs[id] {
			return fmt.Errorf("generated relation column group id '%s' must be unique", id)
		}
		groupIDs[id] = true
		if strings.TrimSpace(group.ArrayField) == "" || strings.TrimSpace(group.SourceSchemaName) == "" || strings.TrimSpace(group.SourceLabelField) == "" {
			return fmt.Errorf("generated relation column group '%s' requires arrayField, sourceSchemaName, and sourceLabelField", id)
		}
		if group.SourceLimit < 0 || group.SourceLimit > 100 {
			return fmt.Errorf("generated relation column group '%s' sourceLimit must be between 1 and 100", id)
		}
		if group.VisibilityToggle != nil && !toggleIDs[strings.TrimSpace(group.VisibilityToggle.ToggleID)] {
			return fmt.Errorf("generated relation column group '%s' visibilityToggle references unknown toggle '%s'", id, group.VisibilityToggle.ToggleID)
		}
		if group.BooleanEditToggle != nil && !toggleIDs[strings.TrimSpace(group.BooleanEditToggle.ToggleID)] {
			return fmt.Errorf("generated relation column group '%s' booleanEditToggle references unknown toggle '%s'", id, group.BooleanEditToggle.ToggleID)
		}
	}
	return nil
}

func validateTableDateFormat(format string) error {
	if format == "" {
		return nil
	}
	for _, allowed := range []string{
		"MM/DD/YYYY", "DD/MM/YYYY", "YYYY/MM/DD",
		"DD-MM-YYYY", "MM-DD-YYYY", "YYYY-MM-DD",
	} {
		if format == allowed {
			return nil
		}
	}
	return fmt.Errorf("invalid dateFormat '%s'", format)
}

func ValidateTableComponentConfig(table *TableComponentConfig) error {
	if table == nil {
		return nil
	}
	if table.DataMode != "" && table.DataMode != "paginated" && table.DataMode != "all" && table.DataMode != "arrayField" {
		return fmt.Errorf("invalid table dataMode %q", table.DataMode)
	}
	if table.DataMode == "arrayField" && (table.ArraySource == nil || !table.ArraySource.Enabled) {
		return fmt.Errorf("table dataMode arrayField requires enabled arraySource")
	}
	if table.ArraySource != nil && table.ArraySource.Enabled {
		if strings.TrimSpace(table.ArraySource.Field) == "" {
			return fmt.Errorf("table arraySource requires field")
		}
		if strings.TrimSpace(table.ArraySource.RowIdentityField) == "" {
			return fmt.Errorf("table arraySource requires rowIdentityField")
		}
		autoGenerate := table.ArraySource.AutoGenerate
		writable := autoGenerate != nil && (autoGenerate.Add || autoGenerate.Edit || autoGenerate.Delete || autoGenerate.Reorder)
		if writable && table.ArraySource.ParentID == nil {
			return fmt.Errorf("writable table arraySource requires parentId")
		}
		if autoGenerate != nil && autoGenerate.Reorder && (table.Drag == nil || !table.Drag.Enabled || strings.TrimSpace(table.Drag.OrderField) == "") {
			return fmt.Errorf("generated array reorder requires enabled table drag with orderField")
		}
	}
	dataFields := map[string]struct{}{}
	for _, field := range table.DataFields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			return fmt.Errorf("table dataFields requires non-empty fields")
		}
		if _, exists := dataFields[trimmed]; exists {
			return fmt.Errorf("table dataFields field '%s' is duplicated", trimmed)
		}
		dataFields[trimmed] = struct{}{}
	}
	if table.Drag != nil && table.Drag.Enabled && strings.TrimSpace(table.Drag.OrderField) == "" {
		return fmt.Errorf("enabled table drag configuration requires orderField")
	}
	if err := validateTableToggles(table); err != nil {
		return err
	}

	for _, column := range table.Columns {
		if err := validateTableDateFormat(column.DateFormat); err != nil {
			return fmt.Errorf("table column '%s': %w", column.Field, err)
		}
		if column.Type == "template" && strings.TrimSpace(column.Template) == "" {
			return fmt.Errorf("table column '%s' requires template", column.Field)
		}
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
			if err := validateTableDateFormat(column.DateFormat); err != nil {
				return fmt.Errorf("table nestedRows column '%s': %w", column.Field, err)
			}
		}
	}
	for key := range table.ConstantFilters {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			return fmt.Errorf("table constantFilters requires non-empty keys")
		}
		if strings.HasPrefix(trimmed, "$") {
			return fmt.Errorf("table constantFilters key '%s' is invalid", key)
		}
	}
	if table.ConstantSort != nil {
		if strings.TrimSpace(table.ConstantSort.Sort) == "" {
			return fmt.Errorf("table constantSort requires sort")
		}
		switch value := table.ConstantSort.Asc.(type) {
		case nil, bool, int, int32, int64, float32, float64, string:
		default:
			return fmt.Errorf("table constantSort asc has invalid type %T", value)
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
	if component.Type == ComponentTypeRelationMatrix {
		if err := ValidateRelationMatrixConfig(component.RelationMatrix); err != nil {
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

// ValidateRelationMatrixConfig ensures inverse membership matrices have a
// complete row, column, and embedded-array contract before persistence.
func ValidateRelationMatrixConfig(config *RelationMatrixConfig) error {
	if config == nil {
		return fmt.Errorf("relationMatrix configuration is required")
	}
	required := map[string]string{
		"rowSchemaName":        config.RowSchemaName,
		"rowIdField":           config.RowIDField,
		"rowLabelField":        config.RowLabelField,
		"columnSchemaName":     config.ColumnSchemaName,
		"columnIdField":        config.ColumnIDField,
		"columnLabelField":     config.ColumnLabelField,
		"targetArrayField":     config.TargetArrayField,
		"targetItemMatchField": config.TargetItemMatchField,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("relationMatrix %s is required", field)
		}
	}
	if config.ColumnLimit < 0 || config.ColumnLimit > 100 {
		return fmt.Errorf("relationMatrix columnLimit must be between 1 and 100")
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
