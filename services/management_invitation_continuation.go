package services

import (
	"context"
	"encoding/base64"
	"errors"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

type InvitationContinuationService struct {
	db         *mongo.Database
	repository *repositories.ManagementInvitationContinuationRepository
	now        func() time.Time
}

func NewInvitationContinuationService(db *mongo.Database) *InvitationContinuationService {
	return &InvitationContinuationService{db: db, repository: repositories.NewManagementInvitationContinuationRepository(db), now: time.Now}
}
func (s *InvitationContinuationService) preview(ctx context.Context, c *models.InvitationContinuation, invite *models.ManagementInvitation) (models.InvitationPreview, error) {
	var tenant models.Tenant
	var project models.Project
	if err := s.db.Collection("tenants").FindOne(ctx, bson.M{"_id": invite.TenantID, "isActive": true}).Decode(&tenant); err != nil {
		return models.InvitationPreview{}, err
	}
	if err := s.db.Collection("projects").FindOne(ctx, bson.M{"_id": invite.ProjectID, "tenantId": invite.TenantID, "isActive": true, "initializing": bson.M{"$ne": true}}).Decode(&project); err != nil {
		return models.InvitationPreview{}, err
	}
	result := models.InvitationPreview{TenantName: tenant.Name, ProjectName: project.Name, Role: invite.Role, ExpiresAt: invite.ExpiresAt, ContinuationExpiresAt: c.ExpiresAt, Stage: "choose_account"}
	if !c.UserID.IsZero() {
		identity := NewInvitationIdentityService(s.db, configs.InvitationMailConfig{}, nil)
		identity.now = s.now
		proof, _, _, err := identity.proofForContinuation(ctx, c)
		if errors.Is(err, ErrInvitationReauthentication) || errors.Is(err, ErrInvitationOwnership) {
			result.Stage = "reauthenticate"
			return result, nil
		}
		if err != nil {
			return result, err
		}
		result.Stage = "verify_email"
		if proof.EmailVerified {
			result.Stage = "ready"
		} else {
			var challenge models.ManagementEmailChallenge
			err := s.db.Collection("management_email_challenges").FindOne(ctx, bson.M{"continuationId": c.ID, "status": "pending"}).Decode(&challenge)
			if err == nil {
				var job struct {
					Status string `bson:"status"`
				}
				err = s.db.Collection("management_invitation_deliveries").FindOne(ctx, bson.M{"subjectId": challenge.ID, "purpose": "email_verification", "tokenVersion": challenge.Version}, options.FindOne().SetProjection(bson.M{"status": 1})).Decode(&job)
				if err == nil {
					result.VerificationDeliveryState = job.Status
				}
			}
			if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
				return result, err
			}
		}
	}
	return result, nil
}
func (s *InvitationContinuationService) Exchange(ctx context.Context, rawToken string) (string, models.InvitationPreview, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(rawToken)
	if err != nil || len(rawToken) != 43 || len(decoded) != 32 {
		return "", models.InvitationPreview{}, ErrInvitationInvalid
	}
	var invitation models.ManagementInvitation
	filter := bson.M{"tokenHash": utils.HashInvitationToken(rawToken), "status": models.InvitationPending, "expiresAt": bson.M{"$gt": s.now()}}
	if err := s.db.Collection("management_invitations").FindOne(ctx, filter).Decode(&invitation); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", models.InvitationPreview{}, ErrInvitationInvalid
		}
		return "", models.InvitationPreview{}, err
	}
	if !IsInvitableProjectRole(invitation.Role) {
		return "", models.InvitationPreview{}, ErrInvitationInvalid
	}
	handle, hash, err := utils.GenerateInvitationToken()
	if err != nil {
		return "", models.InvitationPreview{}, err
	}
	id := primitive.NewObjectID()
	var preview models.InvitationPreview
	actor, target := models.ManagementAuthorizationSubject{UserID: invitation.InvitedBy.Hex(), TenantID: invitation.TenantID.Hex()}, models.ManagementAuthorizationTarget{TenantID: invitation.TenantID.Hex(), ProjectID: invitation.ProjectID.Hex()}
	err = repositories.NewManagementCollaborationRepository(s.db).WithAuthorizationTransaction(ctx, actor, target, func(tx context.Context, snapshot models.ManagementMembershipSnapshot) error {
		if err := authorizeCollaboration(invitation.Authority, actor, target, snapshot, true); err != nil {
			return ErrInvitationInvalid
		}
		var current models.ManagementInvitation
		if err := s.db.Collection("management_invitations").FindOne(tx, bson.M{"tokenHash": utils.HashInvitationToken(rawToken), "status": models.InvitationPending, "expiresAt": bson.M{"$gt": s.now()}}).Decode(&current); err != nil {
			return ErrInvitationInvalid
		}
		now := s.now().UTC().Truncate(time.Millisecond)
		expires := now.Add(30 * time.Minute)
		if current.ExpiresAt.Before(expires) {
			expires = current.ExpiresAt
		}
		continuation := models.InvitationContinuation{ID: id, InvitationID: current.ID, InvitationVersion: current.TokenVersion, HandleHash: hash, Purpose: "project_invitation", Status: "pending", ExpiresAt: expires, CreatedAt: now, UpdatedAt: now}
		if _, err := s.db.Collection("management_invitation_continuations").InsertOne(tx, continuation); err != nil {
			return err
		}
		var err error
		preview, err = s.preview(tx, &continuation, &current)
		return err
	})
	if err != nil {
		return "", models.InvitationPreview{}, err
	}
	return handle, preview, nil
}
func (s *InvitationContinuationService) Resume(ctx context.Context, handle string) (models.InvitationPreview, error) {
	c, err := s.repository.GetByHandle(ctx, handle, s.now())
	if errors.Is(err, mongo.ErrNoDocuments) {
		return models.InvitationPreview{}, ErrInvitationInvalid
	}
	if err != nil {
		return models.InvitationPreview{}, err
	}
	invite, err := activeContinuationInvitation(ctx, s.db, c, s.now())
	if err != nil {
		return models.InvitationPreview{}, err
	}
	return s.preview(ctx, c, invite)
}
func (s *InvitationContinuationService) Cancel(ctx context.Context, handle string) error {
	if handle == "" {
		return nil
	}
	_, err := s.db.Collection("management_invitation_continuations").UpdateOne(ctx, bson.M{"handleHash": utils.HashInvitationToken(handle), "status": "pending"}, bson.M{"$set": bson.M{"status": "canceled", "updatedAt": s.now().UTC()}})
	return err
}
