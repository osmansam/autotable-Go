package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type ManagementMembershipRepository struct {
	users              *mongo.Collection
	tenants            *mongo.Collection
	projects           *mongo.Collection
	tenantMemberships  *mongo.Collection
	projectMemberships *mongo.Collection
}

func NewManagementMembershipRepository() *ManagementMembershipRepository {
	return &ManagementMembershipRepository{
		users: configs.GetCollection("users"), tenants: configs.GetCollection("tenants"), projects: configs.GetCollection("projects"),
		tenantMemberships: configs.GetCollection("tenant_memberships"), projectMemberships: configs.GetCollection("project_memberships"),
	}
}

type managementMembershipIDs struct {
	userID, tenantID primitive.ObjectID
	projectID        *primitive.ObjectID
}

type managementMembershipFilters struct {
	user, tenant, tenantMembership, project, projectMembership bson.M
}

func buildManagementMembershipFilters(ids managementMembershipIDs) managementMembershipFilters {
	filters := managementMembershipFilters{
		user:             bson.M{"_id": ids.userID},
		tenant:           bson.M{"_id": ids.tenantID},
		tenantMembership: bson.M{"userId": ids.userID, "tenantId": ids.tenantID},
	}
	if ids.projectID != nil {
		filters.project = bson.M{"_id": *ids.projectID, "tenantId": ids.tenantID}
		filters.projectMembership = bson.M{"userId": ids.userID, "tenantId": ids.tenantID, "projectId": *ids.projectID}
	}
	return filters
}

func parseManagementMembershipIDs(subject models.ManagementAuthorizationSubject, target models.ManagementAuthorizationTarget) (managementMembershipIDs, error) {
	var ids managementMembershipIDs
	if subject.TenantID == "" || subject.TenantID != target.TenantID {
		return ids, errors.New("management tenant does not match target tenant")
	}
	var err error
	ids.userID, err = primitive.ObjectIDFromHex(subject.UserID)
	if err != nil {
		return ids, fmt.Errorf("invalid management user ID: %w", err)
	}
	ids.tenantID, err = primitive.ObjectIDFromHex(target.TenantID)
	if err != nil {
		return ids, fmt.Errorf("invalid management tenant ID: %w", err)
	}
	if target.ProjectID != "" {
		projectID, parseErr := primitive.ObjectIDFromHex(target.ProjectID)
		if parseErr != nil {
			return ids, fmt.Errorf("invalid management project ID: %w", parseErr)
		}
		ids.projectID = &projectID
	}
	return ids, nil
}

func (r *ManagementMembershipRepository) LoadManagementMembership(ctx context.Context, subject models.ManagementAuthorizationSubject, target models.ManagementAuthorizationTarget) (models.ManagementMembershipSnapshot, error) {
	ids, err := parseManagementMembershipIDs(subject, target)
	if err != nil {
		return models.ManagementMembershipSnapshot{}, err
	}
	filters := buildManagementMembershipFilters(ids)
	var snapshot models.ManagementMembershipSnapshot
	var user models.User
	if err := decodeOptional(r.users.FindOne(ctx, filters.user), &user); err != nil {
		return snapshot, err
	}
	snapshot.UserActive = !user.ID.IsZero() && user.IsActive
	var tenant models.Tenant
	if err := decodeOptional(r.tenants.FindOne(ctx, filters.tenant), &tenant); err != nil {
		return snapshot, err
	}
	snapshot.TenantActive = !tenant.ID.IsZero() && tenant.IsActive
	var tenantMembership models.TenantMembership
	if err := decodeOptional(r.tenantMemberships.FindOne(ctx, filters.tenantMembership), &tenantMembership); err != nil {
		return snapshot, err
	}
	if !tenantMembership.ID.IsZero() {
		snapshot.TenantMembership = &tenantMembership
	}
	if ids.projectID == nil {
		return snapshot, nil
	}
	var project models.Project
	if err := decodeOptional(r.projects.FindOne(ctx, filters.project), &project); err != nil {
		return snapshot, err
	}
	snapshot.ProjectActive = !project.ID.IsZero() && project.IsActive
	var projectMembership models.ProjectMembership
	if err := decodeOptional(r.projectMemberships.FindOne(ctx, filters.projectMembership), &projectMembership); err != nil {
		return snapshot, err
	}
	if !projectMembership.ID.IsZero() {
		snapshot.ProjectMembership = &projectMembership
	}
	return snapshot, nil
}

type singleResultDecoder interface {
	Decode(any) error
}

func decodeOptional(result singleResultDecoder, target any) error {
	err := result.Decode(target)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil
	}
	return err
}
