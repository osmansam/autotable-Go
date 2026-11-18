//go:build integration

package services

import (
	"errors"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"testing"
	"time"
)

func TestCollaborationProjectReactivationRequiresAdministrator(t *testing.T) {
	for _, eligible := range []bool{false, true} {
		t.Run(map[bool]string{false: "adminless", true: "usable_admin"}[eligible], func(t *testing.T) {
			f, actor, target, other := collaborationMemberFixture(t)
			actorID, _ := primitive.ObjectIDFromHex(actor.UserID)
			project, _ := primitive.ObjectIDFromHex(target.ProjectID)
			_, _ = f.DB.Collection("tenant_memberships").UpdateOne(f.Ctx, bson.M{"userId": actorID}, bson.M{"$set": bson.M{"roles": bson.A{models.TenantRoleOwner}}})
			_, _ = f.DB.Collection("project_memberships").UpdateMany(f.Ctx, bson.M{}, bson.M{"$set": bson.M{"status": models.MembershipStatusDisabled}})
			if eligible {
				_, _ = f.DB.Collection("project_memberships").UpdateOne(f.Ctx, bson.M{"userId": other}, bson.M{"$set": bson.M{"status": models.MembershipStatusActive}})
			}
			_, _ = f.DB.Collection("projects").UpdateOne(f.Ctx, bson.M{"_id": project}, bson.M{"$set": bson.M{"isActive": false, "name": "Before"}})
			_, err := NewProjectCollaborationService(f.DB).ReactivateProject(f.Ctx, actor, target, bson.M{"name": "After", "updatedAt": time.Now()})
			if eligible && err != nil {
				t.Fatal(err)
			}
			if !eligible && !errors.Is(err, ErrCollaborationRecoveryRequired) {
				t.Fatalf("adminless project reactivated: %v", err)
			}
			var stored models.Project
			_ = f.DB.Collection("projects").FindOne(f.Ctx, bson.M{"_id": project}).Decode(&stored)
			if stored.IsActive != eligible || (!eligible && stored.Name != "Before") {
				t.Fatal("reactivation escaped administrator invariant or rollback")
			}
		})
	}
}

func TestCollaborationProjectReactivationRejectsProjectAuthority(t *testing.T) {
	f, actor, target, _ := collaborationMemberFixture(t)
	project, _ := primitive.ObjectIDFromHex(target.ProjectID)
	_, _ = f.DB.Collection("projects").UpdateOne(f.Ctx, bson.M{"_id": project}, bson.M{"$set": bson.M{"isActive": false}})
	if _, err := NewProjectCollaborationService(f.DB).ReactivateProject(f.Ctx, actor, target, bson.M{}); !errors.Is(err, ErrManagementForbidden) {
		t.Fatalf("project admin received tenant lifecycle authority: %v", err)
	}
	var stored models.Project
	_ = f.DB.Collection("projects").FindOne(f.Ctx, bson.M{"_id": project}).Decode(&stored)
	if stored.IsActive {
		t.Fatal("forbidden reactivation committed")
	}
}
