package services

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

var processLocalizationJob = ProcessProjectTranslationJob

func StartLocalizationWorker(ctx context.Context, owner string, pollInterval time.Duration) {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	repository := localizationJobRepositoryFactory()
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		job, err := repository.LeaseNextJob(ctx, owner, time.Now().UTC(), localizationJobLeaseDuration)
		switch {
		case err == nil:
			heartbeat := time.NewTicker(localizationJobLeaseDuration / 3)
			processErr := runLocalizationJobWithLease(ctx, repository, job, heartbeat.C, processLocalizationJob)
			heartbeat.Stop()
			if processErr != nil && ctx.Err() == nil {
				log.Printf("localization_worker owner=%s job_id=%s error=%q", owner, job.ID.Hex(), processErr.Error())
			}
			timer.Reset(0)
		case mongo.IsTimeout(err):
			log.Printf("localization_worker owner=%s lease_timeout=%q", owner, err.Error())
			timer.Reset(pollInterval)
		case err == mongo.ErrNoDocuments:
			timer.Reset(pollInterval)
		case ctx.Err() != nil:
			return
		default:
			log.Printf("localization_worker owner=%s lease_error=%q", owner, err.Error())
			timer.Reset(pollInterval)
		}
	}
}
