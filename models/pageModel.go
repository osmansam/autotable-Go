package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type SchemaInfo struct {
	SchemaName   string `bson:"schemaName" json:"schemaName"`
	IsPaginated  bool   `bson:"isPaginated" json:"isPaginated"`
	Icon         string `bson:"icon,omitempty" json:"icon,omitempty"`
}

type PageModel struct {
	ID      primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Name    string             `bson:"name" json:"name" validate:"required"`
	Icon    string             `bson:"icon,omitempty" json:"icon,omitempty"`
	Schemas []SchemaInfo       `bson:"schemas" json:"schemas"`
	Page    *PageModel         `bson:"page,omitempty" json:"page,omitempty"`
}
