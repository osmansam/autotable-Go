package repositories

import (
	"context"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"testing"
	"time"
)

func TestManagementRevokedFamilyDenied(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("revoked or missing", func(mt *mtest.T) {
		repo := NewAuthSessionRepositoryWithCollection(mt.Coll)
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+"."+mt.Coll.Name(), mtest.FirstBatch))
		err := repo.ValidateManagementFamily(context.Background(), models.ManagementPrincipal{UserID: "user", TenantID: "tenant", FamilyID: "family", Scope: "tenant"}, time.Now())
		if err == nil {
			mt.Fatal("revoked family allowed")
		}
		filter := mt.GetStartedEvent().Command.Lookup("filter").Document()
		if filter.Lookup("identity").StringValue() != "management" || filter.Lookup("tenantId").StringValue() != "tenant" || filter.Lookup("userId").StringValue() != "user" {
			mt.Fatalf("family crossed identity scope: %v", filter)
		}
		if filter.Lookup("revokedAt").Type == 0 {
			mt.Fatal("revocation omitted")
		}
	})
}
func TestManagementRuntimeFamilyCannotAuthenticate(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("runtime family excluded from revocation", func(mt *mtest.T) {
		repo := NewAuthSessionRepositoryWithCollection(mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}))
		if err := repo.RevokeUserProjectSessions(context.Background(), "user", "tenant", "project", time.Now()); err != nil {
			mt.Fatal(err)
		}
		updates, _ := mt.GetStartedEvent().Command.Lookup("updates").Array().Values()
		if updates[0].Document().Lookup("q").Document().Lookup("identity").StringValue() != "management" {
			mt.Fatal("revocation includes application sessions")
		}
	})
}
