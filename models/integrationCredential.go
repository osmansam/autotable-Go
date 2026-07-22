package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	IntegrationPermissionKindDynamicRoute = "dynamicRoute"
	IntegrationPermissionKindWorkflow     = "workflow"
	IntegrationPermissionKindAPI          = "api"
	IntegrationPermissionKindPipeline     = "pipeline"
)

type IntegrationPermission struct {
	Kind       string `bson:"kind" json:"kind"`
	SchemaName string `bson:"schemaName" json:"schemaName"`
	Route      string `bson:"route,omitempty" json:"route,omitempty"`
	Name       string `bson:"name,omitempty" json:"name,omitempty"`
	Method     string `bson:"method" json:"method"`
}

type IntegrationCredential struct {
	ID          primitive.ObjectID      `bson:"_id,omitempty" json:"id"`
	TenantID    primitive.ObjectID      `bson:"tenantId" json:"tenantId"`
	ProjectID   primitive.ObjectID      `bson:"projectId" json:"projectId"`
	Name        string                  `bson:"name" json:"name"`
	TokenHash   string                  `bson:"tokenHash" json:"-"`
	Permissions []IntegrationPermission `bson:"permissions" json:"permissions"`
	ExpiresAt   time.Time               `bson:"expiresAt" json:"expiresAt"`
	RevokedAt   *time.Time              `bson:"revokedAt,omitempty" json:"revokedAt,omitempty"`
	CreatedBy   primitive.ObjectID      `bson:"createdBy,omitempty" json:"createdBy,omitempty"`
	CreatedAt   time.Time               `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time               `bson:"updatedAt" json:"updatedAt"`
	LastUsedAt  *time.Time              `bson:"lastUsedAt,omitempty" json:"lastUsedAt,omitempty"`
}
