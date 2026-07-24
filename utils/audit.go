package utils

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var auditLogIndexCollections sync.Map

// GetUserFromContext extracts user info from Fiber context.
func GetUserFromContext(c *fiber.Ctx) *models.AuditUser {
	userIDStr, ok := c.Locals("userID").(string)
	if !ok || userIDStr == "" {
		return nil
	}

	role, _ := c.Locals("userRole").(string)
	roles := []string{}
	if role != "" {
		roles = []string{role}
	}
	displayName, _ := c.Locals("userDisplayName").(string)

	userID, err := primitive.ObjectIDFromHex(userIDStr)
	if err != nil {
		// Log error or handle if userID is not ObjectID (though it should be based on other code)
		// If it's not a valid ObjectID, we can't store it in the ObjectID field.
		// But let's assume it is valid as per system design.
		log.Printf("Warning: UserID in context is not a valid ObjectID: %s", userIDStr)
		return nil
	}

	return &models.AuditUser{
		ID:          userID,
		DisplayName: displayName,
		Roles:       roles,
		// Email is not in context locals currently, so we leave it empty.
		// It might be fetched if needed, but for now we stick to what we have efficiently.
	}
}

func applyAuditUser(auditLog *models.AuditLog, user *models.AuditUser) {
	if auditLog == nil || user == nil {
		return
	}
	auditLog.UserID = user.ID
	auditLog.UserEmail = user.Email
	auditLog.UserDisplayName = user.DisplayName
	auditLog.Roles = user.Roles
}

// LogAudit writes an AuditLog to the audit_logs collection.
func LogAudit(ctx context.Context, auditLog models.AuditLog) error {
	// Ensure timestamp is set
	if auditLog.Timestamp == 0 {
		auditLog.Timestamp = primitive.NewDateTimeFromTime(time.Now())
	}

	// Use project-specific audit_logs collection
	collection := projectCollectionProvider(auditLog.TenantID, auditLog.ProjectID, "audit_logs")
	if err := ensureAuditLogIndexesForCollection(ctx, collection); err != nil {
		log.Printf("Failed to ensure audit log indexes: %v", err)
		return err
	}

	if auditLog.EventID != primitive.NilObjectID {
		_, err := collection.UpdateOne(
			ctx,
			bson.M{"eventId": auditLog.EventID},
			bson.M{"$setOnInsert": auditLog},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			log.Printf("Failed to upsert audit log: %v", err)
			return err
		}
		return nil
	}

	_, err := collection.InsertOne(ctx, auditLog)
	if err != nil {
		log.Printf("Failed to insert audit log: %v", err)
		return err
	}
	return nil
}

func EnsureAuditLogIndexes(ctx context.Context, tenantID, projectID string) error {
	return ensureAuditLogIndexesForCollection(ctx, projectCollectionProvider(tenantID, projectID, "audit_logs"))
}

func ensureAuditLogIndexesForCollection(ctx context.Context, collection *mongo.Collection) error {
	indexKey := collection.Name()
	if _, loaded := auditLogIndexCollections.LoadOrStore(indexKey, struct{}{}); loaded {
		return nil
	}

	_, err := collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "timestamp", Value: 1}},
			Options: options.Index().
				SetName("idx_audit_logs_ttl").
				SetExpireAfterSeconds(configs.GetAuditLogRetentionSeconds()).
				SetBackground(true),
		},
		{
			Keys: bson.D{
				{Key: "tenantId", Value: 1},
				{Key: "projectId", Value: 1},
				{Key: "schemaName", Value: 1},
				{Key: "timestamp", Value: -1},
			},
			Options: options.Index().
				SetName("idx_audit_logs_scope").
				SetBackground(true),
		},
		{
			Keys: bson.D{{Key: "eventId", Value: 1}},
			Options: options.Index().
				SetName("idx_audit_event_id_unique").
				SetUnique(true).
				SetSparse(true).
				SetBackground(true),
		},
	})
	if err != nil && !isIndexExistsError(err) {
		auditLogIndexCollections.Delete(indexKey)
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
func LogCreateAction(ctx context.Context, tenantID, projectID string, container *models.ContainerModel, user *models.AuditUser, createdDoc interface{}) error {
	if container == nil {
		return nil
	}

	docID := extractDocumentID(createdDoc)
	var docIDs []primitive.ObjectID
	if docID != primitive.NilObjectID {
		docIDs = []primitive.ObjectID{docID}
	}

	auditLog := models.AuditLog{
		TenantID:    tenantID,
		ProjectID:   projectID,
		SchemaName:  container.SchemaName,
		Action:      "create",
		DocumentIDs: docIDs,
		After:       createdDoc,
	}

	applyAuditUser(&auditLog, user)

	return LogAudit(ctx, auditLog)
}

// LogUpdateAction logs a single document update.
func LogUpdateAction(ctx context.Context, tenantID, projectID string, container *models.ContainerModel, user *models.AuditUser, beforeDoc, afterDoc interface{}) error {
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
		TenantID:    tenantID,
		ProjectID:   projectID,
		SchemaName:  container.SchemaName,
		Action:      "update",
		DocumentIDs: docIDs,
		Before:      beforeDoc,
		After:       afterDoc,
	}

	applyAuditUser(&auditLog, user)

	return LogAudit(ctx, auditLog)
}

// LogDeleteAction logs a single document deletion.
func LogDeleteAction(ctx context.Context, tenantID, projectID string, container *models.ContainerModel, user *models.AuditUser, deletedDoc interface{}) error {
	if container == nil {
		return nil
	}

	docID := extractDocumentID(deletedDoc)
	var docIDs []primitive.ObjectID
	if docID != primitive.NilObjectID {
		docIDs = []primitive.ObjectID{docID}
	}

	auditLog := models.AuditLog{
		TenantID:    tenantID,
		ProjectID:   projectID,
		SchemaName:  container.SchemaName,
		Action:      "delete",
		DocumentIDs: docIDs,
		Before:      deletedDoc,
	}

	applyAuditUser(&auditLog, user)

	return LogAudit(ctx, auditLog)
}

// LogBulkCreateAction logs bulk document creation.
func LogBulkCreateAction(ctx context.Context, tenantID, projectID string, container *models.ContainerModel, user *models.AuditUser, createdDocs []interface{}) error {
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
		TenantID:    tenantID,
		ProjectID:   projectID,
		SchemaName:  container.SchemaName,
		Action:      "bulk_create",
		DocumentIDs: docIDs,
		After:       createdDocs,
	}

	applyAuditUser(&auditLog, user)

	return LogAudit(ctx, auditLog)
}

// LogBulkUpdateAction logs bulk document updates.
func LogBulkUpdateAction(ctx context.Context, tenantID, projectID string, container *models.ContainerModel, user *models.AuditUser, beforeDocs, afterDocs []interface{}) error {
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
		TenantID:    tenantID,
		ProjectID:   projectID,
		SchemaName:  container.SchemaName,
		Action:      "bulk_update",
		DocumentIDs: docIDs,
		Before:      beforeDocs,
		After:       afterDocs,
	}

	applyAuditUser(&auditLog, user)

	return LogAudit(ctx, auditLog)
}

// LogBulkDeleteAction logs bulk document deletions.
func LogBulkDeleteAction(ctx context.Context, tenantID, projectID string, container *models.ContainerModel, user *models.AuditUser, deletedDocs []interface{}) error {
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
		TenantID:    tenantID,
		ProjectID:   projectID,
		SchemaName:  container.SchemaName,
		Action:      "bulk_delete",
		DocumentIDs: docIDs,
		Before:      deletedDocs,
	}

	applyAuditUser(&auditLog, user)

	return LogAudit(ctx, auditLog)
}

// LogLogin logs a user login event.
func LogLogin(ctx context.Context, tenantID, projectID string, user *models.AuditUser, ip, userAgent string) error {
	auditLog := models.AuditLog{
		TenantID:  tenantID,
		ProjectID: projectID,
		Action:    "login",
		IP:        ip,
		UserAgent: userAgent,
	}

	applyAuditUser(&auditLog, user)

	return LogAudit(ctx, auditLog)
}

// LogLogout logs a user logout event.
func LogLogout(ctx context.Context, tenantID, projectID string, user *models.AuditUser, ip, userAgent string) error {
	auditLog := models.AuditLog{
		TenantID:  tenantID,
		ProjectID: projectID,
		Action:    "logout",
		IP:        ip,
		UserAgent: userAgent,
	}

	applyAuditUser(&auditLog, user)

	return LogAudit(ctx, auditLog)
}

// BuildAuditLogFilter builds a MongoDB filter from query parameters for audit logs.
// Supports filtering by: action, schemaName, userEmail, userId, startDate, endDate
func BuildAuditLogFilter(c *fiber.Ctx) (bson.M, error) {
	filter := bson.M{}

	// Extract tenant and project context from URL slugs (falls back to query params or JWT for backward compatibility)
	tenantID, projectID, err := GetTenantAndProjectContext(c)
	if err != nil {
		return nil, err
	}

	// Add tenant and project filters to ensure isolation
	if tenantID != "" {
		filter["tenantId"] = tenantID
	}
	if projectID != "" {
		filter["projectId"] = projectID
	}

	// Filter by action (e.g., "create", "update", "delete", "login", etc.)
	if action := c.Query("action"); action != "" {
		filter["action"] = action
	}

	// Filter by schemaName
	if schemaName := c.Query("schemaName"); schemaName != "" {
		filter["schemaName"] = schemaName
	}

	// Filter by userEmail
	if userEmail := c.Query("userEmail"); userEmail != "" {
		filter["userEmail"] = userEmail
	}

	if userDisplayName := c.Query("userDisplayName"); userDisplayName != "" {
		filter["userDisplayName"] = userDisplayName
	}

	// Filter by userId
	if userIdStr := c.Query("userId"); userIdStr != "" {
		userId, err := primitive.ObjectIDFromHex(userIdStr)
		if err != nil {
			return nil, err
		}
		filter["userId"] = userId
	}

	// Filter by documentId
	if docIdStr := c.Query("documentId"); docIdStr != "" {
		docId, err := primitive.ObjectIDFromHex(docIdStr)
		if err != nil {
			return nil, err
		}
		filter["documentIds"] = bson.M{"$in": []primitive.ObjectID{docId}}
	}

	// Filter by date range (startDate and endDate in RFC3339 format or similar)
	startDateStr := c.Query("startDate")
	endDateStr := c.Query("endDate")

	if startDateStr != "" || endDateStr != "" {
		dateFilter := bson.M{}

		if startDateStr != "" {
			startTime, err := time.Parse(time.RFC3339, startDateStr)
			if err != nil {
				return nil, err
			}
			dateFilter["$gte"] = primitive.NewDateTimeFromTime(startTime)
		}

		if endDateStr != "" {
			endTime, err := time.Parse(time.RFC3339, endDateStr)
			if err != nil {
				return nil, err
			}
			dateFilter["$lte"] = primitive.NewDateTimeFromTime(endTime)
		}

		filter["timestamp"] = dateFilter
	}

	// Filter by IP address
	if ip := c.Query("ip"); ip != "" {
		filter["ip"] = ip
	}

	// Filter by roles (supports multiple roles)
	if role := c.Query("role"); role != "" {
		filter["roles"] = bson.M{"$in": []string{role}}
	}

	return filter, nil
}

// GetAuditLogs retrieves audit logs with pagination, filtering, and sorting.
func GetAuditLogs(ctx context.Context, c *fiber.Ctx) ([]bson.M, *Pager, error) {
	// Build filter from query parameters
	filter, err := BuildAuditLogFilter(c)
	if err != nil {
		return nil, nil, err
	}

	// Parse pagination
	pager, err := ParsePager(c)
	if err != nil {
		return nil, nil, err
	}

	// Parse sorting (default: sort by timestamp descending)
	sortField := c.Query("sort", "timestamp")
	ascStr := c.Query("asc", "false")
	asc := ascStr == "true"

	sortDir := int32(-1) // Default descending
	if asc {
		sortDir = 1
	}
	sort := bson.D{{Key: sortField, Value: sortDir}}

	// Extract tenant and project context from URL slugs (falls back to query params or JWT for backward compatibility)
	tenantID, projectID, err := GetTenantAndProjectContext(c)
	if err != nil {
		return nil, nil, err
	}

	// Get project-specific audit_logs collection
	collection := projectCollectionProvider(tenantID, projectID, "audit_logs")

	// Build find options
	opts := options.Find().SetSort(sort)

	if pager.Enabled {
		// Get total count for pagination
		totalItems, err := collection.CountDocuments(ctx, filter)
		if err != nil {
			return nil, nil, err
		}
		pager.TotalItems = totalItems
		pager.TotalPages = int((totalItems + int64(pager.Limit) - 1) / int64(pager.Limit))

		opts.SetSkip(pager.Skip)
		opts.SetLimit(int64(pager.Limit))
	}

	// Execute query
	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close(ctx)

	// Decode results
	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, nil, err
	}

	if pager.Enabled {
		return results, &pager, nil
	}
	return results, nil, nil
}
