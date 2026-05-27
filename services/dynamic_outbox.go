package services

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	dynamicevents "github.com/osmansam/autotableGo/events"
	"github.com/osmansam/autotableGo/models"
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
	if err := repository.EnsureOutboxIndexes(ctx); err != nil {
		log.Printf("dynamic outbox: failed to ensure indexes: %v", err)
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
			return
		}
		if err != nil {
			log.Printf("dynamic outbox: failed to claim event: %v", err)
			return
		}

		if err := dispatchDynamicOutboxEvent(ctx, event, events); err != nil {
			log.Printf("dynamic outbox: failed to dispatch event %s: %v", event.ID.Hex(), err)
			if markErr := repository.MarkOutboxEventFailed(ctx, *event, err.Error(), outboxRetryDelay(event.Attempts)); markErr != nil {
				log.Printf("dynamic outbox: failed to mark event %s failed: %v", event.ID.Hex(), markErr)
			}
			continue
		}

		if err := repository.MarkOutboxEventDone(ctx, event.ID); err != nil {
			log.Printf("dynamic outbox: failed to mark event %s done: %v", event.ID.Hex(), err)
		}
	}
}

type modelsOutboxEvents interface {
	EmitInvalidate(schemaName, userID, tenantID, projectID string, eventID ...string)
}

func dispatchDynamicOutboxEvent(ctx context.Context, event *models.DynamicOutboxEvent, events modelsOutboxEvents) error {
	if event.Payload.AuditLog != nil {
		if err := utils.LogAudit(ctx, *event.Payload.AuditLog); err != nil {
			return err
		}
	}

	for _, schemaName := range uniqueSchemaNames(event.Payload.InvalidateSchemas) {
		if err := utils.IncrementSchemaCacheVersion(ctx, event.TenantID, event.ProjectID, schemaName); err != nil {
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
	_, err := s.repository.InsertOutboxEvent(ctx, event)
	return err
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
