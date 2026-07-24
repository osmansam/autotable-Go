package services

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	dynamicevents "github.com/osmansam/autotableGo/events"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/observability"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	dynamicOutboxMaxAttempts = 10
	dynamicOutboxPollEvery   = 1 * time.Second
)

func StartDynamicOutboxProcessor(ctx context.Context) {
	repository := repositories.NewDynamicRepository()
	events := dynamicevents.NewDynamicEvents()
	observability.InfoCtx(ctx, "dynamic outbox processor started",
		slog.String(observability.FieldOperation, "outbox_processor_start"),
		slog.String(observability.FieldStatus, "started"))
	if err := repository.EnsureOutboxIndexes(ctx); err != nil {
		observability.ErrorCtx(ctx, "dynamic outbox index setup failed", err,
			slog.String(observability.FieldOperation, "ensure_indexes"),
			slog.String(observability.FieldStatus, "error"))
	}
	ticker := time.NewTicker(dynamicOutboxPollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processDynamicOutboxBatch(ctx, repository, events, 25)
		}
	}
}

func processDynamicOutboxBatch(ctx context.Context, repository *repositories.DynamicRepository, events modelsOutboxEvents, limit int) {
	for i := 0; i < limit; i++ {
		event, err := repository.ClaimPendingOutboxEvent(ctx, time.Now())
		if errors.Is(err, mongo.ErrNoDocuments) {
			if i == 0 {
				observability.DebugCtx(ctx, "dynamic outbox batch idle",
					slog.String(observability.FieldOperation, "claim"),
					slog.String(observability.FieldStatus, "idle"))
			}
			return
		}
		if err != nil {
			observability.RecordOutboxJob("claim", "error")
			observability.ErrorCtx(ctx, "dynamic outbox claim failed", err,
				slog.String(observability.FieldOperation, "claim"),
				slog.String(observability.FieldStatus, "error"))
			return
		}

		start := time.Now()
		observability.InfoCtx(ctx, "dynamic outbox event claimed",
			append(observability.TenantProjectAttrs(event.TenantID, event.ProjectID),
				slog.String(observability.FieldSchemaName, event.SchemaName),
				slog.String(observability.FieldOperation, event.Operation),
				slog.String("outbox_event_id", event.ID.Hex()),
				slog.String("workflow_name", event.Payload.WorkflowName),
				slog.String("step_name", event.Payload.StepName),
				slog.Int("attempt", event.Attempts+1),
			)...,
		)
		if err := dispatchDynamicOutboxEvent(ctx, repository, event, events); err != nil {
			observability.RecordOutboxJob(event.Operation, "failed")
			attrs := append(observability.TenantProjectAttrs(event.TenantID, event.ProjectID),
				slog.String(observability.FieldSchemaName, event.SchemaName),
				slog.String(observability.FieldOperation, event.Operation),
				slog.String(observability.FieldStatus, "failed"),
				slog.Float64(observability.FieldDurationMS, observability.DurationMS(start)))
			observability.ErrorCtx(ctx, "dynamic outbox dispatch failed", err, attrs...)
			if markErr := repository.MarkOutboxEventFailed(ctx, *event, err.Error(), outboxRetryDelay(event.Attempts)); markErr != nil {
				observability.RecordOutboxJob("mark_failed", "error")
				observability.ErrorCtx(ctx, "dynamic outbox mark failed failed", markErr, attrs...)
			}
			continue
		}

		if err := repository.MarkOutboxEventDone(ctx, event.ID); err != nil {
			observability.RecordOutboxJob("mark_done", "error")
			observability.ErrorCtx(ctx, "dynamic outbox mark done failed", err,
				slog.String(observability.FieldOperation, "mark_done"),
				slog.String(observability.FieldStatus, "error"))
			continue
		}
		observability.RecordOutboxJob(event.Operation, "done")
		observability.InfoCtx(ctx, "dynamic outbox event done",
			append(observability.TenantProjectAttrs(event.TenantID, event.ProjectID),
				slog.String(observability.FieldSchemaName, event.SchemaName),
				slog.String(observability.FieldOperation, event.Operation),
				slog.String("outbox_event_id", event.ID.Hex()),
				slog.String("workflow_name", event.Payload.WorkflowName),
				slog.String("step_name", event.Payload.StepName),
				slog.Float64(observability.FieldDurationMS, observability.DurationMS(start)),
			)...,
		)
	}
}

type modelsOutboxEvents interface {
	EmitInvalidate(schemaName, userID, tenantID, projectID string, eventID ...string)
}

func dispatchDynamicOutboxEvent(ctx context.Context, repository *repositories.DynamicRepository, event *models.DynamicOutboxEvent, events modelsOutboxEvents) error {
	if event.Operation == models.DynamicOutboxOperationWorkflowStep {
		return processWorkflowOutboxStep(ctx, repository, event)
	}

	if event.Payload.AuditLog != nil {
		if err := utils.LogAudit(ctx, *event.Payload.AuditLog); err != nil {
			return err
		}
	}

	for _, schemaName := range uniqueSchemaNames(event.Payload.InvalidateSchemas) {
		events.EmitInvalidate(schemaName, event.Payload.UserID, event.TenantID, event.ProjectID, event.ID.Hex())
	}

	return nil
}

func (s *DynamicService) insertDynamicPostWrite(ctx context.Context, tenantID, projectID, schemaName, operation, userID string, container *models.ContainerModel, auditLog *models.AuditLog) error {
	event := buildDynamicPostWriteEvent(tenantID, projectID, schemaName, operation, userID, container, auditLog)
	if _, err := s.repository.InsertOutboxEvent(ctx, event); err != nil {
		return err
	}
	return utils.InvalidateSchemaAndTriggeredCaches(ctx, tenantID, projectID, schemaName, event.Payload.InvalidateSchemas)
}

func buildDynamicPostWriteEvent(tenantID, projectID, schemaName, operation, userID string, container *models.ContainerModel, auditLog *models.AuditLog) models.DynamicOutboxEvent {
	now := time.Now()
	eventID := primitive.NewObjectID()
	if auditLog != nil {
		auditLog.EventID = eventID
	}

	return models.DynamicOutboxEvent{
		ID:            eventID,
		TenantID:      tenantID,
		ProjectID:     projectID,
		SchemaName:    schemaName,
		Operation:     operation,
		Status:        models.DynamicOutboxStatusPending,
		MaxAttempts:   dynamicOutboxMaxAttempts,
		NextAttemptAt: primitive.NewDateTimeFromTime(now),
		CreatedAt:     primitive.NewDateTimeFromTime(now),
		UpdatedAt:     primitive.NewDateTimeFromTime(now),
		Payload: models.DynamicOutboxPayload{
			AuditLog:          auditLog,
			InvalidateSchemas: writeInvalidateSchemas(schemaName, container),
			UserID:            userID,
		},
	}
}

func buildDynamicAuditLog(tenantID, projectID, schemaName, action string, user *models.AuditUser, beforeDoc, afterDoc interface{}) *models.AuditLog {
	auditLog := &models.AuditLog{
		TenantID:    tenantID,
		ProjectID:   projectID,
		SchemaName:  schemaName,
		Action:      action,
		DocumentIDs: auditDocumentIDs(beforeDoc, afterDoc),
		Before:      beforeDoc,
		After:       afterDoc,
		Timestamp:   primitive.NewDateTimeFromTime(time.Now()),
	}

	if user != nil {
		auditLog.UserID = user.ID
		auditLog.UserEmail = user.Email
		auditLog.UserDisplayName = user.DisplayName
		auditLog.Roles = user.Roles
	}

	return auditLog
}

func auditDocumentIDs(docs ...interface{}) []primitive.ObjectID {
	seen := map[primitive.ObjectID]struct{}{}
	var ids []primitive.ObjectID
	for _, doc := range docs {
		switch value := doc.(type) {
		case []interface{}:
			for _, item := range value {
				ids = appendObjectID(ids, seen, auditDocumentID(item))
			}
		default:
			ids = appendObjectID(ids, seen, auditDocumentID(value))
		}
	}
	return ids
}

func appendObjectID(ids []primitive.ObjectID, seen map[primitive.ObjectID]struct{}, id primitive.ObjectID) []primitive.ObjectID {
	if id == primitive.NilObjectID {
		return ids
	}
	if _, ok := seen[id]; ok {
		return ids
	}
	seen[id] = struct{}{}
	return append(ids, id)
}

func auditDocumentID(doc interface{}) primitive.ObjectID {
	if doc == nil {
		return primitive.NilObjectID
	}

	var id interface{}
	switch value := doc.(type) {
	case map[string]interface{}:
		id = value["_id"]
	case bson.M:
		id = value["_id"]
	}

	switch value := id.(type) {
	case primitive.ObjectID:
		return value
	case string:
		objectID, err := primitive.ObjectIDFromHex(value)
		if err == nil {
			return objectID
		}
	}
	return primitive.NilObjectID
}

func writeInvalidateSchemas(schemaName string, container *models.ContainerModel) []string {
	schemas := []string{schemaName}
	if container != nil {
		schemas = append(schemas, container.Redis.TriggeredRedisCaches...)
	}
	return uniqueSchemaNames(schemas)
}

func uniqueSchemaNames(schemaNames []string) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, schemaName := range schemaNames {
		schemaName = strings.TrimSpace(schemaName)
		if schemaName == "" {
			continue
		}
		if _, ok := seen[schemaName]; ok {
			continue
		}
		seen[schemaName] = struct{}{}
		result = append(result, schemaName)
	}
	return result
}

func outboxRetryDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := time.Duration(attempts*attempts) * time.Second
	if delay > time.Minute {
		return time.Minute
	}
	return delay
}
