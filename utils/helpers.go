// utils/helpers.go
package utils

import (
	"context"
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
)

// ErrNoSchemaName is returned when the schemaName query is missing.
var ErrNoSchemaName = errors.New("schemaName is required")

// FetchContainerModel tries fiber.Locals first, then falls back to DB.
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
    return GetContainerModel(name)
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

// ParsePager reads `?page=` and `?limit=`. If either is missing, returns Enabled=false.
func ParsePager(c *fiber.Ctx) (Pager, error) {
    pStr, lStr := c.Query("page"), c.Query("limit")
    if pStr == "" || lStr == "" {
        return Pager{Enabled: false}, nil
    }
    p, err1 := strconv.Atoi(pStr)
    l, err2 := strconv.Atoi(lStr)
    if err1 != nil || err2 != nil || p < 1 || l < 1 {
        return Pager{}, errors.New("invalid pagination parameters")
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
    return opts
}

// QueryAndDecode runs `.Find()` + `DecodeCursor`, and if pager.Enabled, also counts total.
func QueryAndDecode(
    ctx context.Context,
    collName string,
    filter bson.M,
    opts *options.FindOptions,
    pager *Pager,
) ([]map[string]interface{}, error) {
    coll := configs.GetCollection(collName)
    
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
        // Always count with the current filter to ensure accuracy
        // (filters and search can vary, so we can't cache a single count)
        total, err := coll.CountDocuments(ctx, filter)
        if err != nil {
            return nil, err
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

// PopulateIfNeeded calls PopulateItems if the route is in PopulatedRoutes.
func PopulateIfNeeded(
    ctx context.Context,
    container *models.ContainerModel,
    routeName string,
    items []map[string]interface{},
) ([]map[string]interface{}, error) {
    for _, r := range container.PopulatedRoutes {
        if r == routeName {
            return PopulateItems(ctx, container, items)
        }
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
