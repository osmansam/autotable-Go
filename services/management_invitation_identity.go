package services

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrInvitationOwnership        = errors.New("account ownership could not be verified")
	ErrInvitationReauthentication = errors.New("fresh account authentication is required")
	ErrInvitationAccountMismatch  = errors.New("switch to the invited account before joining")
	ErrInvitationVerification     = errors.New("email verification code is invalid or expired")
)

type InvitationIdentityService struct {
	db            *mongo.Database
	config        configs.InvitationMailConfig
	keys          *utils.InvitationKeyring
	guard         *repositories.ManagementCollaborationRepository
	accounts      *repositories.ManagementAccountRepository
	continuations *repositories.ManagementInvitationContinuationRepository
	now           func() time.Time
}

func invitationOwnershipGuardError(err error) error {
	if errors.Is(err, repositories.ErrCollaborationInactive) {
		return ErrInvitationOwnership
	}
	return err
}

func NewInvitationIdentityService(db *mongo.Database, cfg configs.InvitationMailConfig, keys *utils.InvitationKeyring) *InvitationIdentityService {
	return &InvitationIdentityService{db: db, config: cfg, keys: keys, guard: repositories.NewManagementCollaborationRepository(db), accounts: repositories.NewManagementAccountRepository(db), continuations: repositories.NewManagementInvitationContinuationRepository(db), now: time.Now}
}
func activeContinuationInvitation(ctx context.Context, db *mongo.Database, continuation *models.InvitationContinuation, now time.Time) (*models.ManagementInvitation, error) {
	invite, err := repositories.NewManagementInvitationRepository(db).GetByID(ctx, continuation.InvitationID)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrInvitationInvalid
	}
	if err != nil {
		return nil, err
	}
	if invite.Status != models.InvitationPending || invite.TokenVersion != continuation.InvitationVersion || !invite.ExpiresAt.After(now) {
		return nil, ErrInvitationInvalid
	}
	if err := CurrentInvitationAuthority(ctx, db, *invite); err != nil {
		if errors.Is(err, ErrManagementForbidden) {
			return nil, ErrInvitationInvalid
		}
		return nil, err
	}
	return invite, nil
}
func (s *InvitationIdentityService) proofForContinuation(ctx context.Context, c *models.InvitationContinuation) (models.InvitationIdentityProof, *models.ManagementInvitation, *models.User, error) {
	now := s.now().UTC()
	invite, err := activeContinuationInvitation(ctx, s.db, c, now)
	if err != nil {
		return models.InvitationIdentityProof{}, nil, nil, err
	}
	if c.UserID.IsZero() || !c.ProofExpiresAt.After(now) {
		return models.InvitationIdentityProof{}, nil, nil, ErrInvitationReauthentication
	}
	account, err := s.accounts.GetByID(ctx, c.UserID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return models.InvitationIdentityProof{}, nil, nil, ErrInvitationOwnership
		}
		return models.InvitationIdentityProof{}, nil, nil, err
	}
	if !account.IsActive || account.AccountVersion != c.AccountVersion || account.NormalizedEmail != invite.NormalizedEmail {
		return models.InvitationIdentityProof{}, nil, nil, ErrInvitationOwnership
	}
	proof := models.InvitationIdentityProof{ContinuationID: c.ID, UserID: account.ID.Hex(), NormalizedEmail: account.NormalizedEmail, AccountVersion: account.AccountVersion, AuthenticatedAt: c.AuthenticatedAt, ExpiresAt: c.ProofExpiresAt, ContinuationExpiresAt: c.ExpiresAt, EmailVerified: account.EmailVerifiedAt != nil}
	return proof, invite, account, nil
}
func (s *InvitationIdentityService) ProofForHandle(ctx context.Context, handle string) (models.InvitationIdentityProof, error) {
	continuation, err := s.continuations.GetByHandle(ctx, handle, s.now())
	if errors.Is(err, mongo.ErrNoDocuments) {
		return models.InvitationIdentityProof{}, ErrInvitationInvalid
	}
	if err != nil {
		return models.InvitationIdentityProof{}, err
	}
	proof, _, _, err := s.proofForContinuation(ctx, continuation)
	return proof, err
}
func (s *InvitationIdentityService) validatedProof(ctx context.Context, input models.InvitationIdentityProof) (*models.InvitationContinuation, *models.ManagementInvitation, *models.User, error) {
	c, err := s.continuations.GetByID(ctx, input.ContinuationID, s.now())
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil, nil, ErrInvitationInvalid
		}
		return nil, nil, nil, err
	}
	current, invite, account, err := s.proofForContinuation(ctx, c)
	if err != nil {
		return nil, nil, nil, err
	}
	if input.UserID != current.UserID || input.NormalizedEmail != current.NormalizedEmail || input.AccountVersion != current.AccountVersion || !input.AuthenticatedAt.Equal(current.AuthenticatedAt) || !input.ExpiresAt.Equal(current.ExpiresAt) {
		return nil, nil, nil, ErrInvitationReauthentication
	}
	return c, invite, account, nil
}
func (s *InvitationIdentityService) Reauthenticate(ctx context.Context, handle, email string, password []byte) (string, models.InvitationIdentityProof, error) {
	continuation, err := s.continuations.GetByHandle(ctx, handle, s.now())
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", models.InvitationIdentityProof{}, ErrInvitationInvalid
		}
		return "", models.InvitationIdentityProof{}, err
	}
	invite, err := activeContinuationInvitation(ctx, s.db, continuation, s.now())
	if err != nil {
		return "", models.InvitationIdentityProof{}, err
	}
	account, err := s.accounts.AuthenticatePassword(ctx, email, password)
	if err != nil {
		if errors.Is(err, repositories.ErrManagementAccountInvalid) || errors.Is(err, repositories.ErrManagementAccountDisabled) {
			return "", models.InvitationIdentityProof{}, ErrInvitationOwnership
		}
		return "", models.InvitationIdentityProof{}, err
	}
	if account.NormalizedEmail != invite.NormalizedEmail {
		return "", models.InvitationIdentityProof{}, ErrInvitationOwnership
	}
	if !continuation.UserID.IsZero() && continuation.UserID != account.ID {
		return "", models.InvitationIdentityProof{}, ErrInvitationAccountMismatch
	}
	newHandle, newHash, err := utils.GenerateInvitationToken()
	if err != nil {
		return "", models.InvitationIdentityProof{}, err
	}
	actor, target := models.ManagementAuthorizationSubject{UserID: invite.InvitedBy.Hex(), TenantID: invite.TenantID.Hex()}, models.ManagementAuthorizationTarget{TenantID: invite.TenantID.Hex(), ProjectID: invite.ProjectID.Hex()}
	var result models.InvitationIdentityProof
	err = s.guard.WithAuthorizationTransaction(ctx, actor, target, func(tx context.Context, snapshot models.ManagementMembershipSnapshot) error {
		if err := authorizeCollaboration(invite.Authority, actor, target, snapshot, true); err != nil {
			return ErrInvitationInvalid
		}
		current, err := s.continuations.GetByHandle(tx, handle, s.now())
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return ErrInvitationInvalid
			}
			return err
		}
		freshInvite, err := activeContinuationInvitation(tx, s.db, current, s.now())
		if err != nil {
			return err
		}
		if freshInvite.ID != invite.ID || freshInvite.TokenVersion != invite.TokenVersion {
			return ErrInvitationInvalid
		}
		if !current.UserID.IsZero() && current.UserID != account.ID {
			return ErrInvitationAccountMismatch
		}
		if err := s.guard.GuardAuthorizationEntities(tx, []primitive.ObjectID{account.ID}, invite.TenantID, invite.ProjectID); err != nil {
			return invitationOwnershipGuardError(err)
		}
		live, err := s.accounts.GetByID(tx, account.ID)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return ErrInvitationOwnership
			}
			return err
		}
		if !live.IsActive || live.AccountVersion != account.AccountVersion || live.NormalizedEmail != invite.NormalizedEmail {
			return ErrInvitationOwnership
		}
		now := s.now().UTC().Truncate(time.Millisecond)
		proofExpiry := now.Add(5 * time.Minute)
		if current.ExpiresAt.Before(proofExpiry) {
			proofExpiry = current.ExpiresAt
		}
		changed, err := s.db.Collection("management_invitation_continuations").UpdateOne(tx, bson.M{"_id": current.ID, "handleHash": current.HandleHash, "status": "pending"}, bson.M{"$set": bson.M{"handleHash": newHash, "userId": account.ID, "accountVersion": account.AccountVersion, "authenticatedAt": now, "proofExpiresAt": proofExpiry, "updatedAt": now}})
		if err != nil {
			return err
		}
		if changed.MatchedCount != 1 {
			return ErrInvitationInvalid
		}
		result = models.InvitationIdentityProof{ContinuationID: current.ID, UserID: account.ID.Hex(), NormalizedEmail: live.NormalizedEmail, AccountVersion: account.AccountVersion, AuthenticatedAt: now, ExpiresAt: proofExpiry, ContinuationExpiresAt: current.ExpiresAt, EmailVerified: live.EmailVerifiedAt != nil}
		return nil
	})
	if err != nil {
		return "", models.InvitationIdentityProof{}, err
	}
	return newHandle, result, nil
}
func (s *InvitationIdentityService) verificationEvent(id primitive.ObjectID, account *models.User, invite *models.ManagementInvitation, action string) models.CollaborationEvent {
	now := s.now().UTC()
	return models.CollaborationEvent{ID: id, TenantID: invite.TenantID.Hex(), ProjectID: invite.ProjectID.Hex(), Status: "pending", NextAttemptAt: now, CreatedAt: now, Audit: models.AuditLog{EventID: id, TenantID: invite.TenantID.Hex(), ProjectID: invite.ProjectID.Hex(), UserID: account.ID, Timestamp: primitive.NewDateTimeFromTime(now), Action: action, SchemaName: "management_invitations", DocumentIDs: []primitive.ObjectID{invite.ID}}}
}
func (s *InvitationIdentityService) RequestVerification(ctx context.Context, proof models.InvitationIdentityProof) error {
	_, invite, _, err := s.validatedProof(ctx, proof)
	if err != nil {
		return err
	}
	raw, hash, err := utils.GenerateInvitationToken()
	if err != nil {
		return err
	}
	challengeID, jobID, eventID := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	actor, target := models.ManagementAuthorizationSubject{UserID: invite.InvitedBy.Hex(), TenantID: invite.TenantID.Hex()}, models.ManagementAuthorizationTarget{TenantID: invite.TenantID.Hex(), ProjectID: invite.ProjectID.Hex()}
	return s.guard.WithAuthorizationTransaction(ctx, actor, target, func(tx context.Context, snapshot models.ManagementMembershipSnapshot) error {
		if err := authorizeCollaboration(invite.Authority, actor, target, snapshot, true); err != nil {
			return ErrInvitationInvalid
		}
		continuation, current, account, err := s.validatedProof(tx, proof)
		if err != nil {
			return err
		}
		if account.EmailVerifiedAt != nil {
			return nil
		}
		if err := s.guard.GuardAuthorizationEntities(tx, []primitive.ObjectID{account.ID}, current.TenantID, current.ProjectID); err != nil {
			return invitationOwnershipGuardError(err)
		}
		now := s.now().UTC().Truncate(time.Millisecond)
		expires := now.Add(15 * time.Minute)
		if continuation.ExpiresAt.Before(expires) {
			expires = continuation.ExpiresAt
		}
		var previous models.ManagementEmailChallenge
		err = s.db.Collection("management_email_challenges").FindOne(tx, bson.M{"continuationId": continuation.ID, "status": "pending"}).Decode(&previous)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		id, version := challengeID, int64(1)
		if err == nil {
			id = previous.ID
			version = previous.Version + 1
		}
		challenge := models.ManagementEmailChallenge{ID: id, ContinuationID: continuation.ID, InvitationID: current.ID, InvitationVersion: current.TokenVersion, UserID: account.ID, NormalizedEmail: account.NormalizedEmail, AccountVersion: account.AccountVersion, Version: version, TokenHash: hash, Status: "pending", ExpiresAt: expires, CreatedAt: now, UpdatedAt: now}
		if _, err := s.db.Collection("management_invitation_deliveries").UpdateMany(tx, bson.M{"purpose": "email_verification", "subjectId": id, "status": bson.M{"$in": bson.A{"queued", "leased", "retryable_failure"}}}, bson.M{"$set": bson.M{"status": "obsolete", "updatedAt": now}, "$unset": bson.M{"encryptedPayload": "", "leaseKey": "", "leaseUntil": ""}}); err != nil {
			return err
		}
		if _, err := s.db.Collection("management_email_challenges").ReplaceOne(tx, bson.M{"_id": id}, challenge, options.Replace().SetUpsert(true)); err != nil {
			return err
		}
		panel, err := url.Parse(s.config.PanelURL)
		if err != nil || panel.Scheme != "https" {
			return ErrInvitationConfiguration
		}
		panel.Path = "/invitations/accept"
		panel.Fragment = "verification=" + raw
		job := models.InvitationDeliveryJob{ID: jobID, JobKey: fmt.Sprintf("email_verification:%s:%d", id.Hex(), version), Purpose: "email_verification", SubjectID: id, InvitationID: current.ID, TenantID: current.TenantID, ProjectID: current.ProjectID, TokenVersion: version, MessageID: fmt.Sprintf("autotable.%s.%d.verification@%s", id.Hex(), version, panel.Hostname()), Status: "queued", NextAttemptAt: now, ExpiresAt: expires, CreatedAt: now, UpdatedAt: now}
		message := models.InvitationMessage{To: account.NormalizedEmail, MessageID: job.MessageID, Subject: "Verify your AutoTable email", TextBody: fmt.Sprintf("Confirm your email to join your invited project.\n\nVerification code:\n%s\n\nOpen in the browser where you signed in:\n%s\n\nThe code expires on %s. If asked, sign in again before entering it.\n", raw, panel.String(), expires.Format(time.RFC1123))}
		payload, err := json.Marshal(message)
		if err != nil {
			return err
		}
		job.EncryptedPayload, err = s.keys.Seal(payload, invitationDeliveryAAD(job))
		if err != nil {
			return err
		}
		if _, err := s.db.Collection("management_invitation_deliveries").InsertOne(tx, job); err != nil {
			return err
		}
		return s.guard.InsertCollaborationEvent(tx, s.verificationEvent(eventID, account, current, "project.invitation.email_verification_requested"))
	})
}
func (s *InvitationIdentityService) VerifyEmail(ctx context.Context, proof models.InvitationIdentityProof, raw string) error {
	if len(raw) > 128 {
		return ErrInvitationVerification
	}
	_, invite, _, err := s.validatedProof(ctx, proof)
	if err != nil {
		return err
	}
	eventID := primitive.NewObjectID()
	actor, target := models.ManagementAuthorizationSubject{UserID: invite.InvitedBy.Hex(), TenantID: invite.TenantID.Hex()}, models.ManagementAuthorizationTarget{TenantID: invite.TenantID.Hex(), ProjectID: invite.ProjectID.Hex()}
	var outcome error
	err = s.guard.WithAuthorizationTransaction(ctx, actor, target, func(tx context.Context, snapshot models.ManagementMembershipSnapshot) error {
		outcome = nil
		if err := authorizeCollaboration(invite.Authority, actor, target, snapshot, true); err != nil {
			return ErrInvitationInvalid
		}
		continuation, current, account, err := s.validatedProof(tx, proof)
		if err != nil {
			return err
		}
		var challenge models.ManagementEmailChallenge
		err = s.db.Collection("management_email_challenges").FindOne(tx, bson.M{"continuationId": continuation.ID, "userId": account.ID, "status": "pending", "invitationId": current.ID, "invitationVersion": current.TokenVersion, "expiresAt": bson.M{"$gt": s.now()}, "accountVersion": account.AccountVersion}).Decode(&challenge)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrInvitationVerification
		}
		if err != nil {
			return err
		}
		if challenge.Attempts >= 5 {
			return ErrInvitationVerification
		}
		if subtle.ConstantTimeCompare([]byte(challenge.TokenHash), []byte(utils.HashInvitationToken(raw))) != 1 {
			set := bson.M{"attempts": challenge.Attempts + 1, "updatedAt": s.now().UTC()}
			if challenge.Attempts+1 >= 5 {
				set["status"] = "failed"
			}
			if _, err := s.db.Collection("management_email_challenges").UpdateOne(tx, bson.M{"_id": challenge.ID}, bson.M{"$set": set}); err != nil {
				return err
			}
			outcome = ErrInvitationVerification
			return nil
		}
		if err := s.guard.GuardAuthorizationEntities(tx, []primitive.ObjectID{account.ID}, current.TenantID, current.ProjectID); err != nil {
			return invitationOwnershipGuardError(err)
		}
		now := s.now().UTC().Truncate(time.Millisecond)
		if _, err := s.db.Collection("users").UpdateOne(tx, bson.M{"_id": account.ID}, bson.M{"$set": bson.M{"emailVerifiedAt": now, "updatedAt": now}}); err != nil {
			return err
		}
		if _, err := s.db.Collection("management_email_challenges").UpdateOne(tx, bson.M{"_id": challenge.ID, "status": "pending"}, bson.M{"$set": bson.M{"status": "consumed", "updatedAt": now}}); err != nil {
			return err
		}
		return s.guard.InsertCollaborationEvent(tx, s.verificationEvent(eventID, account, current, "project.invitation.email_verified"))
	})
	if err != nil {
		return err
	}
	return outcome
}
