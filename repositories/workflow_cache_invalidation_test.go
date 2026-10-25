package repositories

import (
	"context"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestLocalizationWritesInvalidateWorkflowResults(t *testing.T) {
	server := miniredis.RunT(t)
	previous := configs.RedisClient
	configs.RedisClient = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = configs.RedisClient.Close(); configs.RedisClient = previous })
	ctx := context.Background()
	tenant, project := primitive.NewObjectID(), primitive.NewObjectID()
	entry := models.TranslationEntry{TenantID: tenant, ProjectID: project, Locale: "tr", TranslationKey: "title"}
	key := fmt.Sprintf("tenant:%s:project:%s:workflow-cache:revision", tenant.Hex(), project.Hex())
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	for _, name := range []string{"manual", "generated", "placeholder", "reconciliation", "failed-generation", "locale-settings", "localization-version"} {
		mt.Run(name, func(mt *mtest.T) {
			if err := configs.RedisClient.Set(ctx, key, 7, 0).Err(); err != nil {
				mt.Fatal(err)
			}
			mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}, bson.E{Key: "value", Value: bson.D{{Key: "_id", Value: project}, {Key: "tenantId", Value: tenant}}}))
			r := NewLocalizationRepositoryWithCollections(mt.Coll, mt.Coll, mt.Coll)
			p := NewProjectLocalizationRepositoryWithCollection(mt.Coll)
			var err error
			switch name {
			case "manual":
				err = r.UpsertManualTranslation(ctx, entry)
			case "generated":
				_, err = r.UpsertGeneratedTranslation(ctx, entry)
			case "placeholder":
				err = r.UpsertPlaceholder(ctx, entry)
			case "reconciliation":
				err = r.ApplyReconciledEntries(ctx, []models.TranslationEntry{entry})
			case "failed-generation":
				err = r.MarkGeneratedTranslationFailed(ctx, entry)
			case "locale-settings":
				_, err = p.UpdateLocaleSettings(ctx, tenant, project, "en", "tr", []string{"en", "tr"})
			case "localization-version":
				err = p.IncrementVersion(ctx, tenant, project)
			}
			if err != nil {
				mt.Fatal(err)
			}
			if got, err := configs.RedisClient.Get(ctx, key).Int64(); err != nil || got != 8 {
				mt.Fatalf("workflow revision after %s = %d, %v; want 8", name, got, err)
			}
		})
	}
}
