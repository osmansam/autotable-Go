package utils

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// GetWorkflowRequestContext loads the active project and its current collection
// metadata in one database round trip when the URL's slug mapping is warm.
// Permissions are never cached across requests; callers must still authorize
// execution using the returned metadata before reading a workflow result cache.
func GetWorkflowRequestContext(c *fiber.Ctx, schemaName string) (tenantID, projectID string, container *models.ContainerModel, err error) {
	if schemaName == "" {
		return "", "", nil, ErrNoSchemaName
	}
	tenantSlug, projectSlug := c.Params("tenantSlug"), c.Params("projectSlug")
	resolved, alreadyResolved := c.Locals(resolvedProjectContextKey{}).(resolvedProjectContext)
	alreadyResolved = alreadyResolved && resolved.tenantSlug == tenantSlug && resolved.projectSlug == projectSlug
	ctx, cancel := context.WithTimeout(c.UserContext(), 10*time.Second)
	defer cancel()

	// Cold mappings, already-verified requests, and legacy query/local scopes
	// retain the existing resolver and metadata-loading behavior.
	fallback := func() (string, string, *models.ContainerModel, error) {
		tID, pID, resolveErr := GetTenantAndProjectContext(c)
		if resolveErr != nil {
			return "", "", nil, resolveErr
		}
		metadata, loadErr := GetContainerModelWithContext(ctx, tID, pID, schemaName)
		return tID, pID, metadata, loadErr
	}
	if tenantSlug == "" || projectSlug == "" || alreadyResolved || configs.RedisClient == nil {
		return fallback()
	}
	mapping, cacheErr := configs.RedisGetString(ctx, slugMappingCacheKey(tenantSlug, projectSlug))
	parts := splitCachedValue(mapping)
	if cacheErr != nil || len(parts) != 2 {
		return fallback()
	}
	tenantOID, tenantErr := primitive.ObjectIDFromHex(parts[0])
	projectOID, projectErr := primitive.ObjectIDFromHex(parts[1])
	if tenantErr != nil || projectErr != nil {
		return fallback()
	}
	tenantID, projectID = parts[0], parts[1]
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"_id": projectOID, "tenantId": tenantOID, "isActive": true}}},
		{{Key: "$lookup", Value: bson.M{
			"from": GetProjectCollectionName(tenantID, projectID, "containers"),
			"pipeline": mongo.Pipeline{
				{{Key: "$match", Value: bson.M{"schemaName": schemaName}}},
				{{Key: "$limit", Value: 1}},
			},
			"as": "containers",
		}}},
		{{Key: "$project", Value: bson.M{"_id": 0, "containers": 1}}},
	}
	cursor, queryErr := globalCollectionProvider("projects").Aggregate(ctx, pipeline)
	if queryErr != nil {
		return "", "", nil, queryErr
	}
	defer cursor.Close(ctx)
	if !cursor.Next(ctx) {
		if cursor.Err() != nil {
			return "", "", nil, cursor.Err()
		}
		// Match the regular resolver: discard deleted/inactive mappings so a
		// replacement project using the same slug can be resolved next request.
		_ = configs.RedisClient.Del(ctx, slugMappingCacheKey(tenantSlug, projectSlug)).Err()
		return "", "", nil, ErrProjectNotFound
	}
	var current struct {
		Containers []models.ContainerModel `bson:"containers"`
	}
	if decodeErr := cursor.Decode(&current); decodeErr != nil {
		return "", "", nil, decodeErr
	}
	if len(current.Containers) == 0 {
		return "", "", nil, mongo.ErrNoDocuments
	}
	// Authentication can be supplied before or after project resolution. Keep
	// the existing contextual resolver's tenant/project mismatch checks intact.
	tokenTenantID, _ := c.Locals("tenantID").(string)
	tokenProjectID, _ := c.Locals("projectID").(string)
	if (tokenTenantID != "" && tokenTenantID != tenantID) || (tokenProjectID != "" && tokenProjectID != projectID) {
		return "", "", nil, ErrProjectMismatch
	}
	container = &current.Containers[0]
	container.SortFieldsByOrder()
	setResolvedProjectContext(c, tenantSlug, projectSlug, tenantID, projectID)
	return tenantID, projectID, container, nil
}
