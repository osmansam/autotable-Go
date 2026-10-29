//go:build integration

package controllers

import (
	"context"
	"encoding/json"
	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"io"
	"net"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWorkflowMongoTemplateCredentialCreateRollbackAndDelete(t *testing.T) {
	uri := os.Getenv("AUTOTABLE_WORKFLOW_MONGO_URI")
	parsed, err := url.Parse(uri)
	if err != nil || uri == "" || parsed.Hostname() != "127.0.0.1" {
		t.Fatal("task-owned loopback Mongo URI required; run scripts/test-workflow-mongo-integration.sh")
	}
	databaseName := "autotable_credential_it_" + primitive.NewObjectID().Hex()
	t.Setenv("MONGO_URI_BASE", "mongodb://"+parsed.Host+"/")
	t.Setenv("COLLECTION_NAME", databaseName)
	t.Setenv("MONGO_URI_SUFFIX", "?"+parsed.RawQuery)
	redisServer := miniredis.RunT(t)
	host, port, err := net.SplitHostPort(redisServer.Addr())
	if err != nil {
		t.Fatal(err)
	}
	cfg := configs.GetAppConfig()
	previousRedis := cfg.Redis
	cfg.Redis.Host = host
	cfg.Redis.Port = port
	t.Cleanup(func() { cfg.Redis = previousRedis })
	configs.InitDB()
	db := configs.GetCollection("projects").Database()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := db.Drop(ctx); err != nil {
			t.Error(err)
		}
		_ = configs.DB.Disconnect(ctx)
		_ = configs.RedisClient.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tenant, user, sourceID, targetKey := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	if _, err = db.Collection("tenants").InsertOne(ctx, models.Tenant{ID: tenant, Slug: "fixture-tenant", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	source := models.Project{ID: sourceID, TenantID: tenant, Name: "Source", Slug: "source", IsActive: true, IsTemplate: true, TemplateScope: "tenant"}
	if _, err = db.Collection("projects").InsertOne(ctx, source); err != nil {
		t.Fatal(err)
	}
	sourceCredential := primitive.NewObjectID().Hex()
	containers := nestedCredentialTemplate(sourceCredential)
	containers[0].ID = primitive.NewObjectID()
	sourceContainers := db.Collection(utils.GetProjectCollectionName(tenant.Hex(), sourceID.Hex(), "containers"))
	if _, err = sourceContainers.InsertOne(ctx, containers[0]); err != nil {
		t.Fatal(err)
	}
	repo := repositories.NewDynamicRepositoryWithDatabase(db)
	key := models.ExternalAPICredential{ID: targetKey, TenantID: tenant, Scope: "tenant", Name: "Target provider", EncryptedSecret: "single-ciphertext", AllowedDomains: []string{"example.com"}}
	if _, err = repo.InsertExternalAPICredential(ctx, key); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("tenantID", tenant.Hex())
		c.Locals("tenantUserID", user.Hex())
		return c.Next()
	})
	app.Post("/projects", CreateProject)
	app.Delete("/projects/:id", DeleteProject)
	app.Get("/templates/:id/external-api-requirements", GetTemplateExternalAPIRequirements)
	oldInvalidate, oldRevoke := invalidateSlugMappings, revokeProjectSessions
	invalidateSlugMappings = func(context.Context, string, ...string) error { return nil }
	revokeProjectSessions = func(context.Context, string, string) error { return nil }
	t.Cleanup(func() { invalidateSlugMappings = oldInvalidate; revokeProjectSessions = oldRevoke })
	request := func(method, path, body string) (int, []byte) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, raw
	}
	input := func(slug string) string {
		raw, _ := json.Marshal(CreateProjectInput{Name: "Created project", Slug: slug, TemplateProjectID: sourceID.Hex(), ExternalAPICredentialIDs: []string{targetKey.Hex()}, ExternalAPICredentialBindings: map[string]string{sourceCredential: targetKey.Hex()}})
		return string(raw)
	}
	status, raw := request("POST", "/projects", input("successful-clone"))
	if status != 201 {
		t.Fatalf("create status=%d: %s", status, raw)
	}
	var created models.Project
	if err = db.Collection("projects").FindOne(ctx, bson.M{"slug": "successful-clone"}).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.GetExternalAPICredential(ctx, tenant.Hex(), created.ID.Hex(), targetKey.Hex()); err != nil {
		t.Fatal("created project not granted", err)
	}
	var copied models.ContainerModel
	if err = db.Collection(utils.GetProjectCollectionName(tenant.Hex(), created.ID.Hex(), "containers")).FindOne(ctx, bson.M{"schemaName": "orders"}).Decode(&copied); err != nil {
		t.Fatal(err)
	}
	requirements := templateExternalAPIRequirements([]models.ContainerModel{copied})
	if len(requirements) != 1 || requirements[0].SourceCredentialID != targetKey.Hex() {
		t.Fatalf("persisted clone references source: %#v", requirements)
	}
	// Make a malformed template cause an ordered insert failure after one container has been persisted.
	duplicate := containers[0]
	duplicate.ID = primitive.NewObjectID()
	if _, err = sourceContainers.InsertOne(ctx, duplicate); err != nil {
		t.Fatal(err)
	}
	status, raw = request("POST", "/projects", input("failed-clone"))
	if status != 500 {
		t.Fatalf("failed clone status=%d: %s", status, raw)
	}
	count, err := db.Collection("projects").CountDocuments(ctx, bson.M{"slug": "failed-clone"})
	if err != nil || count != 0 {
		t.Fatalf("failed project persisted: %d %v", count, err)
	}
	count, err = db.Collection("external_api_credential_grants").CountDocuments(ctx, bson.M{"credentialId": targetKey})
	if err != nil || count != 1 {
		t.Fatalf("failed-create grant remained: %d %v", count, err)
	}
	names, err := db.ListCollectionNames(ctx, bson.M{"name": bson.M{"$regex": "^tenant_" + tenant.Hex() + "_project_"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if !strings.HasPrefix(name, utils.GetProjectCollectionName(tenant.Hex(), sourceID.Hex(), "")) && !strings.HasPrefix(name, utils.GetProjectCollectionName(tenant.Hex(), created.ID.Hex(), "")) {
			t.Fatalf("failed project resource remained: %s", name)
		}
	}
	status, raw = request("DELETE", "/projects/"+created.ID.Hex(), "")
	if status != 200 {
		t.Fatalf("delete status=%d: %s", status, raw)
	}
	count, err = db.Collection("external_api_credential_grants").CountDocuments(ctx, bson.M{"credentialId": targetKey})
	if err != nil || count != 0 {
		t.Fatalf("deleted-project grant remained: %d %v", count, err)
	}
	stored, err := repo.GetTenantExternalAPICredential(ctx, tenant.Hex(), targetKey.Hex())
	if err != nil || stored.EncryptedSecret != "single-ciphertext" {
		t.Fatalf("cleanup altered shared secret: %#v %v", stored, err)
	}
	status, raw = request("GET", "/templates/"+sourceID.Hex()+"/external-api-requirements", "")
	if status != 200 || strings.Contains(string(raw), "single-ciphertext") {
		t.Fatalf("requirements status=%d: %s", status, raw)
	}

	// A foreign/global template advertises generic requirements, never foreign credential labels.
	foreignTenant, foreignTemplate, foreignKey := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	if _, err = db.Collection("projects").InsertOne(ctx, models.Project{ID: foreignTemplate, TenantID: foreignTenant, IsActive: true, IsTemplate: true, TemplateScope: "global"}); err != nil {
		t.Fatal(err)
	}
	foreignContainers := nestedCredentialTemplate(foreignKey.Hex())
	foreignContainers[0].ID = primitive.NewObjectID()
	if _, err = db.Collection(utils.GetProjectCollectionName(foreignTenant.Hex(), foreignTemplate.Hex(), "containers")).InsertOne(ctx, foreignContainers[0]); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.InsertExternalAPICredential(ctx, models.ExternalAPICredential{ID: foreignKey, TenantID: foreignTenant, ProjectID: foreignTemplate, Name: "private-foreign-provider-name", EncryptedSecret: "foreign-ciphertext"}); err != nil {
		t.Fatal(err)
	}
	status, raw = request("GET", "/templates/"+foreignTemplate.Hex()+"/external-api-requirements", "")
	if status != 200 || strings.Contains(string(raw), "private-foreign-provider-name") || strings.Contains(string(raw), "foreign-ciphertext") || !strings.Contains(string(raw), "External API connection 1") {
		t.Fatalf("foreign requirement leaked metadata: status=%d %s", status, raw)
	}

	// Deleting the originating project after promotion keeps the shared key and another project's grant.
	remainingProject, promotedKey := primitive.NewObjectID(), primitive.NewObjectID()
	if _, err = db.Collection("projects").InsertOne(ctx, models.Project{ID: remainingProject, TenantID: tenant, IsActive: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.InsertExternalAPICredential(ctx, models.ExternalAPICredential{ID: promotedKey, TenantID: tenant, ProjectID: sourceID, EncryptedSecret: "promoted-single-secret"}); err != nil {
		t.Fatal(err)
	}
	if err = repo.PromoteExternalAPICredential(ctx, tenant.Hex(), promotedKey.Hex(), user); err != nil {
		t.Fatal(err)
	}
	if err = repo.GrantExternalAPICredential(ctx, tenant.Hex(), promotedKey.Hex(), remainingProject.Hex(), user); err != nil {
		t.Fatal(err)
	}
	status, raw = request("DELETE", "/projects/"+sourceID.Hex(), "")
	if status != 200 {
		t.Fatalf("delete source status=%d: %s", status, raw)
	}
	stored, err = repo.GetExternalAPICredential(ctx, tenant.Hex(), remainingProject.Hex(), promotedKey.Hex())
	if err != nil || stored.EncryptedSecret != "promoted-single-secret" {
		t.Fatalf("deleting original project broke shared key: %#v %v", stored, err)
	}
	count, err = db.Collection("external_api_credential_grants").CountDocuments(ctx, bson.M{"projectId": sourceID})
	if err != nil || count != 0 {
		t.Fatalf("origin grant remained: %d %v", count, err)
	}
}
