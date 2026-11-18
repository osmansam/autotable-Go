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
	"sort"
)

var (
	ErrCollaborationInactive = errors.New("collaboration entity is missing or inactive")
	ErrCollaborationConflict = errors.New("collaboration state changed")
)

type ManagementCollaborationRepository struct{ db *mongo.Database }

type collaborationReadinessKey struct{}

func WithCollaborationReadiness(ctx context.Context) context.Context {
	return context.WithValue(ctx, collaborationReadinessKey{}, true)
}

func NewManagementCollaborationRepository(db *mongo.Database) *ManagementCollaborationRepository {
	return &ManagementCollaborationRepository{db: db}
}
func (r *ManagementCollaborationRepository) Database() *mongo.Database { return r.db }
func (r *ManagementCollaborationRepository) WithAuthorizationTransaction(ctx context.Context, actor models.ManagementAuthorizationSubject, target models.ManagementAuthorizationTarget, fn func(context.Context, models.ManagementMembershipSnapshot) error) error {
	ids, err := parseManagementMembershipIDs(actor, target)
	if err != nil {
		return err
	}
	session, err := r.db.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (interface{}, error) {
		if required, _ := ctx.Value(collaborationReadinessKey{}).(bool); required {
			result, err := r.db.Collection("management_collaboration_migrations").UpdateOne(tx, bson.M{"_id": "schema", "ready": true, "version": bson.M{"$gte": 1}}, bson.M{"$inc": bson.M{"grantRevision": int64(1)}})
			if err != nil {
				return nil, err
			}
			if result.MatchedCount != 1 {
				return nil, ErrCollaborationInactive
			}
		}
		project := primitive.NilObjectID
		if ids.projectID != nil {
			project = *ids.projectID
		}
		if err := r.GuardAuthorizationEntities(tx, []primitive.ObjectID{ids.userID}, ids.tenantID, project); err != nil {
			return nil, err
		}
		snapshot, err := NewManagementMembershipRepositoryWithDatabase(r.db).LoadManagementMembership(tx, actor, target)
		if err != nil {
			return nil, err
		}
		return nil, fn(tx, snapshot)
	}, options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()))
	return err
}

// Every grant writes the same parent documents as legacy lifecycle mutations.
// MongoDB write conflicts force a retry with current eligibility and authority.
func (r *ManagementCollaborationRepository) GuardAuthorizationEntities(ctx context.Context, userIDs []primitive.ObjectID, tenantID, projectID primitive.ObjectID) error {
	if mongo.SessionFromContext(ctx) == nil {
		return errors.New("authorization guards require a transaction")
	}
	users := append([]primitive.ObjectID{}, userIDs...)
	sort.Slice(users, func(i, j int) bool { return users[i].Hex() < users[j].Hex() })
	guard := func(collection string, filter bson.M, field string) error {
		result, err := r.db.Collection(collection).UpdateOne(ctx, filter, bson.M{"$inc": bson.M{field: int64(1)}})
		if err != nil {
			return err
		}
		if result.MatchedCount != 1 {
			return ErrCollaborationInactive
		}
		return nil
	}
	for i, user := range users {
		if user.IsZero() {
			return ErrCollaborationInactive
		}
		if i > 0 && users[i-1] == user {
			continue
		}
		if err := guard("users", bson.M{"_id": user, "isActive": true}, "authorizationVersion"); err != nil {
			return err
		}
	}
	if err := guard("tenants", bson.M{"_id": tenantID, "isActive": true}, "authorizationVersion"); err != nil {
		return err
	}
	if !projectID.IsZero() {
		return guard("projects", bson.M{"_id": projectID, "tenantId": tenantID, "isActive": true, "initializing": bson.M{"$ne": true}}, "collaborationVersion")
	}
	return nil
}
