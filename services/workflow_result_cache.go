package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/mongo"
)

// Only public reads can share results. Unknown operations and nested workflows
// stay uncached rather than relying on an author's read-only assertion.
func workflowResultCacheReadOnly(steps []models.DynamicWorkflowStep) bool {
	for _, step := range steps {
		if !step.IsActive {
			continue
		}
		if step.ExecutionMode == models.WorkflowModeOutbox {
			return false
		}
		switch step.Type {
		case models.WorkflowStepTypeGetRecord, models.WorkflowStepTypeFindRecords,
			models.WorkflowStepTypeAggregate, models.WorkflowStepTypeRunPipeline,
			models.WorkflowStepTypeDistinct, models.WorkflowStepTypeCountRecords,
			models.WorkflowStepTypeQueryDynamicAPI, // This operation enforces GET itself.
			models.WorkflowStepTypeIf, models.WorkflowStepTypeForEach,
			models.WorkflowStepTypeSetVariable, models.WorkflowStepTypeSetRecord,
			models.WorkflowStepTypeTransform, models.WorkflowStepTypeJoinArrays,
			models.WorkflowStepTypeEquation,
			models.WorkflowStepTypeReturn, models.WorkflowStepTypeFail:
		default:
			return false
		}
		if !workflowResultCacheReadOnly(step.Steps) || !workflowResultCacheReadOnly(step.ElseSteps) {
			return false
		}
		for _, branch := range step.Branches {
			if !workflowResultCacheReadOnly(branch.Steps) {
				return false
			}
		}
	}
	return true
}

func workflowResultCacheKey(ctx context.Context, input ExecuteWorkflowInput, workflow models.DynamicWorkflow, container *models.ContainerModel) (string, bool) {
	if !workflow.IsRedisCached || workflow.IsAuthenticated || workflow.IsAuthorized || len(workflow.AuthorizeRole) > 0 ||
		workflowTrigger(workflow) != models.WorkflowTriggerManual || workflow.Mode == models.WorkflowModeOutbox ||
		input.TenantID == "" || input.ProjectID == "" || input.Schema == "" || input.FormConfigRef != nil ||
		input.UserID != "" || input.UserRole != "" || len(input.UserRoles) > 0 || input.AuditUser != nil ||
		input.IdentityKind != AccessIdentityCaller || mongo.SessionFromContext(ctx) != nil ||
		!workflowResultCacheReadOnly(workflow.Steps) || ctx.Err() != nil || container.IsAuthContainer {
		return "", false
	}
	// Definitions and all inputs are hashed, never exposed in Redis key text.
	// Container access metadata participates so changed field permissions cannot
	// reuse responses produced under an earlier policy.
	definition, err := json.Marshal(container)
	if err != nil {
		return "", false
	}
	inputs, err := json.Marshal(struct {
		Record, Query, OldRecord, StepOutputs map[string]interface{}
		Pager                                 interface{}
	}{input.Record, input.Query, input.OldRecord, input.StepOutputs, input.Pager})
	if err != nil {
		return "", false
	}
	version, err := utils.GetProjectWorkflowCacheVersion(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return "", false
	}
	key := fmt.Sprintf("workflow-result:v1:tenant:%s:project:%s:revision:%d:schema:%s:workflow:%s:definition:%s:input:%s",
		input.TenantID, input.ProjectID, version, input.Schema, workflow.Name, utils.HashCacheQuery(string(definition)), utils.HashCacheQuery(string(inputs)))
	return key, true
}

type cachedWorkflowResult struct {
	Version    int                            `json:"version"`
	Data       interface{}                    `json:"data"`
	Pagination *workflowTableSourcePagination `json:"pagination,omitempty"`
}

func (s *DynamicService) cachedWorkflowResult(ctx context.Context, key string) (DynamicExecutionResult, bool) {
	value, ok := s.cache.GetValue(ctx, key)
	if !ok || ctx.Err() != nil {
		return DynamicExecutionResult{}, false
	}
	encoded, ok := value.(string)
	if !ok {
		return DynamicExecutionResult{}, false
	}
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.UseNumber() // Large identifiers/counts must not round through float64.
	var stored cachedWorkflowResult
	if decoder.Decode(&stored) != nil || stored.Version != 1 {
		return DynamicExecutionResult{}, false
	}
	return DynamicExecutionResult{Message: "Workflow result fetched from cache", Data: stored.Data, Source: "cache", pagination: stored.Pagination}, true
}

func (s *DynamicService) storeWorkflowResult(ctx context.Context, key string, workflow models.DynamicWorkflow, result DynamicExecutionResult) {
	encoded, err := json.Marshal(cachedWorkflowResult{Version: 1, Data: result.Data, Pagination: result.pagination})
	if err == nil && ctx.Err() == nil {
		s.cache.SetValue(ctx, key, string(encoded), cacheDuration(workflow.CacheTime))
	}
}
