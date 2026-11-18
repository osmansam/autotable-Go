//go:build integration

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestCollaborationMongoRevocationRetry(t *testing.T) {
	f, actor, target, other := collaborationMemberFixture(t)
	now := time.Now().UTC()
	for _, session := range []models.AuthSession{
		{FamilyID: "target", SessionKey: "target", Identity: "management", UserID: other.Hex(), TenantID: target.TenantID, ProjectID: target.ProjectID, Scope: "project", ExpiresAt: now.Add(time.Hour)},
		{FamilyID: "runtime", SessionKey: "runtime", Identity: "application", UserID: other.Hex(), TenantID: target.TenantID, ProjectID: target.ProjectID, Scope: "project", ExpiresAt: now.Add(time.Hour)},
		{FamilyID: "other", SessionKey: "other", Identity: "management", UserID: other.Hex(), TenantID: target.TenantID, ProjectID: primitive.NewObjectID().Hex(), Scope: "project", ExpiresAt: now.Add(time.Hour)},
	} {
		if _, err := f.DB.Collection("auth_sessions").InsertOne(f.Ctx, session); err != nil {
			t.Fatal(err)
		}
	}
	if err := NewProjectCollaborationService(f.DB).Remove(f.Ctx, models.RemoveMemberInput{Authority: models.CollaborationAuthorityProject, Actor: actor, Target: target, UserID: other.Hex()}); err != nil {
		t.Fatal(err)
	}
	worker := NewManagementCollaborationWorker(f.DB, nil)
	realAudit := worker.audit
	calls := 0
	worker.audit = func(ctx context.Context, log models.AuditLog) error {
		calls++
		if calls == 1 {
			return errors.New("audit sink unavailable")
		}
		return realAudit(ctx, log)
	}
	if found, err := worker.ProcessNext(f.Ctx, "test", now.Add(time.Second)); !found || err == nil {
		t.Fatal("audit failure was not retryable")
	}
	for _, family := range []string{"target", "runtime", "other"} {
		var session models.AuthSession
		if err := f.DB.Collection("auth_sessions").FindOne(f.Ctx, bson.M{"familyId": family}).Decode(&session); err != nil {
			t.Fatal(err)
		}
		if (session.RevokedAt != nil) != (family == "target") {
			t.Fatalf("wrong session revoked: %s", family)
		}
	}
	if found, err := worker.ProcessNext(f.Ctx, "retry", now.Add(time.Minute)); !found || err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	var event models.CollaborationEvent
	if err := f.DB.Collection("management_collaboration_events").FindOne(f.Ctx, bson.M{}).Decode(&event); err != nil || event.Status != "done" {
		t.Fatalf("event not complete: %v %v", event.Status, err)
	}
	if _, err := f.DB.Collection("management_collaboration_events").UpdateOne(f.Ctx, bson.M{"_id": event.ID}, bson.M{"$set": bson.M{"status": "pending", "nextAttemptAt": now}}); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.ProcessNext(f.Ctx, "replay", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	count, err := f.DB.Collection(utils.GetProjectCollectionName(target.TenantID, target.ProjectID, "audit_logs")).CountDocuments(f.Ctx, bson.M{"eventId": event.ID})
	if err != nil || count != 1 {
		t.Fatalf("audit replay duplicated rows: %d %v", count, err)
	}
}

func TestCollaborationMongoRemoveInactiveAdmin(t *testing.T) {
	f, actor, target, other := collaborationMemberFixture(t)
	if _, err := f.DB.Collection("users").UpdateOne(f.Ctx, bson.M{"_id": other}, bson.M{"$set": bson.M{"isActive": false}}); err != nil {
		t.Fatal(err)
	}
	if err := NewProjectCollaborationService(f.DB).Remove(f.Ctx, models.RemoveMemberInput{Authority: models.CollaborationAuthorityProject, Actor: actor, Target: target, UserID: other.Hex()}); err != nil {
		t.Fatalf("inactive admin counted as last usable admin: %v", err)
	}
}

func TestCollaborationMongoProjectLifecycleRevokesBothIdentities(t *testing.T) {
	f, _, target, other := collaborationMemberFixture(t)
	now := time.Now().UTC()
	for _, identity := range []string{"management", "application"} {
		if _, err := f.DB.Collection("auth_sessions").InsertOne(f.Ctx, models.AuthSession{FamilyID: identity, SessionKey: identity, Identity: identity, UserID: other.Hex(), TenantID: target.TenantID, ProjectID: target.ProjectID, Scope: "project", ExpiresAt: now.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repositories.NewAuthSessionRepositoryWithCollection(f.DB.Collection("auth_sessions")).RevokeProjectSessions(f.Ctx, target.TenantID, target.ProjectID, now); err != nil {
		t.Fatal(err)
	}
	count, err := f.DB.Collection("auth_sessions").CountDocuments(f.Ctx, bson.M{"revokedAt": bson.M{"$exists": true}})
	if err != nil || count != 2 {
		t.Fatalf("project lifecycle left sessions active: %d %v", count, err)
	}
}

func TestCollaborationMongoRevocationPreservesFreshGeneration(t *testing.T) {
	f, actor, target, other := collaborationMemberFixture(t)
	now := time.Now()
	repo := repositories.NewAuthSessionRepositoryWithCollection(f.DB.Collection("auth_sessions"))
	create := func(family string) {
		t.Helper()
		if err := repo.Create(f.Ctx, models.AuthSession{Identity: "management", FamilyID: family, UserID: other.Hex(), TenantID: target.TenantID, ProjectID: target.ProjectID, Scope: "project", ExpiresAt: now.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	create("before-change")
	if _, err := NewProjectCollaborationService(f.DB).ChangeRole(f.Ctx, models.RoleChangeInput{Authority: models.CollaborationAuthorityProject, Actor: actor, Target: target, UserID: other.Hex(), Role: models.ProjectRoleViewer}); err != nil {
		t.Fatal(err)
	}
	create("after-change")
	calls := 0
	worker := NewManagementCollaborationWorker(f.DB, func(context.Context, models.AuditLog) error {
		calls++
		if calls == 1 {
			return errors.New("audit unavailable")
		}
		return nil
	})
	if _, err := worker.ProcessNext(f.Ctx, "first", now.Add(time.Second)); err == nil {
		t.Fatal("expected audit retry")
	}
	create("after-retry")
	if _, err := worker.ProcessNext(f.Ctx, "retry", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	for _, family := range []string{"before-change", "after-change", "after-retry"} {
		var session models.AuthSession
		_ = f.DB.Collection("auth_sessions").FindOne(f.Ctx, bson.M{"familyId": family}).Decode(&session)
		if (session.RevokedAt != nil) != (family == "before-change") {
			t.Fatalf("revocation crossed authorization generations: %s", family)
		}
	}
}
