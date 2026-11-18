//go:build integration

package services

import (
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func rawInvitationToken(t *testing.T, service *InvitationService, id primitive.ObjectID) string {
	t.Helper()
	var job models.InvitationDeliveryJob
	if err := service.db.Collection("management_invitation_deliveries").FindOne(t.Context(), bson.M{"invitationId": id, "purpose": "invitation", "tokenVersion": int64(1)}).Decode(&job); err != nil {
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
	for _, line := range strings.Split(message.TextBody, "\n") {
		if strings.HasPrefix(line, "https://") {
			parsed, err := url.Parse(line)
			if err == nil {
				return parsed.Fragment
			}
		}
	}
	t.Fatal("captured invitation token missing")
	return ""
}
func TestCollaborationContinuationReload(t *testing.T) {
	f, issuer, input, _, _ := invitationFixture(t)
	summary, err := issuer.Create(f.Ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	raw := rawInvitationToken(t, issuer, summary.ID)
	tenant, _ := primitive.ObjectIDFromHex(input.Target.TenantID)
	project, _ := primitive.ObjectIDFromHex(input.Target.ProjectID)
	_, _ = f.DB.Collection("tenants").UpdateOne(f.Ctx, bson.M{"_id": tenant}, bson.M{"$set": bson.M{"name": "Acme", "slug": "acme"}})
	_, _ = f.DB.Collection("projects").UpdateOne(f.Ctx, bson.M{"_id": project}, bson.M{"$set": bson.M{"name": "Inventory", "slug": "inventory", "tenantSlug": "acme"}})
	continuation := NewInvitationContinuationService(f.DB)
	handle, preview, err := continuation.Exchange(f.Ctx, raw)
	if err != nil || preview.ProjectName != "Inventory" || preview.Role != models.ProjectRoleViewer {
		t.Fatalf("preview invalid: %#v %v", preview, err)
	}
	if handle == raw || preview.ContinuationExpiresAt.After(time.Now().Add(30*time.Minute)) {
		t.Fatal("continuation leaked invite secret or outlived limit")
	}
	reloaded, err := continuation.Resume(f.Ctx, handle)
	if err != nil || reloaded.ProjectName != preview.ProjectName {
		t.Fatal("reload lost server continuation")
	}
	var stored models.InvitationContinuation
	if err := f.DB.Collection("management_invitation_continuations").FindOne(f.Ctx, bson.M{}).Decode(&stored); err != nil || stored.HandleHash == handle || stored.HandleHash != utils.HashInvitationToken(handle) {
		t.Fatal("raw browser handle persisted")
	}
	wrong, _, _ := utils.GenerateInvitationToken()
	if _, err := continuation.Resume(f.Ctx, wrong); err == nil {
		t.Fatal("wrong browser continued invitation")
	}
	continuation.now = func() time.Time { return time.Now().Add(31 * time.Minute) }
	if _, err := continuation.Resume(f.Ctx, handle); err == nil {
		t.Fatal("expired continuation accepted")
	}
}
func TestCollaborationMongoSingleAcceptance(t *testing.T) {
	f, issuer, input, _, _ := invitationFixture(t)
	summary, err := issuer.Create(f.Ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	handle, _, err := NewInvitationContinuationService(f.DB).Exchange(f.Ctx, rawInvitationToken(t, issuer, summary.ID))
	if err != nil {
		t.Fatal(err)
	}
	service := NewInvitationAcceptanceService(f.DB)
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, err := service.Accept(f.Ctx, models.AcceptProjectInvitationInput{ContinuationHandle: handle, CreateAccount: true, Name: "Collaborator", Password: []byte("new-account-password")})
			results <- err
		}()
	}
	close(start)
	group.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted=%d", accepted)
	}
	users, _ := f.DB.Collection("users").CountDocuments(f.Ctx, bson.M{"normalizedEmail": input.Email})
	members, _ := f.DB.Collection("project_memberships").CountDocuments(f.Ctx, bson.M{})
	tenants, _ := f.DB.Collection("tenants").CountDocuments(f.Ctx, bson.M{})
	sessions, _ := f.DB.Collection("auth_sessions").CountDocuments(f.Ctx, bson.M{})
	if users != 1 || members != 3 || tenants != 1 || sessions != 0 {
		t.Fatalf("duplicate grant, extra tenant or premature session: users=%d members=%d tenants=%d sessions=%d", users, members, tenants, sessions)
	}
	if _, err := service.Accept(f.Ctx, models.AcceptProjectInvitationInput{ContinuationHandle: handle, CreateAccount: true, Name: "Replay", Password: []byte("new-account-password")}); err == nil {
		t.Fatal("consumed continuation replayed")
	}
}
func TestCollaborationMongoAcceptAgainstInviterDemotion(t *testing.T) {
	f, issuer, input, _, _ := invitationFixture(t)
	summary, err := issuer.Create(f.Ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	handle, _, err := NewInvitationContinuationService(f.DB).Exchange(f.Ctx, rawInvitationToken(t, issuer, summary.ID))
	if err != nil {
		t.Fatal(err)
	}
	actor, _ := primitive.ObjectIDFromHex(input.Actor.UserID)
	_, err = f.DB.Collection("project_memberships").UpdateOne(f.Ctx, bson.M{"userId": actor}, bson.M{"$set": bson.M{"roles": bson.A{models.ProjectRoleViewer}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewInvitationAcceptanceService(f.DB).Accept(f.Ctx, models.AcceptProjectInvitationInput{ContinuationHandle: handle, CreateAccount: true, Name: "Denied", Password: []byte("new-account-password")}); err == nil {
		t.Fatal("old inviter authority granted membership")
	}
	count, _ := f.DB.Collection("users").CountDocuments(f.Ctx, bson.M{"normalizedEmail": input.Email})
	if count != 0 {
		t.Fatal("denied acceptance created an account")
	}
}
func TestCollaborationMongoAccountCreationConflict(t *testing.T) {
	f, issuer, input, _, _ := invitationFixture(t)
	summary, err := issuer.Create(f.Ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	handle, _, err := NewInvitationContinuationService(f.DB).Exchange(f.Ctx, rawInvitationToken(t, issuer, summary.ID))
	if err != nil {
		t.Fatal(err)
	}
	user := primitive.NewObjectID()
	if _, err := f.DB.Collection("users").InsertOne(f.Ctx, bson.M{"_id": user, "email": input.Email, "normalizedEmail": input.Email, "password": "existing-hash", "isActive": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewInvitationAcceptanceService(f.DB).Accept(f.Ctx, models.AcceptProjectInvitationInput{ContinuationHandle: handle, CreateAccount: true, Name: "Replace", Password: []byte("replace-account-password")}); err == nil {
		t.Fatal("existing account overwritten")
	}
	var stored struct {
		Password string `bson:"password"`
	}
	_ = f.DB.Collection("users").FindOne(f.Ctx, bson.M{"_id": user}).Decode(&stored)
	if stored.Password != "existing-hash" {
		t.Fatal("invite became account recovery")
	}
}

func TestCollaborationExistingAcceptancePreservesScopes(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(fmt.Sprint(disabled), func(t *testing.T) {
			f, issuer, input, keys, cfg := invitationFixture(t)
			invite, err := issuer.Create(f.Ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			handle, _, err := NewInvitationContinuationService(f.DB).Exchange(f.Ctx, rawInvitationToken(t, issuer, invite.ID))
			if err != nil {
				t.Fatal(err)
			}
			user, otherTenant := primitive.NewObjectID(), primitive.NewObjectID()
			now := time.Now()
			password, _ := bcrypt.GenerateFromPassword([]byte("existing-account-password"), bcrypt.MinCost)
			_, err = f.DB.Collection("users").InsertOne(f.Ctx, bson.M{"_id": user, "email": input.Email, "normalizedEmail": input.Email, "password": string(password), "isActive": true, "emailVerifiedAt": now})
			if err != nil {
				t.Fatal(err)
			}
			tenant, _ := primitive.ObjectIDFromHex(input.Target.TenantID)
			status := models.MembershipStatusActive
			if disabled {
				status = models.MembershipStatusDisabled
			}
			original := models.TenantMembership{ID: primitive.NewObjectID(), UserID: user, TenantID: tenant, Status: status, Roles: []string{models.TenantRoleBilling, models.TenantRoleAuditor}}
			_, err = f.DB.Collection("tenant_memberships").InsertOne(f.Ctx, original)
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.DB.Collection("tenant_memberships").InsertOne(f.Ctx, models.TenantMembership{ID: primitive.NewObjectID(), UserID: user, TenantID: otherTenant, Status: models.MembershipStatusActive, Roles: []string{models.TenantRoleOwner}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.DB.Collection("auth_sessions").InsertOne(f.Ctx, bson.M{"familyId": "existing-other-tenant", "tenantId": otherTenant.Hex(), "userId": user.Hex(), "identity": "management"})
			if err != nil {
				t.Fatal(err)
			}
			identity := NewInvitationIdentityService(f.DB, cfg, keys)
			rotated, proof, err := identity.Reauthenticate(f.Ctx, handle, input.Email, []byte("existing-account-password"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := NewInvitationAcceptanceService(f.DB).Accept(f.Ctx, models.AcceptProjectInvitationInput{ContinuationHandle: rotated, Proof: proof})
			if disabled && err == nil {
				t.Fatal("disabled tenant membership reactivated")
			}
			if !disabled && (err != nil || !reflect.DeepEqual(result.TenantRoles, original.Roles)) {
				t.Fatalf("roles changed: %v %v", result.TenantRoles, err)
			}
			var stored models.TenantMembership
			_ = f.DB.Collection("tenant_memberships").FindOne(f.Ctx, bson.M{"_id": original.ID}).Decode(&stored)
			if !reflect.DeepEqual(stored.Roles, original.Roles) || stored.Status != status {
				t.Fatal("existing membership mutated")
			}
			sessions, _ := f.DB.Collection("auth_sessions").CountDocuments(f.Ctx, bson.M{"familyId": "existing-other-tenant"})
			if sessions != 1 {
				t.Fatal("other tenant session changed")
			}
			var account struct {
				Password string `bson:"password"`
			}
			_ = f.DB.Collection("users").FindOne(f.Ctx, bson.M{"_id": user}).Decode(&account)
			if account.Password != string(password) {
				t.Fatal("password changed")
			}
		})
	}
}
func TestCollaborationAcceptanceAuditRollback(t *testing.T) {
	f, issuer, input, _, _ := invitationFixture(t)
	invite, err := issuer.Create(f.Ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	handle, _, err := NewInvitationContinuationService(f.DB).Exchange(f.Ctx, rawInvitationToken(t, issuer, invite.ID))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.DB.RunCommand(f.Ctx, bson.D{{Key: "collMod", Value: "management_collaboration_events"}, {Key: "validator", Value: bson.M{"requiredSentinel": true}}}).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewInvitationAcceptanceService(f.DB).Accept(f.Ctx, models.AcceptProjectInvitationInput{ContinuationHandle: handle, CreateAccount: true, Name: "Rollback", Password: []byte("new-account-password")}); err == nil {
		t.Fatal("failed durable audit committed acceptance")
	}
	users, _ := f.DB.Collection("users").CountDocuments(f.Ctx, bson.M{"normalizedEmail": input.Email})
	if users != 0 {
		t.Fatal("rollback retained account")
	}
	if _, err := NewInvitationContinuationService(f.DB).Resume(f.Ctx, handle); err != nil {
		t.Fatal("rollback consumed continuation")
	}
}
