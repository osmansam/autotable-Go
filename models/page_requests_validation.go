package models

import (
	"fmt"
	"math"
	"reflect"
	"strings"
)

func requestReservedName(name string) bool {
	switch name {
	case "__proto__", "prototype", "constructor", "schemaName", "sourceType", "pipelineName", "workflowName", "page", "limit", "sort", "asc", "search", "fields", "fieldName", "valueField", "dataFields":
		return true
	}
	return false
}

func validateRequestResult(result PageRequestResult) error {
	if result.Kind != "records" && result.Kind != "value" {
		return fmt.Errorf("result must be records or value")
	}
	for _, path := range [][]string{result.Path, result.TotalPath} {
		for _, key := range path {
			if key == "" || key == "__proto__" || key == "prototype" || key == "constructor" {
				return fmt.Errorf("unsafe result path")
			}
		}
	}
	return nil
}

func requestControlAllowed(source PageRequestSource, key string) bool {
	switch source.Transport {
	case "table-source", "page":
		return key == "page" || key == "limit" || key == "sort" || key == "asc" || key == "search"
	case "list":
		return key == "sort" || key == "asc" || key == "search"
	case "selection":
		return key == "limit"
	}
	return false
}

func validateRequestValues(source PageRequestSource, values *PageRequestValues) error {
	if values == nil {
		return nil
	}
	for _, bag := range []map[string]interface{}{values.Params, values.Filters} {
		for key, value := range bag {
			if key == "" || requestReservedName(key) {
				return fmt.Errorf("reserved request parameter %q", key)
			}
			if err := validateStaticValue(value); err != nil {
				return err
			}
		}
	}
	for key, value := range values.Controls {
		if !requestControlAllowed(source, key) {
			return fmt.Errorf("unsupported control %q", key)
		}
		switch key {
		case "page", "limit":
			if !isNumber(value) {
				return fmt.Errorf("%s must be a positive integer", key)
			}
			n := reflect.ValueOf(value).Convert(reflect.TypeOf(float64(0))).Float()
			if n < 1 || math.IsInf(n, 0) || math.IsNaN(n) || n != math.Trunc(n) {
				return fmt.Errorf("%s must be a positive integer", key)
			}
		case "asc":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("asc must be boolean")
			}
		case "sort", "search":
			if value != nil {
				if _, ok := value.(string); !ok {
					return fmt.Errorf("%s must be text or null", key)
				}
			}
		}
	}
	if values.Fields != nil && source.Transport != "table-source" {
		return fmt.Errorf("fields require a table source")
	}
	return nil
}

func validatePageRequests(page *PageModel, graph pageRuntimeGraph) error {
	requests := map[string]PageRequestDefinition{}
	edges := map[string][]string{}
	for current := page; current != nil; current = current.SubPage {
		for _, request := range current.Requests {
			if strings.TrimSpace(request.ID) == "" || strings.TrimSpace(request.Name) == "" {
				return fmt.Errorf("page request requires id and name")
			}
			if _, exists := requests[request.ID]; exists {
				return fmt.Errorf("duplicate page request %q", request.ID)
			}
			source := request.Source
			if strings.TrimSpace(source.SchemaName) == "" {
				return fmt.Errorf("page request %q requires schema", request.ID)
			}
			switch source.Transport {
			case "table-source":
				if source.Kind != "schema" && source.Kind != "pipeline" && source.Kind != "workflow" {
					return fmt.Errorf("invalid table source kind")
				}
			case "list", "page", "pipeline", "workflow", "selection":
			default:
				return fmt.Errorf("unsupported page request transport %q", source.Transport)
			}
			if (source.Transport == "pipeline" || source.Transport == "table-source" && source.Kind == "pipeline") && source.PipelineName == "" {
				return fmt.Errorf("pipeline name required")
			}
			if (source.Transport == "workflow" || source.Transport == "table-source" && source.Kind == "workflow") && source.WorkflowName == "" {
				return fmt.Errorf("workflow name required")
			}
			if source.Transport == "selection" && source.FieldName == "" {
				return fmt.Errorf("selection label field required")
			}
			if err := validateRequestResult(request.Result); err != nil {
				return fmt.Errorf("request %q: %w", request.ID, err)
			}
			if err := validateRequestValues(source, request.Defaults); err != nil {
				return fmt.Errorf("request %q: %w", request.ID, err)
			}
			if request.Bindings != nil {
				for _, entry := range []struct {
					bag      map[string]ParameterBinding
					controls bool
				}{{request.Bindings.Params, false}, {request.Bindings.Filters, false}, {request.Bindings.Controls, true}} {
					for key, binding := range entry.bag {
						if entry.controls {
							if !requestControlAllowed(source, key) {
								return fmt.Errorf("unsupported bound control %q", key)
							}
						} else if requestReservedName(key) {
							return fmt.Errorf("reserved request parameter %q", key)
						}
						if err := validateParameterBinding(binding, graph); err != nil {
							return fmt.Errorf("request %q binding %q: %w", request.ID, key, err)
						}
						if binding.Source == ParameterBindingSourceComponentOutput {
							edges["request:"+request.ID] = append(edges["request:"+request.ID], "component:"+binding.ComponentID)
						}
					}
				}
			}
			requests[request.ID] = request
		}
	}
	for id, component := range graph.components {
		if component.DataBinding != nil {
			for _, binding := range component.DataBinding.Parameters {
				if binding.Source == ParameterBindingSourceComponentOutput {
					edges["component:"+id] = append(edges["component:"+id], "component:"+binding.ComponentID)
				}
			}
		}
		for slot, connection := range component.RequestBindings {
			request, ok := requests[connection.RequestID]
			if !ok {
				return fmt.Errorf("component %q slot %q references missing page request %q", id, slot, connection.RequestID)
			}
			if slot == "" || strings.Contains(slot, "__proto__") {
				return fmt.Errorf("invalid request slot")
			}
			if err := validateRequestValues(request.Source, connection.Overrides); err != nil {
				return fmt.Errorf("component %q slot %q: %w", id, slot, err)
			}
			result := request.Result
			if connection.Result != nil {
				result = *connection.Result
			}
			if err := validateRequestResult(result); err != nil {
				return err
			}
			if result.Kind != "records" && (component.Type == ComponentTypeTable || slot == "rows" || slot == "columns" || strings.HasPrefix(slot, "collection:") || strings.Contains(slot, "Options:") || strings.HasPrefix(slot, "options:")) {
				return fmt.Errorf("component %q slot %q requires records", id, slot)
			}
			edges["component:"+id] = append(edges["component:"+id], "request:"+connection.RequestID)
		}
	}
	state := map[string]int{}
	var visit func(string) error
	visit = func(node string) error {
		if state[node] == 1 {
			return fmt.Errorf("page request dependency cycle at %q", node)
		}
		if state[node] == 2 {
			return nil
		}
		state[node] = 1
		for _, next := range edges[node] {
			if err := visit(next); err != nil {
				return err
			}
		}
		state[node] = 2
		return nil
	}
	for node := range edges {
		if err := visit(node); err != nil {
			return err
		}
	}
	return nil
}
