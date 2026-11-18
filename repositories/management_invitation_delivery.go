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

func (r *ManagementInvitationRepository) LeaseDelivery(ctx context.Context, worker string, now time.Time) (*models.InvitationDeliveryJob, error) {
	filter := bson.M{"$or": bson.A{
		bson.M{"status": bson.M{"$in": bson.A{"queued", "retryable_failure"}}, "$or": bson.A{bson.M{"nextAttemptAt": bson.M{"$lte": now}}, bson.M{"expiresAt": bson.M{"$lte": now}}}},
		bson.M{"status": "leased", "leaseUntil": bson.M{"$lte": now}},
	}}
	var job models.InvitationDeliveryJob
	err := r.db.Collection("management_invitation_deliveries").FindOneAndUpdate(ctx, filter, bson.M{"$set": bson.M{"status": "leased", "leaseKey": worker + ":" + primitive.NewObjectID().Hex(), "leaseUntil": now.Add(time.Minute), "updatedAt": now}, "$inc": bson.M{"attempts": 1}}, options.FindOneAndUpdate().SetSort(bson.D{{Key: "nextAttemptAt", Value: 1}}).SetReturnDocument(options.After)).Decode(&job)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &job, err
}
func (r *ManagementInvitationRepository) FinishDelivery(ctx context.Context, job models.InvitationDeliveryJob, state, code string, now, next time.Time) error {
	session, err := r.db.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	eventID := primitive.NewObjectID()
	_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (interface{}, error) {
		set := bson.M{"status": state, "failureCode": code, "updatedAt": now}
		if state == "retryable_failure" {
			set["nextAttemptAt"] = next
		}
		unset := bson.M{"leaseKey": "", "leaseUntil": ""}
		if state != "retryable_failure" {
			unset["encryptedPayload"] = ""
		}
		result, err := r.db.Collection("management_invitation_deliveries").UpdateOne(tx, bson.M{"_id": job.ID, "status": "leased", "leaseKey": job.LeaseKey}, bson.M{"$set": set, "$unset": unset})
		if err != nil {
			return nil, err
		}
		if result.MatchedCount == 0 {
			return nil, nil
		}
		deliveryState := "failed"
		switch state {
		case "smtp_accepted":
			deliveryState = "sent"
		case "retryable_failure":
			deliveryState = "retrying"
		case "obsolete":
			deliveryState = "canceled"
		}
		if job.Purpose == "invitation" {
			if _, err := r.db.Collection("management_invitations").UpdateOne(tx, bson.M{"_id": job.InvitationID, "tokenVersion": job.TokenVersion, "status": models.InvitationPending}, bson.M{"$set": bson.M{"deliveryState": deliveryState, "updatedAt": now}}); err != nil {
				return nil, err
			}
		}
		event := invitationDeliveryEvent(eventID, job, state, code, now)
		return nil, NewManagementCollaborationRepository(r.db).InsertCollaborationEvent(tx, event)
	})
	return err
}

func invitationDeliveryEvent(eventID primitive.ObjectID, job models.InvitationDeliveryJob, state, code string, now time.Time) models.CollaborationEvent {
	return models.CollaborationEvent{ID: eventID, TenantID: job.TenantID.Hex(), ProjectID: job.ProjectID.Hex(), Status: "pending", NextAttemptAt: now, CreatedAt: now, Audit: models.AuditLog{EventID: eventID, TenantID: job.TenantID.Hex(), ProjectID: job.ProjectID.Hex(), Timestamp: primitive.NewDateTimeFromTime(now), SchemaName: "management_invitations", DocumentIDs: []primitive.ObjectID{job.InvitationID}, Action: job.Purpose + ".delivery." + state, After: bson.M{"jobId": job.ID, "version": job.TokenVersion, "state": state, "failureCode": code}}}
}

func expiredDeliveryFilter(now time.Time) bson.M {
	return bson.M{"expiresAt": bson.M{"$lte": now}, "expiryFinalizedAt": bson.M{"$exists": false}, "$or": bson.A{
		bson.M{"status": bson.M{"$in": bson.A{"queued", "retryable_failure", "leased"}}},
		bson.M{"status": "terminal_failure", "failureCode": "expired"},
	}}
}

// Expiry cleanup needs neither a decryption key nor an enabled mail sender.
// The bounded batch also repairs terminal expiry records from older cleanup.
func (r *ManagementInvitationRepository) PurgeExpiredDeliveries(ctx context.Context, now time.Time) error {
	cursor, err := r.db.Collection("management_invitation_deliveries").Find(ctx, expiredDeliveryFilter(now), options.Find().SetLimit(100).SetProjection(bson.M{"encryptedPayload": 0}).SetSort(bson.D{{Key: "expiresAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return err
	}
	var jobs []models.InvitationDeliveryJob
	err = cursor.All(ctx, &jobs)
	_ = cursor.Close(ctx)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err := r.expireDelivery(ctx, job, now); err != nil {
			return err
		}
	}
	_, err = r.db.Collection("management_invitations").UpdateMany(ctx, bson.M{"status": models.InvitationPending, "expiresAt": bson.M{"$lte": now}}, bson.M{"$set": bson.M{"status": models.InvitationExpired, "updatedAt": now}})
	return err
}
func (r *ManagementInvitationRepository) expireDelivery(ctx context.Context, job models.InvitationDeliveryJob, now time.Time) error {
	session, err := r.db.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	eventID := primitive.NewObjectID()
	_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (interface{}, error) {
		filter := expiredDeliveryFilter(now)
		filter["_id"] = job.ID
		result, err := r.db.Collection("management_invitation_deliveries").UpdateOne(tx, filter, bson.M{"$set": bson.M{"status": "terminal_failure", "failureCode": "expired", "updatedAt": now, "expiryFinalizedAt": now}, "$unset": bson.M{"encryptedPayload": "", "leaseKey": "", "leaseUntil": ""}})
		if err != nil {
			return nil, err
		}
		if result.MatchedCount == 0 {
			return nil, nil
		}
		if job.Purpose == "invitation" {
			_, err = r.db.Collection("management_invitations").UpdateOne(tx, bson.M{"_id": job.InvitationID, "tokenVersion": job.TokenVersion, "expiresAt": bson.M{"$lte": now}, "status": bson.M{"$in": bson.A{models.InvitationPending, models.InvitationExpired}}}, bson.M{"$set": bson.M{"status": models.InvitationExpired, "deliveryState": "failed", "updatedAt": now}})
			if err != nil {
				return nil, err
			}
		}
		return nil, NewManagementCollaborationRepository(r.db).InsertCollaborationEvent(tx, invitationDeliveryEvent(eventID, job, "terminal_failure", "expired", now))
	}, options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()))
	return err
}
