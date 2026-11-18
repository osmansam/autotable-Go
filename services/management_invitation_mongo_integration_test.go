//go:build integration

package services

import (
	"context"
	"encoding/base64"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/internal/testmongo"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func invitationFixture(t *testing.T) (*testmongo.Fixture, *InvitationService, models.CreateProjectInvitationInput, *utils.InvitationKeyring, configs.InvitationMailConfig) {
	t.Helper()
	f, actor, target, _ := collaborationMemberFixture(t)
	if err := repositories.EnsureCollaborationIndexes(f.Ctx, f.DB); err != nil {
		t.Fatal(err)
	}
	keys, err := utils.NewInvitationKeyring(`{"test":"`+base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))+`"}`, "test")
	if err != nil {
		t.Fatal(err)
	}
	cfg := configs.InvitationMailConfig{PanelURL: "https://tenant.example.test"}
	service := NewInvitationService(f.DB, cfg, keys)
	return f, service, models.CreateProjectInvitationInput{Authority: models.CollaborationAuthorityProject, Actor: actor, Target: target, Email: "collaborator@example.test", Role: models.ProjectRoleViewer, IdempotencyKey: "create-test-invitation"}, keys, cfg
}
func TestCollaborationInvitationIdempotency(t *testing.T) {
	f, service, input, _, _ := invitationFixture(t)
	first, err := service.Create(f.Ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(f.Ctx, input)
	if err != nil || first.ID != second.ID {
		t.Fatalf("idempotency failed: %v", err)
	}
	if first.ExpiresAt.Sub(first.CreatedAt) != 7*24*time.Hour {
		t.Fatal("invitation lifetime changed")
	}
	for _, collection := range []string{"management_invitations", "management_invitation_deliveries", "management_collaboration_requests", "management_collaboration_events"} {
		count, _ := f.DB.Collection(collection).CountDocuments(f.Ctx, bson.M{})
		if count != 1 {
			t.Fatalf("duplicate %s: %d", collection, count)
		}
	}
	input.Role = models.ProjectRoleAdmin
	if _, err := service.Create(f.Ctx, input); err == nil {
		t.Fatal("idempotency key changed request digest")
	}
}
func TestCollaborationInvitationRotation(t *testing.T) {
	f, service, input, _, _ := invitationFixture(t)
	first, err := service.Create(f.Ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	var original models.ManagementInvitation
	if err := f.DB.Collection("management_invitations").FindOne(f.Ctx, bson.M{"_id": first.ID}).Decode(&original); err != nil {
		t.Fatal(err)
	}
	request := models.ResendProjectInvitationInput{Authority: input.Authority, Actor: input.Actor, Target: input.Target, InvitationID: first.ID.Hex(), ExpectedVersion: 1, IdempotencyKey: "resend-test"}
	rotated, err := service.Resend(f.Ctx, request)
	if err != nil || rotated.Version != 2 {
		t.Fatalf("rotation failed: %v", err)
	}
	replay, err := service.Resend(f.Ctx, request)
	if err != nil || replay.Version != 2 {
		t.Fatal("resend idempotency rotated twice")
	}
	var current models.ManagementInvitation
	_ = f.DB.Collection("management_invitations").FindOne(f.Ctx, bson.M{"_id": first.ID}).Decode(&current)
	if current.TokenHash == original.TokenHash {
		t.Fatal("old token remained valid")
	}
	var oldJob models.InvitationDeliveryJob
	if err := f.DB.Collection("management_invitation_deliveries").FindOne(f.Ctx, bson.M{"invitationId": first.ID, "tokenVersion": 1}).Decode(&oldJob); err != nil || oldJob.Status != "obsolete" || oldJob.EncryptedPayload != "" {
		t.Fatalf("old secret was retained: %v", err)
	}
}
func TestCollaborationMongoDeliveryOutboxAtomic(t *testing.T) {
	f, service, input, _, _ := invitationFixture(t)
	if err := f.DB.RunCommand(f.Ctx, bson.D{{Key: "collMod", Value: "management_collaboration_events"}, {Key: "validator", Value: bson.M{"requiredSentinel": true}}}).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(f.Ctx, input); err == nil {
		t.Fatal("invitation survived durable audit failure")
	}
	for _, collection := range []string{"management_invitations", "management_invitation_deliveries", "management_collaboration_requests"} {
		count, _ := f.DB.Collection(collection).CountDocuments(f.Ctx, bson.M{})
		if count != 0 {
			t.Fatalf("transaction left partial %s", collection)
		}
	}
}

type invitationTestTransport func(context.Context, models.InvitationMessage) error

func (fn invitationTestTransport) Send(ctx context.Context, message models.InvitationMessage) error {
	return fn(ctx, message)
}
func TestCollaborationMongoResendAgainstLeasedWorker(t *testing.T) {
	f, service, input, keys, cfg := invitationFixture(t)
	first, err := service.Create(f.Ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	sent := make(chan models.InvitationMessage, 2)
	release := make(chan struct{})
	var calls atomic.Int32
	transport := invitationTestTransport(func(ctx context.Context, message models.InvitationMessage) error {
		sent <- message
		if calls.Add(1) == 1 {
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
	worker := NewInvitationDeliveryWorker(f.DB, cfg, keys, transport)
	done := make(chan error, 1)
	go func() { _, err := worker.ProcessNext(f.Ctx, "first", time.Now().Add(time.Second)); done <- err }()
	var original models.InvitationMessage
	select {
	case original = <-sent:
	case <-f.Ctx.Done():
		t.Fatal("worker did not send")
	}
	rotated, err := service.Resend(f.Ctx, models.ResendProjectInvitationInput{Authority: input.Authority, Actor: input.Actor, Target: input.Target, InvitationID: first.ID.Hex(), ExpectedVersion: 1, IdempotencyKey: "race-resend"})
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := worker.ProcessNext(f.Ctx, "second", time.Now().Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	next := <-sent
	if next.MessageID == original.MessageID || rotated.Version != 2 {
		t.Fatal("new delivery reused obsolete version")
	}
}
func TestCollaborationMongoDeliveryCrashReplay(t *testing.T) {
	f, service, input, keys, cfg := invitationFixture(t)
	summary, err := service.Create(f.Ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	var deliveries []string
	transport := invitationTestTransport(func(_ context.Context, message models.InvitationMessage) error {
		deliveries = append(deliveries, message.MessageID)
		return nil
	})
	worker := NewInvitationDeliveryWorker(f.DB, cfg, keys, transport)
	// Simulate the first SMTP request succeeding before process death loses its ack.
	var job models.InvitationDeliveryJob
	if err := f.DB.Collection("management_invitation_deliveries").FindOneAndUpdate(f.Ctx, bson.M{"invitationId": summary.ID}, bson.M{"$set": bson.M{"status": "leased", "leaseUntil": time.Now().Add(-time.Minute), "attempts": 1}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&job); err != nil {
		t.Fatal(err)
	}
	deliveries = append(deliveries, job.MessageID)
	if found, err := worker.ProcessNext(f.Ctx, "restart", time.Now().Add(time.Second)); !found || err != nil {
		t.Fatalf("crashed job not recovered: %v", err)
	}
	if len(deliveries) != 2 || deliveries[0] != deliveries[1] {
		t.Fatal("retry created a new email/grant identity")
	}
	var finished models.InvitationDeliveryJob
	_ = f.DB.Collection("management_invitation_deliveries").FindOne(f.Ctx, bson.M{"_id": job.ID}).Decode(&finished)
	if finished.Status != "smtp_accepted" || finished.EncryptedPayload != "" {
		t.Fatal("successful send retained recoverable secret")
	}
	count, _ := f.DB.Collection("project_memberships").CountDocuments(f.Ctx, bson.M{})
	if count != 2 {
		t.Fatal("delivery granted membership before acceptance")
	}
}
func TestCollaborationMongoInvitationAuthorityLoss(t *testing.T) {
	f, service, input, keys, cfg := invitationFixture(t)
	if _, err := service.Create(f.Ctx, input); err != nil {
		t.Fatal(err)
	}
	actor, _ := primitive.ObjectIDFromHex(input.Actor.UserID)
	if _, err := f.DB.Collection("project_memberships").UpdateOne(f.Ctx, bson.M{"userId": actor}, bson.M{"$set": bson.M{"roles": bson.A{models.ProjectRoleViewer}}}); err != nil {
		t.Fatal(err)
	}
	sends := 0
	worker := NewInvitationDeliveryWorker(f.DB, cfg, keys, invitationTestTransport(func(context.Context, models.InvitationMessage) error { sends++; return nil }))
	if _, err := worker.ProcessNext(f.Ctx, "test", time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if sends != 0 {
		t.Fatal("lost authority still sent invitation")
	}
}

func TestCollaborationConcurrentInvitationIdempotency(t *testing.T) {
	f, service, input, _, _ := invitationFixture(t)
	concurrent := func(operation func() (*models.InvitationSummary, error)) []*models.InvitationSummary {
		t.Helper()
		start := make(chan struct{})
		results := make(chan *models.InvitationSummary, 6)
		failures := make(chan error, 6)
		var group sync.WaitGroup
		for i := 0; i < 6; i++ {
			group.Add(1)
			go func() { defer group.Done(); <-start; result, err := operation(); results <- result; failures <- err }()
		}
		close(start)
		group.Wait()
		close(results)
		close(failures)
		for err := range failures {
			if err != nil {
				t.Fatalf("concurrent same-key replay failed: %v", err)
			}
		}
		var all []*models.InvitationSummary
		for result := range results {
			all = append(all, result)
		}
		return all
	}
	created := concurrent(func() (*models.InvitationSummary, error) { return service.Create(f.Ctx, input) })
	for _, item := range created {
		if item.ID != created[0].ID || item.Version != 1 {
			t.Fatal("concurrent create duplicated invitation")
		}
	}
	resend := models.ResendProjectInvitationInput{Authority: input.Authority, Actor: input.Actor, Target: input.Target, InvitationID: created[0].ID.Hex(), ExpectedVersion: 1, IdempotencyKey: "concurrent-resend"}
	resent := concurrent(func() (*models.InvitationSummary, error) { return service.Resend(f.Ctx, resend) })
	for _, item := range resent {
		if item.ID != created[0].ID || item.Version != 2 {
			t.Fatal("concurrent resend rotated more than once")
		}
	}
	for collection, want := range map[string]int64{"management_invitations": 1, "management_invitation_deliveries": 2, "management_collaboration_requests": 2, "management_collaboration_events": 2} {
		count, err := f.DB.Collection(collection).CountDocuments(f.Ctx, bson.M{})
		if err != nil || count != want {
			t.Fatalf("duplicate %s: %d %v", collection, count, err)
		}
	}
}
