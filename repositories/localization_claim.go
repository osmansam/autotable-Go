package repositories

import (
	"context"
	"errors"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"time"
)

var ErrLocalizationLeaseLost = errors.New("localization job claim is missing, expired, or no longer owned")

type localizationClaimKey struct{}
type localizationClaim struct {
	ID           primitive.ObjectID
	Owner, Token string
}

func WithLocalizationJobClaim(ctx context.Context, job models.TranslationJob) context.Context {
	return context.WithValue(ctx, localizationClaimKey{}, localizationClaim{job.ID, job.LeaseOwner, job.LeaseToken})
}

func localizationJobClaimFilter(ctx context.Context, id primitive.ObjectID, now time.Time) (bson.M, error) {
	claim, ok := ctx.Value(localizationClaimKey{}).(localizationClaim)
	if !ok || claim.ID != id || claim.Owner == "" || claim.Token == "" {
		return nil, ErrLocalizationLeaseLost
	}
	return bson.M{"_id": id, "leaseOwner": claim.Owner, "leaseToken": claim.Token, "status": models.TranslationJobRunning, "leaseExpiresAt": bson.M{"$gt": now}}, nil
}

func (r *LocalizationRepository) RenewJobLease(ctx context.Context, job *models.TranslationJob, now time.Time, duration time.Duration) (bool, error) {
	if job == nil || duration <= 0 {
		return false, ErrLocalizationLeaseLost
	}
	filter, err := localizationJobClaimFilter(WithLocalizationJobClaim(ctx, *job), job.ID, now)
	if err != nil {
		return false, err
	}
	result, err := r.jobs.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"leaseExpiresAt": now.Add(duration), "updatedAt": now}})
	if err != nil {
		return false, err
	}
	return result.MatchedCount == 1, nil
}

// Lock the current claim and mutate generated content in one transaction.
// A concurrent reclaim either wins before this CAS (and rejects this write), or
// waits/conflicts until this content commit has finished. A read-only claim
// check would leave a gap where a replaced worker could still write content.
func (r *LocalizationRepository) writeContentWithClaim(ctx context.Context, write func(context.Context) error) error {
	claim, worker := ctx.Value(localizationClaimKey{}).(localizationClaim)
	if !worker {
		if err := ctx.Err(); err != nil {
			return err
		}
		return write(ctx)
	}
	fencedWrite := func(tx context.Context) error {
		now := time.Now().UTC()
		filter, err := localizationJobClaimFilter(tx, claim.ID, now)
		if err != nil {
			return err
		}
		result, err := r.jobs.UpdateOne(tx, filter, bson.M{"$set": bson.M{"updatedAt": now}, "$inc": bson.M{"contentWriteFence": 1}})
		if err != nil {
			return err
		}
		if result.MatchedCount != 1 {
			return ErrLocalizationLeaseLost
		}
		return write(tx)
	}
	if mongo.SessionFromContext(ctx) != nil {
		// A session alone does not establish a transaction. Worker writes own
		// their transaction so the claim/content atomicity cannot be bypassed.
		return errors.New("localization worker content requires a repository-owned transaction")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	session, err := r.jobs.Database().Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (interface{}, error) {
		return nil, fencedWrite(tx)
	}, options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()))
	return err
}
