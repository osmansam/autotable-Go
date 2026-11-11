package models

// PageRequestSource selects an existing data-read transport. Actions never use this contract.
type PageRequestSource struct {
	Transport    string   `bson:"transport" json:"transport"`
	Kind         string   `bson:"kind,omitempty" json:"kind,omitempty"`
	SchemaName   string   `bson:"schemaName" json:"schemaName"`
	PipelineName string   `bson:"pipelineName,omitempty" json:"pipelineName,omitempty"`
	WorkflowName string   `bson:"workflowName,omitempty" json:"workflowName,omitempty"`
	FieldName    string   `bson:"fieldName,omitempty" json:"fieldName,omitempty"`
	ValueField   string   `bson:"valueField,omitempty" json:"valueField,omitempty"`
	DataFields   []string `bson:"dataFields,omitempty" json:"dataFields,omitempty"`
}

type PageRequestValues struct {
	Params   map[string]interface{} `bson:"params,omitempty" json:"params,omitempty"`
	Filters  map[string]interface{} `bson:"filters,omitempty" json:"filters,omitempty"`
	Controls map[string]interface{} `bson:"controls,omitempty" json:"controls,omitempty"`
	Fields   *[]string              `bson:"fields,omitempty" json:"fields,omitempty"`
}

type PageRequestParameterBindings struct {
	Params   map[string]ParameterBinding `bson:"params,omitempty" json:"params,omitempty"`
	Filters  map[string]ParameterBinding `bson:"filters,omitempty" json:"filters,omitempty"`
	Controls map[string]ParameterBinding `bson:"controls,omitempty" json:"controls,omitempty"`
}

type PageRequestResult struct {
	Kind      string   `bson:"kind" json:"kind"`
	Path      []string `bson:"path,omitempty" json:"path,omitempty"`
	TotalPath []string `bson:"totalPath,omitempty" json:"totalPath,omitempty"`
}

type PageRequestDefinition struct {
	ID       string                        `bson:"id" json:"id"`
	Name     string                        `bson:"name" json:"name"`
	Source   PageRequestSource             `bson:"source" json:"source"`
	Defaults *PageRequestValues            `bson:"defaults,omitempty" json:"defaults,omitempty"`
	Bindings *PageRequestParameterBindings `bson:"bindings,omitempty" json:"bindings,omitempty"`
	Result   PageRequestResult             `bson:"result" json:"result"`
}

type PageRequestConnection struct {
	RequestID string             `bson:"requestId" json:"requestId"`
	Overrides *PageRequestValues `bson:"overrides,omitempty" json:"overrides,omitempty"`
	Result    *PageRequestResult `bson:"result,omitempty" json:"result,omitempty"`
}
