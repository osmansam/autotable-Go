package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrInvitationConflict      = errors.New("invitation request conflicts with existing state")
	ErrInvitationInvalid       = errors.New("invitation is no longer valid")
	ErrInvitationConfiguration = errors.New("invitation delivery is not configured")
	ErrInvitationMemberExists  = errors.New("the invited user already has a project membership")
)

type InvitationService struct {
	db         *mongo.Database
	guard      *repositories.ManagementCollaborationRepository
	repository *repositories.ManagementInvitationRepository
	config     configs.InvitationMailConfig
	keys       *utils.InvitationKeyring
	now        func() time.Time
}

func NewInvitationService(db *mongo.Database, cfg configs.InvitationMailConfig, keys *utils.InvitationKeyring) *InvitationService {
	return &InvitationService{db: db, guard: repositories.NewManagementCollaborationRepository(db), repository: repositories.NewManagementInvitationRepository(db), config: cfg, keys: keys, now: time.Now}
}
func invitationSummary(invite models.ManagementInvitation, now time.Time) models.InvitationSummary {
	status := invite.Status
	if status == models.InvitationPending && !invite.ExpiresAt.After(now) {
		status = models.InvitationExpired
	}
	return models.InvitationSummary{ID: invite.ID, TenantID: invite.TenantID, ProjectID: invite.ProjectID, Email: invite.NormalizedEmail, Role: invite.Role, Version: invite.TokenVersion, Status: status, DeliveryState: invite.DeliveryState, ExpiresAt: invite.ExpiresAt, CreatedAt: invite.CreatedAt, UpdatedAt: invite.UpdatedAt}
}
func normalizedInvitationEmail(email string) (string, error) {
	email = models.NormalizeManagementEmail(email)
	address, err := mail.ParseAddress(email)
	if err != nil || len(email) > 254 || address.Address != email || address.Name != "" || strings.ContainsAny(email, "\r\n") {
		return "", ErrCollaborationInvalid
	}
	return email, nil
}
func invitationRequestIdentity(authority models.CollaborationAuthority, actor models.ManagementAuthorizationSubject, target models.ManagementAuthorizationTarget, operation, key string, payload interface{}) (string, string, error) {
	if strings.TrimSpace(key) == "" || len(key) > 128 || strings.ContainsAny(key, "\r\n") {
		return "", "", ErrCollaborationInvalid
	}
	scope, _ := json.Marshal([]string{string(authority), actor.UserID, target.TenantID, target.ProjectID, operation, key})
	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}
	return utils.HashInvitationToken(string(scope)), utils.HashInvitationToken(string(body)), nil
}
func (s *InvitationService) replay(ctx context.Context, key, digest string) (*models.InvitationSummary, error) {
	var request models.InvitationRequestRecord
	err := s.db.Collection("management_collaboration_requests").FindOne(ctx, bson.M{"requestKey": key}).Decode(&request)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if request.Digest != digest {
		return nil, ErrInvitationConflict
	}
	return &request.Result, nil
}
func (s *InvitationService) recordRequest(ctx context.Context, key, digest string, result models.InvitationSummary, id primitive.ObjectID) error {
	_, err := s.db.Collection("management_collaboration_requests").InsertOne(ctx, models.InvitationRequestRecord{ID: id, RequestKey: key, Digest: digest, Result: result, ExpiresAt: s.now().Add(8 * 24 * time.Hour)})
	return err
}
func invitationDeliveryAAD(job models.InvitationDeliveryJob) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s:%d", job.Purpose, job.SubjectID.Hex(), job.InvitationID.Hex(), job.TenantID.Hex(), job.ProjectID.Hex(), job.TokenVersion)
}
func (s *InvitationService) deliveryJob(invite models.ManagementInvitation, raw string, now time.Time) (models.InvitationDeliveryJob, error) {
	panel, err := url.Parse(s.config.PanelURL)
	if err != nil || panel.Scheme != "https" || panel.Host == "" {
		return models.InvitationDeliveryJob{}, ErrInvitationConfiguration
	}
	panel.Path = "/invitations/accept"
	panel.Fragment = raw
	job := models.InvitationDeliveryJob{ID: primitive.NewObjectID(), Purpose: "invitation", SubjectID: invite.ID, InvitationID: invite.ID, TenantID: invite.TenantID, ProjectID: invite.ProjectID, TokenVersion: invite.TokenVersion, Status: "queued", NextAttemptAt: now, ExpiresAt: invite.ExpiresAt, CreatedAt: now, UpdatedAt: now}
	job.JobKey = fmt.Sprintf("invitation:%s:%d", invite.ID.Hex(), invite.TokenVersion)
	job.MessageID = fmt.Sprintf("autotable.%s.%d.invitation@%s", invite.ID.Hex(), invite.TokenVersion, panel.Hostname())
	message := models.InvitationMessage{To: invite.NormalizedEmail, MessageID: job.MessageID, Subject: "You are invited to an AutoTable project", TextBody: fmt.Sprintf("You have been invited to collaborate on an AutoTable project as %s.\n\nOpen your invitation:\n%s\n\nSign in with %s to join. This invitation expires on %s.\n", strings.TrimPrefix(invite.Role, "project_"), panel.String(), invite.NormalizedEmail, invite.ExpiresAt.UTC().Format(time.RFC1123))}
	payload, err := json.Marshal(message)
	if err != nil {
		return job, err
	}
	job.EncryptedPayload, err = s.keys.Seal(payload, invitationDeliveryAAD(job))
	return job, err
}
func (s *InvitationService) invitationEvent(id primitive.ObjectID, actor models.ManagementAuthorizationSubject, invite models.ManagementInvitation, action string) models.CollaborationEvent {
	now := s.now().UTC()
	actorID, _ := primitive.ObjectIDFromHex(actor.UserID)
	return models.CollaborationEvent{ID: id, TenantID: invite.TenantID.Hex(), ProjectID: invite.ProjectID.Hex(), Status: "pending", NextAttemptAt: now, CreatedAt: now, Audit: models.AuditLog{EventID: id, TenantID: invite.TenantID.Hex(), ProjectID: invite.ProjectID.Hex(), Timestamp: primitive.NewDateTimeFromTime(now), UserID: actorID, SchemaName: "management_invitations", DocumentIDs: []primitive.ObjectID{invite.ID}, Action: action, After: bson.M{"email": invite.NormalizedEmail, "role": invite.Role, "status": invite.Status, "version": invite.TokenVersion}}}
}
func (s *InvitationService) ensureNotMember(ctx context.Context, invite models.ManagementInvitation) error {
	var user models.User
	err := s.db.Collection("users").FindOne(ctx, bson.M{"normalizedEmail": invite.NormalizedEmail}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil
	}
	if err != nil {
		return err
	}
	count, err := s.db.Collection("project_memberships").CountDocuments(ctx, bson.M{"tenantId": invite.TenantID, "projectId": invite.ProjectID, "userId": user.ID})
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrInvitationMemberExists
	}
	return nil
}
func (s *InvitationService) Create(ctx context.Context, input models.CreateProjectInvitationInput) (*models.InvitationSummary, error) {
	email, err := normalizedInvitationEmail(input.Email)
	if err != nil || !IsInvitableProjectRole(input.Role) {
		return nil, ErrCollaborationInvalid
	}
	tenant, project, actorID, err := collaborationIDs(input.Target, input.Actor.UserID)
	if err != nil {
		return nil, err
	}
	key, digest, err := invitationRequestIdentity(input.Authority, input.Actor, input.Target, "create", input.IdempotencyKey, []string{email, input.Role})
	if err != nil {
		return nil, err
	}
	raw, hash, err := utils.GenerateInvitationToken()
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	invite := models.ManagementInvitation{ID: primitive.NewObjectID(), TenantID: tenant, ProjectID: project, NormalizedEmail: email, Role: input.Role, Authority: input.Authority, InvitedBy: actorID, TokenHash: hash, TokenVersion: 1, Status: models.InvitationPending, DeliveryState: "queued", ExpiresAt: now.Add(7 * 24 * time.Hour), CreatedAt: now, UpdatedAt: now}
	eventID, requestID, jobID := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	var result *models.InvitationSummary
	err = s.guard.WithAuthorizationTransaction(ctx, input.Actor, input.Target, func(tx context.Context, snapshot models.ManagementMembershipSnapshot) error {
		if err := authorizeCollaboration(input.Authority, input.Actor, input.Target, snapshot, true); err != nil {
			return err
		}
		replay, err := s.replay(tx, key, digest)
		if err != nil {
			return err
		}
		if replay != nil {
			result = replay
			return nil
		}
		if err := s.ensureNotMember(tx, invite); err != nil {
			return err
		}
		if _, err := s.db.Collection("management_invitations").UpdateMany(tx, bson.M{"tenantId": tenant, "projectId": project, "normalizedEmail": email, "status": models.InvitationPending, "expiresAt": bson.M{"$lte": now}}, bson.M{"$set": bson.M{"status": models.InvitationExpired, "updatedAt": now}}); err != nil {
			return err
		}
		count, err := s.db.Collection("management_invitations").CountDocuments(tx, bson.M{"tenantId": tenant, "projectId": project, "normalizedEmail": email, "status": models.InvitationPending})
		if err != nil {
			return err
		}
		if count > 0 {
			return ErrInvitationConflict
		}
		job, err := s.deliveryJob(invite, raw, now)
		job.ID = jobID
		if err != nil {
			return err
		}
		if _, err := s.db.Collection("management_invitations").InsertOne(tx, invite); err != nil {
			return err
		}
		if _, err := s.db.Collection("management_invitation_deliveries").InsertOne(tx, job); err != nil {
			return err
		}
		if err := s.guard.InsertCollaborationEvent(tx, s.invitationEvent(eventID, input.Actor, invite, "project.invitation.created")); err != nil {
			return err
		}
		summary := invitationSummary(invite, now)
		result = &summary
		return s.recordRequest(tx, key, digest, summary, requestID)
	})
	return result, err
}
func (s *InvitationService) obsoleteJobs(ctx context.Context, invitationID primitive.ObjectID) error {
	_, err := s.db.Collection("management_invitation_deliveries").UpdateMany(ctx, bson.M{"invitationId": invitationID, "status": bson.M{"$in": bson.A{"queued", "leased", "retryable_failure"}}}, bson.M{"$set": bson.M{"status": "obsolete", "updatedAt": s.now().UTC()}, "$unset": bson.M{"encryptedPayload": "", "leaseKey": "", "leaseUntil": ""}})
	if err != nil {
		return err
	}
	_, err = s.db.Collection("management_invitation_continuations").DeleteMany(ctx, bson.M{"invitationId": invitationID})
	return err
}
func (s *InvitationService) Resend(ctx context.Context, input models.ResendProjectInvitationInput) (*models.InvitationSummary, error) {
	tenant, project, actorID, err := collaborationIDs(input.Target, input.Actor.UserID)
	if err != nil {
		return nil, err
	}
	id, err := primitive.ObjectIDFromHex(input.InvitationID)
	if err != nil || input.ExpectedVersion < 1 {
		return nil, ErrCollaborationInvalid
	}
	key, digest, err := invitationRequestIdentity(input.Authority, input.Actor, input.Target, "resend", input.IdempotencyKey, []interface{}{input.InvitationID, input.ExpectedVersion})
	if err != nil {
		return nil, err
	}
	raw, hash, err := utils.GenerateInvitationToken()
	if err != nil {
		return nil, err
	}
	eventID, requestID, jobID := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	var result *models.InvitationSummary
	err = s.guard.WithAuthorizationTransaction(ctx, input.Actor, input.Target, func(tx context.Context, snapshot models.ManagementMembershipSnapshot) error {
		if err := authorizeCollaboration(input.Authority, input.Actor, input.Target, snapshot, true); err != nil {
			return err
		}
		replay, err := s.replay(tx, key, digest)
		if err != nil {
			return err
		}
		if replay != nil {
			result = replay
			return nil
		}
		invite, err := s.repository.GetScoped(tx, id, tenant, project)
		if err != nil {
			return err
		}
		if invite.TokenVersion != input.ExpectedVersion {
			return ErrCollaborationVersion
		}
		if invite.Status != models.InvitationPending && invite.Status != models.InvitationExpired {
			return ErrInvitationConflict
		}
		if err := s.ensureNotMember(tx, *invite); err != nil {
			return err
		}
		now := s.now().UTC()
		invite.TokenVersion++
		invite.TokenHash = hash
		invite.ExpiresAt = now.Add(7 * 24 * time.Hour)
		invite.Status = models.InvitationPending
		invite.DeliveryState = "queued"
		invite.InvitedBy = actorID
		invite.Authority = input.Authority
		invite.UpdatedAt = now
		job, err := s.deliveryJob(*invite, raw, now)
		job.ID = jobID
		if err != nil {
			return err
		}
		if err := s.obsoleteJobs(tx, id); err != nil {
			return err
		}
		if _, err := s.db.Collection("management_invitations").ReplaceOne(tx, bson.M{"_id": id, "tenantId": tenant, "projectId": project}, invite); err != nil {
			return err
		}
		if _, err := s.db.Collection("management_invitation_deliveries").InsertOne(tx, job); err != nil {
			return err
		}
		if err := s.guard.InsertCollaborationEvent(tx, s.invitationEvent(eventID, input.Actor, *invite, "project.invitation.resent")); err != nil {
			return err
		}
		summary := invitationSummary(*invite, now)
		result = &summary
		return s.recordRequest(tx, key, digest, summary, requestID)
	})
	return result, err
}
func (s *InvitationService) Revoke(ctx context.Context, input models.RevokeProjectInvitationInput) error {
	tenant, project, _, err := collaborationIDs(input.Target, input.Actor.UserID)
	if err != nil {
		return err
	}
	id, err := primitive.ObjectIDFromHex(input.InvitationID)
	if err != nil || input.ExpectedVersion < 1 {
		return ErrCollaborationInvalid
	}
	eventID := primitive.NewObjectID()
	return s.guard.WithAuthorizationTransaction(ctx, input.Actor, input.Target, func(tx context.Context, snapshot models.ManagementMembershipSnapshot) error {
		if err := authorizeCollaboration(input.Authority, input.Actor, input.Target, snapshot, true); err != nil {
			return err
		}
		invite, err := s.repository.GetScoped(tx, id, tenant, project)
		if err != nil {
			return err
		}
		if invite.TokenVersion != input.ExpectedVersion {
			return ErrCollaborationVersion
		}
		if invite.Status == models.InvitationRevoked {
			return nil
		}
		if invite.Status == models.InvitationAccepted {
			return ErrInvitationConflict
		}
		invite.Status = models.InvitationRevoked
		invite.DeliveryState = "canceled"
		invite.UpdatedAt = s.now().UTC()
		if _, err := s.db.Collection("management_invitations").UpdateOne(tx, bson.M{"_id": id}, bson.M{"$set": bson.M{"status": invite.Status, "deliveryState": invite.DeliveryState, "updatedAt": invite.UpdatedAt}}); err != nil {
			return err
		}
		if err := s.obsoleteJobs(tx, id); err != nil {
			return err
		}
		return s.guard.InsertCollaborationEvent(tx, s.invitationEvent(eventID, input.Actor, *invite, "project.invitation.revoked"))
	})
}
func (s *InvitationService) List(ctx context.Context, authority models.CollaborationAuthority, actor models.ManagementAuthorizationSubject, target models.ManagementAuthorizationTarget, page models.CollaborationPageRequest) (*models.InvitationSummaryPage, error) {
	snapshot, err := repositories.NewManagementMembershipRepositoryWithDatabase(s.db).LoadManagementMembership(ctx, actor, target)
	if err != nil {
		return nil, err
	}
	if err := authorizeCollaboration(authority, actor, target, snapshot, false); err != nil {
		return nil, err
	}
	tenant, project, _, err := collaborationIDs(target, actor.UserID)
	if err != nil {
		return nil, err
	}
	if page.Limit == 0 {
		page.Limit = 20
	}
	if page.Limit < 1 || page.Limit > 100 {
		return nil, ErrCollaborationInvalid
	}
	filter := bson.M{"tenantId": tenant, "projectId": project}
	if page.AfterID != "" {
		id, err := primitive.ObjectIDFromHex(page.AfterID)
		if err != nil {
			return nil, ErrCollaborationInvalid
		}
		filter["_id"] = bson.M{"$gt": id}
	}
	cursor, err := s.db.Collection("management_invitations").Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(page.Limit+1)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	invites := []models.ManagementInvitation{}
	if err := cursor.All(ctx, &invites); err != nil {
		return nil, err
	}
	result := &models.InvitationSummaryPage{Items: []models.InvitationSummary{}}
	if len(invites) > page.Limit {
		invites = invites[:page.Limit]
		result.NextCursor = invites[len(invites)-1].ID.Hex()
	}
	for _, invite := range invites {
		result.Items = append(result.Items, invitationSummary(invite, s.now()))
	}
	return result, nil
}
