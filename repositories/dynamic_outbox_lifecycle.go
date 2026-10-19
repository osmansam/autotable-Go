package repositories

import (
	"context"
	"time"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const DynamicOutboxLeaseDuration = 30 * time.Second

// RenewOutboxEventLease extends only the claim owned by this processing attempt.
func (r *DynamicRepository) RenewOutboxEventLease(ctx context.Context, event models.DynamicOutboxEvent, until time.Time) (bool, error) {
	result, err := r.globalCollection("dynamic_outbox").UpdateOne(ctx, outboxClaimFilter(event), bson.M{"$set": bson.M{
		"nextAttemptAt": primitive.NewDateTimeFromTime(until),
		"updatedAt":     primitive.NewDateTimeFromTime(time.Now()),
	}})
	if err != nil {
		return false, err
	}
	return result.MatchedCount > 0, nil
}

// MarkClaimedOutboxEventDone cannot acknowledge a newer worker's claim.
func (r *DynamicRepository) MarkClaimedOutboxEventDone(ctx context.Context, event models.DynamicOutboxEvent) (bool, error) {
	result, err := r.globalCollection("dynamic_outbox").UpdateOne(ctx, outboxClaimFilter(event), outboxEventDoneUpdate(time.Now()))
	if err != nil {
		return false, err
	}
	return result.MatchedCount > 0, nil
}

// MarkClaimedOutboxEventFailed schedules a retry only while this claim is owned.
func (r *DynamicRepository) MarkClaimedOutboxEventFailed(ctx context.Context, event models.DynamicOutboxEvent, message string, retryAfter time.Duration) (bool, error) {
	result, err := r.globalCollection("dynamic_outbox").UpdateOne(ctx, outboxClaimFilter(event), outboxEventFailedUpdate(event, message, retryAfter, time.Now()))
	if err != nil {
		return false, err
	}
	return result.MatchedCount > 0, nil
}

// RecoverExhaustedOutboxEvents handles a worker exiting during its final attempt.
// Such claims cannot be retried, but must still reach terminal state and retention.
func (r *DynamicRepository) RecoverExhaustedOutboxEvents(ctx context.Context, now time.Time) error {
	_, err := r.globalCollection("dynamic_outbox").UpdateMany(ctx, bson.M{
		"status":        bson.M{"$in": []string{models.DynamicOutboxStatusPending, models.DynamicOutboxStatusProcessing}},
		"nextAttemptAt": bson.M{"$lte": primitive.NewDateTimeFromTime(now)},
		"$expr":         bson.M{"$gte": []interface{}{"$attempts", "$maxAttempts"}},
	}, bson.M{"$set": bson.M{
		"status":    models.DynamicOutboxStatusFailed,
		"lastError": "outbox claim expired after final attempt",
		"updatedAt": primitive.NewDateTimeFromTime(now),
		"expireAt":  primitive.NewDateTimeFromTime(now.Add(configs.GetOutboxFailedRetention())),
	}})
	return err
}

func outboxClaimFilter(event models.DynamicOutboxEvent) bson.M {
	return bson.M{"_id": event.ID, "status": models.DynamicOutboxStatusProcessing, "attempts": event.Attempts}
}

func outboxEventDoneUpdate(now time.Time) bson.M {
	return bson.M{"$set": bson.M{
		"status":      models.DynamicOutboxStatusDone,
		"updatedAt":   primitive.NewDateTimeFromTime(now),
		"processedAt": primitive.NewDateTimeFromTime(now),
		"expireAt":    primitive.NewDateTimeFromTime(now.Add(configs.GetOutboxDoneRetention())),
	}, "$unset": bson.M{"lastError": ""}}
}

func outboxEventFailedUpdate(event models.DynamicOutboxEvent, message string, retryAfter time.Duration, now time.Time) bson.M {
	set := bson.M{
		"status":        models.DynamicOutboxStatusPending,
		"lastError":     message,
		"nextAttemptAt": primitive.NewDateTimeFromTime(now.Add(retryAfter)),
		"updatedAt":     primitive.NewDateTimeFromTime(now),
	}
	update := bson.M{"$set": set, "$unset": bson.M{"expireAt": ""}}
	if event.Attempts >= event.MaxAttempts {
		set["status"] = models.DynamicOutboxStatusFailed
		set["expireAt"] = primitive.NewDateTimeFromTime(now.Add(configs.GetOutboxFailedRetention()))
		delete(update, "$unset")
	}
	return update
}
