package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type Field struct {
	Name     string  `bson:"name"`
	Type     string  `bson:"type"`
	Tag      string  `bson:"tag,omitempty"`
	IsForceDelete      bool  `bson:"isForceDelete,omitempty"`
    Unique bool `bson:"unique,omitempty"`
	Children []Field `bson:"children,omitempty"` 
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
    GetAllDynamicModelItemsWithPagination RouteSpec `bson:"getAllDynamicModelItemsWithPagination"`
    GetPipeline RouteSpec `bson:"getPipeline"`
    TestPipeline RouteSpec `bson:"testPipeline"`
    HandleSearchDynamicModelItem RouteSpec `bson:"handleSearchDynamicModelItem"`
    HandleFilterDynamicModelItem RouteSpec `bson:"handleFilterDynamicModelItem"`
    DeleteDynamicModelItem RouteSpec `bson:"deleteDynamicModelItem"`
    UpdateDynamicModelItem RouteSpec `bson:"updateDynamicModelItem"`
    GetDynamicModelItem RouteSpec `bson:"getDynamicModelItem"`
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
    Dependencies    []string `bson:"dependencies"`
    IsAuthenticated bool   `bson:"isAuthenticated"`
    IsAuthorized    bool   `bson:"isAuthorized"`
    AuthorizeRole   string `bson:"authorizeRole"`
    IsActive        bool   `bson:"isActive"`
    IsRedisCached   bool   `bson:"isRedisCached"`
    CacheTime       int    `bson:"cacheTime"`
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
}


// TODO: Authorize Role will be an string array to support multiple roles