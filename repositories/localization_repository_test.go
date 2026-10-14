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
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 0}))

		entry := models.TranslationEntry{
			TenantID:       primitive.NewObjectID(),
			ProjectID:      primitive.NewObjectID(),
			Locale:         "tr",
			TranslationKey: "page:abc.name",
			TranslatedText: "Siparişler",
			SourceHash:     "hash",
		}
		stored, err := repository.UpsertGeneratedTranslation(context.Background(), entry)
		if err != nil {
			mt.Fatal(err)
		}
		if stored {
			mt.Fatal("generated translation reported stored after a manual-entry duplicate")
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
		if got := filter.Lookup("sourceHash").StringValue(); got != "hash" {
			mt.Fatalf("source hash guard = %q", got)
		}
		if !filter.Lookup("isActive").Boolean() {
			mt.Fatal("generated update can reactivate an inactive entry")
		}
		statuses, err := filter.Lookup("status").Document().Lookup("$in").Array().Values()
		if err != nil || len(statuses) != 3 {
			mt.Fatalf("eligible lifecycle statuses = %v, err = %v", statuses, err)
		}
		set := updates[0].Document().Lookup("u").Document().Lookup("$set").Document()
		for _, protected := range []string{"sourceText", "sourceHash", "resourceType", "resourceId", "propertyPath", "isActive", "lastDiscovered"} {
			if set.Lookup(protected).Type != 0 {
				mt.Fatalf("generated update overwrites reconciled field %q: %s", protected, set.String())
			}
		}
	})
}

func TestLocalizationRepositoryGeneratedUpsertReportsStored(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("updates generated entry", func(mt *mtest.T) {
		repository := NewLocalizationRepositoryWithCollections(mt.Coll, mt.Coll, mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))

		stored, err := repository.UpsertGeneratedTranslation(context.Background(), models.TranslationEntry{
			TenantID: primitive.NewObjectID(), ProjectID: primitive.NewObjectID(), Locale: "tr", TranslationKey: "page:abc.name",
		})
		if err != nil {
			mt.Fatal(err)
		}
		if !stored {
			mt.Fatal("generated translation update reported skipped")
		}
	})
}

func TestLocalizationRepositoryGeneratedFailureUsesSameLifecycleGuard(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("failure cannot overwrite reconciled state", func(mt *mtest.T) {
		repository := NewLocalizationRepositoryWithCollections(mt.Coll, mt.Coll, mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 0}))
		entry := models.TranslationEntry{
			TenantID: primitive.NewObjectID(), ProjectID: primitive.NewObjectID(), Locale: "tr",
			TranslationKey: "page:abc.name", SourceHash: "selected-source-hash",
		}
		if err := repository.MarkGeneratedTranslationFailed(context.Background(), entry); err != nil {
			mt.Fatal(err)
		}
		updates, _ := mt.GetStartedEvent().Command.Lookup("updates").Array().Values()
		filter := updates[0].Document().Lookup("q").Document()
		if filter.Lookup("sourceHash").StringValue() != entry.SourceHash || !filter.Lookup("isActive").Boolean() {
			mt.Fatalf("failure lifecycle guard = %s", filter.String())
		}
		set := updates[0].Document().Lookup("u").Document().Lookup("$set").Document()
		if set.Lookup("status").StringValue() != string(models.TranslationStatusFailed) || set.Lookup("sourceText").Type != 0 {
			mt.Fatalf("failure update = %s", set.String())
		}
	})
}

func TestLocalizationRepositoryUpsertPlaceholderPreservesTranslationFields(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("placeholder lifecycle is insert-only", func(mt *mtest.T) {
		repository := NewLocalizationRepositoryWithCollections(mt.Coll, mt.Coll, mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse())
		entry := models.TranslationEntry{
			TenantID: primitive.NewObjectID(), ProjectID: primitive.NewObjectID(), Locale: "tr",
			TranslationKey: "ozmishow:sitePage:page/a:title", ResourceType: "sitePage", ResourceID: "page/a",
			PropertyPath: "title", SourceText: "Orders", SourceHash: "hash", LastDiscovered: time.Now().UTC(),
		}

		if err := repository.UpsertPlaceholder(context.Background(), entry); err != nil {
			mt.Fatal(err)
		}
		updates, _ := mt.GetStartedEvent().Command.Lookup("updates").Array().Values()
		update := updates[0].Document().Lookup("u").Document()
		set := update.Lookup("$set").Document()
		for _, field := range []string{"resourceType", "resourceId", "propertyPath", "sourceText", "sourceHash", "isActive", "lastDiscovered"} {
			if set.Lookup(field).Type == 0 {
				mt.Fatalf("placeholder $set missing %q: %s", field, set.String())
			}
		}
		for _, protected := range []string{"translatedText", "origin", "status"} {
			if set.Lookup(protected).Type != 0 {
				mt.Fatalf("placeholder overwrites %q in $set: %s", protected, set.String())
			}
		}
		setOnInsert := update.Lookup("$setOnInsert").Document()
		if got := setOnInsert.Lookup("origin").StringValue(); got != string(models.TranslationOriginUntranslated) {
			mt.Fatalf("insert origin = %q", got)
		}
		if got := setOnInsert.Lookup("status").StringValue(); got != string(models.TranslationStatusMissing) {
			mt.Fatalf("insert status = %q", got)
		}
		if got := setOnInsert.Lookup("translatedText").StringValue(); got != "" {
			mt.Fatalf("insert translatedText = %q", got)
		}
		if !updates[0].Document().Lookup("upsert").Boolean() {
			mt.Fatal("placeholder update is not an upsert")
		}
	})
}

func TestLocalizationRepositoryApplyReconciledEntriesPersistsLifecycle(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("bulk reconciliation", func(mt *mtest.T) {
		repository := NewLocalizationRepositoryWithCollections(mt.Coll, mt.Coll, mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 2}, bson.E{Key: "nModified", Value: 2}))
		now := time.Now().UTC()
		entries := []models.TranslationEntry{
			{TenantID: primitive.NewObjectID(), ProjectID: primitive.NewObjectID(), Locale: "tr", TranslationKey: "one", Status: models.TranslationStatusOutdated, Origin: models.TranslationOriginManual, TranslatedText: "Bir", IsActive: true, LastDiscovered: now},
			{TenantID: primitive.NewObjectID(), ProjectID: primitive.NewObjectID(), Locale: "de", TranslationKey: "two", Status: models.TranslationStatusMissing, Origin: models.TranslationOriginUntranslated, IsActive: false, LastDiscovered: now},
		}

		if err := repository.ApplyReconciledEntries(context.Background(), entries); err != nil {
			mt.Fatal(err)
		}
		updates, err := mt.GetStartedEvent().Command.Lookup("updates").Array().Values()
		if err != nil || len(updates) != 2 {
			mt.Fatalf("updates = %v, err = %v", updates, err)
		}
		firstSet := updates[0].Document().Lookup("u").Document().Lookup("$set").Document()
		if got := firstSet.Lookup("status").StringValue(); got != string(models.TranslationStatusOutdated) {
			mt.Fatalf("status = %q", got)
		}
		if got := firstSet.Lookup("translatedText").StringValue(); got != "Bir" {
			mt.Fatalf("translated text = %q", got)
		}
	})
}

func TestLocalizationRepositoryGetsProjectScopedJob(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("job identity includes tenant and project", func(mt *mtest.T) {
		repository := NewLocalizationRepositoryWithCollections(mt.Coll, mt.Coll, mt.Coll)
		tenantID, projectID, jobID := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.Coll.Database().Name()+"."+mt.Coll.Name(), mtest.FirstBatch,
			bson.D{{Key: "_id", Value: jobID}, {Key: "tenantId", Value: tenantID}, {Key: "projectId", Value: projectID}, {Key: "status", Value: models.TranslationJobPending}},
		))

		job, err := repository.GetJob(context.Background(), tenantID, projectID, jobID)
		if err != nil {
			mt.Fatal(err)
		}
		if job.ID != jobID {
			mt.Fatalf("job id = %s", job.ID)
		}
		filter := mt.GetStartedEvent().Command.Lookup("filter").Document()
		for key, want := range map[string]primitive.ObjectID{"_id": jobID, "tenantId": tenantID, "projectId": projectID} {
			if got := filter.Lookup(key).ObjectID(); got != want {
				mt.Fatalf("%s = %s, want %s", key, got, want)
			}
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
		for _, required := range []string{"nextAttemptAt", "leaseExpiresAt", string(models.TranslationJobPending), string(models.TranslationJobRunning)} {
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
