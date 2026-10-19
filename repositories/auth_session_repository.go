package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const refreshRotationGrace = 5 * time.Second

type AuthSessionRepository struct {
	collection *mongo.Collection
}

func NewAuthSessionRepository() *AuthSessionRepository {
	return &AuthSessionRepository{collection: configs.GetCollection("auth_sessions")}
}

func NewAuthSessionRepositoryWithCollection(collection *mongo.Collection) *AuthSessionRepository {
	return &AuthSessionRepository{collection: collection}
}

func (r *AuthSessionRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		// Legacy duplicate families remain for expiry/audit but cannot rotate.
		{Keys: bson.D{{Key: "sessionKey", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)},
		{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		{Keys: bson.D{{Key: "userId", Value: 1}, {Key: "scope", Value: 1}}},
	})
	return err
}

func (r *AuthSessionRepository) Create(ctx context.Context, session models.AuthSession) error {
	if session.FamilyID == "" {
		return errors.New("session family is required")
	}
	session.SessionKey = session.FamilyID
	_, err := r.collection.InsertOne(ctx, session)
	return err
}

func (r *AuthSessionRepository) Rotate(ctx context.Context, familyID, presentedHash, newHash string, now, expiresAt time.Time) (models.RefreshRotationState, error) {
	result, err := r.collection.UpdateOne(ctx, bson.M{
		"familyId":       familyID,
		"sessionKey":     familyID,
		"currentJtiHash": presentedHash,
		"revokedAt":      bson.M{"$exists": false},
		"expiresAt":      bson.M{"$gt": now},
	}, bson.M{
		"$set": bson.M{
			"previousJtiHash":    presentedHash,
			"previousValidUntil": now.Add(refreshRotationGrace),
			"currentJtiHash":     newHash,
			"expiresAt":          expiresAt,
			"updatedAt":          now,
		},
	})
	if err != nil {
		return "", err
	}
	if result.ModifiedCount == 1 {
		return models.RefreshRotationSucceeded, nil
	}

	var session models.AuthSession
	err = r.collection.FindOne(ctx, bson.M{"familyId": familyID, "sessionKey": familyID}).Decode(&session)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return models.RefreshRotationReuse, nil
	}
	if err != nil {
		return "", err
	}
	return classifyRefreshRotation(session, presentedHash, now), nil
}

func (r *AuthSessionRepository) RevokeFamily(ctx context.Context, familyID string, revokedAt time.Time) error {
	_, err := r.collection.UpdateMany(ctx, bson.M{"familyId": familyID}, bson.M{
		"$set": bson.M{"revokedAt": revokedAt, "updatedAt": revokedAt},
	})
	return err
}

func (r *AuthSessionRepository) RevokeScopeForUser(ctx context.Context, userID, scope string, revokedAt time.Time) error {
	_, err := r.collection.UpdateMany(ctx, bson.M{
		"userId":    userID,
		"scope":     scope,
		"revokedAt": bson.M{"$exists": false},
	}, bson.M{"$set": bson.M{"revokedAt": revokedAt, "updatedAt": revokedAt}})
	return err
}

// userProjectSessionFilter selects active sessions for exactly one user within exactly one tenant/project.
func userProjectSessionFilter(userID, tenantID, projectID string) bson.M {
	return bson.M{"userId": userID, "tenantId": tenantID, "projectId": projectID, "revokedAt": bson.M{"$exists": false}}
}

// projectSessionFilter selects every active session bound to one tenant/project.
func projectSessionFilter(tenantID, projectID string) bson.M {
	return bson.M{"tenantId": tenantID, "projectId": projectID, "revokedAt": bson.M{"$exists": false}}
}

func (r *AuthSessionRepository) RevokeUserProjectSessions(ctx context.Context, userID, tenantID, projectID string, revokedAt time.Time) error {
	if userID == "" || tenantID == "" || projectID == "" {
		return errors.New("user, tenant, and project are required to revoke project sessions")
	}
	_, err := r.collection.UpdateMany(ctx, userProjectSessionFilter(userID, tenantID, projectID),
		bson.M{"$set": bson.M{"revokedAt": revokedAt, "updatedAt": revokedAt}})
	return err
}

func (r *AuthSessionRepository) RevokeProjectSessions(ctx context.Context, tenantID, projectID string, revokedAt time.Time) error {
	if tenantID == "" || projectID == "" {
		return errors.New("tenant and project are required to revoke project sessions")
	}
	_, err := r.collection.UpdateMany(ctx, projectSessionFilter(tenantID, projectID),
		bson.M{"$set": bson.M{"revokedAt": revokedAt, "updatedAt": revokedAt}})
	return err
}

func classifyRefreshRotation(session models.AuthSession, presentedHash string, now time.Time) models.RefreshRotationState {
	if session.RevokedAt != nil || !session.ExpiresAt.After(now) {
		return models.RefreshRotationReuse
	}
	if session.PreviousJTIHash == presentedHash && session.PreviousValidUntil != nil && !now.After(*session.PreviousValidUntil) {
		return models.RefreshRotationPreviousConflict
	}
	return models.RefreshRotationReuse
}
