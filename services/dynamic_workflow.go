package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"github.com/osmansam/autotableGo/validators"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type workflowExecutionPayload struct {
	TenantID       string
	ProjectID      string
	SchemaName     string
	WorkflowName   string
	WorkflowMode   string
	ExecutionMode  string
	StopOnError    bool
	Record         map[string]interface{}
	OldRecord      map[string]interface{}
	StepOutputs    map[string]interface{}
	Variables      map[string]interface{}
	Loop           map[string]interface{}
	UserID         string
	AuditUser      *models.AuditUser
	Container      *models.ContainerModel
	OutboxEventID  primitive.ObjectID
	IdempotencyKey string
	WorkflowDepth  int
	OutboxEvents   *int
}

const (
	maxWorkflowDepth             = 4
	maxWorkflowSteps             = 100
	maxWorkflowOutboxEvents      = 100
	maxWorkflowLoopItems         = 500
	maxWorkflowQueryLimit        = 500
	minWorkflowCallAPITimeoutSec = 1
)

func (s *DynamicService) runTransactionalWorkflows(ctx mongo.SessionContext, payload workflowExecutionPayload, trigger string) error {
	return s.runWorkflows(ctx, payload, trigger, models.WorkflowModeTransactional)
}

func (s *DynamicService) enqueueOutboxWorkflows(ctx mongo.SessionContext, payload workflowExecutionPayload, trigger string) error {
	return s.runWorkflows(ctx, payload, trigger, models.WorkflowModeOutbox)
}

func (s *DynamicService) runWorkflowDefinition(ctx context.Context, payload workflowExecutionPayload, workflow models.DynamicWorkflow) error {
	if err := validateWorkflowExecutionBounds(payload, workflow); err != nil {
		return err
	}
	ensureWorkflowPayloadMaps(&payload)
	if payload.OutboxEvents == nil {
		outboxEvents := 0
		payload.OutboxEvents = &outboxEvents
	}
	if !workflowConditionsMatch(workflow.Conditions, payload) {
		return nil
	}

	workflowCtx, cancel := workflowContextWithTimeout(ctx, workflow.TimeoutSec)
	defer cancel()

	if workflowSupportsMode(workflow.Mode, models.WorkflowModeTransactional) {
		if err := s.runWorkflowSteps(workflowCtx, payload, workflow, models.WorkflowModeTransactional); err != nil {
			return err
		}
	}
	if workflowSupportsMode(workflow.Mode, models.WorkflowModeOutbox) {
		if err := s.runWorkflowSteps(workflowCtx, payload, workflow, models.WorkflowModeOutbox); err != nil {
			return err
		}
	}

	return nil
}

func (s *DynamicService) runWorkflowSteps(ctx context.Context, payload workflowExecutionPayload, workflow models.DynamicWorkflow, executionMode string) error {
	steps := append([]models.DynamicWorkflowStep(nil), workflow.Steps...)
	sort.SliceStable(steps, func(i, j int) bool {
		return steps[i].Order < steps[j].Order
	})

	ensureWorkflowPayloadMaps(&payload)
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
		if err := validateWorkflowExecutionBounds(payload, workflow); err != nil {
			return err
		}
		if !workflowConditionsMatch(workflow.Conditions, payload) {
			continue
		}

		workflowCtx, cancel := workflowContextWithTimeout(ctx, workflow.TimeoutSec)
		steps := append([]models.DynamicWorkflowStep(nil), workflow.Steps...)
		sort.SliceStable(steps, func(i, j int) bool {
			return steps[i].Order < steps[j].Order
		})

		if err := s.runWorkflowStepList(workflowCtx, payload, workflow.Name, workflow.Mode, executionMode, workflow.StopOnError, steps); err != nil {
			cancel()
			return err
		}
		cancel()
	}
	return nil
}

func ensureWorkflowPayloadMaps(payload *workflowExecutionPayload) {
	if payload.StepOutputs == nil {
		payload.StepOutputs = map[string]interface{}{}
	}
	if payload.Variables == nil {
		payload.Variables = map[string]interface{}{}
	}
	if payload.Loop == nil {
		payload.Loop = map[string]interface{}{}
	}
}

func (s *DynamicService) runWorkflowStepList(ctx context.Context, payload workflowExecutionPayload, workflowName, workflowMode, executionMode string, stopOnError bool, steps []models.DynamicWorkflowStep) error {
	ensureWorkflowPayloadMaps(&payload)
	orderedSteps := append([]models.DynamicWorkflowStep(nil), steps...)
	sort.SliceStable(orderedSteps, func(i, j int) bool {
		return orderedSteps[i].Order < orderedSteps[j].Order
	})

	for _, step := range orderedSteps {
		if !step.IsActive || workflowStepExecutionMode(step, workflowMode) != executionMode {
			continue
		}
		if step.Type != models.WorkflowStepTypeIf && !workflowConditionsMatch(step.Conditions, payload) {
			continue
		}

		stepPayload := payload
		stepPayload.WorkflowName = workflowName
		stepPayload.WorkflowMode = workflowMode
		stepPayload.ExecutionMode = executionMode
		stepPayload.StopOnError = stopOnError
		var err error
		var output interface{}
		if executionMode == models.WorkflowModeOutbox {
			err = s.enqueueWorkflowStep(ctx, stepPayload, workflowName, step)
		} else {
			output, err = s.processWorkflowStepForMode(ctx, step, stepPayload, executionMode)
			if err == nil && step.Name != "" {
				payload.StepOutputs[step.Name] = output
			}
		}
		if err != nil && stopOnError && !step.ContinueOnError {
			return fmt.Errorf("workflow %s step %s failed: %w", workflowName, step.Name, err)
		}
	}
	return nil
}

func (s *DynamicService) processWorkflowStepForMode(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload, executionMode string) (interface{}, error) {
	if executionMode == models.WorkflowModeTransactional && workflowStepHasNonTransactionalSideEffect(step) {
		return nil, fmt.Errorf("%s steps must run in outbox execution mode", step.Type)
	}
	return s.processWorkflowStepWithTimeout(ctx, step, payload)
}

func (s *DynamicService) processWorkflowStepWithTimeout(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	ensureWorkflowPayloadMaps(&payload)
	stepCtx := ctx
	var cancel context.CancelFunc
	if step.TimeoutSec > 0 {
		stepCtx, cancel = context.WithTimeout(ctx, time.Duration(step.TimeoutSec)*time.Second)
		defer cancel()
	}
	return s.processWorkflowStep(stepCtx, step, payload)
}

func (s *DynamicService) processWorkflowStep(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	switch step.Type {
	case models.WorkflowStepTypeCreateRecord:
		return s.workflowCreateRecord(ctx, step, payload)
	case models.WorkflowStepTypeUpdateRecord:
		return s.workflowUpdateRecord(ctx, step, payload)
	case models.WorkflowStepTypeDeleteRecord:
		return s.workflowDeleteRecord(ctx, step, payload)
	case models.WorkflowStepTypeAuditLog:
		return s.workflowAuditLog(ctx, step, payload)
	case models.WorkflowStepTypeInvalidateCache:
		return s.workflowInvalidateCache(ctx, step, payload)
	case models.WorkflowStepTypeCallAPI:
		return s.workflowCallAPI(ctx, step, payload)
	case models.WorkflowStepTypeRunPipeline:
		return s.workflowRunPipeline(ctx, step, payload)
	case models.WorkflowStepTypeDynamicFunction:
		return nil, fmt.Errorf("workflow dynamic_function steps require a request context and are not supported by the write workflow processor yet")
	case models.WorkflowStepTypeEmitOutboxEvent:
		return s.workflowEmitOutboxEvent(ctx, step, payload)
	case models.WorkflowStepTypeGetRecord:
		return s.workflowGetRecord(ctx, step, payload)
	case models.WorkflowStepTypeFindRecords:
		return s.workflowFindRecords(ctx, step, payload)
	case models.WorkflowStepTypeIf:
		return s.workflowIf(ctx, step, payload)
	case models.WorkflowStepTypeForEach:
		return s.workflowForEach(ctx, step, payload)
	case models.WorkflowStepTypeSetVariable:
		return s.workflowSetVariable(step, payload)
	case models.WorkflowStepTypeExecuteWorkflow:
		return s.workflowExecuteWorkflow(ctx, step, payload)
	case models.WorkflowStepTypeExecuteDynamicAPI:
		return s.workflowExecuteDynamicAPI(ctx, step, payload)
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
	if !workflowHasUpdateOperator(update) {
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

	limit := workflowIntConfig(step.Config, "limit", 50)
	if limit <= 0 || limit > maxWorkflowQueryLimit {
		return nil, fmt.Errorf("find_records limit must be between 1 and %d", maxWorkflowQueryLimit)
	}

	searchKey := workflowStringConfig(step.Config, "search", "")
	if searchKey == "" {
		searchKey = workflowStringConfig(step.Config, "searchKey", "")
	}
	if searchKey != "" {
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

	findOptions := options.Find().SetLimit(int64(limit)).SetMaxTime(10 * time.Second)
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
	items = utils.FilterDocuments(items, targetContainer.Fields, userRole)

	return map[string]interface{}{
		"items": items,
		"count": len(items),
	}, nil
}

func (s *DynamicService) workflowSetVariable(step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	ensureWorkflowPayloadMaps(&payload)
	valuesConfig, ok := step.Config["values"]
	if !ok {
		valuesConfig = step.Config
	}
	values, ok := resolveWorkflowTemplates(valuesConfig, payload).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("set_variable step requires values config")
	}
	for key, value := range values {
		payload.Variables[key] = value
	}
	return cloneWorkflowMap(payload.Variables), nil
}

func (s *DynamicService) workflowIf(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
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
		if len(branch.Conditions) > 0 && !workflowConditionsMatch(branch.Conditions, payload) {
			continue
		}
		if err := s.runWorkflowStepList(ctx, payload, payload.WorkflowName, payload.WorkflowMode, workflowNestedExecutionMode(payload), payload.StopOnError, branch.Steps); err != nil {
			return nil, err
		}
		return map[string]interface{}{"branch": branch.Name}, nil
	}

	return map[string]interface{}{"branch": nil}, nil
}

func (s *DynamicService) workflowForEach(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	itemsValue := resolveWorkflowTemplates(step.Config["items"], payload)
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
		loopPayload := payload
		loopPayload.Loop = cloneWorkflowMap(payload.Loop)
		loopPayload.Loop[itemName] = item
		loopPayload.Loop["item"] = item
		loopPayload.Loop["index"] = index
		if err := s.runWorkflowStepList(ctx, loopPayload, payload.WorkflowName, payload.WorkflowMode, workflowNestedExecutionMode(payload), payload.StopOnError, step.Steps); err != nil {
			return nil, err
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
	childPayload.Container = targetContainer
	childPayload.WorkflowDepth = payload.WorkflowDepth + 1
	if err := s.runWorkflowDefinition(ctx, childPayload, workflow); err != nil {
		return nil, err
	}
	return childPayload.StepOutputs, nil
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

func (s *DynamicService) workflowCallAPI(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	if step.TimeoutSec < minWorkflowCallAPITimeoutSec {
		return nil, fmt.Errorf("call_api step requires timeoutSec >= %d", minWorkflowCallAPITimeoutSec)
	}
	method := workflowStringConfig(step.Config, "method", "POST")
	url := workflowStringConfig(step.Config, "url", "")
	if url == "" {
		return nil, fmt.Errorf("call_api step requires url config")
	}
	if err := validateWorkflowCallAPIURL(ctx, url); err != nil {
		return nil, err
	}
	body := resolveWorkflowTemplates(step.Config["body"], payload)
	responseBytes, err := utils.ExecuteApiRequest(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	var response interface{}
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		return string(responseBytes), nil
	}
	return response, nil
}

func (s *DynamicService) workflowRunPipeline(ctx context.Context, step models.DynamicWorkflowStep, payload workflowExecutionPayload) (interface{}, error) {
	targetSchema := workflowTargetSchema(step, payload.SchemaName)
	pipelineJSON := workflowStringConfig(step.Config, "pipelineJson", "")
	if pipelineJSON == "" {
		pipelineJSON = workflowStringConfig(step.Config, "pipeline", "")
	}
	if pipelineJSON == "" {
		return nil, fmt.Errorf("run_pipeline step requires pipelineJson config")
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
	event := buildWorkflowStepOutboxEvent(payload.TenantID, payload.ProjectID, payload.SchemaName, payload.UserID, workflowName, step, payload.Record, payload.OldRecord, payload.StepOutputs, payload.Variables, payload.Loop)
	event.Payload.WorkflowDepth = payload.WorkflowDepth + 1
	_, err := s.repository.InsertOutboxEvent(ctx, event)
	return err
}

func buildWorkflowStepOutboxEvent(tenantID, projectID, schemaName, userID, workflowName string, step models.DynamicWorkflowStep, record, oldRecord, stepOutputs, variables, loop map[string]interface{}) models.DynamicOutboxEvent {
	now := time.Now()
	maxAttempts := dynamicOutboxMaxAttempts
	if step.MaxAttempts > 0 {
		maxAttempts = step.MaxAttempts
	} else if step.RetryCount > 0 {
		maxAttempts = step.RetryCount
	}
	payload := workflowExecutionPayload{
		TenantID:     tenantID,
		ProjectID:    projectID,
		SchemaName:   schemaName,
		WorkflowName: workflowName,
		Record:       record,
		OldRecord:    oldRecord,
		StepOutputs:  stepOutputs,
		UserID:       userID,
	}
	idempotencyKey := workflowStepIdempotencyKey(payload, workflowName, step)
	return models.DynamicOutboxEvent{
		ID:            primitive.NewObjectID(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		SchemaName:    schemaName,
		Operation:     models.DynamicOutboxOperationWorkflowStep,
		Status:        models.DynamicOutboxStatusPending,
		MaxAttempts:   maxAttempts,
		NextAttemptAt: primitive.NewDateTimeFromTime(now),
		CreatedAt:     primitive.NewDateTimeFromTime(now),
		UpdatedAt:     primitive.NewDateTimeFromTime(now),
		Payload: models.DynamicOutboxPayload{
			UserID:         userID,
			WorkflowName:   workflowName,
			StepID:         step.ID,
			StepName:       step.Name,
			StepType:       step.Type,
			StepTimeoutSec: step.TimeoutSec,
			WorkflowDepth:  payload.WorkflowDepth + 1,
			TargetSchema:   step.TargetSchema,
			Record:         record,
			OldRecord:      oldRecord,
			StepOutputs:    cloneWorkflowMap(stepOutputs),
			Variables:      cloneWorkflowMap(variables),
			Loop:           cloneWorkflowMap(loop),
			Config:         step.Config,
			Steps:          step.Steps,
			ElseSteps:      step.ElseSteps,
			Branches:       step.Branches,
			IdempotencyKey: idempotencyKey,
		},
	}
}

func processWorkflowOutboxStep(ctx context.Context, repository *repositories.DynamicRepository, event *models.DynamicOutboxEvent) error {
	if event.Payload.StepType == models.WorkflowStepTypeEmitOutboxEvent {
		return fmt.Errorf("emit_outbox_event cannot be processed as an outbox workflow step")
	}
	if event.Payload.IdempotencyKey != "" {
		done, err := repository.WorkflowStepExecutionDone(ctx, event.Payload.IdempotencyKey)
		if err != nil {
			return err
		}
		if done {
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
	_, err := service.processWorkflowStepWithTimeout(ctx, step, workflowExecutionPayload{
		TenantID:       event.TenantID,
		ProjectID:      event.ProjectID,
		SchemaName:     event.SchemaName,
		WorkflowName:   event.Payload.WorkflowName,
		Record:         event.Payload.Record,
		OldRecord:      event.Payload.OldRecord,
		StepOutputs:    event.Payload.StepOutputs,
		Variables:      event.Payload.Variables,
		Loop:           event.Payload.Loop,
		UserID:         event.Payload.UserID,
		OutboxEventID:  event.ID,
		IdempotencyKey: event.Payload.IdempotencyKey,
		WorkflowDepth:  event.Payload.WorkflowDepth,
	})
	if err != nil {
		return err
	}
	if event.Payload.IdempotencyKey != "" {
		return repository.MarkWorkflowStepExecutionDone(ctx, event.Payload.IdempotencyKey, event.ID)
	}
	return nil
}

func workflowSupportsMode(workflowMode, executionMode string) bool {
	if workflowMode == "" {
		workflowMode = models.WorkflowModeTransactional
	}
	return workflowMode == executionMode || workflowMode == models.WorkflowModeHybrid
}

func validateWorkflowExecutionBounds(payload workflowExecutionPayload, workflow models.DynamicWorkflow) error {
	if payload.WorkflowDepth > maxWorkflowDepth {
		return fmt.Errorf("workflow depth limit exceeded: max %d", maxWorkflowDepth)
	}
	if len(workflow.Steps) > maxWorkflowSteps {
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
		return strings.TrimSpace(fmt.Sprint(resolved))
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
	}, ":")
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
		step.Type == models.WorkflowStepTypeInvalidateCache
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

func workflowConditionsMatch(conditions []models.WorkflowCondition, payload workflowExecutionPayload) bool {
	for _, condition := range conditions {
		if !workflowConditionMatches(condition, payload) {
			return false
		}
	}
	return true
}

func workflowConditionMatches(condition models.WorkflowCondition, payload workflowExecutionPayload) bool {
	current, exists := workflowValueForField(condition.Field, payload)
	previous, previousExists := workflowValueForField("oldRecord."+strings.TrimPrefix(condition.Field, "record."), payload)
	expected := resolveWorkflowTemplates(condition.Value, payload)

	switch condition.Operator {
	case models.WorkflowConditionEqual, "":
		return exists && workflowValuesEqual(current, expected)
	case models.WorkflowConditionNotEqual:
		return !workflowValuesEqual(current, expected)
	case models.WorkflowConditionGreaterThan:
		return workflowCompareNumbers(current, expected) > 0
	case models.WorkflowConditionLessThan:
		return workflowCompareNumbers(current, expected) < 0
	case models.WorkflowConditionIn:
		return workflowValueIn(current, expected)
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

func workflowCompareNumbers(left, right interface{}) int {
	leftNumber, leftOK := workflowNumber(left)
	rightNumber, rightOK := workflowNumber(right)
	if !leftOK || !rightOK {
		return 0
	}
	if leftNumber > rightNumber {
		return 1
	}
	if leftNumber < rightNumber {
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
	switch typed := list.(type) {
	case []interface{}:
		for _, item := range typed {
			if workflowValuesEqual(value, item) {
				return true
			}
		}
	case []string:
		for _, item := range typed {
			if workflowValuesEqual(value, item) {
				return true
			}
		}
	}
	return false
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
	switch {
	case strings.HasPrefix(token, "record."):
		return workflowPathValue(payload.Record, strings.TrimPrefix(token, "record."))
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
				if idString, ok := item.(string); ok {
					if objectID, err := primitive.ObjectIDFromHex(idString); err == nil {
						typed[key] = objectID
						continue
					}
				}
			}
			normalizeWorkflowIDs(item)
		}
	case bson.M:
		for key, item := range typed {
			if key == "_id" {
				if idString, ok := item.(string); ok {
					if objectID, err := primitive.ObjectIDFromHex(idString); err == nil {
						typed[key] = objectID
						continue
					}
				}
			}
			normalizeWorkflowIDs(item)
		}
	case []interface{}:
		for _, item := range typed {
			normalizeWorkflowIDs(item)
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
