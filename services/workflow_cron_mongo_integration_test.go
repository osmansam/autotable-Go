//go:build integration

package services

import (
	"context"
	"errors"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"sync"
	"testing"
	"time"
)

func TestWorkflowMongoCronCursorAndOccurrenceCommitTogether(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	at := time.Now().UTC().Truncate(time.Second)
	s := durableCronFixture()
	s.TenantID = "tenant"
	s.ProjectID = "project"
	s.Schema = "orders"
	s.Container = *f.container
	s.Container.Workflows = []models.DynamicWorkflow{s.Workflow}
	f.container.Workflows = s.Container.Workflows
	f.saveContainer()
	cursor, err := f.repo.EnsureCronSchedule(f.ctx, cronScopeID(s), cronRevision(s), at, models.WorkflowCronSource{TenantID: s.TenantID, ProjectID: s.ProjectID, Schema: s.Schema, WorkflowName: s.Workflow.Name, Workflows: s.Container.Workflows})
	if err != nil {
		t.Fatal(err)
	}
	broken := buildCronWorkflowEvent(s, at, at)
	broken.Payload.Config = map[string]interface{}{"cannotEncode": make(chan int)}
	if err := f.repo.AdvanceCronSchedule(f.ctx, cursor, at.Add(time.Minute), time.Time{}, &broken); err == nil {
		t.Fatal("accepted invalid occurrence")
	}
	var stored models.WorkflowCronSchedule
	if err := f.db.Collection("workflow_cron_schedules").FindOne(f.ctx, bson.M{"_id": cursor.ID}).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	if !stored.NextAt.Equal(at) {
		t.Fatal("cursor consumed an occurrence whose enqueue rolled back")
	}
	event := buildCronWorkflowEvent(s, at, at)
	if err := f.repo.AdvanceCronSchedule(f.ctx, cursor, at.Add(time.Minute), time.Time{}, &event); err != nil {
		t.Fatal(err)
	}
	if n, err := f.db.Collection("dynamic_outbox").CountDocuments(f.ctx, bson.M{"operation": models.DynamicOutboxOperationCronWorkflow}); err != nil || n != 1 {
		t.Fatalf("queued=%d %v", n, err)
	}
}
func TestWorkflowMongoCronConcurrentSchedulersRecoverOnce(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	s := durableCronFixture()
	s.TenantID = "tenant"
	s.ProjectID = "project"
	s.Schema = "orders"
	s.Container = *f.container
	s.Container.Workflows = []models.DynamicWorkflow{s.Workflow}
	f.container.Workflows = s.Container.Workflows
	f.saveContainer()
	start := time.Now().UTC().Truncate(time.Minute).Add(-4 * time.Minute)
	if _, err := materializeCronSchedule(f.ctx, f.repo, s, start, 100); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	failures := make(chan error, 16)
	for range 16 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := materializeCronSchedule(f.ctx, f.repo, s, start.Add(3*time.Minute), 100)
			if err != nil {
				failures <- err
			}
		}()
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if n, err := f.db.Collection("dynamic_outbox").CountDocuments(f.ctx, bson.M{"operation": models.DynamicOutboxOperationCronWorkflow}); err != nil || n != 3 {
		t.Fatalf("duplicate/missing occurrences: %d %v", n, err)
	}
}

func TestWorkflowMongoStaleSchedulerCannotRestoreAnOldDefinition(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	s := durableCronFixture()
	s.TenantID = "tenant"
	s.ProjectID = "project"
	s.Schema = "orders"
	s.Container = *f.container
	s.Container.Workflows = []models.DynamicWorkflow{s.Workflow}
	f.container.Workflows = s.Container.Workflows
	f.saveContainer()
	start := time.Date(2026, 10, 19, 12, 0, 30, 0, time.UTC)
	if _, err := materializeCronSchedule(f.ctx, f.repo, s, start, 100); err != nil {
		t.Fatal(err)
	}
	edited := s
	edited.Workflow.Schedule = "*/5 * * * *"
	edited.Container.Workflows = []models.DynamicWorkflow{edited.Workflow}
	f.container.Workflows = edited.Container.Workflows
	f.saveContainer()
	if _, err := materializeCronSchedule(f.ctx, f.repo, edited, start.Add(10*time.Minute), 100); err != nil {
		t.Fatal(err)
	}
	if _, err := materializeCronSchedule(f.ctx, f.repo, s, start.Add(11*time.Minute), 100); err != nil {
		t.Fatal(err)
	}
	var cursor models.WorkflowCronSchedule
	if err := f.db.Collection("workflow_cron_schedules").FindOne(f.ctx, bson.M{"_id": cronScopeID(s)}).Decode(&cursor); err != nil {
		t.Fatal(err)
	}
	if !cursor.NextAt.Equal(time.Date(2026, 10, 19, 12, 15, 0, 0, time.UTC)) {
		t.Fatalf("stale replica reset edited cursor to %v", cursor.NextAt)
	}
}
func queuedMongoCron(t *testing.T, fail bool) (*workflowMongoFixture, *models.DynamicOutboxEvent) {
	t.Helper()
	f := newWorkflowMongoFixture(t)
	f.container.RowAccess = nil
	f.container.Routes.CreateDynamicModelItem.Access = nil
	workflow := models.DynamicWorkflow{Name: "durable", Trigger: models.WorkflowTriggerCron, Schedule: "* * * * *", IsActive: true, Mode: models.WorkflowModeTransactional, StopOnError: true,
		Steps: []models.DynamicWorkflowStep{{ID: "create", Type: models.WorkflowStepTypeCreateRecord, IsActive: true, TargetSchema: "orders", Config: map[string]interface{}{"values": map[string]interface{}{"name": "scheduled"}}}}}
	if fail {
		workflow.Steps = append(workflow.Steps, models.DynamicWorkflowStep{ID: "fail", Type: models.WorkflowStepTypeFail, IsActive: true, Order: 1, Config: map[string]interface{}{"message": "cron rollback"}})
	}
	f.container.Workflows = []models.DynamicWorkflow{workflow}
	f.saveContainer()
	s := scheduledWorkflow{TenantID: "tenant", ProjectID: "project", Schema: "orders", Container: *f.container, Workflow: workflow}
	at := time.Now().UTC().Truncate(time.Second)
	if _, err := f.repo.EnsureCronSchedule(f.ctx, cronScopeID(s), cronRevision(s), at, models.WorkflowCronSource{TenantID: s.TenantID, ProjectID: s.ProjectID, Schema: s.Schema, WorkflowName: s.Workflow.Name, Workflows: s.Container.Workflows}); err != nil {
		t.Fatal(err)
	}
	event := buildCronWorkflowEvent(s, at, at)
	if _, err := f.repo.InsertOutboxEvent(f.ctx, event); err != nil {
		t.Fatal(err)
	}
	claimed, err := f.repo.ClaimPendingOutboxEvent(f.ctx, at.Add(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	return f, claimed
}
func TestWorkflowMongoCronRootReplayDoesNotRepeatCommittedChanges(t *testing.T) {
	f, event := queuedMongoCron(t, false)
	for range 2 {
		if err := processCronWorkflowOutbox(f.ctx, f.repo, event); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := f.repo.GetProjectCollection("tenant", "project", "orders").CountDocuments(f.ctx, bson.M{}); err != nil || n != 1 {
		t.Fatalf("replayed writes=%d %v", n, err)
	}
	if n, err := f.db.Collection("dynamic_outbox").CountDocuments(f.ctx, bson.M{"operation": models.DynamicOutboxOperationCreate}); err != nil || n != 1 {
		t.Fatalf("replayed child events=%d %v", n, err)
	}
}

func TestWorkflowMongoCronLegacyScopeRemainsSeparateFromProjectData(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	workflow := asyncAuditCronWorkflow("legacy")
	workflow.Timezone = "UTC"
	workflow.Steps = []models.DynamicWorkflowStep{{ID: "create", Type: models.WorkflowStepTypeCreateRecord, IsActive: true, Config: map[string]interface{}{"values": map[string]interface{}{"name": "legacy"}}}}
	container := models.ContainerModel{SchemaName: "orders", Fields: []models.Field{{Name: "name", Type: "string"}}, Workflows: []models.DynamicWorkflow{workflow}}
	if _, err := f.db.Collection("containers").InsertOne(f.ctx, container); err != nil {
		t.Fatal(err)
	}
	scheduled := scheduledWorkflow{Schema: "orders", Workflow: workflow, Container: container}
	at := time.Now().UTC().Truncate(time.Second)
	source := models.WorkflowCronSource{Schema: "orders", WorkflowName: "legacy", Workflows: container.Workflows}
	if _, err := f.repo.EnsureCronSchedule(f.ctx, cronScopeID(scheduled), cronRevision(scheduled), at, source); err != nil {
		t.Fatal(err)
	}
	event := buildCronWorkflowEvent(scheduled, at, at)
	if _, err := f.repo.InsertOutboxEvent(f.ctx, event); err != nil {
		t.Fatal(err)
	}
	claimed, err := f.repo.ClaimPendingOutboxEvent(f.ctx, at.Add(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatchDynamicOutboxEvent(f.ctx, f.repo, claimed, nil); err != nil {
		t.Fatal(err)
	}
	if n, err := f.db.Collection("orders").CountDocuments(f.ctx, bson.M{}); err != nil || n != 1 {
		t.Fatalf("legacy rows=%d %v", n, err)
	}
	if n, err := f.repo.GetProjectCollection("tenant", "project", "orders").CountDocuments(f.ctx, bson.M{}); err != nil || n != 0 {
		t.Fatal("legacy job wrote project data")
	}
}
func TestWorkflowMongoCronRootRollbackKeepsOccurrenceRetryable(t *testing.T) {
	f, event := queuedMongoCron(t, true)
	if err := processCronWorkflowOutbox(f.ctx, f.repo, event); err == nil {
		t.Fatal("failed root was acknowledged")
	}
	if n, err := f.repo.GetProjectCollection("tenant", "project", "orders").CountDocuments(f.ctx, bson.M{}); err != nil || n != 0 {
		t.Fatalf("rollback writes=%d %v", n, err)
	}
	done, err := f.repo.WorkflowStepExecutionDone(f.ctx, event.Payload.IdempotencyKey)
	if err != nil || done {
		t.Fatal("failed root marked done")
	}
	owned, err := f.repo.MarkClaimedOutboxEventFailed(f.ctx, *event, "retry", time.Millisecond)
	if err != nil || !owned {
		t.Fatalf("occurrence could not retry: %v %v", owned, err)
	}
}
func TestWorkflowMongoCronDisabledOrExpiredClaimCannotCommit(t *testing.T) {
	for _, kind := range []string{"disabled", "expired"} {
		t.Run(kind, func(t *testing.T) {
			f, event := queuedMongoCron(t, false)
			if kind == "disabled" {
				f.container.Workflows[0].IsActive = false
				f.saveContainer()
			} else {
				_, err := f.db.Collection("dynamic_outbox").UpdateOne(f.ctx, bson.M{"_id": event.ID}, bson.M{"$set": bson.M{"nextAttemptAt": primitive.NewDateTimeFromTime(time.Now().Add(-time.Minute))}})
				if err != nil {
					t.Fatal(err)
				}
			}
			err := processCronWorkflowOutbox(f.ctx, f.repo, event)
			if kind == "expired" && !errors.Is(err, repositories.ErrCronOutboxClaimLost) {
				t.Fatalf("expired claim error=%v", err)
			}
			if kind == "disabled" && err != nil {
				t.Fatal(err)
			}
			if n, err := f.repo.GetProjectCollection("tenant", "project", "orders").CountDocuments(context.Background(), bson.M{}); err != nil || n != 0 {
				t.Fatalf("forbidden root committed %d records: %v", n, err)
			}
		})
	}
}
