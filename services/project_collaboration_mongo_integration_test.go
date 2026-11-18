//go:build integration

package services

import (
	"github.com/osmansam/autotableGo/internal/testmongo"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"sync"
	"testing"
)

func collaborationMemberFixture(t *testing.T) (*testmongo.Fixture, models.ManagementAuthorizationSubject, models.ManagementAuthorizationTarget, primitive.ObjectID) {
	t.Helper()
	f := testmongo.New(t)
	user, other, tenant, project := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	for _, u := range []primitive.ObjectID{user, other} {
		for collection, doc := range map[string]interface{}{"users": models.User{ID: u, Email: u.Hex() + "@example.com", NormalizedEmail: u.Hex() + "@example.com", IsActive: true}, "tenant_memberships": models.TenantMembership{ID: primitive.NewObjectID(), UserID: u, TenantID: tenant, Roles: []string{models.TenantRoleMember}, Status: models.MembershipStatusActive}, "project_memberships": models.ProjectMembership{ID: primitive.NewObjectID(), UserID: u, TenantID: tenant, ProjectID: project, Roles: []string{models.ProjectRoleAdmin}, Status: models.MembershipStatusActive}} {
			if _, err := f.DB.Collection(collection).InsertOne(f.Ctx, doc); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := f.DB.Collection("tenants").InsertOne(f.Ctx, models.Tenant{ID: tenant, IsActive: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.DB.Collection("projects").InsertOne(f.Ctx, models.Project{ID: project, TenantID: tenant, IsActive: true}); err != nil {
		t.Fatal(err)
	}
	return f, models.ManagementAuthorizationSubject{UserID: user.Hex(), TenantID: tenant.Hex()}, models.ManagementAuthorizationTarget{TenantID: tenant.Hex(), ProjectID: project.Hex()}, other
}
func TestCollaborationMongoLastUsableAdmin(t *testing.T) {
	f, actor, target, other := collaborationMemberFixture(t)
	service := NewProjectCollaborationService(f.DB)
	if err := service.Remove(f.Ctx, models.RemoveMemberInput{Authority: models.CollaborationAuthorityProject, Actor: actor, Target: target, UserID: other.Hex()}); err != nil {
		t.Fatal(err)
	}
	if err := service.Remove(f.Ctx, models.RemoveMemberInput{Authority: models.CollaborationAuthorityProject, Actor: actor, Target: target, UserID: actor.UserID}); err == nil {
		t.Fatal("last admin removed")
	}
	count, _ := f.DB.Collection("management_collaboration_events").CountDocuments(f.Ctx, bson.M{})
	if count != 1 {
		t.Fatalf("removal audit outbox count=%d", count)
	}
}
func TestCollaborationMongoConcurrentAdminRemoval(t *testing.T) {
	f, actor, target, other := collaborationMemberFixture(t)
	service := NewProjectCollaborationService(f.DB)
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for _, pair := range [][2]string{{actor.UserID, other.Hex()}, {other.Hex(), actor.UserID}} {
		group.Add(1)
		go func(pair [2]string) {
			defer group.Done()
			<-start
			results <- service.Remove(f.Ctx, models.RemoveMemberInput{Authority: models.CollaborationAuthorityProject, Actor: models.ManagementAuthorizationSubject{UserID: pair[0], TenantID: actor.TenantID}, Target: target, UserID: pair[1]})
		}(pair)
	}
	close(start)
	group.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent removals committed=%d", successes)
	}
}
func TestCollaborationMongoMutationAuditAtomic(t *testing.T) {
	f, actor, target, other := collaborationMemberFixture(t)
	if err := f.DB.CreateCollection(f.Ctx, "management_collaboration_events", options.CreateCollection().SetValidator(bson.M{"requiredSentinel": true})); err != nil {
		t.Fatal(err)
	}
	_, err := NewProjectCollaborationService(f.DB).ChangeRole(f.Ctx, models.RoleChangeInput{Authority: models.CollaborationAuthorityProject, Actor: actor, Target: target, UserID: other.Hex(), Role: models.ProjectRoleViewer})
	if err == nil {
		t.Fatal("mutation survived failed durable audit")
	}
	var member models.ProjectMembership
	if err := f.DB.Collection("project_memberships").FindOne(f.Ctx, bson.M{"userId": other}).Decode(&member); err != nil {
		t.Fatal(err)
	}
	if member.Version != 0 || member.Roles[0] != models.ProjectRoleAdmin {
		t.Fatal("failed audit left changed permissions")
	}
}
