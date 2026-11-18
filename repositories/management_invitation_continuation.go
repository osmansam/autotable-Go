package repositories

import (
	"context"
	"encoding/base64"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"time"
)

type ManagementInvitationContinuationRepository struct{ db *mongo.Database }

func NewManagementInvitationContinuationRepository(db *mongo.Database) *ManagementInvitationContinuationRepository {
	return &ManagementInvitationContinuationRepository{db: db}
}
func validContinuationHandle(raw string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	return err == nil && len(raw) == 43 && len(decoded) == 32
}
func (r *ManagementInvitationContinuationRepository) GetByHandle(ctx context.Context, handle string, now time.Time) (*models.InvitationContinuation, error) {
	if !validContinuationHandle(handle) {
		return nil, mongo.ErrNoDocuments
	}
	var continuation models.InvitationContinuation
	err := r.db.Collection("management_invitation_continuations").FindOne(ctx, bson.M{"handleHash": utils.HashInvitationToken(handle), "purpose": "project_invitation", "status": "pending", "expiresAt": bson.M{"$gt": now}}).Decode(&continuation)
	return &continuation, err
}
func (r *ManagementInvitationContinuationRepository) GetByID(ctx context.Context, id primitive.ObjectID, now time.Time) (*models.InvitationContinuation, error) {
	var continuation models.InvitationContinuation
	err := r.db.Collection("management_invitation_continuations").FindOne(ctx, bson.M{"_id": id, "purpose": "project_invitation", "status": "pending", "expiresAt": bson.M{"$gt": now}}).Decode(&continuation)
	return &continuation, err
}
