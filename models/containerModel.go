package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type Field struct {
    Name              string  `bson:"name"`
    Type              string  `bson:"type"`
    Tag               string  `bson:"tag,omitempty"`
    ObjectSchemaName  string  `bson:"objectSchemaName,omitempty"`
    IsForceDelete     bool    `bson:"isForceDelete,omitempty"`
    Unique            bool    `bson:"unique,omitempty"`
    IsHashed          bool    `bson:"isHashed,omitempty"`          
    IsLoginCredential bool    `bson:"isLoginCredential,omitempty"` 
    Children          []Field `bson:"children,omitempty"`
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

type Population struct {
    FieldName          string   `bson:"fieldName"`
    PopulatedVariables []string `bson:"populatedVariables"`
}
type ContainerModel struct {
    ID             primitive.ObjectID `bson:"_id,omitempty"`
    SchemaName     string             `bson:"schemaName"`
    Fields         []Field            `bson:"fields"`
    Routes         Routes             `bson:"routes"`
    Redis          Redis              `bson:"redis"`
    Pipelines      []PipelineStage `bson:"pipelines"` 
    DynamicFunctions      []DynamicFunction `bson:"dynamicFunctions"` 
    DynamicApis      []DynamicApiModel `bson:"dynamicApis"`
    IsAuthContainer  bool               `bson:"isAuthContainer,omitempty"`
    PopulationArray  []Population       `bson:"populationArray,omitempty"`
}

var RestrictedSchemaNames = []string{
    "containers",
}//this is needed so that user container is not created by mistake and get all the data.


// TODO: Authorize Role will be an string array to support multiple roles