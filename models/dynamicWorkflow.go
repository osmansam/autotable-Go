package models

const (
	WorkflowTriggerBeforeCreate = "before_create"
	WorkflowTriggerAfterCreate  = "after_create"
	WorkflowTriggerBeforeUpdate = "before_update"
	WorkflowTriggerAfterUpdate  = "after_update"
	WorkflowTriggerBeforeDelete = "before_delete"
	WorkflowTriggerAfterDelete  = "after_delete"
	WorkflowTriggerManual       = "manual"
	WorkflowTriggerCron         = "cron"
)

const (
	WorkflowModeTransactional = "transactional"
	WorkflowModeOutbox        = "outbox"
	WorkflowModeHybrid        = "hybrid"
)

const (
	WorkflowStepTypeCreateRecord       = "create_record"
	WorkflowStepTypeUpdateRecord       = "update_record"
	WorkflowStepTypeUnsetRecord        = "unset_record"
	WorkflowStepTypeDeleteRecord       = "delete_record"
	WorkflowStepTypeAuditLog           = "audit_log"
	WorkflowStepTypeInvalidateCache    = "invalidate_cache"
	WorkflowStepTypeCallAPI            = "call_api"
	WorkflowStepTypeRunPipeline        = "run_pipeline"
	WorkflowStepTypeAggregate          = "aggregate"
	WorkflowStepTypeDistinct           = "distinct"
	WorkflowStepTypeDynamicFunction    = "dynamic_function"
	WorkflowStepTypeEmitOutboxEvent    = "emit_outbox_event"
	WorkflowStepTypeCreateNotification = "create_notification"
	WorkflowStepTypeGetRecord          = "get_record"
	WorkflowStepTypeFindRecords        = "find_records"
	WorkflowStepTypeIf                 = "if"
	WorkflowStepTypeForEach            = "for_each"
	WorkflowStepTypeSetVariable        = "set_variable"
	WorkflowStepTypeExecuteWorkflow    = "execute_workflow"
	WorkflowStepTypeExecuteDynamicAPI  = "execute_dynamic_api"
	WorkflowStepTypeFail               = "fail"
	WorkflowStepTypeSetRecord          = "set_record"
	WorkflowStepTypeTransform          = "transform"
	WorkflowStepTypeAppendArray        = "append_array"
	WorkflowStepTypeRemoveArray        = "remove_array"
	WorkflowStepTypeAddToSet           = "add_to_set"
	WorkflowStepTypePush               = "push"
	WorkflowStepTypePull               = "pull"
	WorkflowStepTypePullAll            = "pull_all"
	WorkflowStepTypeSetArray           = "set_array"
	WorkflowStepTypeCountRecords       = "count_records"
	WorkflowStepTypeEquation           = "equation"
	WorkflowStepTypeReturn             = "return"
)

const (
	WorkflowConditionEqual        = "="
	WorkflowConditionNotEqual     = "!="
	WorkflowConditionGreaterThan  = ">"
	WorkflowConditionGreaterEqual = ">="
	WorkflowConditionLessThan     = "<"
	WorkflowConditionLessEqual    = "<="
	WorkflowConditionIn           = "in"
	WorkflowConditionNotIn        = "not_in"
	WorkflowConditionContains     = "contains"
	WorkflowConditionNotContains  = "not_contains"
	WorkflowConditionStartsWith   = "starts_with"
	WorkflowConditionEndsWith     = "ends_with"
	WorkflowConditionIsEmpty      = "is_empty"
	WorkflowConditionIsNotEmpty   = "is_not_empty"
	WorkflowConditionBetween      = "between"
	WorkflowConditionExists       = "exists"
	WorkflowConditionChanged      = "changed"
	WorkflowConditionChangedTo    = "changed_to"
	WorkflowConditionChangedFrom  = "changed_from"
	WorkflowConditionAnd          = "and"
	WorkflowConditionOr           = "or"
)

type DynamicWorkflow struct {
	ID               string                 `bson:"id,omitempty" json:"id,omitempty"`
	Name             string                 `bson:"name" json:"name"`
	Version          int                    `bson:"version,omitempty" json:"version,omitempty"`
	Trigger          string                 `bson:"trigger" json:"trigger"`
	Schedule         string                 `bson:"schedule,omitempty" json:"schedule,omitempty"`
	Timezone         string                 `bson:"timezone,omitempty" json:"timezone,omitempty"`
	Mode             string                 `bson:"mode" json:"mode"`
	IsActive         bool                   `bson:"isActive" json:"isActive"`
	IsAuthenticated  bool                   `bson:"isAuthenticated" json:"isAuthenticated"`
	IsAuthorized     bool                   `bson:"isAuthorized" json:"isAuthorized"`
	AuthorizeRole    []string               `bson:"authorizeRole,omitempty" json:"authorizeRole,omitempty"`
	Description      string                 `bson:"description,omitempty" json:"description,omitempty"`
	Payload          map[string]interface{} `bson:"payload,omitempty" json:"payload,omitempty"`
	Conditions       []WorkflowCondition    `bson:"conditions,omitempty" json:"conditions,omitempty"`
	Steps            []DynamicWorkflowStep  `bson:"steps" json:"steps"`
	StopOnError      bool                   `bson:"stopOnError" json:"stopOnError"`
	TimeoutSec       int                    `bson:"timeoutSec,omitempty" json:"timeoutSec,omitempty"`
	ReturnStep       string                 `bson:"returnStep,omitempty" json:"returnStep,omitempty"`
	RunInTransaction bool                   `bson:"runInTransaction,omitempty" json:"runInTransaction,omitempty"`
}

type WorkflowCondition struct {
	Field      string              `bson:"field,omitempty" json:"field,omitempty"`
	Operator   string              `bson:"operator" json:"operator"`
	Value      interface{}         `bson:"value,omitempty" json:"value,omitempty"`
	Conditions []WorkflowCondition `bson:"conditions,omitempty" json:"conditions,omitempty"`
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
	Steps           []DynamicWorkflowStep  `bson:"steps,omitempty" json:"steps,omitempty"`
	ElseSteps       []DynamicWorkflowStep  `bson:"elseSteps,omitempty" json:"elseSteps,omitempty"`
	Branches        []WorkflowBranch       `bson:"branches,omitempty" json:"branches,omitempty"`
}

type WorkflowBranch struct {
	Name       string                `bson:"name,omitempty" json:"name,omitempty"`
	Conditions []WorkflowCondition   `bson:"conditions,omitempty" json:"conditions,omitempty"`
	Steps      []DynamicWorkflowStep `bson:"steps,omitempty" json:"steps,omitempty"`
}
