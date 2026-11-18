package repositories

import (
	"context"
	"errors"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"testing"
)

func collaborationMockSnapshot(mt *mtest.T, tenant, user, project primitive.ObjectID, role string) {
	docs := []struct {
		name string
		doc  bson.D
	}{
		{"users", bson.D{{Key: "_id", Value: user}, {Key: "isActive", Value: true}}},
		{"tenants", bson.D{{Key: "_id", Value: tenant}, {Key: "isActive", Value: true}}},
		{"tenant_memberships", bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "status", Value: "active"}, {Key: "roles", Value: bson.A{role}}}},
	}
	if !project.IsZero() {
		docs = append(docs, struct {
			name string
			doc  bson.D
		}{"projects", bson.D{{Key: "_id", Value: project}, {Key: "tenantId", Value: tenant}, {Key: "isActive", Value: true}}}, struct {
			name string
			doc  bson.D
		}{"project_memberships", bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "status", Value: "active"}, {Key: "roles", Value: bson.A{models.ProjectRoleViewer}}}})
	}
	for _, d := range docs {
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+"."+d.name, mtest.FirstBatch, d.doc))
	}
}
func TestManagementProjectDiscoveryAssignedOnly(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("empty assigned list", func(mt *mtest.T) {
		tenant, user := primitive.NewObjectID(), primitive.NewObjectID()
		collaborationMockSnapshot(mt, tenant, user, primitive.NilObjectID, models.TenantRoleMember)
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".projects", mtest.FirstBatch))
		result, err := NewManagementProjectAccessRepository(mt.DB).ListVisibleProjects(context.Background(), models.ManagementAuthorizationSubject{UserID: user.Hex(), TenantID: tenant.Hex()})
		if err != nil {
			mt.Fatal(err)
		}
		if result == nil || len(result) != 0 {
			mt.Fatalf("empty list is not []: %#v", result)
		}
		events := mt.GetAllStartedEvents()
		last := events[len(events)-1]
		if last.CommandName != "aggregate" {
			mt.Fatalf("assigned visibility omitted: %v", last.CommandName)
		}
	})
}
func TestManagementProjectDirectLookupHidden(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("unknown tenant role cannot use project membership", func(mt *mtest.T) {
		tenant, user, project := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
		collaborationMockSnapshot(mt, tenant, user, project, "unknown")
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".projects", mtest.FirstBatch, bson.D{{Key: "_id", Value: project}, {Key: "tenantId", Value: tenant}, {Key: "isActive", Value: true}}))
		_, err := NewManagementProjectAccessRepository(mt.DB).GetVisibleProject(context.Background(), models.ManagementAuthorizationSubject{UserID: user.Hex(), TenantID: tenant.Hex()}, project.Hex())
		if !errors.Is(err, mongo.ErrNoDocuments) {
			mt.Fatalf("unknown tenant membership disclosed project: %v", err)
		}
	})
}
