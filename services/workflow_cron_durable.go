package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/robfig/cron/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const cronCatchupHorizon = 24 * time.Hour
const cronPerWorkflowBatch = 100
const cronPollBatch = 500

// Stay below MongoDB's default 60-second transaction lifetime. The lease is
// longer, so no heartbeat needs to mutate the claim during this transaction.
const cronRootExecutionTimeout = 50 * time.Second
const cronRootLeaseDuration = 90 * time.Second

type cronScheduleQueue interface {
	EnsureCronSchedule(context.Context, string, string, time.Time, models.WorkflowCronSource) (models.WorkflowCronSchedule, error)
	AdvanceCronSchedule(context.Context, models.WorkflowCronSchedule, time.Time, time.Time, *models.DynamicOutboxEvent) error
}

func cronScopeID(s scheduledWorkflow) string {
	encoded, _ := json.Marshal([]string{s.TenantID, s.ProjectID, s.Schema, s.Workflow.Name})
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}
func cronRevision(s scheduledWorkflow) string {
	encoded, _ := json.Marshal(s.Workflow)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}
func parseCronSchedule(workflow models.DynamicWorkflow) (cron.Schedule, error) {
	spec := workflow.Schedule
	if tz := strings.TrimSpace(workflow.Timezone); tz != "" {
		spec = "CRON_TZ=" + tz + " " + spec
	}
	schedule, err := cron.ParseStandard(spec)
	if err != nil {
		return nil, err
	}
	if interval, ok := schedule.(cron.ConstantDelaySchedule); ok {
		return workflowCronInterval{delay: interval.Delay}, nil
	}
	return schedule, nil
}

func materializeCronSchedule(ctx context.Context, queue cronScheduleQueue, s scheduledWorkflow, now time.Time, budget int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if budget <= 0 {
		return 0, nil
	}
	schedule, err := parseCronSchedule(s.Workflow)
	if err != nil {
		return 0, err
	}
	first := schedule.Next(now.UTC())
	if first.IsZero() {
		return 0, fmt.Errorf("cron schedule has no next occurrence")
	}
	source := models.WorkflowCronSource{TenantID: s.TenantID, ProjectID: s.ProjectID, Schema: s.Schema, WorkflowName: s.Workflow.Name, Workflows: cloneWorkflowSnapshot(s.Container.Workflows)}
	cursor, err := queue.EnsureCronSchedule(ctx, cronScopeID(s), cronRevision(s), first, source)
	if errors.Is(err, repositories.ErrCronScheduleAdvanced) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	// The cursor may predate a metadata-only dependency change. Use the current
	// reload's complete workflow snapshot for authoritative checks on each write.
	cursor.Source = source
	cutoff := now.Add(-cronCatchupHorizon).UTC()
	if cursor.NextAt.Before(cutoff) {
		retained := schedule.Next(cutoff.Add(-time.Nanosecond)).UTC()
		if retained.IsZero() {
			return 0, fmt.Errorf("cron recovery has no next occurrence")
		}
		if err := queue.AdvanceCronSchedule(ctx, cursor, retained, cutoff, nil); err != nil {
			if errors.Is(err, repositories.ErrCronScheduleAdvanced) {
				return 0, nil
			}
			return 0, err
		}
		cursor.NextAt = retained
	}
	if budget > cronPerWorkflowBatch {
		budget = cronPerWorkflowBatch
	}
	queued := 0
	for queued < budget && !cursor.NextAt.After(now) {
		if err := ctx.Err(); err != nil {
			return queued, err
		}
		at := cursor.NextAt.UTC()
		next := schedule.Next(at).UTC()
		if next.IsZero() || !next.After(at) {
			return queued, fmt.Errorf("cron schedule did not advance")
		}
		event := buildCronWorkflowEvent(s, at, now)
		encoded, err := bson.Marshal(event)
		if err != nil {
			return queued, err
		}
		if len(encoded) > 4<<20 {
			return queued, fmt.Errorf("cron snapshot exceeds 4 MiB")
		}
		if err := queue.AdvanceCronSchedule(ctx, cursor, next, time.Time{}, &event); err != nil {
			if errors.Is(err, repositories.ErrCronScheduleAdvanced) {
				return queued, nil
			}
			return queued, err
		}
		cursor.NextAt = next
		queued++
	}
	return queued, nil
}

func buildCronWorkflowEvent(s scheduledWorkflow, at, now time.Time) models.DynamicOutboxEvent {
	key := "cron:" + cronScopeID(s) + ":" + at.UTC().Format(time.RFC3339Nano)
	return models.DynamicOutboxEvent{ID: primitive.NewObjectID(), TenantID: s.TenantID, ProjectID: s.ProjectID, SchemaName: s.Schema,
		Operation: models.DynamicOutboxOperationCronWorkflow, Status: models.DynamicOutboxStatusPending, MaxAttempts: dynamicOutboxMaxAttempts,
		NextAttemptAt: primitive.NewDateTimeFromTime(now), CreatedAt: primitive.NewDateTimeFromTime(now), UpdatedAt: primitive.NewDateTimeFromTime(now),
		Payload: models.DynamicOutboxPayload{WorkflowSnapshotVersion: workflowSnapshotVersion, IdentityKind: AccessIdentitySystem, WorkflowName: s.Workflow.Name, WorkflowTrigger: models.WorkflowTriggerCron, WorkflowVersion: workflowVersion(s.Workflow), CronWorkflow: cloneWorkflowSnapshot(&s.Workflow), CronWorkflows: cloneWorkflowSnapshot(s.Container.Workflows), CronScheduledAt: primitive.NewDateTimeFromTime(at), IdempotencyKey: key}}
}
