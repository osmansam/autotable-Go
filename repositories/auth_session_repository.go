package repositories

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const refreshRotationGrace = 5 * time.Second

var ErrManagementSessionInvalid = errors.New("management session is invalid")

func (r *AuthSessionRepository) ValidateManagementFamily(ctx context.Context, p models.ManagementPrincipal, now time.Time) error {
	if p.FamilyID == "" || p.UserID == "" || p.TenantID == "" {
		return ErrManagementSessionInvalid
	}
	var session models.AuthSession
	filter := bson.M{"familyId": p.FamilyID, "sessionKey": p.FamilyID, "identity": "management", "userId": p.UserID, "tenantId": p.TenantID, "projectId": p.ProjectID, "scope": p.Scope, "revokedAt": bson.M{"$exists": false}, "expiresAt": bson.M{"$gt": now}}
	if p.ProjectID == "" {
		filter["projectId"] = bson.M{"$in": bson.A{"", nil}}
	}
	err := r.collection.FindOne(ctx, filter).Decode(&session)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrManagementSessionInvalid
	}
	return err
}

// RotateManagement may adopt a legacy family only after the caller has verified
// its management refresh JWT and current membership. Application families cannot
// be adopted, even when user/project identifiers happen to coincide.
func (r *AuthSessionRepository) RotateManagement(ctx context.Context, familyID, presentedHash, newHash string, now, expiresAt time.Time) (models.RefreshRotationState, error) {
	_, err := r.collection.UpdateOne(ctx, bson.M{"familyId": familyID, "sessionKey": familyID, "identity": bson.M{"$ne": "application"}, "currentJtiHash": presentedHash, "revokedAt": bson.M{"$exists": false}, "expiresAt": bson.M{"$gt": now}}, bson.M{"$set": bson.M{"identity": "management"}})
	if err != nil {
		return "", err
	}
	var session models.AuthSession
	if err := r.collection.FindOne(ctx, bson.M{"familyId": familyID, "sessionKey": familyID, "identity": "management"}).Decode(&session); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return models.RefreshRotationReuse, nil
		}
		return "", err
	}
	return r.Rotate(ctx, familyID, presentedHash, newHash, now, expiresAt)
}

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
	if session.Identity == "management" && session.Scope == "project" {
		user, e1 := primitive.ObjectIDFromHex(session.UserID)
		tenant, e2 := primitive.ObjectIDFromHex(session.TenantID)
		project, e3 := primitive.ObjectIDFromHex(session.ProjectID)
		if e1 != nil || e2 != nil || e3 != nil {
			return ErrManagementSessionInvalid
		}
		var membership struct {
			Version int64 `bson:"version"`
		}
		err := r.collection.Database().Collection("project_memberships").FindOne(ctx, bson.M{"userId": user, "tenantId": tenant, "projectId": project, "status": models.MembershipStatusActive}, options.FindOne().SetProjection(bson.M{"version": 1})).Decode(&membership)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrManagementSessionInvalid
		}
		if err != nil {
			return err
		}
		session.ProjectMembershipVersion = membership.Version
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
	return bson.M{"identity": "management", "userId": userID, "tenantId": tenantID, "projectId": projectID, "revokedAt": bson.M{"$exists": false}}
}

// projectSessionFilter selects every active session bound to one tenant/project.
func projectSessionFilter(tenantID, projectID string) bson.M {
	// Project deactivation/deletion is a lifecycle action affecting both
	// application and management sessions. Membership removal uses the narrower
	// userProjectSessionFilter above.
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

// ValidateManagementRefreshIdentity recognizes a currently refreshable browser
// account without rotating cookies or granting a new session. Legacy families
// may identify their signed account, but application families never do.
func (r *AuthSessionRepository) ValidateManagementRefreshIdentity(ctx context.Context, p models.ManagementPrincipal, jti string, now time.Time) error {
	if p.FamilyID == "" || p.UserID == "" || p.TenantID == "" || p.Scope != "tenant" || p.ProjectID != "" || jti == "" {
		return ErrManagementSessionInvalid
	}
	sum := sha256.Sum256([]byte(jti))
	hash := hex.EncodeToString(sum[:])
	filter := bson.M{"familyId": p.FamilyID, "sessionKey": p.FamilyID, "identity": bson.M{"$ne": "application"}, "userId": p.UserID, "tenantId": p.TenantID, "scope": "tenant", "projectId": bson.M{"$in": bson.A{"", nil}}, "revokedAt": bson.M{"$exists": false}, "expiresAt": bson.M{"$gt": now}, "$or": bson.A{bson.M{"currentJtiHash": hash}, bson.M{"previousJtiHash": hash, "previousValidUntil": bson.M{"$gt": now}}}}
	var session models.AuthSession
	err := r.collection.FindOne(ctx, filter).Decode(&session)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrManagementSessionInvalid
	}
	return err
}

// Version-bounded revocation is idempotent even when audit persistence retries.
// Sessions issued for the new membership generation remain usable.
func (r *AuthSessionRepository) RevokeUserProjectSessionsBeforeVersion(ctx context.Context, user, tenant, project string, version int64, now time.Time) error {
	if user == "" || tenant == "" || project == "" {
		return errors.New("scoped revocation requires user, tenant and project")
	}
	filter := userProjectSessionFilter(user, tenant, project)
	if version > 0 {
		filter["$or"] = bson.A{bson.M{"projectMembershipVersion": bson.M{"$lt": version}}, bson.M{"projectMembershipVersion": bson.M{"$exists": false}}}
	} else {
		filter["projectMembershipVersion"] = bson.M{"$exists": false}
	}
	_, err := r.collection.UpdateMany(ctx, filter, bson.M{"$set": bson.M{"revokedAt": now, "updatedAt": now}})
	return err
}
