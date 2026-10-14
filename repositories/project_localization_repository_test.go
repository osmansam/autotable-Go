package repositories

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestProjectLocalizationRepositoryGetProjectIsTenantScoped(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("tenant and project filter", func(mt *mtest.T) {
		repository := NewProjectLocalizationRepositoryWithCollection(mt.Coll)
		tenantID, projectID := primitive.NewObjectID(), primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.Coll.Database().Name()+"."+mt.Coll.Name(), mtest.FirstBatch,
			bson.D{{Key: "_id", Value: projectID}, {Key: "tenantId", Value: tenantID}, {Key: "sourceLocale", Value: "en"}},
		))

		project, err := repository.GetProject(context.Background(), tenantID, projectID)
		if err != nil {
			mt.Fatal(err)
		}
		if project.ID != projectID || project.DefaultLocale != "en" || project.LocalizationVersion != 1 {
			mt.Fatalf("project = %#v", project)
		}
		assertProjectScope(t, mt.GetStartedEvent().Command.Lookup("filter").Document(), tenantID, projectID)
	})
}

func TestProjectLocalizationRepositoryIncrementVersionIsTenantScoped(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("scoped increment", func(mt *mtest.T) {
		repository := NewProjectLocalizationRepositoryWithCollection(mt.Coll)
		tenantID, projectID := primitive.NewObjectID(), primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))

		if err := repository.IncrementVersion(context.Background(), tenantID, projectID); err != nil {
			mt.Fatal(err)
		}
		updates, _ := mt.GetStartedEvent().Command.Lookup("updates").Array().Values()
		assertProjectScope(t, updates[0].Document().Lookup("q").Document(), tenantID, projectID)
		increment := updates[0].Document().Lookup("u").Document().Lookup("$inc").Document()
		if got := increment.Lookup("localizationVersion").Int32(); got != 1 {
			mt.Fatalf("localizationVersion increment = %d", got)
		}
	})
}

func TestProjectLocalizationRepositoryUpdateLocaleSettingsReturnsUpdatedProject(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("settings and version update atomically", func(mt *mtest.T) {
		repository := NewProjectLocalizationRepositoryWithCollection(mt.Coll)
		tenantID, projectID := primitive.NewObjectID(), primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "value", Value: bson.D{
			{Key: "_id", Value: projectID}, {Key: "tenantId", Value: tenantID}, {Key: "sourceLocale", Value: "en"},
			{Key: "defaultLocale", Value: "tr"}, {Key: "enabledLocales", Value: bson.A{"en", "tr"}}, {Key: "localizationVersion", Value: int64(3)},
		}}))

		project, err := repository.UpdateLocaleSettings(context.Background(), tenantID, projectID, "en", "tr", []string{"en", "tr"})
		if err != nil {
			mt.Fatal(err)
		}
		if project.LocalizationVersion != 3 || project.DefaultLocale != "tr" {
			mt.Fatalf("project = %#v", project)
		}
		command := mt.GetStartedEvent().Command
		assertProjectScope(t, command.Lookup("query").Document(), tenantID, projectID)
		update := command.Lookup("update").Document()
		if got := update.Lookup("$inc").Document().Lookup("localizationVersion").Int32(); got != 1 {
			mt.Fatalf("version increment = %d", got)
		}
		if command.Lookup("new").Boolean() != true {
			mt.Fatal("locale settings update did not request the updated project")
		}
	})
}

func assertProjectScope(t *testing.T, filter bson.Raw, tenantID, projectID primitive.ObjectID) {
	t.Helper()
	for key, want := range map[string]primitive.ObjectID{"tenantId": tenantID, "_id": projectID} {
		if got := filter.Lookup(key).ObjectID(); got != want {
			t.Fatalf("%s filter = %s, want %s", key, got, want)
		}
	}
}
