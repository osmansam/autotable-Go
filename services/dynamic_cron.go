package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"github.com/robfig/cron/v3"
)

const (
	dynamicCronReloadEvery = 1 * time.Minute
	dynamicCronLockTTL     = 10 * time.Minute
	dynamicCronRunTimeout  = 5 * time.Minute
)

type dynamicCronScheduler struct {
	repository *repositories.DynamicRepository
	service    *DynamicService

	mu          sync.Mutex
	cron        *cron.Cron
	fingerprint string
}

type scheduledWorkflow struct {
	TenantID  string
	ProjectID string
	Schema    string
	Workflow  models.DynamicWorkflow
	Container models.ContainerModel
}

// StartDynamicCronScheduler loads active cron workflows and executes them on schedule.
func StartDynamicCronScheduler(ctx context.Context) {
	scheduler := &dynamicCronScheduler{
		repository: repositories.NewDynamicRepository(),
		service:    NewDynamicService(),
	}
	defer scheduler.stop()

	if err := scheduler.reload(ctx); err != nil {
		log.Printf("dynamic cron: initial load failed: %v", err)
	}

	ticker := time.NewTicker(dynamicCronReloadEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := scheduler.reload(ctx); err != nil {
				log.Printf("dynamic cron: reload failed: %v", err)
			}
		}
	}
}

func (s *dynamicCronScheduler) reload(ctx context.Context) error {
	workflows, err := s.loadWorkflows(ctx)
	if err != nil {
		return err
	}

	fingerprint := scheduledWorkflowFingerprint(workflows)
	s.mu.Lock()
	defer s.mu.Unlock()
	if fingerprint == s.fingerprint {
		return nil
	}

	nextCron := cron.New(cron.WithChain(cron.Recover(cron.DefaultLogger)))
	for _, scheduled := range workflows {
		scheduled := scheduled
		spec := scheduled.Workflow.Schedule
		if tz := strings.TrimSpace(scheduled.Workflow.Timezone); tz != "" {
			spec = "CRON_TZ=" + tz + " " + spec
		}
		if _, err := nextCron.AddFunc(spec, func() {
			runCtx, cancel := context.WithTimeout(ctx, dynamicCronRunTimeout)
			defer cancel()
			s.runWorkflow(runCtx, scheduled)
		}); err != nil {
			return fmt.Errorf("register workflow %s/%s/%s: %w", scheduled.TenantID, scheduled.ProjectID, scheduled.Workflow.Name, err)
		}
	}
	nextCron.Start()

	if s.cron != nil {
		stopCtx := s.cron.Stop()
		<-stopCtx.Done()
	}
	s.cron = nextCron
	s.fingerprint = fingerprint
	log.Printf("dynamic cron: registered %d workflow(s)", len(workflows))
	return nil
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

func (s *dynamicCronScheduler) runWorkflow(ctx context.Context, scheduled scheduledWorkflow) {
	lockKey := "cron:" + scheduledWorkflowKey(scheduled)
	lockID, ok := utils.AcquireLock(lockKey, dynamicCronLockTTL)
	if !ok {
		return
	}
	defer utils.ReleaseLock(lockKey, lockID)

	if err := s.service.RunCronWorkflow(ctx, scheduled.TenantID, scheduled.ProjectID, scheduled.Schema, &scheduled.Container, scheduled.Workflow, time.Now()); err != nil {
		log.Printf("dynamic cron: workflow %s failed: %v", scheduledWorkflowKey(scheduled), err)
	}
}

func (s *dynamicCronScheduler) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron == nil {
		return
	}
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()
	s.cron = nil
}

func scheduledWorkflowFingerprint(workflows []scheduledWorkflow) string {
	parts := make([]string, 0, len(workflows))
	for _, workflow := range workflows {
		encoded, err := json.Marshal(workflow.Workflow)
		if err != nil {
			encoded = []byte(fmt.Sprintf("%#v", workflow.Workflow))
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
	payload := workflowExecutionPayload{
		TenantID:        tenantID,
		ProjectID:       projectID,
		SchemaName:      schemaName,
		WorkflowTrigger: models.WorkflowTriggerCron,
		WorkflowVersion: workflowVersion(workflow),
		Container:       container,
		Record: map[string]interface{}{
			"_id":         triggeredAt.UTC().Format(time.RFC3339),
			"triggeredAt": triggeredAt.UTC().Format(time.RFC3339),
		},
	}
	return s.runWorkflowDefinition(ctx, &payload, workflow)
}
