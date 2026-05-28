package models

const (
	WorkflowTriggerBeforeCreate = "before_create"
	WorkflowTriggerAfterCreate  = "after_create"
	WorkflowTriggerBeforeUpdate = "before_update"
	WorkflowTriggerAfterUpdate  = "after_update"
	WorkflowTriggerBeforeDelete = "before_delete"
	WorkflowTriggerAfterDelete  = "after_delete"
)

const (
	WorkflowModeTransactional = "transactional"
	WorkflowModeOutbox        = "outbox"
	WorkflowModeHybrid        = "hybrid"
)

const (
	WorkflowStepTypeCreateRecord    = "create_record"
	WorkflowStepTypeUpdateRecord    = "update_record"
	WorkflowStepTypeDeleteRecord    = "delete_record"
	WorkflowStepTypeAuditLog        = "audit_log"
	WorkflowStepTypeInvalidateCache = "invalidate_cache"
	WorkflowStepTypeCallAPI         = "call_api"
	WorkflowStepTypeRunPipeline     = "run_pipeline"
	WorkflowStepTypeDynamicFunction = "dynamic_function"
	WorkflowStepTypeEmitOutboxEvent = "emit_outbox_event"
)

const (
	WorkflowConditionEqual       = "="
	WorkflowConditionNotEqual    = "!="
	WorkflowConditionGreaterThan = ">"
	WorkflowConditionLessThan    = "<"
	WorkflowConditionIn          = "in"
	WorkflowConditionExists      = "exists"
	WorkflowConditionChanged     = "changed"
	WorkflowConditionChangedTo   = "changed_to"
	WorkflowConditionChangedFrom = "changed_from"
)

type DynamicWorkflow struct {
	ID              string                `bson:"id,omitempty" json:"id,omitempty"`
	Name            string                `bson:"name" json:"name"`
	Trigger         string                `bson:"trigger" json:"trigger"`
	Mode            string                `bson:"mode" json:"mode"`
	IsActive        bool                  `bson:"isActive" json:"isActive"`
	IsAuthenticated bool                  `bson:"isAuthenticated" json:"isAuthenticated"`
	IsAuthorized    bool                  `bson:"isAuthorized" json:"isAuthorized"`
	AuthorizeRole   []string              `bson:"authorizeRole,omitempty" json:"authorizeRole,omitempty"`
	Description     string                `bson:"description,omitempty" json:"description,omitempty"`
	Conditions      []WorkflowCondition   `bson:"conditions,omitempty" json:"conditions,omitempty"`
	Steps           []DynamicWorkflowStep `bson:"steps" json:"steps"`
	StopOnError     bool                  `bson:"stopOnError" json:"stopOnError"`
	TimeoutSec      int                   `bson:"timeoutSec,omitempty" json:"timeoutSec,omitempty"`
}

type WorkflowCondition struct {
	Field    string      `bson:"field" json:"field"`
	Operator string      `bson:"operator" json:"operator"`
	Value    interface{} `bson:"value" json:"value"`
}

type DynamicWorkflowStep struct {
	ID              string                 `bson:"id,omitempty" json:"id,omitempty"`
	Name            string                 `bson:"name" json:"name"`
	Type            string                 `bson:"type" json:"type"`
	Order           int                    `bson:"order" json:"order"`
	IsActive        bool                   `bson:"isActive" json:"isActive"`
	ExecutionMode   string                 `bson:"executionMode,omitempty" json:"executionMode,omitempty"`
	TargetSchema    string                 `bson:"targetSchema,omitempty" json:"targetSchema,omitempty"`
	Config          map[string]interface{} `bson:"config,omitempty" json:"config,omitempty"`
	Conditions      []WorkflowCondition    `bson:"conditions,omitempty" json:"conditions,omitempty"`
	RetryCount      int                    `bson:"retryCount,omitempty" json:"retryCount,omitempty"`
	MaxAttempts     int                    `bson:"maxAttempts,omitempty" json:"maxAttempts,omitempty"`
	TimeoutSec      int                    `bson:"timeoutSec,omitempty" json:"timeoutSec,omitempty"`
	IdempotencyKey  string                 `bson:"idempotencyKey,omitempty" json:"idempotencyKey,omitempty"`
	ContinueOnError bool                   `bson:"continueOnError,omitempty" json:"continueOnError,omitempty"`
}
