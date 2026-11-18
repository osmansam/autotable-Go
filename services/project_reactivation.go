package services

import (
	"context"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// ReactivateProject serializes activation with membership and lifecycle writes.
// The inactive project is made active only inside the transaction, so failure
// to find and fence a usable administrator rolls back the whole update.
func (s *ProjectCollaborationService) ReactivateProject(ctx context.Context, actor models.ManagementAuthorizationSubject, target models.ManagementAuthorizationTarget, updates bson.M) (*mongo.UpdateResult, error) {
	tenant, project, _, err := collaborationIDs(target, actor.UserID)
	if err != nil {
		return nil, err
	}
	set := bson.M{}
	for key, value := range updates {
		switch key {
		case "name", "slug", "updatedAt":
			set[key] = value
		case "isActive":
			if active, ok := value.(bool); !ok || !active {
				return nil, ErrCollaborationInvalid
			}
		default:
			return nil, ErrCollaborationInvalid
		}
	}
	set["isActive"] = true
	var result *mongo.UpdateResult
	err = s.repository.WithAuthorizationTransaction(ctx, actor, models.ManagementAuthorizationTarget{TenantID: target.TenantID}, func(tx context.Context, snapshot models.ManagementMembershipSnapshot) error {
		if err := AuthorizeManagementSnapshot(actor, models.ManagementAuthorizationTarget{TenantID: target.TenantID}, PermissionTenantProjectsUpdate, snapshot); err != nil {
			return err
		}
		var err error
		result, err = s.repository.Database().Collection("projects").UpdateOne(tx, bson.M{"_id": project, "tenantId": tenant, "initializing": bson.M{"$ne": true}}, bson.M{"$set": set, "$inc": bson.M{"collaborationVersion": int64(1)}})
		if err != nil {
			return err
		}
		if result.MatchedCount != 1 {
			return mongo.ErrNoDocuments
		}
		pipeline, err := usableProjectAdminPipeline(target)
		if err != nil {
			return err
		}
		pipeline = append(pipeline, bson.D{{Key: "$limit", Value: 1}}, bson.D{{Key: "$project", Value: bson.M{"userId": 1}}})
		cursor, err := s.repository.Database().Collection("project_memberships").Aggregate(tx, pipeline)
		if err != nil {
			return err
		}
		defer cursor.Close(tx)
		var administrators []struct {
			UserID primitive.ObjectID `bson:"userId"`
		}
		if err := cursor.All(tx, &administrators); err != nil {
			return err
		}
		if len(administrators) == 0 {
			return ErrCollaborationRecoveryRequired
		}
		return s.repository.GuardAuthorizationEntities(tx, []primitive.ObjectID{administrators[0].UserID}, tenant, project)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
