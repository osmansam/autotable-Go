package services

import (
	"context"
	"fmt"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"time"
)

const localizationJobLeaseDuration = 30 * time.Minute

type localizationClaimRenewer interface {
	RenewJobLease(context.Context, *models.TranslationJob, time.Time, time.Duration) (bool, error)
}

func runLocalizationJobWithLease(ctx context.Context, repository localizationClaimRenewer, job *models.TranslationJob, ticks <-chan time.Time, process func(context.Context, *models.TranslationJob) error) error {
	if job == nil || job.LeaseOwner == "" || job.LeaseToken == "" {
		return repositories.ErrLocalizationLeaseLost
	}
	jobCtx, cancel := context.WithCancel(repositories.WithLocalizationJobClaim(ctx, *job))
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- process(jobCtx, job) }()
	for {
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			cancel()
			<-done
			return ctx.Err()
		case <-ticks:
			renewCtx, stop := context.WithTimeout(jobCtx, 5*time.Second)
			owned, err := repository.RenewJobLease(renewCtx, job, time.Now().UTC(), localizationJobLeaseDuration)
			stop()
			if err != nil || !owned {
				cancel()
				<-done
				return fmt.Errorf("%w: renewal error=%v", repositories.ErrLocalizationLeaseLost, err)
			}
		}
	}
}
