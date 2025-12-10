package models

import "go.mongodb.org/mongo-driver/bson/primitive"

// Settings represents system-wide configuration stored in the database
// This allows dynamic configuration that can be managed from the frontend
type Settings struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Key       string             `bson:"key" json:"key"`                   // Unique identifier for the setting (e.g., "audit_logs")
	AuditLogs *AuditLogsConfig   `bson:"auditLogs,omitempty" json:"auditLogs,omitempty"` // Audit logs authorization config
	CreatedAt primitive.DateTime `bson:"createdAt,omitempty" json:"createdAt,omitempty"`
	UpdatedAt primitive.DateTime `bson:"updatedAt,omitempty" json:"updatedAt,omitempty"`
}
