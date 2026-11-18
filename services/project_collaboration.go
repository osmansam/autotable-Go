package services

import (
	"context"
	"errors"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"reflect"
	"sort"
	"time"
)

var (
	ErrCollaborationInvalid             = errors.New("invalid collaboration request")
	ErrCollaborationVersion             = errors.New("member changed; reload before editing")
	ErrCollaborationLegacyRoles         = errors.New("legacy role set requires explicit conversion")
	ErrCollaborationLastAdmin           = errors.New("the last usable project administrator cannot be removed")
	ErrCollaborationRecoveryRequired    = errors.New("this project requires verified administrator recovery")
	ErrCollaborationRecoveryNotRequired = errors.New("a usable project administrator already exists")
)

type ProjectCollaborationService struct {
	repository *repositories.ManagementCollaborationRepository
	now        func() time.Time
}

func NewProjectCollaborationService(db *mongo.Database) *ProjectCollaborationService {
	return &ProjectCollaborationService{repository: repositories.NewManagementCollaborationRepository(db), now: time.Now}
}
func authorizeCollaboration(authority models.CollaborationAuthority, actor models.ManagementAuthorizationSubject, target models.ManagementAuthorizationTarget, snapshot models.ManagementMembershipSnapshot, manage bool) error {
	var permission ManagementPermission
	switch authority {
	case models.CollaborationAuthorityTenant:
		permission = PermissionTenantProjectMembersRead
		if manage {
			permission = PermissionTenantProjectMembersManage
		}
	case models.CollaborationAuthorityProject:
		permission = PermissionProjectMembersRead
		if manage {
			permission = PermissionProjectMembersManage
		}
	default:
		return ErrManagementForbidden
	}
	return AuthorizeManagementSnapshot(actor, target, permission, snapshot)
}
func IsInvitableProjectRole(role string) bool {
	switch role {
	case models.ProjectRoleAdmin, models.ProjectRoleDeveloper, models.ProjectRoleEditor, models.ProjectRoleViewer:
		return true
	}
	return false
}
func hasProjectAdmin(roles []string) bool {
	return containsAnyRole(roles, []string{models.ProjectRoleAdmin})
}
func validateCollaborationRoleChange(member models.ProjectMembership, input models.RoleChangeInput) error {
	if !IsInvitableProjectRole(input.Role) || input.ExpectedVersion < 0 {
		return ErrCollaborationInvalid
	}
	if member.Version != input.ExpectedVersion {
		return ErrCollaborationVersion
	}
	legacy := len(member.Roles) != 1 || !IsInvitableProjectRole(member.Roles[0])
	if legacy {
		if !input.ConvertLegacyRoleSet {
			return ErrCollaborationLegacyRoles
		}
		expected, actual := append([]string{}, input.ExpectedRoles...), append([]string{}, member.Roles...)
		sort.Strings(expected)
		sort.Strings(actual)
		if !reflect.DeepEqual(expected, actual) {
			return ErrCollaborationVersion
		}
	}
	return nil
}
func collaborationIDs(target models.ManagementAuthorizationTarget, userID string) (primitive.ObjectID, primitive.ObjectID, primitive.ObjectID, error) {
	tenant, e1 := primitive.ObjectIDFromHex(target.TenantID)
	project, e2 := primitive.ObjectIDFromHex(target.ProjectID)
	user, e3 := primitive.ObjectIDFromHex(userID)
	if e1 != nil || e2 != nil || e3 != nil {
		return tenant, project, user, ErrCollaborationInvalid
	}
	return tenant, project, user, nil
}
func (s *ProjectCollaborationService) member(ctx context.Context, target models.ManagementAuthorizationTarget, userID string) (models.ProjectMembership, error) {
	tenant, project, user, err := collaborationIDs(target, userID)
	if err != nil {
		return models.ProjectMembership{}, err
	}
	var member models.ProjectMembership
	err = s.repository.Database().Collection("project_memberships").FindOne(ctx, bson.M{"tenantId": tenant, "projectId": project, "userId": user}).Decode(&member)
	return member, err
}
func (s *ProjectCollaborationService) collaborator(ctx context.Context, member models.ProjectMembership) (*models.ProjectCollaborator, error) {
	var user models.User
	err := s.repository.Database().Collection("users").FindOne(ctx, bson.M{"_id": member.UserID}).Decode(&user)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	return &models.ProjectCollaborator{ID: member.ID, UserID: member.UserID, Email: user.Email, Name: user.Name, Roles: append([]string{}, member.Roles...), Status: member.Status, Version: member.Version, CreatedAt: member.CreatedAt, UpdatedAt: member.UpdatedAt}, nil
}
func usableProjectAdminPipeline(target models.ManagementAuthorizationTarget) (mongo.Pipeline, error) {
	tenant, project, _, err := collaborationIDs(target, primitive.NewObjectID().Hex())
	if err != nil {
		return nil, err
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"tenantId": tenant, "projectId": project, "status": models.MembershipStatusActive, "roles": models.ProjectRoleAdmin}}},
		{{Key: "$lookup", Value: bson.M{"from": "users", "localField": "userId", "foreignField": "_id", "as": "user"}}},
		{{Key: "$match", Value: bson.M{"user.isActive": true}}},
		{{Key: "$lookup", Value: bson.M{"from": "tenant_memberships", "let": bson.M{"user": "$userId"}, "pipeline": bson.A{bson.M{"$match": bson.M{"tenantId": tenant, "status": models.MembershipStatusActive, "roles": bson.M{"$in": bson.A{models.TenantRoleOwner, models.TenantRoleAdmin, models.TenantRoleBilling, models.TenantRoleAuditor, models.TenantRoleMember}}, "$expr": bson.M{"$eq": bson.A{"$userId", "$$user"}}}}}, "as": "tenantMembership"}}},
		{{Key: "$match", Value: bson.M{"tenantMembership.0": bson.M{"$exists": true}}}},
	}
	return pipeline, nil
}

func (s *ProjectCollaborationService) usableAdmins(ctx context.Context, target models.ManagementAuthorizationTarget) (int, error) {
	pipeline, err := usableProjectAdminPipeline(target)
	if err != nil {
		return 0, err
	}
	pipeline = append(pipeline, bson.D{{Key: "$count", Value: "count"}})
	cursor, err := s.repository.Database().Collection("project_memberships").Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)
	var result []struct {
		Count int `bson:"count"`
	}
	if err = cursor.All(ctx, &result); err != nil {
		return 0, err
	}
	if len(result) == 0 {
		return 0, nil
	}
	return result[0].Count, nil
}
func (s *ProjectCollaborationService) mutationEvent(id primitive.ObjectID, actor models.ManagementAuthorizationSubject, target models.ManagementAuthorizationTarget, member models.ProjectMembership, before models.ProjectMembership, action, reason string) models.CollaborationEvent {
	actorID, _ := primitive.ObjectIDFromHex(actor.UserID)
	now := s.now().UTC()
	return models.CollaborationEvent{ID: id, TenantID: target.TenantID, ProjectID: target.ProjectID, RevokeUserID: member.UserID.Hex(), RevokeBeforeVersion: member.Version, Status: "pending", NextAttemptAt: now, CreatedAt: now, Audit: models.AuditLog{EventID: id, TenantID: target.TenantID, ProjectID: target.ProjectID, Timestamp: primitive.NewDateTimeFromTime(now), UserID: actorID, SchemaName: "project_memberships", DocumentIDs: []primitive.ObjectID{member.ID}, Action: action, Description: reason, Before: bson.M{"roles": before.Roles, "status": before.Status, "version": before.Version}, After: bson.M{"roles": member.Roles, "status": member.Status, "version": member.Version}}}
}
func (s *ProjectCollaborationService) ChangeRole(ctx context.Context, input models.RoleChangeInput) (*models.ProjectCollaborator, error) {
	tenant, project, user, err := collaborationIDs(input.Target, input.UserID)
	if err != nil {
		return nil, err
	}
	eventID := primitive.NewObjectID()
	var result *models.ProjectCollaborator
	err = s.repository.WithAuthorizationTransaction(ctx, input.Actor, input.Target, func(tx context.Context, snapshot models.ManagementMembershipSnapshot) error {
		if err := authorizeCollaboration(input.Authority, input.Actor, input.Target, snapshot, true); err != nil {
			return err
		}
		member, err := s.member(tx, input.Target, input.UserID)
		if err != nil {
			return err
		}
		if err := validateCollaborationRoleChange(member, input); err != nil {
			return err
		}
		if err := s.repository.GuardAuthorizationEntities(tx, []primitive.ObjectID{user}, tenant, project); err != nil {
			return err
		}
		recipient, err := repositories.NewManagementMembershipRepositoryWithDatabase(s.repository.Database()).LoadManagementMembership(tx, models.ManagementAuthorizationSubject{UserID: input.UserID, TenantID: input.Target.TenantID}, input.Target)
		if err != nil {
			return err
		}
		if _, err := CurrentManagementSessionRoles(recipient, false); err != nil {
			return err
		}
		admins, err := s.usableAdmins(tx, input.Target)
		if err != nil {
			return err
		}
		if admins == 0 {
			return ErrCollaborationRecoveryRequired
		}
		before := member
		newStatus := member.Status
		if input.ActivateMembership {
			newStatus = models.MembershipStatusActive
		}
		if member.Status == models.MembershipStatusActive && hasProjectAdmin(member.Roles) && (input.Role != models.ProjectRoleAdmin || newStatus != models.MembershipStatusActive) && admins <= 1 {
			return ErrCollaborationLastAdmin
		}
		member.Roles = []string{input.Role}
		member.Status = newStatus
		member.Version++
		member.UpdatedAt = s.now().UTC()
		updated, err := s.repository.Database().Collection("project_memberships").UpdateOne(tx, bson.M{"_id": member.ID}, bson.M{"$set": bson.M{"roles": member.Roles, "status": member.Status, "version": member.Version, "updatedAt": member.UpdatedAt}})
		if err != nil {
			return err
		}
		if updated.MatchedCount != 1 {
			return ErrCollaborationVersion
		}
		if err := s.repository.InsertCollaborationEvent(tx, s.mutationEvent(eventID, input.Actor, input.Target, member, before, "project.member.role_changed", input.Reason)); err != nil {
			return err
		}
		result, err = s.collaborator(tx, member)
		return err
	})
	return result, err
}
func (s *ProjectCollaborationService) Remove(ctx context.Context, input models.RemoveMemberInput) error {
	if input.ExpectedVersion < 0 {
		return ErrCollaborationInvalid
	}
	eventID := primitive.NewObjectID()
	return s.repository.WithAuthorizationTransaction(ctx, input.Actor, input.Target, func(tx context.Context, snapshot models.ManagementMembershipSnapshot) error {
		if err := authorizeCollaboration(input.Authority, input.Actor, input.Target, snapshot, true); err != nil {
			return err
		}
		member, err := s.member(tx, input.Target, input.UserID)
		if err != nil {
			return err
		}
		if member.Version != input.ExpectedVersion {
			return ErrCollaborationVersion
		}
		admins, err := s.usableAdmins(tx, input.Target)
		if err != nil {
			return err
		}
		if admins == 0 {
			return ErrCollaborationRecoveryRequired
		}
		if member.Status == models.MembershipStatusActive && hasProjectAdmin(member.Roles) && admins <= 1 {
			recipient, loadErr := repositories.NewManagementMembershipRepositoryWithDatabase(s.repository.Database()).LoadManagementMembership(tx, models.ManagementAuthorizationSubject{UserID: input.UserID, TenantID: input.Target.TenantID}, input.Target)
			if loadErr != nil {
				return loadErr
			}
			if _, eligibleErr := CurrentManagementSessionRoles(recipient, true); eligibleErr == nil {
				return ErrCollaborationLastAdmin
			}
		}
		before := member
		member.Status = models.MembershipStatusDisabled
		member.Version++
		member.UpdatedAt = s.now().UTC()
		if _, err := s.repository.Database().Collection("project_memberships").UpdateOne(tx, bson.M{"_id": member.ID}, bson.M{"$set": bson.M{"status": member.Status, "version": member.Version, "updatedAt": member.UpdatedAt}}); err != nil {
			return err
		}
		return s.repository.InsertCollaborationEvent(tx, s.mutationEvent(eventID, input.Actor, input.Target, member, before, "project.member.removed", input.Reason))
	})
}
func (s *ProjectCollaborationService) List(ctx context.Context, authority models.CollaborationAuthority, actor models.ManagementAuthorizationSubject, target models.ManagementAuthorizationTarget, page models.CollaborationPageRequest) (*models.ProjectCollaboratorPage, error) {
	snapshot, err := repositories.NewManagementMembershipRepositoryWithDatabase(s.repository.Database()).LoadManagementMembership(ctx, actor, target)
	if err != nil {
		return nil, err
	}
	if err := authorizeCollaboration(authority, actor, target, snapshot, false); err != nil {
		return nil, err
	}
	tenant, project, _, err := collaborationIDs(target, actor.UserID)
	if err != nil {
		return nil, err
	}
	if page.Limit == 0 {
		page.Limit = 20
	}
	if page.Limit < 1 || page.Limit > 100 {
		return nil, ErrCollaborationInvalid
	}
	filter := bson.M{"tenantId": tenant, "projectId": project}
	if page.AfterID != "" {
		id, err := primitive.ObjectIDFromHex(page.AfterID)
		if err != nil {
			return nil, ErrCollaborationInvalid
		}
		filter["_id"] = bson.M{"$gt": id}
	}
	cursor, err := s.repository.Database().Collection("project_memberships").Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(page.Limit+1)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	members := []models.ProjectMembership{}
	if err := cursor.All(ctx, &members); err != nil {
		return nil, err
	}
	result := &models.ProjectCollaboratorPage{Items: []models.ProjectCollaborator{}}
	admins, countErr := s.usableAdmins(ctx, target)
	if countErr != nil {
		return nil, countErr
	}
	result.RecoveryRequired = admins == 0
	if len(members) > page.Limit {
		members = members[:page.Limit]
		result.NextCursor = members[len(members)-1].ID.Hex()
	}
	for _, member := range members {
		collaborator, err := s.collaborator(ctx, member)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, *collaborator)
	}
	return result, nil
}
