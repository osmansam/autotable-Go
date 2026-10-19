//go:build integration

package services

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestWorkflowMongoLocalizationContentWriteSerializesWithTakeover(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	jobs, translations := f.db.Collection("translation_jobs"), f.db.Collection("translations")
	r := repositories.NewLocalizationRepositoryWithCollections(translations, f.db.Collection("preferences"), jobs)
	now := time.Now().UTC()
	job := &models.TranslationJob{ID: primitive.NewObjectID(), Status: models.TranslationJobPending, NextAttemptAt: now}
	if err := r.CreateJob(f.ctx, job); err != nil {
		t.Fatal(err)
	}
	first, err := r.LeaseNextJob(f.ctx, "worker-one", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	entry := models.TranslationEntry{TenantID: primitive.NewObjectID(), ProjectID: primitive.NewObjectID(), Locale: "tr", TranslationKey: "title", SourceHash: "source", SourceText: "Title", TranslatedText: "First"}
	if err := r.UpsertPlaceholder(f.ctx, entry); err != nil {
		t.Fatal(err)
	}
	paused, resume := make(chan struct{}), make(chan struct{})
	var pauseOnce, resumeOnce sync.Once
	unpause := func() { resumeOnce.Do(func() { close(resume) }) }
	defer unpause()
	monitor := &event.CommandMonitor{Started: func(ctx context.Context, e *event.CommandStartedEvent) {
		if e.CommandName == "update" && e.DatabaseName == f.db.Name() && e.Command.Lookup("update").StringValue() == "translations" {
			pauseOnce.Do(func() {
				close(paused)
				select {
				case <-resume:
				case <-ctx.Done():
				}
			})
		}
	}}
	client, err := mongo.Connect(f.ctx, options.Client().ApplyURI(os.Getenv("AUTOTABLE_WORKFLOW_MONGO_URI")).SetMonitor(monitor))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		unpause()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(cleanupCtx)
	}()
	writeDB := client.Database(f.db.Name())
	writer := repositories.NewLocalizationRepositoryWithCollections(writeDB.Collection("translations"), writeDB.Collection("preferences"), writeDB.Collection("translation_jobs"))
	writeDone := make(chan error, 1)
	go func() {
		stored, err := writer.UpsertGeneratedTranslation(repositories.WithLocalizationJobClaim(f.ctx, *first), entry)
		if err == nil && !stored {
			err = errors.New("current owner did not store content")
		}
		writeDone <- err
	}()
	select {
	case <-paused:
	case err := <-writeDone:
		t.Fatalf("writer failed before pause: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not reach content mutation")
	}
	takeover := make(chan error, 1)
	go func() {
		_, err := r.LeaseNextJob(f.ctx, "worker-two", now.Add(2*time.Minute), 5*time.Minute)
		takeover <- err
	}()
	select {
	case err := <-takeover:
		unpause()
		t.Fatalf("claim transferred during an uncommitted content write: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	unpause()
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	if err := <-takeover; err != nil {
		t.Fatal(err)
	}
	entry.TranslatedText = "Stale overwrite"
	if _, err := writer.UpsertGeneratedTranslation(repositories.WithLocalizationJobClaim(f.ctx, *first), entry); !errors.Is(err, repositories.ErrLocalizationLeaseLost) {
		t.Fatalf("stale content write error=%v", err)
	}
	if err := writer.MarkGeneratedTranslationFailed(repositories.WithLocalizationJobClaim(f.ctx, *first), entry); !errors.Is(err, repositories.ErrLocalizationLeaseLost) {
		t.Fatalf("stale failure write error=%v", err)
	}
	stored, err := r.GetTranslation(f.ctx, entry.TenantID, entry.ProjectID, entry.Locale, entry.TranslationKey)
	if err != nil || stored.TranslatedText != "First" || stored.Status != models.TranslationStatusCurrent {
		t.Fatalf("stale owner changed content: %#v %v", stored, err)
	}
}

// These tests deliberately fail without a real replica set. Run them with
// scripts/test-workflow-mongo-integration.sh; every test owns a disposable DB.
type workflowMongoFixture struct {
	t         *testing.T
	ctx       context.Context
	db        *mongo.Database
	repo      *repositories.DynamicRepository
	service   *DynamicService
	container *models.ContainerModel
	user      primitive.ObjectID
}

func newWorkflowMongoFixture(t *testing.T) *workflowMongoFixture {
	t.Helper()
	uri := os.Getenv("AUTOTABLE_WORKFLOW_MONGO_URI")
	if uri == "" {
		t.Fatal("AUTOTABLE_WORKFLOW_MONGO_URI is required; run scripts/test-workflow-mongo-integration.sh")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetServerSelectionTimeout(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	var hello bson.M
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		t.Fatalf("real MongoDB unavailable: %v", err)
	}
	if hello["setName"] == nil {
		t.Fatal("MongoDB integration requires a replica set for real transactions")
	}
	db := client.Database("autotable_workflow_integration_" + primitive.NewObjectID().Hex())
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := db.Drop(cleanupCtx); err != nil {
			t.Errorf("drop task-owned DB: %v", err)
		}
	})
	redisServer := miniredis.RunT(t)
	previousRedis := configs.RedisClient
	configs.RedisClient = redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = configs.RedisClient.Close(); configs.RedisClient = previousRedis })
	repo := repositories.NewDynamicRepositoryWithDatabase(db)
	if err := repo.EnsureOutboxIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	service := &DynamicService{repository: repo, runTransaction: repo.WithTransaction}
	service.loadAccessUser = service.loadCurrentAccessUser
	user := primitive.NewObjectID()
	policy := &models.RecordAccessPolicy{Any: []models.RecordAccessRule{{Field: "ownerId", Operator: "eq", Value: "{{auth.user.id}}"}}}
	container := &models.ContainerModel{SchemaName: "orders", Fields: []models.Field{
		{Name: "ownerId", Type: "objectId"}, {Name: "status", Type: "string"}, {Name: "name", Type: "string"},
		{Name: "count", Type: "int"}, {Name: "items", Type: "array"}, {Name: "remove", Type: "string"},
	}, RowAccess: &models.RowAccessRule{Conditions: []models.Condition{{Field: "status", Operator: "=", Value: "open", Roles: []string{"member"}}}}}
	container.Routes.CreateDynamicModelItem.Access = &models.RecordAccessPolicy{Assign: map[string]interface{}{"ownerId": "{{auth.user.id}}"}, Any: policy.Any}
	container.Routes.UpdateDynamicModelItem.Access = policy
	container.Routes.DeleteDynamicModelItem.Access = policy
	container.Routes.GetAllDynamicModelItems.Access = policy
	f := &workflowMongoFixture{t: t, ctx: ctx, db: db, repo: repo, service: service, container: container, user: user}
	f.saveContainer()
	if _, err := repo.GetProjectCollection("tenant", "project", "containers").InsertOne(ctx, models.ContainerModel{SchemaName: "users", IsAuthContainer: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Insert(ctx, "tenant", "project", "users", map[string]interface{}{"_id": user, "name": "caller"}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *workflowMongoFixture) saveContainer() {
	f.t.Helper()
	_, err := f.repo.GetProjectCollection("tenant", "project", "containers").ReplaceOne(f.ctx, bson.M{"schemaName": "orders"}, f.container, options.Replace().SetUpsert(true))
	if err != nil {
		f.t.Fatal(err)
	}
}
func mongoWorkflowStep(name, kind string, config map[string]interface{}) models.DynamicWorkflowStep {
	step := models.DynamicWorkflowStep{Name: name, Type: kind, IsActive: true, Config: config}
	if kind == models.WorkflowStepTypeCallAPI {
		step.TimeoutSec = 2
	}
	return step
}
func (f *workflowMongoFixture) run(steps ...models.DynamicWorkflowStep) (DynamicExecutionResult, error) {
	f.t.Helper()
	mode := models.WorkflowModeTransactional
	for i := range steps {
		steps[i].Order = i + 1
		if steps[i].Type == models.WorkflowStepTypeCallAPI {
			mode = models.WorkflowModeHybrid
		}
	}
	if mode == models.WorkflowModeHybrid {
		for i := range steps {
			steps[i].ExecutionMode = models.WorkflowModeTransactional
			if steps[i].Type == models.WorkflowStepTypeCallAPI {
				steps[i].ExecutionMode = models.WorkflowModeOutbox
			}
		}
	}
	f.container.Workflows = []models.DynamicWorkflow{{Name: "test", Trigger: models.WorkflowTriggerManual, Mode: mode, IsActive: true, StopOnError: true, ReturnStep: steps[len(steps)-1].Name, Steps: steps}}
	f.saveContainer()
	return f.service.ExecuteWorkflow(f.ctx, ExecuteWorkflowInput{AccessIdentityInput: AccessIdentityInput{IdentityKind: AccessIdentityCaller, UserID: f.user.Hex(), UserRole: "member", UserRoles: []string{"member"}}, TenantID: "tenant", ProjectID: "project", Schema: "orders", WorkflowName: "test"})
}
func (f *workflowMongoFixture) insert(doc bson.M) primitive.ObjectID {
	f.t.Helper()
	if _, ok := doc["ownerId"]; !ok {
		doc["ownerId"] = f.user
	}
	if _, ok := doc["status"]; !ok {
		doc["status"] = "open"
	}
	result, err := f.repo.Insert(f.ctx, "tenant", "project", "orders", doc)
	if err != nil {
		f.t.Fatal(err)
	}
	return result.InsertedID.(primitive.ObjectID)
}
func (f *workflowMongoFixture) document(id primitive.ObjectID) bson.M {
	f.t.Helper()
	doc, err := f.repo.FindByID(f.ctx, "tenant", "project", "orders", id)
	if err != nil {
		f.t.Fatal(err)
	}
	return doc
}
func (f *workflowMongoFixture) count(collection string, filter bson.M) int64 {
	f.t.Helper()
	n, err := f.db.Collection(collection).CountDocuments(f.ctx, filter)
	if err != nil {
		f.t.Fatal(err)
	}
	return n
}
func mongoResultMap(t *testing.T, result DynamicExecutionResult) map[string]interface{} {
	t.Helper()
	m, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("workflow result type %T: %#v", result.Data, result.Data)
	}
	return m
}

func TestWorkflowMongoCRUDAndCombinedAccess(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	visible := f.insert(bson.M{"name": "visible", "count": int32(1), "remove": "old"})
	blocked := f.insert(bson.M{"name": "row-denied", "status": "closed", "count": int32(10)})
	other := f.insert(bson.M{"name": "policy-denied", "ownerId": primitive.NewObjectID(), "count": int32(20)})
	if _, err := f.repo.Insert(f.ctx, "tenant", "other-project", "orders", map[string]interface{}{"ownerId": f.user, "status": "open", "name": "other-project"}); err != nil {
		t.Fatal(err)
	}
	result, err := f.run(mongoWorkflowStep("find", models.WorkflowStepTypeFindRecords, map[string]interface{}{}))
	if err != nil {
		t.Fatal(err)
	}
	items := mongoResultMap(t, result)["items"].([]map[string]interface{})
	if len(items) != 1 || items[0]["_id"] != visible {
		t.Fatalf("combined route/row/project filter returned %#v", items)
	}
	result, err = f.run(mongoWorkflowStep("count", models.WorkflowStepTypeCountRecords, map[string]interface{}{}))
	if err != nil || mongoResultMap(t, result)["count"] != int64(1) {
		t.Fatalf("count=%#v err=%v", result.Data, err)
	}
	result, err = f.run(mongoWorkflowStep("distinct", models.WorkflowStepTypeDistinct, map[string]interface{}{"field": "name"}))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(mongoResultMap(t, result)["values"], []interface{}{"visible"}) {
		t.Fatalf("distinct leaked inaccessible records: %#v", result.Data)
	}
	result, err = f.run(mongoWorkflowStep("get", models.WorkflowStepTypeGetRecord, map[string]interface{}{"filter": bson.M{"_id": other}}))
	if err != nil || mongoResultMap(t, result)["found"] != false {
		t.Fatalf("get leaked inaccessible record: %#v err=%v", result.Data, err)
	}
	result, err = f.run(mongoWorkflowStep("create", models.WorkflowStepTypeCreateRecord, map[string]interface{}{"document": bson.M{"name": "created", "status": "open", "ownerId": primitive.NewObjectID().Hex()}}))
	if err != nil {
		t.Fatal(err)
	}
	created := mongoResultMap(t, result)["_id"].(primitive.ObjectID)
	if f.document(created)["ownerId"] != f.user {
		t.Fatal("create trusted owner assignment was not persisted")
	}
	result, err = f.run(mongoWorkflowStep("update", models.WorkflowStepTypeUpdateRecord, map[string]interface{}{"filter": bson.M{}, "update": bson.M{"$inc": bson.M{"count": int32(1)}}}))
	if err != nil || mongoResultMap(t, result)["matchedCount"] != int64(2) {
		t.Fatalf("update=%#v err=%v", result.Data, err)
	}
	if f.document(visible)["count"] != int32(2) || f.document(blocked)["count"] != int32(10) || f.document(other)["count"] != int32(20) {
		t.Fatal("update did not preserve route/row access boundaries")
	}
	_, err = f.run(mongoWorkflowStep("unset", models.WorkflowStepTypeUnsetRecord, map[string]interface{}{"filter": bson.M{"_id": visible}, "fields": []interface{}{"remove"}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := f.document(visible)["remove"]; exists {
		t.Fatal("unset did not persist")
	}
	result, err = f.run(mongoWorkflowStep("delete", models.WorkflowStepTypeDeleteRecord, map[string]interface{}{"filter": bson.M{}}))
	if err != nil || mongoResultMap(t, result)["deletedCount"] != int64(2) {
		t.Fatalf("delete=%#v err=%v", result.Data, err)
	}
	if f.document(blocked)["name"] != "row-denied" || f.document(other)["name"] != "policy-denied" {
		t.Fatal("delete escaped access filter")
	}
}

func TestWorkflowMongoRejectsFinalOwnershipTransfer(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	id := f.insert(bson.M{"name": "owned"})
	_, err := f.run(mongoWorkflowStep("transfer", models.WorkflowStepTypeUpdateRecord, map[string]interface{}{"filter": bson.M{"_id": id}, "update": bson.M{"$set": bson.M{"ownerId": primitive.NewObjectID()}}}))
	var serviceErr *ServiceError
	if !errors.As(err, &serviceErr) || serviceErr.Status != 404 {
		t.Fatalf("ownership transfer error=%v", err)
	}
	if f.document(id)["ownerId"] != f.user || f.count("dynamic_outbox", bson.M{}) != 0 {
		t.Fatal("denied candidate changed data or inserted outbox")
	}
}

func TestWorkflowMongoOperatorsMatchAuthorizedPreview(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	cases := []struct {
		name   string
		doc    bson.M
		update bson.M
		want   bson.M
	}{
		{"exact int64 increment", bson.M{"count": int64(9007199254740992)}, bson.M{"$inc": bson.M{"count": int64(1)}}, bson.M{"count": int64(9007199254740993)}},
		{"int32 promotion", bson.M{"count": int32(2147483647)}, bson.M{"$inc": bson.M{"count": int32(1)}}, bson.M{"count": int64(2147483648)}},
		{"numeric pull retains string", bson.M{"items": bson.A{"1", int32(1), int64(2)}}, bson.M{"$pull": bson.M{"items": int64(1)}}, bson.M{"items": bson.A{"1", int64(2)}}},
		{"set deduplicates numeric types", bson.M{"items": bson.A{int32(1)}}, bson.M{"$addToSet": bson.M{"items": bson.M{"$each": bson.A{int64(1), int32(2), int32(2)}}}}, bson.M{"items": bson.A{int32(1), int32(2)}}},
		{"push array is one value", bson.M{}, bson.M{"$push": bson.M{"items": bson.A{int32(1), int32(2)}}}, bson.M{"items": bson.A{bson.A{int32(1), int32(2)}}}},
		{"pullAll retains string", bson.M{"items": bson.A{int32(1), "1", int32(2)}}, bson.M{"$pullAll": bson.M{"items": bson.A{int64(1)}}}, bson.M{"items": bson.A{"1", int32(2)}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := f.insert(tc.doc)
			step := mongoWorkflowStep("mutate", models.WorkflowStepTypeUpdateRecord, map[string]interface{}{"filter": bson.M{"_id": id}, "update": tc.update})
			if values, ok := tc.update["$pullAll"].(bson.M); ok {
				step = mongoWorkflowStep("mutate", models.WorkflowStepTypePullAll, map[string]interface{}{"filter": bson.M{"_id": id}, "field": "items", "value": values["items"]})
			}
			_, err := f.run(step)
			if err != nil {
				t.Fatal(err)
			}
			actual := f.document(id)
			for key, want := range tc.want {
				if !reflect.DeepEqual(actual[key], want) {
					t.Fatalf("Mongo field %s=%#v (%T), want %#v (%T)", key, actual[key], actual[key], want, want)
				}
			}
		})
	}
	id := f.insert(bson.M{"items": bson.A{int32(1), int32(2)}})
	_, err := f.run(mongoWorkflowStep("unsupported", models.WorkflowStepTypeUpdateRecord, map[string]interface{}{"filter": bson.M{"_id": id}, "update": bson.M{"$pull": bson.M{"items": bson.M{"$gte": int32(2)}}}}))
	if err == nil {
		t.Fatal("unsupported selector was allowed past authorization preview")
	}
	if !reflect.DeepEqual(f.document(id)["items"], bson.A{int32(1), int32(2)}) {
		t.Fatal("rejected update changed Mongo document")
	}
}

func TestWorkflowMongoTransactionRollsBackDataAndQueuedOutbox(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	create := mongoWorkflowStep("create", models.WorkflowStepTypeCreateRecord, map[string]interface{}{"document": bson.M{"name": "transactional", "status": "open"}})
	notify := mongoWorkflowStep("notify", models.WorkflowStepTypeCallAPI, map[string]interface{}{"url": "https://example.invalid/queued-only", "method": "POST"})
	fail := mongoWorkflowStep("fail", models.WorkflowStepTypeFail, map[string]interface{}{"message": "intentional rollback"})
	if _, err := f.run(create, fail); err == nil || !strings.Contains(err.Error(), "intentional rollback") {
		t.Fatalf("expected workflow business failure after the insert, got %v", err)
	}
	if f.count(utils.GetProjectCollectionName("tenant", "project", "orders"), bson.M{}) != 0 || f.count("dynamic_outbox", bson.M{}) != 0 {
		t.Fatal("transaction rollback left records or queued side effects")
	}
	// Reject only the queued workflow effect, after the primary insert and its
	// ordinary post-write outbox event have already succeeded in the session.
	if err := f.db.RunCommand(f.ctx, bson.D{{Key: "collMod", Value: "dynamic_outbox"}, {Key: "validator", Value: bson.M{"operation": bson.M{"$ne": models.DynamicOutboxOperationWorkflowStep}}}}).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(create, notify); err == nil || !strings.Contains(err.Error(), "validation") {
		t.Fatalf("expected queued effect validation failure, got %v", err)
	}
	if f.count(utils.GetProjectCollectionName("tenant", "project", "orders"), bson.M{}) != 0 || f.count("dynamic_outbox", bson.M{}) != 0 {
		t.Fatal("outbox failure committed primary data or an earlier outbox event")
	}
	if err := f.db.RunCommand(f.ctx, bson.D{{Key: "collMod", Value: "dynamic_outbox"}, {Key: "validator", Value: bson.M{}}}).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(create, notify); err != nil {
		t.Fatal(err)
	}
	if f.count(utils.GetProjectCollectionName("tenant", "project", "orders"), bson.M{}) != 1 || f.count("dynamic_outbox", bson.M{}) != 2 {
		t.Fatal("transaction commit did not persist data and both outbox events")
	}
}

func TestWorkflowMongoRepeatedUpdatesKeepDistinctOutboxEvents(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	id := f.insert(bson.M{"count": int32(0)})
	update := mongoWorkflowStep("increment", models.WorkflowStepTypeUpdateRecord, map[string]interface{}{"filter": bson.M{"_id": id}, "update": bson.M{"$inc": bson.M{"count": int32(1)}}})
	notify := mongoWorkflowStep("notify", models.WorkflowStepTypeCallAPI, map[string]interface{}{"url": "https://example.invalid/queued-only", "method": "POST"})
	for i := 0; i < 2; i++ {
		if _, err := f.run(update, notify); err != nil {
			t.Fatal(err)
		}
	}
	if f.document(id)["count"] != int32(2) {
		t.Fatal("repeated independent update was suppressed")
	}
	if f.count("dynamic_outbox", bson.M{"operation": models.DynamicOutboxOperationWorkflowStep}) != 2 || f.count("dynamic_outbox", bson.M{"operation": models.DynamicOutboxOperationUpdate}) != 2 {
		t.Fatal("repeated updates lost outbox events to idempotency collision")
	}
}

func TestWorkflowMongoReplayMarkerFailureRollsBackMutation(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	id := f.insert(bson.M{"count": int32(0)})
	// A rejecting ledger validator creates a deterministic failure after the
	// real mutation, proving completion and data share the same transaction.
	if err := f.db.RunCommand(f.ctx, bson.D{{Key: "collMod", Value: "dynamic_workflow_step_executions"}, {Key: "validator", Value: bson.M{"requiredMarkerField": bson.M{"$exists": true}}}}).Err(); err != nil {
		t.Fatal(err)
	}
	event := f.replayEvent(id, models.WorkflowStepTypeUpdateRecord, "replay-once")
	if err := processWorkflowOutboxStep(f.ctx, f.repo, event); err == nil {
		t.Fatal("rejecting completion ledger did not fail replay")
	}
	if got := f.document(id)["count"]; got != int32(0) {
		t.Fatalf("marker failure committed mutation: count=%v, want 0", got)
	}
	if f.count("dynamic_outbox", bson.M{}) != 0 || f.count("dynamic_workflow_step_executions", bson.M{}) != 0 {
		t.Fatal("marker failure left outbox or completion state")
	}
	if err := f.db.RunCommand(f.ctx, bson.D{{Key: "collMod", Value: "dynamic_workflow_step_executions"}, {Key: "validator", Value: bson.M{}}}).Err(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := processWorkflowOutboxStep(f.ctx, f.repo, event); err != nil {
			t.Fatal(err)
		}
	}
	if f.document(id)["count"] != int32(1) || f.count("dynamic_workflow_step_executions", bson.M{}) != 1 || f.count("dynamic_outbox", bson.M{}) != 1 {
		t.Fatal("replayed transaction did not apply exactly once")
	}
}

func (f *workflowMongoFixture) replayEvent(id primitive.ObjectID, kind, key string) *models.DynamicOutboxEvent {
	f.t.Helper()
	step := mongoWorkflowStep("increment", models.WorkflowStepTypeUpdateRecord, map[string]interface{}{"filter": bson.M{"_id": id}, "update": bson.M{"$inc": bson.M{"count": int32(1)}}})
	if kind == models.WorkflowStepTypeIf {
		step = models.DynamicWorkflowStep{Name: "group", Type: kind, IsActive: true, Steps: []models.DynamicWorkflowStep{step}}
	} else if kind == models.WorkflowStepTypeForEach {
		step = models.DynamicWorkflowStep{Name: "group", Type: kind, IsActive: true, Config: map[string]interface{}{"items": []interface{}{"one"}}, Steps: []models.DynamicWorkflowStep{step}}
	}
	event := buildWorkflowStepOutboxEvent(workflowExecutionPayload{TenantID: "tenant", ProjectID: "project", SchemaName: "orders", IdentityKind: AccessIdentityCaller, UserID: f.user.Hex(), UserRole: "member", StopOnError: true, IdempotencyKey: key}, "mongo-replay", step)
	raw, err := bson.Marshal(event)
	if err != nil {
		f.t.Fatal(err)
	}
	var persisted models.DynamicOutboxEvent
	if err := bson.Unmarshal(raw, &persisted); err != nil {
		f.t.Fatal(err)
	}
	return &persisted
}

func TestWorkflowMongoCapturedGroupsRollbackWithCompletion(t *testing.T) {
	for _, kind := range []string{models.WorkflowStepTypeIf, models.WorkflowStepTypeForEach} {
		t.Run(kind, func(t *testing.T) {
			f := newWorkflowMongoFixture(t)
			id := f.insert(bson.M{"count": int32(0)})
			if err := f.db.RunCommand(f.ctx, bson.D{{Key: "collMod", Value: "dynamic_workflow_step_executions"}, {Key: "validator", Value: bson.M{"requiredMarkerField": bson.M{"$exists": true}}}}).Err(); err != nil {
				t.Fatal(err)
			}
			event := f.replayEvent(id, kind, "group-once")
			if err := processWorkflowOutboxStep(f.ctx, f.repo, event); err == nil {
				t.Fatal("rejecting group completion ledger did not fail replay")
			}
			if got := f.document(id)["count"]; got != int32(0) {
				t.Fatalf("group marker failure committed mutation: count=%v, want 0", got)
			}
			if f.count("dynamic_outbox", bson.M{}) != 0 {
				t.Fatal("group marker failure committed outbox")
			}
			if err := f.db.RunCommand(f.ctx, bson.D{{Key: "collMod", Value: "dynamic_workflow_step_executions"}, {Key: "validator", Value: bson.M{}}}).Err(); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := processWorkflowOutboxStep(f.ctx, f.repo, event); err != nil {
					t.Fatal(err)
				}
			}
			if f.document(id)["count"] != int32(1) || f.count("dynamic_workflow_step_executions", bson.M{}) != 1 {
				t.Fatal("captured group replay did not apply once")
			}
		})
	}
}

func TestWorkflowMongoConcurrentReplayCommitsOnce(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	id := f.insert(bson.M{"count": int32(0)})
	event := f.replayEvent(id, models.WorkflowStepTypeUpdateRecord, "concurrent-once")
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; errs <- processWorkflowOutboxStep(f.ctx, f.repo, event) }()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.document(id)["count"] != int32(1) || f.count("dynamic_workflow_step_executions", bson.M{}) != 1 || f.count("dynamic_outbox", bson.M{}) != 1 {
		t.Fatal("concurrent deliveries committed duplicate mutations, markers, or side effects")
	}
}

func TestWorkflowMongoUniqueIdempotencyAndLeaseOwnership(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	makeEvent := func() models.DynamicOutboxEvent {
		return models.DynamicOutboxEvent{ID: primitive.NewObjectID(), Status: models.DynamicOutboxStatusPending, MaxAttempts: 3, CreatedAt: primitive.NewDateTimeFromTime(now), NextAttemptAt: primitive.NewDateTimeFromTime(now), Payload: models.DynamicOutboxPayload{IdempotencyKey: "same-key"}}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.repo.InsertOutboxEvent(f.ctx, makeEvent()); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.count("dynamic_outbox", bson.M{}) != 1 {
		t.Fatal("concurrent idempotent enqueue created duplicate records")
	}
	if _, err := f.db.Collection("dynamic_outbox").InsertOne(f.ctx, makeEvent()); !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("unique Mongo index did not reject duplicate key: %v", err)
	}
	first, err := f.repo.ClaimPendingOutboxEvent(f.ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.ClaimPendingOutboxEvent(f.ctx, now.Add(time.Second)); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("unexpired lease was stolen: %v", err)
	}
	if owned, err := f.repo.RenewOutboxEventLease(f.ctx, *first, now.Add(2*time.Minute)); err != nil || !owned {
		t.Fatalf("renew owned lease=%v err=%v", owned, err)
	}
	if _, err := f.repo.ClaimPendingOutboxEvent(f.ctx, now.Add(time.Minute)); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("renewed lease was stolen: %v", err)
	}
	second, err := f.repo.ClaimPendingOutboxEvent(f.ctx, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.Attempts != 1 || second.Attempts != 2 {
		t.Fatalf("reclaim attempts: first=%#v second=%#v", first, second)
	}
	for name, attempt := range map[string]func() (bool, error){
		"stale renewal":    func() (bool, error) { return f.repo.RenewOutboxEventLease(f.ctx, *first, now.Add(time.Hour)) },
		"stale completion": func() (bool, error) { return f.repo.MarkClaimedOutboxEventDone(f.ctx, *first) },
		"stale failure":    func() (bool, error) { return f.repo.MarkClaimedOutboxEventFailed(f.ctx, *first, "old", time.Second) },
	} {
		if owned, err := attempt(); err != nil || owned {
			t.Fatalf("%s affected newer claim: owned=%v err=%v", name, owned, err)
		}
	}
	if owned, err := f.repo.MarkClaimedOutboxEventDone(f.ctx, *second); err != nil || !owned {
		t.Fatalf("current completion=%v err=%v", owned, err)
	}
	if f.count("dynamic_outbox", bson.M{"status": models.DynamicOutboxStatusDone, "expireAt": bson.M{"$exists": true}}) != 1 {
		t.Fatal("successful current claim did not reach retained terminal state")
	}
	// Expired final attempts must reach terminal state instead of remaining stuck.
	exhausted := makeEvent()
	exhausted.Payload.IdempotencyKey = "exhausted"
	exhausted.Status = models.DynamicOutboxStatusProcessing
	exhausted.Attempts = 3
	if _, err := f.repo.InsertOutboxEvent(f.ctx, exhausted); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.RecoverExhaustedOutboxEvents(f.ctx, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if f.count("dynamic_outbox", bson.M{"_id": exhausted.ID, "status": models.DynamicOutboxStatusFailed, "expireAt": bson.M{"$exists": true}}) != 1 {
		t.Fatal("expired final attempt never became terminal")
	}
}

func TestWorkflowMongoCronRollsBackDataAndOutbox(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	// Scheduled work uses an explicit system identity; this fixture has no caller policy.
	f.container.Routes.CreateDynamicModelItem.Access = nil
	f.container.RowAccess = nil
	create := mongoWorkflowStep("create", models.WorkflowStepTypeCreateRecord, map[string]interface{}{"document": bson.M{"name": "scheduled"}})
	fail := mongoWorkflowStep("fail", models.WorkflowStepTypeFail, map[string]interface{}{"message": "scheduled rollback"})
	create.Order = 1
	fail.Order = 2
	workflow := models.DynamicWorkflow{Name: "scheduled", Trigger: models.WorkflowTriggerCron, Schedule: "* * * * *", Mode: models.WorkflowModeTransactional, RunInTransaction: true, IsActive: true, StopOnError: true, Steps: []models.DynamicWorkflowStep{create, fail}}
	if err := f.service.RunCronWorkflow(f.ctx, "tenant", "project", "orders", f.container, workflow, time.Now()); err == nil || !strings.Contains(err.Error(), "scheduled rollback") {
		t.Fatalf("cron error=%v", err)
	}
	if f.count(utils.GetProjectCollectionName("tenant", "project", "orders"), bson.M{}) != 0 || f.count("dynamic_outbox", bson.M{}) != 0 {
		t.Fatal("failed scheduled transaction left committed data/events")
	}
	workflow.Steps = workflow.Steps[:1]
	if err := f.service.RunCronWorkflow(f.ctx, "tenant", "project", "orders", f.container, workflow, time.Now()); err != nil {
		t.Fatal(err)
	}
	if f.count(utils.GetProjectCollectionName("tenant", "project", "orders"), bson.M{}) != 1 || f.count("dynamic_outbox", bson.M{}) != 1 {
		t.Fatal("successful scheduled transaction did not commit data and event")
	}
}

func TestWorkflowMongoReconciliationPreservesConcurrentManualEdits(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	translations := f.db.Collection("translations")
	r := repositories.NewLocalizationRepositoryWithCollections(translations, f.db.Collection("preferences"), f.db.Collection("translation_jobs"))
	for _, test := range []struct {
		name       string
		existing   bool
		manualHash string
		want       models.TranslationStatus
	}{
		{"existing same source", true, "new", models.TranslationStatusCurrent},
		{"existing outdated source", true, "old", models.TranslationStatusOutdated},
		{"concurrent manual insert", false, "new", models.TranslationStatusCurrent},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := models.TranslationEntry{TenantID: primitive.NewObjectID(), ProjectID: primitive.NewObjectID(), Locale: "tr", TranslationKey: "title", SourceText: "$source text", SourceHash: "old", Origin: models.TranslationOriginAI, TranslatedText: "stale AI", Status: models.TranslationStatusCurrent, IsActive: true}
			if test.existing {
				if _, err := translations.InsertOne(f.ctx, entry); err != nil {
					t.Fatal(err)
				}
			}
			snapshot := entry
			snapshot.SourceHash = "new"
			snapshot.Status = models.TranslationStatusOutdated
			// The human edit lands after discovery's snapshot was read.
			manual := entry
			manual.TranslatedText = "fresh human edit"
			manual.SourceHash = test.manualHash
			if err := r.UpsertManualTranslation(f.ctx, manual); err != nil {
				t.Fatal(err)
			}
			if err := r.ApplyReconciledEntries(f.ctx, []models.TranslationEntry{snapshot}); err != nil {
				t.Fatal(err)
			}
			var stored models.TranslationEntry
			if err := translations.FindOne(f.ctx, bson.M{"tenantId": entry.TenantID, "projectId": entry.ProjectID, "locale": "tr", "translationKey": "title"}).Decode(&stored); err != nil {
				t.Fatal(err)
			}
			if stored.TranslatedText != "fresh human edit" || stored.Origin != models.TranslationOriginManual || stored.Status != test.want || stored.SourceText != "$source text" {
				t.Fatalf("manual content/lifecycle changed: %#v", stored)
			}
		})
	}
}

func TestWorkflowMongoLocalizationLeaseFencesStaleWorkers(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	jobs := f.db.Collection("translation_jobs")
	r := repositories.NewLocalizationRepositoryWithCollections(f.db.Collection("translations"), f.db.Collection("preferences"), jobs)
	now := time.Now().UTC()
	job := &models.TranslationJob{ID: primitive.NewObjectID(), Status: models.TranslationJobPending, NextAttemptAt: now}
	if err := r.CreateJob(f.ctx, job); err != nil {
		t.Fatal(err)
	}
	first, err := r.LeaseNextJob(f.ctx, "worker-one", now, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.LeaseNextJob(f.ctx, "worker-two", now.Add(2*time.Second), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if first.LeaseToken == second.LeaseToken || first.LeaseToken == "" {
		t.Fatal("lease claim token was reused")
	}
	if err := r.CompleteJob(repositories.WithLocalizationJobClaim(f.ctx, *first), job.ID, 1, 0, 0, nil); !errors.Is(err, repositories.ErrLocalizationLeaseLost) {
		t.Fatalf("stale completion error=%v", err)
	}
	var current models.TranslationJob
	if err := jobs.FindOne(f.ctx, bson.M{"_id": job.ID}).Decode(&current); err != nil {
		t.Fatal(err)
	}
	if current.LeaseOwner != "worker-two" || current.Status != models.TranslationJobRunning {
		t.Fatal("stale worker overwrote current lease")
	}
	if err := r.CompleteJob(repositories.WithLocalizationJobClaim(f.ctx, *second), job.ID, 1, 0, 0, nil); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowMongoAuthSessionsExcludeDuplicateLegacyFamilies(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	collection := f.db.Collection("auth_sessions")
	expires := time.Now().Add(time.Hour)
	for i := 0; i < 2; i++ {
		if _, err := collection.InsertOne(f.ctx, models.AuthSession{FamilyID: "legacy", CurrentJTIHash: "old", ExpiresAt: expires}); err != nil {
			t.Fatal(err)
		}
	}
	r := repositories.NewAuthSessionRepositoryWithCollection(collection)
	if err := r.EnsureIndexes(f.ctx); err != nil {
		t.Fatal(err)
	}
	state, err := r.Rotate(f.ctx, "legacy", "old", "next", time.Now(), expires)
	if err != nil || state != models.RefreshRotationReuse {
		t.Fatalf("legacy family rotation=%s %v", state, err)
	}
	session := models.AuthSession{FamilyID: "fresh", CurrentJTIHash: "current", ExpiresAt: expires}
	if err := r.Create(f.ctx, session); err != nil {
		t.Fatal(err)
	}
	if err := r.Create(f.ctx, session); !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("duplicate fresh session error=%v", err)
	}
	if err := r.RevokeFamily(f.ctx, "legacy", time.Now()); err != nil {
		t.Fatal(err)
	}
	count, err := collection.CountDocuments(f.ctx, bson.M{"familyId": "legacy", "revokedAt": bson.M{"$exists": true}})
	if err != nil || count != 2 {
		t.Fatalf("legacy family revocation count=%d %v", count, err)
	}
}
