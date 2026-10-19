package services

import (
	"fmt"
	"strings"
	"testing"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestWorkflowValidationAuditInactiveReturnDoesNotBlockActiveSteps(t *testing.T) {
	workflow := models.DynamicWorkflow{
		Name: "disabled-return",
		Steps: []models.DynamicWorkflowStep{
			{Name: "disabled", Type: models.WorkflowStepTypeReturn, Order: 1},
			{Name: "work", Type: models.WorkflowStepTypeSetVariable, Order: 2, IsActive: true},
		},
	}
	if err := ValidateWorkflow(workflow); err != nil {
		t.Fatalf("an inactive return is skipped at execution and must not block later steps: %v", err)
	}
}

func TestWorkflowValidationAuditNamesAndCollections(t *testing.T) {
	for _, name := range []string{"", " ", "\n\t"} {
		t.Run(fmt.Sprintf("empty_%q", name), func(t *testing.T) {
			auditWorkflowValidation(t, models.DynamicWorkflow{Name: name}, "name is required")
		})
	}
	tests := []struct {
		name      string
		workflows []models.DynamicWorkflow
		wantErr   string
	}{
		{name: "nil collection"},
		{name: "empty collection", workflows: []models.DynamicWorkflow{}},
		{name: "distinct names", workflows: []models.DynamicWorkflow{{Name: "first"}, {Name: "second"}}},
		{name: "duplicate name", workflows: []models.DynamicWorkflow{{Name: "same"}, {Name: "same"}}, wantErr: "duplicate workflow name"},
		{name: "invalid later definition", workflows: []models.DynamicWorkflow{{Name: "valid"}, {Name: "invalid", Trigger: "typo"}}, wantErr: "invalid trigger"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auditWorkflowValidationError(t, ValidateWorkflows(tt.workflows), tt.wantErr)
		})
	}
}

func TestWorkflowValidationAuditTriggersAndSchedules(t *testing.T) {
	for _, trigger := range []string{"", "manual", "before_create", "after_create", "before_update", "after_update", "before_delete", "after_delete", "cron"} {
		t.Run("trigger_"+trigger, func(t *testing.T) {
			workflow := models.DynamicWorkflow{Name: "valid-trigger", Trigger: trigger}
			if trigger == "cron" {
				workflow.Schedule = "0 2 * * *"
			}
			auditWorkflowValidation(t, workflow, "")
		})
	}
	tests := []struct {
		name     string
		workflow models.DynamicWorkflow
		wantErr  string
	}{
		{name: "unknown trigger", workflow: models.DynamicWorkflow{Name: "trigger", Trigger: "AFTER_CREATE"}, wantErr: "invalid trigger"},
		{name: "missing schedule", workflow: models.DynamicWorkflow{Name: "cron", Trigger: "cron"}, wantErr: "cron schedule is required"},
		{name: "blank schedule", workflow: models.DynamicWorkflow{Name: "cron", Trigger: "cron", Schedule: " \t"}, wantErr: "cron schedule is required"},
		{name: "invalid schedule", workflow: models.DynamicWorkflow{Name: "cron", Trigger: "cron", Schedule: "60 * * * *"}, wantErr: "invalid cron schedule"},
		{name: "seconds not supported", workflow: models.DynamicWorkflow{Name: "cron", Trigger: "cron", Schedule: "0 0 2 * * *"}, wantErr: "invalid cron schedule"},
		{name: "valid timezone", workflow: models.DynamicWorkflow{Name: "cron", Trigger: "cron", Schedule: "0 2 * * *", Timezone: "America/Chicago"}},
		{name: "descriptor schedule", workflow: models.DynamicWorkflow{Name: "cron", Trigger: "cron", Schedule: "@hourly", Timezone: "UTC"}},
		{name: "invalid timezone", workflow: models.DynamicWorkflow{Name: "cron", Trigger: "cron", Schedule: "0 2 * * *", Timezone: "No/SuchZone"}, wantErr: "invalid timezone"},
		{name: "schedule requires cron", workflow: models.DynamicWorkflow{Name: "manual", Trigger: "manual", Schedule: "0 2 * * *"}, wantErr: "schedule is only supported for cron"},
		{name: "timezone requires cron", workflow: models.DynamicWorkflow{Name: "manual", Trigger: "manual", Timezone: "UTC"}, wantErr: "timezone is only supported for cron"},
		{name: "blank noncron metadata", workflow: models.DynamicWorkflow{Name: "manual", Schedule: " \t", Timezone: " \n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { auditWorkflowValidation(t, tt.workflow, tt.wantErr) })
	}
}

func TestWorkflowValidationAuditExecutionModeMatrix(t *testing.T) {
	tests := []struct {
		workflowMode string
		stepMode     string
		wantErr      string
	}{
		{},
		{stepMode: "transactional"},
		{stepMode: "outbox"},
		{stepMode: "hybrid", wantErr: "invalid executionMode"},
		{workflowMode: "transactional"},
		{workflowMode: "transactional", stepMode: "transactional"},
		{workflowMode: "transactional", stepMode: "outbox", wantErr: "cannot use outbox"},
		{workflowMode: "outbox"},
		{workflowMode: "outbox", stepMode: "outbox"},
		{workflowMode: "outbox", stepMode: "transactional", wantErr: "cannot use transactional"},
		{workflowMode: "hybrid", wantErr: "requires executionMode"},
		{workflowMode: "hybrid", stepMode: "transactional"},
		{workflowMode: "hybrid", stepMode: "outbox"},
		{workflowMode: "unknown", wantErr: "invalid mode"},
		{workflowMode: "hybrid", stepMode: "invalid", wantErr: "invalid executionMode"},
	}
	for _, tt := range tests {
		t.Run(tt.workflowMode+"/"+tt.stepMode, func(t *testing.T) {
			auditWorkflowValidation(t, models.DynamicWorkflow{Name: "mode-matrix", Mode: tt.workflowMode,
				Steps: []models.DynamicWorkflowStep{{Name: "work", Type: "set_variable", IsActive: true, ExecutionMode: tt.stepMode}},
			}, tt.wantErr)
		})
	}
}

func TestWorkflowValidationAuditTimeoutAndStepCountLimits(t *testing.T) {
	for _, seconds := range []int{-1, 0, 1, 300, 301} {
		wantErr := ""
		if seconds < 0 || seconds > 300 {
			wantErr = "timeoutSec must be between 0 and 300"
		}
		t.Run(fmt.Sprintf("workflow_timeout_%d", seconds), func(t *testing.T) {
			auditWorkflowValidation(t, models.DynamicWorkflow{Name: "timeout", TimeoutSec: seconds}, wantErr)
		})
		t.Run(fmt.Sprintf("step_timeout_%d", seconds), func(t *testing.T) {
			auditWorkflowValidation(t, models.DynamicWorkflow{Name: "timeout", Steps: []models.DynamicWorkflowStep{{Type: "set_variable", TimeoutSec: seconds}}}, wantErr)
		})
	}
	for _, location := range []string{"top", "steps", "elseSteps", "branches", "mixed"} {
		for _, total := range []int{100, 101} {
			t.Run(fmt.Sprintf("%s_%d_steps", location, total), func(t *testing.T) {
				steps := make([]models.DynamicWorkflowStep, total)
				for i := range steps {
					steps[i] = models.DynamicWorkflowStep{Type: "set_variable"}
				}
				if location != "top" {
					parent := models.DynamicWorkflowStep{Type: "if"}
					switch location {
					case "steps":
						parent.Steps = steps[1:]
					case "elseSteps":
						parent.ElseSteps = steps[1:]
					case "branches":
						parent.Branches = []models.WorkflowBranch{{Steps: steps[1:]}}
					case "mixed":
						parent.Steps = steps[1:26]
						parent.ElseSteps = steps[26:51]
						parent.Branches = []models.WorkflowBranch{{Steps: steps[51:76]}, {Steps: steps[76:]}}
					}
					steps = []models.DynamicWorkflowStep{parent}
				}
				wantErr := ""
				if total > 100 {
					wantErr = "too many steps: max 100"
				}
				auditWorkflowValidation(t, models.DynamicWorkflow{Name: "step-limit", Steps: steps}, wantErr)
			})
		}
	}
}

func TestWorkflowValidationAuditConditions(t *testing.T) {
	for _, operator := range []string{"", "=", "!=", ">", ">=", "<", "<=", "in", "not_in", "contains", "not_contains", "starts_with", "ends_with", "is_empty", "is_not_empty", "between", "exists"} {
		t.Run("supported_"+operator, func(t *testing.T) {
			auditWorkflowValidation(t, models.DynamicWorkflow{Name: "conditions", Conditions: []models.WorkflowCondition{{Field: "status", Operator: operator, Value: "open"}}}, "")
		})
	}
	for _, operator := range []string{"changed", "changed_to", "changed_from"} {
		for _, trigger := range []string{"", "manual", "before_create", "after_create", "before_update", "after_update", "before_delete", "after_delete", "cron"} {
			t.Run(operator+"/"+trigger, func(t *testing.T) {
				workflow := models.DynamicWorkflow{Name: "change", Trigger: trigger, Conditions: []models.WorkflowCondition{{Field: "status", Operator: operator}}}
				if trigger == "cron" {
					workflow.Schedule = "0 2 * * *"
				}
				wantErr := "condition operator " + operator + " requires an update trigger"
				if trigger == "before_update" || trigger == "after_update" {
					wantErr = ""
				}
				auditWorkflowValidation(t, workflow, wantErr)
			})
		}
	}
	for _, operator := range []string{"and", "or"} {
		t.Run(operator+"_empty", func(t *testing.T) {
			auditWorkflowValidation(t, models.DynamicWorkflow{Name: "empty-group", Conditions: []models.WorkflowCondition{{Operator: operator}}}, "requires nested conditions")
		})
		t.Run(operator+"_nested", func(t *testing.T) {
			auditWorkflowValidation(t, models.DynamicWorkflow{Name: "nested-group", Conditions: []models.WorkflowCondition{{Operator: operator, Conditions: []models.WorkflowCondition{{Operator: "and", Conditions: []models.WorkflowCondition{{Field: "status", Value: "open"}}}}}}}, "")
		})
		t.Run(operator+"_nested_change_requires_update", func(t *testing.T) {
			auditWorkflowValidation(t, models.DynamicWorkflow{Name: "nested-group", Conditions: []models.WorkflowCondition{{Operator: operator, Conditions: []models.WorkflowCondition{{Field: "status", Operator: "changed"}}}}}, "requires an update trigger")
		})
	}
}

func TestWorkflowValidationAuditNestedLists(t *testing.T) {
	for _, location := range []string{"steps", "elseSteps", "branches", "nested branch"} {
		for _, invalid := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s_invalid_%t", location, invalid), func(t *testing.T) {
				child := models.DynamicWorkflowStep{Name: "child", Type: "set_variable"}
				wantErr := ""
				if invalid {
					child.Type = "not_a_step"
					wantErr = "step child has unsupported type"
				}
				parent := models.DynamicWorkflowStep{Name: "parent", Type: "if"}
				switch location {
				case "steps":
					parent.Steps = []models.DynamicWorkflowStep{child}
				case "elseSteps":
					parent.ElseSteps = []models.DynamicWorkflowStep{child}
				case "branches":
					parent.Branches = []models.WorkflowBranch{{Name: "branch", Steps: []models.DynamicWorkflowStep{child}}}
				case "nested branch":
					parent.Steps = []models.DynamicWorkflowStep{{Type: "if", ElseSteps: []models.DynamicWorkflowStep{{Type: "if", Branches: []models.WorkflowBranch{{Steps: []models.DynamicWorkflowStep{child}}}}}}}
				}
				auditWorkflowValidation(t, models.DynamicWorkflow{Name: "nested", ReturnStep: "child", Steps: []models.DynamicWorkflowStep{parent}}, wantErr)
			})
		}
	}
	auditWorkflowValidation(t, models.DynamicWorkflow{Name: "missing-return", ReturnStep: "missing", Steps: []models.DynamicWorkflowStep{{Name: "other", Type: "set_variable"}}}, "returnStep does not match a named step")
}

func TestWorkflowValidationAuditReturnPlacement(t *testing.T) {
	activeReturn := models.DynamicWorkflowStep{Name: "response", Type: "return", IsActive: true, Order: 2}
	activeWork := models.DynamicWorkflowStep{Name: "work", Type: "set_variable", IsActive: true, Order: 3}
	tests := []struct {
		name    string
		steps   []models.DynamicWorkflowStep
		mode    string
		wantErr string
	}{
		{name: "terminal", steps: []models.DynamicWorkflowStep{activeReturn}},
		{name: "later active step", steps: []models.DynamicWorkflowStep{activeReturn, activeWork}, wantErr: "return must be the last active step"},
		{name: "later inactive step", steps: []models.DynamicWorkflowStep{activeReturn, {Type: "set_variable", Order: 3}}},
		{name: "inactive return before active step", steps: []models.DynamicWorkflowStep{{Type: "return", Order: 1}, activeWork}},
		{name: "conditional return", steps: []models.DynamicWorkflowStep{{Name: "response", Type: "return", IsActive: true, Order: 2, Conditions: []models.WorkflowCondition{{Field: "done", Value: true}}}, activeWork}},
		{name: "sorted before return", steps: []models.DynamicWorkflowStep{activeReturn, {Type: "set_variable", IsActive: true, Order: 1}}},
		{name: "sorted after return", steps: []models.DynamicWorkflowStep{activeWork, activeReturn}, wantErr: "return must be the last active step"},
		{name: "stable equal orders return first", steps: []models.DynamicWorkflowStep{activeReturn, {Type: "set_variable", IsActive: true, Order: 2}}, wantErr: "return must be the last active step"},
		{name: "stable equal orders return last", steps: []models.DynamicWorkflowStep{{Type: "set_variable", IsActive: true, Order: 2}, activeReturn}},
		{name: "outbox return", mode: "outbox", steps: []models.DynamicWorkflowStep{activeReturn}, wantErr: "return requires transactional executionMode"},
		{name: "later other mode", mode: "hybrid", steps: []models.DynamicWorkflowStep{{Name: "response", Type: "return", IsActive: true, Order: 2, ExecutionMode: "transactional"}, {Type: "set_variable", IsActive: true, Order: 3, ExecutionMode: "outbox"}}},
		{name: "later same mode", mode: "hybrid", steps: []models.DynamicWorkflowStep{{Name: "response", Type: "return", IsActive: true, Order: 2, ExecutionMode: "transactional"}, {Type: "set_variable", IsActive: true, Order: 3, ExecutionMode: "transactional"}}, wantErr: "return must be the last active step"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, location := range []string{"top", "steps", "elseSteps", "branches"} {
				t.Run(location, func(t *testing.T) {
					steps := tt.steps
					if location != "top" {
						parent := models.DynamicWorkflowStep{Type: "if", ExecutionMode: "transactional"}
						if tt.mode != "hybrid" {
							parent.ExecutionMode = ""
						}
						switch location {
						case "steps":
							parent.Steps = steps
						case "elseSteps":
							parent.ElseSteps = steps
						case "branches":
							parent.Branches = []models.WorkflowBranch{{Steps: steps}}
						}
						steps = []models.DynamicWorkflowStep{parent}
					}
					auditWorkflowValidation(t, models.DynamicWorkflow{Name: "return-placement", Mode: tt.mode, Steps: steps}, tt.wantErr)
				})
			}
		})
	}
}

func TestWorkflowValidationAuditSideEffectsRequireOutbox(t *testing.T) {
	for _, stepType := range []string{"call_api", "execute_dynamic_api", "invalidate_cache", "create_notification"} {
		for _, mode := range []string{"transactional", "outbox"} {
			t.Run(stepType+"/"+mode, func(t *testing.T) {
				step := auditValidWorkflowStep(stepType)
				wantErr := ""
				if mode == "transactional" {
					wantErr = "requires outbox executionMode"
				}
				auditWorkflowValidation(t, models.DynamicWorkflow{Name: "side-effect", Mode: mode, Steps: []models.DynamicWorkflowStep{step}}, wantErr)
			})
		}
	}
}

func TestWorkflowValidationAuditSupportedStepTypes(t *testing.T) {
	for _, stepType := range []string{
		"create_record", "update_record", "unset_record", "delete_record", "audit_log", "invalidate_cache", "call_api",
		"run_pipeline", "aggregate", "distinct", "emit_outbox_event", "create_notification", "get_record",
		"find_records", "if", "for_each", "set_variable", "execute_workflow", "execute_dynamic_api", "query_dynamic_api", "join_arrays",
		"fail", "set_record", "transform", "append_array", "remove_array", "add_to_set", "push", "pull", "pull_all", "set_array", "count_records", "equation", "return",
	} {
		t.Run(stepType, func(t *testing.T) {
			mode := "transactional"
			if stepType == "call_api" || stepType == "execute_dynamic_api" || stepType == "invalidate_cache" || stepType == "create_notification" {
				mode = "outbox"
			}
			auditWorkflowValidation(t, models.DynamicWorkflow{Name: "supported-type", Mode: mode, Steps: []models.DynamicWorkflowStep{auditValidWorkflowStep(stepType)}}, "")
		})
	}
	for _, stepType := range []string{"", "SET_VARIABLE", "unknown", "dynamic_function"} {
		t.Run("unsupported_"+stepType, func(t *testing.T) {
			auditWorkflowValidation(t, models.DynamicWorkflow{Name: "unsupported-type", Steps: []models.DynamicWorkflowStep{{Type: stepType}}}, "unsupported type")
		})
	}
}

func TestWorkflowValidationAuditOutboxReferenceContexts(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*models.DynamicWorkflowStep)
	}{
		{name: "nested config map", modify: func(step *models.DynamicWorkflowStep) {
			step.Config = map[string]interface{}{"data": map[string]interface{}{"value": "{{steps.previous}}"}}
		}},
		{name: "nested config array", modify: func(step *models.DynamicWorkflowStep) {
			step.Config = map[string]interface{}{"data": []interface{}{[]interface{}{"{{vars.value}}"}}}
		}},
		{name: "condition field", modify: func(step *models.DynamicWorkflowStep) {
			step.Conditions = []models.WorkflowCondition{{Field: "steps.previous", Value: true}}
		}},
		{name: "condition value", modify: func(step *models.DynamicWorkflowStep) {
			step.Conditions = []models.WorkflowCondition{{Field: "record.id", Value: "{{vars.id}}"}}
		}},
		{name: "nested condition", modify: func(step *models.DynamicWorkflowStep) {
			step.Conditions = []models.WorkflowCondition{{Operator: "or", Conditions: []models.WorkflowCondition{{Field: "vars.ready", Value: true}}}}
		}},
		{name: "idempotency key", modify: func(step *models.DynamicWorkflowStep) { step.IdempotencyKey = "{{steps.created.id}}" }},
		{name: "branch condition", modify: func(step *models.DynamicWorkflowStep) {
			step.Branches = []models.WorkflowBranch{{Conditions: []models.WorkflowCondition{{Field: "vars.ready", Value: true}}}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, scenario := range []string{"outbox", "event hybrid", "manual hybrid", "transactional", "captured by parent"} {
				t.Run(scenario, func(t *testing.T) {
					step := models.DynamicWorkflowStep{Name: "consumer", Type: "if", IsActive: true}
					tt.modify(&step)
					workflow := models.DynamicWorkflow{Name: "references", Trigger: "after_create", Mode: "outbox"}
					wantErr := "outbox execution cannot depend"
					switch scenario {
					case "event hybrid":
						workflow.Mode = "hybrid"
						step.ExecutionMode = "outbox"
					case "manual hybrid":
						workflow.Mode, workflow.Trigger = "hybrid", "manual"
						step.ExecutionMode = "outbox"
						wantErr = ""
					case "transactional":
						workflow.Mode = "transactional"
						wantErr = ""
					case "captured by parent":
						step = models.DynamicWorkflowStep{Type: "if", Steps: []models.DynamicWorkflowStep{{Type: "if", ElseSteps: []models.DynamicWorkflowStep{step}}}}
						wantErr = ""
					}
					workflow.Steps = []models.DynamicWorkflowStep{step}
					auditWorkflowValidation(t, workflow, wantErr)
				})
			}
		})
	}
	t.Run("record user and loop references", func(t *testing.T) {
		auditWorkflowValidation(t, models.DynamicWorkflow{Name: "captured-context", Mode: "outbox", Steps: []models.DynamicWorkflowStep{{Type: "set_variable", Config: map[string]interface{}{"values": map[string]interface{}{"record": "{{record.id}}", "user": "{{user.id}}", "loop": "{{loop.item}}"}}}}}, "")
	})
}

func TestWorkflowValidationAuditStepConfiguration(t *testing.T) {
	tests := []struct {
		name       string
		stepType   string
		config     map[string]interface{}
		timeoutSec int
		wantErr    string
	}{
		{name: "call api requires timeout", stepType: "call_api", config: map[string]interface{}{"url": "https://example.com"}, wantErr: "timeoutSec must be at least 1"},
		{name: "call api requires URL", stepType: "call_api", timeoutSec: 1, wantErr: "call_api url is required"},
		{name: "call api blank URL", stepType: "call_api", timeoutSec: 1, config: map[string]interface{}{"url": " \t"}, wantErr: "call_api url is required"},
		{name: "call api nonstring URL", stepType: "call_api", timeoutSec: 1, config: map[string]interface{}{"url": 123}, wantErr: "call_api url is required"},
		{name: "call api unsupported method", stepType: "call_api", timeoutSec: 1, config: map[string]interface{}{"url": "https://example.com", "method": "TRACE"}, wantErr: "method"},
		{name: "call api normalizes method", stepType: "call_api", timeoutSec: 1, config: map[string]interface{}{"url": "https://example.com", "method": " get "}},
		{name: "call api default method", stepType: "call_api", timeoutSec: 1, config: map[string]interface{}{"url": "https://example.com"}},
		{name: "query API requires name", stepType: "query_dynamic_api", wantErr: "apiName is required"},
		{name: "query API blank name", stepType: "query_dynamic_api", config: map[string]interface{}{"apiName": " \t"}, wantErr: "apiName is required"},
		{name: "query API wrong name type", stepType: "query_dynamic_api", config: map[string]interface{}{"apiName": true}, wantErr: "apiName is required"},
		{name: "aggregate requires pipeline", stepType: "aggregate", wantErr: "aggregate pipeline is required"},
		{name: "aggregate accepts pipeline JSON", stepType: "aggregate", config: map[string]interface{}{"pipelineJson": `[{"$match":{}}]`}},
		{name: "aggregate accepts dynamic pipeline", stepType: "aggregate", config: map[string]interface{}{"pipeline": "{{record.pipeline}}"}},
		{name: "distinct requires field", stepType: "distinct", wantErr: "distinct field is required"},
		{name: "distinct rejects operator field", stepType: "distinct", config: map[string]interface{}{"field": "$status"}, wantErr: "distinct field is required"},
		{name: "distinct allows nested field", stepType: "distinct", config: map[string]interface{}{"field": "owner.id"}},
		{name: "update disallows rename", stepType: "update_record", config: map[string]interface{}{"update": map[string]interface{}{"$rename": map[string]interface{}{"a": "b"}}}, wantErr: "operator is not allowed"},
		{name: "update rejects mixed styles", stepType: "update_record", config: map[string]interface{}{"update": map[string]interface{}{"$set": map[string]interface{}{"a": 1}, "other": 2}}, wantErr: "operator is not allowed"},
		{name: "update allows insert modifier", stepType: "update_record", config: map[string]interface{}{"update": map[string]interface{}{"$setOnInsert": map[string]interface{}{"status": "new"}, "$inc": map[string]interface{}{"count": 1}}}},
		{name: "update excludes must be array", stepType: "update_record", config: map[string]interface{}{"excludeFields": "password"}, wantErr: "excludeFields"},
		{name: "unset requires fields", stepType: "unset_record", wantErr: "unset_record"},
		{name: "unset rejects operator field", stepType: "unset_record", config: map[string]interface{}{"field": "$bad"}, wantErr: "invalid field"},
		{name: "unset allows nested field", stepType: "unset_record", config: map[string]interface{}{"field": "profile.obsolete"}},
		{name: "notification requires config", stepType: "create_notification", wantErr: "config is required"},
		{name: "notification requires title", stepType: "create_notification", config: map[string]interface{}{"message": "Message"}, wantErr: "title is required"},
		{name: "notification title type", stepType: "create_notification", config: map[string]interface{}{"title": 123, "message": "Message"}, wantErr: "title must be a string"},
		{name: "notification blank title", stepType: "create_notification", config: map[string]interface{}{"title": " \t", "message": "Message"}, wantErr: "title is required"},
		{name: "notification requires message", stepType: "create_notification", config: map[string]interface{}{"title": "Title"}, wantErr: "message is required"},
		{name: "notification message type", stepType: "create_notification", config: map[string]interface{}{"title": "Title", "message": false}, wantErr: "message must be a string"},
		{name: "notification blank message", stepType: "create_notification", config: map[string]interface{}{"title": "Title", "message": "\n "}, wantErr: "message is required"},
		{name: "notification type type", stepType: "create_notification", config: map[string]interface{}{"title": "Title", "message": "Message", "type": 1}, wantErr: "type must be a string"},
		{name: "notification invalid type", stepType: "create_notification", config: map[string]interface{}{"title": "Title", "message": "Message", "type": "critical"}, wantErr: "type must be one of"},
		{name: "notification dynamic type", stepType: "create_notification", config: map[string]interface{}{"title": "{{record.title}}", "message": "Message", "type": "{{record.type}}"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auditWorkflowValidation(t, models.DynamicWorkflow{Name: "step-config", Mode: "outbox", Steps: []models.DynamicWorkflowStep{{Name: "work", Type: tt.stepType, TimeoutSec: tt.timeoutSec, Config: tt.config}}}, tt.wantErr)
		})
	}
	for _, notificationType := range []string{"info", "warning", "error", "success"} {
		t.Run("notification_type_"+notificationType, func(t *testing.T) {
			auditWorkflowValidation(t, models.DynamicWorkflow{Name: "notification", Mode: "outbox", Steps: []models.DynamicWorkflowStep{{Type: "create_notification", Config: map[string]interface{}{"title": "Title", "message": "Message", "type": notificationType}}}}, "")
		})
	}
}

func TestWorkflowValidationAuditJoinConfiguration(t *testing.T) {
	for _, key := range []string{"items", "lookupItems", "localField", "lookupField", "mappings"} {
		t.Run("missing_"+key, func(t *testing.T) {
			step := auditValidWorkflowStep("join_arrays")
			delete(step.Config, key)
			auditWorkflowValidation(t, models.DynamicWorkflow{Name: "join", Steps: []models.DynamicWorkflowStep{step}}, "join_arrays "+key)
		})
	}
	for _, key := range []string{"localField", "lookupField"} {
		for _, value := range []interface{}{"", " \t", 123} {
			t.Run(fmt.Sprintf("invalid_%s_%v", key, value), func(t *testing.T) {
				step := auditValidWorkflowStep("join_arrays")
				step.Config[key] = value
				auditWorkflowValidation(t, models.DynamicWorkflow{Name: "join", Steps: []models.DynamicWorkflowStep{step}}, "join_arrays "+key+" is required")
			})
		}
	}
	for _, mappings := range []interface{}{
		nil, []interface{}{}, map[string]interface{}{},
		map[string]interface{}{" ": "name"}, map[string]interface{}{"name": " "}, map[string]interface{}{"name": true},
	} {
		t.Run(fmt.Sprintf("invalid_mapping_%#v", mappings), func(t *testing.T) {
			step := auditValidWorkflowStep("join_arrays")
			step.Config["mappings"] = mappings
			auditWorkflowValidation(t, models.DynamicWorkflow{Name: "join", Steps: []models.DynamicWorkflowStep{step}}, "join_arrays mappings")
		})
	}
	t.Run("BSON mappings", func(t *testing.T) {
		step := auditValidWorkflowStep("join_arrays")
		step.Config["mappings"] = bson.M{"label": "name"}
		auditWorkflowValidation(t, models.DynamicWorkflow{Name: "join", Steps: []models.DynamicWorkflowStep{step}}, "")
	})
}

func auditValidWorkflowStep(stepType string) models.DynamicWorkflowStep {
	step := models.DynamicWorkflowStep{Name: "work", Type: stepType, IsActive: true}
	switch stepType {
	case "call_api":
		step.TimeoutSec = 1
		step.Config = map[string]interface{}{"url": "https://example.com", "method": "POST"}
	case "unset_record":
		step.Config = map[string]interface{}{"field": "obsolete"}
	case "aggregate":
		step.Config = map[string]interface{}{"pipeline": []interface{}{map[string]interface{}{"$match": map[string]interface{}{}}}}
	case "distinct":
		step.Config = map[string]interface{}{"field": "status"}
	case "create_notification":
		step.Config = map[string]interface{}{"title": "Title", "message": "Message"}
	case "query_dynamic_api":
		step.Config = map[string]interface{}{"apiName": "query"}
	case "join_arrays":
		step.Config = map[string]interface{}{"items": []interface{}{}, "lookupItems": []interface{}{}, "localField": "id", "lookupField": "id", "mappings": map[string]interface{}{"name": "name"}}
	}
	return step
}

func auditWorkflowValidation(t *testing.T, workflow models.DynamicWorkflow, wantErr string) {
	t.Helper()
	auditWorkflowValidationError(t, ValidateWorkflow(workflow), wantErr)
}

func auditWorkflowValidationError(t *testing.T, err error, wantErr string) {
	t.Helper()
	if wantErr == "" {
		if err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("validation error = %v, want containing %q", err, wantErr)
	}
}

func TestWorkflowValidationAuditRejectsUnknownConditionOperators(t *testing.T) {
	unknown := []models.WorkflowCondition{{Field: "status", Operator: "equal_to", Value: "open"}}
	tests := []struct {
		name     string
		workflow models.DynamicWorkflow
	}{
		{name: "workflow", workflow: models.DynamicWorkflow{Name: "unknown", Conditions: unknown}},
		{name: "step", workflow: models.DynamicWorkflow{Name: "unknown", Steps: []models.DynamicWorkflowStep{{Name: "work", Type: models.WorkflowStepTypeSetVariable, Conditions: unknown}}}},
		{name: "branch", workflow: models.DynamicWorkflow{Name: "unknown", Steps: []models.DynamicWorkflowStep{{Name: "choose", Type: models.WorkflowStepTypeIf, Branches: []models.WorkflowBranch{{Name: "bad", Conditions: unknown}}}}}},
		{name: "nested group", workflow: models.DynamicWorkflow{Name: "unknown", Conditions: []models.WorkflowCondition{{Operator: models.WorkflowConditionAnd, Conditions: unknown}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkflow(tt.workflow)
			if err == nil || !strings.Contains(err.Error(), "unsupported condition operator") {
				t.Fatalf("ValidateWorkflow() error = %v, want unsupported condition operator", err)
			}
		})
	}
}

func TestWorkflowValidationAuditRejectsUnsafeBSONOutboxReferences(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
	}{
		{name: "document", value: bson.M{"id": "{{steps.create._id}}"}},
		{name: "nested document", value: map[string]interface{}{"data": bson.M{"id": "{{vars.id}}"}}},
		{name: "document in array", value: []interface{}{bson.M{"id": "{{steps.create._id}}"}}},
		{name: "BSON array", value: primitive.A{"{{vars.id}}"}},
		{name: "BSON documents", value: []bson.M{{"id": "{{steps.create._id}}"}}},
		{name: "typed documents", value: []map[string]interface{}{{"id": "{{vars.id}}"}}},
		{name: "typed strings", value: []string{"{{steps.create._id}}"}},
		{name: "nested typed slices", value: [][]string{{"{{vars.id}}"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkflow(models.DynamicWorkflow{
				Name: "unsafe-bson", Mode: models.WorkflowModeOutbox,
				Steps: []models.DynamicWorkflowStep{{Name: "work", Type: models.WorkflowStepTypeSetVariable, Config: map[string]interface{}{"values": tt.value}}},
			})
			if err == nil || !strings.Contains(err.Error(), "outbox execution cannot depend") {
				t.Fatalf("ValidateWorkflow() error = %v, want unsafe outbox dependency rejection", err)
			}
		})
	}
}
