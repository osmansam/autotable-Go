package services

import (
	"context"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/mongo"
	"log"
	"time"
)

type ManagementCollaborationWorker struct {
	repository *repositories.ManagementCollaborationRepository
	audit      func(context.Context, models.AuditLog) error
}

func NewManagementCollaborationWorker(db *mongo.Database, audit func(context.Context, models.AuditLog) error) *ManagementCollaborationWorker {
	if audit == nil {
		audit = func(ctx context.Context, event models.AuditLog) error {
			return utils.LogAuditToCollection(ctx, db.Collection(utils.GetProjectCollectionName(event.TenantID, event.ProjectID, "audit_logs")), event)
		}
	}
	return &ManagementCollaborationWorker{repository: repositories.NewManagementCollaborationRepository(db), audit: audit}
}
func (w *ManagementCollaborationWorker) ProcessNext(ctx context.Context, workerID string, now time.Time) (bool, error) {
	event, err := w.repository.LeaseCollaborationEvent(ctx, workerID, now)
	if err != nil || event == nil {
		return false, err
	}
	if event.RevokeUserID != "" {
		err = repositories.NewAuthSessionRepositoryWithCollection(w.repository.Database().Collection("auth_sessions")).RevokeUserProjectSessionsBeforeVersion(ctx, event.RevokeUserID, event.TenantID, event.ProjectID, event.RevokeBeforeVersion, now)
	}
	if err == nil {
		err = w.audit(ctx, event.Audit)
	}
	ackErr := w.repository.FinishCollaborationEvent(ctx, *event, now, err != nil)
	if err != nil {
		return true, err
	}
	return true, ackErr
}
func ProcessCollaborationEvents(ctx context.Context, workerID string, now time.Time) error {
	worker := NewManagementCollaborationWorker(configs.GetCollection("management_collaboration_events").Database(), nil)
	for i := 0; i < 20; i++ {
		found, err := worker.ProcessNext(ctx, workerID, now)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
	}
	return nil
}
func StartManagementCollaborationWorker(ctx context.Context, workerID string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := ProcessCollaborationEvents(ctx, workerID, now); err != nil {
				log.Print("Collaboration security event processing failed; durable retry scheduled")
			}
		}
	}
}
