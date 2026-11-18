//go:build integration

package services

import (
	"encoding/json"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"testing"
	"time"
)

func TestCollaborationMongoAdminRecovery(t *testing.T) {
	f, actor, target, other := collaborationMemberFixture(t)
	actorID, _ := primitive.ObjectIDFromHex(actor.UserID)
	_, _ = f.DB.Collection("tenant_memberships").UpdateOne(f.Ctx, bson.M{"userId": actorID}, bson.M{"$set": bson.M{"roles": bson.A{models.TenantRoleOwner}}})
	_, _ = f.DB.Collection("project_memberships").UpdateMany(f.Ctx, bson.M{}, bson.M{"$set": bson.M{"status": models.MembershipStatusDisabled}})
	service := NewProjectCollaborationService(f.DB)
	request := models.RecoverAdminInput{Actor: actor, Target: target, UserID: other.Hex(), Reason: "Restore administration after team changes"}
	if _, err := service.RecoverAdmin(f.Ctx, request); err == nil {
		t.Fatal("unverified account became recovery admin")
	}
	_, _ = f.DB.Collection("users").UpdateOne(f.Ctx, bson.M{"_id": other}, bson.M{"$set": bson.M{"emailVerifiedAt": time.Now()}})
	result, err := service.RecoverAdmin(f.Ctx, request)
	if err != nil || !hasProjectAdmin(result.Roles) || result.Status != models.MembershipStatusActive {
		t.Fatalf("verified recovery failed: %v", err)
	}
	snapshot, err := repositories.NewManagementMembershipRepositoryWithDatabase(f.DB).LoadManagementMembership(f.Ctx, actor, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := AuthorizeManagementSnapshot(actor, target, PermissionProjectConfigRead, snapshot); err == nil {
		t.Fatal("recovery implicitly granted caller project configuration")
	}
	request.UserID = actor.UserID
	if _, err := service.RecoverAdmin(f.Ctx, request); err == nil {
		t.Fatal("recovery bypassed ordinary role management with a usable admin present")
	}
}

func TestCollaborationRecoveryCandidateCarriesMembershipVersion(t *testing.T) {
	f, actor, target, other := collaborationMemberFixture(t)
	actorID, _ := primitive.ObjectIDFromHex(actor.UserID)
	_, _ = f.DB.Collection("tenant_memberships").UpdateOne(f.Ctx, bson.M{"userId": actorID}, bson.M{"$set": bson.M{"roles": bson.A{models.TenantRoleOwner}}})
	_, _ = f.DB.Collection("users").UpdateOne(f.Ctx, bson.M{"_id": other}, bson.M{"$set": bson.M{"emailVerifiedAt": time.Now()}})
	_, _ = f.DB.Collection("project_memberships").UpdateOne(f.Ctx, bson.M{"userId": other}, bson.M{"$set": bson.M{"version": int64(7), "status": models.MembershipStatusDisabled}})
	candidates, err := NewProjectCollaborationService(f.DB).RecoveryCandidates(f.Ctx, actor, target, other.Hex()[:5])
	if err != nil || len(candidates) != 1 {
		t.Fatalf("candidate lookup %v %v", candidates, err)
	}
	encoded, _ := json.Marshal(candidates[0])
	var candidate map[string]interface{}
	_ = json.Unmarshal(encoded, &candidate)
	if candidate["membershipVersion"] != float64(7) {
		t.Fatal("recovery candidate omitted version for membership outside current page")
	}
}
