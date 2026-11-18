package repositories

import (
	"context"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type ManagementInvitationRepository struct{ db *mongo.Database }

func NewManagementInvitationRepository(db *mongo.Database) *ManagementInvitationRepository {
	return &ManagementInvitationRepository{db: db}
}
func (r *ManagementInvitationRepository) GetByID(ctx context.Context, id primitive.ObjectID) (*models.ManagementInvitation, error) {
	var invitation models.ManagementInvitation
	err := r.db.Collection("management_invitations").FindOne(ctx, bson.M{"_id": id}).Decode(&invitation)
	return &invitation, err
}
func (r *ManagementInvitationRepository) GetScoped(ctx context.Context, id, tenant, project primitive.ObjectID) (*models.ManagementInvitation, error) {
	var invitation models.ManagementInvitation
	err := r.db.Collection("management_invitations").FindOne(ctx, bson.M{"_id": id, "tenantId": tenant, "projectId": project}).Decode(&invitation)
	return &invitation, err
}
