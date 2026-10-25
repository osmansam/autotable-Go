package utils

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var (
	ErrTenantSlugMissing  = errors.New("tenant slug is required in URL path")
	ErrProjectSlugMissing = errors.New("project slug is required in URL path")
	ErrTenantNotFound     = errors.New("tenant not found")
	ErrProjectNotFound    = errors.New("project not found")
	ErrProjectMismatch    = errors.New("URL project does not match authenticated project scope")
)

func slugMappingCacheKey(tenantSlug, projectSlug string) string {
	return "slug_mapping:" + tenantSlug + ":" + projectSlug
}

// InvalidateProjectSlugMappings removes cached slug->ID mappings so renamed,
// deactivated, or deleted projects stop resolving immediately. Pass every slug
// that may be cached (previous and new). Failures are returned so callers can
// log them; correctness for cache hits is additionally protected by the live
// active-project check in GetTenantAndProjectFromSlugs.
func InvalidateProjectSlugMappings(ctx context.Context, tenantSlug string, projectSlugs ...string) error {
	if tenantSlug == "" || len(projectSlugs) == 0 || configs.RedisClient == nil {
		return nil
	}
	keys := make([]string, 0, len(projectSlugs))
	for _, slug := range projectSlugs {
		if slug != "" {
			keys = append(keys, slugMappingCacheKey(tenantSlug, slug))
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return configs.RedisClient.Del(ctx, keys...).Err()
}

// verifyCachedProjectActive ensures a cached mapping never outlives project
// deactivation or deletion; stale entries are evicted. Lookup errors fail closed.
func verifyCachedProjectActive(ctx context.Context, cacheKey, tenantID, projectID string) error {
	active, err := projectActiveChecker(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	if !active {
		_ = configs.RedisClient.Del(ctx, cacheKey).Err()
		return ErrProjectNotFound
	}
	return nil
}

// projectActiveChecker verifies against the database that a project is still active.
// It is a variable so tests can run without MongoDB.
var projectActiveChecker = func(ctx context.Context, tenantID, projectID string) (bool, error) {
	tenantOID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return false, nil
	}
	projectOID, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return false, nil
	}
	count, err := configs.GetCollection("projects").CountDocuments(ctx, bson.M{
		"_id": projectOID, "tenantId": tenantOID, "isActive": true,
	})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

type resolvedProjectContextKey struct{}

type resolvedProjectContext struct {
	tenantSlug, projectSlug string
	tenantID, projectID     string
}

// resolveProjectFromSlugs reuses a successful URL resolution only within this
// request. Authentication locals are deliberately not a verification marker.
func resolveProjectFromSlugs(c *fiber.Ctx, tenantSlug, projectSlug string) (tenantID, projectID string, err error) {
	if resolved, ok := c.Locals(resolvedProjectContextKey{}).(resolvedProjectContext); ok &&
		resolved.tenantSlug == tenantSlug && resolved.projectSlug == projectSlug {
		return resolved.tenantID, resolved.projectID, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to get from Redis cache first
	cacheKey := slugMappingCacheKey(tenantSlug, projectSlug)
	if configs.RedisCircuitAllow() {
		cachedValue, err := configs.RedisClient.Get(ctx, cacheKey).Result()
		configs.RedisCircuitRecordResult(err)
		if err == nil && cachedValue != "" {
			// Cache hit: parse cached value (format: "tenantID|projectID")
			// Split and return
			parts := splitCachedValue(cachedValue)
			if len(parts) == 2 {
				tenantID = parts[0]
				projectID = parts[1]

				if activeErr := verifyCachedProjectActive(ctx, cacheKey, tenantID, projectID); activeErr != nil {
					return "", "", activeErr
				}

				return tenantID, projectID, nil
			}
		}
	}

	// Cache miss: Query database
	projectsCollection := globalCollectionProvider("projects")
	var project struct {
		ID       primitive.ObjectID `bson:"_id"`
		TenantID primitive.ObjectID `bson:"tenantId"`
		IsActive bool               `bson:"isActive"`
	}
	err = projectsCollection.FindOne(ctx, bson.M{
		"tenantSlug": tenantSlug,
		"slug":       projectSlug,
		"isActive":   true,
	}).Decode(&project)
	if err != nil {
		return "", "", ErrProjectNotFound
	}

	projectID = project.ID.Hex()
	tenantID = project.TenantID.Hex()

	// Cache the result for 1 hour (3600 seconds)
	cacheValue := tenantID + "|" + projectID
	redisClient := configs.RedisClient
	go func() {
		if !configs.RedisCircuitAllow() {
			return
		}
		// Cache in background to not block the request
		if err := redisClient.Set(context.Background(), cacheKey, cacheValue, 3600*time.Second).Err(); err != nil {
			configs.RedisCircuitRecordResult(err)
			// Log error but don't fail the request
			return
		}
		configs.RedisCircuitRecordSuccess()
	}()

	return tenantID, projectID, nil
}

func setResolvedProjectContext(c *fiber.Ctx, tenantSlug, projectSlug, tenantID, projectID string) {
	// Fiber parameter strings borrow the URI buffer, which can change on reroute.
	c.Locals(resolvedProjectContextKey{}, resolvedProjectContext{
		tenantSlug: strings.Clone(tenantSlug), projectSlug: strings.Clone(projectSlug),
		tenantID: tenantID, projectID: projectID,
	})
	c.Locals("tenantID", tenantID)
	c.Locals("projectID", projectID)
	c.Locals("tenantSlug", tenantSlug)
	c.Locals("projectSlug", projectSlug)
}

// GetTenantAndProjectFromSlugs extracts tenant and project IDs from URL slugs.
// Callers remain responsible for checking their authenticated project scope.
func GetTenantAndProjectFromSlugs(c *fiber.Ctx) (tenantID, projectID string, err error) {
	tenantSlug, projectSlug := c.Params("tenantSlug"), c.Params("projectSlug")
	if tenantSlug == "" {
		return "", "", ErrTenantSlugMissing
	}
	if projectSlug == "" {
		return "", "", ErrProjectSlugMissing
	}
	tenantID, projectID, err = resolveProjectFromSlugs(c, tenantSlug, projectSlug)
	if err != nil {
		return "", "", err
	}
	setResolvedProjectContext(c, tenantSlug, projectSlug, tenantID, projectID)
	return tenantID, projectID, nil
}

// splitCachedValue splits the cached value format "tenantID|projectID"
func splitCachedValue(value string) []string {
	parts := make([]string, 0, 2)
	start := 0
	for i := 0; i < len(value); i++ {
		if value[i] == '|' {
			parts = append(parts, value[start:i])
			start = i + 1
		}
	}
	if start < len(value) {
		parts = append(parts, value[start:])
	}
	return parts
}

// GetTenantAndProjectContext extracts tenant/project from URL slugs or falls back to query/locals
// This provides backward compatibility during migration
// SECURITY: If JWT token has project scope, validates that URL matches the token's project
func GetTenantAndProjectContext(c *fiber.Ctx) (tenantID, projectID string, err error) {
	// SECURITY: Check if user has JWT token with project scope FIRST
	tokenProjectID, hasTokenProject := c.Locals("projectID").(string)
	tokenTenantID, hasTokenTenant := c.Locals("tenantID").(string)

	// Try URL slugs first (new pattern)
	tenantSlug := c.Params("tenantSlug")
	projectSlug := c.Params("projectSlug")

	if tenantSlug != "" && projectSlug != "" {
		// Resolve the URL independently of auth locals, then validate the current
		// scope on every call, including after a later authentication middleware.
		urlTenantID, urlProjectID, resolveErr := resolveProjectFromSlugs(c, tenantSlug, projectSlug)
		if resolveErr != nil {
			return "", "", resolveErr
		}

		// SECURITY CHECK: URL tenant must always match authenticated tenant context.
		if hasTokenTenant && tokenTenantID != "" && urlTenantID != tokenTenantID {
			return "", "", ErrProjectMismatch
		}
		// Project-scoped tokens must also match the exact project.
		if hasTokenProject && tokenProjectID != "" && urlProjectID != tokenProjectID {
			return "", "", ErrProjectMismatch
		}

		// Only NOW update locals after validation passes
		setResolvedProjectContext(c, tenantSlug, projectSlug, urlTenantID, urlProjectID)

		return urlTenantID, urlProjectID, nil
	}

	// Fall back to query params or locals (old pattern for backward compatibility)
	tenantID = c.Query("tenantID")
	projectID = c.Query("projectID")
	if tenantID != "" && hasTokenTenant && tokenTenantID != "" && tenantID != tokenTenantID {
		return "", "", ErrProjectMismatch
	}
	if projectID != "" && hasTokenProject && tokenProjectID != "" && projectID != tokenProjectID {
		return "", "", ErrProjectMismatch
	}

	if tenantID == "" {
		tenantID = tokenTenantID
	}
	if projectID == "" {
		projectID = tokenProjectID
	}

	// Store in locals if not already there
	if tenantID != "" {
		c.Locals("tenantID", tenantID)
	}
	if projectID != "" {
		c.Locals("projectID", projectID)
	}

	return tenantID, projectID, nil
}

// OverrideProjectActiveChecker replaces the live active-project lookup used on
// slug-cache hits and returns a restore function. Intended for tests that run
// without MongoDB.
func OverrideProjectActiveChecker(checker func(ctx context.Context, tenantID, projectID string) (bool, error)) (restore func()) {
	previous := projectActiveChecker
	projectActiveChecker = checker
	return func() { projectActiveChecker = previous }
}
