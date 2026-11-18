package repositories

import (
	"context"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type ManagementProjectAccessRepository struct{ db *mongo.Database }

func NewManagementProjectAccessRepository(db *mongo.Database) *ManagementProjectAccessRepository {
	return &ManagementProjectAccessRepository{db: db}
}
func tenantCanReadAllProjects(roles []string) bool {
	for _, r := range roles {
		switch r {
		case models.TenantRoleOwner, models.TenantRoleAdmin, models.TenantRoleBilling, models.TenantRoleAuditor:
			return true
		}
	}
	return false
}
func validProjectMembership(m *models.ProjectMembership) bool {
	if m == nil || m.Status != models.MembershipStatusActive {
		return false
	}
	for _, r := range m.Roles {
		switch r {
		case models.ProjectRoleAdmin, models.ProjectRoleDeveloper, models.ProjectRoleEditor, models.ProjectRoleViewer, models.ProjectRoleSupport:
			return true
		}
	}
	return false
}

func tenantIsBasicMember(roles []string) bool {
	for _, role := range roles {
		if role == models.TenantRoleMember {
			return true
		}
	}
	return false
}
func (r *ManagementProjectAccessRepository) ListVisibleProjects(ctx context.Context, subject models.ManagementAuthorizationSubject) ([]models.Project, error) {
	snapshot, err := NewManagementMembershipRepositoryWithDatabase(r.db).LoadManagementMembership(ctx, subject, models.ManagementAuthorizationTarget{TenantID: subject.TenantID})
	if err != nil {
		return nil, err
	}
	if !snapshot.UserActive || !snapshot.TenantActive || snapshot.TenantMembership == nil || snapshot.TenantMembership.Status != models.MembershipStatusActive {
		return nil, mongo.ErrNoDocuments
	}
	tenant, _ := primitive.ObjectIDFromHex(subject.TenantID)
	user, _ := primitive.ObjectIDFromHex(subject.UserID)
	pipeline := mongo.Pipeline{{{Key: "$match", Value: bson.M{"tenantId": tenant, "initializing": bson.M{"$ne": true}}}}}
	if !tenantCanReadAllProjects(snapshot.TenantMembership.Roles) {
		member := false
		for _, role := range snapshot.TenantMembership.Roles {
			if role == models.TenantRoleMember {
				member = true
			}
		}
		if !member {
			return nil, mongo.ErrNoDocuments
		}
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.M{"isActive": true, "initializing": bson.M{"$ne": true}}}}, bson.D{{Key: "$lookup", Value: bson.M{"from": "project_memberships", "let": bson.M{"project": "$_id"}, "pipeline": bson.A{bson.M{"$match": bson.M{"tenantId": tenant, "userId": user, "status": models.MembershipStatusActive, "roles": bson.M{"$in": bson.A{models.ProjectRoleAdmin, models.ProjectRoleDeveloper, models.ProjectRoleEditor, models.ProjectRoleViewer, models.ProjectRoleSupport}}, "$expr": bson.M{"$eq": bson.A{"$projectId", "$$project"}}}}}, "as": "_collaborationMembership"}}}, bson.D{{Key: "$match", Value: bson.M{"_collaborationMembership.0": bson.M{"$exists": true}}}}, bson.D{{Key: "$project", Value: bson.M{"_collaborationMembership": 0}}})
	}
	cursor, err := r.db.Collection("projects").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	result := []models.Project{}
	err = cursor.All(ctx, &result)
	return result, err
}
func (r *ManagementProjectAccessRepository) GetVisibleProject(ctx context.Context, subject models.ManagementAuthorizationSubject, projectID string) (*models.Project, error) {
	snapshot, err := NewManagementMembershipRepositoryWithDatabase(r.db).LoadManagementMembership(ctx, subject, models.ManagementAuthorizationTarget{TenantID: subject.TenantID, ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	if !snapshot.UserActive || !snapshot.TenantActive || snapshot.TenantMembership == nil || snapshot.TenantMembership.Status != models.MembershipStatusActive {
		return nil, mongo.ErrNoDocuments
	}
	if !tenantCanReadAllProjects(snapshot.TenantMembership.Roles) && (!tenantIsBasicMember(snapshot.TenantMembership.Roles) || !snapshot.ProjectActive || !validProjectMembership(snapshot.ProjectMembership)) {
		return nil, mongo.ErrNoDocuments
	}
	tenant, _ := primitive.ObjectIDFromHex(subject.TenantID)
	project, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return nil, mongo.ErrNoDocuments
	}
	var result models.Project
	err = r.db.Collection("projects").FindOne(ctx, bson.M{"_id": project, "tenantId": tenant, "initializing": bson.M{"$ne": true}}).Decode(&result)
	return &result, err
}
