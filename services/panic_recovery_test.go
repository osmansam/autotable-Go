package services

import (
	"context"
	"github.com/osmansam/autotableGo/models"
	"strings"
	"testing"
	"time"
)

func TestLocalizationChildPanicReturnsError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	job := &models.TranslationJob{LeaseOwner: "worker", LeaseToken: "claim"}
	err := runLocalizationJobWithLease(ctx, leaseTestRenewer{}, job, nil, func(context.Context, *models.TranslationJob) error { panic("private-provider-credential") })
	if err == nil || strings.Contains(err.Error(), "private-provider-credential") {
		t.Fatalf("panic must return sanitized error, got %v", err)
	}
}

func TestOutboxPanicFailsOwnedAttempt(t *testing.T) {
	for _, operation := range []string{models.DynamicOutboxOperationWorkflowStep, models.DynamicOutboxOperationCronWorkflow} {
		t.Run(operation, func(t *testing.T) {
			repo := &asyncAuditOutboxRepository{lease: func(context.Context, models.DynamicOutboxEvent, time.Time) (bool, error) { return true, nil }}
			event := &models.DynamicOutboxEvent{Operation: operation}
			err, owned := dispatchDynamicOutboxEventWithLease(context.Background(), repo, event, func(context.Context, *models.DynamicOutboxEvent) error { panic("private-payload") }, nil)
			if err == nil || !owned || strings.Contains(err.Error(), "private-payload") {
				t.Fatalf("panic result err=%v owned=%v", err, owned)
			}
		})
	}
}

func TestOutboxBatchPanicIsRetriedNeverAcknowledged(t *testing.T) {
	repo := &asyncAuditOutboxRepository{claims: []*models.DynamicOutboxEvent{{Operation: models.DynamicOutboxOperationWorkflowStep, Attempts: 2}}}
	processDynamicOutboxBatchWithDispatch(context.Background(), repo, func(context.Context, *models.DynamicOutboxEvent) error { panic("private-payload") }, 1)
	if len(repo.doneIDs) != 0 || len(repo.failedAttempts) != 1 || repo.failedAttempts[0] != 2 || len(repo.retryDelays) != 1 || repo.retryDelays[0] <= 0 {
		t.Fatalf("panic was not retried safely: %#v", repo)
	}
}
