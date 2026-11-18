//go:build integration

package services

import (
	"context"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"testing"
	"time"
)

func TestCollaborationMongoIdentityChangeInvalidatesProof(t *testing.T) {
	for _, change := range []bson.M{{"accountVersion": int64(1)}, {"isActive": false}, {"email": "changed@example.test", "normalizedEmail": "changed@example.test"}} {
		t.Run("account state", func(t *testing.T) {
			service, handle, _, email := invitationIdentityFixture(t)
			newHandle, proof, err := service.Reauthenticate(t.Context(), handle, email, []byte("existing-account-password"))
			if err != nil {
				t.Fatal(err)
			}
			user, _ := primitive.ObjectIDFromHex(proof.UserID)
			if _, err := service.db.Collection("users").UpdateOne(t.Context(), bson.M{"_id": user}, bson.M{"$set": change}); err != nil {
				t.Fatal(err)
			}
			if _, err := service.ProofForHandle(t.Context(), newHandle); err == nil {
				t.Fatal("changed account retained invitation proof")
			}
		})
	}
}
func TestCollaborationMongoInvitationProofPreservesOtherTenant(t *testing.T) {
	service, handle, _, email := invitationIdentityFixture(t)
	var user models.User
	if err := service.db.Collection("users").FindOne(t.Context(), bson.M{"normalizedEmail": email}).Decode(&user); err != nil {
		t.Fatal(err)
	}
	otherTenant := primitive.NewObjectID()
	if _, err := service.db.Collection("tenants").InsertOne(t.Context(), models.Tenant{ID: otherTenant, IsActive: true, OwnerUserID: user.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.db.Collection("tenant_memberships").InsertOne(t.Context(), models.TenantMembership{ID: primitive.NewObjectID(), UserID: user.ID, TenantID: otherTenant, Status: models.MembershipStatusActive, Roles: []string{models.TenantRoleOwner}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.db.Collection("auth_sessions").InsertOne(t.Context(), models.AuthSession{Identity: "management", FamilyID: "other-tenant-session", SessionKey: "other-tenant-session", UserID: user.ID.Hex(), TenantID: otherTenant.Hex(), Scope: "tenant", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Reauthenticate(t.Context(), handle, email, []byte("existing-account-password")); err != nil {
		t.Fatal(err)
	}
	var session models.AuthSession
	if err := service.db.Collection("auth_sessions").FindOne(t.Context(), bson.M{"familyId": "other-tenant-session"}).Decode(&session); err != nil || session.RevokedAt != nil {
		t.Fatal("project proof revoked another tenant session")
	}
	var membership models.TenantMembership
	if err := service.db.Collection("tenant_memberships").FindOne(t.Context(), bson.M{"tenantId": otherTenant, "userId": user.ID}).Decode(&membership); err != nil || len(membership.Roles) != 1 || membership.Roles[0] != models.TenantRoleOwner {
		t.Fatal("project proof changed another tenant role")
	}
}
func TestCollaborationMongoVerificationDeliveryPurpose(t *testing.T) {
	service, handle, _, email := invitationIdentityFixture(t)
	_, proof, err := service.Reauthenticate(t.Context(), handle, email, []byte("existing-account-password"))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RequestVerification(t.Context(), proof); err != nil {
		t.Fatal(err)
	}
	subjects := []string{}
	worker := NewInvitationDeliveryWorker(service.db, service.config, service.keys, invitationTestTransport(func(_ context.Context, message models.InvitationMessage) error {
		subjects = append(subjects, message.Subject)
		if message.To != email {
			t.Fatal("verification sent to another account")
		}
		return nil
	}))
	for i := 0; i < 2; i++ {
		if found, err := worker.ProcessNext(t.Context(), "verification-test", time.Now().Add(time.Second)); !found || err != nil {
			t.Fatalf("verification delivery missing: %v", err)
		}
	}
	if len(subjects) != 2 || subjects[1] != "Verify your AutoTable email" {
		t.Fatalf("verification email not delivered: %v", subjects)
	}
}
