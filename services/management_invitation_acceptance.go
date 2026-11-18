package services

import (
	"context"
	"errors"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvitationMailboxRequired = errors.New("verify the invited email before joining")

type InvitationAcceptanceService struct {
	db            *mongo.Database
	guard         *repositories.ManagementCollaborationRepository
	continuations *repositories.ManagementInvitationContinuationRepository
	now           func() time.Time
}

func NewInvitationAcceptanceService(db *mongo.Database) *InvitationAcceptanceService {
	return &InvitationAcceptanceService{db: db, guard: repositories.NewManagementCollaborationRepository(db), continuations: repositories.NewManagementInvitationContinuationRepository(db), now: time.Now}
}
func (s *InvitationAcceptanceService) Accept(ctx context.Context, input models.AcceptProjectInvitationInput) (models.AcceptedProjectInvitation, error) {
	continuation, err := s.continuations.GetByHandle(ctx, input.ContinuationHandle, s.now())
	if err != nil {
		return models.AcceptedProjectInvitation{}, ErrInvitationInvalid
	}
	invite, err := activeContinuationInvitation(ctx, s.db, continuation, s.now())
	if err != nil {
		return models.AcceptedProjectInvitation{}, err
	}
	if !IsInvitableProjectRole(invite.Role) {
		return models.AcceptedProjectInvitation{}, ErrInvitationInvalid
	}
	userID, membershipID, tenantMembershipID, eventID := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	var passwordHash []byte
	if input.CreateAccount {
		if !continuation.UserID.IsZero() || input.Proof.UserID != "" {
			return models.AcceptedProjectInvitation{}, ErrInvitationOwnership
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Name == "" || utf8.RuneCountInString(input.Name) > 120 || len(input.Password) < 8 || len(input.Password) > 72 {
			return models.AcceptedProjectInvitation{}, ErrCollaborationInvalid
		}
		passwordHash, err = bcrypt.GenerateFromPassword(input.Password, bcrypt.DefaultCost)
		if err != nil {
			return models.AcceptedProjectInvitation{}, ErrCollaborationInvalid
		}
		defer clear(passwordHash)
	} else if input.Proof.UserID == "" || input.Name != "" || len(input.Password) > 0 {
		return models.AcceptedProjectInvitation{}, ErrInvitationReauthentication
	}
	actor, target := models.ManagementAuthorizationSubject{UserID: invite.InvitedBy.Hex(), TenantID: invite.TenantID.Hex()}, models.ManagementAuthorizationTarget{TenantID: invite.TenantID.Hex(), ProjectID: invite.ProjectID.Hex()}
	var result models.AcceptedProjectInvitation
	err = s.guard.WithAuthorizationTransaction(ctx, actor, target, func(tx context.Context, snapshot models.ManagementMembershipSnapshot) error {
		if err := authorizeCollaboration(invite.Authority, actor, target, snapshot, true); err != nil {
			return ErrInvitationInvalid
		}
		current, err := s.continuations.GetByHandle(tx, input.ContinuationHandle, s.now())
		if err != nil {
			return ErrInvitationInvalid
		}
		fresh, err := activeContinuationInvitation(tx, s.db, current, s.now())
		if err != nil {
			return err
		}
		if fresh.ID != invite.ID || fresh.TokenVersion != invite.TokenVersion || !IsInvitableProjectRole(fresh.Role) {
			return ErrInvitationInvalid
		}
		now := s.now().UTC().Truncate(time.Millisecond)
		var user models.User
		if input.CreateAccount {
			if !current.UserID.IsZero() {
				return ErrInvitationOwnership
			}
			count, err := s.db.Collection("users").CountDocuments(tx, repositories.ManagementEmailLookup(fresh.NormalizedEmail))
			if err != nil {
				return err
			}
			if count > 0 {
				return ErrInvitationOwnership
			}
			user = models.User{ID: userID, Email: fresh.NormalizedEmail, NormalizedEmail: fresh.NormalizedEmail, Name: input.Name, EmailVerifiedAt: &now, IsActive: true, CreatedAt: now, UpdatedAt: now}
			document := bson.M{"_id": user.ID, "email": user.Email, "normalizedEmail": user.NormalizedEmail, "name": user.Name, "password": string(passwordHash), "emailVerifiedAt": now, "isActive": true, "accountVersion": int64(0), "createdAt": now, "updatedAt": now}
			if _, err := s.db.Collection("users").InsertOne(tx, document); err != nil {
				if mongo.IsDuplicateKeyError(err) {
					return ErrInvitationOwnership
				}
				return err
			}
		} else {
			identity := NewInvitationIdentityService(s.db, configs.InvitationMailConfig{}, nil)
			identity.now = s.now
			proofContinuation, _, account, err := identity.validatedProof(tx, input.Proof)
			if err != nil {
				return err
			}
			if proofContinuation.ID != current.ID || account.ID != current.UserID {
				return ErrInvitationOwnership
			}
			if account.EmailVerifiedAt == nil {
				return ErrInvitationMailboxRequired
			}
			user = *account
			if err := s.guard.GuardAuthorizationEntities(tx, []primitive.ObjectID{user.ID}, fresh.TenantID, fresh.ProjectID); err != nil {
				return invitationOwnershipGuardError(err)
			}
		}
		var tenantMembership models.TenantMembership
		err = s.db.Collection("tenant_memberships").FindOne(tx, bson.M{"tenantId": fresh.TenantID, "userId": user.ID}).Decode(&tenantMembership)
		createdTenantMembership := false
		if errors.Is(err, mongo.ErrNoDocuments) {
			tenantMembership = models.TenantMembership{ID: tenantMembershipID, TenantID: fresh.TenantID, UserID: user.ID, Roles: []string{models.TenantRoleMember}, Status: models.MembershipStatusActive, CreatedBy: fresh.InvitedBy, CreatedAt: now, UpdatedAt: now}
			if _, err := s.db.Collection("tenant_memberships").InsertOne(tx, tenantMembership); err != nil {
				return err
			}
			createdTenantMembership = true
		} else if err != nil {
			return err
		}
		if _, err := CurrentManagementSessionRoles(models.ManagementMembershipSnapshot{UserActive: true, TenantActive: true, TenantMembership: &tenantMembership}, false); err != nil {
			return ErrManagementForbidden
		}
		count, err := s.db.Collection("project_memberships").CountDocuments(tx, bson.M{"tenantId": fresh.TenantID, "projectId": fresh.ProjectID, "userId": user.ID})
		if err != nil {
			return err
		}
		if count != 0 {
			return ErrInvitationMemberExists
		}
		projectMembership := models.ProjectMembership{ID: membershipID, TenantID: fresh.TenantID, ProjectID: fresh.ProjectID, UserID: user.ID, Roles: []string{fresh.Role}, Status: models.MembershipStatusActive, Version: 1, CreatedBy: fresh.InvitedBy, CreatedAt: now, UpdatedAt: now}
		if _, err := s.db.Collection("project_memberships").InsertOne(tx, projectMembership); err != nil {
			return err
		}
		changed, err := s.db.Collection("management_invitations").UpdateOne(tx, bson.M{"_id": fresh.ID, "status": models.InvitationPending, "tokenVersion": fresh.TokenVersion, "expiresAt": bson.M{"$gt": s.now()}}, bson.M{"$set": bson.M{"status": models.InvitationAccepted, "acceptedBy": user.ID, "acceptedAt": now, "updatedAt": now}})
		if err != nil {
			return err
		}
		if changed.MatchedCount != 1 {
			return ErrInvitationInvalid
		}
		consumed, err := s.db.Collection("management_invitation_continuations").DeleteOne(tx, bson.M{"_id": current.ID, "handleHash": current.HandleHash, "status": "pending", "expiresAt": bson.M{"$gt": s.now()}})
		if err != nil {
			return err
		}
		if consumed.DeletedCount != 1 {
			return ErrInvitationInvalid
		}
		issuer := NewInvitationService(s.db, configs.InvitationMailConfig{}, nil)
		issuer.now = s.now
		if err := issuer.obsoleteJobs(tx, fresh.ID); err != nil {
			return err
		}
		event := issuer.invitationEvent(eventID, models.ManagementAuthorizationSubject{UserID: user.ID.Hex(), TenantID: fresh.TenantID.Hex()}, *fresh, "project.invitation.accepted")
		event.Audit.After = bson.M{"userId": user.ID, "roles": projectMembership.Roles, "tenantMembershipCreated": createdTenantMembership}
		if err := s.guard.InsertCollaborationEvent(tx, event); err != nil {
			return err
		}
		var tenant models.Tenant
		var project models.Project
		if err := s.db.Collection("tenants").FindOne(tx, bson.M{"_id": fresh.TenantID, "isActive": true}).Decode(&tenant); err != nil {
			return err
		}
		if err := s.db.Collection("projects").FindOne(tx, bson.M{"_id": fresh.ProjectID, "tenantId": fresh.TenantID, "isActive": true, "initializing": bson.M{"$ne": true}}).Decode(&project); err != nil {
			return err
		}
		tenants, err := ActiveManagementTenants(tx, s.db, user.ID)
		if err != nil {
			return err
		}
		result = models.AcceptedProjectInvitation{User: user, Tenant: tenant, Project: project, AllTenants: tenants, TenantRoles: append([]string{}, tenantMembership.Roles...), Roles: projectMembership.Roles}
		return nil
	})
	return result, err
}
func ActiveManagementTenants(ctx context.Context, db *mongo.Database, user primitive.ObjectID) ([]models.Tenant, error) {
	cursor, err := db.Collection("tenant_memberships").Find(ctx, bson.M{"userId": user, "status": models.MembershipStatusActive, "roles": bson.M{"$in": bson.A{models.TenantRoleOwner, models.TenantRoleAdmin, models.TenantRoleBilling, models.TenantRoleAuditor, models.TenantRoleMember}}})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var memberships []models.TenantMembership
	if err := cursor.All(ctx, &memberships); err != nil {
		return nil, err
	}
	ids := bson.A{}
	for _, m := range memberships {
		ids = append(ids, m.TenantID)
	}
	tenantCursor, err := db.Collection("tenants").Find(ctx, bson.M{"_id": bson.M{"$in": ids}, "isActive": true})
	if err != nil {
		return nil, err
	}
	defer tenantCursor.Close(ctx)
	tenants := []models.Tenant{}
	err = tenantCursor.All(ctx, &tenants)
	return tenants, err
}
