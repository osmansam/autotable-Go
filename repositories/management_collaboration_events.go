package repositories

import (
	"context"
	"errors"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

func (r *ManagementCollaborationRepository) InsertCollaborationEvent(ctx context.Context, event models.CollaborationEvent) error {
	if mongo.SessionFromContext(ctx) == nil {
		return errors.New("collaboration event requires a transaction")
	}
	_, err := r.db.Collection("management_collaboration_events").InsertOne(ctx, event)
	return err
}
func (r *ManagementCollaborationRepository) LeaseCollaborationEvent(ctx context.Context, worker string, now time.Time) (*models.CollaborationEvent, error) {
	leaseKey := worker + ":" + primitive.NewObjectID().Hex()
	var event models.CollaborationEvent
	err := r.db.Collection("management_collaboration_events").FindOneAndUpdate(ctx, bson.M{"$or": bson.A{bson.M{"status": "pending", "nextAttemptAt": bson.M{"$lte": now}}, bson.M{"status": "processing", "leaseUntil": bson.M{"$lte": now}}}}, bson.M{"$set": bson.M{"status": "processing", "leaseKey": leaseKey, "leaseUntil": now.Add(time.Minute)}, "$inc": bson.M{"attempts": 1}}, options.FindOneAndUpdate().SetSort(bson.D{{Key: "nextAttemptAt", Value: 1}}).SetReturnDocument(options.After)).Decode(&event)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &event, err
}
func (r *ManagementCollaborationRepository) FinishCollaborationEvent(ctx context.Context, event models.CollaborationEvent, now time.Time, failed bool) error {
	set := bson.M{"status": "done", "finishedAt": now}
	if failed {
		delay := time.Second * time.Duration(1<<min(event.Attempts, 12))
		if delay > time.Hour {
			delay = time.Hour
		}
		set = bson.M{"status": "pending", "nextAttemptAt": now.Add(delay)}
	}
	_, err := r.db.Collection("management_collaboration_events").UpdateOne(ctx, bson.M{"_id": event.ID, "leaseKey": event.LeaseKey, "status": "processing"}, bson.M{"$set": set, "$unset": bson.M{"leaseKey": "", "leaseUntil": ""}})
	return err
}
