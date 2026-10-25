package controllers

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestContainerCacheInvalidationAlsoInvalidatesWorkflowResults(t *testing.T) {
	server := miniredis.RunT(t)
	previous := configs.RedisClient
	configs.RedisClient = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = configs.RedisClient.Close(); configs.RedisClient = previous })
	ctx := context.Background()
	key := "tenant:tenant:project:project:workflow-cache:revision"
	if err := configs.RedisClient.Set(ctx, key, 7, 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := configs.RedisClient.Set(ctx, "container-list", "old metadata", 0).Err(); err != nil {
		t.Fatal(err)
	}
	invalidateContainerCaches(ctx, "tenant", "project", "container-list")
	if got, err := configs.RedisClient.Get(ctx, key).Int64(); err != nil || got != 8 {
		t.Fatalf("workflow revision = %d, %v; want 8", got, err)
	}
	if server.Exists("container-list") {
		t.Fatal("old container metadata is still cached")
	}
}

func TestCommittedContainerWriteInvalidatesBeforeReferenceSyncFailure(t *testing.T) {
	server := miniredis.RunT(t)
	previous := configs.RedisClient
	configs.RedisClient = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = configs.RedisClient.Close(); configs.RedisClient = previous })
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("reference update fails after primary write", func(mt *mtest.T) {
		ctx := context.Background()
		key := "tenant:tenant:project:project:workflow-cache:revision"
		if err := configs.RedisClient.Set(ctx, key, 7, 0).Err(); err != nil {
			mt.Fatal(err)
		}
		mt.AddMockResponses(mtest.CreateCommandErrorResponse(mtest.CommandError{Code: 91, Message: "reference sync failed"}))
		_, err := syncContainerInvalidationsAfterWrite(ctx, mt.Coll, "tenant", "project", models.ContainerModel{}, models.ContainerModel{SchemaName: "orders", Fields: []models.Field{{Name: "users", Type: "objectId", ObjectSchemaName: "users"}}}, map[string]primitive.ObjectID{"users": primitive.NewObjectID()})
		if err == nil {
			mt.Fatal("expected secondary reference update failure")
		}
		if got, _ := configs.RedisClient.Get(ctx, key).Int64(); got != 8 {
			mt.Fatalf("primary write remained cached after secondary failure; revision=%d", got)
		}
	})
}
