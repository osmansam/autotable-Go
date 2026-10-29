package controllers

import (
	"context"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"regexp"
	"testing"
)

func TestProjectCollectionNamePatternOnlyMatchesOwnedCollections(t *testing.T) {
	tenantID := "65a1b2c3d4e5f67890abcdef"
	projectID := "78f9e8d7c6b5a43210fedcba"
	re := regexp.MustCompile(projectCollectionNamePattern(tenantID, projectID))

	owned := []string{
		"tenant_65a1b2c3d4e5f67890abcdef_project_78f9e8d7c6b5a43210fedcba_containers",
		"tenant_65a1b2c3d4e5f67890abcdef_project_78f9e8d7c6b5a43210fedcba_pages",
		"tenant_65a1b2c3d4e5f67890abcdef_project_78f9e8d7c6b5a43210fedcba_products",
	}
	for _, name := range owned {
		if !re.MatchString(name) {
			t.Errorf("expected %q to match", name)
		}
	}

	notOwned := []string{
		"projects",
		"containers",
		"tenant_65a1b2c3d4e5f67890abcdef_project_000000000000000000000000_pages",
		"tenant_000000000000000000000000_project_78f9e8d7c6b5a43210fedcba_pages",
		"x_tenant_65a1b2c3d4e5f67890abcdef_project_78f9e8d7c6b5a43210fedcba_pages",
	}
	for _, name := range notOwned {
		if re.MatchString(name) {
			t.Errorf("expected %q not to match", name)
		}
	}
}

func TestProjectCleanupAttemptsGrantRemovalAfterCollectionDropFailure(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("attempt grants independently", func(mt *mtest.T) {
		tenant, project := primitive.NewObjectID(), primitive.NewObjectID()
		name := GetProjectPrefix(tenant.Hex(), project.Hex()) + "containers"
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".$cmd.listCollections", mtest.FirstBatch, bson.D{{Key: "name", Value: name}}), mtest.CreateCommandErrorResponse(mtest.CommandError{Code: 13, Message: "drop denied"}))
		for range projectScopedSharedCollections {
			mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}))
		}
		_, err := deleteProjectDataFromDatabase(context.Background(), mt.DB, tenant, project)
		if err == nil {
			mt.Fatal("drop failure ignored")
		}
		attemptedGrantCleanup := false
		for _, event := range mt.GetAllStartedEvents() {
			if event.CommandName == "delete" && event.Command.Lookup("delete").StringValue() == "external_api_credential_grants" {
				attemptedGrantCleanup = true
			}
		}
		if !attemptedGrantCleanup {
			mt.Fatal("collection drop failure skipped grant removal")
		}
	})
}

func TestFailedCreateRollbackPreservesInactiveProjectWhenCleanupFails(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("recoverable project remains", func(mt *mtest.T) {
		tenant, project := primitive.NewObjectID(), primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}), mtest.CreateCommandErrorResponse(mtest.CommandError{Code: 13, Message: "list collections denied"}))
		for i := 0; i < len(projectScopedSharedCollections)+2; i++ {
			mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}))
		}
		if err := rollbackProjectCreateFromDatabase(context.Background(), mt.DB, tenant, project); err == nil {
			mt.Fatal("failed cleanup reported success")
		}
		for _, event := range mt.GetAllStartedEvents() {
			if event.CommandName == "delete" && (event.Command.Lookup("delete").StringValue() == "projects" || event.Command.Lookup("delete").StringValue() == "project_memberships") {
				mt.Fatalf("failed cleanup deleted recoverable ownership: %s", event.Command)
			}
		}
	})
}
