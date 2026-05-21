package utils

import (
	"context"
	"errors"
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

// GetTenantAndProjectFromSlugs extracts tenant and project IDs from URL slugs
// URL pattern: /api/:tenantSlug/:projectSlug/...
// Optimized: Checks Redis cache first, falls back to DB query if not cached
func GetTenantAndProjectFromSlugs(c *fiber.Ctx) (tenantID, projectID string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get slugs from URL params
	tenantSlug := c.Params("tenantSlug")
	projectSlug := c.Params("projectSlug")

	if tenantSlug == "" {
		return "", "", ErrTenantSlugMissing
	}
	if projectSlug == "" {
		return "", "", ErrProjectSlugMissing
	}

	// Try to get from Redis cache first
	cacheKey := "slug_mapping:" + tenantSlug + ":" + projectSlug
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

				// Store in locals for downstream use
				c.Locals("tenantID", tenantID)
				c.Locals("projectID", projectID)
				c.Locals("tenantSlug", tenantSlug)
				c.Locals("projectSlug", projectSlug)

				return tenantID, projectID, nil
			}
		}
	}

	// Cache miss: Query database
	projectsCollection := configs.GetCollection("projects")
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
	go func() {
		if !configs.RedisCircuitAllow() {
			return
		}
		// Cache in background to not block the request
		if err := configs.RedisClient.Set(context.Background(), cacheKey, cacheValue, 3600*time.Second).Err(); err != nil {
			configs.RedisCircuitRecordResult(err)
			// Log error but don't fail the request
			return
		}
		configs.RedisCircuitRecordSuccess()
	}()

	// Store in locals for downstream use
	c.Locals("tenantID", tenantID)
	c.Locals("projectID", projectID)
	c.Locals("tenantSlug", tenantSlug)
	c.Locals("projectSlug", projectSlug)

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
		// Get IDs from URL slugs (but DON'T overwrite locals yet)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Try cache first
		cacheKey := "slug_mapping:" + tenantSlug + ":" + projectSlug
		var urlTenantID, urlProjectID string

		if configs.RedisCircuitAllow() {
			cachedValue, cacheErr := configs.RedisClient.Get(ctx, cacheKey).Result()
			configs.RedisCircuitRecordResult(cacheErr)
			if cacheErr == nil && cachedValue != "" {
				parts := splitCachedValue(cachedValue)
				if len(parts) == 2 {
					urlTenantID = parts[0]
					urlProjectID = parts[1]
				}
			}
		}

		// If not in cache, query database
		if urlProjectID == "" {
			projectsCollection := configs.GetCollection("projects")
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

			urlProjectID = project.ID.Hex()
			urlTenantID = project.TenantID.Hex()

			// Cache the result
			cacheValue := urlTenantID + "|" + urlProjectID
			go func() {
				if !configs.RedisCircuitAllow() {
					return
				}
				err := configs.RedisClient.Set(context.Background(), cacheKey, cacheValue, 3600*time.Second).Err()
				configs.RedisCircuitRecordResult(err)
			}()
		}

		// SECURITY CHECK: If user has project-scoped JWT token, URL MUST match
		if hasTokenProject && tokenProjectID != "" {
			if urlProjectID != tokenProjectID {
				return "", "", ErrProjectMismatch
			}
			if hasTokenTenant && tokenTenantID != "" && urlTenantID != tokenTenantID {
				return "", "", ErrProjectMismatch
			}
		}

		// Only NOW update locals after validation passes
		c.Locals("tenantID", urlTenantID)
		c.Locals("projectID", urlProjectID)
		c.Locals("tenantSlug", tenantSlug)
		c.Locals("projectSlug", projectSlug)

		return urlTenantID, urlProjectID, nil
	}

	// Fall back to query params or locals (old pattern for backward compatibility)
	tenantID = c.Query("tenantID")
	projectID = c.Query("projectID")

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
