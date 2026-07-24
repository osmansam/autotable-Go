package models

import "go.mongodb.org/mongo-driver/bson/primitive"

// AuditLog represents a record in the audit_logs collection.
type AuditLog struct {
	ID              primitive.ObjectID   `bson:"_id,omitempty"`
	EventID         primitive.ObjectID   `bson:"eventId,omitempty"`
	TenantID        string               `bson:"tenantId,omitempty"`  // Project tenant ID
	ProjectID       string               `bson:"projectId,omitempty"` // Project ID
	Timestamp       primitive.DateTime   `bson:"timestamp"`
	UserID          primitive.ObjectID   `bson:"userId,omitempty"`
	UserEmail       string               `bson:"userEmail,omitempty"`
	UserDisplayName string               `bson:"userDisplayName,omitempty" json:"userDisplayName,omitempty"`
	Roles           []string             `bson:"roles,omitempty"`
	SchemaName      string               `bson:"schemaName,omitempty"`
	DocumentIDs     []primitive.ObjectID `bson:"documentIds,omitempty"` // one or many
	Action          string               `bson:"action"`                // "create", "update", "delete", "bulk_create", "bulk_update", "bulk_delete", "login", "logout", "custom"
	Description     string               `bson:"description,omitempty"`
	Before          interface{}          `bson:"before,omitempty"` // snapshot or diff before change
	After           interface{}          `bson:"after,omitempty"`  // snapshot or diff after change
	IP              string               `bson:"ip,omitempty"`
	UserAgent       string               `bson:"userAgent,omitempty"`
}

// AuditUser represents the user context for audit logging.
// This is used in the helper functions to pass user information.
// This is separate from the tenant User model and represents project-level user context.
type AuditUser struct {
	ID          primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Email       string             `json:"email,omitempty" bson:"email,omitempty"`
	DisplayName string             `json:"displayName,omitempty" bson:"displayName,omitempty"`
	Roles       []string           `json:"roles,omitempty" bson:"roles,omitempty"`
}
