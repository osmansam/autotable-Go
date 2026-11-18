//go:build integration

package repositories

import (
	"context"
	"github.com/osmansam/autotableGo/internal/testmongo"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"os"
	"sync"
	"testing"
)

func seedCollaborationGuard(t *testing.T) (*testmongo.Fixture, models.ManagementAuthorizationSubject, models.ManagementAuthorizationTarget) {
	t.Helper()
	f := testmongo.New(t)
	user, tenant, project := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	for name, doc := range map[string]interface{}{"users": models.User{ID: user, IsActive: true}, "tenants": models.Tenant{ID: tenant, IsActive: true}, "projects": models.Project{ID: project, TenantID: tenant, IsActive: true}, "tenant_memberships": models.TenantMembership{ID: primitive.NewObjectID(), TenantID: tenant, UserID: user, Status: models.MembershipStatusActive, Roles: []string{models.TenantRoleOwner}}, "project_memberships": models.ProjectMembership{ID: primitive.NewObjectID(), TenantID: tenant, ProjectID: project, UserID: user, Status: models.MembershipStatusActive, Roles: []string{models.ProjectRoleAdmin}}} {
		if _, err := f.DB.Collection(name).InsertOne(f.Ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	return f, models.ManagementAuthorizationSubject{UserID: user.Hex(), TenantID: tenant.Hex()}, models.ManagementAuthorizationTarget{TenantID: tenant.Hex(), ProjectID: project.Hex()}
}
func TestCollaborationMongoConcurrentLegacyDeactivate(t *testing.T) {
	f, actor, target := seedCollaborationGuard(t)
	project, _ := primitive.ObjectIDFromHex(target.ProjectID)
	if _, err := f.DB.Collection("projects").UpdateOne(f.Ctx, bson.M{"_id": project}, bson.M{"$set": bson.M{"isActive": false}}); err != nil {
		t.Fatal(err)
	}
	called := false
	err := NewManagementCollaborationRepository(f.DB).WithAuthorizationTransaction(f.Ctx, actor, target, func(context.Context, models.ManagementMembershipSnapshot) error { called = true; return nil })
	if err == nil || called {
		t.Fatal("legacy deactivation allowed a new grant")
	}
}
func TestCollaborationMongoRollbackBlocksGrant(t *testing.T) {
	f, actor, target := seedCollaborationGuard(t)
	project, _ := primitive.ObjectIDFromHex(target.ProjectID)
	if _, err := f.DB.Collection("projects").DeleteOne(f.Ctx, bson.M{"_id": project}); err != nil {
		t.Fatal(err)
	}
	if err := NewManagementCollaborationRepository(f.DB).WithAuthorizationTransaction(f.Ctx, actor, target, func(context.Context, models.ManagementMembershipSnapshot) error { return nil }); err == nil {
		t.Fatal("missing rolled-back project allowed grant")
	}
}

func TestCollaborationMongoReadinessFenceBlocksMigration(t *testing.T) {
	f, actor, target := seedCollaborationGuard(t)
	if _, err := f.DB.Collection("management_collaboration_migrations").InsertOne(f.Ctx, bson.M{"_id": "schema", "ready": false, "version": 1}); err != nil {
		t.Fatal(err)
	}
	called := false
	err := NewManagementCollaborationRepository(f.DB).WithAuthorizationTransaction(WithCollaborationReadiness(f.Ctx), actor, target, func(context.Context, models.ManagementMembershipSnapshot) error { called = true; return nil })
	if err == nil || called {
		t.Fatal("request passed readiness after migration paused grants")
	}
}

func TestCollaborationMongoGuardRetriesLiveAuthority(t *testing.T) {
	f, actor, target := seedCollaborationGuard(t)
	paused, resume := make(chan struct{}), make(chan struct{})
	var once, release sync.Once
	unpause := func() { release.Do(func() { close(resume) }) }
	defer unpause()
	monitor := &event.CommandMonitor{Started: func(ctx context.Context, e *event.CommandStartedEvent) {
		if e.CommandName == "update" && e.Command.Lookup("update").StringValue() == "projects" {
			once.Do(func() {
				close(paused)
				select {
				case <-resume:
				case <-ctx.Done():
				}
			})
		}
	}}
	client, err := mongo.Connect(f.Ctx, options.Client().ApplyURI(os.Getenv("AUTOTABLE_COLLABORATION_MONGO_URI")).SetMonitor(monitor))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- NewManagementCollaborationRepository(client.Database(f.DB.Name())).WithAuthorizationTransaction(f.Ctx, actor, target, func(context.Context, models.ManagementMembershipSnapshot) error { return nil })
	}()
	select {
	case <-paused:
	case err := <-done:
		t.Fatalf("grant stopped before race: %v", err)
	case <-f.Ctx.Done():
		t.Fatal("grant did not reach guard")
	}
	project, _ := primitive.ObjectIDFromHex(target.ProjectID)
	if _, err := f.DB.Collection("projects").UpdateOne(f.Ctx, bson.M{"_id": project}, bson.M{"$set": bson.M{"isActive": false}}); err != nil {
		t.Fatal(err)
	}
	unpause()
	if err := <-done; err == nil {
		t.Fatal("grant survived concurrent legacy deactivation")
	}
}
