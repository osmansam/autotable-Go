package repositories

import (
	"context"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func EnsureCollaborationIndexes(ctx context.Context, db *mongo.Database) error {
	definitions := map[string][]mongo.IndexModel{
		"users":               {{Keys: bson.D{{Key: "normalizedEmail", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"normalizedEmail": bson.M{"$type": "string"}})}},
		"tenant_memberships":  {{Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "userId", Value: 1}}, Options: options.Index().SetUnique(true)}},
		"project_memberships": {{Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}, {Key: "userId", Value: 1}}, Options: options.Index().SetUnique(true)}},
		"management_invitations": {
			{Keys: bson.D{{Key: "tokenHash", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}, {Key: "normalizedEmail", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"status": "pending"})},
		},
		"management_invitation_continuations": {
			{Keys: bson.D{{Key: "handleHash", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		},
		"management_email_challenges": {
			{Keys: bson.D{{Key: "tokenHash", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		},
		"management_invitation_deliveries":  {{Keys: bson.D{{Key: "jobKey", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "status", Value: 1}, {Key: "nextAttemptAt", Value: 1}}}, {Keys: bson.D{{Key: "expiryFinalizedAt", Value: 1}, {Key: "expiresAt", Value: 1}, {Key: "_id", Value: 1}}, Options: options.Index().SetName("invitation_expiry_cleanup")}},
		"management_collaboration_events":   {{Keys: bson.D{{Key: "status", Value: 1}, {Key: "nextAttemptAt", Value: 1}}}},
		"management_collaboration_requests": {{Keys: bson.D{{Key: "requestKey", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)}},
	}
	for collection, indexes := range definitions {
		if _, err := db.Collection(collection).Indexes().CreateMany(ctx, indexes); err != nil {
			return err
		}
	}
	return nil
}
