package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type RowClassConfig struct {
    Condition string `bson:"condition"`
    ClassName string `bson:"className"`
}

type Frontend struct {
    DisplayName string `bson:"displayName,omitempty"`
    RowClassName []RowClassConfig `bson:"rowClassName,omitempty"`
    RowKeyClassName []RowClassConfig `bson:"rowKeyClassName,omitempty"`
    InvalidateKeys []string `json:"invalidateKeys" bson:"invalidateKeys"`
}

type Field struct {
    Name              string  `bson:"name"`
    Type              string  `bson:"type"`
    Tag               string  `bson:"tag,omitempty"`
    ObjectSchemaName  string  `bson:"objectSchemaName,omitempty"`
    EnumList          []interface{} `bson:"enumList,omitempty"`
    IsForceDelete     bool    `bson:"isForceDelete,omitempty"`
    Unique            bool    `bson:"unique,omitempty"`
    IsHashed          bool    `bson:"isHashed,omitempty"`          
    IsLoginCredential bool    `bson:"isLoginCredential,omitempty"` 
    IsSearchable      bool    `bson:"isSearchable,omitempty"`  
    Children          []Field `bson:"children,omitempty"`
    Frontend          *Frontend `bson:"frontend,omitempty"`
    PopulationSettings *PopulationSettings `bson:"populationSettings,omitempty"`
    Equation          string              `bson:"equation,omitempty"`
}

type PopulationSettings struct {
    FieldName           string   `bson:"fieldName"`
    PopulatedFields     []string `bson:"populatedFields"`
    DisplayFields       []string `bson:"displayFields"`
    InputSelectionField string   `bson:"inputSelectionField"`
    DisplayLabel        string   `bson:"displayLabel"`
}
type RouteSpec struct {
    IsAuthenticated bool   `bson:"isAuthenticated" ` 
    IsAuthorized    bool   `bson:"isAuthorized" `
    AuthorizeRole   string `bson:"authorizeRole" `
    IsActive     bool   `bson:"isActivated" `
    Method       string `bson:"method" `
}

type Routes struct {
    CreateDynamicModelItem RouteSpec `bson:"createDynamicModelItem"`
    GetAllDynamicModelItems RouteSpec `bson:"getAllDynamicModelItems"`
    CreateMultipleDynamicModelItem RouteSpec `bson:"createMultipleDynamicModelItem"`
    GetAllDynamicModelItemsWithPagination RouteSpec `bson:"getAllDynamicModelItemsWithPagination"`
    GetPipeline RouteSpec `bson:"getPipeline"`
    TestPipeline RouteSpec `bson:"testPipeline"`
    HandleSearchDynamicModelItem RouteSpec `bson:"handleSearchDynamicModelItem"`
    HandleFilterDynamicModelItem RouteSpec `bson:"handleFilterDynamicModelItem"`
    DeleteDynamicModelItem RouteSpec `bson:"deleteDynamicModelItem"`
    UpdateDynamicModelItem RouteSpec `bson:"updateDynamicModelItem"`
    UpdateMultipleDynamicModelItem RouteSpec `bson:"updateMultipleDynamicModelItem"`
    GetDynamicModelItem RouteSpec `bson:"getDynamicModelItem"`
    DeleteMultipleDynamicModelItem RouteSpec `bson:"deleteMultipleDynamicModelItem"`
}

type Redis struct {
    IsRedisCached bool `bson:"isRedisCached" `
    CacheTime int `bson:"cacheTime" `
    TriggeredRedisCaches []string `bson:"triggeredRedisCaches" `
}

type PipelineStage struct {
    Name            string `bson:"name"`
    PipelineJSON    string `bson:"pipelineJson"` 
    IsAuthenticated bool   `bson:"isAuthenticated"`
    IsAuthorized    bool   `bson:"isAuthorized"`
    AuthorizeRole   string `bson:"authorizeRole"`
    IsActive        bool   `bson:"isActive"`
    IsRedisCached   bool   `bson:"isRedisCached"`
    CacheTime       int    `bson:"cacheTime"`

}
type DynamicFunction struct {
    Name            string `bson:"name"`
    CodeJSON    string `bson:"codeJson"` 
    IsAuthenticated bool   `bson:"isAuthenticated"`
    IsAuthorized    bool   `bson:"isAuthorized"`
    AuthorizeRole   string `bson:"authorizeRole"`
    IsActive        bool   `bson:"isActive"`
    IsRedisCached   bool   `bson:"isRedisCached"`
    CacheTime       int    `bson:"cacheTime"`
}
type DynamicApiModel struct {
    Name            string `bson:"name"`    
    Url             string `bson:"url"`
    Method          string `bson:"method"`
    Dependencies    []string `bson:"dependencies,omitempty"`
    IsAuthenticated bool   `bson:"isAuthenticated"`
    IsAuthorized    bool   `bson:"isAuthorized"`
    AuthorizeRole   string `bson:"authorizeRole"`
    IsActive        bool   `bson:"isActive"`
    IsRedisCached   bool   `bson:"isRedisCached"`
    CacheTime       int    `bson:"cacheTime"`
}

// IndexField represents a single field in an index
type IndexField struct {
    FieldName string `bson:"fieldName"` // Name of the field to index
    Order     int    `bson:"order"`     // 1 for ascending, -1 for descending
}

// Index represents a MongoDB index configuration
type Index struct {
    Name       string       `bson:"name"`              // Index name (e.g., "idx_createdAt")
    Fields     []IndexField `bson:"fields"`            // Fields to index (supports compound indexes)
    Unique     bool         `bson:"unique,omitempty"`  // Whether index should enforce uniqueness
    Sparse     bool         `bson:"sparse,omitempty"`  // Whether to index only documents with the field
    TTL        int          `bson:"ttl,omitempty"`     // TTL in seconds (0 = no TTL)
    Background bool         `bson:"background,omitempty"` // Build index in background
}

type ContainerModel struct {
    ID               primitive.ObjectID `bson:"_id,omitempty"`
    SchemaName       string             `bson:"schemaName"`
    Fields           []Field            `bson:"fields"`
    Routes           Routes             `bson:"routes"`
    Redis            Redis              `bson:"redis"`
    Pipelines        []PipelineStage    `bson:"pipelines"` 
    DynamicFunctions []DynamicFunction  `bson:"dynamicFunctions"` 
    DynamicApis      []DynamicApiModel  `bson:"dynamicApis"`
    IsAuthContainer  bool               `bson:"isAuthContainer,omitempty"`
    PopulatedRoutes  []string           `bson:"populatedRoutes"`
    Indexes          []Index            `bson:"indexes,omitempty"` // MongoDB indexes for performance

}

type ContainerTypes struct {
    ID         string            `json:"id"`
    SchemaName string            `json:"schemaName"`
    FieldTypes map[string]string `json:"fieldTypes"` // key: field name, value: declared type
}


var RestrictedSchemaNames = []string{
    "containers",
}//this is needed so that user container is not created by mistake and get all the data.


// TODO: Authorize Role will be an string array to support multiple roles