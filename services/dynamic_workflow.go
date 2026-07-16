package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/observability"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"github.com/osmansam/autotableGo/validators"
	"github.com/osmansam/autotableGo/ws"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

type workflowExecutionPayload struct {
	TenantID        string
	ProjectID       string
	SchemaName      string
	WorkflowName    string
	WorkflowTrigger string
	WorkflowVersion int
	WorkflowMode    string
	ExecutionMode   string
	StopOnError     bool
	Record          map[string]interface{}
	Query           map[string]interface{}
	OldRecord       map[string]interface{}
	StepOutputs     map[string]interface{}
	Variables       map[string]interface{}
	Loop            map[string]interface{}
	UserID          string
	AuditUser       *models.AuditUser
	Container       *models.ContainerModel
	OutboxEventID   primitive.ObjectID
	IdempotencyKey  string
	WorkflowDepth   int
	OutboxEvents    *int
	ReturnValue     interface{}
	HasReturn       bool
	Pagination      *workflowTableSourcePagination
}

type workflowTableSourcePagination struct {
	Pager   utils.Pager
	Applied bool
}

func (p *workflowTableSourcePagination) disableWorkflowPagination() {
	if p == nil {
		return
	}
	p.Applied = false
	p.Pager.TotalItems = 0
	p.Pager.TotalPages = 0
}

const (
	maxWorkflowDepth             = 4
	maxWorkflowSteps             = 100
	maxWorkflowOutboxEvents      = 100
	maxWorkflowLoopItems         = 500
	maxWorkflowQueryLimit        = 4000
	maxWorkflowAggregateLimit    = 500
	maxWorkflowAggregateSkip     = 5000
	minWorkflowCallAPITimeoutSec = 1
)

func (s *DynamicService) runTransactionalWorkflows(ctx mongo.SessionContext, payload workflowExecutionPayload, trigger string) error {
	return s.runWorkflows(ctx, payload, trigger, models.WorkflowModeTransactional)
}

func (s *DynamicService) enqueueOutboxWorkflows(ctx mongo.SessionContext, payload workflowExecutionPayload, trigger string) error {
	return s.runWorkflows(ctx, payload, trigger, models.WorkflowModeOutbox)
}

func (s *DynamicService) runWorkflowDefinition(ctx context.Context, payload *workflowExecutionPayload, workflow models.DynamicWorkflow) error {
	ctx, span := observability.StartSpan(ctx, "workflow.execute", observability.WorkflowTraceAttrs(payload.TenantID, payload.ProjectID, payload.SchemaName, workflow.Name)...)
	start := time.Now()
	status := "success"
	var spanErr error
	defer func() {
		duration := time.Since(start)
		observability.RecordWorkflowExecution(payload.TenantID, payload.ProjectID, workflow.Name, payload.SchemaName, status, duration)
		attrs := append(observability.WorkflowAttrs(payload.TenantID, payload.ProjectID, payload.SchemaName, workflow.Name),
			observability.OperationAttrs("workflow_execute", status, duration)...)
		observability.InfoCtx(ctx, "workflow execution completed", attrs...)
		span.SetAttributes(attribute.String(observability.FieldOperation, "workflow_execute"))
		observability.EndSpan(span, status, spanErr)
	}()

	if err := ValidateWorkflow(workflow); err != nil {
		status = "error"
		spanErr = err
		return err
	}
	if err := validateWorkflowExecutionBounds(*payload, workflow); err != nil {
		status = "error"
		spanErr = err
		return err
	}
	payload.WorkflowTrigger = workflowTrigger(workflow)
	payload.WorkflowVersion = workflowVersion(workflow)
	ensureWorkflowPayloadMaps(payload)
	if payload.OutboxEvents == nil {
		outboxEvents := 0
		payload.OutboxEvents = &outboxEvents
	}
	if !workflowConditionsMatch(workflow.Conditions, *payload) {
		status = "skipped"
		return nil
	}

	workflowCtx, cancel := workflowContextWithTimeout(ctx, workflow.TimeoutSec)
	defer cancel()

	if workflowSupportsMode(workflow.Mode, models.WorkflowModeTransactional) {
		if err := s.runWorkflowSteps(workflowCtx, payload, workflow, models.WorkflowModeTransactional); err != nil {
			status = "error"
			spanErr = err
			return err
		}
	}
	if workflowSupportsMode(workflow.Mode, models.WorkflowModeOutbox) {
		outboxPayload := *payload
		outboxPayload.ReturnValue = nil
		outboxPayload.HasReturn = false
		if err := s.runWorkflowSteps(workflowCtx, &outboxPayload, workflow, models.WorkflowModeOutbox); err != nil {
			status = "error"
			spanErr = err
			return err
		}
	}

	return nil
}

func (s *DynamicService) runWorkflowSteps(ctx context.Context, payload *workflowExecutionPayload, workflow models.DynamicWorkflow, executionMode string) error {
	steps := append([]models.DynamicWorkflowStep(nil), workflow.Steps...)
	sort.SliceStable(steps, func(i, j int) bool {
		return steps[i].Order < steps[j].Order
	})

	ensureWorkflowPayloadMaps(payload)
	return s.runWorkflowStepList(ctx, payload, workflow.Name, workflow.Mode, executionMode, workflow.StopOnError, steps)
}

func (s *DynamicService) runWorkflows(ctx mongo.SessionContext, payload workflowExecutionPayload, trigger, executionMode string) error {
	if payload.Container == nil || len(payload.Container.Workflows) == 0 {
		return nil
	}
	ensureWorkflowPayloadMaps(&payload)
	if payload.OutboxEvents == nil {
		outboxEvents := 0
		payload.OutboxEvents = &outboxEvents
	}

	for _, workflow := range payload.Container.Workflows {
		if !workflow.IsActive || workflow.Trigger != trigger || !workflowSupportsMode(workflow.Mode, executionMode) {
			continue
		}
		if err := ValidateWorkflow(workflow); err != nil {
			return err
		}
		if err := validateWorkflowExecutionBounds(payload, workflow); err != nil {
			return err
		}
		if !workflowConditionsMatch(workflow.Conditions, payload) {
			continue
		}

		start := time.Now()
		status := "success"

		workflowCtx, cancel := workflowContextWithTimeout(ctx, workflow.TimeoutSec)
		steps := append([]models.DynamicWorkflowStep(nil), workflow.Steps...)
		sort.SliceStable(steps, func(i, j int) bool {
			return steps[i].Order < steps[j].Order
		})

		workflowPayload := payload
		workflowPayload.WorkflowTrigger = trigger
		workflowPayload.WorkflowVersion = workflowVersion(workflow)
		if err := s.runWorkflowStepList(workflowCtx, &workflowPayload, workflow.Name, workflow.Mode, executionMode, workflow.StopOnError, steps); err != nil {
			cancel()
			status = "error"
			duration := time.Since(start)
			observability.RecordWorkflowExecution(payload.TenantID, payload.ProjectID, workflow.Name, payload.SchemaName, status, duration)
			attrs := append(observability.WorkflowAttrs(payload.TenantID, payload.ProjectID, payload.SchemaName, workflow.Name),
				observability.OperationAttrs("workflow_execute", status, duration)...)
			observability.ErrorCtx(ctx, "workflow execution failed", err, attrs...)
			return err
		}
		cancel()
		duration := time.Since(start)
		observability.RecordWorkflowExecution(payload.TenantID, payload.ProjectID, workflow.Name, payload.SchemaName, status, duration)
		attrs := append(observability.WorkflowAttrs(payload.TenantID, payload.ProjectID, payload.SchemaName, workflow.Name),
			observability.OperationAttrs("workflow_execute", status, duration)...)
		observability.InfoCtx(ctx, "workflow execution completed", attrs...)
	}
	return nil
}

func ensureWorkflowPayloadMaps(payload *workflowExecutionPayload) {
	if payload.StepOutputs == nil {
		payload.StepOutputs = map[string]interface{}{}
	}
	if payload.Query == nil {
		payload.Query = map[string]interface{}{}
	}
	if payload.Variables == nil {
		payload.Variables = map[string]interface{}{}
	}
	if payload.Loop == nil {
		payload.Loop = map[string]interface{}{}
	}
}

func (s *DynamicService) runWorkflowStepList(ctx context.Context, payload *workflowExecutionPayload, workflowName, workflowMode, executionMode string, stopOnError bool, steps []models.DynamicWorkflowStep) error {
	ensureWorkflowPayloadMaps(payload)
	orderedSteps := append([]models.DynamicWorkflowStep(nil), steps...)
	sort.SliceStable(orderedSteps, func(i, j int) bool {
		return orderedSteps[i].Order < orderedSteps[j].Order
	})

	for _, step := range orderedSteps {
		if !step.IsActive || workflowStepExecutionMode(step, workflowMode) != executionMode {
			continue
		}
		if step.Type != models.WorkflowStepTypeIf && !workflowConditionsMatch(step.Conditions, *payload) {
			continue
		}

		var err error
		var output interface{}
		start := time.Now()
		status := "success"
		stepAttrs := append(observability.WorkflowTraceAttrs(payload.TenantID, payload.ProjectID, payload.SchemaName, workflowName),
			attribute.String(observability.FieldOperation, "workflow_step_execute"),
			attribute.String(observability.FieldStepType, step.Type),
		)
		stepCtx, stepSpan := observability.StartSpan(ctx, "workflow.step", stepAttrs...)
		if executionMode == models.WorkflowModeOutbox {
			stepPayload := *payload
			stepPayload.WorkflowName = workflowName
			stepPayload.WorkflowMode = workflowMode
			stepPayload.ExecutionMode = executionMode
			stepPayload.StopOnError = stopOnError
			err = s.enqueueWorkflowStep(stepCtx, stepPayload, workflowName, step)
			if err == nil {
				status = "queued"
			}
		} else {
			payload.WorkflowName = workflowName
			payload.WorkflowMode = workflowMode
			payload.ExecutionMode = executionMode
			payload.StopOnError = stopOnError
			output, err = s.processWorkflowStepForMode(stepCtx, step, payload, executionMode)
			if err == nil && step.Name != "" {
				payload.StepOutputs[step.Name] = output
			}
		}
		if err != nil {
			status = "error"
		}
		observability.EndSpan(stepSpan, status, err)
		duration := time.Since(start)
		observability.RecordWorkflowStepExecution(payload.TenantID, payload.ProjectID, workflowName, step.Type, status, duration)
		attrs := append(observability.WorkflowAttrs(payload.TenantID, payload.ProjectID, payload.SchemaName, workflowName),
			observability.OperationAttrs("workflow_step_execute", status, duration)...)
		attrs = append(attrs, slog.String(observability.FieldStepType, step.Type))
		if err != nil {
			observability.ErrorCtx(ctx, "workflow step execution failed", err, attrs...)
		} else {
			observability.DebugCtx(ctx, "workflow step execution completed", attrs...)
		}
		if err != nil && stopOnError && !step.ContinueOnError {
			return fmt.Errorf("workflow %s step %s failed: %w", workflowName, step.Name, err)
		}
		if payload.HasReturn {
			return nil
		}
	}
	return nil
}

func (s *DynamicService) processWorkflowStepForMode(ctx context.Context, step models.DynamicWorkflowStep, payload *workflowExecutionPayload, executionMode string) (interface{}, error) {
	if executionMode == models.WorkflowModeTransactional && workflowStepHasNonTransactionalSideEffect(step) {
		return nil, fmt.Errorf("%s steps must run in outbox execution mode", step.Type)
	}
	return s.processWorkflowStepWithTimeout(ctx, step, payload)
}

func (s *DynamicService) processWorkflowStepWithTimeout(ctx context.Context, step models.DynamicWorkflowStep, payload *workflowExecutionPayload) (interface{}, error) {
	ensureWorkflowPayloadMaps(payload)
	stepCtx := ctx
	var cancel context.CancelFunc
	if step.TimeoutSec > 0 {
		stepCtx, cancel = context.WithTimeout(ctx, time.Duration(step.TimeoutSec)*time.Second)
		defer cancel()
	}
	return s.processWorkflowStep(stepCtx, step, payload)
}

func (s *DynamicService) processWorkflowStep(ctx context.Context, step models.DynamicWorkflowStep, payload *workflowExecutionPayload) (interface{}, error) {
	switch step.Type {
	case models.WorkflowStepTypeCreateRecord:
		return s.workflowCreateRecord(ctx, step, *payload)
	case models.WorkflowStepTypeUpdateRecord:
		return s.workflowUpdateRecord(ctx, step, *payload)
	case models.WorkflowStepTypeUnsetRecord:
		return s.workflowUnsetRecord(ctx, step, *payload)
	case models.WorkflowStepTypeDeleteRecord:
		return s.workflowDeleteRecord(ctx, step, *payload)
	case models.WorkflowStepTypeAuditLog:
		return s.workflowAuditLog(ctx, step, *payload)
	case models.WorkflowStepTypeInvalidateCache:
		return s.workflowInvalidateCache(ctx, step, *payload)
	case models.WorkflowStepTypeCallAPI:
		return s.workflowCallAPI(ctx, step, *payload)
	case models.WorkflowStepTypeRunPipeline:
		return s.workflowRunPipeline(ctx, step, *payload)
	case models.WorkflowStepTypeAggregate:
		return s.workflowAggregate(ctx, step, *payload)
	case models.WorkflowStepTypeDynamicFunction:
		return nil, fmt.Errorf("workflow dynamic_function steps require a request context and are not supported by the write workflow processor yet")
	case models.WorkflowStepTypeEmitOutboxEvent:
		return s.workflowEmitOutboxEvent(ctx, step, *payload)
	case models.WorkflowStepTypeCreateNotification:
		return s.workflowCreateNotification(ctx, step, *payload)
	case models.WorkflowStepTypeGetRecord:
		return s.workflowGetRecord(ctx, step, *payload)
	case models.WorkflowStepTypeFindRecords:
		return s.workflowFindRecords(ctx, step, *payload)
	case models.WorkflowStepTypeCountRecords:
		return s.workflowCountRecords(ctx, step, *payload)
	case models.WorkflowStepTypeDistinct:
		return s.workflowDistinct(ctx, step, *payload)
	case models.WorkflowStepTypeIf:
		return s.workflowIf(ctx, step, payload)
	case models.WorkflowStepTypeForEach:
		return s.workflowForEach(ctx, step, payload)
	case models.WorkflowStepTypeSetVariable:
		return s.workflowSetVariable(step, payload)
	case models.WorkflowStepTypeExecuteWorkflow:
		return s.workflowExecuteWorkflow(ctx, step, *payload)
	case models.WorkflowStepTypeExecuteDynamicAPI:
		return s.workflowExecuteDynamicAPI(ctx, step, *payload)
	case models.WorkflowStepTypeQueryDynamicAPI:
		return s.workflowQueryDynamicAPI(ctx, step, *payload)
	case models.WorkflowStepTypeJoinArrays:
		return workflowJoinArrays(step, *payload)
	case models.WorkflowStepTypeFail:
		return nil, workflowFail(step, *payload)
	case models.WorkflowStepTypeSetRecord:
		return workflowSetRecord(step, payload)
	case models.WorkflowStepTypeTransform:
		return workflowTransform(step, payload)
	case models.WorkflowStepTypeAppendArray,
		models.WorkflowStepTypeRemoveArray,
		models.WorkflowStepTypeAddToSet,
		models.WorkflowStepTypePush,
		models.WorkflowStepTypePull,
		models.WorkflowStepTypePullAll,
		models.WorkflowStepTypeSetArray:
		return s.workflowUpdateArray(ctx, step, *payload)
	case models.WorkflowStepTypeEquation:
		return workflowEquation(step, payload)
	case models.WorkflowStepTypeReturn:
		return workflowReturn(step, payload)
	default:
		return nil, fmt.Errorf("unsupported workflow step type: %s", step.Type)
	}
}

func (s *DynamicService) workflowCreateRecord(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	targetSchema := workflowTargetSchema(step, payload.SchemaName)
	documentConfig, ok := step.Config["document"]
	if !ok {
		documentConfig = step.Config
	}
	document, ok := resolveWorkflowTemplates(documentConfig, payload).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("create_record step requires document config")
	}
	targetContainer, err := s.workflowTargetContainer(ctx, payload, targetSchema)
	if err != nil {
		return nil, err
	}
	workflowStringifyObjectIDsForValidation(targetContainer, document)
	if err := validators.PrepareCreateItem(payload.TenantID, payload.ProjectID, targetContainer, document); err != nil {
		return nil, err
	}
	if err := s.applyAutoIncrementFields(ctx, targetSchema, targetContainer, document); err != nil {
		return nil, err
	}
	if payload.IdempotencyKey != "" {
		existing, err := s.repository.FindOne(ctx, payload.TenantID, payload.ProjectID, targetSchema, bson.M{"_workflowIdempotencyKey": payload.IdempotencyKey})
		if err == nil {
			return map[string]interface{}(existing), nil
		}
		if err != mongo.ErrNoDocuments {
			return nil, err
		}
		document["_workflowIdempotencyKey"] = payload.IdempotencyKey
	}
	result, err := s.repository.Insert(ctx, payload.TenantID, payload.ProjectID, targetSchema, document)
	if err != nil {
		return nil, err
	}
	document["_id"] = result.InsertedID
	if err := s.insertDynamicPostWrite(ctx, payload.TenantID, payload.ProjectID, targetSchema, models.DynamicOutboxOperationCreate, payload.UserID, targetContainer,
		buildDynamicAuditLog(payload.TenantID, payload.ProjectID, targetSchema, models.DynamicOutboxOperationCreate, payload.AuditUser, nil, document)); err != nil {
		return nil, err
	}
	return document, nil
}

func (s *DynamicService) workflowUpdateRecord(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	targetSchema := workflowTargetSchema(step, payload.SchemaName)
	filter, ok := resolveWorkflowTemplates(step.Config["filter"], payload).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("update_record step requires filter config")
	}
	update, ok := resolveWorkflowTemplates(step.Config["update"], payload).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("update_record step requires update config")
	}
	normalizeWorkflowIDs(filter)
	if workflowHasUpdateOperator(update) {
		if err := validateWorkflowUpdateOperators(update); err != nil {
			return nil, err
		}
		normalizeWorkflowIDs(update)
	} else {
		targetContainer, err := s.workflowTargetContainer(ctx, payload, targetSchema)
		if err != nil {
			return nil, err
		}
		workflowStringifyObjectIDsForValidation(targetContainer, update)
		if err := validators.PrepareUpdateFields(targetContainer, update); err != nil {
			return nil, err
		}
		update = map[string]interface{}{"$set": update}
	}
	result, err := s.repository.GetCollection(payload.TenantID, payload.ProjectID, targetSchema).UpdateMany(ctx, bson.M(filter), bson.M(update))
	if err != nil {
		return nil, err
	}
	output := map[string]interface{}{
		"matchedCount":  result.MatchedCount,
		"modifiedCount": result.ModifiedCount,
		"upsertedCount": result.UpsertedCount,
		"upsertedId":    result.UpsertedID,
	}
	targetContainer, err := s.workflowTargetContainer(ctx, payload, targetSchema)
	if err != nil {
		return nil, err
	}
	if err := s.insertDynamicPostWrite(ctx, payload.TenantID, payload.ProjectID, targetSchema, models.DynamicOutboxOperationUpdate, payload.UserID, targetContainer,
		buildDynamicAuditLog(payload.TenantID, payload.ProjectID, targetSchema, models.DynamicOutboxOperationUpdate, payload.AuditUser, map[string]interface{}{"filter": filter}, map[string]interface{}{"update": update, "result": output})); err != nil {
		return nil, err
	}
	return output, nil
}

func (s *DynamicService) workflowUnsetRecord(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	unset, err := workflowUnsetValues(step.Config)
	if err != nil {
		return nil, err
	}
	updateStep := step
	updateStep.Type = models.WorkflowStepTypeUpdateRecord
	updateStep.Config = cloneWorkflowMap(step.Config)
	updateStep.Config["update"] = map[string]interface{}{"$unset": unset}
	return s.workflowUpdateRecord(ctx, updateStep, payload)
}

func (s *DynamicService) workflowDeleteRecord(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	targetSchema := workflowTargetSchema(step, payload.SchemaName)
	filter, ok := resolveWorkflowTemplates(step.Config["filter"], payload).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("delete_record step requires filter config")
	}
	normalizeWorkflowIDs(filter)
	result, err := s.repository.GetCollection(payload.TenantID, payload.ProjectID, targetSchema).DeleteMany(ctx, bson.M(filter))
	if err != nil {
		return nil, err
	}
	output := map[string]interface{}{"deletedCount": result.DeletedCount}
	targetContainer, err := s.workflowTargetContainer(ctx, payload, targetSchema)
	if err != nil {
		return nil, err
	}
	if err := s.insertDynamicPostWrite(ctx, payload.TenantID, payload.ProjectID, targetSchema, models.DynamicOutboxOperationDelete, payload.UserID, targetContainer,
		buildDynamicAuditLog(payload.TenantID, payload.ProjectID, targetSchema, models.DynamicOutboxOperationDelete, payload.AuditUser, map[string]interface{}{"filter": filter}, output)); err != nil {
		return nil, err
	}
	return output, nil
}

func (s *DynamicService) workflowGetRecord(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	targetSchema := workflowTargetSchema(step, payload.SchemaName)
	targetContainer, err := s.workflowTargetContainer(ctx, payload, targetSchema)
	if err != nil {
		return nil, err
	}
	if _, hasFilter := step.Config["filter"]; !hasFilter {
		if _, hasFilters := step.Config["filters"]; !hasFilters {
			id := resolveWorkflowTemplates(step.Config["id"], payload)
			if id == nil {
				return nil, fmt.Errorf("get_record step requires id, filter, or filters config")
			}
			result, err := s.GetDynamicItem(ctx, GetDynamicItemInput{
				TenantID:  payload.TenantID,
				ProjectID: payload.ProjectID,
				Schema:    targetSchema,
				ID:        fmt.Sprint(id),
				UserRole:  workflowUserRole(payload),
				Container: targetContainer,
			})
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"found":     true,
				"data":      result.Item,
				"fromCache": result.FromCache,
			}, nil
		}
	}

	filter, ok, err := workflowFilterConfig(step.Config, payload, targetContainer)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("get_record step requires id, filter, or filters config")
	}
	normalizeWorkflowIDs(filter)

	item, err := s.repository.FindOne(ctx, payload.TenantID, payload.ProjectID, targetSchema, filter)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return map[string]interface{}{"found": false, "data": nil}, nil
		}
		return nil, err
	}
	resultItem := map[string]interface{}(item)
	utils.StripHashed(targetContainer.Fields, []map[string]interface{}{resultItem})
	populated, err := utils.PopulateIfNeeded(ctx, payload.TenantID, payload.ProjectID, targetContainer, []map[string]interface{}{resultItem})
	if err != nil {
		return nil, err
	}
	if len(populated) > 0 {
		resultItem = populated[0]
	}
	filtered := utils.FilterDocuments([]map[string]interface{}{resultItem}, targetContainer.Fields, workflowUserRole(payload))
	if len(filtered) > 0 {
		resultItem = filtered[0]
	}
	return map[string]interface{}{"found": true, "data": resultItem}, nil
}

func (s *DynamicService) workflowFindRecords(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	targetSchema := workflowTargetSchema(step, payload.SchemaName)
	targetContainer, err := s.workflowTargetContainer(ctx, payload, targetSchema)
	if err != nil {
		return nil, err
	}
	filter, ok, err := workflowFilterConfig(step.Config, payload, targetContainer)
	if err != nil {
		return nil, err
	}
	if !ok {
		filter = bson.M{}
	}
	normalizeWorkflowIDs(filter)

	searchKey := workflowFindRecordsSearchKey(step.Config, payload)
	searchFields := workflowFindRecordsSearchFields(step.Config, payload)
	usesPostPopulationSearch := searchKey != "" && len(searchFields) > 0
	if searchKey != "" && !usesPostPopulationSearch {
		orClauses, err := utils.BuildSearchWithReferences(ctx, targetContainer, searchKey)
		if err != nil {
			return nil, err
		}
		if len(orClauses) > 0 {
			if len(filter) > 0 {
				filter = bson.M{"$and": []bson.M{filter, bson.M{"$or": orClauses}}}
			} else {
				filter = bson.M{"$or": orClauses}
			}
		}
	}

	userRole := workflowUserRole(payload)
	userMap := map[string]interface{}{"id": payload.UserID, "_id": payload.UserID, "role": userRole}
	rowAccessFilter, err := utils.GetRowAccessFilter(targetContainer, userRole, userMap)
	if err != nil {
		return nil, err
	}
	if rowAccessFilter != nil {
		if len(filter) > 0 {
			filter = bson.M{"$and": []bson.M{filter, rowAccessFilter}}
		} else {
			filter = rowAccessFilter
		}
	}

	limit, skip, err := workflowFindRecordsWindow(step.Config, payload.Pagination, usesPostPopulationSearch)
	if err != nil {
		return nil, err
	}
	if payload.Pagination != nil && payload.Pagination.Pager.Enabled && !usesPostPopulationSearch {
		totalItems, err := s.repository.Count(ctx, payload.TenantID, payload.ProjectID, targetSchema, filter)
		if err != nil {
			return nil, err
		}
		totalPages := 0
		if totalItems > 0 {
			totalPages = int((totalItems + int64(limit) - 1) / int64(limit))
		}
		payload.Pagination.Pager.TotalItems = totalItems
		payload.Pagination.Pager.TotalPages = totalPages
		payload.Pagination.Applied = true
	}

	findOptions := options.Find().SetLimit(int64(limit)).SetSkip(skip).SetMaxTime(10 * time.Second)
	if sortConfig, ok := resolveWorkflowTemplates(step.Config["sort"], payload).(map[string]interface{}); ok && len(sortConfig) > 0 {
		findOptions.SetSort(bson.M(sortConfig))
	}
	if projectionConfig, ok := resolveWorkflowTemplates(step.Config["projection"], payload).(map[string]interface{}); ok && len(projectionConfig) > 0 {
		findOptions.SetProjection(bson.M(projectionConfig))
	}

	items, err := s.repository.Query(ctx, payload.TenantID, payload.ProjectID, targetSchema, filter, findOptions, &utils.Pager{Enabled: false})
	if err != nil {
		return nil, err
	}

	utils.StripHashed(targetContainer.Fields, items)
	items, err = utils.PopulateIfNeeded(ctx, payload.TenantID, payload.ProjectID, targetContainer, items)
	if err != nil {
		return nil, err
	}
	if usesPostPopulationSearch {
		items = workflowFilterItemsBySearchFields(items, searchKey, searchFields)
		payload.Pagination.disableWorkflowPagination()
	}
	items = utils.FilterDocuments(items, targetContainer.Fields, userRole)
	items = workflowApplyOutputMappings(step.Config, items)

	return map[string]interface{}{
		"items": items,
		"count": len(items),
	}, nil
}

func workflowApplyOutputMappings(config map[string]interface{}, items []map[string]interface{}) []map[string]interface{} {
	rawMappings, ok := config["outputMappings"]
	if !ok {
		return items
	}
	mappings, ok := workflowMapConfig(rawMappings)
	if !ok || len(mappings) == 0 {
		return items
	}

	for _, item := range items {
		for outputField, rawPath := range mappings {
			path := strings.TrimSpace(fmt.Sprint(rawPath))
			if path == "" {
				continue
			}
			if value, ok := workflowPathValue(item, path); ok {
				item[outputField] = value
			}
		}
	}
	return items
}

func workflowFindRecordsWindow(config map[string]interface{}, pagination *workflowTableSourcePagination, ignorePagination bool) (int, int64, error) {
	limit := workflowIntConfig(config, "limit", 50)
	skip := int64(0)
	if pagination != nil && pagination.Pager.Enabled && !ignorePagination {
		limit = pagination.Pager.Limit
		skip = pagination.Pager.Skip
	}
	if limit <= 0 || limit > maxWorkflowQueryLimit {
		return 0, 0, fmt.Errorf("find_records limit must be between 1 and %d", maxWorkflowQueryLimit)
	}
	if skip < 0 {
		return 0, 0, fmt.Errorf("find_records skip must be non-negative")
	}
	return limit, skip, nil
}

func (s *DynamicService) workflowCountRecords(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	targetSchema := workflowTargetSchema(step, payload.SchemaName)
	targetContainer, err := s.workflowTargetContainer(ctx, payload, targetSchema)
	if err != nil {
		return nil, err
	}
	filter, ok, err := workflowFilterConfig(step.Config, payload, targetContainer)
	if err != nil {
		return nil, err
	}
	if !ok {
		filter = bson.M{}
	}
	normalizeWorkflowIDs(filter)
	if workflowReadStepApplyRowAccess(step, payload) {
		userRole := workflowUserRole(payload)
		userMap := map[string]interface{}{"id": payload.UserID, "_id": payload.UserID, "role": userRole}
		rowAccessFilter, err := utils.GetRowAccessFilter(targetContainer, userRole, userMap)
		if err != nil {
			return nil, err
		}
		if rowAccessFilter != nil {
			if len(filter) > 0 {
				filter = bson.M{"$and": []bson.M{filter, rowAccessFilter}}
			} else {
				filter = rowAccessFilter
			}
		}
	}

	count, err := s.repository.Count(ctx, payload.TenantID, payload.ProjectID, targetSchema, filter)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"count": count}, nil
}

func (s *DynamicService) workflowDistinct(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	targetSchema := workflowTargetSchema(step, payload.SchemaName)
	targetContainer, err := s.workflowTargetContainer(ctx, payload, targetSchema)
	if err != nil {
		return nil, err
	}
	field := workflowStringConfig(step.Config, "field", "")
	if field == "" || strings.HasPrefix(field, "$") {
		return nil, fmt.Errorf("distinct step requires valid field config")
	}
	filter, ok, err := workflowFilterConfig(step.Config, payload, targetContainer)
	if err != nil {
		return nil, err
	}
	if !ok {
		filter = bson.M{}
	}
	normalizeWorkflowIDs(filter)
	if workflowReadStepApplyRowAccess(step, payload) {
		userRole := workflowUserRole(payload)
		userMap := map[string]interface{}{"id": payload.UserID, "_id": payload.UserID, "role": userRole}
		rowAccessFilter, err := utils.GetRowAccessFilter(targetContainer, userRole, userMap)
		if err != nil {
			return nil, err
		}
		if rowAccessFilter != nil {
			if len(filter) > 0 {
				filter = bson.M{"$and": []bson.M{filter, rowAccessFilter}}
			} else {
				filter = rowAccessFilter
			}
		}
	}

	values, err := s.repository.Distinct(ctx, payload.TenantID, payload.ProjectID, targetSchema, field, filter)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"values": values, "count": len(values)}, nil
}

func (s *DynamicService) workflowSetVariable(step models.DynamicWorkflowStep, payload *workflowExecutionPayload) (interface{}, error) {
	ensureWorkflowPayloadMaps(payload)
	valuesConfig, ok := step.Config["values"]
	if !ok {
		valuesConfig = step.Config
	}
	values, ok := resolveWorkflowTemplates(valuesConfig, *payload).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("set_variable step requires values config")
	}
	for key, value := range values {
		payload.Variables[key] = value
	}
	return cloneWorkflowMap(payload.Variables), nil
}

type workflowBusinessError struct {
	Status  int
	Message string
}

func (e *workflowBusinessError) Error() string {
	return e.Message
}

func workflowFail(step models.DynamicWorkflowStep, payload workflowExecutionPayload) error {
	message := "workflow validation failed"
	if configuredMessage, ok := resolveWorkflowTemplates(step.Config["message"], payload).(string); ok && strings.TrimSpace(configuredMessage) != "" {
		message = configuredMessage
	}
	return &workflowBusinessError{
		Status:  workflowIntConfig(step.Config, "status", 400),
		Message: message,
	}
}

func workflowSetRecord(step models.DynamicWorkflowStep, payload *workflowExecutionPayload) (interface{}, error) {
	if payload.Record == nil {
		return nil, fmt.Errorf("set_record step requires record context")
	}
	values, err := workflowResolvedValues(step, *payload)
	if err != nil {
		return nil, err
	}
	for path, value := range values {
		if err := workflowSetPath(payload.Record, path, value); err != nil {
			return nil, err
		}
	}
	return cloneWorkflowMap(payload.Record), nil
}

func workflowTransform(step models.DynamicWorkflowStep, payload *workflowExecutionPayload) (interface{}, error) {
	ensureWorkflowPayloadMaps(payload)
	values, err := workflowResolvedValues(step, *payload)
	if err != nil {
		return nil, err
	}

	target := workflowStringConfig(step.Config, "target", "vars")
	switch target {
	case "vars", "variables":
		for path, value := range values {
			if err := workflowSetPath(payload.Variables, path, value); err != nil {
				return nil, err
			}
		}
		return cloneWorkflowMap(payload.Variables), nil
	case "record":
		if payload.Record == nil {
			return nil, fmt.Errorf("transform record target requires record context")
		}
		for path, value := range values {
			if err := workflowSetPath(payload.Record, path, value); err != nil {
				return nil, err
			}
		}
		return cloneWorkflowMap(payload.Record), nil
	default:
		return nil, fmt.Errorf("transform target must be vars or record")
	}
}

func workflowEquation(step models.DynamicWorkflowStep, payload *workflowExecutionPayload) (interface{}, error) {
	ensureWorkflowPayloadMaps(payload)
	valuesConfig, ok := workflowObjectConfig(step.Config, "values")
	if !ok {
		return nil, fmt.Errorf("equation step requires values config")
	}

	targetName := workflowStringConfig(step.Config, "target", "vars")
	var target map[string]interface{}
	switch targetName {
	case "vars", "variables":
		target = payload.Variables
	case "record":
		if payload.Record == nil {
			return nil, fmt.Errorf("equation record target requires record context")
		}
		target = payload.Record
	default:
		return nil, fmt.Errorf("equation target must be vars or record")
	}

	for path, rawEquation := range valuesConfig {
		resolvedEquation := resolveWorkflowTemplates(rawEquation, *payload)
		equation, ok := resolvedEquation.(string)
		if !ok || strings.TrimSpace(equation) == "" {
			return nil, fmt.Errorf("equation for %s must resolve to a non-empty string", path)
		}
		value, err := utils.EvaluateEquationWithContext(equation, &utils.EquationContext{
			TenantID:  payload.TenantID,
			ProjectID: payload.ProjectID,
			Data:      workflowEquationData(*payload),
		})
		if err != nil {
			return nil, fmt.Errorf("error evaluating workflow equation for %s: %w", path, err)
		}
		if err := workflowSetPath(target, path, value); err != nil {
			return nil, err
		}
	}
	return cloneWorkflowMap(target), nil
}

func workflowObjectConfig(config map[string]interface{}, key string) (map[string]interface{}, bool) {
	switch typed := config[key].(type) {
	case map[string]interface{}:
		return typed, true
	case bson.M:
		return map[string]interface{}(typed), true
	default:
		return nil, false
	}
}

func workflowEquationData(payload workflowExecutionPayload) map[string]interface{} {
	data := cloneWorkflowMap(payload.Record)
	if data == nil {
		data = map[string]interface{}{}
	}
	for key, value := range payload.Variables {
		data[key] = value
	}
	data["record"] = payload.Record
	data["vars"] = payload.Variables
	data["loop"] = payload.Loop
	data["steps"] = payload.StepOutputs
	return data
}

func (s *DynamicService) workflowUpdateArray(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	targetSchema := workflowTargetSchema(step, payload.SchemaName)
	filter, ok := resolveWorkflowTemplates(step.Config["filter"], payload).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s step requires filter config", step.Type)
	}
	field := workflowStringConfig(step.Config, "field", "")
	if field == "" || strings.HasPrefix(field, "$") {
		return nil, fmt.Errorf("%s step requires valid field config", step.Type)
	}
	value, exists := step.Config["value"]
	if !exists {
		return nil, fmt.Errorf("%s step requires value config", step.Type)
	}
	value = resolveWorkflowTemplates(value, payload)
	normalizeWorkflowIDs(filter)
	normalizeWorkflowIDs(value)

	operator, err := workflowArrayUpdateOperator(step)
	if err != nil {
		return nil, err
	}
	if (operator == "$pullAll" || operator == "$set") && !workflowArrayValue(value) {
		return nil, fmt.Errorf("%s step value must resolve to an array", step.Type)
	}
	result, err := s.repository.GetCollection(payload.TenantID, payload.ProjectID, targetSchema).
		UpdateMany(ctx, bson.M(filter), bson.M{operator: bson.M{field: value}})
	if err != nil {
		return nil, err
	}
	output := map[string]interface{}{
		"matchedCount":  result.MatchedCount,
		"modifiedCount": result.ModifiedCount,
	}
	targetContainer, err := s.workflowTargetContainer(ctx, payload, targetSchema)
	if err != nil {
		return nil, err
	}
	if err := s.insertDynamicPostWrite(ctx, payload.TenantID, payload.ProjectID, targetSchema, models.DynamicOutboxOperationUpdate, payload.UserID, targetContainer,
		buildDynamicAuditLog(payload.TenantID, payload.ProjectID, targetSchema, models.DynamicOutboxOperationUpdate, payload.AuditUser, map[string]interface{}{"filter": filter}, map[string]interface{}{"arrayUpdate": bson.M{"operator": operator, "field": field, "value": value}, "result": output})); err != nil {
		return nil, err
	}
	return output, nil
}

func workflowArrayUpdateOperator(step models.DynamicWorkflowStep) (string, error) {
	switch step.Type {
	case models.WorkflowStepTypeAppendArray:
		if workflowStringConfig(step.Config, "mode", "") == "push" {
			return "$push", nil
		}
		return "$addToSet", nil
	case models.WorkflowStepTypeRemoveArray, models.WorkflowStepTypePull:
		return "$pull", nil
	case models.WorkflowStepTypeAddToSet:
		return "$addToSet", nil
	case models.WorkflowStepTypePush:
		return "$push", nil
	case models.WorkflowStepTypePullAll:
		return "$pullAll", nil
	case models.WorkflowStepTypeSetArray:
		return "$set", nil
	default:
		return "", fmt.Errorf("unsupported array update step type: %s", step.Type)
	}
}

func workflowArrayValue(value interface{}) bool {
	_, ok := workflowInterfaceSlice(value)
	return ok
}

func workflowResolvedValues(step models.DynamicWorkflowStep, payload workflowExecutionPayload) (map[string]interface{}, error) {
	valuesConfig, ok := step.Config["values"]
	if !ok {
		return nil, fmt.Errorf("%s step requires values config", step.Type)
	}
	values, ok := resolveWorkflowTemplates(valuesConfig, payload).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s values config must resolve to an object", step.Type)
	}
	return values, nil
}

func workflowSetPath(target map[string]interface{}, path string, value interface{}) error {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 || parts[0] == "" {
		return fmt.Errorf("workflow target path is required")
	}
	current := target
	for _, part := range parts[:len(parts)-1] {
		if part == "" || strings.HasPrefix(part, "$") {
			return fmt.Errorf("invalid workflow target path: %s", path)
		}
		next, ok := current[part]
		if !ok {
			child := map[string]interface{}{}
			current[part] = child
			current = child
			continue
		}
		child, ok := next.(map[string]interface{})
		if !ok {
			return fmt.Errorf("workflow target path is not an object: %s", path)
		}
		current = child
	}
	last := parts[len(parts)-1]
	if last == "" || strings.HasPrefix(last, "$") {
		return fmt.Errorf("invalid workflow target path: %s", path)
	}
	current[last] = value
	return nil
}

func (s *DynamicService) workflowIf(ctx context.Context, step models.DynamicWorkflowStep, payload *workflowExecutionPayload) (interface{}, error) {
	branches := step.Branches
	if len(branches) == 0 {
		branches = []models.WorkflowBranch{
			{Name: "if", Conditions: step.Conditions, Steps: step.Steps},
		}
		if len(step.ElseSteps) > 0 {
			branches = append(branches, models.WorkflowBranch{Name: "else", Steps: step.ElseSteps})
		}
	}

	for _, branch := range branches {
		if len(branch.Conditions) > 0 && !workflowConditionsMatch(branch.Conditions, *payload) {
			continue
		}
		if err := s.runWorkflowStepList(ctx, payload, payload.WorkflowName, payload.WorkflowMode, workflowNestedExecutionMode(*payload), payload.StopOnError, branch.Steps); err != nil {
			return nil, err
		}
		return map[string]interface{}{"branch": branch.Name}, nil
	}

	return map[string]interface{}{"branch": nil}, nil
}

func (s *DynamicService) workflowForEach(ctx context.Context, step models.DynamicWorkflowStep, payload *workflowExecutionPayload) (interface{}, error) {
	ensureWorkflowPayloadMaps(payload)
	itemsValue := resolveWorkflowTemplates(step.Config["items"], *payload)
	items, ok := workflowSlice(itemsValue)
	if !ok {
		return nil, fmt.Errorf("for_each step requires items config resolving to an array")
	}

	maxItems := workflowIntConfig(step.Config, "maxItems", maxWorkflowLoopItems)
	if maxItems <= 0 || maxItems > maxWorkflowLoopItems {
		return nil, fmt.Errorf("for_each maxItems must be between 1 and %d", maxWorkflowLoopItems)
	}
	if len(items) > maxItems {
		return nil, fmt.Errorf("for_each item limit exceeded: got %d max %d", len(items), maxItems)
	}

	itemName := workflowStringConfig(step.Config, "itemName", "item")
	outputs := make([]interface{}, 0, len(items))
	for index, item := range items {
		loopPayload := *payload
		loopPayload.Loop = cloneWorkflowMap(payload.Loop)
		loopPayload.Loop[itemName] = item
		loopPayload.Loop["item"] = item
		loopPayload.Loop["index"] = index
		if err := s.runWorkflowStepList(ctx, &loopPayload, payload.WorkflowName, payload.WorkflowMode, workflowNestedExecutionMode(*payload), payload.StopOnError, step.Steps); err != nil {
			return nil, err
		}
		if loopPayload.HasReturn {
			payload.ReturnValue = loopPayload.ReturnValue
			payload.HasReturn = true
			return loopPayload.ReturnValue, nil
		}
		outputs = append(outputs, map[string]interface{}{"index": index})
	}

	return map[string]interface{}{
		"count": len(items),
		"items": outputs,
	}, nil
}

func (s *DynamicService) workflowExecuteWorkflow(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	if payload.WorkflowDepth >= maxWorkflowDepth {
		return nil, fmt.Errorf("workflow depth limit exceeded: max %d", maxWorkflowDepth)
	}

	targetSchema := workflowTargetSchema(step, payload.SchemaName)
	workflowName := workflowStringConfig(step.Config, "workflowName", "")
	if workflowName == "" {
		workflowName = workflowStringConfig(step.Config, "name", "")
	}
	if workflowName == "" {
		return nil, fmt.Errorf("execute_workflow step requires workflowName config")
	}

	record := payload.Record
	if recordConfig, ok := step.Config["record"]; ok {
		resolvedRecord, ok := resolveWorkflowTemplates(recordConfig, payload).(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("execute_workflow record config must resolve to an object")
		}
		record = resolvedRecord
	}
	oldRecord := payload.OldRecord
	if oldRecordConfig, ok := step.Config["oldRecord"]; ok {
		resolvedOldRecord, ok := resolveWorkflowTemplates(oldRecordConfig, payload).(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("execute_workflow oldRecord config must resolve to an object")
		}
		oldRecord = resolvedOldRecord
	}

	targetContainer, err := s.workflowTargetContainer(ctx, payload, targetSchema)
	if err != nil {
		return nil, err
	}
	workflow, found := findWorkflow(targetContainer, workflowName)
	if !found {
		return nil, fmt.Errorf("workflow not found: %s", workflowName)
	}
	if !workflow.IsActive {
		return nil, fmt.Errorf("workflow is disabled: %s", workflowName)
	}

	childPayload := payload
	childPayload.SchemaName = targetSchema
	childPayload.WorkflowName = workflow.Name
	childPayload.Record = cloneWorkflowMap(record)
	childPayload.OldRecord = cloneWorkflowMap(oldRecord)
	childPayload.StepOutputs = cloneWorkflowMap(payload.StepOutputs)
	childPayload.Container = targetContainer
	childPayload.WorkflowDepth = payload.WorkflowDepth + 1
	if err := s.runWorkflowDefinition(ctx, &childPayload, workflow); err != nil {
		return nil, err
	}
	return workflowExecutionReturnValue(workflow, &childPayload), nil
}

func (s *DynamicService) workflowExecuteDynamicAPI(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	targetSchema := workflowTargetSchema(step, payload.SchemaName)
	apiName := workflowStringConfig(step.Config, "apiName", "")
	if apiName == "" {
		apiName = workflowStringConfig(step.Config, "name", "")
	}
	if apiName == "" {
		return nil, fmt.Errorf("execute_dynamic_api step requires apiName config")
	}
	body := map[string]interface{}{}
	if bodyConfig, ok := step.Config["body"]; ok {
		resolvedBody, ok := resolveWorkflowTemplates(bodyConfig, payload).(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("execute_dynamic_api body config must resolve to an object")
		}
		body = resolvedBody
	}
	targetContainer, err := s.workflowTargetContainer(ctx, payload, targetSchema)
	if err != nil {
		return nil, err
	}
	result, err := s.ExecuteDynamicAPI(ctx, ExecuteDynamicAPIInput{
		TenantID:  payload.TenantID,
		ProjectID: payload.ProjectID,
		Schema:    targetSchema,
		APIName:   apiName,
		Body:      body,
		Container: targetContainer,
	})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"message": result.Message,
		"data":    result.Data,
		"source":  result.Source,
	}, nil
}

func (s *DynamicService) workflowQueryDynamicAPI(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	targetSchema := workflowTargetSchema(step, payload.SchemaName)
	apiName := workflowStringConfig(step.Config, "apiName", "")
	if apiName == "" {
		return nil, fmt.Errorf("query_dynamic_api step requires apiName config")
	}

	targetContainer, err := s.workflowTargetContainer(ctx, payload, targetSchema)
	if err != nil {
		return nil, err
	}
	dynamicAPI, found := findDynamicAPI(targetContainer, apiName)
	if !found {
		return nil, fmt.Errorf("query_dynamic_api API not found: %s", apiName)
	}
	if !dynamicAPI.IsActive {
		return nil, fmt.Errorf("query_dynamic_api API is disabled: %s", apiName)
	}
	if !strings.EqualFold(strings.TrimSpace(dynamicAPI.Method), http.MethodGet) {
		return nil, fmt.Errorf("query_dynamic_api API %s must use GET", apiName)
	}

	return s.workflowExecuteDynamicAPI(ctx, step, payload)
}

func workflowJoinArrays(step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	items, ok := workflowSlice(resolveWorkflowTemplates(step.Config["items"], payload))
	if !ok {
		return nil, fmt.Errorf("join_arrays items must resolve to an array")
	}
	lookupItems, ok := workflowSlice(resolveWorkflowTemplates(step.Config["lookupItems"], payload))
	if !ok {
		return nil, fmt.Errorf("join_arrays lookupItems must resolve to an array")
	}

	localField := workflowStringConfig(step.Config, "localField", "")
	lookupField := workflowStringConfig(step.Config, "lookupField", "")
	mappings, ok := workflowObjectConfig(step.Config, "mappings")
	if localField == "" || lookupField == "" || !ok || len(mappings) == 0 {
		return nil, fmt.Errorf("join_arrays requires localField, lookupField, and mappings")
	}
	fallbacks, _ := resolveWorkflowTemplates(step.Config["fallbacks"], payload).(map[string]interface{})

	lookupByKey := make(map[string]map[string]interface{}, len(lookupItems))
	for index, rawItem := range lookupItems {
		item, ok := workflowJoinObject(rawItem)
		if !ok {
			return nil, fmt.Errorf("join_arrays lookupItems[%d] must be an object", index)
		}
		value, exists := workflowPathValue(item, lookupField)
		key, valid := workflowJoinKey(value)
		if !exists || !valid {
			continue
		}
		if _, duplicate := lookupByKey[key]; !duplicate {
			lookupByKey[key] = item
		}
	}

	enriched := make([]map[string]interface{}, 0, len(items))
	for index, rawItem := range items {
		item, ok := workflowJoinObject(rawItem)
		if !ok {
			return nil, fmt.Errorf("join_arrays items[%d] must be an object", index)
		}
		next := workflowCloneJoinValue(item).(map[string]interface{})

		var matched map[string]interface{}
		if value, exists := workflowPathValue(item, localField); exists {
			if key, valid := workflowJoinKey(value); valid {
				matched = lookupByKey[key]
			}
		}

		for destination, rawSource := range mappings {
			source, ok := rawSource.(string)
			if !ok || strings.TrimSpace(source) == "" {
				return nil, fmt.Errorf("join_arrays mapping %s must reference a source field", destination)
			}
			value := interface{}(nil)
			found := false
			if matched != nil {
				value, found = workflowPathValue(matched, source)
			}
			if !found {
				value = fallbacks[destination]
			}
			if err := workflowSetPath(next, destination, workflowCloneJoinValue(value)); err != nil {
				return nil, fmt.Errorf("join_arrays destination %s: %w", destination, err)
			}
		}
		enriched = append(enriched, next)
	}

	return map[string]interface{}{"items": enriched, "count": len(enriched)}, nil
}

func workflowJoinObject(value interface{}) (map[string]interface{}, bool) {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed, true
	case bson.M:
		return map[string]interface{}(typed), true
	default:
		return nil, false
	}
}

func workflowJoinKey(value interface{}) (string, bool) {
	switch typed := value.(type) {
	case string:
		if objectID, err := primitive.ObjectIDFromHex(typed); err == nil {
			return "objectId:" + objectID.Hex(), true
		}
		return "string:" + typed, true
	case bool:
		return "bool:" + strconv.FormatBool(typed), true
	case primitive.ObjectID:
		return "objectId:" + typed.Hex(), true
	}
	if number, ok := workflowNumber(value); ok {
		return "number:" + strconv.FormatFloat(number, 'g', -1, 64), true
	}
	return "", false
}

func workflowCloneJoinValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		cloned := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			cloned[key] = workflowCloneJoinValue(item)
		}
		return cloned
	case bson.M:
		cloned := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			cloned[key] = workflowCloneJoinValue(item)
		}
		return cloned
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for index, item := range typed {
			cloned[index] = workflowCloneJoinValue(item)
		}
		return cloned
	case []map[string]interface{}:
		cloned := make([]map[string]interface{}, len(typed))
		for index, item := range typed {
			cloned[index] = workflowCloneJoinValue(item).(map[string]interface{})
		}
		return cloned
	case []bson.M:
		cloned := make([]map[string]interface{}, len(typed))
		for index, item := range typed {
			cloned[index] = workflowCloneJoinValue(item).(map[string]interface{})
		}
		return cloned
	default:
		return value
	}
}

func (s *DynamicService) workflowAuditLog(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	action := workflowStringConfig(step.Config, "action", "workflow:"+step.Name)
	auditLog := buildDynamicAuditLog(payload.TenantID, payload.ProjectID, payload.SchemaName, action, payload.AuditUser, payload.OldRecord, payload.Record)
	if payload.OutboxEventID != primitive.NilObjectID {
		auditLog.EventID = payload.OutboxEventID
	}
	if err := utils.LogAudit(ctx, *auditLog); err != nil {
		return nil, err
	}
	return auditLog, nil
}

func (s *DynamicService) workflowInvalidateCache(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	schemas := []string{payload.SchemaName}
	if configuredSchemas, ok := step.Config["schemas"].([]interface{}); ok {
		schemas = schemas[:0]
		for _, schema := range configuredSchemas {
			if schemaName, ok := schema.(string); ok {
				schemas = append(schemas, schemaName)
			}
		}
	}
	for _, schemaName := range uniqueSchemaNames(schemas) {
		if err := utils.IncrementSchemaCacheVersion(ctx, payload.TenantID, payload.ProjectID, schemaName); err != nil {
			return nil, err
		}
	}
	return map[string]interface{}{"schemas": uniqueSchemaNames(schemas)}, nil
}

func (s *DynamicService) workflowCreateNotification(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	config := resolveWorkflowTemplates(step.Config, payload)
	document, ok := config.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("create_notification config must resolve to an object")
	}

	title := strings.TrimSpace(workflowStringValue(document["title"]))
	message := strings.TrimSpace(workflowStringValue(document["message"]))
	if title == "" {
		return nil, fmt.Errorf("create_notification title is required")
	}
	if message == "" {
		return nil, fmt.Errorf("create_notification message is required")
	}

	notificationType := strings.TrimSpace(workflowStringValue(document["type"]))
	if notificationType == "" {
		notificationType = models.NotificationTypeInfo
	}
	notificationType = models.NormalizeNotificationType(notificationType)
	if !models.IsValidNotificationType(notificationType) {
		return nil, fmt.Errorf("create_notification type must be one of: info, warning, error, success")
	}
	expireAt, _ := workflowTime(document["expireAt"])

	notification := models.DynamicNotification{
		ID:            primitive.NewObjectID(),
		TenantID:      payload.TenantID,
		ProjectID:     payload.ProjectID,
		Title:         title,
		Message:       message,
		Type:          notificationType,
		Event:         strings.TrimSpace(workflowStringValue(document["event"])),
		SchemaName:    strings.TrimSpace(workflowStringValue(document["schemaName"])),
		RecordID:      strings.TrimSpace(workflowStringValue(document["recordId"])),
		SelectedUsers: workflowStringSliceValue(document["selectedUsers"]),
		SelectedRoles: workflowStringSliceValue(document["selectedRoles"]),
		SeenBy:        []string{},
		DeletedBy:     []string{},
		CreatedBy:     payload.UserID,
		CreatedAt:     time.Now(),
		IsActive:      true,
	}
	if !expireAt.IsZero() {
		notification.ExpireAt = &expireAt
	}
	if notification.SchemaName == "" {
		notification.SchemaName = payload.SchemaName
	}

	if err := s.repository.EnsureNotificationIndexes(ctx, payload.TenantID, payload.ProjectID); err != nil {
		return nil, err
	}
	if _, err := s.repository.InsertNotification(ctx, notification); err != nil {
		return nil, err
	}

	ws.EmitNotificationChanged(payload.UserID, payload.TenantID, payload.ProjectID)
	return notification, nil
}

func (s *DynamicService) workflowCallAPI(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	if step.TimeoutSec < minWorkflowCallAPITimeoutSec {
		return nil, fmt.Errorf("call_api step requires timeoutSec >= %d", minWorkflowCallAPITimeoutSec)
	}
	method, err := workflowAllowedHTTPMethod(workflowStringConfig(step.Config, "method", "POST"))
	if err != nil {
		return nil, err
	}
	url := workflowStringConfig(step.Config, "url", "")
	if url == "" {
		return nil, fmt.Errorf("call_api step requires url config")
	}
	if err := validateWorkflowCallAPIURL(ctx, url); err != nil {
		return nil, err
	}
	body := resolveWorkflowTemplates(step.Config["body"], payload)
	requestBody, _ := json.Marshal(body)
	callAttrs := append(observability.WorkflowAttrs(payload.TenantID, payload.ProjectID, payload.SchemaName, payload.WorkflowName),
		slog.String("step_name", step.Name),
		slog.String("method", method),
		slog.String("url", url),
		slog.String("request_body", workflowLogPreview(requestBody, 4000)),
	)
	observability.InfoCtx(ctx, "workflow call_api request", callAttrs...)
	responseBytes, statusCode, err := utils.ExecuteApiRequestWithStatus(ctx, method, url, body)
	if err != nil {
		observability.ErrorCtx(ctx, "workflow call_api request failed", err, callAttrs...)
		return nil, err
	}
	observability.InfoCtx(ctx, "workflow call_api response",
		append(callAttrs,
			slog.Int("response_status", statusCode),
			slog.String("response_body", workflowLogPreview(responseBytes, 4000)),
		)...,
	)
	if err := workflowCallAPIStatusError(statusCode, responseBytes); err != nil {
		return nil, err
	}
	var response interface{}
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		return string(responseBytes), nil
	}
	return response, nil
}

func workflowCallAPIStatusError(statusCode int, responseBytes []byte) error {
	if statusCode >= 200 && statusCode < 300 {
		return nil
	}
	return fmt.Errorf("call_api returned status %d: %s", statusCode, workflowLogPreview(responseBytes, 1000))
}

func workflowLogPreview(value []byte, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return string(value)
	}
	return fmt.Sprintf("%s...(truncated %d bytes)", string(value[:maxBytes]), len(value)-maxBytes)
}

func (s *DynamicService) workflowRunPipeline(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	return s.workflowAggregate(ctx, step, payload)
}

func (s *DynamicService) workflowAggregate(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	targetSchema := workflowTargetSchema(step, payload.SchemaName)
	pipelineJSON, err := workflowPipelineJSONConfig(step, payload)
	if err != nil {
		return nil, err
	}
	result, err := s.repository.ExecutePipeline(ctx, payload.TenantID, payload.ProjectID, targetSchema, models.PipelineStage{
		Name:         step.Name,
		PipelineJSON: pipelineJSON,
		IsActive:     true,
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *DynamicService) workflowEmitOutboxEvent(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	if err := s.enqueueWorkflowStep(ctx, payload, payload.WorkflowName, step); err != nil {
		return nil, err
	}
	return map[string]interface{}{"idempotencyKey": workflowStepIdempotencyKey(payload, payload.WorkflowName, step)}, nil
}

func (s *DynamicService) enqueueWorkflowStep(ctx context.Context, payload workflowExecutionPayload, workflowName string, step models.DynamicWorkflowStep) error {
	if step.Type == models.WorkflowStepTypeEmitOutboxEvent {
		return fmt.Errorf("emit_outbox_event steps cannot run in outbox execution mode")
	}
	if payload.OutboxEvents != nil {
		if *payload.OutboxEvents >= maxWorkflowOutboxEvents {
			return fmt.Errorf("workflow outbox event limit exceeded: max %d", maxWorkflowOutboxEvents)
		}
		*payload.OutboxEvents++
	}
	event := buildWorkflowStepOutboxEvent(payload, workflowName, step)
	event.Payload.WorkflowDepth = payload.WorkflowDepth + 1
	if _, err := s.repository.InsertOutboxEvent(ctx, event); err != nil {
		return err
	}
	observability.InfoCtx(ctx, "workflow outbox step queued",
		append(observability.WorkflowAttrs(payload.TenantID, payload.ProjectID, payload.SchemaName, workflowName),
			slog.String("step_name", step.Name),
			slog.String(observability.FieldStepType, step.Type),
			slog.String("outbox_event_id", event.ID.Hex()),
			slog.String("idempotency_key", event.Payload.IdempotencyKey),
		)...,
	)
	return nil
}

func buildWorkflowStepOutboxEvent(payload workflowExecutionPayload, workflowName string, step models.DynamicWorkflowStep) models.DynamicOutboxEvent {
	now := time.Now()
	maxAttempts := dynamicOutboxMaxAttempts
	if step.MaxAttempts > 0 {
		maxAttempts = step.MaxAttempts
	} else if step.RetryCount > 0 {
		maxAttempts = step.RetryCount
	}
	idempotencyKey := workflowStepIdempotencyKey(payload, workflowName, step)
	return models.DynamicOutboxEvent{
		ID:            primitive.NewObjectID(),
		TenantID:      payload.TenantID,
		ProjectID:     payload.ProjectID,
		SchemaName:    payload.SchemaName,
		Operation:     models.DynamicOutboxOperationWorkflowStep,
		Status:        models.DynamicOutboxStatusPending,
		MaxAttempts:   maxAttempts,
		NextAttemptAt: primitive.NewDateTimeFromTime(now),
		CreatedAt:     primitive.NewDateTimeFromTime(now),
		UpdatedAt:     primitive.NewDateTimeFromTime(now),
		Payload: models.DynamicOutboxPayload{
			UserID:          payload.UserID,
			WorkflowName:    workflowName,
			WorkflowTrigger: payload.WorkflowTrigger,
			WorkflowVersion: payload.WorkflowVersion,
			StepID:          step.ID,
			StepName:        step.Name,
			StepType:        step.Type,
			StepTimeoutSec:  step.TimeoutSec,
			WorkflowDepth:   payload.WorkflowDepth + 1,
			TargetSchema:    step.TargetSchema,
			Record:          payload.Record,
			OldRecord:       payload.OldRecord,
			StepOutputs:     cloneWorkflowMap(payload.StepOutputs),
			Variables:       cloneWorkflowMap(payload.Variables),
			Loop:            cloneWorkflowMap(payload.Loop),
			Config:          step.Config,
			Steps:           step.Steps,
			ElseSteps:       step.ElseSteps,
			Branches:        step.Branches,
			IdempotencyKey:  idempotencyKey,
		},
	}
}

func processWorkflowOutboxStep(ctx context.Context, repository *repositories.DynamicRepository, event *models.DynamicOutboxEvent) error {
	ctx, span := observability.StartSpan(ctx, "workflow.step", append(observability.WorkflowTraceAttrs(event.TenantID, event.ProjectID, event.SchemaName, event.Payload.WorkflowName),
		attribute.String(observability.FieldOperation, "workflow_outbox_step_execute"),
		attribute.String(observability.FieldStepType, event.Payload.StepType),
	)...)
	start := time.Now()
	status := "success"
	var spanErr error
	defer func() {
		duration := time.Since(start)
		observability.RecordWorkflowStepExecution(event.TenantID, event.ProjectID, event.Payload.WorkflowName, event.Payload.StepType, status, duration)
		attrs := append(observability.WorkflowAttrs(event.TenantID, event.ProjectID, event.SchemaName, event.Payload.WorkflowName),
			observability.OperationAttrs("workflow_outbox_step_execute", status, duration)...)
		attrs = append(attrs, slog.String(observability.FieldStepType, event.Payload.StepType))
		observability.DebugCtx(ctx, "workflow outbox step execution completed", attrs...)
		observability.EndSpan(span, status, spanErr)
	}()

	if event.Payload.StepType == models.WorkflowStepTypeEmitOutboxEvent {
		status = "error"
		err := fmt.Errorf("emit_outbox_event cannot be processed as an outbox workflow step")
		spanErr = err
		return err
	}
	if event.Payload.IdempotencyKey != "" {
		done, err := repository.WorkflowStepExecutionDone(ctx, event.Payload.IdempotencyKey)
		if err != nil {
			status = "error"
			spanErr = err
			return err
		}
		if done {
			status = "skipped"
			observability.InfoCtx(ctx, "workflow outbox step skipped",
				append(observability.WorkflowAttrs(event.TenantID, event.ProjectID, event.SchemaName, event.Payload.WorkflowName),
					slog.String("step_name", event.Payload.StepName),
					slog.String(observability.FieldStepType, event.Payload.StepType),
					slog.String("outbox_event_id", event.ID.Hex()),
					slog.String("idempotency_key", event.Payload.IdempotencyKey),
					slog.String("reason", "idempotency_key_already_done"),
				)...,
			)
			return nil
		}
	}

	service := &DynamicService{repository: repository}
	step := models.DynamicWorkflowStep{
		ID:           event.Payload.StepID,
		Name:         event.Payload.StepName,
		Type:         event.Payload.StepType,
		IsActive:     true,
		TargetSchema: event.Payload.TargetSchema,
		Config:       event.Payload.Config,
		Steps:        event.Payload.Steps,
		ElseSteps:    event.Payload.ElseSteps,
		Branches:     event.Payload.Branches,
		TimeoutSec:   event.Payload.StepTimeoutSec,
	}
	payload := workflowExecutionPayload{
		TenantID:        event.TenantID,
		ProjectID:       event.ProjectID,
		SchemaName:      event.SchemaName,
		WorkflowName:    event.Payload.WorkflowName,
		WorkflowTrigger: event.Payload.WorkflowTrigger,
		WorkflowVersion: event.Payload.WorkflowVersion,
		Record:          event.Payload.Record,
		OldRecord:       event.Payload.OldRecord,
		StepOutputs:     event.Payload.StepOutputs,
		Variables:       event.Payload.Variables,
		Loop:            event.Payload.Loop,
		UserID:          event.Payload.UserID,
		OutboxEventID:   event.ID,
		IdempotencyKey:  event.Payload.IdempotencyKey,
		WorkflowDepth:   event.Payload.WorkflowDepth,
	}
	observability.InfoCtx(ctx, "workflow outbox step started",
		append(observability.WorkflowAttrs(event.TenantID, event.ProjectID, event.SchemaName, event.Payload.WorkflowName),
			slog.String("step_name", event.Payload.StepName),
			slog.String(observability.FieldStepType, event.Payload.StepType),
			slog.String("outbox_event_id", event.ID.Hex()),
			slog.String("idempotency_key", event.Payload.IdempotencyKey),
			slog.Int("attempt", event.Attempts+1),
		)...,
	)
	_, err := service.processWorkflowStepWithTimeout(ctx, step, &payload)
	if err != nil {
		status = "error"
		spanErr = err
		return err
	}
	if event.Payload.IdempotencyKey != "" {
		if err := repository.MarkWorkflowStepExecutionDone(ctx, event.Payload.IdempotencyKey, event.ID); err != nil {
			status = "error"
			spanErr = err
			return err
		}
	}
	return nil
}

func workflowSupportsMode(workflowMode, executionMode string) bool {
	if workflowMode == "" {
		workflowMode = models.WorkflowModeTransactional
	}
	return workflowMode == executionMode || workflowMode == models.WorkflowModeHybrid
}

func workflowRequiresTransaction(workflow models.DynamicWorkflow) bool {
	return workflow.RunInTransaction || workflowStepsRequireTransaction(workflow.Steps, workflow.Mode)
}

func workflowStepsRequireTransaction(steps []models.DynamicWorkflowStep, workflowMode string) bool {
	for _, step := range steps {
		if !step.IsActive {
			continue
		}
		if workflowStepExecutionMode(step, workflowMode) == models.WorkflowModeOutbox || workflowStepWritesTransactionally(step.Type) {
			return true
		}
		if workflowStepsRequireTransaction(step.Steps, workflowMode) || workflowStepsRequireTransaction(step.ElseSteps, workflowMode) {
			return true
		}
		for _, branch := range step.Branches {
			if workflowStepsRequireTransaction(branch.Steps, workflowMode) {
				return true
			}
		}
	}
	return false
}

func workflowStepWritesTransactionally(stepType string) bool {
	switch stepType {
	case models.WorkflowStepTypeCreateRecord,
		models.WorkflowStepTypeUpdateRecord,
		models.WorkflowStepTypeUnsetRecord,
		models.WorkflowStepTypeDeleteRecord,
		models.WorkflowStepTypeAppendArray,
		models.WorkflowStepTypeRemoveArray,
		models.WorkflowStepTypeAddToSet,
		models.WorkflowStepTypePush,
		models.WorkflowStepTypePull,
		models.WorkflowStepTypePullAll,
		models.WorkflowStepTypeSetArray,
		models.WorkflowStepTypeAuditLog,
		models.WorkflowStepTypeEmitOutboxEvent,
		models.WorkflowStepTypeExecuteWorkflow:
		return true
	default:
		return false
	}
}

func validateWorkflowExecutionBounds(payload workflowExecutionPayload, workflow models.DynamicWorkflow) error {
	if payload.WorkflowDepth > maxWorkflowDepth {
		return fmt.Errorf("workflow depth limit exceeded: max %d", maxWorkflowDepth)
	}
	if countWorkflowSteps(workflow.Steps) > maxWorkflowSteps {
		return fmt.Errorf("workflow %s has too many steps: max %d", workflow.Name, maxWorkflowSteps)
	}
	return nil
}

func workflowContextWithTimeout(ctx context.Context, timeoutSec int) (context.Context, context.CancelFunc) {
	if timeoutSec <= 0 {
		return ctx, func() {}
	}
	childCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	if session := mongo.SessionFromContext(ctx); session != nil {
		childCtx = mongo.NewSessionContext(childCtx, session)
	}
	return childCtx, cancel
}

func workflowStepExecutionMode(step models.DynamicWorkflowStep, workflowMode string) string {
	if step.ExecutionMode != "" {
		return step.ExecutionMode
	}
	if workflowMode == models.WorkflowModeOutbox {
		return models.WorkflowModeOutbox
	}
	return models.WorkflowModeTransactional
}

func workflowTargetSchema(step models.DynamicWorkflowStep, fallback string) string {
	if strings.TrimSpace(step.TargetSchema) != "" {
		return step.TargetSchema
	}
	return fallback
}

func workflowStringifyObjectIDsForValidation(container *models.ContainerModel, item map[string]interface{}) {
	if container == nil || item == nil {
		return
	}
	for _, field := range container.Fields {
		value, ok := item[field.Name]
		if !ok || value == nil {
			continue
		}
		switch field.Type {
		case "objectId":
			if objectID, ok := value.(primitive.ObjectID); ok {
				item[field.Name] = objectID.Hex()
			}
		case "objectIdArray":
			item[field.Name] = workflowStringifyObjectIDArray(value)
		}
	}
}

func workflowStringifyObjectIDArray(value interface{}) interface{} {
	switch typed := value.(type) {
	case []primitive.ObjectID:
		result := make([]interface{}, 0, len(typed))
		for _, objectID := range typed {
			result = append(result, objectID.Hex())
		}
		return result
	case []interface{}:
		result := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			if objectID, ok := item.(primitive.ObjectID); ok {
				result = append(result, objectID.Hex())
				continue
			}
			result = append(result, item)
		}
		return result
	default:
		return value
	}
}

func workflowStepIdempotencyKey(payload workflowExecutionPayload, workflowName string, step models.DynamicWorkflowStep) string {
	if strings.TrimSpace(step.IdempotencyKey) != "" {
		resolved := resolveWorkflowTemplateString(step.IdempotencyKey, payload)
		customKey := strings.TrimSpace(fmt.Sprint(resolved))
		if customKey == "" {
			return ""
		}
		parts := []string{
			payload.TenantID,
			payload.ProjectID,
			payload.SchemaName,
			workflowName,
			workflowStepIdentifier(step),
			payload.WorkflowTrigger,
			strconv.Itoa(payload.WorkflowVersion),
		}
		if recordID := workflowRecordID(payload.Record); recordID != "" {
			parts = append(parts, recordID)
		}
		return strings.Join(append(parts, customKey), ":")
	}

	recordID := workflowRecordID(payload.Record)
	if recordID == "" {
		return ""
	}

	return strings.Join([]string{
		payload.TenantID,
		payload.ProjectID,
		payload.SchemaName,
		workflowName,
		workflowStepIdentifier(step),
		recordID,
		payload.WorkflowTrigger,
		strconv.Itoa(payload.WorkflowVersion),
	}, ":")
}

func workflowTrigger(workflow models.DynamicWorkflow) string {
	if workflow.Trigger == "" {
		return models.WorkflowTriggerManual
	}
	return workflow.Trigger
}

func workflowVersion(workflow models.DynamicWorkflow) int {
	if workflow.Version <= 0 {
		return 1
	}
	return workflow.Version
}

func workflowReturnValue(workflow models.DynamicWorkflow, stepOutputs map[string]interface{}) interface{} {
	if workflow.ReturnStep == "" {
		return stepOutputs
	}
	return stepOutputs[workflow.ReturnStep]
}

func workflowExecutionReturnValue(workflow models.DynamicWorkflow, payload *workflowExecutionPayload) interface{} {
	if payload.HasReturn {
		return payload.ReturnValue
	}
	return workflowReturnValue(workflow, payload.StepOutputs)
}

func workflowReturn(step models.DynamicWorkflowStep, payload *workflowExecutionPayload) (interface{}, error) {
	valueConfig, ok := step.Config["value"]
	if !ok {
		valueConfig = step.Config
	}
	value := resolveWorkflowTemplates(valueConfig, *payload)
	payload.ReturnValue = value
	payload.HasReturn = true
	return value, nil
}

func workflowStepIdentifier(step models.DynamicWorkflowStep) string {
	if strings.TrimSpace(step.ID) != "" {
		return step.ID
	}
	if strings.TrimSpace(step.Name) != "" {
		return step.Name
	}
	return fmt.Sprintf("%s:%d", step.Type, step.Order)
}

func workflowRecordID(record map[string]interface{}) string {
	if record == nil {
		return ""
	}
	for _, key := range []string{"_id", "id"} {
		value, ok := record[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case primitive.ObjectID:
			return typed.Hex()
		default:
			return strings.TrimSpace(fmt.Sprint(typed))
		}
	}
	return ""
}

func (s *DynamicService) workflowTargetContainer(ctx context.Context, payload workflowExecutionPayload, targetSchema string) (*models.ContainerModel, error) {
	if payload.Container != nil && payload.Container.SchemaName == targetSchema {
		return payload.Container, nil
	}
	return s.repository.GetContainerModel(ctx, payload.TenantID, payload.ProjectID, targetSchema)
}

func workflowStepHasNonTransactionalSideEffect(step models.DynamicWorkflowStep) bool {
	return step.Type == models.WorkflowStepTypeCallAPI ||
		step.Type == models.WorkflowStepTypeExecuteDynamicAPI ||
		step.Type == models.WorkflowStepTypeInvalidateCache ||
		step.Type == models.WorkflowStepTypeCreateNotification
}

func workflowStringValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
}

func workflowStringSliceValue(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return compactWorkflowStrings(typed)
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if str := strings.TrimSpace(workflowStringValue(item)); str != "" {
				result = append(result, str)
			}
		}
		return result
	default:
		if str := strings.TrimSpace(workflowStringValue(value)); str != "" {
			return []string{str}
		}
		return nil
	}
}

func compactWorkflowStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func validateWorkflowCallAPIURL(ctx context.Context, rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid call_api url: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("call_api url scheme must be http or https")
	}

	host := parsedURL.Hostname()
	if host == "" {
		return fmt.Errorf("call_api url host is required")
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return fmt.Errorf("call_api url host is not allowed")
	}
	if !workflowCallAPIHostAllowed(host) {
		return fmt.Errorf("call_api url host is not in WORKFLOW_CALL_API_ALLOWED_DOMAINS")
	}

	if ip := net.ParseIP(host); ip != nil {
		if workflowBlockedOutboundIP(ip) {
			return fmt.Errorf("call_api url resolves to a blocked network")
		}
		return nil
	}

	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("failed to resolve call_api host: %w", err)
	}
	if len(addresses) == 0 {
		return fmt.Errorf("call_api host did not resolve")
	}
	for _, address := range addresses {
		if workflowBlockedOutboundIP(address.IP) {
			return fmt.Errorf("call_api url resolves to a blocked network")
		}
	}
	return nil
}

func workflowAllowedHTTPMethod(method string) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
		return method, nil
	default:
		return "", fmt.Errorf("call_api method is not allowed: %s", method)
	}
}

func workflowCallAPIHostAllowed(host string) bool {
	configured := strings.TrimSpace(os.Getenv("WORKFLOW_CALL_API_ALLOWED_DOMAINS"))
	if configured == "" {
		return true
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, allowed := range strings.Split(configured, ",") {
		allowed = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(allowed), "."))
		if allowed != "" && (host == allowed || strings.HasSuffix(host, "."+allowed)) {
			return true
		}
	}
	return false
}

func workflowBlockedOutboundIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}

func findWorkflow(container *models.ContainerModel, workflowName string) (models.DynamicWorkflow, bool) {
	if container == nil || workflowName == "" {
		return models.DynamicWorkflow{}, false
	}
	for _, workflow := range container.Workflows {
		if workflow.Name == workflowName {
			return workflow, true
		}
	}
	return models.DynamicWorkflow{}, false
}

func workflowStringConfig(config map[string]interface{}, key, fallback string) string {
	if value, ok := config[key].(string); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

// workflowResolvedStringConfig is like workflowStringConfig but resolves
// template expressions (e.g. {{record.search}}) against the workflow payload.
func workflowResolvedStringConfig(config map[string]interface{}, key string, payload workflowExecutionPayload) string {
	raw, ok := config[key].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return ""
	}
	resolved := resolveWorkflowTemplates(raw, payload)
	if resolved == nil {
		return ""
	}
	if s, ok := resolved.(string); ok {
		normalized := strings.TrimSpace(s)
		if normalized == "<nil>" {
			return ""
		}
		return normalized
	}
	return fmt.Sprint(resolved)
}

func workflowFindRecordsSearchFields(config map[string]interface{}, payload workflowExecutionPayload) []string {
	raw, ok := config["search"]
	if !ok {
		return nil
	}
	resolved := resolveWorkflowTemplates(raw, payload)
	searchConfig, ok := workflowMapConfig(resolved)
	if !ok {
		return nil
	}
	return workflowStringSliceConfig(searchConfig["fields"])
}

func workflowStringSliceConfig(value interface{}) []string {
	var fields []string
	switch typed := value.(type) {
	case []interface{}:
		for _, item := range typed {
			if field := strings.TrimSpace(fmt.Sprint(item)); field != "" {
				fields = append(fields, field)
			}
		}
	case []string:
		for _, item := range typed {
			if field := strings.TrimSpace(item); field != "" {
				fields = append(fields, field)
			}
		}
	}
	return fields
}

func workflowFilterItemsBySearchFields(items []map[string]interface{}, searchKey string, fields []string) []map[string]interface{} {
	searchKey = strings.ToLower(strings.TrimSpace(searchKey))
	if searchKey == "" || len(fields) == 0 {
		return items
	}
	filtered := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		for _, field := range fields {
			if value, found := workflowPathValue(item, field); found && workflowSearchFieldValueMatches(value, searchKey) {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

func workflowSearchFieldValueMatches(value interface{}, searchKey string) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.Contains(strings.ToLower(typed), searchKey)
	case []interface{}:
		for _, item := range typed {
			if workflowSearchFieldValueMatches(item, searchKey) {
				return true
			}
		}
		return false
	case []map[string]interface{}:
		for _, item := range typed {
			if workflowSearchFieldValueMatches(item, searchKey) {
				return true
			}
		}
		return false
	case map[string]interface{}:
		for _, item := range typed {
			if workflowSearchFieldValueMatches(item, searchKey) {
				return true
			}
		}
		return false
	case bson.M:
		for _, item := range typed {
			if workflowSearchFieldValueMatches(item, searchKey) {
				return true
			}
		}
		return false
	default:
		return strings.Contains(strings.ToLower(fmt.Sprint(value)), searchKey)
	}
}

func workflowFindRecordsSearchKey(config map[string]interface{}, payload workflowExecutionPayload) string {
	searchKey := workflowResolvedStringConfig(config, "search", payload)
	if searchKey != "" {
		return searchKey
	}

	if raw, ok := config["search"]; ok {
		resolved := resolveWorkflowTemplates(raw, payload)
		if searchConfig, ok := workflowMapConfig(resolved); ok {
			if value, ok := searchConfig["value"]; ok {
				return workflowSearchValueString(value)
			}
		}
	}

	return workflowResolvedStringConfig(config, "searchKey", payload)
}

func workflowMapConfig(value interface{}) (map[string]interface{}, bool) {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed, true
	case bson.M:
		return map[string]interface{}(typed), true
	default:
		return nil, false
	}
}

func workflowSearchValueString(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		normalized := strings.TrimSpace(typed)
		if normalized == "<nil>" {
			return ""
		}
		return normalized
	default:
		normalized := strings.TrimSpace(fmt.Sprint(typed))
		if normalized == "<nil>" {
			return ""
		}
		return normalized
	}
}

func workflowIntConfig(config map[string]interface{}, key string, fallback int) int {
	value, ok := config[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func workflowBoolConfig(config map[string]interface{}, key string, fallback bool) bool {
	value, ok := config[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func workflowReadStepApplyRowAccess(step models.DynamicWorkflowStep, payload workflowExecutionPayload) bool {
	return workflowBoolConfig(step.Config, "applyRowAccess", payload.WorkflowTrigger == models.WorkflowTriggerManual)
}

func workflowCountRecordsApplyRowAccess(step models.DynamicWorkflowStep, payload workflowExecutionPayload) bool {
	return workflowReadStepApplyRowAccess(step, payload)
}

func workflowPipelineJSONConfig(step models.DynamicWorkflowStep, payload workflowExecutionPayload) (string, error) {
	var pipeline []interface{}
	if rawPipelineJSON, ok := step.Config["pipelineJson"]; ok {
		resolved := resolveWorkflowTemplates(rawPipelineJSON, payload)
		pipelineJSON, ok := resolved.(string)
		if !ok || strings.TrimSpace(pipelineJSON) == "" {
			return "", fmt.Errorf("%s step pipelineJson config must resolve to a non-empty string", step.Type)
		}
		if err := json.Unmarshal([]byte(pipelineJSON), &pipeline); err != nil {
			return "", fmt.Errorf("%s step pipelineJson config must resolve to a JSON array: %w", step.Type, err)
		}
	} else {
		rawPipeline, ok := step.Config["pipeline"]
		if !ok {
			return "", fmt.Errorf("%s step requires pipeline or pipelineJson config", step.Type)
		}
		resolved := resolveWorkflowTemplates(rawPipeline, payload)
		if pipelineJSON, ok := resolved.(string); ok {
			if strings.TrimSpace(pipelineJSON) == "" {
				return "", fmt.Errorf("%s step pipeline config must resolve to a non-empty string", step.Type)
			}
			if err := json.Unmarshal([]byte(pipelineJSON), &pipeline); err != nil {
				return "", fmt.Errorf("%s step pipeline config must resolve to a JSON array: %w", step.Type, err)
			}
		} else {
			var ok bool
			pipeline, ok = workflowPipelineSlice(resolved)
			if !ok {
				return "", fmt.Errorf("%s step pipeline config must resolve to an array", step.Type)
			}
		}
	}

	pipeline, err := workflowAggregatePipelineWithLimit(pipeline)
	if err != nil {
		return "", err
	}
	pipelineBytes, err := json.Marshal(pipeline)
	if err != nil {
		return "", fmt.Errorf("%s step pipeline config must resolve to JSON: %w", step.Type, err)
	}
	return string(pipelineBytes), nil
}

func workflowPipelineSlice(value interface{}) ([]interface{}, bool) {
	switch typed := value.(type) {
	case []interface{}:
		return typed, true
	case []map[string]interface{}:
		items := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
		return items, true
	case []bson.M:
		items := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			items = append(items, map[string]interface{}(item))
		}
		return items, true
	default:
		return nil, false
	}
}

func workflowAggregatePipelineWithLimit(pipeline []interface{}) ([]interface{}, error) {
	if err := validateWorkflowAggregatePipeline(pipeline); err != nil {
		return nil, err
	}
	hasLimit := false
	for _, rawStage := range pipeline {
		stage, _ := workflowPipelineStage(rawStage)
		if rawLimit, ok := stage["$limit"]; ok {
			hasLimit = true
			limit, ok := workflowNumber(rawLimit)
			if !ok || limit <= 0 || limit > maxWorkflowAggregateLimit || limit != float64(int64(limit)) {
				return nil, fmt.Errorf("aggregate $limit must be an integer between 1 and %d", maxWorkflowAggregateLimit)
			}
		}
		if rawSkip, ok := stage["$skip"]; ok {
			skip, ok := workflowNumber(rawSkip)
			if !ok || skip < 0 || skip > maxWorkflowAggregateSkip || skip != float64(int64(skip)) {
				return nil, fmt.Errorf("aggregate $skip must be an integer between 0 and %d", maxWorkflowAggregateSkip)
			}
		}
	}
	if hasLimit {
		return pipeline, nil
	}
	return append(append([]interface{}{}, pipeline...), map[string]interface{}{"$limit": maxWorkflowAggregateLimit}), nil
}

func validateWorkflowAggregatePipeline(pipeline []interface{}) error {
	if len(pipeline) == 0 {
		return fmt.Errorf("aggregate pipeline must include at least one stage")
	}
	allowedStages := map[string]bool{
		"$match":     true,
		"$group":     true,
		"$project":   true,
		"$sort":      true,
		"$limit":     true,
		"$skip":      true,
		"$count":     true,
		"$unwind":    true,
		"$addFields": true,
	}

	for _, rawStage := range pipeline {
		stage, ok := workflowPipelineStage(rawStage)
		if !ok || len(stage) != 1 {
			return fmt.Errorf("aggregate pipeline stage must be an object with one operator")
		}
		for op := range stage {
			if !allowedStages[op] {
				return fmt.Errorf("aggregate stage %s is not allowed", op)
			}
		}
	}
	return nil
}

func workflowPipelineStage(rawStage interface{}) (map[string]interface{}, bool) {
	switch typed := rawStage.(type) {
	case map[string]interface{}:
		return typed, true
	case bson.M:
		return map[string]interface{}(typed), true
	default:
		return nil, false
	}
}

func workflowNestedExecutionMode(payload workflowExecutionPayload) string {
	if payload.ExecutionMode != "" {
		return payload.ExecutionMode
	}
	return models.WorkflowModeTransactional
}

func workflowFilterConfig(config map[string]interface{}, payload workflowExecutionPayload, container *models.ContainerModel) (bson.M, bool, error) {
	if filtersConfig, ok := config["filters"]; ok {
		filters, ok := resolveWorkflowTemplates(filtersConfig, payload).(map[string]interface{})
		if !ok {
			return nil, true, fmt.Errorf("workflow filters config must resolve to an object")
		}
		filter, err := buildWorkflowFilterFromValues(container, filters)
		return filter, true, err
	}

	filterConfig, ok := config["filter"]
	if !ok {
		return nil, false, nil
	}
	filter, ok := resolveWorkflowTemplates(filterConfig, payload).(map[string]interface{})
	if !ok {
		return nil, true, fmt.Errorf("workflow filter config must resolve to an object")
	}
	return bson.M(filter), true, nil
}

func buildWorkflowFilterFromValues(container *models.ContainerModel, filters map[string]interface{}) (bson.M, error) {
	filter := bson.M{}
	if container == nil || len(filters) == 0 {
		return filter, nil
	}

	fieldsByName := make(map[string]models.Field, len(container.Fields))
	for _, field := range container.Fields {
		if !field.IsHashed {
			fieldsByName[field.Name] = field
		}
	}

	for key, rawValue := range filters {
		field, ok := fieldsByName[key]
		if !ok {
			continue
		}

		values := workflowFilterValues(rawValue)
		fieldFilter := bson.M{}
		simpleValues := make([]interface{}, 0, len(values))

		for _, value := range values {
			converted, err := utils.ConvertQueryValueToFieldType(field.Name, field.Type, fmt.Sprint(value))
			if err != nil {
				return nil, err
			}
			if condition, ok := converted.(bson.M); ok {
				for operator, conditionValue := range condition {
					fieldFilter[operator] = conditionValue
				}
				continue
			}
			simpleValues = append(simpleValues, converted)
		}

		if len(simpleValues) > 1 {
			filter[field.Name] = bson.M{"$in": simpleValues}
			continue
		}
		if len(simpleValues) == 1 {
			filter[field.Name] = simpleValues[0]
			continue
		}
		if len(fieldFilter) > 0 {
			filter[field.Name] = fieldFilter
		}
	}

	return filter, nil
}

func workflowFilterValues(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		return typed
	case []string:
		values := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			values = append(values, item)
		}
		return values
	default:
		return []interface{}{value}
	}
}

func workflowUserRole(payload workflowExecutionPayload) string {
	if payload.AuditUser == nil || len(payload.AuditUser.Roles) == 0 {
		return ""
	}
	return payload.AuditUser.Roles[0]
}

func workflowSlice(value interface{}) ([]interface{}, bool) {
	switch typed := value.(type) {
	case []interface{}:
		return typed, true
	case []map[string]interface{}:
		items := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
		return items, true
	case []bson.M:
		items := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			items = append(items, map[string]interface{}(item))
		}
		return items, true
	default:
		return nil, false
	}
}

func workflowHasUpdateOperator(update map[string]interface{}) bool {
	for key := range update {
		if strings.HasPrefix(key, "$") {
			return true
		}
	}
	return false
}

func validateWorkflowUpdateOperators(update map[string]interface{}) error {
	if err := validateWorkflowUpdateOperatorNames(update); err != nil {
		return err
	}
	for operator, value := range update {
		var fields map[string]interface{}
		switch typed := value.(type) {
		case map[string]interface{}:
			fields = typed
		case bson.M:
			fields = map[string]interface{}(typed)
		default:
			return fmt.Errorf("update_record operator %s requires an object value", operator)
		}
		if len(fields) == 0 {
			return fmt.Errorf("update_record operator %s requires at least one field", operator)
		}
		for field := range fields {
			if !workflowValidUpdateField(field) {
				return fmt.Errorf("update_record operator %s has invalid field: %s", operator, field)
			}
		}
	}
	return nil
}

func validateWorkflowUpdateOperatorNames(update map[string]interface{}) error {
	if len(update) == 0 {
		return fmt.Errorf("update_record update cannot be empty")
	}
	allowed := map[string]bool{
		"$set":      true,
		"$unset":    true,
		"$inc":      true,
		"$addToSet": true,
		"$push":     true,
		"$pull":     true,
	}
	for operator := range update {
		if !allowed[operator] {
			return fmt.Errorf("update_record operator is not allowed: %s", operator)
		}
	}
	return nil
}

func workflowUnsetValues(config map[string]interface{}) (map[string]interface{}, error) {
	if values, ok := workflowObjectConfig(config, "values"); ok && len(values) > 0 {
		for field := range values {
			if !workflowValidUpdateField(field) {
				return nil, fmt.Errorf("unset_record has invalid field: %s", field)
			}
		}
		return values, nil
	}
	if field := workflowStringConfig(config, "field", ""); field != "" {
		if !workflowValidUpdateField(field) {
			return nil, fmt.Errorf("unset_record has invalid field: %s", field)
		}
		return map[string]interface{}{field: ""}, nil
	}
	fields, ok := workflowInterfaceSlice(config["fields"])
	if !ok || len(fields) == 0 {
		return nil, fmt.Errorf("unset_record step requires field, fields, or values config")
	}
	values := make(map[string]interface{}, len(fields))
	for _, field := range fields {
		fieldName, ok := field.(string)
		if !ok || strings.TrimSpace(fieldName) == "" {
			return nil, fmt.Errorf("unset_record fields must contain non-empty strings")
		}
		if !workflowValidUpdateField(fieldName) {
			return nil, fmt.Errorf("unset_record has invalid field: %s", fieldName)
		}
		values[fieldName] = ""
	}
	return values, nil
}

func workflowValidUpdateField(field string) bool {
	field = strings.TrimSpace(field)
	return field != "" && !strings.HasPrefix(field, "$")
}

func workflowConditionsMatch(conditions []models.WorkflowCondition, payload workflowExecutionPayload) bool {
	for _, condition := range conditions {
		if !workflowConditionMatches(condition, payload) {
			return false
		}
	}
	return true
}

func workflowConditionMatches(condition models.WorkflowCondition, payload workflowExecutionPayload) bool {
	switch condition.Operator {
	case models.WorkflowConditionAnd:
		return workflowConditionsMatch(condition.Conditions, payload)
	case models.WorkflowConditionOr:
		for _, nested := range condition.Conditions {
			if workflowConditionMatches(nested, payload) {
				return true
			}
		}
		return false
	}

	current, exists := workflowValueForField(condition.Field, payload)
	previous, previousExists := workflowValueForField("oldRecord."+strings.TrimPrefix(condition.Field, "record."), payload)
	expected := resolveWorkflowTemplates(condition.Value, payload)

	switch condition.Operator {
	case models.WorkflowConditionEqual, "":
		return exists && workflowValuesEqual(current, expected)
	case models.WorkflowConditionNotEqual:
		return !workflowValuesEqual(current, expected)
	case models.WorkflowConditionGreaterThan:
		return workflowCompareValues(current, expected) > 0
	case models.WorkflowConditionGreaterEqual:
		return workflowCompareValues(current, expected) >= 0
	case models.WorkflowConditionLessThan:
		return workflowCompareValues(current, expected) < 0
	case models.WorkflowConditionLessEqual:
		return workflowCompareValues(current, expected) <= 0
	case models.WorkflowConditionIn:
		return workflowValueIn(current, expected)
	case models.WorkflowConditionNotIn:
		return !workflowValueIn(current, expected)
	case models.WorkflowConditionContains:
		return workflowContainsValue(current, expected)
	case models.WorkflowConditionNotContains:
		return !workflowContainsValue(current, expected)
	case models.WorkflowConditionStartsWith:
		return strings.HasPrefix(fmt.Sprint(current), fmt.Sprint(expected))
	case models.WorkflowConditionEndsWith:
		return strings.HasSuffix(fmt.Sprint(current), fmt.Sprint(expected))
	case models.WorkflowConditionIsEmpty:
		return workflowValueIsEmpty(current)
	case models.WorkflowConditionIsNotEmpty:
		return !workflowValueIsEmpty(current)
	case models.WorkflowConditionBetween:
		return workflowValueBetween(current, expected)
	case models.WorkflowConditionExists:
		wantExists, ok := expected.(bool)
		if !ok {
			wantExists = true
		}
		return exists == wantExists
	case models.WorkflowConditionChanged:
		return exists && previousExists && !workflowValuesEqual(current, previous)
	case models.WorkflowConditionChangedTo:
		return exists && previousExists && !workflowValuesEqual(current, previous) && workflowValuesEqual(current, expected)
	case models.WorkflowConditionChangedFrom:
		return exists && previousExists && !workflowValuesEqual(current, previous) && workflowValuesEqual(previous, expected)
	default:
		return false
	}
}

func workflowValueForField(field string, payload workflowExecutionPayload) (interface{}, bool) {
	switch field {
	case "user.id", "user._id", "userId":
		return payload.UserID, payload.UserID != ""
	case "user.role":
		userRole := workflowUserRole(payload)
		return userRole, userRole != ""
	}
	if field == "" {
		return nil, false
	}
	switch {
	case strings.HasPrefix(field, "record."):
		return workflowPathValue(payload.Record, strings.TrimPrefix(field, "record."))
	case strings.HasPrefix(field, "oldRecord."):
		return workflowPathValue(payload.OldRecord, strings.TrimPrefix(field, "oldRecord."))
	case strings.HasPrefix(field, "steps."):
		return workflowPathValue(payload.StepOutputs, strings.TrimPrefix(field, "steps."))
	case strings.HasPrefix(field, "vars."):
		return workflowPathValue(payload.Variables, strings.TrimPrefix(field, "vars."))
	case strings.HasPrefix(field, "loop."):
		return workflowPathValue(payload.Loop, strings.TrimPrefix(field, "loop."))
	default:
		return workflowPathValue(payload.Record, field)
	}
}

func workflowPathValue(value interface{}, path string) (interface{}, bool) {
	current := value
	for _, part := range strings.Split(path, ".") {
		switch typed := current.(type) {
		case map[string]interface{}:
			next, ok := typed[part]
			if !ok {
				return nil, false
			}
			current = next
		case bson.M:
			next, ok := typed[part]
			if !ok {
				return nil, false
			}
			current = next
		case []interface{}:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		case []map[string]interface{}:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func workflowValuesEqual(left, right interface{}) bool {
	if leftNumber, ok := workflowNumber(left); ok {
		if rightNumber, ok := workflowNumber(right); ok {
			return leftNumber == rightNumber
		}
	}
	return reflect.DeepEqual(left, right) || fmt.Sprint(left) == fmt.Sprint(right)
}

func workflowCompareValues(left, right interface{}) int {
	if leftNumber, leftOK := workflowNumber(left); leftOK {
		if rightNumber, rightOK := workflowNumber(right); rightOK {
			return workflowCompareFloats(leftNumber, rightNumber)
		}
	}
	if leftTime, leftOK := workflowTime(left); leftOK {
		if rightTime, rightOK := workflowTime(right); rightOK {
			if leftTime.After(rightTime) {
				return 1
			}
			if leftTime.Before(rightTime) {
				return -1
			}
			return 0
		}
	}
	return strings.Compare(fmt.Sprint(left), fmt.Sprint(right))
}

func workflowCompareFloats(left, right float64) int {
	if left > right {
		return 1
	}
	if left < right {
		return -1
	}
	return 0
}

func workflowNumber(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	case primitive.Decimal128:
		parsed, err := strconv.ParseFloat(typed.String(), 64)
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func workflowValueIn(value, list interface{}) bool {
	items, ok := workflowInterfaceSlice(list)
	if !ok {
		return false
	}
	for _, item := range items {
		if workflowValuesEqual(value, item) {
			return true
		}
	}
	return false
}

func workflowContainsValue(value, expected interface{}) bool {
	switch typed := value.(type) {
	case string:
		return strings.Contains(typed, fmt.Sprint(expected))
	default:
		items, ok := workflowInterfaceSlice(value)
		if !ok {
			return false
		}
		for _, item := range items {
			if workflowValuesEqual(item, expected) {
				return true
			}
		}
		return false
	}
}

func workflowValueIsEmpty(value interface{}) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) == ""
	case map[string]interface{}:
		return len(typed) == 0
	case bson.M:
		return len(typed) == 0
	}
	items, ok := workflowInterfaceSlice(value)
	return ok && len(items) == 0
}

func workflowValueBetween(value, expected interface{}) bool {
	items, ok := workflowInterfaceSlice(expected)
	if !ok || len(items) != 2 {
		return false
	}
	return workflowCompareValues(value, items[0]) >= 0 && workflowCompareValues(value, items[1]) <= 0
}

func workflowTime(value interface{}) (time.Time, bool) {
	switch typed := value.(type) {
	case time.Time:
		return typed, true
	case primitive.DateTime:
		return typed.Time(), true
	case string:
		trimmed := strings.TrimSpace(typed)
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
			if parsed, err := time.Parse(layout, trimmed); err == nil {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

func resolveWorkflowTemplates(value interface{}, payload workflowExecutionPayload) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		resolved := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			resolved[key] = resolveWorkflowTemplates(item, payload)
		}
		return resolved
	case bson.M:
		resolved := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			resolved[key] = resolveWorkflowTemplates(item, payload)
		}
		return resolved
	case []interface{}:
		resolved := make([]interface{}, len(typed))
		for i, item := range typed {
			resolved[i] = resolveWorkflowTemplates(item, payload)
		}
		return resolved
	case string:
		return resolveWorkflowTemplateString(typed, payload)
	default:
		return value
	}
}

func resolveWorkflowTemplateString(value string, payload workflowExecutionPayload) interface{} {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "{{") && strings.HasSuffix(trimmed, "}}") && strings.Count(trimmed, "{{") == 1 {
		if resolved, ok := workflowTemplateValue(strings.TrimSuffix(strings.TrimPrefix(trimmed, "{{"), "}}"), payload); ok {
			return resolved
		}
	}

	result := value
	for {
		start := strings.Index(result, "{{")
		if start < 0 {
			break
		}
		end := strings.Index(result[start:], "}}")
		if end < 0 {
			break
		}
		end += start
		token := result[start+2 : end]
		resolved, _ := workflowTemplateValue(token, payload)
		result = result[:start] + fmt.Sprint(resolved) + result[end+2:]
	}
	return coerceWorkflowScalar(result)
}

func workflowTemplateValue(token string, payload workflowExecutionPayload) (interface{}, bool) {
	token = strings.TrimSpace(token)
	if token == "now" {
		return time.Now().UTC(), true
	}
	if token == "today" {
		now := time.Now().UTC()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), true
	}
	if name, args, ok := workflowFunctionCall(token); ok {
		return resolveWorkflowFunction(name, args, payload)
	}
	switch {
	case strings.HasPrefix(token, "record."):
		return workflowPathValue(payload.Record, strings.TrimPrefix(token, "record."))
	case strings.HasPrefix(token, "query."):
		return workflowPathValue(payload.Query, strings.TrimPrefix(token, "query."))
	case strings.HasPrefix(token, "oldRecord."):
		return workflowPathValue(payload.OldRecord, strings.TrimPrefix(token, "oldRecord."))
	case strings.HasPrefix(token, "steps."):
		return workflowPathValue(payload.StepOutputs, strings.TrimPrefix(token, "steps."))
	case strings.HasPrefix(token, "vars."):
		return workflowPathValue(payload.Variables, strings.TrimPrefix(token, "vars."))
	case strings.HasPrefix(token, "loop."):
		return workflowPathValue(payload.Loop, strings.TrimPrefix(token, "loop."))
	case token == "user.id" || token == "user._id" || token == "userId":
		return payload.UserID, payload.UserID != ""
	case token == "schemaName":
		return payload.SchemaName, payload.SchemaName != ""
	case token == "workflowName":
		return payload.WorkflowName, payload.WorkflowName != ""
	default:
		return "", false
	}
}

func workflowFunctionCall(token string) (string, []string, bool) {
	open := strings.Index(token, "(")
	if open <= 0 || !strings.HasSuffix(token, ")") {
		return "", nil, false
	}
	name := strings.TrimSpace(token[:open])
	if name == "" {
		return "", nil, false
	}
	args, ok := workflowSplitFunctionArgs(token[open+1 : len(token)-1])
	return name, args, ok
}

func workflowSplitFunctionArgs(value string) ([]string, bool) {
	if strings.TrimSpace(value) == "" {
		return nil, true
	}
	var args []string
	start := 0
	depth := 0
	var quote rune
	escaped := false
	runes := []rune(value)
	for index, char := range runes {
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' && quote != 0 {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			}
			continue
		}
		switch char {
		case '\'', '"':
			quote = char
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return nil, false
			}
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(string(runes[start:index])))
				start = index + 1
			}
		}
	}
	if quote != 0 || depth != 0 {
		return nil, false
	}
	args = append(args, strings.TrimSpace(string(runes[start:])))
	return args, true
}

func resolveWorkflowFunction(name string, args []string, payload workflowExecutionPayload) (interface{}, bool) {
	switch name {
	case "default":
		if len(args) != 2 {
			return nil, false
		}
		value, ok := resolveWorkflowExpressionArg(args[0], payload)
		if ok && value != nil && value != "" {
			return value, true
		}
		return resolveWorkflowExpressionArg(args[1], payload)
	case "multiply":
		return workflowNumericFunction(args, payload, func(left, right float64) (float64, bool) {
			return left * right, true
		})
	case "add":
		return workflowNumericFunction(args, payload, func(left, right float64) (float64, bool) {
			return left + right, true
		})
	case "subtract":
		return workflowNumericFunction(args, payload, func(left, right float64) (float64, bool) {
			return left - right, true
		})
	case "divide":
		return workflowNumericFunction(args, payload, func(left, right float64) (float64, bool) {
			if right == 0 {
				return 0, false
			}
			return left / right, true
		})
	case "min":
		return workflowNumericFunction(args, payload, func(left, right float64) (float64, bool) {
			if left < right {
				return left, true
			}
			return right, true
		})
	case "addDays":
		if len(args) != 2 {
			return nil, false
		}
		value, ok := resolveWorkflowExpressionArg(args[0], payload)
		if !ok {
			return nil, false
		}
		date, ok := workflowTime(value)
		if !ok {
			return nil, false
		}
		daysValue, ok := resolveWorkflowExpressionArg(args[1], payload)
		if !ok {
			return nil, false
		}
		days, ok := workflowNumber(daysValue)
		if !ok || days != float64(int64(days)) {
			return nil, false
		}
		return date.AddDate(0, 0, int(days)), true
	case "join":
		if len(args) != 2 {
			return nil, false
		}
		value, ok := resolveWorkflowExpressionArg(args[0], payload)
		if !ok {
			return nil, false
		}
		separator, ok := resolveWorkflowExpressionArg(args[1], payload)
		if !ok {
			return nil, false
		}
		return workflowJoin(value, fmt.Sprint(separator))
	case "last":
		if len(args) != 1 {
			return nil, false
		}
		value, ok := resolveWorkflowExpressionArg(args[0], payload)
		if !ok {
			return nil, false
		}
		return workflowLast(value)
	case "length":
		if len(args) != 1 {
			return nil, false
		}
		value, ok := resolveWorkflowExpressionArg(args[0], payload)
		if !ok {
			return nil, false
		}
		return workflowLength(value)
	case "array":
		items := make([]interface{}, 0, len(args))
		for _, arg := range args {
			value, ok := resolveWorkflowExpressionArg(arg, payload)
			if !ok {
				return nil, false
			}
			items = append(items, value)
		}
		return items, true
	default:
		return nil, false
	}
}

func resolveWorkflowExpressionArg(value string, payload workflowExecutionPayload) (interface{}, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", true
	}
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		return value[1 : len(value)-1], true
	}
	if resolved, ok := workflowTemplateValue(value, payload); ok {
		return resolved, true
	}
	if workflowExpressionReference(value) {
		return nil, false
	}
	switch strings.ToLower(value) {
	case "nil", "null":
		return nil, true
	case "true":
		return true, true
	case "false":
		return false, true
	}
	coerced := coerceWorkflowScalar(value)
	if _, ok := coerced.(string); !ok {
		return coerced, true
	}
	return value, true
}

func workflowExpressionReference(value string) bool {
	for _, prefix := range []string{"record.", "oldRecord.", "steps.", "vars.", "loop."} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	switch value {
	case "user.id", "user._id", "userId", "schemaName", "workflowName", "now", "today":
		return true
	default:
		return false
	}
}

func workflowNumericFunction(args []string, payload workflowExecutionPayload, operation func(float64, float64) (float64, bool)) (interface{}, bool) {
	if len(args) != 2 {
		return nil, false
	}
	left, ok := resolveWorkflowExpressionArg(args[0], payload)
	if !ok {
		return nil, false
	}
	right, ok := resolveWorkflowExpressionArg(args[1], payload)
	if !ok {
		return nil, false
	}
	leftNumber, ok := workflowNumber(left)
	if !ok {
		return nil, false
	}
	rightNumber, ok := workflowNumber(right)
	if !ok {
		return nil, false
	}
	return operation(leftNumber, rightNumber)
}

func workflowJoin(value interface{}, separator string) (interface{}, bool) {
	items, ok := workflowInterfaceSlice(value)
	if !ok {
		return nil, false
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, fmt.Sprint(item))
	}
	return strings.Join(values, separator), true
}

func workflowLast(value interface{}) (interface{}, bool) {
	items, ok := workflowInterfaceSlice(value)
	if !ok || len(items) == 0 {
		return nil, false
	}
	return items[len(items)-1], true
}

func workflowLength(value interface{}) (interface{}, bool) {
	if value == nil {
		return 0, true
	}
	switch typed := value.(type) {
	case string:
		return len(typed), true
	case map[string]interface{}:
		return len(typed), true
	case bson.M:
		return len(typed), true
	}
	items, ok := workflowInterfaceSlice(value)
	if !ok {
		return nil, false
	}
	return len(items), true
}

func workflowInterfaceSlice(value interface{}) ([]interface{}, bool) {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || (reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array) {
		return nil, false
	}
	items := make([]interface{}, reflected.Len())
	for index := 0; index < reflected.Len(); index++ {
		items[index] = reflected.Index(index).Interface()
	}
	return items, true
}

func coerceWorkflowScalar(value string) interface{} {
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		return parsed
	}
	switch strings.ToLower(value) {
	case "true":
		return true
	case "false":
		return false
	}
	return value
}

func normalizeWorkflowIDs(value interface{}) {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, item := range typed {
			if key == "_id" {
				if normalized, ok := normalizeWorkflowIDValue(item); ok {
					typed[key] = normalized
					continue
				}
			}
			normalizeWorkflowIDs(item)
		}
	case bson.M:
		for key, item := range typed {
			if key == "_id" {
				if normalized, ok := normalizeWorkflowIDValue(item); ok {
					typed[key] = normalized
					continue
				}
			}
			normalizeWorkflowIDs(item)
		}
	case []interface{}:
		for _, item := range typed {
			normalizeWorkflowIDs(item)
		}
	case []map[string]interface{}:
		for _, item := range typed {
			normalizeWorkflowIDs(item)
		}
	case []bson.M:
		for _, item := range typed {
			normalizeWorkflowIDs(item)
		}
	case primitive.A:
		for _, item := range typed {
			normalizeWorkflowIDs(item)
		}
	}
}

func normalizeWorkflowIDValue(value interface{}) (interface{}, bool) {
	if idString, ok := value.(string); ok {
		if objectID, err := primitive.ObjectIDFromHex(idString); err == nil {
			return objectID, true
		}
		return nil, false
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		normalizeWorkflowIDOperatorMap(typed)
		return typed, true
	case bson.M:
		normalizeWorkflowIDOperatorMap(typed)
		return typed, true
	default:
		return nil, false
	}
}

func normalizeWorkflowIDOperatorMap(operatorMap map[string]interface{}) {
	for operator, value := range operatorMap {
		switch typed := value.(type) {
		case []interface{}:
			for index, item := range typed {
				if idString, ok := item.(string); ok {
					if objectID, err := primitive.ObjectIDFromHex(idString); err == nil {
						typed[index] = objectID
					}
				}
			}
			operatorMap[operator] = typed
		case primitive.A:
			for index, item := range typed {
				if idString, ok := item.(string); ok {
					if objectID, err := primitive.ObjectIDFromHex(idString); err == nil {
						typed[index] = objectID
					}
				}
			}
			operatorMap[operator] = typed
		}
	}
}

func cloneWorkflowMap(value map[string]interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
