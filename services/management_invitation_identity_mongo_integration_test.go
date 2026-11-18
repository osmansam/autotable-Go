//go:build integration

package services

import (
	"encoding/json"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"testing"
	"time"
)

func invitationIdentityFixture(t *testing.T) (*InvitationIdentityService, string, func(), string) {
	t.Helper()
	f, issuer, input, keys, cfg := invitationFixture(t)
	summary, err := issuer.Create(f.Ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	password := "existing-account-password"
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	user := primitive.NewObjectID()
	if _, err := f.DB.Collection("users").InsertOne(f.Ctx, bson.M{"_id": user, "email": input.Email, "normalizedEmail": input.Email, "password": string(hash), "isActive": true}); err != nil {
		t.Fatal(err)
	}
	handle, handleHash, err := utils.GenerateInvitationToken()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	continuation := models.InvitationContinuation{ID: primitive.NewObjectID(), InvitationID: summary.ID, InvitationVersion: 1, HandleHash: handleHash, Purpose: "project_invitation", Status: "pending", ExpiresAt: now.Add(30 * time.Minute), CreatedAt: now, UpdatedAt: now}
	if _, err := f.DB.Collection("management_invitation_continuations").InsertOne(f.Ctx, continuation); err != nil {
		t.Fatal(err)
	}
	service := NewInvitationIdentityService(f.DB, cfg, keys)
	assertUnchanged := func() {
		var account struct {
			Password string `bson:"password"`
		}
		if err := f.DB.Collection("users").FindOne(f.Ctx, bson.M{"_id": user}).Decode(&account); err != nil || account.Password != string(hash) {
			t.Fatal("invitation modified existing password")
		}
		count, _ := f.DB.Collection("auth_sessions").CountDocuments(f.Ctx, bson.M{})
		if count != 0 {
			t.Fatal("proof issued an account session")
		}
		count, _ = f.DB.Collection("tenant_memberships").CountDocuments(f.Ctx, bson.M{"userId": user})
		if count != 0 {
			t.Fatal("proof granted tenant membership before acceptance")
		}
	}
	return service, handle, assertUnchanged, input.Email
}
func TestCollaborationInvitationCannotRecoverAccount(t *testing.T) {
	service, handle, assertUnchanged, email := invitationIdentityFixture(t)
	if _, _, err := service.Reauthenticate(t.Context(), handle, email, []byte("invitation-token-is-not-a-password")); err == nil {
		t.Fatal("invitation replaced account ownership proof")
	}
	assertUnchanged()
}
func TestCollaborationUnverifiedAccountSeparateChallenge(t *testing.T) {
	service, handle, assertUnchanged, email := invitationIdentityFixture(t)
	newHandle, proof, err := service.Reauthenticate(t.Context(), handle, email, []byte("existing-account-password"))
	if err != nil || newHandle == handle || proof.EmailVerified {
		t.Fatalf("invalid reauthentication: %v", err)
	}
	if err := service.RequestVerification(t.Context(), proof); err != nil {
		t.Fatal(err)
	}
	var job models.InvitationDeliveryJob
	if err := service.db.Collection("management_invitation_deliveries").FindOne(t.Context(), bson.M{"purpose": "email_verification"}).Decode(&job); err != nil {
		t.Fatal(err)
	}
	raw, err := service.keys.Open(job.EncryptedPayload, invitationDeliveryAAD(job))
	if err != nil {
		t.Fatal(err)
	}
	var message models.InvitationMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(message.TextBody, "Verification code:\n")
	if len(parts) != 2 {
		t.Fatal("independent verification challenge missing")
	}
	challenge := strings.Split(parts[1], "\n")[0]
	if err := service.VerifyEmail(t.Context(), proof, "invalid-invitation-token"); err == nil {
		t.Fatal("wrong challenge verified email")
	}
	if err := service.VerifyEmail(t.Context(), proof, challenge); err != nil {
		t.Fatal(err)
	}
	current, err := service.ProofForHandle(t.Context(), newHandle)
	if err != nil || !current.EmailVerified {
		t.Fatalf("mailbox not verified: %v", err)
	}
	if err := service.VerifyEmail(t.Context(), proof, challenge); err == nil {
		t.Fatal("verification challenge reused")
	}
	assertUnchanged()
}
func TestCollaborationIdentityProofExpiry(t *testing.T) {
	service, handle, assertUnchanged, email := invitationIdentityFixture(t)
	newHandle, _, err := service.Reauthenticate(t.Context(), handle, email, []byte("existing-account-password"))
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Now().Add(6 * time.Minute) }
	if _, err := service.ProofForHandle(t.Context(), newHandle); err == nil {
		t.Fatal("stale account proof accepted")
	}
	assertUnchanged()
}
