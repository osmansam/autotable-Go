package services

import (
	"context"
	"errors"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"regexp"
	"strings"
)

func (s *ProjectCollaborationService) RecoverAdmin(ctx context.Context, input models.RecoverAdminInput) (*models.ProjectCollaborator, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" || len(input.Reason) > 500 || input.ExpectedVersion < 0 {
		return nil, ErrCollaborationInvalid
	}
	tenant, project, user, err := collaborationIDs(input.Target, input.UserID)
	if err != nil {
		return nil, err
	}
	eventID, membershipID := primitive.NewObjectID(), primitive.NewObjectID()
	var result *models.ProjectCollaborator
	err = s.repository.WithAuthorizationTransaction(ctx, input.Actor, input.Target, func(tx context.Context, snapshot models.ManagementMembershipSnapshot) error {
		if err := authorizeCollaboration(models.CollaborationAuthorityTenant, input.Actor, input.Target, snapshot, true); err != nil {
			return err
		}
		admins, err := s.usableAdmins(tx, input.Target)
		if err != nil {
			return err
		}
		if admins > 0 {
			return ErrCollaborationRecoveryNotRequired
		}
		if err := s.repository.GuardAuthorizationEntities(tx, []primitive.ObjectID{user}, tenant, project); err != nil {
			return err
		}
		account, err := repositories.NewManagementAccountRepository(s.repository.Database()).GetByID(tx, user)
		if err != nil {
			return err
		}
		if !account.IsActive || account.EmailVerifiedAt == nil {
			return ErrManagementForbidden
		}
		recipient, err := repositories.NewManagementMembershipRepositoryWithDatabase(s.repository.Database()).LoadManagementMembership(tx, models.ManagementAuthorizationSubject{UserID: input.UserID, TenantID: input.Target.TenantID}, input.Target)
		if err != nil {
			return err
		}
		if _, err := CurrentManagementSessionRoles(recipient, false); err != nil {
			return err
		}
		member, err := s.member(tx, input.Target, input.UserID)
		missing := errors.Is(err, mongo.ErrNoDocuments)
		if err != nil && !missing {
			return err
		}
		if member.Version != input.ExpectedVersion {
			return ErrCollaborationVersion
		}
		before := member
		now := s.now().UTC()
		if missing {
			member = models.ProjectMembership{ID: membershipID, TenantID: tenant, ProjectID: project, UserID: user, CreatedBy: primitive.NilObjectID, CreatedAt: now}
			member.CreatedBy, _ = primitive.ObjectIDFromHex(input.Actor.UserID)
		}
		if !hasProjectAdmin(member.Roles) {
			member.Roles = append(append([]string{}, member.Roles...), models.ProjectRoleAdmin)
		}
		member.Status = models.MembershipStatusActive
		member.Version++
		member.UpdatedAt = now
		if missing {
			_, err = s.repository.Database().Collection("project_memberships").InsertOne(tx, member)
		} else {
			_, err = s.repository.Database().Collection("project_memberships").UpdateOne(tx, bson.M{"_id": member.ID}, bson.M{"$set": bson.M{"roles": member.Roles, "status": member.Status, "version": member.Version, "updatedAt": member.UpdatedAt}})
		}
		if err != nil {
			return err
		}
		if err := s.repository.InsertCollaborationEvent(tx, s.mutationEvent(eventID, input.Actor, input.Target, member, before, "project.member.admin_recovered", input.Reason)); err != nil {
			return err
		}
		result, err = s.collaborator(tx, member)
		return err
	})
	return result, err
}
func (s *ProjectCollaborationService) RecoveryCandidates(ctx context.Context, actor models.ManagementAuthorizationSubject, target models.ManagementAuthorizationTarget, query string) ([]models.CollaborationRecoveryCandidate, error) {
	snapshot, err := repositories.NewManagementMembershipRepositoryWithDatabase(s.repository.Database()).LoadManagementMembership(ctx, actor, target)
	if err != nil {
		return nil, err
	}
	if err := authorizeCollaboration(models.CollaborationAuthorityTenant, actor, target, snapshot, false); err != nil {
		return nil, err
	}
	query = models.NormalizeManagementEmail(query)
	if len(query) < 3 {
		return []models.CollaborationRecoveryCandidate{}, nil
	}
	if len(query) > 254 {
		return nil, ErrCollaborationInvalid
	}
	tenant, err := primitive.ObjectIDFromHex(target.TenantID)
	if err != nil {
		return nil, ErrCollaborationInvalid
	}
	project, err := primitive.ObjectIDFromHex(target.ProjectID)
	if err != nil {
		return nil, ErrCollaborationInvalid
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"tenantId": tenant, "status": models.MembershipStatusActive, "roles": bson.M{"$in": bson.A{models.TenantRoleOwner, models.TenantRoleAdmin, models.TenantRoleBilling, models.TenantRoleAuditor, models.TenantRoleMember}}}}},
		{{Key: "$lookup", Value: bson.M{"from": "users", "let": bson.M{"user": "$userId"}, "pipeline": bson.A{bson.M{"$match": bson.M{"$expr": bson.M{"$eq": bson.A{"$_id", "$$user"}}, "isActive": true, "emailVerifiedAt": bson.M{"$type": "date"}, "normalizedEmail": primitive.Regex{Pattern: "^" + regexp.QuoteMeta(query)}}}, bson.M{"$project": bson.M{"_id": 1, "name": 1, "email": 1}}}, "as": "account"}}},
		{{Key: "$unwind", Value: "$account"}}, {{Key: "$replaceRoot", Value: bson.M{"newRoot": "$account"}}}, {{Key: "$sort", Value: bson.M{"_id": 1}}}, {{Key: "$limit", Value: 20}},
		{{Key: "$lookup", Value: bson.M{"from": "project_memberships", "let": bson.M{"user": "$_id"}, "pipeline": bson.A{bson.M{"$match": bson.M{"tenantId": tenant, "projectId": project, "$expr": bson.M{"$eq": bson.A{"$userId", "$$user"}}}}, bson.M{"$project": bson.M{"version": 1}}, bson.M{"$limit": 1}}, "as": "membership"}}},
		{{Key: "$set", Value: bson.M{"membershipVersion": bson.M{"$ifNull": bson.A{bson.M{"$arrayElemAt": bson.A{"$membership.version", 0}}, int64(0)}}}}},
		{{Key: "$project", Value: bson.M{"membership": 0}}},
	}
	cursor, err := s.repository.Database().Collection("tenant_memberships").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	result := []models.CollaborationRecoveryCandidate{}
	err = cursor.All(ctx, &result)
	return result, err
}
