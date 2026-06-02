package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestDynamicRepositoryCRUD(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()

	mt.Run("insert", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}))
		if _, err := repository.Insert(context.Background(), "tenant", "project", "orders", map[string]interface{}{"name": "Ada"}); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
	})

	mt.Run("insert many", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 2}))
		if _, err := repository.InsertMany(context.Background(), "tenant", "project", "orders", []map[string]interface{}{{"name": "Ada"}, {"name": "Lin"}}); err != nil {
			t.Fatalf("InsertMany() error = %v", err)
		}
	})

	mt.Run("find by id", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		id := primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.Coll.Database().Name()+"."+mt.Coll.Name(), mtest.FirstBatch, bson.D{{Key: "_id", Value: id}, {Key: "name", Value: "Ada"}}))
		got, err := repository.FindByID(context.Background(), "tenant", "project", "orders", id)
		if err != nil || got["name"] != "Ada" {
			t.Fatalf("FindByID() = %#v, %v", got, err)
		}
	})

	mt.Run("find one", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.Coll.Database().Name()+"."+mt.Coll.Name(), mtest.FirstBatch, bson.D{{Key: "name", Value: "Ada"}}))
		got, err := repository.FindOne(context.Background(), "tenant", "project", "orders", bson.M{"name": "Ada"})
		if err != nil || got["name"] != "Ada" {
			t.Fatalf("FindOne() = %#v, %v", got, err)
		}
	})

	mt.Run("find selection", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.Coll.Database().Name()+"."+mt.Coll.Name(), mtest.FirstBatch, bson.D{{Key: "name", Value: "Ada"}}))
		got, err := repository.FindForSelection(context.Background(), "tenant", "project", "orders", "name")
		if err != nil || len(got) != 1 || got[0]["name"] != "Ada" {
			t.Fatalf("FindForSelection() = %#v, %v", got, err)
		}
	})

	mt.Run("find all", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.Coll.Database().Name()+"."+mt.Coll.Name(), mtest.FirstBatch, bson.D{{Key: "name", Value: "Ada"}}))
		got, err := repository.FindAll(context.Background(), "tenant", "project", "orders", 10)
		if err != nil || len(got) != 1 || got[0]["name"] != "Ada" {
			t.Fatalf("FindAll() = %#v, %v", got, err)
		}
	})

	mt.Run("query paginated", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, mt.Coll.Database().Name()+"."+mt.Coll.Name(), mtest.FirstBatch, bson.D{{Key: "name", Value: "Ada"}}),
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: int32(3)}),
		)
		pager := &utils.Pager{Enabled: true, Page: 1, Limit: 2}
		got, err := repository.Query(context.Background(), "tenant", "project", "orders", bson.M{}, nil, pager)
		if err != nil || len(got) != 1 || pager.TotalItems != 3 || pager.TotalPages != 2 {
			t.Fatalf("Query() = %#v, %v; pager = %#v", got, err, pager)
		}
	})

	mt.Run("delete and update", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}),
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 2}),
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}),
		)
		if _, err := repository.DeleteByID(context.Background(), "tenant", "project", "orders", "id"); err != nil {
			t.Fatalf("DeleteByID() error = %v", err)
		}
		if _, err := repository.DeleteManyByField(context.Background(), "tenant", "project", "orders", "owner", "id"); err != nil {
			t.Fatalf("DeleteManyByField() error = %v", err)
		}
		if _, err := repository.UpdateByID(context.Background(), "tenant", "project", "orders", "id", map[string]interface{}{"name": "Ada"}); err != nil {
			t.Fatalf("UpdateByID() error = %v", err)
		}
	})

	mt.Run("count variants", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		namespace := mt.Coll.Database().Name() + "." + mt.Coll.Name()
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, namespace, mtest.FirstBatch, bson.D{{Key: "n", Value: int32(1)}}),
			mtest.CreateCursorResponse(0, namespace, mtest.FirstBatch, bson.D{{Key: "n", Value: int32(2)}}),
			mtest.CreateCursorResponse(0, namespace, mtest.FirstBatch, bson.D{{Key: "n", Value: int32(3)}}),
		)
		if got, err := repository.CountByField(context.Background(), "tenant", "project", "orders", "state", "open"); err != nil || got != 1 {
			t.Fatalf("CountByField() = %d, %v", got, err)
		}
		if got, err := repository.CountByFieldIn(context.Background(), "tenant", "project", "orders", "state", []interface{}{"open", "closed"}); err != nil || got != 2 {
			t.Fatalf("CountByFieldIn() = %d, %v", got, err)
		}
		if got, err := repository.CountByFieldExcludingID(context.Background(), "tenant", "project", "orders", "state", "open", primitive.NewObjectID()); err != nil || got != 3 {
			t.Fatalf("CountByFieldExcludingID() = %d, %v", got, err)
		}
	})

	mt.Run("execute pipeline", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.Coll.Database().Name()+"."+mt.Coll.Name(), mtest.FirstBatch, bson.D{{Key: "state", Value: "open"}}))
		got, err := repository.ExecutePipeline(context.Background(), "tenant", "project", "orders", models.PipelineStage{PipelineJSON: `[{"$match":{"state":"open"}}]`})
		if err != nil || len(got) != 1 || got[0]["state"] != "open" {
			t.Fatalf("ExecutePipeline() = %#v, %v", got, err)
		}
	})
}

func TestDynamicRepositoryOutbox(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()

	mt.Run("insert event", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		event := models.DynamicOutboxEvent{ID: primitive.NewObjectID()}
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}))
		if _, err := repository.InsertOutboxEvent(context.Background(), event); err != nil {
			t.Fatalf("InsertOutboxEvent() error = %v", err)
		}
	})

	mt.Run("idempotent insert event", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		event := models.DynamicOutboxEvent{ID: primitive.NewObjectID(), Payload: models.DynamicOutboxPayload{IdempotencyKey: "key"}}
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}))
		if got, err := repository.InsertOutboxEvent(context.Background(), event); err != nil || got.InsertedID != event.ID {
			t.Fatalf("InsertOutboxEvent() = %#v, %v", got, err)
		}
	})

	mt.Run("claim pending event", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		id := primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "value", Value: bson.D{
			{Key: "_id", Value: id},
			{Key: "status", Value: models.DynamicOutboxStatusProcessing},
			{Key: "attempts", Value: 1},
		}}))
		got, err := repository.ClaimPendingOutboxEvent(context.Background(), time.Now())
		if err != nil || got.ID != id || got.Status != models.DynamicOutboxStatusProcessing {
			t.Fatalf("ClaimPendingOutboxEvent() = %#v, %v", got, err)
		}
	})

	mt.Run("mark event done", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
		if err := repository.MarkOutboxEventDone(context.Background(), primitive.NewObjectID()); err != nil {
			t.Fatalf("MarkOutboxEventDone() error = %v", err)
		}
	})

	for _, tt := range []struct {
		name     string
		attempts int
	}{
		{name: "retry event", attempts: 1},
		{name: "terminal failure", attempts: 3},
	} {
		mt.Run(tt.name, func(mt *mtest.T) {
			repository := mockRepository(mt.Coll)
			mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
			event := models.DynamicOutboxEvent{ID: primitive.NewObjectID(), Attempts: tt.attempts, MaxAttempts: 3}
			if err := repository.MarkOutboxEventFailed(context.Background(), event, "failure", time.Second); err != nil {
				t.Fatalf("MarkOutboxEventFailed() error = %v", err)
			}
		})
	}

	mt.Run("empty workflow idempotency key", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		if done, err := repository.WorkflowStepExecutionDone(context.Background(), ""); err != nil || done {
			t.Fatalf("WorkflowStepExecutionDone() = %v, %v", done, err)
		}
		if err := repository.MarkWorkflowStepExecutionDone(context.Background(), "", primitive.NewObjectID()); err != nil {
			t.Fatalf("MarkWorkflowStepExecutionDone() error = %v", err)
		}
	})

	mt.Run("workflow execution found", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.Coll.Database().Name()+"."+mt.Coll.Name(), mtest.FirstBatch, bson.D{{Key: "idempotencyKey", Value: "key"}}))
		if done, err := repository.WorkflowStepExecutionDone(context.Background(), "key"); err != nil || !done {
			t.Fatalf("WorkflowStepExecutionDone() = %v, %v", done, err)
		}
	})

	mt.Run("workflow execution missing", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.Coll.Database().Name()+"."+mt.Coll.Name(), mtest.FirstBatch))
		if done, err := repository.WorkflowStepExecutionDone(context.Background(), "key"); err != nil || done {
			t.Fatalf("WorkflowStepExecutionDone() = %v, %v", done, err)
		}
	})

	mt.Run("mark workflow execution done", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}))
		if err := repository.MarkWorkflowStepExecutionDone(context.Background(), "key", primitive.NewObjectID()); err != nil {
			t.Fatalf("MarkWorkflowStepExecutionDone() error = %v", err)
		}
	})

	mt.Run("ensure indexes", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(
			mtest.CreateSuccessResponse(),
			mtest.CreateSuccessResponse(),
		)
		if err := repository.EnsureOutboxIndexes(context.Background()); err != nil {
			t.Fatalf("EnsureOutboxIndexes() error = %v", err)
		}
	})
}

func TestNewDynamicRepository(t *testing.T) {
	repository := NewDynamicRepository()
	if repository == nil || repository.collection == nil || repository.globalCollection == nil || repository.getContainerModel == nil || repository.getContainerModels == nil || repository.nextSequence == nil {
		t.Fatalf("NewDynamicRepository() = %#v", repository)
	}
}

func TestDynamicRepositoryDelegates(t *testing.T) {
	container := &models.ContainerModel{SchemaName: "orders"}
	transactionCalled := false
	repository := &DynamicRepository{
		getContainerModel: func(context.Context, string, string, string) (*models.ContainerModel, error) {
			return container, nil
		},
		getContainerModels: func(context.Context) ([]models.ContainerModel, error) {
			return []models.ContainerModel{*container}, nil
		},
		nextSequence: func(context.Context, string) (int64, error) {
			return 7, nil
		},
		withTransaction: func(_ context.Context, fn func(mongo.SessionContext) error) error {
			transactionCalled = true
			return fn(nil)
		},
	}
	if got, err := repository.GetContainerModel(context.Background(), "tenant", "project", "orders"); err != nil || got != container {
		t.Fatalf("GetContainerModel() = %#v, %v", got, err)
	}
	if got, err := repository.GetAllContainerModels(context.Background()); err != nil || len(got) != 1 || got[0].SchemaName != "orders" {
		t.Fatalf("GetAllContainerModels() = %#v, %v", got, err)
	}
	if got, err := repository.NextSequence(context.Background(), "orders"); err != nil || got != 7 {
		t.Fatalf("NextSequence() = %d, %v", got, err)
	}
	callbackCalled := false
	if err := repository.WithTransaction(context.Background(), func(mongo.SessionContext) error {
		callbackCalled = true
		return nil
	}); err != nil || !transactionCalled || !callbackCalled {
		t.Fatalf("WithTransaction() error = %v, transactionCalled = %v, callbackCalled = %v", err, transactionCalled, callbackCalled)
	}
}

func mockRepository(collection *mongo.Collection) *DynamicRepository {
	return &DynamicRepository{
		collection:       func(_, _, _ string) *mongo.Collection { return collection },
		globalCollection: func(_ string) *mongo.Collection { return collection },
	}
}
