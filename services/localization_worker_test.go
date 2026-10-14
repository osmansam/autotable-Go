package services

import (
	"context"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestLocalizationWorkerProcessesLeasedJobAndStopsOnCancellation(t *testing.T) {
	job := &models.TranslationJob{ID: primitive.NewObjectID()}
	jobs := &fakeLocalizationJobRepository{leasedJobs: []*models.TranslationJob{job}}
	projects := &fakeProjectLocalizationRepository{}
	withLocalizationJobDependencies(t, jobs, projects)
	oldProcess := processLocalizationJob
	ctx, cancel := context.WithCancel(context.Background())
	processed := make(chan primitive.ObjectID, 1)
	processLocalizationJob = func(_ context.Context, got *models.TranslationJob) error {
		processed <- got.ID
		cancel()
		return nil
	}
	t.Cleanup(func() { processLocalizationJob = oldProcess })
	done := make(chan struct{})
	go func() {
		StartLocalizationWorker(ctx, "worker-test", time.Millisecond)
		close(done)
	}()

	select {
	case got := <-processed:
		if got != job.ID {
			t.Fatalf("processed job = %s", got)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not process leased job")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}
}
