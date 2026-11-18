//go:build integration

package services

import (
	"sync"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestCollaborationMongoExpiryDeliveryAudit(t *testing.T) {
	for _, state := range []string{"queued", "retryable_failure", "leased", "legacy_terminal"} {
		t.Run(state, func(t *testing.T) {
			f, issuer, input, _, _ := invitationFixture(t)
			invite, err := issuer.Create(f.Ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			past := now.Add(-time.Minute)
			set := bson.M{"expiresAt": past, "status": state}
			if state == "legacy_terminal" {
				set["status"] = "terminal_failure"
				set["failureCode"] = "expired"
			}
			if state == "leased" {
				set["leaseKey"] = "old-worker"
				set["leaseUntil"] = now.Add(time.Minute)
			}
			_, _ = f.DB.Collection("management_invitation_deliveries").UpdateOne(f.Ctx, bson.M{"invitationId": invite.ID}, bson.M{"$set": set})
			invitationSet := bson.M{"expiresAt": past}
			if state == "legacy_terminal" {
				invitationSet["status"] = models.InvitationExpired
			}
			_, _ = f.DB.Collection("management_invitations").UpdateOne(f.Ctx, bson.M{"_id": invite.ID}, bson.M{"$set": invitationSet})
			repository := repositories.NewManagementInvitationRepository(f.DB)
			start := make(chan struct{})
			failures := make(chan error, 2)
			var group sync.WaitGroup
			for i := 0; i < 2; i++ {
				group.Add(1)
				go func() { defer group.Done(); <-start; failures <- repository.PurgeExpiredDeliveries(f.Ctx, now) }()
			}
			close(start)
			group.Wait()
			close(failures)
			for err := range failures {
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := repository.PurgeExpiredDeliveries(f.Ctx, now); err != nil {
				t.Fatal(err)
			}
			var stored models.ManagementInvitation
			_ = f.DB.Collection("management_invitations").FindOne(f.Ctx, bson.M{"_id": invite.ID}).Decode(&stored)
			if stored.Status != models.InvitationExpired || stored.DeliveryState != "failed" {
				t.Fatalf("expired metadata was not reconciled: %s / %s", stored.Status, stored.DeliveryState)
			}
			var job models.InvitationDeliveryJob
			_ = f.DB.Collection("management_invitation_deliveries").FindOne(f.Ctx, bson.M{"invitationId": invite.ID}).Decode(&job)
			if job.Status != "terminal_failure" || job.EncryptedPayload != "" || job.LeaseKey != "" {
				t.Fatal("expiry retained payload or lease")
			}
			count, err := f.DB.Collection("management_collaboration_events").CountDocuments(f.Ctx, bson.M{"audit.action": "invitation.delivery.terminal_failure", "audit.after.jobId": job.ID, "audit.after.failureCode": "expired"})
			if err != nil || count != 1 {
				t.Fatalf("expiry audit count=%d %v", count, err)
			}
		})
	}
}
func TestCollaborationMongoExpiryAuditRollback(t *testing.T) {
	f, issuer, input, _, _ := invitationFixture(t)
	invite, err := issuer.Create(f.Ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	past := now.Add(-time.Minute)
	_, _ = f.DB.Collection("management_invitation_deliveries").UpdateOne(f.Ctx, bson.M{"invitationId": invite.ID}, bson.M{"$set": bson.M{"expiresAt": past}})
	_, _ = f.DB.Collection("management_invitations").UpdateOne(f.Ctx, bson.M{"_id": invite.ID}, bson.M{"$set": bson.M{"expiresAt": past}})
	if err := f.DB.RunCommand(f.Ctx, bson.D{{Key: "collMod", Value: "management_collaboration_events"}, {Key: "validator", Value: bson.M{"requiredSentinel": true}}}).Err(); err != nil {
		t.Fatal(err)
	}
	if err := repositories.NewManagementInvitationRepository(f.DB).PurgeExpiredDeliveries(f.Ctx, now); err == nil {
		t.Fatal("cleanup committed without durable audit")
	}
	var job models.InvitationDeliveryJob
	_ = f.DB.Collection("management_invitation_deliveries").FindOne(f.Ctx, bson.M{"invitationId": invite.ID}).Decode(&job)
	if job.Status != "queued" || job.EncryptedPayload == "" {
		t.Fatal("audit failure left partial cleanup")
	}
	var stored models.ManagementInvitation
	_ = f.DB.Collection("management_invitations").FindOne(f.Ctx, bson.M{"_id": invite.ID}).Decode(&stored)
	if stored.Status != models.InvitationPending || stored.DeliveryState != "queued" {
		t.Fatal("audit failure left partial invitation metadata")
	}
}
func TestCollaborationMongoExpiryPreservesCurrentInvitation(t *testing.T) {
	for _, purpose := range []string{"rotated_invitation", "email_verification"} {
		t.Run(purpose, func(t *testing.T) {
			f, issuer, input, _, _ := invitationFixture(t)
			invite, err := issuer.Create(f.Ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			past := now.Add(-time.Minute)
			var job models.InvitationDeliveryJob
			_ = f.DB.Collection("management_invitation_deliveries").FindOne(f.Ctx, bson.M{"invitationId": invite.ID}).Decode(&job)
			if purpose == "rotated_invitation" {
				if _, err := issuer.Resend(f.Ctx, models.ResendProjectInvitationInput{Authority: input.Authority, Actor: input.Actor, Target: input.Target, InvitationID: invite.ID.Hex(), ExpectedVersion: 1, IdempotencyKey: "expiry-resend"}); err != nil {
					t.Fatal(err)
				}
				_, _ = f.DB.Collection("management_invitation_deliveries").UpdateOne(f.Ctx, bson.M{"_id": job.ID}, bson.M{"$set": bson.M{"expiresAt": past, "status": "terminal_failure", "failureCode": "expired"}})
			} else {
				job.ID = primitive.NewObjectID()
				job.JobKey = "verification-expiry"
				job.SubjectID = primitive.NewObjectID()
				job.Purpose = "email_verification"
				job.ExpiresAt = past
				if _, err := f.DB.Collection("management_invitation_deliveries").InsertOne(f.Ctx, job); err != nil {
					t.Fatal(err)
				}
			}
			if err := repositories.NewManagementInvitationRepository(f.DB).PurgeExpiredDeliveries(f.Ctx, now); err != nil {
				t.Fatal(err)
			}
			var current models.ManagementInvitation
			_ = f.DB.Collection("management_invitations").FindOne(f.Ctx, bson.M{"_id": invite.ID}).Decode(&current)
			if current.Status != models.InvitationPending || current.DeliveryState != "queued" {
				t.Fatal("old/verification delivery expiry changed current invitation")
			}
			count, _ := f.DB.Collection("management_collaboration_events").CountDocuments(f.Ctx, bson.M{"audit.after.jobId": job.ID, "audit.after.failureCode": "expired"})
			if count != 1 {
				t.Fatal("old/verification expiry audit missing")
			}
		})
	}
}
