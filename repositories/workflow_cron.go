package repositories

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ErrCronScheduleAdvanced = errors.New("cron schedule changed or advanced by another replica")
var ErrCronOutboxClaimLost = errors.New("cron outbox claim is expired or no longer owned")

func (r *DynamicRepository) EnsureCronSchedule(ctx context.Context, id, revision string, first time.Time, source models.WorkflowCronSource) (models.WorkflowCronSchedule, error) {
	collection := r.globalCollection("workflow_cron_schedules")
	var cursor models.WorkflowCronSchedule
	err := collection.FindOne(ctx, bson.M{"_id": id}).Decode(&cursor)
	if err == nil && cursor.Revision == revision {
		return cursor, nil
	}
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return cursor, err
	}
	err = r.WithTransaction(ctx, func(tx mongo.SessionContext) error {
		if err := r.fenceCronSource(tx, source); err != nil {
			return err
		}
		// Recheck after taking the source write fence; a concurrent replica may have
		// initialized/advanced this cursor while the metadata lock was pending.
		err := collection.FindOne(tx, bson.M{"_id": id}).Decode(&cursor)
		if err == nil && cursor.Revision == revision {
			return nil
		}
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		desired := models.WorkflowCronSchedule{ID: id, Revision: revision, NextAt: first, Source: source, UpdatedAt: time.Now().UTC()}
		_, err = collection.UpdateOne(tx, bson.M{"_id": id}, bson.M{"$set": bson.M{"revision": revision, "nextAt": first, "source": source, "updatedAt": desired.UpdatedAt}, "$unset": bson.M{"skippedBefore": ""}}, options.Update().SetUpsert(true))
		cursor = desired
		return err
	})
	return cursor, err
}

// A stale scheduler cannot restore an old revision or enqueue outdated code.
// The metadata write also serializes against concurrent management edits.
func (r *DynamicRepository) fenceCronSource(ctx context.Context, source models.WorkflowCronSource) error {
	collection := r.collection(source.TenantID, source.ProjectID, "containers")
	var raw bson.Raw
	err := collection.FindOne(ctx, bson.M{"schemaName": source.Schema}).Decode(&raw)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrCronScheduleAdvanced
	}
	if err != nil {
		return err
	}
	var current models.ContainerModel
	if err := bson.Unmarshal(raw, &current); err != nil {
		return err
	}
	actual, err := json.Marshal(current.Workflows)
	if err != nil {
		return err
	}
	expected, err := json.Marshal(source.Workflows)
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, expected) {
		return ErrCronScheduleAdvanced
	}
	active := false
	for _, workflow := range current.Workflows {
		if workflow.Name == source.WorkflowName && workflow.IsActive && workflow.Trigger == models.WorkflowTriggerCron {
			active = true
		}
	}
	if !active {
		return ErrCronScheduleAdvanced
	}
	_, err = collection.UpdateOne(ctx, bson.M{"_id": raw.Lookup("_id")}, bson.M{"$inc": bson.M{"cronSchedulingFence": 1}})
	return err
}

// A crash cannot leave a consumed cursor without its runnable occurrence.
func (r *DynamicRepository) AdvanceCronSchedule(ctx context.Context, previous models.WorkflowCronSchedule, next, skippedBefore time.Time, event *models.DynamicOutboxEvent) error {
	if next.IsZero() || !next.After(previous.NextAt) {
		return errors.New("cron cursor must advance")
	}
	return r.WithTransaction(ctx, func(tx mongo.SessionContext) error {
		if err := r.fenceCronSource(tx, previous.Source); err != nil {
			return err
		}
		set := bson.M{"nextAt": next, "source": previous.Source, "updatedAt": time.Now().UTC()}
		if !skippedBefore.IsZero() {
			set["skippedBefore"] = skippedBefore
		}
		result, err := r.globalCollection("workflow_cron_schedules").UpdateOne(tx, bson.M{"_id": previous.ID, "revision": previous.Revision, "nextAt": previous.NextAt}, bson.M{"$set": set})
		if err != nil {
			return err
		}
		if result.MatchedCount != 1 {
			return ErrCronScheduleAdvanced
		}
		if event != nil {
			_, err = r.InsertOutboxEvent(tx, *event)
		}
		return err
	})
}

// Serialize same-workflow database execution without a Redis overlap lease.
func (r *DynamicRepository) FenceCronWorkflowScope(ctx context.Context, id string) error {
	if mongo.SessionFromContext(ctx) == nil {
		return errors.New("cron execution fence requires a transaction")
	}
	result, err := r.globalCollection("workflow_cron_schedules").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$inc": bson.M{"executionFence": 1}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return ErrCronScheduleAdvanced
	}
	return nil
}

// Call at the end of the root transaction. Holding this lock throughout a long
// execution would block the outbox heartbeat that keeps the claim alive.
func (r *DynamicRepository) FenceCronOutboxClaim(ctx context.Context, event models.DynamicOutboxEvent) error {
	if mongo.SessionFromContext(ctx) == nil {
		return errors.New("cron claim fence requires a transaction")
	}
	filter := outboxClaimFilter(event)
	filter["nextAttemptAt"] = bson.M{"$gt": primitive.NewDateTimeFromTime(time.Now())}
	result, err := r.globalCollection("dynamic_outbox").UpdateOne(ctx, filter, bson.M{"$inc": bson.M{"executionFence": 1}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return ErrCronOutboxClaimLost
	}
	return nil
}
