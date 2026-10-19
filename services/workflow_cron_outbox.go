package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"go.mongodb.org/mongo-driver/mongo"
)

func processCronWorkflowOutbox(ctx context.Context, repository *repositories.DynamicRepository, event *models.DynamicOutboxEvent) error {
	if event == nil || event.Payload.CronWorkflow == nil || event.Payload.WorkflowSnapshotVersion != workflowSnapshotVersion {
		return fmt.Errorf("missing or unsupported cron snapshot")
	}
	workflow := cloneWorkflowSnapshot(*event.Payload.CronWorkflow)
	at := event.Payload.CronScheduledAt.Time()
	if (event.TenantID == "") != (event.ProjectID == "") || event.SchemaName == "" || event.Payload.CronScheduledAt == 0 || workflow.Name != event.Payload.WorkflowName || !workflow.IsActive || workflow.Trigger != models.WorkflowTriggerCron {
		return fmt.Errorf("invalid cron snapshot identity")
	}
	scheduled := scheduledWorkflow{TenantID: event.TenantID, ProjectID: event.ProjectID, Schema: event.SchemaName, Workflow: workflow}
	expected := buildCronWorkflowEvent(scheduled, at, time.Now()).Payload.IdempotencyKey
	if event.Payload.IdempotencyKey != expected {
		return fmt.Errorf("cron occurrence identity mismatch")
	}
	if err := ValidateWorkflow(workflow); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, cronRootExecutionTimeout)
	defer cancel()
	service := &DynamicService{repository: repository, runTransaction: repository.WithTransaction}
	service.loadAccessUser = service.loadCurrentAccessUser
	return repository.WithTransaction(ctx, func(tx mongo.SessionContext) error {
		done, err := repository.WorkflowStepExecutionDone(tx, event.Payload.IdempotencyKey)
		if err != nil || done {
			return err
		}
		if err := repository.FenceCronWorkflowScope(tx, cronScopeID(scheduled)); err != nil {
			return err
		}
		container, err := repository.GetContainerModel(tx, event.TenantID, event.ProjectID, event.SchemaName)
		enabled := false
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		if container != nil {
			if current, exists := findWorkflow(container, workflow.Name); exists && current.IsActive && current.Trigger == models.WorkflowTriggerCron {
				enabled = true
			}
		}
		if enabled {
			// Access policy comes from live metadata; executable workflow definitions
			// stay pinned so retrying the same occurrence cannot switch its code.
			snapshot := *container
			snapshot.Workflows = cloneWorkflowSnapshot(event.Payload.CronWorkflows)
			if len(snapshot.Workflows) == 0 {
				snapshot.Workflows = []models.DynamicWorkflow{workflow}
			}
			payload := newCronWorkflowPayload(event.TenantID, event.ProjectID, event.SchemaName, &snapshot, workflow, at)
			payload.OutboxEventID = event.ID
			payload.IdempotencyKey = event.Payload.IdempotencyKey
			if err := service.runWorkflowDefinition(tx, &payload, workflow); err != nil {
				return err
			}
		}
		if err := repository.MarkWorkflowStepExecutionDone(tx, event.Payload.IdempotencyKey, event.ID); err != nil {
			return err
		}
		// Fence only just before commit so lease renewal can continue during work.
		return repository.FenceCronOutboxClaim(tx, *event)
	})
}
