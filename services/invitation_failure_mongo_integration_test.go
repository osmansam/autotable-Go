//go:build integration

package services

import (
	"context"
	"errors"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"strings"
	"testing"
	"time"
)

func TestCollaborationMongoDeliveryFailureBudget(t *testing.T) {
	f, service, input, keys, cfg := invitationFixture(t)
	summary, err := service.Create(f.Ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	sends := 0
	worker := NewInvitationDeliveryWorker(f.DB, cfg, keys, invitationTestTransport(func(context.Context, models.InvitationMessage) error {
		sends++
		return errors.New("do-not-persist-raw-transport-response")
	}))
	now := time.Now()
	for attempt := 1; attempt <= 10; attempt++ {
		if found, err := worker.ProcessNext(f.Ctx, "failure", now.Add(time.Duration(attempt)*2*time.Hour)); !found || err != nil {
			t.Fatalf("retry %d not tracked: %v", attempt, err)
		}
	}
	if found, err := worker.ProcessNext(f.Ctx, "finished", now.Add(24*time.Hour)); found || err != nil {
		t.Fatal("terminal job was sent again")
	}
	if sends != 10 {
		t.Fatalf("send attempts=%d", sends)
	}
	var job models.InvitationDeliveryJob
	if err := f.DB.Collection("management_invitation_deliveries").FindOne(f.Ctx, bson.M{"invitationId": summary.ID}).Decode(&job); err != nil || job.Status != "terminal_failure" || job.EncryptedPayload != "" {
		t.Fatal("failed terminal delivery retained secret")
	}
	var raw bson.M
	_ = f.DB.Collection("management_invitation_deliveries").FindOne(f.Ctx, bson.M{"_id": job.ID}).Decode(&raw)
	encoded, _ := bson.MarshalExtJSON(raw, false, false)
	if strings.Contains(string(encoded), "do-not-persist-raw-transport-response") {
		t.Fatal("raw SMTP response persisted")
	}
	var invite models.ManagementInvitation
	_ = f.DB.Collection("management_invitations").FindOne(f.Ctx, bson.M{"_id": summary.ID}).Decode(&invite)
	if invite.DeliveryState != "failed" {
		t.Fatalf("UI falsely reports delivery state %s", invite.DeliveryState)
	}
}
