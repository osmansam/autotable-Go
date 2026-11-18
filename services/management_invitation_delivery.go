package services

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/mongo"
	"log"
	"time"
)

type MailTransport interface {
	Send(context.Context, models.InvitationMessage) error
}
type InvitationDeliveryWorker struct {
	db         *mongo.Database
	repository *repositories.ManagementInvitationRepository
	keys       *utils.InvitationKeyring
	transport  MailTransport
	config     configs.InvitationMailConfig
}

func NewInvitationDeliveryWorker(db *mongo.Database, cfg configs.InvitationMailConfig, keys *utils.InvitationKeyring, transport MailTransport) *InvitationDeliveryWorker {
	return &InvitationDeliveryWorker{db: db, repository: repositories.NewManagementInvitationRepository(db), keys: keys, transport: transport, config: cfg}
}
func CurrentInvitationAuthority(ctx context.Context, db *mongo.Database, invite models.ManagementInvitation) error {
	actor, target := models.ManagementAuthorizationSubject{UserID: invite.InvitedBy.Hex(), TenantID: invite.TenantID.Hex()}, models.ManagementAuthorizationTarget{TenantID: invite.TenantID.Hex(), ProjectID: invite.ProjectID.Hex()}
	snapshot, err := repositories.NewManagementMembershipRepositoryWithDatabase(db).LoadManagementMembership(ctx, actor, target)
	if err != nil {
		return err
	}
	return authorizeCollaboration(invite.Authority, actor, target, snapshot, true)
}
func (w *InvitationDeliveryWorker) ProcessNext(ctx context.Context, workerID string, now time.Time) (bool, error) {
	job, err := w.repository.LeaseDelivery(ctx, workerID, now)
	if err != nil || job == nil {
		return false, err
	}
	finish := func(state, code string) (bool, error) {
		return true, w.repository.FinishDelivery(ctx, *job, state, code, now, now.Add(invitationRetryDelay(job.Attempts)))
	}
	failure := func(code string) (bool, error) {
		state := "retryable_failure"
		if job.Attempts >= 10 {
			state = "terminal_failure"
		}
		return finish(state, code)
	}
	if !job.ExpiresAt.After(now) || job.Attempts > 10 {
		return finish("terminal_failure", "expired_or_exhausted")
	}
	invite, err := w.repository.GetByID(ctx, job.InvitationID)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return finish("obsolete", "invitation_unavailable")
	}
	if err != nil {
		return failure("database_unavailable")
	}
	if invite.Status != models.InvitationPending || !invite.ExpiresAt.After(now) || invite.TenantID != job.TenantID || invite.ProjectID != job.ProjectID {
		return finish("obsolete", "invitation_unavailable")
	}
	if job.Purpose == "invitation" && (job.SubjectID != invite.ID || job.TokenVersion != invite.TokenVersion) {
		return finish("obsolete", "invitation_rotated")
	}
	if err := CurrentInvitationAuthority(ctx, w.db, *invite); err != nil {
		if errors.Is(err, ErrManagementForbidden) {
			return finish("obsolete", "sender_not_authorized")
		}
		return failure("authorization_unavailable")
	}
	if job.Purpose == "email_verification" {
		challenge, err := repositories.NewManagementEmailChallengeRepository(w.db).GetByID(ctx, job.SubjectID)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return finish("obsolete", "verification_unavailable")
		}
		if err != nil {
			return failure("database_unavailable")
		}
		if challenge.Status != "pending" || challenge.Attempts >= 5 || challenge.Version != job.TokenVersion || challenge.InvitationID != invite.ID || challenge.InvitationVersion != invite.TokenVersion || !challenge.ExpiresAt.After(now) {
			return finish("obsolete", "verification_unavailable")
		}
		continuation, err := repositories.NewManagementInvitationContinuationRepository(w.db).GetByID(ctx, challenge.ContinuationID, now)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return finish("obsolete", "verification_unavailable")
		}
		if err != nil {
			return failure("database_unavailable")
		}
		account, err := repositories.NewManagementAccountRepository(w.db).GetByID(ctx, challenge.UserID)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return finish("obsolete", "verification_unavailable")
		}
		if err != nil {
			return failure("database_unavailable")
		}
		if continuation.UserID != account.ID || continuation.InvitationVersion != invite.TokenVersion || !account.IsActive || account.EmailVerifiedAt != nil || account.AccountVersion != challenge.AccountVersion || account.NormalizedEmail != invite.NormalizedEmail || challenge.NormalizedEmail != invite.NormalizedEmail {
			return finish("obsolete", "verification_unavailable")
		}
	} else if job.Purpose != "invitation" {
		return finish("obsolete", "delivery_purpose_unavailable")
	}
	plaintext, err := w.keys.Open(job.EncryptedPayload, invitationDeliveryAAD(*job))
	if err != nil {
		return failure("delivery_key_unavailable")
	}
	defer clear(plaintext)
	var message models.InvitationMessage
	if err := json.Unmarshal(plaintext, &message); err != nil || message.To != invite.NormalizedEmail || message.MessageID != job.MessageID {
		return finish("terminal_failure", "invalid_delivery_payload")
	}
	if w.transport == nil {
		return failure("transport_unavailable")
	}
	if err := w.transport.Send(ctx, message); err != nil {
		return failure("smtp_failed")
	}
	return finish("smtp_accepted", "")
}
func ValidateInvitationConfiguration() (configs.InvitationMailConfig, *utils.InvitationKeyring, error) {
	cfg, err := configs.LoadInvitationMailConfig()
	if err != nil {
		return cfg, nil, ErrInvitationConfiguration
	}
	keys, err := utils.NewInvitationKeyring(cfg.KeyringJSON, cfg.ActiveKeyID)
	if err != nil {
		return cfg, nil, ErrInvitationConfiguration
	}
	return cfg, keys, nil
}
func StartInvitationDeliveryWorker(ctx context.Context, workerID string, interval time.Duration) {
	db := configs.GetCollection("management_invitation_deliveries").Database()
	repository := repositories.NewManagementInvitationRepository(db)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			taskCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			if err := repository.PurgeExpiredDeliveries(taskCtx, now); err != nil {
				log.Print("Invitation expiry cleanup failed; retry scheduled")
			}
			flags := configs.GetCollaborationConfig()
			if flags.MembersEnabled && flags.InvitationsEnabled {
				cfg, keys, err := ValidateInvitationConfiguration()
				if err != nil {
					log.Print("Invitation delivery unavailable; configuration requires attention")
				} else {
					worker := NewInvitationDeliveryWorker(db, cfg, keys, NewSMTPInvitationTransport(cfg))
					for i := 0; i < 10; i++ {
						found, err := worker.ProcessNext(taskCtx, workerID, now)
						if err != nil {
							log.Print("Invitation delivery persistence failed; durable retry scheduled")
							break
						}
						if !found {
							break
						}
					}
				}
			}
			cancel()
		}
	}
}
