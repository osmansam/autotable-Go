package repositories

import (
	"context"
	"regexp"
	"time"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/observability"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

var projectContainersCollectionPattern = regexp.MustCompile(`^tenant_(.+)_project_(.+)_containers$`)

type ScheduledWorkflowContainer struct {
	TenantID  string
	ProjectID string
	Container models.ContainerModel
}

type DynamicRepository struct {
	collection         func(tenantID, projectID, schemaName string) *mongo.Collection
	globalCollection   func(collectionName string) *mongo.Collection
	getContainerModel  func(context.Context, string, string, string) (*models.ContainerModel, error)
	getContainerModels func(context.Context) ([]models.ContainerModel, error)
	nextSequence       func(context.Context, string) (int64, error)
	withTransaction    func(context.Context, func(mongo.SessionContext) error) error
}

func NewDynamicRepository() *DynamicRepository {
	return &DynamicRepository{
		collection:         utils.GetDynamicCollectionForProject,
		globalCollection:   configs.GetCollection,
		getContainerModel:  utils.GetContainerModelWithContext,
		getContainerModels: utils.GetAllContainerModelsWithContext,
		nextSequence:       utils.GetNextSequence,
	}
}

func (r *DynamicRepository) GetContainerModel(ctx context.Context, tenantID, projectID, schemaName string) (*models.ContainerModel, error) {
	return r.getContainerModel(ctx, tenantID, projectID, schemaName)
}

func (r *DynamicRepository) GetAllContainerModels(ctx context.Context) ([]models.ContainerModel, error) {
	return r.getContainerModels(ctx)
}

func (r *DynamicRepository) GetScheduledWorkflowContainers(ctx context.Context) ([]ScheduledWorkflowContainer, error) {
	var result []ScheduledWorkflowContainer

	legacyContainers, err := r.GetAllContainerModels(ctx)
	if err != nil {
		return nil, err
	}
	for _, container := range legacyContainers {
		if containerHasCronWorkflow(container) {
			result = append(result, ScheduledWorkflowContainer{Container: container})
		}
	}

	names, err := r.globalCollection("containers").Database().ListCollectionNames(ctx, bson.M{"name": bson.M{"$regex": "^tenant_.+_project_.+_containers$"}})
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		matches := projectContainersCollectionPattern.FindStringSubmatch(name)
		if len(matches) != 3 {
			continue
		}
		tenantID, projectID := matches[1], matches[2]
		cursor, err := r.globalCollection(name).Find(ctx, bson.M{"workflows.trigger": models.WorkflowTriggerCron})
		if err != nil {
			return nil, err
		}
		for cursor.Next(ctx) {
			var container models.ContainerModel
			if err := cursor.Decode(&container); err != nil {
				cursor.Close(ctx)
				return nil, err
			}
			if containerHasCronWorkflow(container) {
				result = append(result, ScheduledWorkflowContainer{
					TenantID:  tenantID,
					ProjectID: projectID,
					Container: container,
				})
			}
		}
		if err := cursor.Err(); err != nil {
			cursor.Close(ctx)
			return nil, err
		}
		cursor.Close(ctx)
	}

	return result, nil
}

func containerHasCronWorkflow(container models.ContainerModel) bool {
	for _, workflow := range container.Workflows {
		if workflow.IsActive && workflow.Trigger == models.WorkflowTriggerCron {
			return true
		}
	}
	return false
}

func (r *DynamicRepository) GetCollection(tenantID, projectID, schemaName string) *mongo.Collection {
	return r.collection(tenantID, projectID, schemaName)
}

func (r *DynamicRepository) GetProjectCollection(tenantID, projectID, collectionName string) *mongo.Collection {
	if tenantID == "" || projectID == "" {
		return r.globalCollection(collectionName)
	}
	return utils.GetProjectCollection(tenantID, projectID, collectionName)
}

func (r *DynamicRepository) externalAPICredentialsCollection() *mongo.Collection {
	return r.globalCollection("external_api_credentials")
}

func (r *DynamicRepository) InsertExternalAPICredential(ctx context.Context, credential models.ExternalAPICredential) (*mongo.InsertOneResult, error) {
	return r.externalAPICredentialsCollection().InsertOne(ctx, credential)
}

func (r *DynamicRepository) ListExternalAPICredentials(ctx context.Context, tenantID, projectID string) ([]models.ExternalAPICredential, error) {
	tenantObjID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return nil, err
	}
	projectObjID, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return nil, err
	}
	cursor, err := r.externalAPICredentialsCollection().Find(ctx, bson.M{"tenantId": tenantObjID, "projectId": projectObjID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	credentials := make([]models.ExternalAPICredential, 0)
	if err := cursor.All(ctx, &credentials); err != nil {
		return nil, err
	}
	return credentials, nil
}

func (r *DynamicRepository) GetExternalAPICredential(ctx context.Context, tenantID, projectID, credentialID string) (*models.ExternalAPICredential, error) {
	credentialObjID, err := primitive.ObjectIDFromHex(credentialID)
	if err != nil {
		return nil, err
	}
	tenantObjID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return nil, err
	}
	projectObjID, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return nil, err
	}
	var credential models.ExternalAPICredential
	err = r.externalAPICredentialsCollection().FindOne(ctx, bson.M{
		"_id":       credentialObjID,
		"tenantId":  tenantObjID,
		"projectId": projectObjID,
	}).Decode(&credential)
	if err != nil {
		return nil, err
	}
	return &credential, nil
}

func (r *DynamicRepository) RevokeExternalAPICredential(ctx context.Context, tenantID, projectID, credentialID string, revokedAt time.Time) (*mongo.UpdateResult, error) {
	credentialObjID, err := primitive.ObjectIDFromHex(credentialID)
	if err != nil {
		return nil, err
	}
	tenantObjID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return nil, err
	}
	projectObjID, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return nil, err
	}
	return r.externalAPICredentialsCollection().UpdateOne(ctx,
		bson.M{"_id": credentialObjID, "tenantId": tenantObjID, "projectId": projectObjID},
		bson.M{"$set": bson.M{"revokedAt": revokedAt, "updatedAt": revokedAt}},
	)
}

func (r *DynamicRepository) MarkExternalAPICredentialUsed(ctx context.Context, credentialID primitive.ObjectID, usedAt time.Time) error {
	_, err := r.externalAPICredentialsCollection().UpdateByID(ctx, credentialID, bson.M{"$set": bson.M{"lastUsedAt": usedAt, "updatedAt": usedAt}})
	return err
}

func (r *DynamicRepository) InsertNotification(ctx context.Context, notification models.DynamicNotification) (*mongo.InsertOneResult, error) {
	return r.GetProjectCollection(notification.TenantID, notification.ProjectID, "notifications").InsertOne(ctx, notification)
}

func (r *DynamicRepository) QueryNotifications(ctx context.Context, tenantID, projectID string, filter bson.M, opts *options.FindOptions, pager *utils.Pager) ([]map[string]interface{}, error) {
	collection := r.GetProjectCollection(tenantID, projectID, "notifications")
	return utils.QueryAndDecodeCollection(ctx, collection, "notifications", filter, opts, pager)
}

func (r *DynamicRepository) MarkNotificationRead(ctx context.Context, tenantID, projectID string, notificationID primitive.ObjectID, userID string) (*mongo.UpdateResult, error) {
	return r.GetProjectCollection(tenantID, projectID, "notifications").UpdateOne(ctx, bson.M{
		"_id":       notificationID,
		"tenantId":  tenantID,
		"projectId": projectID,
		"isActive":  true,
	}, bson.M{"$addToSet": bson.M{"seenBy": userID}})
}

func (r *DynamicRepository) MarkNotificationsRead(ctx context.Context, tenantID, projectID, userID string, filter bson.M) (*mongo.UpdateResult, error) {
	filter["tenantId"] = tenantID
	filter["projectId"] = projectID
	filter["isActive"] = true
	return r.GetProjectCollection(tenantID, projectID, "notifications").UpdateMany(ctx, filter, bson.M{"$addToSet": bson.M{"seenBy": userID}})
}

func (r *DynamicRepository) DeleteNotificationForUser(ctx context.Context, tenantID, projectID string, notificationID primitive.ObjectID, userID string) (*mongo.UpdateResult, error) {
	return r.GetProjectCollection(tenantID, projectID, "notifications").UpdateOne(ctx, bson.M{
		"_id":       notificationID,
		"tenantId":  tenantID,
		"projectId": projectID,
		"isActive":  true,
	}, bson.M{"$addToSet": bson.M{"deletedBy": userID}})
}

func (r *DynamicRepository) EnsureNotificationIndexes(ctx context.Context, tenantID, projectID string) error {
	_, err := r.GetProjectCollection(tenantID, projectID, "notifications").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "tenantId", Value: 1},
				{Key: "projectId", Value: 1},
				{Key: "isActive", Value: 1},
				{Key: "createdAt", Value: -1},
			},
			Options: options.Index().SetName("idx_notifications_scope_created").SetBackground(true),
		},
		{
			Keys: bson.D{
				{Key: "tenantId", Value: 1},
				{Key: "projectId", Value: 1},
				{Key: "selectedUsers", Value: 1},
			},
			Options: options.Index().SetName("idx_notifications_selected_users").SetBackground(true),
		},
		{
			Keys: bson.D{
				{Key: "tenantId", Value: 1},
				{Key: "projectId", Value: 1},
				{Key: "selectedRoles", Value: 1},
			},
			Options: options.Index().SetName("idx_notifications_selected_roles").SetBackground(true),
		},
		{
			Keys: bson.D{
				{Key: "tenantId", Value: 1},
				{Key: "projectId", Value: 1},
				{Key: "seenBy", Value: 1},
			},
			Options: options.Index().SetName("idx_notifications_seen_by").SetBackground(true),
		},
		{
			Keys: bson.D{
				{Key: "tenantId", Value: 1},
				{Key: "projectId", Value: 1},
				{Key: "deletedBy", Value: 1},
			},
			Options: options.Index().SetName("idx_notifications_deleted_by").SetBackground(true),
		},
		{
			Keys:    bson.D{{Key: "expireAt", Value: 1}},
			Options: options.Index().SetName("idx_notifications_expire_at_ttl").SetExpireAfterSeconds(0).SetSparse(true).SetBackground(true),
		},
	})
	return err
}

func (r *DynamicRepository) EnsureNotificationIndexesForAllProjects(ctx context.Context) error {
	cursor, err := r.globalCollection("projects").Find(ctx, bson.M{"isActive": true})
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var project struct {
			ID       primitive.ObjectID `bson:"_id"`
			TenantID primitive.ObjectID `bson:"tenantId"`
		}
		if err := cursor.Decode(&project); err != nil {
			return err
		}
		if err := r.EnsureNotificationIndexes(ctx, project.TenantID.Hex(), project.ID.Hex()); err != nil {
			return err
		}
	}
	return cursor.Err()
}

func (r *DynamicRepository) CountByField(ctx context.Context, tenantID, projectID, schemaName, fieldName string, fieldValue interface{}) (int64, error) {
	return r.GetCollection(tenantID, projectID, schemaName).CountDocuments(ctx, bson.M{fieldName: fieldValue})
}

func (r *DynamicRepository) Count(ctx context.Context, tenantID, projectID, schemaName string, filter bson.M) (int64, error) {
	ctx, span := observability.StartSpan(ctx, "mongo.operation", observability.MongoTraceAttrs("count", schemaName)...)
	count, err := r.GetCollection(tenantID, projectID, schemaName).CountDocuments(ctx, filter)
	observability.EndSpan(span, traceStatus(err), err)
	return count, err
}

func (r *DynamicRepository) Distinct(ctx context.Context, tenantID, projectID, schemaName, fieldName string, filter bson.M) ([]interface{}, error) {
	ctx, span := observability.StartSpan(ctx, "mongo.operation", observability.MongoTraceAttrs("distinct", schemaName)...)
	result, err := r.GetCollection(tenantID, projectID, schemaName).Distinct(ctx, fieldName, filter)
	observability.EndSpan(span, traceStatus(err), err)
	return result, err
}

func (r *DynamicRepository) CountByFieldIn(ctx context.Context, tenantID, projectID, schemaName, fieldName string, values []interface{}) (int64, error) {
	return r.GetCollection(tenantID, projectID, schemaName).CountDocuments(ctx, bson.M{fieldName: bson.M{"$in": values}})
}

func (r *DynamicRepository) CountByFieldExcludingID(ctx context.Context, tenantID, projectID, schemaName, fieldName string, fieldValue interface{}, id interface{}) (int64, error) {
	filter := bson.M{
		fieldName: fieldValue,
		"_id":     bson.M{"$ne": id},
	}
	return r.GetCollection(tenantID, projectID, schemaName).CountDocuments(ctx, filter)
}

func (r *DynamicRepository) Insert(ctx context.Context, tenantID, projectID, schemaName string, item map[string]interface{}) (*mongo.InsertOneResult, error) {
	ctx, span := observability.StartSpan(ctx, "mongo.operation", observability.MongoTraceAttrs("insert_one", schemaName)...)
	result, err := r.GetCollection(tenantID, projectID, schemaName).InsertOne(ctx, item)
	observability.EndSpan(span, traceStatus(err), err)
	return result, err
}

func (r *DynamicRepository) InsertMany(ctx context.Context, tenantID, projectID, schemaName string, items []map[string]interface{}) (*mongo.InsertManyResult, error) {
	ctx, span := observability.StartSpan(ctx, "mongo.operation", observability.MongoTraceAttrs("insert_many", schemaName)...)
	docs := make([]interface{}, len(items))
	for i, item := range items {
		docs[i] = item
	}
	result, err := r.GetCollection(tenantID, projectID, schemaName).InsertMany(ctx, docs)
	observability.EndSpan(span, traceStatus(err), err)
	return result, err
}

func (r *DynamicRepository) FindByID(ctx context.Context, tenantID, projectID, schemaName string, id interface{}) (bson.M, error) {
	ctx, span := observability.StartSpan(ctx, "mongo.operation", observability.MongoTraceAttrs("find_by_id", schemaName)...)
	var item bson.M
	err := r.GetCollection(tenantID, projectID, schemaName).FindOne(ctx, bson.M{"_id": id}).Decode(&item)
	observability.EndSpan(span, traceStatus(err), err)
	return item, err
}

func (r *DynamicRepository) FindOne(ctx context.Context, tenantID, projectID, schemaName string, filter bson.M) (bson.M, error) {
	ctx, span := observability.StartSpan(ctx, "mongo.operation", observability.MongoTraceAttrs("find_one", schemaName)...)
	var item bson.M
	err := r.GetCollection(tenantID, projectID, schemaName).FindOne(ctx, filter).Decode(&item)
	observability.EndSpan(span, traceStatus(err), err)
	return item, err
}

func (r *DynamicRepository) FindAll(ctx context.Context, tenantID, projectID, schemaName string, limit int64) ([]map[string]interface{}, error) {
	ctx, span := observability.StartSpan(ctx, "mongo.operation", observability.MongoTraceAttrs("find_all", schemaName)...)
	pager := utils.Pager{Enabled: false}
	opts := options.Find().SetLimit(limit)
	result, err := utils.QueryAndDecodeCollection(ctx, r.GetCollection(tenantID, projectID, schemaName), schemaName, bson.M{}, opts, &pager)
	observability.EndSpan(span, traceStatus(err), err)
	return result, err
}

func (r *DynamicRepository) Query(ctx context.Context, tenantID, projectID, schemaName string, filter bson.M, opts *options.FindOptions, pager *utils.Pager) ([]map[string]interface{}, error) {
	ctx, span := observability.StartSpan(ctx, "mongo.operation", observability.MongoTraceAttrs("query", schemaName)...)
	result, err := utils.QueryAndDecodeCollection(ctx, r.GetCollection(tenantID, projectID, schemaName), schemaName, filter, opts, pager)
	observability.EndSpan(span, traceStatus(err), err)
	return result, err
}

func (r *DynamicRepository) FindForSelection(ctx context.Context, tenantID, projectID, schemaName, fieldName string, extraFields ...string) ([]map[string]interface{}, error) {
	ctx, span := observability.StartSpan(ctx, "mongo.operation", observability.MongoTraceAttrs("find_for_selection", schemaName)...)
	status := "success"
	var spanErr error
	defer func() { observability.EndSpan(span, status, spanErr) }()
	projection := bson.M{
		"_id":     1,
		fieldName: 1,
	}
	for _, extraField := range extraFields {
		if extraField == "" || extraField == "_id" {
			continue
		}
		projection[extraField] = 1
	}

	cursor, err := r.GetCollection(tenantID, projectID, schemaName).Find(ctx, bson.M{}, options.Find().SetProjection(projection))
	if err != nil {
		status = "error"
		spanErr = err
		return nil, err
	}
	defer cursor.Close(ctx)

	var items []map[string]interface{}
	if err := cursor.All(ctx, &items); err != nil {
		status = "error"
		spanErr = err
		return nil, err
	}
	return items, nil
}

func (r *DynamicRepository) ExecutePipeline(ctx context.Context, tenantID, projectID, schemaName string, pipelineStage models.PipelineStage) ([]map[string]interface{}, error) {
	ctx, span := observability.StartSpan(ctx, "mongo.operation", observability.MongoTraceAttrs("aggregate", schemaName)...)
	items, err := utils.ExecuteDynamicPipeline(ctx, r.GetCollection(tenantID, projectID, schemaName), pipelineStage)
	if err != nil {
		observability.EndSpan(span, "error", err)
		return nil, err
	}

	resultItems := make([]map[string]interface{}, 0, len(items))
	for _, doc := range items {
		resultItems = append(resultItems, map[string]interface{}(doc))
	}
	observability.EndSpan(span, "success", nil)
	return resultItems, nil
}

func (r *DynamicRepository) DeleteByID(ctx context.Context, tenantID, projectID, schemaName string, id interface{}) (*mongo.DeleteResult, error) {
	ctx, span := observability.StartSpan(ctx, "mongo.operation", observability.MongoTraceAttrs("delete_one", schemaName)...)
	result, err := r.GetCollection(tenantID, projectID, schemaName).DeleteOne(ctx, bson.M{"_id": id})
	observability.EndSpan(span, traceStatus(err), err)
	return result, err
}

func (r *DynamicRepository) DeleteManyByField(ctx context.Context, tenantID, projectID, schemaName, fieldName string, fieldValue interface{}) (*mongo.DeleteResult, error) {
	ctx, span := observability.StartSpan(ctx, "mongo.operation", observability.MongoTraceAttrs("delete_many", schemaName)...)
	result, err := r.GetCollection(tenantID, projectID, schemaName).DeleteMany(ctx, bson.M{fieldName: fieldValue})
	observability.EndSpan(span, traceStatus(err), err)
	return result, err
}

func (r *DynamicRepository) UpdateByID(ctx context.Context, tenantID, projectID, schemaName string, id interface{}, item map[string]interface{}) (*mongo.UpdateResult, error) {
	ctx, span := observability.StartSpan(ctx, "mongo.operation", observability.MongoTraceAttrs("update_one", schemaName)...)
	result, err := r.GetCollection(tenantID, projectID, schemaName).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": item})
	observability.EndSpan(span, traceStatus(err), err)
	return result, err
}

func (r *DynamicRepository) UpdateMany(ctx context.Context, tenantID, projectID, schemaName string, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
	ctx, span := observability.StartSpan(ctx, "mongo.operation", observability.MongoTraceAttrs("update_many", schemaName)...)
	result, err := r.GetCollection(tenantID, projectID, schemaName).UpdateMany(ctx, filter, update, opts...)
	observability.EndSpan(span, traceStatus(err), err)
	return result, err
}

func (r *DynamicRepository) NextSequence(ctx context.Context, schemaName string) (int64, error) {
	return r.nextSequence(ctx, schemaName)
}

func (r *DynamicRepository) WithTransaction(ctx context.Context, fn func(mongo.SessionContext) error) error {
	if r.withTransaction != nil {
		return r.withTransaction(ctx, fn)
	}
	configs.InitDB()
	session, err := configs.DB.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)

	txnOptions := options.Transaction().
		SetReadConcern(readconcern.Snapshot()).
		SetWriteConcern(writeconcern.New(writeconcern.WMajority()))

	_, err = session.WithTransaction(ctx, func(sessionCtx mongo.SessionContext) (interface{}, error) {
		return nil, fn(sessionCtx)
	}, txnOptions)
	return err
}

func (r *DynamicRepository) InsertOutboxEvent(ctx context.Context, event models.DynamicOutboxEvent) (*mongo.InsertOneResult, error) {
	if event.Payload.IdempotencyKey != "" {
		_, err := r.globalCollection("dynamic_outbox").UpdateOne(
			ctx,
			bson.M{"payload.idempotencyKey": event.Payload.IdempotencyKey},
			bson.M{"$setOnInsert": event},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return &mongo.InsertOneResult{InsertedID: event.ID}, nil
			}
			return nil, err
		}
		return &mongo.InsertOneResult{InsertedID: event.ID}, nil
	}
	return r.globalCollection("dynamic_outbox").InsertOne(ctx, event)
}

func (r *DynamicRepository) EnsureOutboxIndexes(ctx context.Context) error {
	_, err := r.globalCollection("dynamic_outbox").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "nextAttemptAt", Value: 1},
				{Key: "createdAt", Value: 1},
			},
			Options: options.Index().SetName("idx_dynamic_outbox_claim").SetBackground(true),
		},
		{
			Keys: bson.D{
				{Key: "tenantId", Value: 1},
				{Key: "projectId", Value: 1},
				{Key: "schemaName", Value: 1},
				{Key: "createdAt", Value: -1},
			},
			Options: options.Index().SetName("idx_dynamic_outbox_scope").SetBackground(true),
		},
		{
			Keys:    bson.D{{Key: "expireAt", Value: 1}},
			Options: options.Index().SetName("idx_dynamic_outbox_expire_at_ttl").SetExpireAfterSeconds(0).SetBackground(true),
		},
		{
			Keys: bson.D{{Key: "payload.idempotencyKey", Value: 1}},
			Options: options.Index().
				SetName("idx_dynamic_outbox_idempotency_key").
				SetUnique(true).
				SetSparse(true).
				SetBackground(true),
		},
	})
	if err != nil {
		return err
	}
	_, err = r.globalCollection("dynamic_workflow_step_executions").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "idempotencyKey", Value: 1}},
		Options: options.Index().SetName("idx_workflow_step_executions_idempotency_key").SetUnique(true).SetBackground(true),
	})
	return err
}

func (r *DynamicRepository) WorkflowStepExecutionDone(ctx context.Context, idempotencyKey string) (bool, error) {
	if idempotencyKey == "" {
		return false, nil
	}
	err := r.globalCollection("dynamic_workflow_step_executions").
		FindOne(ctx, bson.M{"idempotencyKey": idempotencyKey}).Err()
	if err == nil {
		return true, nil
	}
	if err == mongo.ErrNoDocuments {
		return false, nil
	}
	return false, err
}

func (r *DynamicRepository) MarkWorkflowStepExecutionDone(ctx context.Context, idempotencyKey string, eventID primitive.ObjectID) error {
	if idempotencyKey == "" {
		return nil
	}
	now := time.Now()
	_, err := r.globalCollection("dynamic_workflow_step_executions").UpdateOne(
		ctx,
		bson.M{"idempotencyKey": idempotencyKey},
		bson.M{"$setOnInsert": bson.M{
			"idempotencyKey": idempotencyKey,
			"eventId":        eventID,
			"createdAt":      primitive.NewDateTimeFromTime(now),
		}},
		options.Update().SetUpsert(true),
	)
	return err
}

func (r *DynamicRepository) ClaimPendingOutboxEvent(ctx context.Context, now time.Time) (*models.DynamicOutboxEvent, error) {
	collection := r.globalCollection("dynamic_outbox")
	filter := bson.M{
		"status": bson.M{"$in": []string{
			models.DynamicOutboxStatusPending,
			models.DynamicOutboxStatusProcessing,
		}},
		"nextAttemptAt": bson.M{"$lte": primitive.NewDateTimeFromTime(now)},
		"$expr": bson.M{
			"$lt": []interface{}{"$attempts", "$maxAttempts"},
		},
	}
	update := bson.M{
		"$set": bson.M{
			"status":        models.DynamicOutboxStatusProcessing,
			"nextAttemptAt": primitive.NewDateTimeFromTime(now.Add(30 * time.Second)),
			"updatedAt":     primitive.NewDateTimeFromTime(now),
		},
		"$inc": bson.M{"attempts": 1},
	}
	opts := options.FindOneAndUpdate().SetSort(bson.D{{Key: "createdAt", Value: 1}}).SetReturnDocument(options.After)

	var event models.DynamicOutboxEvent
	err := collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&event)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *DynamicRepository) MarkOutboxEventDone(ctx context.Context, eventID primitive.ObjectID) error {
	now := time.Now()
	_, err := r.globalCollection("dynamic_outbox").UpdateByID(ctx, eventID, bson.M{
		"$set": bson.M{
			"status":      models.DynamicOutboxStatusDone,
			"updatedAt":   primitive.NewDateTimeFromTime(now),
			"processedAt": primitive.NewDateTimeFromTime(now),
			"expireAt":    primitive.NewDateTimeFromTime(now.Add(configs.GetOutboxDoneRetention())),
		},
		"$unset": bson.M{"lastError": ""},
	})
	return err
}

func (r *DynamicRepository) MarkOutboxEventFailed(ctx context.Context, event models.DynamicOutboxEvent, errMessage string, retryAfter time.Duration) error {
	now := time.Now()
	status := models.DynamicOutboxStatusPending
	setFields := bson.M{
		"status":        status,
		"lastError":     errMessage,
		"nextAttemptAt": primitive.NewDateTimeFromTime(now.Add(retryAfter)),
		"updatedAt":     primitive.NewDateTimeFromTime(now),
	}
	update := bson.M{"$set": setFields, "$unset": bson.M{"expireAt": ""}}
	if event.Attempts >= event.MaxAttempts {
		status = models.DynamicOutboxStatusFailed
		setFields["status"] = status
		setFields["expireAt"] = primitive.NewDateTimeFromTime(now.Add(configs.GetOutboxFailedRetention()))
		delete(update, "$unset")
	}
	_, err := r.globalCollection("dynamic_outbox").UpdateByID(ctx, event.ID, update)
	return err
}

func traceStatus(err error) string {
	if err != nil {
		return "error"
	}
	return "success"
}
