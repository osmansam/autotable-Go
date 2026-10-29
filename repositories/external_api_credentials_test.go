package repositories

import (
	"context"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"testing"
)

func TestSharedExternalCredentialRequiresLiveGrant(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("ungranted tenant key denied", func(mt *mtest.T) {
		tenant, project, id := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".external_api_credentials", mtest.FirstBatch, bson.D{{Key: "_id", Value: id}, {Key: "tenantId", Value: tenant}, {Key: "scope", Value: "tenant"}}), mtest.CreateCursorResponse(0, mt.DB.Name()+".external_api_credential_grants", mtest.FirstBatch))
		_, err := mockRepository(mt.Coll).GetExternalAPICredential(context.Background(), tenant.Hex(), project.Hex(), id.Hex())
		if err != mongo.ErrNoDocuments {
			mt.Fatalf("ungranted credential resolved: %v", err)
		}
	})
	mt.Run("granted tenant key allowed", func(mt *mtest.T) {
		tenant, project, id := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".external_api_credentials", mtest.FirstBatch, bson.D{{Key: "_id", Value: id}, {Key: "tenantId", Value: tenant}, {Key: "scope", Value: "tenant"}}), mtest.CreateCursorResponse(0, mt.DB.Name()+".external_api_credential_grants", mtest.FirstBatch, bson.D{{Key: "tenantId", Value: tenant}, {Key: "projectId", Value: project}, {Key: "credentialId", Value: id}}))
		got, err := mockRepository(mt.Coll).GetExternalAPICredential(context.Background(), tenant.Hex(), project.Hex(), id.Hex())
		if err != nil || got.ID != id {
			mt.Fatalf("granted key denied: %#v %v", got, err)
		}
	})
	_ = models.ExternalAPICredential{}
}

func TestProjectCredentialListDoesNotRevealOtherProjectGrants(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("project metadata projects limited to its own grant", func(mt *mtest.T) {
		tenant, project, other, id := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".credentials", mtest.FirstBatch, bson.D{{Key: "_id", Value: id}, {Key: "tenantId", Value: tenant}, {Key: "scope", Value: "tenant"}}), mtest.CreateCursorResponse(0, mt.DB.Name()+".grants", mtest.FirstBatch, bson.D{{Key: "projectId", Value: project}}, bson.D{{Key: "projectId", Value: other}}))
		got, err := mockRepository(mt.Coll).ListExternalAPICredentials(context.Background(), tenant.Hex(), project.Hex())
		if err != nil || len(got) != 1 {
			mt.Fatalf("list: %v %v", got, err)
		}
		if len(got[0].GrantedProjectIDs) != 1 || got[0].GrantedProjectIDs[0] != project {
			mt.Fatalf("other project grants exposed: %v", got[0].GrantedProjectIDs)
		}
	})
}
