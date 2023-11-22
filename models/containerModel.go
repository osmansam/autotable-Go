package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type Field struct {
	Name     string  `bson:"name"`
	Type     string  `bson:"type"`
	Tag      string  `bson:"tag,omitempty"`
	Children []Field `bson:"children,omitempty"` // Changed from NestedField to Field to support recursive nesting
}
type RouteSpec struct {
    IsAuthenticated bool   `bson:"isAuthenticated" default:"false"` 
    IsAuthorized    bool   `bson:"isAuthorized" default:"false"`
    AuthorizeRole   string `bson:"authorizeRole" default:""`
    IsRedisCached   bool   `bson:"isRedisCached" default:"false"`
}

type Routes struct {
    CreateDynamicModelItem RouteSpec `bson:"createDynamicModelItem"`
    GetAllDynamicModelItems RouteSpec `bson:"getAllDynamicModelItems"`
    GetPipeline RouteSpec `bson:"getPipeline"`
    HandleSearchDynamicModelItem RouteSpec `bson:"handleSearchDynamicModelItem"`
    DeleteDynamicModelItem RouteSpec `bson:"deleteDynamicModelItem"`
    UpdateDynamicModelItem RouteSpec `bson:"updateDynamicModelItem"`
    GetDynamicModelItem RouteSpec `bson:"getDynamicModelItem"`
}

type ContainerModel struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"`
	SchemaName string             `bson:"schemaName"`
	Fields     []Field            `bson:"fields"`
	Routes Routes `bson:"routes"`
}
