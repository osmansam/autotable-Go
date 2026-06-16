package models

import "go.mongodb.org/mongo-driver/bson/primitive"

// BindingKind defines the type of data binding for a component
type BindingKind string

const (
	BindingKindSchema   BindingKind = "schema"
	BindingKindPipeline BindingKind = "pipeline"
	BindingKindWorkflow BindingKind = "workflow"
	BindingKindApi      BindingKind = "api"
	BindingKindFunction BindingKind = "function"
)

// DataBinding defines how a component binds to data sources
type DataBinding struct {
	Kind         BindingKind            `bson:"kind" json:"kind"` // "schema" | "pipeline" | "api" | "function"
	SchemaName   string                 `bson:"schemaName,omitempty" json:"schemaName,omitempty"`
	PipelineName string                 `bson:"pipelineName,omitempty" json:"pipelineName,omitempty"`
	WorkflowName string                 `bson:"workflowName,omitempty" json:"workflowName,omitempty"`
	ApiName      string                 `bson:"apiName,omitempty" json:"apiName,omitempty"`
	FunctionName string                 `bson:"functionName,omitempty" json:"functionName,omitempty"`
	Params       map[string]interface{} `bson:"params,omitempty" json:"params,omitempty"` // Optional extra info (e.g., default filters, params)
}

// GroupBy defines grouping configuration for table components
type GroupBy struct {
	GroupByObjectId string `bson:"groupByObjectId,omitempty" json:"groupByObjectId,omitempty"` // Schema name to group by (e.g., "can")
	GroupByField    string `bson:"groupByField,omitempty" json:"groupByField,omitempty"`       // Field name to display from grouped object (e.g., "name")
}

// TableLinkConfig defines link rendering for a table column.
type TableLinkConfig struct {
	Template   string `bson:"template,omitempty" json:"template,omitempty"`
	LabelField string `bson:"labelField,omitempty" json:"labelField,omitempty"`
	Type       string `bson:"type,omitempty" json:"type,omitempty"` // external | internal | email | phone | file
}

// TableColumnConfig defines display and cell behavior for one table column.
type TableColumnConfig struct {
	Field         string           `bson:"field" json:"field"`
	DisplayName   string           `bson:"displayName,omitempty" json:"displayName,omitempty"`
	CellClassName []RowClassConfig `bson:"cellClassName,omitempty" json:"cellClassName,omitempty"`
	Link          *TableLinkConfig `bson:"link,omitempty" json:"link,omitempty"`
}

// TableRowsConfig defines row-level table behavior.
type TableRowsConfig struct {
	ClassName []RowClassConfig `bson:"className,omitempty" json:"className,omitempty"`
}

// TableCacheConfig defines cache invalidation behavior for table mutations.
type TableCacheConfig struct {
	InvalidateKeys []string `bson:"invalidateKeys,omitempty" json:"invalidateKeys,omitempty"`
}

// TableComponentConfig keeps table-specific configuration on page table components.
type TableComponentConfig struct {
	Columns []TableColumnConfig `bson:"columns,omitempty" json:"columns,omitempty"`
	Rows    *TableRowsConfig    `bson:"rows,omitempty" json:"rows,omitempty"`
	Cache   *TableCacheConfig   `bson:"cache,omitempty" json:"cache,omitempty"`
}

// ComponentType defines the type of component
type ComponentType string

const (
	ComponentTypeTable    ComponentType = "table"
	ComponentTypeTabPanel ComponentType = "tabPanel" // tabPanel with embedded tabs
	ComponentTypeForm     ComponentType = "form"
	ComponentTypeText     ComponentType = "text"
	ComponentTypeCustom   ComponentType = "custom"

	// Chart Types - Specific chart components
	ComponentTypeBarChart           ComponentType = "barChart"           // Bar Chart
	ComponentTypeLineChart          ComponentType = "lineChart"          // Line Chart
	ComponentTypePieChart           ComponentType = "pieChart"           // Pie Chart
	ComponentTypeAreaChart          ComponentType = "areaChart"          // Area Chart
	ComponentTypeRadarChart         ComponentType = "radarChart"         // Radar Chart
	ComponentTypeHeatmapChart       ComponentType = "heatmapChart"       // Heat Map
	ComponentTypeScatterChart       ComponentType = "scatterChart"       // Scatter Plot
	ComponentTypeFunnelChart        ComponentType = "funnelChart"        // Funnel Chart
	ComponentTypeSankeyChart        ComponentType = "sankeyChart"        // Sankey Diagram
	ComponentTypeSunburstChart      ComponentType = "sunburstChart"      // Sunburst Chart
	ComponentTypeTreemapChart       ComponentType = "treemapChart"       // Tree Map
	ComponentTypeCalendarChart      ComponentType = "calendarChart"      // Calendar Chart
	ComponentTypeBumpChart          ComponentType = "bumpChart"          // Bump Chart
	ComponentTypeStreamChart        ComponentType = "streamChart"        // Stream Chart
	ComponentTypeWaffleChart        ComponentType = "waffleChart"        // Waffle Chart
	ComponentTypeCirclePackingChart ComponentType = "circlePackingChart" // Circle Packing
)

// TabPanelTab represents a tab inside a tabPanel component
type TabPanelTab struct {
	Title      string           `bson:"title" json:"title"`
	Icon       string           `bson:"icon,omitempty" json:"icon,omitempty"`
	Components []ComponentBlock `bson:"components" json:"components"`
}

// ComponentBlock represents a single component with its data binding and configuration
type ComponentBlock struct {
	ID            string                 `bson:"id" json:"id"`
	Type          ComponentType          `bson:"type" json:"type"`
	Title         string                 `bson:"title,omitempty" json:"title,omitempty"`
	Order         int                    `bson:"order,omitempty" json:"order,omitempty"` // order inside grid cell or section
	DataBinding   *DataBinding           `bson:"dataBinding,omitempty" json:"dataBinding,omitempty"`
	GroupBy       *GroupBy               `bson:"groupBy,omitempty" json:"groupBy,omitempty"`             // Grouping configuration for table components
	Table         *TableComponentConfig  `bson:"table,omitempty" json:"table,omitempty"`                 // Table-specific display, row, link, and cache config
	IsAuthorized  bool                   `bson:"isAuthorized,omitempty" json:"isAuthorized,omitempty"`   // Component-level auth (optional)
	AuthorizeRole []string               `bson:"authorizeRole,omitempty" json:"authorizeRole,omitempty"` // Component-level roles
	Props         map[string]interface{} `bson:"props,omitempty" json:"props,omitempty"`                 // Free-form config (columns, chart type, etc.)
	Tabs          []TabPanelTab          `bson:"tabs,omitempty" json:"tabs,omitempty"`                   // For tabPanel type components
}

// GridCell represents a cell in a grid layout
type GridCell struct {
	ID         string           `bson:"id" json:"id"`
	Row        int              `bson:"row" json:"row"`                             // 1-based row index
	Column     int              `bson:"column" json:"column"`                       // 1-based column index
	RowSpan    int              `bson:"rowSpan,omitempty" json:"rowSpan,omitempty"` // Number of rows to span
	ColSpan    int              `bson:"colSpan,omitempty" json:"colSpan,omitempty"` // Number of columns to span
	Components []ComponentBlock `bson:"components" json:"components"`               // Multiple components allowed in one cell (stacked)
}

// GridSection represents a grid layout container
type GridSection struct {
	Columns int        `bson:"columns" json:"columns"`             // e.g. 1, 2, 3
	Gap     int        `bson:"gap,omitempty" json:"gap,omitempty"` // px or your unit
	Cells   []GridCell `bson:"cells" json:"cells"`
}

// Tab represents a single tab in a tabs container
type Tab struct {
	ID       string    `bson:"id" json:"id"`
	Label    string    `bson:"label" json:"label"`
	Icon     string    `bson:"icon,omitempty" json:"icon,omitempty"`
	Order    int       `bson:"order" json:"order"`
	Sections []Section `bson:"sections" json:"sections"` // Each tab has its own layout sections
}

// TabsSection represents a tabs container
type TabsSection struct {
	Tabs []Tab `bson:"tabs" json:"tabs"`
}

// SectionType defines the type of section
type SectionType string

const (
	SectionTypeGrid      SectionType = "grid"
	SectionTypeComponent SectionType = "component"
	SectionTypeTabs      SectionType = "tabs"
)

// Section represents a layout section (grid, tabs, or single component)
// Supports both nested structure (Grid/Tabs/Component) and flat structure (direct grid properties)
type Section struct {
	ID    string      `bson:"id,omitempty" json:"id,omitempty"`     // for frontend references
	Type  SectionType `bson:"type,omitempty" json:"type,omitempty"` // "grid" | "tabs" | "component"
	Order int         `bson:"order,omitempty" json:"order,omitempty"`

	// Nested structure (preferred)
	Grid      *GridSection    `bson:"grid,omitempty" json:"grid,omitempty"`
	Tabs      *TabsSection    `bson:"tabs,omitempty" json:"tabs,omitempty"`
	Component *ComponentBlock `bson:"component,omitempty" json:"component,omitempty"`

	// Flat structure (for backward compatibility) - acts as implicit grid
	Columns int        `bson:"columns,omitempty" json:"columns,omitempty"`
	Gap     int        `bson:"gap,omitempty" json:"gap,omitempty"`
	Cells   []GridCell `bson:"cells,omitempty" json:"cells,omitempty"`
}

// PageModel represents a page with hierarchical structure, auth, and layout
type PageModel struct {
	ID              primitive.ObjectID  `bson:"_id,omitempty" json:"id,omitempty"`
	Name            string              `bson:"name" json:"name" validate:"required"`
	Icon            string              `bson:"icon,omitempty" json:"icon,omitempty"`
	Slug            string              `bson:"slug,omitempty" json:"slug,omitempty"` // e.g. "rewards", "rewards/members"
	ParentPageID    *primitive.ObjectID `bson:"parentPageId,omitempty" json:"parentPageId,omitempty"`
	Order           int                 `bson:"order,omitempty" json:"order,omitempty"`                 // order in sidebar under same parent
	IsGroupOnly     bool                `bson:"isGroupOnly,omitempty" json:"isGroupOnly,omitempty"`     // If true → used as parent group in sidebar, but no direct route
	IsAuthenticated bool                `bson:"isAuthenticated" json:"isAuthenticated"`                 // Page-level authentication
	IsAuthorized    bool                `bson:"isAuthorized" json:"isAuthorized"`                       // Page-level authorization
	AuthorizeRole   []string            `bson:"authorizeRole,omitempty" json:"authorizeRole,omitempty"` // Page-level roles
	Sections        []Section           `bson:"sections,omitempty" json:"sections,omitempty"`           // Layout: list of top-level sections
	SubPage         *PageModel          `bson:"subPage,omitempty" json:"subPage,omitempty"`             // Nested subpage (alternative to ParentPageID)
}
