package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/services"
	"github.com/xuri/excelize/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestDetectFieldType(t *testing.T) {
	tests := []struct {
		name    string
		samples []string
		want    string
	}{
		{name: "empty", want: "string"},
		{name: "blank", samples: []string{" ", ""}, want: "string"},
		{name: "bool", samples: []string{"true", "no", "1"}, want: "bool"},
		{name: "int", samples: []string{"12", "-3"}, want: "int"},
		{name: "float", samples: []string{"1.2", "-3.4"}, want: "float"},
		{name: "uuid", samples: []string{"550e8400-e29b-41d4-a716-446655440000"}, want: "uuid"},
		{name: "ip", samples: []string{"127.0.0.1", "192.168.1.1"}, want: "ip"},
		{name: "url", samples: []string{"https://example.com"}, want: "url"},
		{name: "email remains string", samples: []string{"user@example.com"}, want: "string"},
		{name: "date", samples: []string{"2026-06-01"}, want: "date"},
		{name: "mixed", samples: []string{"1", "text"}, want: "string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectFieldType(tt.samples); got != tt.want {
				t.Fatalf("detectFieldType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeFieldName(t *testing.T) {
	tests := map[string]string{
		"First Name":         "firstName",
		"customer-id":        "customerId",
		" already_clean ":    "alreadyClean",
		"***":                "field",
		"HTTP Response CODE": "httpResponseCode",
	}
	for input, want := range tests {
		if got := sanitizeFieldName(input); got != want {
			t.Fatalf("sanitizeFieldName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAnalyzeExcelData(t *testing.T) {
	got, err := analyzeExcelData(
		[]string{"Name", "", "Age", "Enabled"},
		[][]string{{"Ada", "ignored", "42", "true"}, {"Lin", "ignored", "31", "false"}},
	)
	if err != nil {
		t.Fatalf("analyzeExcelData() error = %v", err)
	}
	want := []models.Field{
		{Name: "name", Type: "string", IsSearchable: true},
		{Name: "age", Type: "int", IsSearchable: true},
		{Name: "enabled", Type: "bool", IsSearchable: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("analyzeExcelData() = %#v, want %#v", got, want)
	}
}

func TestAnalyzeExcelFile(t *testing.T) {
	workbook := excelize.NewFile()
	t.Cleanup(func() { _ = workbook.Close() })
	sheet := workbook.GetSheetName(0)
	if err := workbook.SetSheetRow(sheet, "A1", &[]interface{}{"Name", "Age"}); err != nil {
		t.Fatalf("SetSheetRow(header) error = %v", err)
	}
	if err := workbook.SetSheetRow(sheet, "A2", &[]interface{}{"Ada", 42}); err != nil {
		t.Fatalf("SetSheetRow(data) error = %v", err)
	}
	var content bytes.Buffer
	if err := workbook.Write(&content); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	file := uploadedFileHeader(t, "Customer Orders.xlsx", content.Bytes())
	got, err := analyzeExcelFile(file)
	if err != nil || got.SchemaName != "customerOrders" || len(got.Fields) != 2 || got.Fields[1].Type != "int" {
		t.Fatalf("analyzeExcelFile() = %#v, %v", got, err)
	}
	if _, err := analyzeExcelFile(uploadedFileHeader(t, "invalid.xlsx", []byte("not an excel file"))); err == nil {
		t.Fatal("analyzeExcelFile(invalid) error = nil")
	}
}

func uploadedFileHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", filename)
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	form, err := multipart.NewReader(&body, writer.Boundary()).ReadForm(int64(body.Len()))
	if err != nil {
		t.Fatalf("ReadForm() error = %v", err)
	}
	t.Cleanup(func() { _ = form.RemoveAll() })
	return form.File["files"][0]
}

func TestConvertValue(t *testing.T) {
	date := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		value     string
		fieldType string
		want      interface{}
	}{
		{name: "empty", value: "", fieldType: "string", want: nil},
		{name: "bool true", value: "yes", fieldType: "bool", want: true},
		{name: "bool false", value: "no", fieldType: "boolean", want: false},
		{name: "int", value: "42", fieldType: "int", want: 42},
		{name: "invalid int stays string", value: "bad", fieldType: "int", want: "bad"},
		{name: "float", value: "2.5", fieldType: "decimal", want: 2.5},
		{name: "date", value: "2026-06-01", fieldType: "date", want: date},
		{name: "string", value: "Ada", fieldType: "string", want: "Ada"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := convertValue(tt.value, tt.fieldType); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("convertValue() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDetectRelationshipsAndOrderDependencies(t *testing.T) {
	analyses := []FileAnalysis{
		{SchemaName: "orders", Fields: []models.Field{{Name: "userId"}, {Name: "total"}}},
		{SchemaName: "users", Fields: []models.Field{{Name: "name"}}},
	}
	relationships := detectRelationships(analyses)
	want := []RelationshipDetection{{SourceSchema: "orders", SourceField: "userId", TargetSchema: "users", TargetFieldIndex: 0, Confidence: "high"}}
	if !reflect.DeepEqual(relationships, want) {
		t.Fatalf("detectRelationships() = %#v, want %#v", relationships, want)
	}
	updateFieldsWithRelationships(analyses, relationships)
	if analyses[0].Fields[0].Type != "reference" || analyses[0].Fields[0].ObjectSchemaName != "users" {
		t.Fatalf("updated field = %#v", analyses[0].Fields[0])
	}
	ordered := orderByDependencies(analyses, relationships)
	if ordered[0].SchemaName != "users" || ordered[1].SchemaName != "orders" {
		t.Fatalf("orderByDependencies() = %#v", ordered)
	}
}

func TestReferencedObjectSchemaNames(t *testing.T) {
	fields := []models.Field{
		{Name: "owner", Type: "objectId", ObjectSchemaName: "users"},
		{Name: "members", Type: "objectIdArray", ObjectSchemaName: "users"},
		{Name: "category", Type: "objectIdArray", ObjectSchemaName: "categories"},
		{Name: "driver", Type: "objectid", ObjectSchemaName: " car "},
		{Name: "ignored", Type: "string", ObjectSchemaName: "products"},
		{Name: "blank", Type: "objectId"},
	}
	want := []string{"users", "categories", "car"}
	if got := referencedObjectSchemaNames(fields); !reflect.DeepEqual(got, want) {
		t.Fatalf("referencedObjectSchemaNames() = %#v, want %#v", got, want)
	}
}

func TestDefaultContainerRedis(t *testing.T) {
	got := defaultContainerRedis("orders", []string{"stock", "orders", " stock "})
	if !got.IsRedisCached {
		t.Fatal("defaultContainerRedis().IsRedisCached = false, want true")
	}
	if got.CacheTime <= 0 {
		t.Fatalf("defaultContainerRedis().CacheTime = %d, want positive", got.CacheTime)
	}
	if !reflect.DeepEqual(got.TriggeredRedisCaches, []string{"orders", "stock"}) {
		t.Fatalf("defaultContainerRedis().TriggeredRedisCaches = %#v", got.TriggeredRedisCaches)
	}
}

func TestApplyContainerAuthFlagDefaults(t *testing.T) {
	existing := models.ContainerModel{
		IsAuthContainer:     true,
		IsRegisterActive:    true,
		IsGoogleLoginActive: true,
	}
	updated := models.ContainerModel{
		IsRegisterActive: false,
	}

	applyContainerAuthFlagDefaults(existing, &updated, map[string]bool{
		"isRegisterActive": true,
	})

	if !updated.IsAuthContainer {
		t.Fatal("IsAuthContainer = false, want preserved true")
	}
	if updated.IsRegisterActive {
		t.Fatal("IsRegisterActive = true, want explicit false")
	}
	if !updated.IsGoogleLoginActive {
		t.Fatal("IsGoogleLoginActive = false, want preserved true")
	}
}

func TestObjectReferenceValidationAndInvalidationHelpers(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()

	mt.Run("rejects self reference", func(mt *mtest.T) {
		container := models.ContainerModel{
			SchemaName: "orders",
			Fields:     []models.Field{{Name: "parent", Type: "objectIdArray", ObjectSchemaName: "orders"}},
		}
		if _, err := validateObjectReferences(context.Background(), mt.Coll, container, primitive.NilObjectID); err == nil {
			t.Fatal("validateObjectReferences() error = nil")
		}
	})

	mt.Run("validates objectIdArray and adds invalidation trigger", func(mt *mtest.T) {
		container := models.ContainerModel{
			SchemaName: "orders",
			Fields:     []models.Field{{Name: "users", Type: "objectIdArray", ObjectSchemaName: "users"}},
		}
		userID := primitive.NewObjectID()
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, mt.Coll.Database().Name()+"."+mt.Coll.Name(), mtest.FirstBatch, bson.D{{Key: "_id", Value: userID}}),
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}),
		)
		referencedIDs, err := validateObjectReferences(context.Background(), mt.Coll, container, primitive.NewObjectID())
		if err != nil {
			t.Fatalf("validateObjectReferences() error = %v", err)
		}
		gotIDs, err := syncReferencedInvalidations(context.Background(), mt.Coll, models.ContainerModel{}, container, referencedIDs)
		if err != nil {
			t.Fatalf("syncReferencedInvalidations() error = %v", err)
		}
		if !reflect.DeepEqual(gotIDs, []primitive.ObjectID{userID}) {
			t.Fatalf("syncReferencedInvalidations() ids = %#v, want %#v", gotIDs, []primitive.ObjectID{userID})
		}
	})

	mt.Run("removes stale invalidation trigger when reference is removed", func(mt *mtest.T) {
		carID := primitive.NewObjectID()
		existing := models.ContainerModel{
			SchemaName: "stock",
			Fields:     []models.Field{{Name: "car", Type: "objectId", ObjectSchemaName: "car"}},
		}
		updated := models.ContainerModel{
			SchemaName: "stock",
			Fields:     []models.Field{{Name: "quantity", Type: "int"}},
		}
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, mt.Coll.Database().Name()+"."+mt.Coll.Name(), mtest.FirstBatch, bson.D{{Key: "_id", Value: carID}}),
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}),
		)
		gotIDs, err := syncReferencedInvalidations(context.Background(), mt.Coll, existing, updated, nil)
		if err != nil {
			t.Fatalf("syncReferencedInvalidations(remove) error = %v", err)
		}
		if !reflect.DeepEqual(gotIDs, []primitive.ObjectID{carID}) {
			t.Fatalf("syncReferencedInvalidations(remove) ids = %#v, want %#v", gotIDs, []primitive.ObjectID{carID})
		}
	})
}

func TestConvertValueWithReference(t *testing.T) {
	id := primitive.NewObjectID()
	if got := convertValueWithReference(id.Hex(), models.Field{Type: "reference"}); got != id {
		t.Fatalf("reference ObjectID = %#v, want %s", got, id.Hex())
	}
	if got := convertValueWithReference("external-id", models.Field{Type: "reference"}); got != "external-id" {
		t.Fatalf("reference string = %#v", got)
	}
	if got := convertValueWithReference("", models.Field{Type: "reference"}); got != nil {
		t.Fatalf("empty reference = %#v", got)
	}
}

func TestProjectNamingAndSlugValidation(t *testing.T) {
	if got := GetCollectionNameForProject("t", "p", "orders"); got != "tenant_t_project_p_orders" {
		t.Fatalf("GetCollectionNameForProject() = %q", got)
	}
	if got := GetProjectPrefix("t", "p"); got != "tenant_t_project_p_" {
		t.Fatalf("GetProjectPrefix() = %q", got)
	}
	tests := map[string]bool{
		"project-1":  true,
		"ab":         true,
		"a":          false,
		"Project":    false,
		"project--1": false,
		"1project":   false,
		"project-":   false,
	}
	for slug, want := range tests {
		if got := ValidateSlug(slug); got != want {
			t.Fatalf("ValidateSlug(%q) = %v, want %v", slug, got, want)
		}
	}
}

func TestSwaggerSchemaHelpers(t *testing.T) {
	if err := validateUniqueSchemaNames([]models.ContainerModel{{SchemaName: "orders"}, {SchemaName: "customers"}}); err != nil {
		t.Fatalf("validateUniqueSchemaNames() error = %v", err)
	}
	err := validateUniqueSchemaNames([]models.ContainerModel{{SchemaName: " orders "}, {SchemaName: "orders"}, {SchemaName: ""}})
	if err == nil || !strings.Contains(err.Error(), "Duplicate schema names: orders (x2)") || !strings.Contains(err.Error(), "Empty schema names at indices: [2]") {
		t.Fatalf("validateUniqueSchemaNames() error = %v", err)
	}
	if got := extractSchemaNames([]models.ContainerModel{{SchemaName: "orders"}, {SchemaName: "customers"}}); !reflect.DeepEqual(got, []string{"orders", "customers"}) {
		t.Fatalf("extractSchemaNames() = %#v", got)
	}
}

func TestProjectContextHelpers(t *testing.T) {
	app := fiber.New()
	app.Get("/dynamic", func(c *fiber.Ctx) error {
		tenantID, projectID, err := getProjectContext(c)
		if err != nil || tenantID != "tenant" || projectID != "project" {
			t.Fatalf("getProjectContext() = %q, %q, %v", tenantID, projectID, err)
		}
		return nil
	})
	app.Get("/page", func(c *fiber.Ctx) error {
		c.Locals("tenantID", "local-tenant")
		c.Locals("projectID", "local-project")
		tenantID, projectID, err := getPageProjectContext(c)
		if err != nil || tenantID != "local-tenant" || projectID != "local-project" {
			t.Fatalf("getPageProjectContext() = %q, %q, %v", tenantID, projectID, err)
		}
		return nil
	})
	app.Get("/missing", func(c *fiber.Ctx) error {
		if _, _, err := getProjectContext(c); err == nil {
			t.Fatal("getProjectContext(missing) error = nil")
		}
		return nil
	})
	for _, path := range []string{"/dynamic?tenantID=tenant&projectID=project", "/page", "/missing"} {
		if _, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil)); err != nil {
			t.Fatalf("app.Test(%q) error = %v", path, err)
		}
	}
}

func TestDynamicResponseHelpers(t *testing.T) {
	serviceErr := &services.ServiceError{Status: http.StatusTeapot, Message: "tea", Data: map[string]string{"kind": "pot"}}
	app := fiber.New()
	app.Get("/idempotent", func(c *fiber.Ctx) error {
		return sendIdempotentResponse(context.Background(), c, "", http.StatusCreated, "created", map[string]string{"id": "1"})
	})
	app.Get("/service", func(c *fiber.Ctx) error {
		return sendDynamicServiceError(context.Background(), c, "", serviceErr, "generic")
	})
	app.Get("/dynamic", func(c *fiber.Ctx) error {
		return sendDynamicError(c, serviceErr, "generic")
	})

	tests := []struct {
		path        string
		wantStatus  int
		wantMessage string
	}{
		{path: "/idempotent", wantStatus: http.StatusCreated, wantMessage: "created"},
		{path: "/service", wantStatus: http.StatusTeapot, wantMessage: "tea"},
		{path: "/dynamic", wantStatus: http.StatusTeapot, wantMessage: "tea"},
	}
	for _, tt := range tests {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, tt.path, nil))
		if err != nil {
			t.Fatalf("app.Test(%q) error = %v", tt.path, err)
		}
		var body responses.GeneralResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || resp.StatusCode != tt.wantStatus || body.Message != tt.wantMessage {
			t.Fatalf("%s response = %#v, status = %d, error = %v", tt.path, body, resp.StatusCode, err)
		}
	}
}

func TestControllerRedisHelpers(t *testing.T) {
	server := miniredis.RunT(t)
	oldClient := configs.RedisClient
	configs.RedisClient = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = configs.RedisClient.Close()
		configs.RedisClient = oldClient
	})
	if err := configs.RedisClient.Set(context.Background(), "cached", "value", time.Minute).Err(); err != nil {
		t.Fatalf("Redis Set() error = %v", err)
	}

	app := fiber.New()
	app.Post("/begin", func(c *fiber.Ctx) error {
		key, proceed, err := beginDynamicIdempotency(context.Background(), c, "tenant", "project", "user")
		if err != nil || key != "" || !proceed {
			t.Fatalf("beginDynamicIdempotency() = %q, %v, %v", key, proceed, err)
		}
		return c.SendStatus(http.StatusNoContent)
	})
	app.Post("/reset", ResetRedis)

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/begin", nil))
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("begin response = %#v, %v", resp, err)
	}
	resp, err = app.Test(httptest.NewRequest(http.MethodPost, "/reset", nil))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("reset response = %#v, %v", resp, err)
	}
	if server.Exists("cached") {
		t.Fatal("ResetRedis() left cached key behind")
	}
}

func TestSwaggerPathGeneration(t *testing.T) {
	spec := SwaggerSpec{
		Paths:      map[string]interface{}{},
		Components: SwaggerComponents{Schemas: map[string]interface{}{}},
	}
	container := models.ContainerModel{
		SchemaName: "orders",
		Fields: []models.Field{
			{Name: "title", Type: "string"},
			{Name: "createdAt", Type: "date"},
			{Name: "owner", Type: "objectId", ObjectSchemaName: "users"},
		},
	}
	generateSchemaDefinition(&spec, container)
	addPathsForContainer(&spec, container)
	addUniversalDynamicPaths(&spec, []models.ContainerModel{container})

	if _, ok := spec.Components.Schemas["orders"]; !ok {
		t.Fatal("orders schema missing")
	}
	if _, ok := spec.Components.Schemas["ordersInput"]; !ok {
		t.Fatal("orders input schema missing")
	}
	for _, path := range []string{"/api/v1/orders", "/api/v1/orders/{id}", "/api/v1/dynamic", "/api/v1/dynamic/multiple", "/api/v1/dynamic/{id}"} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("path %q missing", path)
		}
	}
}

func TestGetSwaggerUI(t *testing.T) {
	app := fiber.New()
	app.Get("/docs", GetSwaggerUI)
	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	req.Host = "example.test"
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if resp.Header.Get("Content-Type") != "text/html" || !strings.Contains(string(body), "http://example.test/api/swagger.json") {
		t.Fatalf("GetSwaggerUI() content type = %q, body = %q", resp.Header.Get("Content-Type"), body)
	}
}

func TestSwaggerHandlersWithSuppliedContainers(t *testing.T) {
	oldProvider := getAllContainerModelsForSwagger
	t.Cleanup(func() { getAllContainerModelsForSwagger = oldProvider })

	app := fiber.New()
	app.Get("/swagger", GenerateDynamicSwagger)
	app.Get("/schemas", ListAllSchemas)
	getAllContainerModelsForSwagger = func() ([]models.ContainerModel, error) {
		return []models.ContainerModel{{SchemaName: "orders", Fields: []models.Field{{Name: "name", Type: "string"}}}}, nil
	}
	for _, path := range []string{"/swagger", "/schemas"} {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("%s response = %#v, %v", path, resp, err)
		}
	}

	getAllContainerModelsForSwagger = func() ([]models.ContainerModel, error) {
		return []models.ContainerModel{{SchemaName: "orders"}, {SchemaName: "orders"}}, nil
	}
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/swagger", nil))
	if err != nil {
		t.Fatalf("duplicate swagger response error = %v", err)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body["error"] == nil {
		t.Fatalf("duplicate swagger body = %#v, %v", body, err)
	}
}
