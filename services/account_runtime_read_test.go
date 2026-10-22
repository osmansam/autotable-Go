package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestAccountTableAndSelectionReadsOmitHashedAndRestrictedData(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	for _, schema := range []string{"auth", "role"} {
		for _, mode := range []string{"table", "selection"} {
			mt.Run(schema+"/"+mode, func(mt *mtest.T) {
				container := &models.ContainerModel{SchemaName: schema, IsAuthContainer: schema == "auth", Fields: []models.Field{
					{Name: "name", Type: "string"}, {Name: "password", Type: "string", IsHashed: true},
					{Name: "profile", Type: "object", Children: []models.Field{{Name: "label", Type: "string"}, {Name: "recovery", Type: "string", IsHashed: true}, {Name: "internal", Type: "string", IsAuthorized: true, AuthorizeRole: []string{"admin"}}}},
				}}
				mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".tenant_t_project_p_"+schema, mtest.FirstBatch, bson.D{
					{Key: "name", Value: "Visible User"}, {Key: "password", Value: "top-level-hash-marker"}, {Key: "profile", Value: bson.D{{Key: "label", Value: "Visible Label"}, {Key: "recovery", Value: "nested-hash-marker"}, {Key: "internal", Value: "restricted-marker"}}},
				}))
				service := &DynamicService{repository: repositories.NewDynamicRepositoryWithDatabase(mt.DB)}
				var result interface{}
				var err error
				if mode == "table" {
					result, err = service.GetTableSource(context.Background(), GetTableSourceInput{TenantID: "t", ProjectID: "p", Schema: schema, SourceType: "schema", Container: container})
				} else {
					result, err = service.GetItemsForSelection(context.Background(), GetItemsForSelectionInput{TenantID: "t", ProjectID: "p", Schema: schema, FieldName: "profile", Container: container})
				}
				if err != nil {
					mt.Fatal(err)
				}
				encoded, err := json.Marshal(result)
				if err != nil {
					mt.Fatal(err)
				}
				for _, secret := range []string{"top-level-hash-marker", "nested-hash-marker", "restricted-marker"} {
					if strings.Contains(string(encoded), secret) {
						mt.Errorf("%s leaked: %s", secret, encoded)
					}
				}
				if !strings.Contains(string(encoded), "Visible Label") {
					mt.Fatalf("readable field missing: %s", encoded)
				}
			})
		}
	}
}
