package repositories

import (
	"context"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"testing"
	"time"
)

func TestAuthSessionsEnforceFreshUniqueKeys(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("indexes", func(mt *mtest.T) {
		r := NewAuthSessionRepositoryWithCollection(mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse())
		if err := r.EnsureIndexes(context.Background()); err != nil {
			mt.Fatal(err)
		}
		indexes := mt.GetStartedEvent().Command.Lookup("indexes").Array()
		found := false
		values, _ := indexes.Values()
		for _, value := range values {
			index := value.Document()
			if index.Lookup("key").Document().Lookup("sessionKey").Type != 0 {
				found = index.Lookup("unique").Boolean() && index.Lookup("sparse").Boolean()
			}
		}
		if !found {
			mt.Fatal("missing sparse unique sessionKey index")
		}
	})
	mt.Run("create", func(mt *mtest.T) {
		r := NewAuthSessionRepositoryWithCollection(mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}))
		if err := r.Create(context.Background(), models.AuthSession{FamilyID: "family", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
			mt.Fatal(err)
		}
		document := mt.GetStartedEvent().Command.Lookup("documents").Array().Index(0).Value().Document()
		if document.Lookup("sessionKey").StringValue() != "family" {
			mt.Fatal("fresh session lacks unique family key")
		}
	})
	mt.Run("rotation excludes ambiguous legacy rows", func(mt *mtest.T) {
		r := NewAuthSessionRepositoryWithCollection(mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
		if _, err := r.Rotate(context.Background(), "family", "current", "next", time.Now(), time.Now().Add(time.Hour)); err != nil {
			mt.Fatal(err)
		}
		filter := mt.GetStartedEvent().Command.Lookup("updates").Array().Index(0).Value().Document().Lookup("q").Document()
		if filter.Lookup("sessionKey").StringValue() != "family" {
			mt.Fatal("legacy duplicate rows can still rotate")
		}
	})
}
