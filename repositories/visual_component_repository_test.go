package repositories

import (
	"context"
	"encoding/json"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"testing"
)

func TestVisualComponentsAreProjectScopedAndRevisionChecked(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("list scope", func(mt *mtest.T) {
		r := NewVisualComponentRepository()
		r.collection = func() *mongo.Collection { return mt.Coll }
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+"."+mt.Coll.Name(), mtest.FirstBatch))
		result, err := r.List(context.Background(), "tenant-a", "project-a")
		if err != nil {
			mt.Fatal(err)
		}
		if len(result) != 0 {
			mt.Fatal(result)
		}
		filter := mt.GetStartedEvent().Command.Lookup("filter").Document()
		if filter.Lookup("tenantId").StringValue() != "tenant-a" || filter.Lookup("projectId").StringValue() != "project-a" {
			mt.Fatal(filter)
		}
	})
	mt.Run("stale update", func(mt *mtest.T) {
		r := NewVisualComponentRepository()
		r.collection = func() *mongo.Collection { return mt.Coll }
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "value", Value: nil}))
		_, err := r.Update(context.Background(), "tenant-a", "project-a", models.VisualComponentDesign{ID: primitive.NewObjectID(), Revision: 4, Name: "Card", Value: json.RawMessage(`{}`)})
		if err != ErrVisualComponentConflict {
			mt.Fatalf("error=%v", err)
		}
		filter := mt.GetStartedEvent().Command.Lookup("query").Document()
		if filter.Lookup("tenantId").StringValue() != "tenant-a" || filter.Lookup("projectId").StringValue() != "project-a" || filter.Lookup("revision").Int64() != 4 {
			mt.Fatal(filter)
		}
	})
	mt.Run("scope required", func(mt *mtest.T) {
		r := NewVisualComponentRepository()
		if _, err := r.List(context.Background(), "", "project"); err == nil {
			mt.Fatal("empty tenant accepted")
		}
	})
}

func TestVisualComponentTemplateCopyGetsNewScopeAndIdentity(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("independent copied definitions", func(mt *mtest.T) {
		r := NewVisualComponentRepository()
		r.collection = func() *mongo.Collection { return mt.Coll }
		id := primitive.NewObjectID()
		raw := []byte(`{"document":{"version":2,"id":"doc","name":"Card","root":{"id":"root","type":"section","children":[]}}}`)
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+"."+mt.Coll.Name(), mtest.FirstBatch, bson.D{{Key: "_id", Value: id}, {Key: "tenantId", Value: "source"}, {Key: "projectId", Value: "old"}, {Key: "name", Value: "Card"}, {Key: "value", Value: raw}, {Key: "revision", Value: int64(4)}}), mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}))
		if err := r.Clone(context.Background(), "source", "old", "target", "new"); err != nil {
			mt.Fatal(err)
		}
		for _, event := range mt.GetAllStartedEvents() {
			if event.CommandName == "insert" {
				docs, _ := event.Command.Lookup("documents").Array().Values()
				row := docs[0].Document()
				if row.Lookup("tenantId").StringValue() != "target" || row.Lookup("projectId").StringValue() != "new" || row.Lookup("_id").ObjectID() == id || row.Lookup("revision").Int64() != 1 {
					mt.Fatal(row)
				}
			}
		}
	})
}
