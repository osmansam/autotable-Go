package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
)

type memoryCronQueue struct {
	mu      sync.Mutex
	cursors map[string]models.WorkflowCronSchedule
	events  []models.DynamicOutboxEvent
	fail    bool
}

func (q *memoryCronQueue) EnsureCronSchedule(ctx context.Context, id, revision string, first time.Time, source models.WorkflowCronSource) (models.WorkflowCronSchedule, error) {
	if err := ctx.Err(); err != nil {
		return models.WorkflowCronSchedule{}, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.cursors == nil {
		q.cursors = map[string]models.WorkflowCronSchedule{}
	}
	cursor, exists := q.cursors[id]
	if !exists || cursor.Revision != revision {
		cursor = models.WorkflowCronSchedule{ID: id, Revision: revision, NextAt: first, Source: source}
		q.cursors[id] = cursor
	}
	return cursor, nil
}
func (q *memoryCronQueue) AdvanceCronSchedule(ctx context.Context, old models.WorkflowCronSchedule, next, skipped time.Time, event *models.DynamicOutboxEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.fail {
		return errors.New("queue unavailable")
	}
	current := q.cursors[old.ID]
	if current.Revision != old.Revision || !current.NextAt.Equal(old.NextAt) {
		return repositories.ErrCronScheduleAdvanced
	}
	current.NextAt = next
	if !skipped.IsZero() {
		current.SkippedBefore = skipped
	}
	q.cursors[old.ID] = current
	if event != nil {
		q.events = append(q.events, *event)
	}
	return nil
}
func durableCronFixture() scheduledWorkflow {
	workflow := asyncAuditCronWorkflow("daily")
	workflow.Timezone = "UTC"
	workflow.Schedule = "* * * * *"
	return scheduledWorkflow{TenantID: "t", ProjectID: "p", Schema: "records", Workflow: workflow, Container: models.ContainerModel{SchemaName: "records", Workflows: []models.DynamicWorkflow{workflow}}}
}

func TestDurableCronFirstDiscoveryDoesNotInventPastRuns(t *testing.T) {
	q := &memoryCronQueue{}
	at := time.Date(2026, 10, 19, 12, 0, 30, 0, time.UTC)
	n, err := materializeCronSchedule(context.Background(), q, durableCronFixture(), at, 100)
	if err != nil || n != 0 || len(q.events) != 0 {
		t.Fatalf("initial discovery=%d events=%d err=%v", n, len(q.events), err)
	}
	for _, cursor := range q.cursors {
		if !cursor.NextAt.Equal(time.Date(2026, 10, 19, 12, 1, 0, 0, time.UTC)) {
			t.Fatalf("next=%v", cursor.NextAt)
		}
	}
}
func TestDurableCronRestartRecoversEveryDueNominalInstant(t *testing.T) {
	q := &memoryCronQueue{}
	scheduled := durableCronFixture()
	at := time.Date(2026, 10, 19, 12, 0, 30, 0, time.UTC)
	_, _ = materializeCronSchedule(context.Background(), q, scheduled, at, 100)
	n, err := materializeCronSchedule(context.Background(), q, scheduled, at.Add(3*time.Minute), 100)
	if err != nil || n != 3 || len(q.events) != 3 {
		t.Fatalf("recovery=%d events=%d err=%v", n, len(q.events), err)
	}
	for i, event := range q.events {
		want := time.Date(2026, 10, 19, 12, i+1, 0, 0, time.UTC)
		if !event.Payload.CronScheduledAt.Time().Equal(want) {
			t.Fatalf("occurrence %d=%v", i, event.Payload.CronScheduledAt.Time())
		}
	}
	if n, err := materializeCronSchedule(context.Background(), q, scheduled, at.Add(3*time.Minute), 100); err != nil || n != 0 || len(q.events) != 3 {
		t.Fatal("restart duplicated occurrences")
	}
}
func TestDurableCronQueueFailureDoesNotConsumeOccurrence(t *testing.T) {
	q := &memoryCronQueue{}
	scheduled := durableCronFixture()
	at := time.Date(2026, 10, 19, 12, 0, 30, 0, time.UTC)
	_, _ = materializeCronSchedule(context.Background(), q, scheduled, at, 100)
	q.fail = true
	if _, err := materializeCronSchedule(context.Background(), q, scheduled, at.Add(time.Minute), 100); err == nil {
		t.Fatal("lost durable queue failure")
	}
	q.fail = false
	n, err := materializeCronSchedule(context.Background(), q, scheduled, at.Add(time.Minute), 100)
	if err != nil || n != 1 || len(q.events) != 1 {
		t.Fatal("failed enqueue lost the occurrence")
	}
}
func TestDurableCronCatchupIsBoundedAndSkipsExpiredHistory(t *testing.T) {
	q := &memoryCronQueue{}
	scheduled := durableCronFixture()
	at := time.Date(2026, 10, 16, 12, 0, 30, 0, time.UTC)
	_, _ = materializeCronSchedule(context.Background(), q, scheduled, at, 100)
	now := at.Add(72 * time.Hour)
	n, err := materializeCronSchedule(context.Background(), q, scheduled, now, 2)
	if err != nil || n != 2 || len(q.events) != 2 {
		t.Fatalf("bounded recovery=%d err=%v", n, err)
	}
	cutoff := now.Add(-24 * time.Hour)
	for _, event := range q.events {
		if event.Payload.CronScheduledAt.Time().Before(cutoff) {
			t.Fatal("expired occurrence replayed")
		}
	}
	for _, cursor := range q.cursors {
		if !cursor.SkippedBefore.Equal(cutoff) {
			t.Fatal("expired backlog not recorded")
		}
	}
}
func TestDurableCronDefinitionEditStartsFutureSchedule(t *testing.T) {
	q := &memoryCronQueue{}
	scheduled := durableCronFixture()
	at := time.Date(2026, 10, 19, 12, 0, 30, 0, time.UTC)
	_, _ = materializeCronSchedule(context.Background(), q, scheduled, at, 100)
	scheduled.Workflow.Schedule = "*/5 * * * *"
	n, err := materializeCronSchedule(context.Background(), q, scheduled, at.Add(10*time.Minute), 100)
	if err != nil || n != 0 {
		t.Fatal("new definition ran old missed ticks")
	}
}

func TestDurableCronLeaseCoversTransactionWithoutConcurrentHeartbeatWrites(t *testing.T) {
	ticks := make(chan time.Time, 2)
	ticks <- time.Now()
	ticks <- time.Now()
	renewals := 0
	repo := &asyncAuditOutboxRepository{lease: func(ctx context.Context, _ models.DynamicOutboxEvent, until time.Time) (bool, error) {
		renewals++
		if until.Before(time.Now().Add(time.Minute)) {
			t.Error("cron lease does not cover transaction budget")
		}
		return true, nil
	}}
	ran := false
	err, owned := dispatchDynamicOutboxEventWithLease(context.Background(), repo, &models.DynamicOutboxEvent{Operation: models.DynamicOutboxOperationCronWorkflow}, func(ctx context.Context, _ *models.DynamicOutboxEvent) error {
		ran = true
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 51*time.Second {
			t.Error("cron transaction deadline is missing or too long")
		}
		return nil
	}, ticks)
	if err != nil || !owned || !ran || renewals != 1 {
		t.Fatalf("err=%v owned=%v ran=%v renewals=%d", err, owned, ran, renewals)
	}
}

func TestDurableCronPollingBoundsTotalBacklogAndRotatesFairly(t *testing.T) {
	q := &memoryCronQueue{}
	start := time.Date(2026, 10, 19, 0, 0, 30, 0, time.UTC)
	var workflows []scheduledWorkflow
	for i := 0; i < 6; i++ {
		scheduled := durableCronFixture()
		scheduled.Workflow.Name = fmt.Sprintf("workflow-%d", i)
		scheduled.Container.Workflows = []models.DynamicWorkflow{scheduled.Workflow}
		workflows = append(workflows, scheduled)
		if _, err := materializeCronSchedule(context.Background(), q, scheduled, start, 100); err != nil {
			t.Fatal(err)
		}
	}
	scheduler := &dynamicCronScheduler{queue: q, workflows: workflows}
	now := start.Add(200 * time.Minute)
	if err := scheduler.poll(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if len(q.events) != 500 {
		t.Fatalf("poll materialized %d events instead of the bounded 500", len(q.events))
	}
	if err := scheduler.poll(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	last := 0
	for _, event := range q.events {
		if event.Payload.WorkflowName == "workflow-5" {
			last++
		}
	}
	if last == 0 {
		t.Fatal("first workflows starved the last workflow's backlog")
	}
}
