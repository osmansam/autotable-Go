package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type RowClassConfig struct {
	Condition string `bson:"condition" json:"condition"`
	ClassName string `bson:"className" json:"className"`
}

type Frontend struct {
	DisplayName     string           `bson:"displayName,omitempty"`
	RowClassName    []RowClassConfig `bson:"rowClassName,omitempty"`
	RowKeyClassName []RowClassConfig `bson:"rowKeyClassName,omitempty"`
	InvalidateKeys  []string         `json:"invalidateKeys" bson:"invalidateKeys"`
	LinkTemplate    string           `bson:"linkTemplate,omitempty" json:"linkTemplate,omitempty"`
	LinkLabelField  string           `bson:"linkLabelField,omitempty" json:"linkLabelField,omitempty"`
	LinkType        string           `bson:"linkType,omitempty" json:"linkType,omitempty"`
	Actions         []ActionConfig   `bson:"actions,omitempty" json:"actions,omitempty"`
}

type ActionFieldConfig struct {
	Field             string      `bson:"field,omitempty" json:"field,omitempty"`
	Name              string      `bson:"name,omitempty" json:"name,omitempty"`
	Required          *bool       `bson:"required,omitempty" json:"required,omitempty"`
	Disabled          *bool       `bson:"disabled,omitempty" json:"disabled,omitempty"`
	Hidden            *bool       `bson:"hidden,omitempty" json:"hidden,omitempty"`
	ConstantValue     interface{} `bson:"constantValue,omitempty" json:"constantValue,omitempty"`
	DefaultValue      interface{} `bson:"defaultValue,omitempty" json:"defaultValue,omitempty"`
	RequiredCondition string      `bson:"requiredCondition,omitempty" json:"requiredCondition,omitempty"`
	DisabledCondition string      `bson:"disabledCondition,omitempty" json:"disabledCondition,omitempty"`
}

type ActionFormOptionConfig struct {
	Value interface{} `bson:"value" json:"value"`
	Label string      `bson:"label" json:"label"`
}

type ActionFormFieldConfig struct {
	ID                    string                   `bson:"id,omitempty" json:"id,omitempty"`
	FormKey               string                   `bson:"formKey" json:"formKey"`
	Type                  string                   `bson:"type" json:"type"`
	FormKeyType           string                   `bson:"formKeyType,omitempty" json:"formKeyType,omitempty"`
	Label                 string                   `bson:"label,omitempty" json:"label,omitempty"`
	Placeholder           string                   `bson:"placeholder,omitempty" json:"placeholder,omitempty"`
	Required              *bool                    `bson:"required,omitempty" json:"required,omitempty"`
	RequiredCondition     string                   `bson:"requiredCondition,omitempty" json:"requiredCondition,omitempty"`
	DisabledCondition     string                   `bson:"disabledCondition,omitempty" json:"disabledCondition,omitempty"`
	IsDisabled            *bool                    `bson:"isDisabled,omitempty" json:"isDisabled,omitempty"`
	IsMultiple            *bool                    `bson:"isMultiple,omitempty" json:"isMultiple,omitempty"`
	IsNumberButtonsActive *bool                    `bson:"isNumberButtonsActive,omitempty" json:"isNumberButtonsActive,omitempty"`
	OptionsSource         string                   `bson:"optionsSource,omitempty" json:"optionsSource,omitempty"`
	StaticOptions         []ActionFormOptionConfig `bson:"staticOptions,omitempty" json:"staticOptions,omitempty"`
	StaticOptionsJson     string                   `bson:"staticOptionsJson,omitempty" json:"staticOptionsJson,omitempty"`
	SourceSchemaName      string                   `bson:"sourceSchemaName,omitempty" json:"sourceSchemaName,omitempty"`
	SourceValueField      string                   `bson:"sourceValueField,omitempty" json:"sourceValueField,omitempty"`
	SourceLabelField      string                   `bson:"sourceLabelField,omitempty" json:"sourceLabelField,omitempty"`
	SourceRequestFilters  map[string]interface{}   `bson:"sourceRequestFilters,omitempty" json:"sourceRequestFilters,omitempty"`
	SourceFilterCondition string                   `bson:"sourceFilterCondition,omitempty" json:"sourceFilterCondition,omitempty"`
	InvalidateKeys        []string                 `bson:"invalidateKeys,omitempty" json:"invalidateKeys,omitempty"`
	DefaultValue          interface{}              `bson:"defaultValue,omitempty" json:"defaultValue,omitempty"`
	Min                   interface{}              `bson:"min,omitempty" json:"min,omitempty"`
	Max                   interface{}              `bson:"max,omitempty" json:"max,omitempty"`
	MinLength             interface{}              `bson:"minLength,omitempty" json:"minLength,omitempty"`
	MaxLength             interface{}              `bson:"maxLength,omitempty" json:"maxLength,omitempty"`
	Pattern               string                   `bson:"pattern,omitempty" json:"pattern,omitempty"`
	ValidationMessage     string                   `bson:"validationMessage,omitempty" json:"validationMessage,omitempty"`
}

type ActionSubmitConfig struct {
	Fields         []ActionFieldConfig    `bson:"fields,omitempty" json:"fields,omitempty"`
	IncludeFields  []string               `bson:"includeFields,omitempty" json:"includeFields,omitempty"`
	ExcludeFields  []string               `bson:"excludeFields,omitempty" json:"excludeFields,omitempty"`
	ConstantValues map[string]interface{} `bson:"constantValues,omitempty" json:"constantValues,omitempty"`
	WorkflowName   string                 `bson:"workflowName,omitempty" json:"workflowName,omitempty"`
	WorkflowSchema string                 `bson:"workflowSchema,omitempty" json:"workflowSchema,omitempty"`
	FunctionName   string                 `bson:"functionName,omitempty" json:"functionName,omitempty"`
}

type ActionConfig struct {
	ID                 string                   `bson:"id,omitempty" json:"id,omitempty"`
	Key                string                   `bson:"key" json:"key"`
	Name               string                   `bson:"name,omitempty" json:"name,omitempty"`
	Label              string                   `bson:"label,omitempty" json:"label,omitempty"`
	ButtonName         string                   `bson:"buttonName,omitempty" json:"buttonName,omitempty"`
	Kind               string                   `bson:"kind" json:"kind"` // edit | delete | update | link
	Icon               string                   `bson:"icon,omitempty" json:"icon,omitempty"`
	ClassName          string                   `bson:"className,omitempty" json:"className,omitempty"`
	ButtonClassName    string                   `bson:"buttonClassName,omitempty" json:"buttonClassName,omitempty"`
	Order              int                      `bson:"order,omitempty" json:"order,omitempty"`
	Enabled            *bool                    `bson:"enabled,omitempty" json:"enabled,omitempty"`
	IsModal            *bool                    `bson:"isModal,omitempty" json:"isModal,omitempty"`
	IsButton           *bool                    `bson:"isButton,omitempty" json:"isButton,omitempty"`
	ModalType          string                   `bson:"modalType,omitempty" json:"modalType,omitempty"` // form | confirmation
	ConfirmTitle       string                   `bson:"confirmTitle,omitempty" json:"confirmTitle,omitempty"`
	ConfirmText        string                   `bson:"confirmText,omitempty" json:"confirmText,omitempty"`
	Path               string                   `bson:"path,omitempty" json:"path,omitempty"`
	LinkTemplate       string                   `bson:"linkTemplate,omitempty" json:"linkTemplate,omitempty"`
	LinkType           string                   `bson:"linkType,omitempty" json:"linkType,omitempty"`
	DisabledCondition  string                   `bson:"disabledCondition,omitempty" json:"disabledCondition,omitempty"`
	HiddenCondition    string                   `bson:"hiddenCondition,omitempty" json:"hiddenCondition,omitempty"`
	RequiredCondition  string                   `bson:"requiredCondition,omitempty" json:"requiredCondition,omitempty"`
	DisabledConditions []string                 `bson:"disabledConditions,omitempty" json:"disabledConditions,omitempty"`
	RequiredConditions []string                 `bson:"requiredConditions,omitempty" json:"requiredConditions,omitempty"`
	FormFields         *[]ActionFormFieldConfig `bson:"formFields,omitempty" json:"formFields,omitempty"`
	FieldOverrides     []ActionFieldConfig      `bson:"fieldOverrides,omitempty" json:"fieldOverrides,omitempty"`
	ConstantValues     map[string]interface{}   `bson:"constantValues,omitempty" json:"constantValues,omitempty"`
	Submit             ActionSubmitConfig       `bson:"submit,omitempty" json:"submit,omitempty"`
}

type Field struct {
	Name               string              `bson:"name"`
	Type               string              `bson:"type"`
	Tag                string              `bson:"tag,omitempty"`
	ObjectSchemaName   string              `bson:"objectSchemaName,omitempty"`
	EnumList           []interface{}       `bson:"enumList,omitempty"`
	IsForceDelete      bool                `bson:"isForceDelete,omitempty"`
	Unique             bool                `bson:"unique,omitempty"`
	IsHashed           bool                `bson:"isHashed,omitempty"`
	IsLoginCredential  bool                `bson:"isLoginCredential,omitempty"`
	IsAuditIdentity    bool                `bson:"isAuditIdentity,omitempty" json:"isAuditIdentity,omitempty"`
	IsSearchable       bool                `bson:"isSearchable,omitempty"`
	Children           []Field             `bson:"children,omitempty"`
	Frontend           *Frontend           `bson:"frontend,omitempty"`
	PopulationSettings *PopulationSettings `bson:"populationSettings,omitempty"`
	Equation           string              `bson:"equation,omitempty"`
	AuthorizeRole      []string            `bson:"authorizeRole,omitempty" json:"authorizeRole,omitempty"`
	IsAuthorized       bool                `bson:"isAuthorized,omitempty" json:"isAuthorized,omitempty"`
	Order              int                 `bson:"order,omitempty" json:"order,omitempty"`
}

type PopulationSettings struct {
	FieldName           string   `bson:"fieldName"`
	PopulatedFields     []string `bson:"populatedFields"`
	DisplayFields       []string `bson:"displayFields"`
	InputSelectionField string   `bson:"inputSelectionField"`
	DisplayLabel        string   `bson:"displayLabel"`
}
type RouteSpec struct {
	IsAuthenticated bool     `bson:"isAuthenticated" `
	IsAuthorized    bool     `bson:"isAuthorized" `
	AuthorizeRole   []string `bson:"authorizeRole" `
	IsActive        bool     `bson:"isActivated" `
	Method          string   `bson:"method" `
}

type Routes struct {
	CreateDynamicModelItem                RouteSpec `bson:"createDynamicModelItem"`
	GetAllDynamicModelItems               RouteSpec `bson:"getAllDynamicModelItems"`
	CreateMultipleDynamicModelItem        RouteSpec `bson:"createMultipleDynamicModelItem"`
	GetAllDynamicModelItemsWithPagination RouteSpec `bson:"getAllDynamicModelItemsWithPagination"`
	GetPipeline                           RouteSpec `bson:"getPipeline"`
	TestPipeline                          RouteSpec `bson:"testPipeline"`
	HandleSearchDynamicModelItem          RouteSpec `bson:"handleSearchDynamicModelItem"`
	HandleFilterDynamicModelItem          RouteSpec `bson:"handleFilterDynamicModelItem"`
	DeleteDynamicModelItem                RouteSpec `bson:"deleteDynamicModelItem"`
	UpdateDynamicModelItem                RouteSpec `bson:"updateDynamicModelItem"`
	UpdateMultipleDynamicModelItem        RouteSpec `bson:"updateMultipleDynamicModelItem"`
	GetDynamicModelItem                   RouteSpec `bson:"getDynamicModelItem"`
	DeleteMultipleDynamicModelItem        RouteSpec `bson:"deleteMultipleDynamicModelItem"`
	ExportDynamicModelItems               RouteSpec `bson:"exportDynamicModelItems"`
	GetItemsForSelection                  RouteSpec `bson:"getItemsForSelection"`
}

type Redis struct {
	IsRedisCached        bool     `bson:"isRedisCached" `
	CacheTime            int      `bson:"cacheTime" `
	TriggeredRedisCaches []string `bson:"triggeredRedisCaches" `
}

type PipelineStage struct {
	Name            string   `bson:"name"`
	PipelineJSON    string   `bson:"pipelineJson"`
	OutputFields    []string `bson:"outputFields,omitempty" json:"outputFields,omitempty"`
	IsAuthenticated bool     `bson:"isAuthenticated"`
	IsAuthorized    bool     `bson:"isAuthorized"`
	AuthorizeRole   []string `bson:"authorizeRole"`
	IsActive        bool     `bson:"isActive"`
	IsRedisCached   bool     `bson:"isRedisCached"`
	CacheTime       int      `bson:"cacheTime"`
}
type DynamicFunction struct {
	Name            string   `bson:"name"`
	CodeJSON        string   `bson:"codeJson"`
	IsAuthenticated bool     `bson:"isAuthenticated"`
	IsAuthorized    bool     `bson:"isAuthorized"`
	AuthorizeRole   []string `bson:"authorizeRole"`
	IsActive        bool     `bson:"isActive"`
	IsRedisCached   bool     `bson:"isRedisCached"`
	CacheTime       int      `bson:"cacheTime"`
}
type DynamicApiModel struct {
	Name            string   `bson:"name"`
	Url             string   `bson:"url"`
	Method          string   `bson:"method"`
	Dependencies    []string `bson:"dependencies,omitempty"`
	IsAuthenticated bool     `bson:"isAuthenticated"`
	IsAuthorized    bool     `bson:"isAuthorized"`
	AuthorizeRole   []string `bson:"authorizeRole"`
	IsActive        bool     `bson:"isActive"`
	IsRedisCached   bool     `bson:"isRedisCached"`
	CacheTime       int      `bson:"cacheTime"`
}

// IndexField represents a single field in an index
type IndexField struct {
	FieldName string `bson:"fieldName"` // Name of the field to index
	Order     int    `bson:"order"`     // 1 for ascending, -1 for descending
}

// Index represents a MongoDB index configuration
type Index struct {
	Name       string       `bson:"name"`                 // Index name (e.g., "idx_createdAt")
	Fields     []IndexField `bson:"fields"`               // Fields to index (supports compound indexes)
	Unique     bool         `bson:"unique,omitempty"`     // Whether index should enforce uniqueness
	Sparse     bool         `bson:"sparse,omitempty"`     // Whether to index only documents with the field
	TTL        int          `bson:"ttl,omitempty"`        // TTL in seconds (0 = no TTL)
	Background bool         `bson:"background,omitempty"` // Build index in background
}

type ContainerModel struct {
	ID                  primitive.ObjectID `bson:"_id,omitempty"`
	SchemaName          string             `bson:"schemaName"`
	Fields              []Field            `bson:"fields"`
	Routes              Routes             `bson:"routes"`
	Redis               Redis              `bson:"redis"`
	Pipelines           []PipelineStage    `bson:"pipelines"`
	DynamicFunctions    []DynamicFunction  `bson:"dynamicFunctions"`
	DynamicApis         []DynamicApiModel  `bson:"dynamicApis"`
	Workflows           []DynamicWorkflow  `bson:"workflows,omitempty" json:"workflows,omitempty"`
	IsAuthContainer     bool               `bson:"isAuthContainer" json:"isAuthContainer"`
	IsRegisterActive    bool               `bson:"isRegisterActive" json:"isRegisterActive"`
	IsGoogleLoginActive bool               `bson:"isGoogleLoginActive" json:"isGoogleLoginActive"`
	PopulatedRoutes     []string           `bson:"populatedRoutes"`
	Indexes             []Index            `bson:"indexes,omitempty"` // MongoDB indexes for performance
	RowAccess           *RowAccessRule     `bson:"rowAccess,omitempty"`
	Frontend            *Frontend          `bson:"frontend,omitempty" json:"frontend,omitempty"`

	// Multi-tenancy fields
	TenantID       *primitive.ObjectID `bson:"tenantId,omitempty" json:"tenantId,omitempty"`             // Project-scoped containers
	ProjectID      *primitive.ObjectID `bson:"projectId,omitempty" json:"projectId,omitempty"`           // Project-scoped containers
	CollectionName string              `bson:"collectionName,omitempty" json:"collectionName,omitempty"` // Stores "schemaName_<projectIdHex>"
}

// Condition is the same shape you already use for filters / rowClass
type Condition struct {
	Field         string      `bson:"field"`
	Operator      string      `bson:"operator"` // "=", ">", "<", "in", etc.
	Value         interface{} `bson:"value"`    // can support "{{user.id}}" etc.
	ExtractFilter bool        `bson:"extractFilter,omitempty" json:"extractFilter,omitempty"`
	Roles         []string    `bson:"roles,omitempty" json:"roles,omitempty"`
}

type RowAccessRule struct {
	Conditions []Condition `bson:"conditions"` // all row rules, each with its own Roles
}

type ContainerTypes struct {
	ID         string            `json:"id"`
	SchemaName string            `json:"schemaName"`
	FieldTypes map[string]string `json:"fieldTypes"` // key: field name, value: declared type
}

type AuditLogsConfig struct {
	IsAuthorized  bool     `bson:"isAuthorized" json:"isAuthorized"`
	AuthorizeRole []string `bson:"authorizeRole" json:"authorizeRole"`
}

var RestrictedSchemaNames = []string{
	"containers",
} //this is needed so that user container is not created by mistake and get all the data.

// SortFieldsByOrder sorts fields by their order property (low to high)
func (c *ContainerModel) SortFieldsByOrder() {
	for i := 0; i < len(c.Fields); i++ {
		for j := i + 1; j < len(c.Fields); j++ {
			if c.Fields[i].Order > c.Fields[j].Order {
				c.Fields[i], c.Fields[j] = c.Fields[j], c.Fields[i]
			}
		}
	}
}
