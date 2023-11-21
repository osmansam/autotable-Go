package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type Field struct {
	Name     string  `bson:"name"`
	Type     string  `bson:"type"`
	Tag      string  `bson:"tag,omitempty"`
	Children []Field `bson:"children,omitempty"` // Changed from NestedField to Field to support recursive nesting
}
type AuthenticatedField struct {
    IsAuthenticated bool   `bson:"isAuthenticated" default:"false"` 
    IsAuthorized    bool   `bson:"isAuthorized" default:"false"`
    AuthorizeRole   string `bson:"authorizeRole" default:""`
}

type Routes struct {
    CreateDynamicModelItem AuthenticatedField `bson:"createDynamicModelItem"`
    GetAllDynamicModelItems AuthenticatedField `bson:"getAllDynamicModelItems"`
    GetPipeline AuthenticatedField `bson:"getPipeline"`
    HandleSearchDynamicModelItem AuthenticatedField `bson:"handleSearchDynamicModelItem"`
    DeleteDynamicModelItem AuthenticatedField `bson:"deleteDynamicModelItem"`
    UpdateDynamicModelItem AuthenticatedField `bson:"updateDynamicModelItem"`
    GetDynamicModelItem AuthenticatedField `bson:"getDynamicModelItem"`
}

type ContainerModel struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"`
	SchemaName string             `bson:"schemaName"`
	Fields     []Field            `bson:"fields"`
	Routes Routes `bson:"routes"`
}
