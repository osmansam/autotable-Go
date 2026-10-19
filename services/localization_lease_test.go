package services

import (
	"context"
	"errors"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"testing"
	"time"
)

type leaseTestRenewer struct {
	renewed chan struct{}
	owned   bool
}

func (r leaseTestRenewer) RenewJobLease(context.Context, *models.TranslationJob, time.Time, time.Duration) (bool, error) {
	r.renewed <- struct{}{}
	return r.owned, nil
}

func TestLocalizationWorkerCancelsProcessingWhenLeaseIsLost(t *testing.T) {
	ticks := make(chan time.Time, 1)
	started := make(chan struct{})
	done := make(chan error, 1)
	renewer := leaseTestRenewer{renewed: make(chan struct{}, 1)}
	job := &models.TranslationJob{ID: primitive.NewObjectID(), LeaseOwner: "worker", LeaseToken: "claim"}
	go func() {
		done <- runLocalizationJobWithLease(context.Background(), renewer, job, ticks, func(ctx context.Context, _ *models.TranslationJob) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	<-started
	ticks <- time.Now()
	select {
	case err := <-done:
		if !errors.Is(err, repositories.ErrLocalizationLeaseLost) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stale worker was not cancelled")
	}
}
