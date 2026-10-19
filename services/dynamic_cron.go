package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

const dynamicCronReloadEvery = time.Minute
const dynamicCronRunTimeout = 5 * time.Minute

type dynamicCronScheduler struct {
	repository  dynamicCronRepository
	queue       cronScheduleQueue
	mu          sync.Mutex
	workflows   []scheduledWorkflow
	fingerprint string
	rotation    int
	stopped     bool
}
type dynamicCronRepository interface {
	GetScheduledWorkflowContainers(context.Context) ([]repositories.ScheduledWorkflowContainer, error)
}
type scheduledWorkflow struct {
	TenantID, ProjectID, Schema string
	Workflow                    models.DynamicWorkflow
	Container                   models.ContainerModel
}

// The scheduler only materializes durable jobs. Execution, retry, and recovery
// belong to the existing outbox processor, so timer/process death loses no claim.
func StartDynamicCronScheduler(ctx context.Context) {
	repository := repositories.NewDynamicRepository()
	scheduler := &dynamicCronScheduler{repository: repository, queue: repository}
	defer scheduler.stop()
	reload := func() {
		loadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := scheduler.reload(loadCtx); err != nil {
			log.Printf("dynamic cron: reload failed: %v", err)
		}
	}
	poll := func() {
		pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := scheduler.poll(pollCtx, time.Now().UTC()); err != nil && ctx.Err() == nil {
			log.Printf("dynamic cron: materialization failed: %v", err)
		}
	}
	reload()
	poll()
	due := time.NewTicker(time.Second)
	defer due.Stop()
	registry := time.NewTicker(dynamicCronReloadEvery)
	defer registry.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-registry.C:
			reload()
		case <-due.C:
			poll()
		}
	}
}
func (s *dynamicCronScheduler) reload(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	workflows, err := s.loadWorkflows(ctx)
	if err != nil {
		return err
	}
	fingerprint := scheduledWorkflowFingerprint(workflows)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || fingerprint == s.fingerprint {
		return nil
	}
	s.workflows = workflows
	s.fingerprint = fingerprint
	log.Printf("dynamic cron: loaded %d durable schedule(s)", len(workflows))
	return nil
}
func (s *dynamicCronScheduler) poll(ctx context.Context, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.stopped || len(s.workflows) == 0 {
		s.mu.Unlock()
		return nil
	}
	workflows := append([]scheduledWorkflow(nil), s.workflows...)
	start := s.rotation % len(workflows)
	s.rotation = (s.rotation + 1) % len(workflows)
	s.mu.Unlock()
	if s.queue == nil {
		return errors.New("durable cron queue is unavailable")
	}
	budget := cronPollBatch
	var failures []error
	for offset := 0; offset < len(workflows) && budget > 0; offset++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		scheduled := workflows[(start+offset)%len(workflows)]
		queued, err := materializeCronSchedule(ctx, s.queue, scheduled, now, budget)
		budget -= queued
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", scheduledWorkflowKey(scheduled), err))
		}
	}
	return errors.Join(failures...)
}
func (s *dynamicCronScheduler) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	s.workflows = nil
}

func (s *dynamicCronScheduler) loadWorkflows(ctx context.Context) ([]scheduledWorkflow, error) {
	containers, err := s.repository.GetScheduledWorkflowContainers(ctx)
	if err != nil {
		return nil, err
	}

	var workflows []scheduledWorkflow
	for _, item := range containers {
		container := item.Container
		for _, workflow := range container.Workflows {
			if !workflow.IsActive || workflow.Trigger != models.WorkflowTriggerCron {
				continue
			}
			if err := ValidateWorkflow(workflow); err != nil {
				log.Printf("dynamic cron: skipping invalid workflow %s/%s/%s/%s: %v", item.TenantID, item.ProjectID, container.SchemaName, workflow.Name, err)
				continue
			}
			workflows = append(workflows, scheduledWorkflow{
				TenantID:  item.TenantID,
				ProjectID: item.ProjectID,
				Schema:    container.SchemaName,
				Workflow:  workflow,
				Container: container,
			})
		}
	}

	sort.SliceStable(workflows, func(i, j int) bool {
		return scheduledWorkflowKey(workflows[i]) < scheduledWorkflowKey(workflows[j])
	})
	return workflows, nil
}

func scheduledWorkflowFingerprint(workflows []scheduledWorkflow) string {
	parts := make([]string, 0, len(workflows))
	for _, workflow := range workflows {
		encoded, err := json.Marshal(workflow)
		if err != nil {
			encoded = []byte(fmt.Sprintf("%#v", workflow))
		}
		parts = append(parts, scheduledWorkflowKey(workflow)+":"+string(encoded))
	}
	return strings.Join(parts, "|")
}

func scheduledWorkflowKey(workflow scheduledWorkflow) string {
	return strings.Join([]string{
		workflow.TenantID,
		workflow.ProjectID,
		workflow.Schema,
		workflow.Workflow.Name,
	}, ":")
}

func (s *DynamicService) RunCronWorkflow(ctx context.Context, tenantID, projectID, schemaName string, container *models.ContainerModel, workflow models.DynamicWorkflow, triggeredAt time.Time) error {
	if container == nil {
		return fmt.Errorf("container is required")
	}
	payload := newCronWorkflowPayload(tenantID, projectID, schemaName, container, workflow, triggeredAt)
	return s.runWorkflowWithTransaction(ctx, &payload, workflow)
}

func newCronWorkflowPayload(tenantID, projectID, schemaName string, container *models.ContainerModel, workflow models.DynamicWorkflow, triggeredAt time.Time) workflowExecutionPayload {
	return workflowExecutionPayload{
		TenantID:        tenantID,
		ProjectID:       projectID,
		SchemaName:      schemaName,
		WorkflowTrigger: models.WorkflowTriggerCron,
		WorkflowVersion: workflowVersion(workflow),
		IdentityKind:    AccessIdentitySystem,
		Container:       container,
		Record: map[string]interface{}{
			"_id":         triggeredAt.UTC().Format(time.RFC3339Nano),
			"triggeredAt": triggeredAt.UTC().Format(time.RFC3339Nano),
		},
	}
}
