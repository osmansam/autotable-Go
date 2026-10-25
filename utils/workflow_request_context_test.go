package utils

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestWorkflowRequestContextReadsFreshScopeAndMetadataTogether(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()
	mt.Run("one scoped aggregate", func(mt *mtest.T) {
		setupMiniRedis(mt.T)
		restore := useMockCollections(mt.Coll)
		defer restore()
		tenant, project := primitive.NewObjectID(), primitive.NewObjectID()
		seedRequestProject(mt.T, "acme", "store", tenant.Hex(), project.Hex())
		mt.AddMockResponses(mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{
			{Key: "containers", Value: bson.A{bson.D{
				{Key: "schemaName", Value: "siteConfig"},
				{Key: "fields", Value: bson.A{bson.D{{Key: "name", Value: "later"}, {Key: "order", Value: 2}}, bson.D{{Key: "name", Value: "first"}, {Key: "order", Value: 1}}}},
				{Key: "workflows", Value: bson.A{bson.D{{Key: "name", Value: "loadPublicPage"}, {Key: "isActive", Value: true}, {Key: "isAuthenticated", Value: true}}}},
			}}},
		}))
		app := fiber.New()
		app.Get("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
			tenantID, projectID, container, err := GetWorkflowRequestContext(c, "siteConfig")
			if err != nil || tenantID != tenant.Hex() || projectID != project.Hex() {
				mt.Errorf("context = %q, %q, %v", tenantID, projectID, err)
				return c.SendStatus(500)
			}
			if container.SchemaName != "siteConfig" || container.Fields[0].Name != "first" || !container.Workflows[0].IsAuthenticated {
				mt.Errorf("metadata lost permissions or ordering: %+v", container)
			}
			if tID, pID, err := GetTenantAndProjectContext(c); err != nil || tID != tenantID || pID != projectID {
				mt.Errorf("handler context = %q, %q, %v", tID, pID, err)
			}
			return c.SendStatus(200)
		})
		assertProjectRequestStatus(mt.T, app, "/acme/store", 200)
		events := mt.GetAllStartedEvents()
		if len(events) != 1 || events[0].CommandName != "aggregate" {
			mt.Fatalf("database commands = %+v, want one aggregate", events)
		}
		var command struct {
			Pipeline []bson.M `bson:"pipeline"`
		}
		if err := bson.Unmarshal(events[0].Command, &command); err != nil {
			mt.Fatal(err)
		}
		wantMatch := bson.M{"_id": project, "tenantId": tenant, "isActive": true}
		if !reflect.DeepEqual(command.Pipeline[0]["$match"], wantMatch) {
			mt.Errorf("project filter = %#v", command.Pipeline[0])
		}
		lookup := command.Pipeline[1]["$lookup"].(bson.M)
		if lookup["from"] != "tenant_"+tenant.Hex()+"_project_"+project.Hex()+"_containers" {
			mt.Errorf("unscoped collection = %v", lookup["from"])
		}
		steps := lookup["pipeline"].(bson.A)
		if !reflect.DeepEqual(steps[0], bson.M{"$match": bson.M{"schemaName": "siteConfig"}}) {
			mt.Errorf("schema filter = %#v", steps[0])
		}
	})
}

func TestWorkflowRequestContextPreservesAuthenticationAndFailures(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()
	for _, tc := range []struct {
		name, authField               string
		missingProject, missingSchema bool
		want                          error
	}{
		{name: "foreign tenant", authField: "tenantID", want: ErrProjectMismatch},
		{name: "foreign project", authField: "projectID", want: ErrProjectMismatch},
		{name: "inactive or deleted project", missingProject: true, want: ErrProjectNotFound},
		{name: "missing schema", missingSchema: true, want: mongo.ErrNoDocuments},
	} {
		mt.Run(tc.name, func(mt *mtest.T) {
			setupMiniRedis(mt.T)
			restore := useMockCollections(mt.Coll)
			defer restore()
			tenant, project := primitive.NewObjectID(), primitive.NewObjectID()
			seedRequestProject(mt.T, "acme", "store", tenant.Hex(), project.Hex())
			var docs []bson.D
			if !tc.missingProject {
				containers := bson.A{}
				if !tc.missingSchema {
					containers = append(containers, bson.D{{Key: "schemaName", Value: "siteConfig"}})
				}
				docs = append(docs, bson.D{{Key: "containers", Value: containers}})
			}
			mt.AddMockResponses(mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, docs...))
			app := fiber.New()
			app.Get("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
				if tc.authField != "" {
					c.Locals(tc.authField, "foreign")
				}
				_, _, container, err := GetWorkflowRequestContext(c, "siteConfig")
				if !errors.Is(err, tc.want) || container != nil {
					mt.Errorf("rejected context = %+v, %v, want %v", container, err, tc.want)
				}
				if tc.authField != "" && c.Locals(tc.authField) != "foreign" {
					mt.Error("authentication scope overwritten")
				}
				if c.Locals(resolvedProjectContextKey{}) != nil {
					mt.Error("failed context retained as verified")
				}
				return c.SendStatus(200)
			})
			assertProjectRequestStatus(mt.T, app, "/acme/store", 200)
			if tc.missingProject {
				if exists := configs.RedisClient.Exists(context.Background(), "slug_mapping:acme:store").Val(); exists != 0 {
					mt.Error("inactive/deleted project mapping was retained, preventing fresh slug resolution")
				}
			}
		})
	}
}

func TestWorkflowRequestContextDoesNotCachePermissionsAcrossRequests(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()
	mt.Run("next request sees changed workflow permissions", func(mt *mtest.T) {
		setupMiniRedis(mt.T)
		restore := useMockCollections(mt.Coll)
		defer restore()
		tenant, project := primitive.NewObjectID(), primitive.NewObjectID()
		seedRequestProject(mt.T, "acme", "store", tenant.Hex(), project.Hex())
		for _, auth := range []bool{false, true} {
			mt.AddMockResponses(mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "containers", Value: bson.A{bson.D{
				{Key: "schemaName", Value: "siteConfig"}, {Key: "workflows", Value: bson.A{bson.D{{Key: "name", Value: "loadPublicPage"}, {Key: "isAuthenticated", Value: auth}}}},
			}}}}))
		}
		var observed []bool
		app := fiber.New()
		app.Get("/:tenantSlug/:projectSlug", func(c *fiber.Ctx) error {
			_, _, metadata, err := GetWorkflowRequestContext(c, "siteConfig")
			if err != nil {
				mt.Error(err)
				return c.SendStatus(500)
			}
			observed = append(observed, metadata.Workflows[0].IsAuthenticated)
			return c.SendStatus(200)
		})
		assertProjectRequestStatus(mt.T, app, "/acme/store", 200)
		assertProjectRequestStatus(mt.T, app, "/acme/store", 200)
		if !reflect.DeepEqual(observed, []bool{false, true}) {
			mt.Errorf("permissions = %v, want fresh permissions each request", observed)
		}
		if len(mt.GetAllStartedEvents()) != 2 {
			mt.Errorf("want one fresh query per request")
		}
	})
}

func TestWorkflowRequestContextColdAndLegacyFallback(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()
	for _, legacy := range []bool{false, true} {
		name := "cold slug mapping"
		if legacy {
			name = "legacy query scope"
		}
		mt.Run(name, func(mt *mtest.T) {
			setupMiniRedis(mt.T)
			restore := useMockCollections(mt.Coll)
			defer restore()
			tenant, project := primitive.NewObjectID(), primitive.NewObjectID()
			if !legacy {
				mt.AddMockResponses(mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "_id", Value: project}, {Key: "tenantId", Value: tenant}, {Key: "isActive", Value: true}}))
			}
			mt.AddMockResponses(mtest.CreateCursorResponse(0, namespace(mt.Coll), mtest.FirstBatch, bson.D{{Key: "schemaName", Value: "siteConfig"}}))
			app := fiber.New()
			path := "/acme/store"
			route := "/:tenantSlug/:projectSlug"
			if legacy {
				route = "/"
				path = "/?tenantID=" + tenant.Hex() + "&projectID=" + project.Hex()
			}
			app.Get(route, func(c *fiber.Ctx) error {
				tID, pID, container, err := GetWorkflowRequestContext(c, "siteConfig")
				if err != nil || tID != tenant.Hex() || pID != project.Hex() || container == nil || container.SchemaName != "siteConfig" {
					mt.Errorf("fallback = %s, %s, %+v, %v", tID, pID, container, err)
				}
				return c.SendStatus(200)
			})
			assertProjectRequestStatus(mt.T, app, path, 200)
			want := 2
			if legacy {
				want = 1
			}
			if got := len(mt.GetAllStartedEvents()); got != want {
				mt.Errorf("fallback queries = %d, want %d", got, want)
			}
		})
	}
}
