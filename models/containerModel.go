package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type Field struct {
	Name     string  `bson:"name"`
	Type     string  `bson:"type"`
	Tag      string  `bson:"tag,omitempty"`
	Children []Field `bson:"children,omitempty"` // Changed from NestedField to Field to support recursive nesting
}

type ContainerModel struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"`
	SchemaName string             `bson:"schemaName"`
	Fields     []Field            `bson:"fields"`
}
