package utils

import (
	"context"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"testing"
)

func TestSyncContainerIndexesPreservesConstraintsOnCreationFailure(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()
	mt.Run("duplicate data prevents new index without dropping existing indexes", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		old := &models.ContainerModel{SchemaName: "people", Fields: []models.Field{{Name: "email", Unique: true}}}
		updated := &models.ContainerModel{SchemaName: "people", Fields: []models.Field{{Name: "email", Unique: true}, {Name: "phone", Unique: true}}}
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "name", Value: "idx_email_unique"}}),
			mtest.CreateSuccessResponse(), mtest.CreateCommandErrorResponse(mtest.CommandError{Code: 11000, Message: "duplicate key"}),
			mtest.CreateCommandErrorResponse(mtest.CommandError{Code: 27, Message: "index not found"}))
		if err := SyncContainerIndexes(context.Background(), old, updated, "tenant", "project"); err == nil {
			mt.Fatal("expected duplicate index creation failure")
		}
		for _, e := range mt.GetAllStartedEvents() {
			if e.CommandName == "dropIndexes" && e.Command.Lookup("index").StringValue() != "idx_phone_unique" {
				mt.Fatal("dropped constraints before new indexes succeeded")
			}
		}
	})
}

func TestSyncContainerIndexesRemovesOnlyRetiredDefinitions(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()
	mt.Run("remove old unique after ensuring new indexes", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		old := &models.ContainerModel{SchemaName: "people", Fields: []models.Field{{Name: "email", Unique: true}, {Name: "phone", Unique: true}}}
		updated := &models.ContainerModel{SchemaName: "people", Fields: []models.Field{{Name: "email", Unique: true}}}
		mt.AddMockResponses(mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "name", Value: "idx_email_unique"}}, bson.D{{Key: "name", Value: "idx_phone_unique"}}), mtest.CreateSuccessResponse(), mtest.CreateSuccessResponse())
		if err := SyncContainerIndexes(context.Background(), old, updated, "tenant", "project"); err != nil {
			mt.Fatal(err)
		}
		events := mt.GetAllStartedEvents()[1:]
		if len(events) != 2 || events[0].CommandName != "createIndexes" || events[1].CommandName != "dropIndexes" {
			mt.Fatalf("unexpected index commands: %v", events)
		}
		if got := events[1].Command.Lookup("index").StringValue(); got != "idx_phone_unique" {
			mt.Fatalf("dropped %q", got)
		}
		indexes := events[0].Command.Lookup("indexes").Array()
		values, _ := indexes.Values()
		var index bson.M
		if err := bson.Unmarshal(values[0].Document(), &index); err != nil {
			mt.Fatal(err)
		}
		if index["unique"] != true {
			mt.Fatalf("unique not enforced: %v", index)
		}
	})
}

func TestSyncContainerIndexesRollsBackPartialAdditions(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()
	mt.Run("failed addition followed by original definition retry", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		old := &models.ContainerModel{SchemaName: "people", Fields: []models.Field{{Name: "email", Unique: true}}}
		updated := &models.ContainerModel{SchemaName: "people", Fields: []models.Field{{Name: "email", Unique: true}, {Name: "phone", Unique: true}, {Name: "username", Unique: true}}}
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "name", Value: "idx_email_unique"}}),
			mtest.CreateSuccessResponse(), mtest.CreateSuccessResponse(), mtest.CreateCommandErrorResponse(mtest.CommandError{Code: 11000, Message: "duplicate key"}),
			mtest.CreateSuccessResponse(), mtest.CreateCommandErrorResponse(mtest.CommandError{Code: 27, Message: "index not found"}),
			mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "name", Value: "idx_email_unique"}}), mtest.CreateSuccessResponse())
		if err := SyncContainerIndexes(context.Background(), old, updated, "tenant", "project"); err == nil {
			mt.Fatal("expected error")
		}
		if err := SyncContainerIndexes(context.Background(), old, old, "tenant", "project"); err != nil {
			mt.Fatal(err)
		}
		dropped := map[string]bool{}
		for _, e := range mt.GetAllStartedEvents() {
			if e.CommandName == "dropIndexes" {
				dropped[e.Command.Lookup("index").StringValue()] = true
			}
		}
		if !dropped["idx_phone_unique"] || dropped["idx_email_unique"] || dropped["*"] {
			mt.Fatalf("incorrect rollback: %v", dropped)
		}
	})
}
