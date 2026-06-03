package services

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/osmansam/autotableGo/models"
)

const maxWorkflowTimeoutSec = 300

var workflowStepTypes = map[string]bool{
	models.WorkflowStepTypeCreateRecord:      true,
	models.WorkflowStepTypeUpdateRecord:      true,
	models.WorkflowStepTypeUnsetRecord:       true,
	models.WorkflowStepTypeDeleteRecord:      true,
	models.WorkflowStepTypeAuditLog:          true,
	models.WorkflowStepTypeInvalidateCache:   true,
	models.WorkflowStepTypeCallAPI:           true,
	models.WorkflowStepTypeRunPipeline:       true,
	models.WorkflowStepTypeAggregate:         true,
	models.WorkflowStepTypeDistinct:          true,
	models.WorkflowStepTypeDynamicFunction:   true,
	models.WorkflowStepTypeEmitOutboxEvent:   true,
	models.WorkflowStepTypeGetRecord:         true,
	models.WorkflowStepTypeFindRecords:       true,
	models.WorkflowStepTypeIf:                true,
	models.WorkflowStepTypeForEach:           true,
	models.WorkflowStepTypeSetVariable:       true,
	models.WorkflowStepTypeExecuteWorkflow:   true,
	models.WorkflowStepTypeExecuteDynamicAPI: true,
	models.WorkflowStepTypeFail:              true,
	models.WorkflowStepTypeSetRecord:         true,
	models.WorkflowStepTypeTransform:         true,
	models.WorkflowStepTypeAppendArray:       true,
	models.WorkflowStepTypeRemoveArray:       true,
	models.WorkflowStepTypeAddToSet:          true,
	models.WorkflowStepTypePush:              true,
	models.WorkflowStepTypePull:              true,
	models.WorkflowStepTypePullAll:           true,
	models.WorkflowStepTypeSetArray:          true,
	models.WorkflowStepTypeCountRecords:      true,
	models.WorkflowStepTypeEquation:          true,
	models.WorkflowStepTypeReturn:            true,
}

func ValidateWorkflows(workflows []models.DynamicWorkflow) error {
	names := map[string]bool{}
	for _, workflow := range workflows {
		if names[workflow.Name] {
			return fmt.Errorf("duplicate workflow name: %s", workflow.Name)
		}
		names[workflow.Name] = true
		if err := ValidateWorkflow(workflow); err != nil {
			return err
		}
	}
	return nil
}

func ValidateWorkflow(workflow models.DynamicWorkflow) error {
	if strings.TrimSpace(workflow.Name) == "" {
		return fmt.Errorf("workflow name is required")
	}
	if !workflowTriggerValid(workflow.Trigger) {
		return fmt.Errorf("workflow %s has invalid trigger: %s", workflow.Name, workflow.Trigger)
	}
	if !workflowModeValid(workflow.Mode) {
		return fmt.Errorf("workflow %s has invalid mode: %s", workflow.Name, workflow.Mode)
	}
	if workflow.TimeoutSec < 0 || workflow.TimeoutSec > maxWorkflowTimeoutSec {
		return fmt.Errorf("workflow %s timeoutSec must be between 0 and %d", workflow.Name, maxWorkflowTimeoutSec)
	}
	if countWorkflowSteps(workflow.Steps) > maxWorkflowSteps {
		return fmt.Errorf("workflow %s has too many steps: max %d", workflow.Name, maxWorkflowSteps)
	}
	if workflow.ReturnStep != "" && !workflowHasNamedStep(workflow.Steps, workflow.ReturnStep) {
		return fmt.Errorf("workflow %s returnStep does not match a named step: %s", workflow.Name, workflow.ReturnStep)
	}
	if err := validateWorkflowConditions(workflow.Trigger, workflow.Conditions); err != nil {
		return fmt.Errorf("workflow %s: %w", workflow.Name, err)
	}
	return validateWorkflowSteps(workflow, workflow.Steps)
}

func validateWorkflowSteps(workflow models.DynamicWorkflow, steps []models.DynamicWorkflowStep) error {
	orderedSteps := append([]models.DynamicWorkflowStep(nil), steps...)
	sort.SliceStable(orderedSteps, func(i, j int) bool {
		return orderedSteps[i].Order < orderedSteps[j].Order
	})
	for index, step := range orderedSteps {
		if !workflowStepTypes[step.Type] {
			return fmt.Errorf("workflow %s step %s has unsupported type: %s", workflow.Name, step.Name, step.Type)
		}
		if step.ExecutionMode != "" && step.ExecutionMode != models.WorkflowModeTransactional && step.ExecutionMode != models.WorkflowModeOutbox {
			return fmt.Errorf("workflow %s step %s has invalid executionMode: %s", workflow.Name, step.Name, step.ExecutionMode)
		}
		if workflow.Mode == models.WorkflowModeHybrid && step.ExecutionMode == "" {
			return fmt.Errorf("workflow %s hybrid step %s requires executionMode", workflow.Name, step.Name)
		}
		if workflow.Mode == models.WorkflowModeTransactional && step.ExecutionMode == models.WorkflowModeOutbox {
			return fmt.Errorf("workflow %s transactional step %s cannot use outbox executionMode", workflow.Name, step.Name)
		}
		if workflow.Mode == models.WorkflowModeOutbox && step.ExecutionMode == models.WorkflowModeTransactional {
			return fmt.Errorf("workflow %s outbox step %s cannot use transactional executionMode", workflow.Name, step.Name)
		}
		if step.TimeoutSec < 0 || step.TimeoutSec > maxWorkflowTimeoutSec {
			return fmt.Errorf("workflow %s step %s timeoutSec must be between 0 and %d", workflow.Name, step.Name, maxWorkflowTimeoutSec)
		}
		if step.Type == models.WorkflowStepTypeCallAPI {
			if step.TimeoutSec < minWorkflowCallAPITimeoutSec {
				return fmt.Errorf("workflow %s step %s call_api timeoutSec must be at least %d", workflow.Name, step.Name, minWorkflowCallAPITimeoutSec)
			}
			if _, err := workflowAllowedHTTPMethod(workflowStringConfig(step.Config, "method", http.MethodPost)); err != nil {
				return fmt.Errorf("workflow %s step %s: %w", workflow.Name, step.Name, err)
			}
			if workflowStringConfig(step.Config, "url", "") == "" {
				return fmt.Errorf("workflow %s step %s call_api url is required", workflow.Name, step.Name)
			}
		}
		if step.Type == models.WorkflowStepTypeUpdateRecord {
			if update, ok := workflowObjectConfig(step.Config, "update"); ok && workflowHasUpdateOperator(update) {
				if err := validateWorkflowUpdateOperatorNames(update); err != nil {
					return fmt.Errorf("workflow %s step %s: %w", workflow.Name, step.Name, err)
				}
			}
		}
		if step.Type == models.WorkflowStepTypeUnsetRecord {
			if _, err := workflowUnsetValues(step.Config); err != nil {
				return fmt.Errorf("workflow %s step %s: %w", workflow.Name, step.Name, err)
			}
		}
		if step.Type == models.WorkflowStepTypeAggregate {
			if _, hasPipeline := step.Config["pipeline"]; !hasPipeline {
				if _, hasPipelineJSON := step.Config["pipelineJson"]; !hasPipelineJSON {
					return fmt.Errorf("workflow %s step %s aggregate pipeline is required", workflow.Name, step.Name)
				}
			}
		}
		if step.Type == models.WorkflowStepTypeDistinct {
			field := workflowStringConfig(step.Config, "field", "")
			if field == "" || strings.HasPrefix(field, "$") {
				return fmt.Errorf("workflow %s step %s distinct field is required", workflow.Name, step.Name)
			}
		}
		if workflowStepExecutionMode(step, workflow.Mode) == models.WorkflowModeOutbox && workflowStepHasUnsafeOutboxReference(step) {
			return fmt.Errorf("workflow %s step %s outbox execution cannot depend on steps.* or vars.* values", workflow.Name, step.Name)
		}
		if step.Type == models.WorkflowStepTypeReturn && workflowStepExecutionMode(step, workflow.Mode) == models.WorkflowModeOutbox {
			return fmt.Errorf("workflow %s step %s return requires transactional executionMode", workflow.Name, step.Name)
		}
		if step.Type == models.WorkflowStepTypeReturn && len(step.Conditions) == 0 && workflowHasActiveStepAfter(orderedSteps, index, workflowStepExecutionMode(step, workflow.Mode), workflow.Mode) {
			return fmt.Errorf("workflow %s step %s return must be the last active step in its list", workflow.Name, step.Name)
		}
		if workflowStepExecutionMode(step, workflow.Mode) == models.WorkflowModeTransactional && workflowStepHasNonTransactionalSideEffect(step) {
			return fmt.Errorf("workflow %s step %s type %s requires outbox executionMode", workflow.Name, step.Name, step.Type)
		}
		if err := validateWorkflowConditions(workflow.Trigger, step.Conditions); err != nil {
			return fmt.Errorf("workflow %s step %s: %w", workflow.Name, step.Name, err)
		}
		if err := validateWorkflowSteps(workflow, step.Steps); err != nil {
			return err
		}
		if err := validateWorkflowSteps(workflow, step.ElseSteps); err != nil {
			return err
		}
		for _, branch := range step.Branches {
			if err := validateWorkflowConditions(workflow.Trigger, branch.Conditions); err != nil {
				return fmt.Errorf("workflow %s step %s branch %s: %w", workflow.Name, step.Name, branch.Name, err)
			}
			if err := validateWorkflowSteps(workflow, branch.Steps); err != nil {
				return err
			}
		}
	}
	return nil
}

func workflowHasActiveStepAfter(steps []models.DynamicWorkflowStep, index int, executionMode, workflowMode string) bool {
	for _, step := range steps[index+1:] {
		if step.IsActive && workflowStepExecutionMode(step, workflowMode) == executionMode {
			return true
		}
	}
	return false
}

func validateWorkflowConditions(trigger string, conditions []models.WorkflowCondition) error {
	for _, condition := range conditions {
		switch condition.Operator {
		case models.WorkflowConditionAnd, models.WorkflowConditionOr:
			if len(condition.Conditions) == 0 {
				return fmt.Errorf("condition group %s requires nested conditions", condition.Operator)
			}
			if err := validateWorkflowConditions(trigger, condition.Conditions); err != nil {
				return err
			}
			continue
		}
		switch condition.Operator {
		case models.WorkflowConditionChanged, models.WorkflowConditionChangedTo, models.WorkflowConditionChangedFrom:
			if trigger != models.WorkflowTriggerBeforeUpdate && trigger != models.WorkflowTriggerAfterUpdate {
				return fmt.Errorf("condition operator %s requires an update trigger", condition.Operator)
			}
		}
	}
	return nil
}

func workflowStepHasUnsafeOutboxReference(step models.DynamicWorkflowStep) bool {
	if workflowContainsUnsafeOutboxReference(step.Config) ||
		workflowContainsUnsafeOutboxReference(step.Conditions) ||
		strings.Contains(step.IdempotencyKey, "steps.") ||
		strings.Contains(step.IdempotencyKey, "vars.") {
		return true
	}
	for _, branch := range step.Branches {
		if workflowContainsUnsafeOutboxReference(branch.Conditions) {
			return true
		}
	}
	return false
}

func workflowContainsUnsafeOutboxReference(value interface{}) bool {
	switch typed := value.(type) {
	case string:
		return strings.Contains(typed, "steps.") || strings.Contains(typed, "vars.")
	case map[string]interface{}:
		for _, item := range typed {
			if workflowContainsUnsafeOutboxReference(item) {
				return true
			}
		}
	case []interface{}:
		for _, item := range typed {
			if workflowContainsUnsafeOutboxReference(item) {
				return true
			}
		}
	case []models.WorkflowCondition:
		for _, condition := range typed {
			if strings.Contains(condition.Field, "steps.") || strings.Contains(condition.Field, "vars.") ||
				workflowContainsUnsafeOutboxReference(condition.Value) ||
				workflowContainsUnsafeOutboxReference(condition.Conditions) {
				return true
			}
		}
	}
	return false
}

func countWorkflowSteps(steps []models.DynamicWorkflowStep) int {
	count := 0
	for _, step := range steps {
		count++
		count += countWorkflowSteps(step.Steps)
		count += countWorkflowSteps(step.ElseSteps)
		for _, branch := range step.Branches {
			count += countWorkflowSteps(branch.Steps)
		}
	}
	return count
}

func workflowHasNamedStep(steps []models.DynamicWorkflowStep, name string) bool {
	for _, step := range steps {
		if step.Name == name || workflowHasNamedStep(step.Steps, name) || workflowHasNamedStep(step.ElseSteps, name) {
			return true
		}
		for _, branch := range step.Branches {
			if workflowHasNamedStep(branch.Steps, name) {
				return true
			}
		}
	}
	return false
}

func workflowTriggerValid(trigger string) bool {
	switch trigger {
	case "", models.WorkflowTriggerManual,
		models.WorkflowTriggerBeforeCreate, models.WorkflowTriggerAfterCreate,
		models.WorkflowTriggerBeforeUpdate, models.WorkflowTriggerAfterUpdate,
		models.WorkflowTriggerBeforeDelete, models.WorkflowTriggerAfterDelete:
		return true
	default:
		return false
	}
}

func workflowModeValid(mode string) bool {
	return mode == "" || mode == models.WorkflowModeTransactional || mode == models.WorkflowModeOutbox || mode == models.WorkflowModeHybrid
}
