// utils/helpers.go
package utils

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
)

// ErrNoSchemaName is returned when the schemaName query is missing.
var ErrNoSchemaName = errors.New("schemaName is required")

// FetchContainerModel tries fiber.Locals first, then falls back to DB.
// Requires tenantID and projectID from c.Locals() for project-specific access
func FetchContainerModel(c *fiber.Ctx) (*models.ContainerModel, error) {
	name := c.Query("schemaName")
	if name == "" {
		return nil, ErrNoSchemaName
	}
	if stored := c.Locals("containerModel"); stored != nil {
		if cm, ok := stored.(*models.ContainerModel); ok {
			return cm, nil
		}
	}

	// Extract tenant and project IDs from context
	tenantID, _ := c.Locals("tenantID").(string)
	projectID, _ := c.Locals("projectID").(string)
	if tenantID == "" || projectID == "" {
		return nil, errors.New("missing tenant or project context")
	}

	return GetContainerModel(tenantID, projectID, name)
}

// Pager holds pagination parameters & metadata.
type Pager struct {
	Enabled    bool
	Page       int
	Limit      int
	Skip       int64
	TotalItems int64
	TotalPages int
}

// ParsePager reads `?page=` and `?limit=`. If both are missing, returns Enabled=false.
func ParsePager(c *fiber.Ctx) (Pager, error) {
	pStr, lStr := c.Query("page"), c.Query("limit")
	if pStr == "" && lStr == "" {
		return Pager{Enabled: false}, nil
	}

	p := 1
	if pStr != "" {
		parsedPage, err := strconv.Atoi(pStr)
		if err != nil {
			return Pager{}, errors.New("invalid pagination parameters")
		}
		p = parsedPage
	}

	l := configs.GetDefaultPageLimit()
	if lStr != "" {
		parsedLimit, err := strconv.Atoi(lStr)
		if err != nil {
			return Pager{}, errors.New("invalid pagination parameters")
		}
		l = parsedLimit
	}

	if p < 1 || l < 1 {
		return Pager{}, errors.New("invalid pagination parameters")
	}

	if maxLimit := configs.GetMaxPageLimit(); l > maxLimit {
		log.Printf("Pagination limit exceeded: requested=%d max=%d path=%s", l, maxLimit, c.Path())
		l = maxLimit
	}

	return Pager{
		Enabled: true,
		Page:    p,
		Limit:   l,
		Skip:    int64((p - 1) * l),
	}, nil
}

// ParseSort reads `?sort=` and `?asc=` and returns a bson.D or nil.
func ParseSort(c *fiber.Ctx) (bson.D, error) {
	field, ascStr := c.Query("sort"), c.Query("asc")
	if field == "" || ascStr == "" {
		return nil, nil
	}
	asc, err := strconv.ParseBool(ascStr)
	if err != nil {
		return nil, err
	}
	dir := int32(1)
	if !asc {
		dir = -1
	}
	return bson.D{{Key: field, Value: dir}}, nil
}

// BuildFindOptions applies sort + pagination to a new FindOptions.
func BuildFindOptions(sort bson.D, pager Pager) *options.FindOptions {
	opts := options.Find()
	if sort != nil {
		opts.SetSort(sort)
	}
	if pager.Enabled {
		opts.SetSkip(pager.Skip).SetLimit(int64(pager.Limit))
	}
	// OPTIMIZATION: Add max execution time to prevent slow queries from blocking
	opts.SetMaxTime(10 * time.Second)
	return opts
}

// QueryAndDecode runs `.Find()` + `DecodeCursor`, and if pager.Enabled, also counts total.
// Now requires tenantID and projectID for project-specific collection access
func QueryAndDecode(
	ctx context.Context,
	tenantID string,
	projectID string,
	collName string,
	filter bson.M,
	opts *options.FindOptions,
	pager *Pager,
) ([]map[string]interface{}, error) {
	return QueryAndDecodeCollection(ctx, GetDynamicCollectionForProject(tenantID, projectID, collName), collName, filter, opts, pager)
}

// QueryAndDecodeCollection executes QueryAndDecode against a supplied collection.
func QueryAndDecodeCollection(
	ctx context.Context,
	coll *mongo.Collection,
	collName string,
	filter bson.M,
	opts *options.FindOptions,
	pager *Pager,
) ([]map[string]interface{}, error) {
	// Execute query ONCE
	cur, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	// Decode all results
	items, err := DecodeCursor(cur, ctx)
	if err != nil {
		return nil, err
	}

	// Count total if pagination is enabled
	if pager.Enabled {
		// OPTIMIZATION: Use estimated count for empty filters, exact count otherwise
		var total int64

		if len(filter) == 0 {
			// For queries without filters, use estimatedDocumentCount (much faster)
			total, err = coll.EstimatedDocumentCount(ctx)
		} else {
			// For filtered queries, use CountDocuments with a timeout
			countCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			total, err = coll.CountDocuments(countCtx, filter, options.Count().SetMaxTime(3*time.Second))
		}

		if err != nil {
			// If count fails, estimate based on current page results
			log.Printf("Warning: Count failed for %s, estimating: %v", collName, err)
			if len(items) < pager.Limit {
				// Last page - calculate total
				total = int64((pager.Page-1)*pager.Limit + len(items))
			} else {
				// Not last page - estimate conservatively
				total = int64(pager.Page * pager.Limit)
			}
		}

		pager.TotalItems = total
		pager.TotalPages = int(total)/pager.Limit + boolToInt(total%int64(pager.Limit) > 0)
	}

	return items, nil
}

// StripHashed removes any fields marked IsHashed from every item.
func StripHashed(fields []models.Field, items []map[string]interface{}) {
	for _, f := range fields {
		if f.IsHashed {
			for _, doc := range items {
				delete(doc, f.Name)
			}
		}
	}
}

// PopulateIfNeeded calls PopulateItems if any field has PopulationSettings configured.
// This automatically populates fields that are configured for population, regardless of the route.
func PopulateIfNeeded(
	ctx context.Context,
	tenantID, projectID string,
	container *models.ContainerModel,
	items []map[string]interface{},
) ([]map[string]interface{}, error) {
	// Check if any field has population settings configured
	hasPopulationSettings := false
	for _, field := range container.Fields {
		if field.PopulationSettings != nil {
			hasPopulationSettings = true
			break
		}
	}

	// If any field has population settings, populate the items
	if hasPopulationSettings {
		return PopulateItems(ctx, tenantID, projectID, container, items)
	}

	return items, nil
}

// boolToInt helper
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// GetAuditLogsConfig fetches the audit logs authorization configuration from the settings collection
// It looks for a settings document with key: "audit_logs"
// Returns default config (authentication only, no role authorization) if not found in database
func GetAuditLogsConfig() (*models.AuditLogsConfig, error) {
	ctx := context.Background()
	collection := globalCollectionProvider("settings")

	// Find the audit_logs settings document
	var settings models.Settings
	err := collection.FindOne(ctx, bson.M{"key": "audit_logs"}).Decode(&settings)

	if err == nil && settings.AuditLogs != nil {
		return settings.AuditLogs, nil
	}

	// Return default configuration if not found
	// Default: requires authentication only, no role-based authorization
	return &models.AuditLogsConfig{
		IsAuthorized:  false,
		AuthorizeRole: []string{},
	}, nil
}
