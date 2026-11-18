//go:build integration

package testmongo

import (
	"context"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"os"
	"strings"
	"testing"
	"time"
)

type Fixture struct {
	DB     *mongo.Database
	Client *mongo.Client
	Ctx    context.Context
}

func New(t *testing.T) *Fixture {
	t.Helper()
	uri := os.Getenv("AUTOTABLE_COLLABORATION_MONGO_URI")
	if uri == "" {
		t.Skip("isolated collaboration MongoDB replica set not supplied")
	}
	if !strings.HasPrefix(uri, "mongodb://127.0.0.1:") {
		t.Fatal("refusing non-loopback collaboration test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	db := client.Database("collaboration_test_" + primitive.NewObjectID().Hex())
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = db.Drop(cleanup)
		_ = client.Disconnect(cleanup)
		cancel()
	})
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatal(err)
	}
	return &Fixture{DB: db, Client: client, Ctx: ctx}
}
