package utils

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestValidateContainerModelScenarios(t *testing.T) {
	validID := primitive.NewObjectID().Hex()
	tests := []struct {
		name      string
		item      map[string]interface{}
		container models.ContainerModel
		wantErr   bool
	}{
		{
			name:      "login credential required",
			item:      map[string]interface{}{},
			container: models.ContainerModel{Fields: []models.Field{{Name: "email", Type: "string", IsLoginCredential: true}}},
			wantErr:   true,
		},
		{
			name:      "valid object id array",
			item:      map[string]interface{}{"ids": []interface{}{validID}},
			container: models.ContainerModel{Fields: []models.Field{{Name: "ids", Type: "objectIdArray", Tag: "required"}}},
		},
		{
			name:      "invalid nested object",
			item:      map[string]interface{}{"profile": map[string]interface{}{}},
			container: models.ContainerModel{Fields: []models.Field{{Name: "profile", Type: "object", Children: []models.Field{{Name: "name", Type: "string", Tag: "required"}}}}},
			wantErr:   true,
		},
		{
			name:      "valid nested array",
			item:      map[string]interface{}{"lines": []interface{}{map[string]interface{}{"quantity": 2}}},
			container: models.ContainerModel{Fields: []models.Field{{Name: "lines", Type: "array", Children: []models.Field{{Name: "quantity", Type: "int", Tag: "positive"}}}}},
		},
		{
			name:      "invalid enum array",
			item:      map[string]interface{}{"states": []interface{}{"closed"}},
			container: models.ContainerModel{Fields: []models.Field{{Name: "states", Type: "stringArray", EnumList: []interface{}{"open"}}}},
			wantErr:   true,
		},
		{
			name:      "valid float unix timestamp",
			item:      map[string]interface{}{"createdAt": float64(1704067200)},
			container: models.ContainerModel{Fields: []models.Field{{Name: "createdAt", Type: "date"}}},
		},
		{
			name:      "invalid email",
			item:      map[string]interface{}{"email": "invalid"},
			container: models.ContainerModel{Fields: []models.Field{{Name: "email", Type: "string", Tag: "email"}}},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateContainerModel(tt.item, tt.container); (err != nil) != tt.wantErr {
				t.Fatalf("ValidateContainerModel() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePartialUpdateScenarios(t *testing.T) {
	container := models.ContainerModel{Fields: []models.Field{
		{Name: "email", Type: "string", IsLoginCredential: true, Tag: "email"},
		{Name: "age", Type: "int", Tag: "positive"},
	}}
	tests := []struct {
		name    string
		update  map[string]interface{}
		wantErr bool
	}{
		{name: "unrelated fields skip login credential", update: map[string]interface{}{"name": "Ada"}},
		{name: "valid updated credential", update: map[string]interface{}{"email": "ada@example.com"}},
		{name: "empty updated credential", update: map[string]interface{}{"email": ""}, wantErr: true},
		{name: "invalid present field", update: map[string]interface{}{"age": 0}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidatePartialUpdate(tt.update, container); (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePartialUpdate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFormFieldHelpers(t *testing.T) {
	got := ProcessFormFields(map[string][]string{
		"name":           {"Ada"},
		"profile[email]": {"ada@example.com"},
	})
	want := map[string]interface{}{
		"name": "Ada",
		"profile": map[string]interface{}{
			"email": "ada@example.com",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ProcessFormFields() = %#v, want %#v", got, want)
	}
	if converted := ConvertFormFieldTypes(got, nil); !reflect.DeepEqual(converted, got) {
		t.Fatalf("ConvertFormFieldTypes() = %#v, want %#v", converted, got)
	}
}

func TestGetUserFromContext(t *testing.T) {
	id := primitive.NewObjectID()
	app := fiber.New()
	app.Get("/valid", func(c *fiber.Ctx) error {
		c.Locals("userID", id.Hex())
		c.Locals("userRole", "admin")
		c.Locals("userDisplayName", "Ada Lovelace")
		user := GetUserFromContext(c)
		if user == nil || user.ID != id || user.DisplayName != "Ada Lovelace" || !reflect.DeepEqual(user.Roles, []string{"admin"}) {
			t.Fatalf("GetUserFromContext() = %#v", user)
		}
		return nil
	})
	app.Get("/missing", func(c *fiber.Ctx) error {
		if user := GetUserFromContext(c); user != nil {
			t.Fatalf("GetUserFromContext() = %#v, want nil", user)
		}
		return nil
	})
	app.Get("/invalid", func(c *fiber.Ctx) error {
		c.Locals("userID", "invalid")
		if user := GetUserFromContext(c); user != nil {
			t.Fatalf("GetUserFromContext() = %#v, want nil", user)
		}
		return nil
	})
	for _, path := range []string{"/valid", "/missing", "/invalid"} {
		if _, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil)); err != nil {
			t.Fatalf("app.Test(%q) error = %v", path, err)
		}
	}
}

func TestBuildAuditLogFilter(t *testing.T) {
	userID := primitive.NewObjectID()
	documentID := primitive.NewObjectID()
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		got, err := BuildAuditLogFilter(c)
		if err != nil {
			t.Fatalf("BuildAuditLogFilter() error = %v", err)
		}
		want := bson.M{
			"tenantId":    "tenant",
			"projectId":   "project",
			"action":      "create",
			"schemaName":  "orders",
			"userEmail":   "ada@example.com",
			"userId":      userID,
			"documentIds": bson.M{"$in": []primitive.ObjectID{documentID}},
			"timestamp": bson.M{
				"$gte": primitive.NewDateTimeFromTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
				"$lte": primitive.NewDateTimeFromTime(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)),
			},
			"ip":    "127.0.0.1",
			"roles": bson.M{"$in": []string{"admin"}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("BuildAuditLogFilter() = %#v, want %#v", got, want)
		}
		return nil
	})
	path := "/?tenantID=tenant&projectID=project&action=create&schemaName=orders&userEmail=ada%40example.com&userId=" +
		userID.Hex() + "&documentId=" + documentID.Hex() +
		"&startDate=2026-01-01T00%3A00%3A00Z&endDate=2026-01-02T00%3A00%3A00Z&ip=127.0.0.1&role=admin"
	if _, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil)); err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
}

func TestFetchAndInvalidateCacheHelpers(t *testing.T) {
	setupMiniRedis(t)
	ctx := context.Background()
	if err := configs.RedisClient.Set(ctx, "items", `[{"id":1}]`, time.Minute).Err(); err != nil {
		t.Fatalf("Redis Set() error = %v", err)
	}
	items, found, err := TryFetchFromCache(ctx, configs.RedisClient, "items")
	if err != nil || !found || len(items) != 1 || items[0]["id"] != float64(1) {
		t.Fatalf("TryFetchFromCache() = %#v, %v, %v", items, found, err)
	}
	if _, found, err := TryFetchFromCache(ctx, configs.RedisClient, "missing"); !errors.Is(err, redis.Nil) || found {
		t.Fatalf("TryFetchFromCache(missing) found = %v, error = %v", found, err)
	}
	if err := configs.RedisClient.Set(ctx, "invalid", `{`, time.Minute).Err(); err != nil {
		t.Fatalf("Redis Set() error = %v", err)
	}
	if _, found, err := TryFetchFromCache(ctx, configs.RedisClient, "invalid"); err == nil || found {
		t.Fatalf("TryFetchFromCache(invalid) found = %v, error = %v", found, err)
	}

	if err := DeleteCacheForSchema(ctx, "tenant", "project", "orders", nil); err != nil {
		t.Fatalf("DeleteCacheForSchema() error = %v", err)
	}
	if version, err := GetSchemaCacheVersion(ctx, "tenant", "project", "orders"); err != nil || version != 1 {
		t.Fatalf("GetSchemaCacheVersion() = %d, %v", version, err)
	}
}

func TestBasicLockHelpers(t *testing.T) {
	setupMiniRedis(t)
	lockID, locked := AcquireLock("lock", time.Minute)
	if !locked || lockID == "" {
		t.Fatalf("AcquireLock() = %q, %v", lockID, locked)
	}
	if _, locked := AcquireLock("lock", time.Minute); locked {
		t.Fatal("second AcquireLock() locked = true")
	}
	ReleaseLock("lock", "wrong")
	if _, locked := AcquireLock("lock", time.Minute); locked {
		t.Fatal("wrong ReleaseLock() removed lock")
	}
	ReleaseLock("lock", lockID)
	if _, locked := AcquireLock("lock", time.Minute); !locked {
		t.Fatal("correct ReleaseLock() did not remove lock")
	}
}

func TestFetchContainerModelFromLocals(t *testing.T) {
	container := &models.ContainerModel{SchemaName: "orders"}
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		c.Locals("containerModel", container)
		got, err := FetchContainerModel(c)
		if err != nil || got != container {
			t.Fatalf("FetchContainerModel() = %#v, %v", got, err)
		}
		return nil
	})
	if _, err := app.Test(httptest.NewRequest(http.MethodGet, "/?schemaName=orders", nil)); err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
}

func TestFetchContainerModelErrors(t *testing.T) {
	app := fiber.New()
	app.Get("/missing-schema", func(c *fiber.Ctx) error {
		if _, err := FetchContainerModel(c); !errors.Is(err, ErrNoSchemaName) {
			t.Fatalf("FetchContainerModel() error = %v, want %v", err, ErrNoSchemaName)
		}
		return nil
	})
	app.Get("/missing-context", func(c *fiber.Ctx) error {
		if _, err := FetchContainerModel(c); err == nil {
			t.Fatal("FetchContainerModel() error = nil")
		}
		return nil
	})
	for _, path := range []string{"/missing-schema", "/missing-context?schemaName=orders"} {
		if _, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil)); err != nil {
			t.Fatalf("app.Test(%q) error = %v", path, err)
		}
	}
}

func TestTenantContextFallback(t *testing.T) {
	app := fiber.New()
	app.Get("/query", func(c *fiber.Ctx) error {
		tenantID, projectID, err := GetTenantAndProjectContext(c)
		if err != nil || tenantID != "tenant-query" || projectID != "project-query" {
			t.Fatalf("GetTenantAndProjectContext() = %q, %q, %v", tenantID, projectID, err)
		}
		if c.Locals("tenantID") != tenantID || c.Locals("projectID") != projectID {
			t.Fatalf("locals = %#v, %#v", c.Locals("tenantID"), c.Locals("projectID"))
		}
		return nil
	})
	app.Get("/locals", func(c *fiber.Ctx) error {
		c.Locals("tenantID", "tenant-local")
		c.Locals("projectID", "project-local")
		tenantID, projectID, err := GetTenantAndProjectContext(c)
		if err != nil || tenantID != "tenant-local" || projectID != "project-local" {
			t.Fatalf("GetTenantAndProjectContext() = %q, %q, %v", tenantID, projectID, err)
		}
		return nil
	})
	for _, path := range []string{"/query?tenantID=tenant-query&projectID=project-query", "/locals"} {
		if _, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil)); err != nil {
			t.Fatalf("app.Test(%q) error = %v", path, err)
		}
	}
}

func TestPopulateIfNeededWithoutPopulationSettings(t *testing.T) {
	items := []map[string]interface{}{{"id": "1"}}
	got, err := PopulateIfNeeded(context.Background(), "tenant", "project", &models.ContainerModel{}, items)
	if err != nil || !reflect.DeepEqual(got, items) {
		t.Fatalf("PopulateIfNeeded() = %#v, %v", got, err)
	}
}

func TestParseDate(t *testing.T) {
	got, err := parseDate("2026-01-02T10:20:30Z")
	want := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if err != nil || !got.Equal(want) {
		t.Fatalf("parseDate() = %v, %v, want %v", got, err, want)
	}
	if _, err := parseDate("invalid"); err == nil {
		t.Fatal("parseDate(invalid) error = nil")
	}
}
