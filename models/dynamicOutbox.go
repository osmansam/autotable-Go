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
	DynamicOutboxOperationCronWorkflow = "cron_workflow"
)

type DynamicOutboxPayload struct {
	CronWorkflow            *DynamicWorkflow       `bson:"cronWorkflow,omitempty" json:"cronWorkflow,omitempty"`
	CronWorkflows           []DynamicWorkflow      `bson:"cronWorkflows,omitempty" json:"cronWorkflows,omitempty"`
	CronScheduledAt         primitive.DateTime     `bson:"cronScheduledAt,omitempty" json:"cronScheduledAt,omitempty"`
	WorkflowSnapshotVersion int                    `bson:"workflowSnapshotVersion,omitempty" json:"workflowSnapshotVersion,omitempty"`
	AuditLog                *AuditLog              `bson:"auditLog,omitempty" json:"auditLog,omitempty"`
	InvalidateSchemas       []string               `bson:"invalidateSchemas,omitempty" json:"invalidateSchemas,omitempty"`
	IdentityKind            string                 `bson:"identityKind,omitempty" json:"identityKind,omitempty"`
	UserID                  string                 `bson:"userId,omitempty" json:"userId,omitempty"`
	UserRole                string                 `bson:"userRole,omitempty" json:"userRole,omitempty"`
	UserRoles               []string               `bson:"userRoles,omitempty" json:"userRoles,omitempty"`
	AuditUser               *AuditUser             `bson:"auditUser,omitempty" json:"auditUser,omitempty"`
	WorkflowName            string                 `bson:"workflowName,omitempty" json:"workflowName,omitempty"`
	WorkflowTrigger         string                 `bson:"workflowTrigger,omitempty" json:"workflowTrigger,omitempty"`
	WorkflowVersion         int                    `bson:"workflowVersion,omitempty" json:"workflowVersion,omitempty"`
	StopOnError             bool                   `bson:"stopOnError,omitempty" json:"stopOnError,omitempty"`
	StepID                  string                 `bson:"stepId,omitempty" json:"stepId,omitempty"`
	StepName                string                 `bson:"stepName,omitempty" json:"stepName,omitempty"`
	StepType                string                 `bson:"stepType,omitempty" json:"stepType,omitempty"`
	StepTimeoutSec          int                    `bson:"stepTimeoutSec,omitempty" json:"stepTimeoutSec,omitempty"`
	WorkflowDepth           int                    `bson:"workflowDepth,omitempty" json:"workflowDepth,omitempty"`
	TargetSchema            string                 `bson:"targetSchema,omitempty" json:"targetSchema,omitempty"`
	Record                  map[string]interface{} `bson:"record,omitempty" json:"record,omitempty"`
	Query                   map[string]interface{} `bson:"query,omitempty" json:"query,omitempty"`
	OldRecord               map[string]interface{} `bson:"oldRecord,omitempty" json:"oldRecord,omitempty"`
	StepOutputs             map[string]interface{} `bson:"stepOutputs,omitempty" json:"stepOutputs,omitempty"`
	Variables               map[string]interface{} `bson:"variables,omitempty" json:"variables,omitempty"`
	Loop                    map[string]interface{} `bson:"loop,omitempty" json:"loop,omitempty"`
	CurrentUser             map[string]interface{} `bson:"currentUser,omitempty" json:"currentUser,omitempty"`
	Conditions              []WorkflowCondition    `bson:"conditions,omitempty" json:"conditions,omitempty"`
	Config                  map[string]interface{} `bson:"config,omitempty" json:"config,omitempty"`
	Steps                   []DynamicWorkflowStep  `bson:"steps,omitempty" json:"steps,omitempty"`
	ElseSteps               []DynamicWorkflowStep  `bson:"elseSteps,omitempty" json:"elseSteps,omitempty"`
	Branches                []WorkflowBranch       `bson:"branches,omitempty" json:"branches,omitempty"`
	IdempotencyKey          string                 `bson:"idempotencyKey,omitempty" json:"idempotencyKey,omitempty"`
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
