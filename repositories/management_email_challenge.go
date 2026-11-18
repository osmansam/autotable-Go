package repositories

import (
	"context"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type ManagementEmailChallengeRepository struct{ db *mongo.Database }

func NewManagementEmailChallengeRepository(db *mongo.Database) *ManagementEmailChallengeRepository {
	return &ManagementEmailChallengeRepository{db: db}
}
func (r *ManagementEmailChallengeRepository) GetByID(ctx context.Context, id primitive.ObjectID) (*models.ManagementEmailChallenge, error) {
	var challenge models.ManagementEmailChallenge
	err := r.db.Collection("management_email_challenges").FindOne(ctx, bson.M{"_id": id}).Decode(&challenge)
	return &challenge, err
}
