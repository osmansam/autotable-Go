package repositories

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestLocalizationRepositoryManualUpsertSetsManualOrigin(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("manual origin", func(mt *mtest.T) {
		repository := NewLocalizationRepositoryWithCollections(mt.Coll, mt.Coll, mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse())
		entry := models.TranslationEntry{
			TenantID:       primitive.NewObjectID(),
			ProjectID:      primitive.NewObjectID(),
			Locale:         "tr",
			TranslationKey: "page:abc.name",
			TranslatedText: "Elle düzenlendi",
		}

		if err := repository.UpsertManualTranslation(context.Background(), entry); err != nil {
			mt.Fatal(err)
		}
		updates, _ := mt.GetStartedEvent().Command.Lookup("updates").Array().Values()
		update := updates[0].Document().Lookup("u").Document().Lookup("$set").Document()
		if got := update.Lookup("origin").StringValue(); got != string(models.TranslationOriginManual) {
			mt.Fatalf("origin = %q, want manual", got)
		}
	})
}

func TestLocalizationRepositoryGeneratedUpsertCannotOverwriteManualEntry(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("filters out manual entries", func(mt *mtest.T) {
		repository := NewLocalizationRepositoryWithCollections(mt.Coll, mt.Coll, mt.Coll)
		mt.AddMockResponses(
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 0}),
			mtest.CreateWriteErrorsResponse(mtest.WriteError{Index: 0, Code: 11000, Message: "duplicate translation identity"}),
		)

		entry := models.TranslationEntry{
			TenantID:       primitive.NewObjectID(),
			ProjectID:      primitive.NewObjectID(),
			Locale:         "tr",
			TranslationKey: "page:abc.name",
			TranslatedText: "Siparişler",
			SourceHash:     "hash",
		}
		if err := repository.UpsertGeneratedTranslation(context.Background(), entry); err != nil {
			mt.Fatal(err)
		}

		command := mt.GetStartedEvent().Command
		updates, err := command.Lookup("updates").Array().Values()
		if err != nil || len(updates) != 1 {
			mt.Fatalf("updates command = %v, err = %v", command, err)
		}
		filter := updates[0].Document().Lookup("q").Document()
		origin := filter.Lookup("origin").Document()
		if got := origin.Lookup("$ne").StringValue(); got != string(models.TranslationOriginManual) {
			mt.Fatalf("origin protection = %q, want %q", got, models.TranslationOriginManual)
		}
	})
}

func TestLocalizationRepositoryUpsertsProjectScopedPreference(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("uses user tenant and project identity", func(mt *mtest.T) {
		repository := NewLocalizationRepositoryWithCollections(mt.Coll, mt.Coll, mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse())
		preference := models.ProjectLocalePreference{
			UserID:    primitive.NewObjectID(),
			TenantID:  primitive.NewObjectID(),
			ProjectID: primitive.NewObjectID(),
			Locale:    "tr",
			UpdatedAt: time.Now(),
		}

		if err := repository.UpsertPreference(context.Background(), preference); err != nil {
			mt.Fatal(err)
		}

		command := mt.GetStartedEvent().Command
		updates, _ := command.Lookup("updates").Array().Values()
		filter := updates[0].Document().Lookup("q").Document()
		for key, want := range map[string]primitive.ObjectID{"userId": preference.UserID, "tenantId": preference.TenantID, "projectId": preference.ProjectID} {
			if got := filter.Lookup(key).ObjectID(); got != want {
				mt.Fatalf("%s filter = %s, want %s", key, got, want)
			}
		}
		if upsert := updates[0].Document().Lookup("upsert").Boolean(); !upsert {
			mt.Fatal("preference update is not an upsert")
		}
	})
}

func TestLocalizationRepositoryGetsProjectScopedPreference(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("scoped preference", func(mt *mtest.T) {
		repository := NewLocalizationRepositoryWithCollections(mt.Coll, mt.Coll, mt.Coll)
		userID := primitive.NewObjectID()
		tenantID := primitive.NewObjectID()
		projectID := primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.Coll.Database().Name()+"."+mt.Coll.Name(), mtest.FirstBatch,
			bson.D{{Key: "userId", Value: userID}, {Key: "tenantId", Value: tenantID}, {Key: "projectId", Value: projectID}, {Key: "locale", Value: "tr"}},
		))

		preference, err := repository.GetPreference(context.Background(), userID, tenantID, projectID)
		if err != nil {
			mt.Fatal(err)
		}
		if preference.Locale != "tr" {
			mt.Fatalf("locale = %q, want tr", preference.Locale)
		}
		filter := mt.GetStartedEvent().Command.Lookup("filter").Document()
		if filter.Lookup("projectId").ObjectID() != projectID {
			mt.Fatalf("project filter = %v, want %v", filter.Lookup("projectId"), projectID)
		}
	})
}

func TestLocalizationRepositoryLeaseNextJobUsesAtomicLeaseFilter(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("atomic lease", func(mt *mtest.T) {
		repository := NewLocalizationRepositoryWithCollections(mt.Coll, mt.Coll, mt.Coll)
		now := time.Now().UTC()
		jobID := primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "value", Value: bson.D{
			{Key: "_id", Value: jobID}, {Key: "status", Value: models.TranslationJobRunning}, {Key: "leaseOwner", Value: "worker-1"},
		}}))

		job, err := repository.LeaseNextJob(context.Background(), "worker-1", now, time.Minute)
		if err != nil {
			mt.Fatal(err)
		}
		if job.ID != jobID || job.LeaseOwner != "worker-1" {
			mt.Fatalf("leased job = %#v", job)
		}
		command := mt.GetStartedEvent().Command
		if command.Lookup("findAndModify").StringValue() == "" {
			mt.Fatal("lease did not use findAndModify")
		}
		query := command.Lookup("query").Document().String()
		for _, required := range []string{"nextAttemptAt", "leaseExpiresAt", string(models.TranslationJobPending)} {
			if !strings.Contains(query, required) {
				mt.Fatalf("lease query %q missing %q", query, required)
			}
		}
	})
}

func TestLocalizationIndexDefinitionsContainRequiredUniqueIdentities(t *testing.T) {
	translationIndexes, preferenceIndexes, jobIndexes := localizationIndexModels()
	if len(translationIndexes) < 2 || len(preferenceIndexes) < 1 || len(jobIndexes) < 1 {
		t.Fatalf("index counts = %d/%d/%d", len(translationIndexes), len(preferenceIndexes), len(jobIndexes))
	}
	assertUniqueIndexKeys(t, translationIndexes[0].Keys, []string{"tenantId", "projectId", "locale", "translationKey"})
	assertUniqueIndexKeys(t, preferenceIndexes[0].Keys, []string{"userId", "tenantId", "projectId"})
}

func assertUniqueIndexKeys(t *testing.T, keys interface{}, want []string) {
	t.Helper()
	document, ok := keys.(bson.D)
	if !ok {
		t.Fatalf("index keys type = %T, want bson.D", keys)
	}
	if len(document) != len(want) {
		t.Fatalf("index keys = %#v, want %v", document, want)
	}
	for i, key := range want {
		if document[i].Key != key {
			t.Fatalf("index key %d = %q, want %q", i, document[i].Key, key)
		}
	}
}
