package models

import "go.mongodb.org/mongo-driver/bson/primitive"

const (
	DynamicOutboxStatusPending    = "pending"
	DynamicOutboxStatusProcessing = "processing"
	DynamicOutboxStatusDone       = "done"
	DynamicOutboxStatusFailed     = "failed"
)

const (
	DynamicOutboxOperationCreate       = "create"
	DynamicOutboxOperationUpdate       = "update"
	DynamicOutboxOperationDelete       = "delete"
	DynamicOutboxOperationBulkCreate   = "bulk_create"
	DynamicOutboxOperationBulkUpdate   = "bulk_update"
	DynamicOutboxOperationBulkDelete   = "bulk_delete"
	DynamicOutboxOperationWorkflowStep = "workflow_step"
)

type DynamicOutboxPayload struct {
	AuditLog          *AuditLog              `bson:"auditLog,omitempty" json:"auditLog,omitempty"`
	InvalidateSchemas []string               `bson:"invalidateSchemas,omitempty" json:"invalidateSchemas,omitempty"`
	UserID            string                 `bson:"userId,omitempty" json:"userId,omitempty"`
	WorkflowName      string                 `bson:"workflowName,omitempty" json:"workflowName,omitempty"`
	StepID            string                 `bson:"stepId,omitempty" json:"stepId,omitempty"`
	StepName          string                 `bson:"stepName,omitempty" json:"stepName,omitempty"`
	StepType          string                 `bson:"stepType,omitempty" json:"stepType,omitempty"`
	StepTimeoutSec    int                    `bson:"stepTimeoutSec,omitempty" json:"stepTimeoutSec,omitempty"`
	WorkflowDepth     int                    `bson:"workflowDepth,omitempty" json:"workflowDepth,omitempty"`
	TargetSchema      string                 `bson:"targetSchema,omitempty" json:"targetSchema,omitempty"`
	Record            map[string]interface{} `bson:"record,omitempty" json:"record,omitempty"`
	OldRecord         map[string]interface{} `bson:"oldRecord,omitempty" json:"oldRecord,omitempty"`
	StepOutputs       map[string]interface{} `bson:"stepOutputs,omitempty" json:"stepOutputs,omitempty"`
	Variables         map[string]interface{} `bson:"variables,omitempty" json:"variables,omitempty"`
	Loop              map[string]interface{} `bson:"loop,omitempty" json:"loop,omitempty"`
	Config            map[string]interface{} `bson:"config,omitempty" json:"config,omitempty"`
	Steps             []DynamicWorkflowStep  `bson:"steps,omitempty" json:"steps,omitempty"`
	ElseSteps         []DynamicWorkflowStep  `bson:"elseSteps,omitempty" json:"elseSteps,omitempty"`
	Branches          []WorkflowBranch       `bson:"branches,omitempty" json:"branches,omitempty"`
	IdempotencyKey    string                 `bson:"idempotencyKey,omitempty" json:"idempotencyKey,omitempty"`
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
