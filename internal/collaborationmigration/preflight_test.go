package collaborationmigration

import (
	"context"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"testing"
)

func conflictingUsers(mt *mtest.T) {
	for _, collection := range []string{"users", "tenants", "tenant_memberships", "project_memberships", "projects"} {
		var docs []bson.D
		if collection == "users" {
			docs = []bson.D{{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "email", Value: "Alice@example.com"}}, {{Key: "_id", Value: primitive.NewObjectID()}, {Key: "email", Value: " alice@EXAMPLE.com "}}}
		}
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+"."+collection, mtest.FirstBatch, docs...))
	}
}
func TestCollaborationPreflightReportsWithoutMutation(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("duplicate normalized accounts", func(mt *mtest.T) {
		conflictingUsers(mt)
		report, err := Preflight(context.Background(), mt.DB)
		if err != nil {
			mt.Fatal(err)
		}
		if len(report.Conflicts) == 0 || report.Hash == "" {
			mt.Fatalf("identity collisions not reported: %#v", report)
		}
		for _, event := range mt.GetAllStartedEvents() {
			if event.CommandName != "find" {
				mt.Fatalf("preflight mutated database: %s", event.CommandName)
			}
		}
	})
}
func TestCollaborationApplyRejectsConflicts(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("no unsafe normalization", func(mt *mtest.T) {
		conflictingUsers(mt)
		if err := Apply(context.Background(), mt.DB, "reviewed-report"); err == nil {
			mt.Fatal("conflicting accounts applied")
		}
		for _, event := range mt.GetAllStartedEvents() {
			if event.CommandName != "find" {
				mt.Fatalf("conflicting apply mutated database: %s", event.CommandName)
			}
		}
	})
}
