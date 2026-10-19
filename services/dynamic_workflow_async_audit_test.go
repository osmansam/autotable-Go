package services

import (
	"context"
	"errors"
	"fmt"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/repositories"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestAsyncOutboxSnapshotsDoNotAliasWorkflowState(t *testing.T) {
	id := primitive.NewObjectID()
	source := map[string]interface{}{"nested": map[string]interface{}{"value": "original"}, "id": id}
	payload := workflowExecutionPayload{
		IdentityKind: AccessIdentityCaller, UserID: "user", UserRole: "editor", UserRoles: []string{"editor"},
		TenantID: "tenant", ProjectID: "project", SchemaName: "orders", WorkflowTrigger: models.WorkflowTriggerAfterUpdate, WorkflowVersion: 3,
		Record: source, OldRecord: source, StepOutputs: source, Variables: source, Loop: source, CurrentUser: source,
	}
	step := models.DynamicWorkflowStep{ID: "step", Type: models.WorkflowStepTypeIf, Config: source,
		Steps:     []models.DynamicWorkflowStep{{Name: "nested", Config: source}},
		ElseSteps: []models.DynamicWorkflowStep{{Name: "else", Config: source}},
		Branches:  []models.WorkflowBranch{{Name: "branch", Conditions: []models.WorkflowCondition{{Operator: "=", Value: source}}, Steps: []models.DynamicWorkflowStep{{Config: source}}}},
	}
	event := buildWorkflowStepOutboxEvent(payload, "notify", step)
	source["nested"].(map[string]interface{})["value"] = "changed"
	payload.UserRoles[0] = "admin"
	step.Steps[0].Name = "changed"
	step.ElseSteps[0].Name = "changed"
	step.Branches[0].Name = "changed"
	for name, snapshot := range map[string]map[string]interface{}{
		"record": event.Payload.Record, "oldRecord": event.Payload.OldRecord, "outputs": event.Payload.StepOutputs,
		"variables": event.Payload.Variables, "loop": event.Payload.Loop, "currentUser": event.Payload.CurrentUser,
		"config": event.Payload.Config, "nestedConfig": event.Payload.Steps[0].Config,
		"elseConfig": event.Payload.ElseSteps[0].Config, "branchConfig": event.Payload.Branches[0].Steps[0].Config,
		"branchCondition": event.Payload.Branches[0].Conditions[0].Value.(map[string]interface{}),
	} {
		if snapshot["nested"].(map[string]interface{})["value"] != "original" || snapshot["id"] != id {
			t.Errorf("%s snapshot changed or lost ObjectID type: %#v", name, snapshot)
		}
	}
	if event.Payload.Steps[0].Name != "nested" || event.Payload.ElseSteps[0].Name != "else" || event.Payload.Branches[0].Name != "branch" {
		t.Fatal("nested workflow definitions alias source slices")
	}
	if !reflect.DeepEqual(event.Payload.UserRoles, []string{"editor"}) {
		t.Fatalf("roles = %#v", event.Payload.UserRoles)
	}
	if event.Payload.IdentityKind != AccessIdentityCaller || event.Payload.UserID != "user" || event.Payload.UserRole != "editor" {
		t.Fatal("caller identity was lost")
	}
}

func TestAsyncOutboxReplayDoesNotMutatePersistedSnapshot(t *testing.T) {
	event := models.DynamicOutboxEvent{ID: primitive.NewObjectID(), TenantID: "tenant", ProjectID: "project", SchemaName: "orders", Payload: models.DynamicOutboxPayload{
		IdentityKind: AccessIdentityCaller, UserID: "user", UserRole: "editor", UserRoles: []string{"editor"},
		WorkflowName: "notify", WorkflowTrigger: models.WorkflowTriggerAfterUpdate, WorkflowVersion: 3, WorkflowDepth: 2, IdempotencyKey: "key", StopOnError: true,
	}}
	for _, target := range []*map[string]interface{}{&event.Payload.Record, &event.Payload.OldRecord, &event.Payload.StepOutputs, &event.Payload.Variables, &event.Payload.Loop, &event.Payload.CurrentUser} {
		*target = map[string]interface{}{"nested": map[string]interface{}{"value": "original"}}
	}
	restored := workflowPayloadFromOutboxEvent(&event)
	for _, snapshot := range []map[string]interface{}{restored.Record, restored.OldRecord, restored.StepOutputs, restored.Variables, restored.Loop, restored.CurrentUser} {
		snapshot["nested"].(map[string]interface{})["value"] = "replayed"
	}
	restored.UserRoles[0] = "admin"
	for name, snapshot := range map[string]map[string]interface{}{"record": event.Payload.Record, "old": event.Payload.OldRecord, "outputs": event.Payload.StepOutputs, "vars": event.Payload.Variables, "loop": event.Payload.Loop, "user": event.Payload.CurrentUser} {
		if snapshot["nested"].(map[string]interface{})["value"] != "original" {
			t.Errorf("replay mutated %s snapshot", name)
		}
	}
	if restored.IdentityKind != AccessIdentityCaller || restored.UserID != "user" || restored.UserRole != "editor" || event.Payload.UserRoles[0] != "editor" {
		t.Fatal("caller identity was not preserved independently")
	}
	if restored.OutboxEventID != event.ID || restored.WorkflowDepth != 2 || restored.IdempotencyKey != "key" || !restored.StopOnError || restored.WorkflowName != "notify" || restored.WorkflowVersion != 3 {
		t.Fatalf("execution context = %#v", restored)
	}
}

func TestAsyncCronFingerprintIncludesCapturedContainer(t *testing.T) {
	original := scheduledWorkflow{TenantID: "tenant", ProjectID: "project", Schema: "orders", Workflow: models.DynamicWorkflow{Name: "nightly", Trigger: models.WorkflowTriggerCron, Schedule: "0 2 * * *"}, Container: models.ContainerModel{SchemaName: "orders", Redis: models.Redis{TriggeredRedisCaches: []string{"reports"}}}}
	changed := original
	changed.Container.Redis.TriggeredRedisCaches = []string{"dashboard"}
	if scheduledWorkflowFingerprint([]scheduledWorkflow{original}) == scheduledWorkflowFingerprint([]scheduledWorkflow{changed}) {
		t.Fatal("container change must reload cron's captured execution context")
	}
}

func TestAsyncCronPayloadUsesUTCInstantAndDefaultVersion(t *testing.T) {
	instant := time.Date(2026, 10, 18, 1, 2, 3, 456000000, time.FixedZone("local", -5*60*60))
	payload := newCronWorkflowPayload("tenant", "project", "orders", &models.ContainerModel{SchemaName: "orders"}, models.DynamicWorkflow{}, instant)
	if payload.Record["triggeredAt"] != "2026-10-18T06:02:03.456Z" || payload.Record["_id"] != "2026-10-18T06:02:03.456Z" {
		t.Fatalf("cron instant loses precision/timezone: %#v", payload.Record)
	}
	if payload.IdentityKind != AccessIdentitySystem || payload.UserID != "" || payload.WorkflowTrigger != models.WorkflowTriggerCron || payload.WorkflowVersion != 1 {
		t.Fatalf("cron execution identity/version = %#v", payload)
	}
}

func TestAsyncOutboxRetryDelayCannotOverflow(t *testing.T) {
	for _, attempts := range []int{1 << 32, int(^uint(0) >> 1)} {
		if got := outboxRetryDelay(attempts); got != time.Minute {
			t.Errorf("outboxRetryDelay(%d) = %s, want one minute", attempts, got)
		}
	}
}

type asyncAuditOutboxRepository struct {
	claims         []*models.DynamicOutboxEvent
	claimCalls     int
	recoveryCalls  int
	doneIDs        []primitive.ObjectID
	failedAttempts []int
	retryDelays    []time.Duration
	lease          func(context.Context, models.DynamicOutboxEvent, time.Time) (bool, error)
	claimError     error
	recoveryError  error
	doneError      error
	failedError    error
}

func (r *asyncAuditOutboxRepository) RecoverExhaustedOutboxEvents(context.Context, time.Time) error {
	r.recoveryCalls++
	return r.recoveryError
}
func (r *asyncAuditOutboxRepository) ClaimPendingOutboxEvent(context.Context, time.Time) (*models.DynamicOutboxEvent, error) {
	r.claimCalls++
	if r.claimError != nil {
		return nil, r.claimError
	}
	if len(r.claims) == 0 {
		return nil, mongo.ErrNoDocuments
	}
	event := r.claims[0]
	r.claims = r.claims[1:]
	return event, nil
}
func (r *asyncAuditOutboxRepository) RenewOutboxEventLease(ctx context.Context, event models.DynamicOutboxEvent, until time.Time) (bool, error) {
	return r.lease(ctx, event, until)
}
func (r *asyncAuditOutboxRepository) MarkClaimedOutboxEventDone(_ context.Context, event models.DynamicOutboxEvent) (bool, error) {
	r.doneIDs = append(r.doneIDs, event.ID)
	return r.doneError == nil, r.doneError
}
func (r *asyncAuditOutboxRepository) MarkClaimedOutboxEventFailed(_ context.Context, event models.DynamicOutboxEvent, _ string, delay time.Duration) (bool, error) {
	r.failedAttempts = append(r.failedAttempts, event.Attempts)
	r.retryDelays = append(r.retryDelays, delay)
	return r.failedError == nil, r.failedError
}

func TestAsyncOutboxLeaseRenewalPreservesOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	event := &models.DynamicOutboxEvent{ID: primitive.NewObjectID(), Attempts: 2, MaxAttempts: 3}
	ticks := make(chan time.Time)
	started, release, renewed := make(chan struct{}), make(chan struct{}), make(chan time.Time, 1)
	repository := &asyncAuditOutboxRepository{lease: func(_ context.Context, claimed models.DynamicOutboxEvent, until time.Time) (bool, error) {
		if claimed.ID != event.ID || claimed.Attempts != 2 {
			t.Errorf("renewed wrong claim: %#v", claimed)
		}
		renewed <- until
		return true, nil
	}}
	type outcome struct {
		err   error
		owned bool
	}
	result := make(chan outcome, 1)
	go func() {
		err, owned := dispatchDynamicOutboxEventWithLease(ctx, repository, event, func(ctx context.Context, _ *models.DynamicOutboxEvent) error {
			close(started)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}, ticks)
		result <- outcome{err, owned}
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("dispatch never started")
	}
	tick := time.Now()
	select {
	case ticks <- tick:
	case <-ctx.Done():
		t.Fatal("renewal never read")
	}
	select {
	case until := <-renewed:
		if until.Before(tick.Add(30 * time.Second)) {
			t.Fatalf("lease ends too early: %v", until)
		}
	case <-ctx.Done():
		t.Fatal("lease not renewed")
	}
	close(release)
	select {
	case got := <-result:
		if got.err != nil || !got.owned {
			t.Fatalf("dispatch = %#v", got)
		}
	case <-ctx.Done():
		t.Fatal("dispatch did not return")
	}
}

func TestAsyncOutboxLostLeaseCancelsDispatch(t *testing.T) {
	for _, renewError := range []error{nil, errors.New("database unavailable")} {
		t.Run(fmt.Sprint(renewError), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			ticks := make(chan time.Time)
			started, canceled := make(chan struct{}), make(chan struct{})
			repository := &asyncAuditOutboxRepository{lease: func(context.Context, models.DynamicOutboxEvent, time.Time) (bool, error) { return false, renewError }}
			result := make(chan bool, 1)
			go func() {
				err, owned := dispatchDynamicOutboxEventWithLease(ctx, repository, &models.DynamicOutboxEvent{}, func(ctx context.Context, _ *models.DynamicOutboxEvent) error {
					close(started)
					<-ctx.Done()
					close(canceled)
					return ctx.Err()
				}, ticks)
				if err == nil {
					t.Error("lost claim must return an error")
				}
				result <- owned
			}()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("dispatch did not start")
			}
			select {
			case ticks <- time.Now():
			case <-ctx.Done():
				t.Fatal("renewal not attempted")
			}
			select {
			case owned := <-result:
				if owned {
					t.Fatal("lost claim can still acknowledge event")
				}
			case <-ctx.Done():
				t.Fatal("lease loss did not return")
			}
			select {
			case <-canceled:
			case <-ctx.Done():
				t.Fatal("lost claim did not cancel dispatch")
			}
		})
	}
}

func TestAsyncOutboxBatchAcknowledgesAndRetriesClaimedAttempts(t *testing.T) {
	success := &models.DynamicOutboxEvent{ID: primitive.NewObjectID(), Attempts: 1, MaxAttempts: 3}
	failure := &models.DynamicOutboxEvent{ID: primitive.NewObjectID(), Attempts: 3, MaxAttempts: 3}
	repository := &asyncAuditOutboxRepository{claims: []*models.DynamicOutboxEvent{success, failure}}
	dispatches := 0
	processDynamicOutboxBatchWithDispatch(context.Background(), repository, func(_ context.Context, event *models.DynamicOutboxEvent) error {
		dispatches++
		if event.ID == failure.ID {
			return errors.New("upstream unavailable")
		}
		return nil
	}, 2)
	if dispatches != 2 || repository.claimCalls != 2 || repository.recoveryCalls != 1 {
		t.Fatalf("batch dispatched %d, claimed %d, recovered %d", dispatches, repository.claimCalls, repository.recoveryCalls)
	}
	if !reflect.DeepEqual(repository.doneIDs, []primitive.ObjectID{success.ID}) || !reflect.DeepEqual(repository.failedAttempts, []int{3}) || !reflect.DeepEqual(repository.retryDelays, []time.Duration{9 * time.Second}) {
		t.Fatalf("ack/retry state = %#v", repository)
	}
}

func TestAsyncOutboxBatchStopsOnIdleErrorsAndCancellation(t *testing.T) {
	for _, test := range []struct {
		name                     string
		repository               asyncAuditOutboxRepository
		cancel                   bool
		wantClaims, wantRecovery int
	}{
		{name: "idle", wantClaims: 1, wantRecovery: 1},
		{name: "claim error", repository: asyncAuditOutboxRepository{claimError: errors.New("read unavailable")}, wantClaims: 1, wantRecovery: 1},
		{name: "recovery error", repository: asyncAuditOutboxRepository{recoveryError: errors.New("write unavailable")}, wantRecovery: 1},
		{name: "canceled", cancel: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.cancel {
				cancel()
			}
			processDynamicOutboxBatchWithDispatch(ctx, &test.repository, func(context.Context, *models.DynamicOutboxEvent) error { t.Fatal("empty batch dispatched"); return nil }, 25)
			if test.repository.claimCalls != test.wantClaims || test.repository.recoveryCalls != test.wantRecovery {
				t.Fatalf("claims/recovery = %d/%d", test.repository.claimCalls, test.repository.recoveryCalls)
			}
		})
	}
}

type asyncAuditCronRepository struct {
	containers []repositories.ScheduledWorkflowContainer
	err        error
}

func (r *asyncAuditCronRepository) GetScheduledWorkflowContainers(context.Context) ([]repositories.ScheduledWorkflowContainer, error) {
	return r.containers, r.err
}

func asyncAuditCronWorkflow(name string) models.DynamicWorkflow {
	return models.DynamicWorkflow{Name: name, Trigger: models.WorkflowTriggerCron, Schedule: "0 9 * * *", Timezone: "America/Chicago", Mode: models.WorkflowModeTransactional, IsActive: true, Steps: []models.DynamicWorkflowStep{{Name: "value", Type: models.WorkflowStepTypeSetVariable, IsActive: true, Config: map[string]interface{}{"name": "result", "value": true}}}}
}

func TestAsyncCronLoadsOnlyValidActiveSchedulesInScopeOrder(t *testing.T) {
	valid := asyncAuditCronWorkflow("daily")
	inactive := valid
	inactive.Name = "disabled"
	inactive.IsActive = false
	manual := valid
	manual.Name = "manual"
	manual.Trigger = models.WorkflowTriggerManual
	manual.Schedule = ""
	manual.Timezone = ""
	invalid := valid
	invalid.Name = "invalid"
	invalid.Schedule = "invalid"
	repository := &asyncAuditCronRepository{containers: []repositories.ScheduledWorkflowContainer{
		{TenantID: "z", ProjectID: "project", Container: models.ContainerModel{SchemaName: "orders", Workflows: []models.DynamicWorkflow{valid, inactive, manual, invalid}}},
		{TenantID: "a", ProjectID: "project", Container: models.ContainerModel{SchemaName: "orders", Workflows: []models.DynamicWorkflow{valid}}},
	}}
	scheduler := &dynamicCronScheduler{repository: repository}
	workflows, err := scheduler.loadWorkflows(context.Background())
	if err != nil || len(workflows) != 2 {
		t.Fatalf("loaded schedules = %#v, %v", workflows, err)
	}
	if workflows[0].TenantID != "a" || workflows[1].TenantID != "z" || workflows[0].Schema != "orders" {
		t.Fatalf("scope/order = %#v", workflows)
	}
	repository.err = errors.New("registry offline")
	if _, err := scheduler.loadWorkflows(context.Background()); err == nil {
		t.Fatal("registry failure ignored")
	}
}

func TestAsyncCronReloadKeepsCursorsAndRefreshesMetadata(t *testing.T) {
	repository := &asyncAuditCronRepository{containers: []repositories.ScheduledWorkflowContainer{{TenantID: "tenant", ProjectID: "project", Container: models.ContainerModel{SchemaName: "orders", Workflows: []models.DynamicWorkflow{asyncAuditCronWorkflow("daily")}}}}}
	scheduler := &dynamicCronScheduler{repository: repository, queue: &memoryCronQueue{}}
	if err := scheduler.reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.workflows) != 1 {
		t.Fatal("active schedule missing")
	}
	next, err := parseCronSchedule(scheduler.workflows[0].Workflow)
	if err != nil {
		t.Fatal(err)
	}
	if at := next.Next(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)); !at.Equal(time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC)) {
		t.Fatalf("Chicago schedule=%v", at)
	}
	original := scheduler.fingerprint
	if err := scheduler.reload(context.Background()); err != nil || scheduler.fingerprint != original {
		t.Fatal("unchanged reload changed schedule identity")
	}
	repository.containers[0].Container.Redis.TriggeredRedisCaches = []string{"dashboard"}
	if err := scheduler.reload(context.Background()); err != nil || scheduler.fingerprint == original {
		t.Fatal("changed metadata was not reloaded")
	}
	original = scheduler.fingerprint
	repository.err = errors.New("registry offline")
	if err := scheduler.reload(context.Background()); err == nil || scheduler.fingerprint != original || len(scheduler.workflows) != 1 {
		t.Fatal("failed reload discarded schedules")
	}
	repository.err = nil
	repository.containers = nil
	if err := scheduler.reload(context.Background()); err != nil || len(scheduler.workflows) != 0 {
		t.Fatal("removed schedule remains")
	}
}
func TestAsyncCronStopPreventsFutureMaterialization(t *testing.T) {
	queue := &memoryCronQueue{}
	scheduler := &dynamicCronScheduler{queue: queue, workflows: []scheduledWorkflow{durableCronFixture()}}
	scheduler.stop()
	scheduler.stop()
	if err := scheduler.poll(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(queue.cursors) != 0 || len(queue.events) != 0 {
		t.Fatal("stopped scheduler materialized work")
	}
}
func TestAsyncCronCanceledPollDoesNotAdvanceOrEnqueue(t *testing.T) {
	queue := &memoryCronQueue{}
	scheduler := &dynamicCronScheduler{queue: queue, workflows: []scheduledWorkflow{durableCronFixture()}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := scheduler.poll(ctx, time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled poll=%v", err)
	}
	if len(queue.cursors) != 0 || len(queue.events) != 0 {
		t.Fatal("canceled scheduler touched durable state")
	}
}

func asyncAuditRedis(t *testing.T) {
	t.Helper()
	server := miniredis.RunT(t)
	original := configs.RedisClient
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	configs.RedisClient = client
	t.Cleanup(func() { configs.RedisClient = original; _ = client.Close() })
}

func TestAsyncOutboxRoundTripPreservesQueryAndAuditIdentity(t *testing.T) {
	userID := primitive.NewObjectID()
	query := map[string]interface{}{"filter": map[string]interface{}{"region": "west"}}
	audit := &models.AuditUser{ID: userID, Email: "editor@example.com", DisplayName: "Editor", Roles: []string{"editor"}}
	event := buildWorkflowStepOutboxEvent(workflowExecutionPayload{Query: query, AuditUser: audit, IdentityKind: AccessIdentityCaller, UserID: userID.Hex()}, "notify", models.DynamicWorkflowStep{ID: "step", Type: models.WorkflowStepTypeAuditLog})
	query["filter"].(map[string]interface{})["region"] = "east"
	audit.Roles[0] = "admin"
	restored := workflowPayloadFromOutboxEvent(&event)
	if restored.Query == nil || restored.Query["filter"].(map[string]interface{})["region"] != "west" {
		t.Errorf("async query snapshot was lost/changed: %#v", restored.Query)
	}
	if restored.AuditUser == nil {
		t.Fatal("async workflow lost audit caller identity")
	}
	if restored.AuditUser.ID != userID || restored.AuditUser.Email != "editor@example.com" || restored.AuditUser.DisplayName != "Editor" || !reflect.DeepEqual(restored.AuditUser.Roles, []string{"editor"}) {
		t.Fatalf("audit snapshot = %#v", restored.AuditUser)
	}
	restored.Query["filter"].(map[string]interface{})["region"] = "replayed"
	restored.AuditUser.Roles[0] = "replayed"
	again := workflowPayloadFromOutboxEvent(&event)
	if again.Query["filter"].(map[string]interface{})["region"] != "west" || again.AuditUser.Roles[0] != "editor" {
		t.Fatal("async replay mutated persisted query/audit identity")
	}
}

func TestAsyncCronRequiresDurableQueue(t *testing.T) {
	scheduler := &dynamicCronScheduler{workflows: []scheduledWorkflow{durableCronFixture()}}
	if err := scheduler.poll(context.Background(), time.Now()); err == nil {
		t.Fatal("scheduler allowed a missing durable queue")
	}
}

func TestAsyncOutboxLeaseRenewalIsBoundedBeforeClaimExpires(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ticks := make(chan time.Time)
	started, renewed := make(chan struct{}), make(chan struct{})
	repository := &asyncAuditOutboxRepository{lease: func(ctx context.Context, _ models.DynamicOutboxEvent, _ time.Time) (bool, error) {
		deadline, ok := ctx.Deadline()
		// A renewal must have its own deadline even when dispatch has no deadline.
		if !ok || time.Until(deadline) > 11*time.Second {
			t.Error("database renewal may block past the 30-second lease")
		}
		close(renewed)
		return false, nil
	}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = dispatchDynamicOutboxEventWithLease(context.Background(), repository, &models.DynamicOutboxEvent{}, func(ctx context.Context, _ *models.DynamicOutboxEvent) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}, ticks)
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("dispatch did not start")
	}
	select {
	case ticks <- time.Now():
	case <-ctx.Done():
		t.Fatal("renewal did not start")
	}
	select {
	case <-renewed:
	case <-ctx.Done():
		t.Fatal("renewal did not run")
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("lease loss did not end dispatch")
	}
}

func TestAsyncOutboxShutdownCancelsInFlightDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, canceled := make(chan struct{}), make(chan struct{})
	result := make(chan bool, 1)
	go func() {
		err, owned := dispatchDynamicOutboxEventWithLease(ctx, &asyncAuditOutboxRepository{}, &models.DynamicOutboxEvent{}, func(ctx context.Context, _ *models.DynamicOutboxEvent) error {
			close(started)
			<-ctx.Done()
			close(canceled)
			return ctx.Err()
		}, make(chan time.Time))
		if !errors.Is(err, context.Canceled) {
			t.Errorf("shutdown = %v", err)
		}
		result <- owned
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("dispatch did not start")
	}
	cancel()
	select {
	case owned := <-result:
		if owned {
			t.Fatal("canceled worker can acknowledge")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not return")
	}
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not cancel step")
	}
}

func TestAsyncOutboxDefaultIdempotencyIsPerEvent(t *testing.T) {
	payload := workflowExecutionPayload{TenantID: "tenant", ProjectID: "project", SchemaName: "orders", Record: map[string]interface{}{"_id": "record"}, WorkflowTrigger: models.WorkflowTriggerAfterUpdate, WorkflowVersion: 2}
	step := models.DynamicWorkflowStep{ID: "notify", Type: models.WorkflowStepTypeCallAPI}
	first := buildWorkflowStepOutboxEvent(payload, "workflow", step)
	second := buildWorkflowStepOutboxEvent(payload, "workflow", step)
	if first.Payload.IdempotencyKey == "" || first.Payload.IdempotencyKey == second.Payload.IdempotencyKey {
		t.Fatalf("independent record updates share key: %q, %q", first.Payload.IdempotencyKey, second.Payload.IdempotencyKey)
	}
	if !strings.Contains(first.Payload.IdempotencyKey, first.ID.Hex()) {
		t.Fatalf("default key is not tied to durable event identity: %q", first.Payload.IdempotencyKey)
	}
	payload.Record = nil
	if got := buildWorkflowStepOutboxEvent(payload, "workflow", step); got.Payload.IdempotencyKey == "" {
		t.Fatal("recordless manual invocation lacks a replay identity")
	}
}

func TestAsyncOutboxDescendantEnqueueIsStableAcrossReplay(t *testing.T) {
	payload := workflowExecutionPayload{TenantID: "tenant", ProjectID: "project", SchemaName: "orders", OutboxEventID: primitive.NewObjectID(), IdempotencyKey: "parent-event"}
	firstStep := models.DynamicWorkflowStep{ID: "first", Type: models.WorkflowStepTypeCallAPI}
	first := buildWorkflowStepOutboxEvent(payload, "child", firstStep)
	replay := buildWorkflowStepOutboxEvent(payload, "child", firstStep)
	other := buildWorkflowStepOutboxEvent(payload, "child", models.DynamicWorkflowStep{ID: "second", Type: models.WorkflowStepTypeCallAPI})
	if first.Payload.IdempotencyKey == "" || first.Payload.IdempotencyKey != replay.Payload.IdempotencyKey || first.Payload.IdempotencyKey == other.Payload.IdempotencyKey {
		t.Fatalf("descendant keys first/replay/other = %q/%q/%q", first.Payload.IdempotencyKey, replay.Payload.IdempotencyKey, other.Payload.IdempotencyKey)
	}
}

func TestAsyncOutboxNestedCreateKeysSeparateStepsAndIterations(t *testing.T) {
	first := workflowScopedStepIdempotencyKey("event", "workflow", models.DynamicWorkflowStep{ID: "create"}, 0)
	second := workflowScopedStepIdempotencyKey("event", "workflow", models.DynamicWorkflowStep{ID: "create"}, 1)
	if first != "event:step:0:workflow:create" || second != "event:step:1:workflow:create" {
		t.Fatalf("step keys = %q/%q", first, second)
	}
	keys := map[string]bool{}
	for iteration := 0; iteration < 2; iteration++ {
		base := workflowLoopIdempotencyKey("event", iteration)
		for position := 0; position < 2; position++ {
			key := workflowScopedStepIdempotencyKey(base, "workflow", models.DynamicWorkflowStep{ID: "create"}, position)
			if keys[key] {
				t.Fatalf("nested create shares an idempotency key: %q", key)
			}
			keys[key] = true
			if key != workflowScopedStepIdempotencyKey(workflowLoopIdempotencyKey("event", iteration), "workflow", models.DynamicWorkflowStep{ID: "create"}, position) {
				t.Fatal("replay changed nested create key")
			}
		}
	}
	if workflowLoopIdempotencyKey("", 0) != "" || workflowScopedStepIdempotencyKey("", "workflow", models.DynamicWorkflowStep{}, 0) != "" {
		t.Fatal("transactional work without event identity acquired a dedupe key")
	}
	a := workflowScopedStepIdempotencyKey("event", "a:b", models.DynamicWorkflowStep{ID: "c"}, 0)
	b := workflowScopedStepIdempotencyKey("event", "a", models.DynamicWorkflowStep{ID: "b:c"}, 0)
	if a == b {
		t.Fatal("scope delimiter collision")
	}
}

func TestAsyncNestedExecutionRestoresParentIdempotencyKey(t *testing.T) {
	for _, failure := range []bool{false, true} {
		payload := workflowExecutionPayload{IdempotencyKey: "parent", OutboxEventID: primitive.NewObjectID()}
		step := models.DynamicWorkflowStep{Name: "step", Type: models.WorkflowStepTypeSetVariable, IsActive: true, Config: map[string]interface{}{"name": "value", "value": true}}
		if failure {
			step.Type = models.WorkflowStepTypeFail
		}
		_ = (&DynamicService{}).runWorkflowStepList(context.Background(), &payload, "workflow", "", models.WorkflowModeTransactional, true, []models.DynamicWorkflowStep{step})
		if payload.IdempotencyKey != "parent" {
			t.Fatalf("parent key not restored after failure=%v: %q", failure, payload.IdempotencyKey)
		}
	}
}

func TestAsyncCapturedOutboxControlFlowExecutesExplicitModeChildren(t *testing.T) {
	set := models.DynamicWorkflowStep{Name: "set", Type: models.WorkflowStepTypeSetVariable, IsActive: true, ExecutionMode: models.WorkflowModeOutbox, Config: map[string]interface{}{"values": map[string]interface{}{"ran": true}}}
	for _, kind := range []string{models.WorkflowStepTypeIf, models.WorkflowStepTypeForEach} {
		t.Run(kind, func(t *testing.T) {
			event := &models.DynamicOutboxEvent{ID: primitive.NewObjectID(), Payload: models.DynamicOutboxPayload{WorkflowName: "captured", IdempotencyKey: "event", StepType: kind, Steps: []models.DynamicWorkflowStep{set}, Config: map[string]interface{}{"items": []interface{}{"one", "two"}}}}
			step := workflowStepFromOutboxEvent(event)
			payload := workflowPayloadFromOutboxEvent(event)
			if _, err := (&DynamicService{}).processWorkflowStepWithTimeout(context.Background(), step, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Variables["ran"] != true {
				t.Fatalf("explicit outbox child was skipped: %#v", payload.Variables)
			}
			if event.Payload.Steps[0].ExecutionMode != models.WorkflowModeOutbox {
				t.Fatal("replay normalization mutated persisted snapshot")
			}
		})
	}
}

func TestAsyncOutboxBatchAcknowledgmentErrorsDoNotRetryDispatchInSameClaim(t *testing.T) {
	for _, failedDispatch := range []bool{false, true} {
		t.Run(fmt.Sprint(failedDispatch), func(t *testing.T) {
			event := &models.DynamicOutboxEvent{ID: primitive.NewObjectID(), Attempts: 2, MaxAttempts: 3}
			repository := &asyncAuditOutboxRepository{claims: []*models.DynamicOutboxEvent{event}, doneError: errors.New("write failed"), failedError: errors.New("write failed")}
			dispatches := 0
			processDynamicOutboxBatchWithDispatch(context.Background(), repository, func(context.Context, *models.DynamicOutboxEvent) error {
				dispatches++
				if failedDispatch {
					return errors.New("upstream failed")
				}
				return nil
			}, 25)
			if dispatches != 1 || repository.claimCalls != 2 {
				t.Fatalf("dispatches/claims = %d/%d", dispatches, repository.claimCalls)
			}
			if failedDispatch && len(repository.failedAttempts) != 1 {
				t.Fatal("dispatch failure was not recorded")
			}
			if !failedDispatch && (len(repository.doneIDs) != 1 || len(repository.failedAttempts) != 0) {
				t.Fatal("done-write failure incorrectly reran or failed successful operation")
			}
		})
	}
}

func TestAsyncCapturedOutboxElseAndBranchesRetainLocalExecution(t *testing.T) {
	set := models.DynamicWorkflowStep{Name: "set", Type: models.WorkflowStepTypeSetVariable, IsActive: true, ExecutionMode: models.WorkflowModeOutbox, Config: map[string]interface{}{"values": map[string]interface{}{"ran": true}}}
	for _, branchForm := range []bool{false, true} {
		t.Run(fmt.Sprint(branchForm), func(t *testing.T) {
			nested := models.DynamicWorkflowStep{Name: "nested", Type: models.WorkflowStepTypeIf, IsActive: true, ExecutionMode: models.WorkflowModeOutbox, Steps: []models.DynamicWorkflowStep{set}}
			event := &models.DynamicOutboxEvent{ID: primitive.NewObjectID(), Payload: models.DynamicOutboxPayload{StepType: models.WorkflowStepTypeIf, IdempotencyKey: "event"}}
			if branchForm {
				event.Payload.Branches = []models.WorkflowBranch{{Name: "branch", Steps: []models.DynamicWorkflowStep{nested}}}
			} else {
				// Captured root conditions were already evaluated when the event was queued;
				// an empty first branch's children are explicitly inactive here.
				event.Payload.Steps = []models.DynamicWorkflowStep{{Name: "inactive", Type: models.WorkflowStepTypeSetVariable}}
				event.Payload.Branches = []models.WorkflowBranch{{Name: "skip", Conditions: []models.WorkflowCondition{{Field: "record.state", Operator: "=", Value: "missing"}}, Steps: []models.DynamicWorkflowStep{set}}, {Name: "else", Steps: []models.DynamicWorkflowStep{nested}}}
			}
			payload := workflowPayloadFromOutboxEvent(event)
			if _, err := (&DynamicService{}).processWorkflowStepWithTimeout(context.Background(), workflowStepFromOutboxEvent(event), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Variables["ran"] != true {
				t.Fatalf("branch/nested explicit outbox work skipped: %#v", payload.Variables)
			}
			if event.Payload.Branches[len(event.Payload.Branches)-1].Steps[0].ExecutionMode != models.WorkflowModeOutbox {
				t.Fatal("mode normalization mutated persisted branches")
			}
		})
	}
}

func TestAsyncCapturedIfPreservesConditionForElseBranch(t *testing.T) {
	step := models.DynamicWorkflowStep{Name: "check", Type: models.WorkflowStepTypeIf, Conditions: []models.WorkflowCondition{{Field: "record.status", Operator: "=", Value: "new"}},
		Steps:     []models.DynamicWorkflowStep{{Name: "if", Type: models.WorkflowStepTypeSetVariable, IsActive: true, Config: map[string]interface{}{"values": map[string]interface{}{"branch": "if"}}}},
		ElseSteps: []models.DynamicWorkflowStep{{Name: "else", Type: models.WorkflowStepTypeSetVariable, IsActive: true, Config: map[string]interface{}{"values": map[string]interface{}{"branch": "else"}}}},
	}
	event := buildWorkflowStepOutboxEvent(workflowExecutionPayload{Record: map[string]interface{}{"status": "old"}}, "workflow", step)
	payload := workflowPayloadFromOutboxEvent(&event)
	if _, err := (&DynamicService{}).processWorkflowStepWithTimeout(context.Background(), workflowStepFromOutboxEvent(&event), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Variables["branch"] != "else" {
		t.Fatalf("captured if lost condition and selected %v", payload.Variables["branch"])
	}
}

func TestAsyncNestedAuditEventIdentitiesAreDistinctAndReplayStable(t *testing.T) {
	eventID := primitive.NewObjectID()
	first := workflowExecutionPayload{OutboxEventID: eventID, IdempotencyKey: "event:step:0:workflow:audit"}
	second := workflowExecutionPayload{OutboxEventID: eventID, IdempotencyKey: "event:step:1:workflow:audit"}
	firstID := workflowAuditEventID(first)
	if firstID == primitive.NilObjectID || firstID != workflowAuditEventID(first) || firstID == workflowAuditEventID(second) {
		t.Fatal("nested audit events collapse or change across retry")
	}
	loop := first
	loop.IdempotencyKey = workflowLoopIdempotencyKey(first.IdempotencyKey, 1)
	if firstID == workflowAuditEventID(loop) {
		t.Fatal("loop audit entries collapse")
	}
	if got := workflowAuditEventID(workflowExecutionPayload{OutboxEventID: eventID}); got != eventID {
		t.Fatalf("legacy replay audit identity = %v", got)
	}
	if got := workflowAuditEventID(workflowExecutionPayload{}); got != primitive.NilObjectID {
		t.Fatalf("transactional audit acquired a replay identity: %v", got)
	}
}

func TestAsyncNestedBranchesHaveIndependentReplayIdentities(t *testing.T) {
	first := workflowBranchIdempotencyKey("event", 0, "same")
	second := workflowBranchIdempotencyKey("event", 1, "same")
	step := models.DynamicWorkflowStep{Name: "create"}
	if first != "event:branch:0:same" || second != "event:branch:1:same" {
		t.Fatalf("branch scopes = %q/%q", first, second)
	}
	if workflowScopedStepIdempotencyKey(first, "workflow", step, 0) == workflowScopedStepIdempotencyKey(second, "workflow", step, 0) {
		t.Fatal("same-named create in separate branches shares replay key")
	}
	if workflowBranchIdempotencyKey("", 0, "same") != "" {
		t.Fatal("branch without replay identity acquired dedupe key")
	}
	payload := workflowExecutionPayload{IdempotencyKey: "parent", WorkflowName: "workflow"}
	_, err := (&DynamicService{}).workflowIf(context.Background(), models.DynamicWorkflowStep{Branches: []models.WorkflowBranch{{Name: "same", Steps: []models.DynamicWorkflowStep{{Name: "set", Type: models.WorkflowStepTypeSetVariable, IsActive: true, Config: map[string]interface{}{"values": map[string]interface{}{"ran": true}}}}}}}, &payload)
	if err != nil || payload.Variables["ran"] != true || payload.IdempotencyKey != "parent" {
		t.Fatalf("branch did not preserve shared state/parent key: %#v, %v", payload, err)
	}
}
