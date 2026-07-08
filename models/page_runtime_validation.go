package models

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type pageRuntimeGraph struct {
	components map[string]*ComponentBlock
	outputs    map[string]map[string]ComponentOutputDefinition
	filters    map[string]PageFilterDefinition
}

var safeRuntimeName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func ValidatePageRuntimeConfig(page *PageModel) error {
	if page == nil {
		return nil
	}
	if !hasPageRuntimeConfig(page) {
		return nil
	}
	for currentPage := page; currentPage != nil; currentPage = currentPage.SubPage {
		if err := validatePageVariables(currentPage.Variables); err != nil {
			return err
		}
	}

	components := collectPageComponents(page)
	graph := pageRuntimeGraph{
		components: make(map[string]*ComponentBlock, len(components)),
		outputs:    make(map[string]map[string]ComponentOutputDefinition, len(components)),
		filters:    make(map[string]PageFilterDefinition),
	}
	for currentPage := page; currentPage != nil; currentPage = currentPage.SubPage {
		filters, err := validatePageFilters(currentPage.Filters, collectPageCellIDs(currentPage))
		if err != nil {
			return err
		}
		for id, filter := range filters {
			if _, exists := graph.filters[id]; exists {
				return fmt.Errorf("duplicate page filter id %q", id)
			}
			graph.filters[id] = filter
		}
	}
	stateKeys := map[string]bool{}
	allFilterIDs := map[string]bool{}
	componentFilters := make(map[*ComponentBlock]map[string]ActionFormFieldConfig, len(components))

	for _, component := range components {
		if component.ID == "" {
			return fmt.Errorf("component requires id")
		}
		if _, exists := graph.components[component.ID]; exists {
			return fmt.Errorf("duplicate component id %q", component.ID)
		}
		graph.components[component.ID] = component
		if component.StateKey != "" {
			if stateKeys[component.StateKey] {
				return fmt.Errorf("duplicate component state key %q", component.StateKey)
			}
			stateKeys[component.StateKey] = true
		}

		filters := map[string]ActionFormFieldConfig{}
		if component.Table != nil && component.Table.FilterPanel != nil &&
			component.Table.FilterPanel.Inputs != nil {
			for _, filter := range *component.Table.FilterPanel.Inputs {
				if filter.ID == "" {
					continue
				}
				if allFilterIDs[filter.ID] {
					return fmt.Errorf("duplicate filter id %q", filter.ID)
				}
				allFilterIDs[filter.ID] = true
				filters[filter.ID] = filter
			}
		}
		componentFilters[component] = filters

		outputIDs := map[string]ComponentOutputDefinition{}
		outputKeys := map[string]bool{}
		for _, output := range component.Outputs {
			if output.ID == "" {
				return fmt.Errorf("component %q output requires id", component.ID)
			}
			if _, exists := outputIDs[output.ID]; exists {
				return fmt.Errorf("component %q has duplicate output id %q", component.ID, output.ID)
			}
			if output.Key == "" {
				return fmt.Errorf("component %q output %q requires key", component.ID, output.ID)
			}
			if outputKeys[output.Key] {
				return fmt.Errorf("component %q has duplicate output key %q", component.ID, output.Key)
			}
			if err := validateRuntimeValueType(output.Type); err != nil {
				return fmt.Errorf("component %q output %q: %w", component.ID, output.ID, err)
			}
			outputIDs[output.ID] = output
			outputKeys[output.Key] = true
		}
		graph.outputs[component.ID] = outputIDs
	}

	for _, component := range components {
		if component.DataBinding == nil {
			continue
		}
		parameters := make([]string, 0, len(component.DataBinding.Parameters))
		for parameter := range component.DataBinding.Parameters {
			parameters = append(parameters, parameter)
		}
		sort.Strings(parameters)
		for _, parameter := range parameters {
			binding := component.DataBinding.Parameters[parameter]
			if err := validateParameterBinding(binding, graph); err != nil {
				return fmt.Errorf("component %q parameter %q: %w", component.ID, parameter, err)
			}
		}
	}

	for _, component := range components {
		if err := validateComponentOutputs(component, componentFilters[component]); err != nil {
			return err
		}
	}

	return nil
}

func hasPageRuntimeConfig(page *PageModel) bool {
	for currentPage := page; currentPage != nil; currentPage = currentPage.SubPage {
		if len(currentPage.Variables) > 0 || len(currentPage.Filters) > 0 {
			return true
		}
	}
	for _, component := range collectPageComponents(page) {
		if len(component.Outputs) > 0 ||
			(component.DataBinding != nil && len(component.DataBinding.Parameters) > 0) {
			return true
		}
	}
	return false
}

func collectPageComponents(page *PageModel) []*ComponentBlock {
	if page == nil {
		return nil
	}

	var components []*ComponentBlock
	var collectComponent func(*ComponentBlock)
	collectComponent = func(component *ComponentBlock) {
		if component == nil {
			return
		}
		components = append(components, component)
		for i := range component.Tabs {
			for j := range component.Tabs[i].Components {
				collectComponent(&component.Tabs[i].Components[j])
			}
		}
	}

	var collectSection func(*Section)
	collectSection = func(section *Section) {
		if section == nil {
			return
		}
		collectComponent(section.Component)
		if section.Grid != nil {
			for i := range section.Grid.Cells {
				for j := range section.Grid.Cells[i].Components {
					collectComponent(&section.Grid.Cells[i].Components[j])
				}
			}
		}
		for i := range section.Cells {
			for j := range section.Cells[i].Components {
				collectComponent(&section.Cells[i].Components[j])
			}
		}
		if section.Tabs != nil {
			for i := range section.Tabs.Tabs {
				for j := range section.Tabs.Tabs[i].Sections {
					collectSection(&section.Tabs.Tabs[i].Sections[j])
				}
			}
		}
	}

	for i := range page.Sections {
		collectSection(&page.Sections[i])
	}
	components = append(components, collectPageComponents(page.SubPage)...)
	return components
}

func collectPageCellIDs(page *PageModel) map[string]struct{} {
	cellIDs := map[string]struct{}{}
	if page == nil {
		return cellIDs
	}

	var collectSection func(*Section)
	collectSection = func(section *Section) {
		if section == nil {
			return
		}
		collectCellIDs := func(cells []GridCell) {
			for i := range cells {
				if cells[i].ID != "" {
					cellIDs[cells[i].ID] = struct{}{}
				}
				for j := range cells[i].Components {
					for _, tab := range cells[i].Components[j].Tabs {
						for k := range tab.Components {
							_ = tab.Components[k]
						}
					}
				}
			}
		}
		if section.Grid != nil {
			collectCellIDs(section.Grid.Cells)
		}
		collectCellIDs(section.Cells)
		if section.Tabs != nil {
			for i := range section.Tabs.Tabs {
				for j := range section.Tabs.Tabs[i].Sections {
					collectSection(&section.Tabs.Tabs[i].Sections[j])
				}
			}
		}
	}

	for i := range page.Sections {
		collectSection(&page.Sections[i])
	}
	return cellIDs
}

func validatePageVariables(vars []PageVariableDefinition) error {
	ids := map[string]bool{}
	keys := map[string]bool{}
	for _, variable := range vars {
		if variable.ID == "" {
			return fmt.Errorf("page variable requires id")
		}
		if ids[variable.ID] {
			return fmt.Errorf("duplicate page variable id %q", variable.ID)
		}
		if variable.Key == "" {
			return fmt.Errorf("page variable %q requires key", variable.ID)
		}
		if keys[variable.Key] {
			return fmt.Errorf("duplicate page variable key %q", variable.Key)
		}
		if err := validateRuntimeValueType(variable.Type); err != nil {
			return fmt.Errorf("page variable %q: %w", variable.ID, err)
		}
		if variable.InitialValue != nil {
			if err := validateRuntimeValue(variable.InitialValue, variable.Type); err != nil {
				return fmt.Errorf("page variable %q initial value: %w", variable.ID, err)
			}
		}
		ids[variable.ID] = true
		keys[variable.Key] = true
	}
	return nil
}

func validatePageFilters(filters []PageFilterDefinition, cellIDs map[string]struct{}) (map[string]PageFilterDefinition, error) {
	byID := map[string]PageFilterDefinition{}
	keys := map[string]struct{}{}
	for _, filter := range filters {
		if filter.ID == "" {
			return nil, fmt.Errorf("page filter id is required")
		}
		if _, exists := byID[filter.ID]; exists {
			return nil, fmt.Errorf("duplicate page filter id %q", filter.ID)
		}
		if filter.Key == "" {
			return nil, fmt.Errorf("page filter %q requires key", filter.ID)
		}
		if !safeRuntimeName.MatchString(filter.Key) {
			return nil, fmt.Errorf("page filter key %q is invalid", filter.Key)
		}
		if _, exists := keys[filter.Key]; exists {
			return nil, fmt.Errorf("duplicate page filter key %q", filter.Key)
		}
		if err := validateRuntimeValueType(filter.Type); err != nil {
			return nil, fmt.Errorf("page filter %q: %w", filter.ID, err)
		}
		if filter.DefaultValue != nil {
			if err := validateRuntimeValue(filter.DefaultValue, filter.Type); err != nil {
				return nil, fmt.Errorf("page filter %q defaultValue: %w", filter.ID, err)
			}
		}
		if filter.DefaultPreset != "" && !isValidPageFilterDefaultPreset(filter.Type, filter.DefaultPreset) {
			return nil, fmt.Errorf("page filter %q defaultPreset is invalid", filter.ID)
		}
		switch filter.ArraySerialization {
		case "", PageFilterArraySerializationComma, PageFilterArraySerializationRepeat:
		default:
			return nil, fmt.Errorf("page filter %q arraySerialization is invalid", filter.ID)
		}
		switch filter.Placement.Kind {
		case PageFilterPlacementNavbar:
		case PageFilterPlacementCell:
			if filter.Placement.CellID == "" {
				return nil, fmt.Errorf("page filter %q cell placement requires cellId", filter.ID)
			}
			if _, exists := cellIDs[filter.Placement.CellID]; !exists {
				return nil, fmt.Errorf("page filter %q references unknown cell %q", filter.ID, filter.Placement.CellID)
			}
		default:
			return nil, fmt.Errorf("page filter %q placement kind is invalid", filter.ID)
		}
		byID[filter.ID] = filter
		keys[filter.Key] = struct{}{}
	}
	return byID, nil
}

func isValidPageFilterDefaultPreset(valueType RuntimeValueType, preset PageFilterDefaultPreset) bool {
	switch valueType {
	case RuntimeValueTypeDate:
		switch preset {
		case PageFilterDefaultPresetToday, PageFilterDefaultPresetYesterday, PageFilterDefaultPresetTomorrow:
			return true
		}
	case RuntimeValueTypeDateRange:
		switch preset {
		case PageFilterDefaultPresetToday,
			PageFilterDefaultPresetYesterday,
			PageFilterDefaultPresetThisWeek,
			PageFilterDefaultPresetLastWeek,
			PageFilterDefaultPresetThisMonth,
			PageFilterDefaultPresetLastMonth,
			PageFilterDefaultPresetThisYear,
			PageFilterDefaultPresetLastYear:
			return true
		}
	}
	return false
}

func validateComponentOutputs(component *ComponentBlock, filters map[string]ActionFormFieldConfig) error {
	for _, output := range component.Outputs {
		var expectedType RuntimeValueType
		switch output.Source.Kind {
		case ComponentOutputSourceTableFilter:
			if component.Type != ComponentTypeTable {
				return fmt.Errorf("component %q output %q: tableFilter source requires table component", component.ID, output.ID)
			}
			filter, exists := filters[output.Source.FilterID]
			if !exists {
				return fmt.Errorf("component %q output %q: referenced filter %q does not exist", component.ID, output.ID, output.Source.FilterID)
			}
			expectedType = runtimeTypeForFilter(filter)
		case ComponentOutputSourceTableSelectedIDs:
			if component.Type != ComponentTypeTable {
				return fmt.Errorf("component %q output %q: tableSelectedIds source requires table component", component.ID, output.ID)
			}
			expectedType = RuntimeValueTypeStringArray
		case ComponentOutputSourceTableSearch:
			if component.Type != ComponentTypeTable {
				return fmt.Errorf("component %q output %q: tableSearch source requires table component", component.ID, output.ID)
			}
			expectedType = RuntimeValueTypeString
		default:
			return fmt.Errorf("component %q output %q: unsupported output source %q", component.ID, output.ID, output.Source.Kind)
		}
		if output.Type != expectedType {
			return fmt.Errorf(
				"component %q output %q: output type %q does not match source type %q",
				component.ID, output.ID, output.Type, expectedType,
			)
		}
	}
	return nil
}

func validateParameterBinding(binding ParameterBinding, graph pageRuntimeGraph) error {
	switch binding.Source {
	case ParameterBindingSourceStatic:
		return validateStaticValue(binding.Value)
	case ParameterBindingSourcePageFilter:
		if binding.FilterID == "" {
			return fmt.Errorf("pageFilter binding requires filterId")
		}
		if _, exists := graph.filters[binding.FilterID]; !exists {
			return fmt.Errorf("pageFilter binding references unknown page filter %q", binding.FilterID)
		}
		return nil
	case ParameterBindingSourceComponentOutput:
		component, exists := graph.components[binding.ComponentID]
		if !exists {
			return fmt.Errorf("referenced component %q does not exist", binding.ComponentID)
		}
		output, exists := graph.outputs[component.ID][binding.OutputID]
		if !exists {
			return fmt.Errorf("referenced output %q does not exist on component %q", binding.OutputID, component.ID)
		}
		if output.Type == RuntimeValueTypeDateRange {
			switch binding.Field {
			case "", "start", "end", "preset", "timezone":
				return nil
			default:
				return fmt.Errorf("invalid date-range field %q", binding.Field)
			}
		}
		if binding.Field != "" {
			return fmt.Errorf("scalar output %q of type %q does not allow field accessor %q", output.ID, output.Type, binding.Field)
		}
		return nil
	case ParameterBindingSourcePageVariable,
		ParameterBindingSourceSystem,
		ParameterBindingSourceDerived:
		return fmt.Errorf("unsupported binding source %q", binding.Source)
	default:
		return fmt.Errorf("unsupported binding source %q", binding.Source)
	}
}

func validateRuntimeValue(value interface{}, valueType RuntimeValueType) error {
	switch valueType {
	case RuntimeValueTypeString:
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
	case RuntimeValueTypeNumber:
		if !isNumber(value) {
			return fmt.Errorf("expected number, got %T", value)
		}
	case RuntimeValueTypeBoolean:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected boolean, got %T", value)
		}
	case RuntimeValueTypeDate:
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected date string, got %T", value)
		}
	case RuntimeValueTypeDateRange:
		dateRange, ok := value.(map[string]interface{})
		if !ok {
			return fmt.Errorf("expected dateRange object, got %T", value)
		}
		for key := range dateRange {
			switch key {
			case "start", "end", "preset", "timezone":
			default:
				return fmt.Errorf("unexpected dateRange field %q", key)
			}
		}
		for _, key := range []string{"start", "end"} {
			if item, exists := dateRange[key]; exists {
				if _, ok := item.(string); !ok {
					return fmt.Errorf("expected dateRange %s to be string, got %T", key, item)
				}
			}
		}
		for _, key := range []string{"preset", "timezone"} {
			if item, exists := dateRange[key]; exists && item != nil {
				if _, ok := item.(string); !ok {
					return fmt.Errorf("expected dateRange %s to be string, got %T", key, item)
				}
			}
		}
	case RuntimeValueTypeStringArray:
		if err := validateArrayItems(value, func(item interface{}) bool {
			_, ok := item.(string)
			return ok
		}); err != nil {
			return fmt.Errorf("expected stringArray: %w", err)
		}
	case RuntimeValueTypeNumberArray:
		if err := validateArrayItems(value, isNumber); err != nil {
			return fmt.Errorf("expected numberArray: %w", err)
		}
	default:
		return fmt.Errorf("unsupported runtime value type %q", valueType)
	}
	return nil
}

func validateRuntimeValueType(valueType RuntimeValueType) error {
	switch valueType {
	case RuntimeValueTypeString,
		RuntimeValueTypeNumber,
		RuntimeValueTypeBoolean,
		RuntimeValueTypeDate,
		RuntimeValueTypeDateRange,
		RuntimeValueTypeStringArray,
		RuntimeValueTypeNumberArray:
		return nil
	default:
		return fmt.Errorf("unsupported runtime value type %q", valueType)
	}
}

func runtimeTypeForFilter(filter ActionFormFieldConfig) RuntimeValueType {
	valueType := strings.ToLower(filter.FormKeyType)
	if valueType == "" {
		valueType = strings.ToLower(filter.Type)
	}
	if filter.IsMultiple != nil && *filter.IsMultiple {
		if valueType == "number" || valueType == "numberarray" || valueType == "intarray" {
			return RuntimeValueTypeNumberArray
		}
		return RuntimeValueTypeStringArray
	}
	switch valueType {
	case "number":
		return RuntimeValueTypeNumber
	case "boolean", "checkbox":
		return RuntimeValueTypeBoolean
	case "numberarray", "intarray":
		return RuntimeValueTypeNumberArray
	case "stringarray":
		return RuntimeValueTypeStringArray
	default:
		return RuntimeValueTypeString
	}
}

func validateStaticValue(value interface{}) error {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case json.Number:
		number, err := strconv.ParseFloat(string(typed), 64)
		if err != nil {
			return fmt.Errorf("invalid JSON number %q", typed)
		}
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return fmt.Errorf("non-finite number %q", typed)
		}
		return nil
	case primitive.Decimal128:
		if typed.IsNaN() || typed.IsInf() != 0 {
			return fmt.Errorf("non-finite BSON decimal %q", typed.String())
		}
		return nil
	case primitive.D:
		for _, element := range typed {
			if err := validateStaticValue(element.Value); err != nil {
				return fmt.Errorf("object field %q: %w", element.Key, err)
			}
		}
		return nil
	}

	valueOf := reflect.ValueOf(value)
	switch valueOf.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return nil
	case reflect.Float32, reflect.Float64:
		number := valueOf.Float()
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return fmt.Errorf("non-finite number %v", number)
		}
		return nil
	case reflect.Array, reflect.Slice:
		for index := 0; index < valueOf.Len(); index++ {
			if err := validateStaticValue(valueOf.Index(index).Interface()); err != nil {
				return fmt.Errorf("array item %d: %w", index, err)
			}
		}
		return nil
	case reflect.Map:
		if valueOf.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("non-string object key type %s", valueOf.Type().Key())
		}
		iter := valueOf.MapRange()
		for iter.Next() {
			key := iter.Key().String()
			if err := validateStaticValue(iter.Value().Interface()); err != nil {
				return fmt.Errorf("object field %q: %w", key, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported static value type %T", value)
	}
}

func validateArrayItems(value interface{}, valid func(interface{}) bool) error {
	valueOf := reflect.ValueOf(value)
	if !valueOf.IsValid() || valueOf.Kind() != reflect.Slice {
		return fmt.Errorf("got %T", value)
	}
	for i := 0; i < valueOf.Len(); i++ {
		if !valid(valueOf.Index(i).Interface()) {
			return fmt.Errorf("item %d has type %T", i, valueOf.Index(i).Interface())
		}
	}
	return nil
}

func isNumber(value interface{}) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	default:
		return false
	}
}
