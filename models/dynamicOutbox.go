package models

import "go.mongodb.org/mongo-driver/bson/primitive"

const (
	DynamicOutboxStatusPending    = "pending"
	DynamicOutboxStatusProcessing = "processing"
	DynamicOutboxStatusDone       = "done"
	DynamicOutboxStatusFailed     = "failed"
)

const (
	DynamicOutboxOperationCreate     = "create"
	DynamicOutboxOperationUpdate     = "update"
	DynamicOutboxOperationDelete     = "delete"
	DynamicOutboxOperationBulkCreate = "bulk_create"
	DynamicOutboxOperationBulkUpdate = "bulk_update"
	DynamicOutboxOperationBulkDelete = "bulk_delete"
)

type DynamicOutboxPayload struct {
	AuditLog          *AuditLog `bson:"auditLog,omitempty" json:"auditLog,omitempty"`
	InvalidateSchemas []string  `bson:"invalidateSchemas,omitempty" json:"invalidateSchemas,omitempty"`
	UserID            string    `bson:"userId,omitempty" json:"userId,omitempty"`
}

type DynamicOutboxEvent struct {
	ID            primitive.ObjectID   `bson:"_id,omitempty" json:"id,omitempty"`
	TenantID      string               `bson:"tenantId" json:"tenantId"`
	ProjectID     string               `bson:"projectId" json:"projectId"`
	SchemaName    string               `bson:"schemaName" json:"schemaName"`
	Operation     string               `bson:"operation" json:"operation"`
	Status        string               `bson:"status" json:"status"`
	Attempts      int                  `bson:"attempts" json:"attempts"`
	MaxAttempts   int                  `bson:"maxAttempts" json:"maxAttempts"`
	NextAttemptAt primitive.DateTime   `bson:"nextAttemptAt" json:"nextAttemptAt"`
	CreatedAt     primitive.DateTime   `bson:"createdAt" json:"createdAt"`
	UpdatedAt     primitive.DateTime   `bson:"updatedAt" json:"updatedAt"`
	ProcessedAt   primitive.DateTime   `bson:"processedAt,omitempty" json:"processedAt,omitempty"`
	ExpireAt      primitive.DateTime   `bson:"expireAt,omitempty" json:"expireAt,omitempty"`
	LastError     string               `bson:"lastError,omitempty" json:"lastError,omitempty"`
	Payload       DynamicOutboxPayload `bson:"payload" json:"payload"`
}
