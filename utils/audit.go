package utils

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GetUserFromContext extracts user info from Fiber context.
func GetUserFromContext(c *fiber.Ctx) *models.User {
	userIDStr, ok := c.Locals("userID").(string)
	if !ok || userIDStr == "" {
		return nil
	}

	role, _ := c.Locals("userRole").(string)
    roles := []string{}
    if role != "" {
        roles = []string{role}
    }

	userID, err := primitive.ObjectIDFromHex(userIDStr)
	if err != nil {
		// Log error or handle if userID is not ObjectID (though it should be based on other code)
        // If it's not a valid ObjectID, we can't store it in the ObjectID field.
        // But let's assume it is valid as per system design.
        log.Printf("Warning: UserID in context is not a valid ObjectID: %s", userIDStr)
		return nil
	}

	return &models.User{
		ID:    userID,
		Roles: roles,
        // Email is not in context locals currently, so we leave it empty.
        // It might be fetched if needed, but for now we stick to what we have efficiently.
	}
}

// LogAudit writes an AuditLog to the audit_logs collection.
func LogAudit(ctx context.Context, auditLog models.AuditLog) error {
	// Ensure timestamp is set
	if auditLog.Timestamp == 0 {
		auditLog.Timestamp = primitive.NewDateTimeFromTime(time.Now())
	}

	collection := configs.GetCollection("audit_logs")
	_, err := collection.InsertOne(ctx, auditLog)
	if err != nil {
		log.Printf("Failed to insert audit log: %v", err)
		return err
	}
	return nil
}

// extractDocumentID attempts to extract the _id from a bson.M or map[string]interface{}
func extractDocumentID(doc interface{}) primitive.ObjectID {
	if doc == nil {
		return primitive.NilObjectID
	}

	var idInterface interface{}
	switch v := doc.(type) {
	case bson.M:
		idInterface = v["_id"]
	case map[string]interface{}:
		idInterface = v["_id"]
	}

	if id, ok := idInterface.(primitive.ObjectID); ok {
		return id
	}
	if idStr, ok := idInterface.(string); ok {
		if oid, err := primitive.ObjectIDFromHex(idStr); err == nil {
			return oid
		}
	}
	return primitive.NilObjectID
}

// LogCreateAction logs a single document creation.
func LogCreateAction(ctx context.Context, container *models.ContainerModel, user *models.User, createdDoc interface{}) error {
	if container == nil {
		return nil
	}
	
	docID := extractDocumentID(createdDoc)
	var docIDs []primitive.ObjectID
	if docID != primitive.NilObjectID {
		docIDs = []primitive.ObjectID{docID}
	}

	auditLog := models.AuditLog{
		SchemaName:  container.SchemaName,
		Action:      "create",
		DocumentIDs: docIDs,
		After:       createdDoc,
	}

	if user != nil {
		auditLog.UserID = user.ID
		auditLog.UserEmail = user.Email
		auditLog.Roles = user.Roles
	}

	return LogAudit(ctx, auditLog)
}

// LogUpdateAction logs a single document update.
func LogUpdateAction(ctx context.Context, container *models.ContainerModel, user *models.User, beforeDoc, afterDoc interface{}) error {
	if container == nil {
		return nil
	}

	docID := extractDocumentID(afterDoc)
	if docID == primitive.NilObjectID {
		docID = extractDocumentID(beforeDoc)
	}

	var docIDs []primitive.ObjectID
	if docID != primitive.NilObjectID {
		docIDs = []primitive.ObjectID{docID}
	}

	auditLog := models.AuditLog{
		SchemaName:  container.SchemaName,
		Action:      "update",
		DocumentIDs: docIDs,
		Before:      beforeDoc,
		After:       afterDoc,
	}

	if user != nil {
		auditLog.UserID = user.ID
		auditLog.UserEmail = user.Email
		auditLog.Roles = user.Roles
	}

	return LogAudit(ctx, auditLog)
}

// LogDeleteAction logs a single document deletion.
func LogDeleteAction(ctx context.Context, container *models.ContainerModel, user *models.User, deletedDoc interface{}) error {
	if container == nil {
		return nil
	}

	docID := extractDocumentID(deletedDoc)
	var docIDs []primitive.ObjectID
	if docID != primitive.NilObjectID {
		docIDs = []primitive.ObjectID{docID}
	}

	auditLog := models.AuditLog{
		SchemaName:  container.SchemaName,
		Action:      "delete",
		DocumentIDs: docIDs,
		Before:      deletedDoc,
	}

	if user != nil {
		auditLog.UserID = user.ID
		auditLog.UserEmail = user.Email
		auditLog.Roles = user.Roles
	}

	return LogAudit(ctx, auditLog)
}

// LogBulkCreateAction logs bulk document creation.
func LogBulkCreateAction(ctx context.Context, container *models.ContainerModel, user *models.User, createdDocs []interface{}) error {
	if container == nil || len(createdDocs) == 0 {
		return nil
	}

	var docIDs []primitive.ObjectID
	for _, doc := range createdDocs {
		if id := extractDocumentID(doc); id != primitive.NilObjectID {
			docIDs = append(docIDs, id)
		}
	}

	auditLog := models.AuditLog{
		SchemaName:  container.SchemaName,
		Action:      "bulk_create",
		DocumentIDs: docIDs,
		After:       createdDocs,
	}

	if user != nil {
		auditLog.UserID = user.ID
		auditLog.UserEmail = user.Email
		auditLog.Roles = user.Roles
	}

	return LogAudit(ctx, auditLog)
}

// LogBulkUpdateAction logs bulk document updates.
func LogBulkUpdateAction(ctx context.Context, container *models.ContainerModel, user *models.User, beforeDocs, afterDocs []interface{}) error {
	if container == nil {
		return nil
	}

	var docIDs []primitive.ObjectID
	for _, doc := range afterDocs {
		if id := extractDocumentID(doc); id != primitive.NilObjectID {
			docIDs = append(docIDs, id)
		}
	}

	auditLog := models.AuditLog{
		SchemaName:  container.SchemaName,
		Action:      "bulk_update",
		DocumentIDs: docIDs,
		Before:      beforeDocs,
		After:       afterDocs,
	}

	if user != nil {
		auditLog.UserID = user.ID
		auditLog.UserEmail = user.Email
		auditLog.Roles = user.Roles
	}

	return LogAudit(ctx, auditLog)
}

// LogBulkDeleteAction logs bulk document deletions.
func LogBulkDeleteAction(ctx context.Context, container *models.ContainerModel, user *models.User, deletedDocs []interface{}) error {
	if container == nil || len(deletedDocs) == 0 {
		return nil
	}

	var docIDs []primitive.ObjectID
	for _, doc := range deletedDocs {
		if id := extractDocumentID(doc); id != primitive.NilObjectID {
			docIDs = append(docIDs, id)
		}
	}

	auditLog := models.AuditLog{
		SchemaName:  container.SchemaName,
		Action:      "bulk_delete",
		DocumentIDs: docIDs,
		Before:      deletedDocs,
	}

	if user != nil {
		auditLog.UserID = user.ID
		auditLog.UserEmail = user.Email
		auditLog.Roles = user.Roles
	}

	return LogAudit(ctx, auditLog)
}

// LogLogin logs a user login event.
func LogLogin(ctx context.Context, user *models.User, ip, userAgent string) error {
	auditLog := models.AuditLog{
		Action:    "login",
		IP:        ip,
		UserAgent: userAgent,
	}

	if user != nil {
		auditLog.UserID = user.ID
		auditLog.UserEmail = user.Email
		auditLog.Roles = user.Roles
	}

	return LogAudit(ctx, auditLog)
}

// LogLogout logs a user logout event.
func LogLogout(ctx context.Context, user *models.User, ip, userAgent string) error {
	auditLog := models.AuditLog{
		Action:    "logout",
		IP:        ip,
		UserAgent: userAgent,
	}

	if user != nil {
		auditLog.UserID = user.ID
		auditLog.UserEmail = user.Email
		auditLog.Roles = user.Roles
	}

	return LogAudit(ctx, auditLog)
}
