package controllers

import (
	"context"
	"fmt"
	"regexp"

	"github.com/osmansam/autotableGo/configs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// projectScopedSharedCollections are global collections that store rows tagged
// with a projectId. Rows belonging to a deleted project are removed from them.
// Audit logs are intentionally excluded so the audit trail survives deletion.
var projectScopedSharedCollections = []string{
	"project_translations",
	"translation_jobs",
	"project_locale_preferences",
	"integration_credentials",
	"external_api_credentials",
	"workflow_cron_schedules",
	"dynamic_outbox",
}

// projectCollectionNamePattern matches every physical collection that belongs to
// a project (tenant_{tenantId}_project_{projectId}_*).
func projectCollectionNamePattern(tenantID, projectID string) string {
	return "^" + regexp.QuoteMeta(GetProjectPrefix(tenantID, projectID))
}

// listProjectCollections returns the names of all physical collections owned by the project.
func listProjectCollections(ctx context.Context, db *mongo.Database, tenantID, projectID string) ([]string, error) {
	return db.ListCollectionNames(ctx, bson.M{
		"name": bson.M{"$regex": projectCollectionNamePattern(tenantID, projectID)},
	})
}

// deleteProjectData drops every collection owned by the project and removes the
// project's rows from shared collections. It is idempotent, so a failed
// deletion can be safely retried.
func deleteProjectData(ctx context.Context, tenantOID, projectOID primitive.ObjectID) (int, error) {
	tenantID := tenantOID.Hex()
	projectID := projectOID.Hex()
	db := configs.GetCollection("projects").Database()

	names, err := listProjectCollections(ctx, db, tenantID, projectID)
	if err != nil {
		return 0, fmt.Errorf("list project collections: %w", err)
	}
	dropped := 0
	for _, name := range names {
		if err := db.Collection(name).Drop(ctx); err != nil {
			return dropped, fmt.Errorf("drop collection %s: %w", name, err)
		}
		dropped++
	}

	// projectId is stored as an ObjectID in some collections and as a hex string in others.
	filter := bson.M{"projectId": bson.M{"$in": bson.A{projectOID, projectID}}}
	for _, name := range projectScopedSharedCollections {
		if _, err := db.Collection(name).DeleteMany(ctx, filter); err != nil {
			return dropped, fmt.Errorf("delete project rows from %s: %w", name, err)
		}
	}
	return dropped, nil
}
