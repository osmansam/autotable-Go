package utils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestMongoBackedUtilityHelpers(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()

	mt.Run("audit insert and event upsert reuse indexes", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		auditLogIndexCollections.Delete(mt.Coll.Name())

		mt.AddMockResponses(
			mtest.CreateSuccessResponse(),
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}),
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}),
		)
		if err := LogAudit(context.Background(), models.AuditLog{TenantID: "tenant", ProjectID: "project"}); err != nil {
			t.Fatalf("LogAudit(insert) error = %v", err)
		}
		if err := LogAudit(context.Background(), models.AuditLog{TenantID: "tenant", ProjectID: "project", EventID: primitive.NewObjectID()}); err != nil {
			t.Fatalf("LogAudit(upsert) error = %v", err)
		}
	})

	mt.Run("audit action wrappers", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		auditLogIndexCollections.Delete(mt.Coll.Name())
		mt.AddMockResponses(
			mtest.CreateSuccessResponse(),
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}),
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}),
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}),
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}),
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}),
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}),
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}),
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}),
		)
		ctx := context.Background()
		id := primitive.NewObjectID()
		container := &models.ContainerModel{SchemaName: "orders"}
		user := &models.AuditUser{ID: primitive.NewObjectID(), Email: "ada@example.com", Roles: []string{"admin"}}
		doc := map[string]interface{}{"_id": id}
		for name, run := range map[string]func() error{
			"create": func() error { return LogCreateAction(ctx, "tenant", "project", container, user, doc) },
			"update": func() error { return LogUpdateAction(ctx, "tenant", "project", container, user, doc, doc) },
			"delete": func() error { return LogDeleteAction(ctx, "tenant", "project", container, user, doc) },
			"bulk create": func() error {
				return LogBulkCreateAction(ctx, "tenant", "project", container, user, []interface{}{doc})
			},
			"bulk update": func() error {
				return LogBulkUpdateAction(ctx, "tenant", "project", container, user, []interface{}{doc}, []interface{}{doc})
			},
			"bulk delete": func() error {
				return LogBulkDeleteAction(ctx, "tenant", "project", container, user, []interface{}{doc})
			},
			"login":  func() error { return LogLogin(ctx, "tenant", "project", user, "127.0.0.1", "test") },
			"logout": func() error { return LogLogout(ctx, "tenant", "project", user, "127.0.0.1", "test") },
		} {
			if err := run(); err != nil {
				t.Fatalf("%s audit error = %v", name, err)
			}
		}
		if err := LogCreateAction(ctx, "tenant", "project", nil, user, doc); err != nil {
			t.Fatalf("LogCreateAction(nil) error = %v", err)
		}
	})

	mt.Run("gets paginated audit logs", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "n", Value: int32(1)}}),
			mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "action", Value: "create"}}),
		)
		app := fiber.New()
		app.Get("/", func(c *fiber.Ctx) error {
			got, pager, err := GetAuditLogs(context.Background(), c)
			if err != nil || len(got) != 1 || pager == nil || pager.TotalItems != 1 {
				t.Fatalf("GetAuditLogs() = %#v, %#v, %v", got, pager, err)
			}
			return nil
		})
		if _, err := app.Test(httptest.NewRequest(http.MethodGet, "/?tenantID=tenant&projectID=project&page=1&limit=2", nil)); err != nil {
			t.Fatalf("app.Test() error = %v", err)
		}
	})

	mt.Run("container model sorts fields", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		mt.AddMockResponses(mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{
			{Key: "schemaName", Value: "orders"},
			{Key: "fields", Value: bson.A{
				bson.D{{Key: "name", Value: "second"}, {Key: "order", Value: int32(2)}},
				bson.D{{Key: "name", Value: "first"}, {Key: "order", Value: int32(1)}},
			}},
		}))
		got, err := GetContainerModelWithContext(nil, "tenant", "project", "orders")
		if err != nil || len(got.Fields) != 2 || got.Fields[0].Name != "first" {
			t.Fatalf("GetContainerModelWithContext() = %#v, %v", got, err)
		}
	})

	mt.Run("lists global containers", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		mt.AddMockResponses(mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "schemaName", Value: "orders"}}))
		got, err := GetAllContainerModelsWithContext(nil)
		if err != nil || len(got) != 1 || got[0].SchemaName != "orders" {
			t.Fatalf("GetAllContainerModelsWithContext() = %#v, %v", got, err)
		}
	})

	mt.Run("container wrapper helpers", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "schemaName", Value: "orders"}}),
			mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "schemaName", Value: "legacy"}}),
			mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "schemaName", Value: "orders"}}),
		)
		if got, err := GetContainerModel("tenant", "project", "orders"); err != nil || got.SchemaName != "orders" {
			t.Fatalf("GetContainerModel() = %#v, %v", got, err)
		}
		if got, err := GetContainerModelLegacy("legacy"); err != nil || got.SchemaName != "legacy" {
			t.Fatalf("GetContainerModelLegacy() = %#v, %v", got, err)
		}
		if got, err := GetAllContainerModels(); err != nil || len(got) != 1 || got[0].SchemaName != "orders" {
			t.Fatalf("GetAllContainerModels() = %#v, %v", got, err)
		}
	})

	mt.Run("explicit audit index setup", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		auditLogIndexCollections.Delete(mt.Coll.Name())
		mt.AddMockResponses(mtest.CreateSuccessResponse())
		if err := EnsureAuditLogIndexes(context.Background(), "tenant", "project"); err != nil {
			t.Fatalf("EnsureAuditLogIndexes() error = %v", err)
		}
	})

	mt.Run("audit logs config", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{
				{Key: "key", Value: "audit_logs"},
				{Key: "auditLogs", Value: bson.D{{Key: "isAuthorized", Value: true}, {Key: "authorizeRole", Value: bson.A{"admin"}}}},
			}),
			mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch),
		)
		if got, err := GetAuditLogsConfig(); err != nil || !got.IsAuthorized || !reflect.DeepEqual(got.AuthorizeRole, []string{"admin"}) {
			t.Fatalf("GetAuditLogsConfig() = %#v, %v", got, err)
		}
		if got, err := GetAuditLogsConfig(); err != nil || got.IsAuthorized || len(got.AuthorizeRole) != 0 {
			t.Fatalf("GetAuditLogsConfig(default) = %#v, %v", got, err)
		}
	})

	mt.Run("increments sequence", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "value", Value: bson.D{{Key: "seq", Value: int64(4)}}}))
		got, err := GetNextSequence(context.Background(), "orders")
		if err != nil || got != 4 {
			t.Fatalf("GetNextSequence() = %d, %v", got, err)
		}
	})

	mt.Run("populated document supports scalar ids", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		mt.AddMockResponses(mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "_id", Value: 42}, {Key: "name", Value: "Ada"}}))
		got, err := GetPopulatedDocument(context.Background(), "tenant", "project", "users", "42", []string{"name"})
		if err != nil || !reflect.DeepEqual(got, map[string]interface{}{"_id": int32(42), "name": "Ada"}) {
			t.Fatalf("GetPopulatedDocument() = %#v, %v", got, err)
		}
	})

	mt.Run("populated documents batch", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		id := primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "_id", Value: id}, {Key: "name", Value: "Ada"}}))
		got, err := GetPopulatedDocuments(context.Background(), "tenant", "project", "users", []primitive.ObjectID{id}, []string{"name"})
		if err != nil || len(got) != 1 || got[0]["name"] != "Ada" {
			t.Fatalf("GetPopulatedDocuments() = %#v, %v", got, err)
		}
		if got, err := GetPopulatedDocuments(context.Background(), "tenant", "project", "users", nil, nil); err != nil || len(got) != 0 {
			t.Fatalf("GetPopulatedDocuments(empty) = %#v, %v", got, err)
		}
	})

	mt.Run("populate items maps references", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		ownerID := primitive.NewObjectID()
		memberID := primitive.NewObjectID()
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "_id", Value: ownerID}, {Key: "name", Value: "Owner"}}),
			mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "_id", Value: memberID}, {Key: "name", Value: "Member"}}),
			mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "_id", Value: "external"}, {Key: "name", Value: "External"}}),
		)
		items := []map[string]interface{}{{"owner": ownerID, "members": []interface{}{memberID}, "external": "external"}}
		got, err := PopulateItems(context.Background(), "tenant", "project", &models.ContainerModel{Fields: []models.Field{
			{Name: "owner", Type: "objectId", ObjectSchemaName: "users", PopulationSettings: &models.PopulationSettings{PopulatedFields: []string{"name"}}},
			{Name: "members", Type: "objectIdArray", ObjectSchemaName: "users", PopulationSettings: &models.PopulationSettings{PopulatedFields: []string{"name"}}},
			{Name: "external", Type: "string", ObjectSchemaName: "users", PopulationSettings: &models.PopulationSettings{PopulatedFields: []string{"name"}}},
		}}, items)
		if err != nil || got[0]["owner"].(map[string]interface{})["name"] != "Owner" || len(got[0]["members"].([]map[string]interface{})) != 1 || got[0]["external"].(map[string]interface{})["name"] != "External" {
			t.Fatalf("PopulateItems() = %#v, %v", got, err)
		}
	})

	mt.Run("index helpers", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		container := &models.ContainerModel{
			SchemaName: "orders",
			Fields:     []models.Field{{Name: "email", Unique: true}},
			Indexes:    []models.Index{{Name: "idx_state", Fields: []models.IndexField{{FieldName: "state", Order: 1}}}},
		}
		mt.AddMockResponses(
			mtest.CreateSuccessResponse(),
			mtest.CreateSuccessResponse(),
			mtest.CreateSuccessResponse(),
			mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "name", Value: "_id_"}}),
		)
		if err := EnsureIndexes(context.Background(), container, "tenant", "project"); err != nil {
			t.Fatalf("EnsureIndexes() error = %v", err)
		}
		if err := DropIndexes(context.Background(), "orders"); err != nil {
			t.Fatalf("DropIndexes() error = %v", err)
		}
		if got, err := ListIndexes(context.Background(), "orders"); err != nil || len(got) != 1 {
			t.Fatalf("ListIndexes() = %#v, %v", got, err)
		}
		if err := EnsureIndexes(context.Background(), nil, "tenant", "project"); err == nil {
			t.Fatal("EnsureIndexes(nil) error = nil")
		}
		if err := RebuildIndexes(context.Background(), nil, "tenant", "project"); err == nil {
			t.Fatal("RebuildIndexes(nil) error = nil")
		}
	})

	mt.Run("public query wrapper and collection helpers", func(mt *mtest.T) {
		restore := useMockCollections(mt.Coll)
		defer restore()
		mt.AddMockResponses(mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "name", Value: "Ada"}}))
		got, err := QueryAndDecode(context.Background(), "tenant", "project", "orders", bson.M{}, nil, &Pager{})
		if err != nil || len(got) != 1 || got[0]["name"] != "Ada" {
			t.Fatalf("QueryAndDecode() = %#v, %v", got, err)
		}

		var names []string
		oldGlobal := globalCollectionProvider
		globalCollectionProvider = func(name string) *mongo.Collection {
			names = append(names, name)
			return mt.Coll
		}
		defer func() { globalCollectionProvider = oldGlobal }()
		GetProjectCollection("tenant", "project", "orders")
		GetContainerCollectionForProject("", "")
		GetContainerCollectionForProject("tenant", "project")
		GetDynamicCollectionForProject("", "", "orders")
		GetDynamicCollectionForProject("tenant", "project", "orders")
		GetPageCollectionForProject("", "")
		GetPageCollectionForProject("tenant", "project")
		want := []string{
			"tenant_tenant_project_project_orders",
			"containers",
			"tenant_tenant_project_project_containers",
			"orders",
			"tenant_tenant_project_project_orders",
			"pages",
			"tenant_tenant_project_project_pages",
		}
		if !reflect.DeepEqual(names, want) {
			t.Fatalf("collection names = %#v, want %#v", names, want)
		}
	})
}

func useMockCollections(collection *mongo.Collection) func() {
	oldProject := projectCollectionProvider
	oldContainer := containerCollectionProvider
	oldDynamic := dynamicCollectionProvider
	oldGlobal := globalCollectionProvider
	oldCounters := countersCollectionProvider
	projectCollectionProvider = func(_, _, _ string) *mongo.Collection { return collection }
	containerCollectionProvider = func(_, _ string) *mongo.Collection { return collection }
	dynamicCollectionProvider = func(_, _, _ string) *mongo.Collection { return collection }
	globalCollectionProvider = func(_ string) *mongo.Collection { return collection }
	countersCollectionProvider = func() *mongo.Collection { return collection }
	return func() {
		projectCollectionProvider = oldProject
		containerCollectionProvider = oldContainer
		dynamicCollectionProvider = oldDynamic
		globalCollectionProvider = oldGlobal
		countersCollectionProvider = oldCounters
	}
}

func namespace(collection *mongo.Collection) string {
	return collection.Database().Name() + "." + collection.Name()
}
