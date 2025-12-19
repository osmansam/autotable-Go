package utils

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// EnsureIndexes creates or updates indexes for a given schema based on the container configuration
func EnsureIndexes(ctx context.Context, container *models.ContainerModel, tenantID, projectID string) error {
	if container == nil {
		return fmt.Errorf("container model is nil")
	}

	// Get project-specific collection
	collection := GetDynamicCollectionForProject(tenantID, projectID, container.SchemaName)
	if collection == nil {
		return fmt.Errorf("failed to get collection for schema: %s", container.SchemaName)
	}

	// Create indexes from Field.Unique (automatic unique indexes)
	if err := createUniqueFieldIndexes(ctx, collection, container); err != nil {
		log.Printf("Warning: Failed to create unique field indexes for %s: %v", container.SchemaName, err)
		// Don't return error, continue with other indexes
	}

	// Create indexes from container.Indexes (custom performance indexes)
	if err := createCustomIndexes(ctx, collection, container); err != nil {
		log.Printf("Warning: Failed to create custom indexes for %s: %v", container.SchemaName, err)
		// Don't return error, continue
	}

	log.Printf("Successfully ensured indexes for schema: %s", container.SchemaName)
	return nil
}

// createUniqueFieldIndexes creates unique indexes for fields marked as Unique
func createUniqueFieldIndexes(ctx context.Context, collection *mongo.Collection, container *models.ContainerModel) error {
	for _, field := range container.Fields {
		if field.Unique {
			indexModel := mongo.IndexModel{
				Keys: bson.D{{Key: field.Name, Value: 1}}, // Ascending order
				Options: options.Index().
					SetUnique(true).
					SetName(fmt.Sprintf("idx_%s_unique", field.Name)).
					SetBackground(true),
			}

			indexName, err := collection.Indexes().CreateOne(ctx, indexModel)
			if err != nil {
				// Check if error is because index already exists
				if !mongo.IsDuplicateKeyError(err) && !isIndexExistsError(err) {
					log.Printf("Failed to create unique index for field %s.%s: %v", container.SchemaName, field.Name, err)
					continue
				}
				log.Printf("Index already exists for field %s.%s", container.SchemaName, field.Name)
			} else {
				log.Printf("Created unique index '%s' for field %s.%s", indexName, container.SchemaName, field.Name)
			}
		}
	}
	return nil
}

// createCustomIndexes creates indexes defined in container.Indexes
func createCustomIndexes(ctx context.Context, collection *mongo.Collection, container *models.ContainerModel) error {
	if len(container.Indexes) == 0 {
		return nil
	}

	for _, idx := range container.Indexes {
		if len(idx.Fields) == 0 {
			log.Printf("Skipping index %s: no fields defined", idx.Name)
			continue
		}

		// Build index keys
		keys := bson.D{}
		for _, field := range idx.Fields {
			order := field.Order
			if order != 1 && order != -1 {
				order = 1 // Default to ascending
			}
			keys = append(keys, bson.E{Key: field.FieldName, Value: order})
		}

		// Build index options
		indexOpts := options.Index().SetName(idx.Name)

		if idx.Unique {
			indexOpts.SetUnique(true)
		}
		if idx.Sparse {
			indexOpts.SetSparse(true)
		}
		if idx.TTL > 0 {
			indexOpts.SetExpireAfterSeconds(int32(idx.TTL))
		}
		if idx.Background {
			indexOpts.SetBackground(true)
		}

		indexModel := mongo.IndexModel{
			Keys:    keys,
			Options: indexOpts,
		}

		indexName, err := collection.Indexes().CreateOne(ctx, indexModel)
		if err != nil {
			if !isIndexExistsError(err) {
				log.Printf("Failed to create index %s for schema %s: %v", idx.Name, container.SchemaName, err)
				continue
			}
			log.Printf("Index %s already exists for schema %s", idx.Name, container.SchemaName)
		} else {
			log.Printf("Created index '%s' for schema %s", indexName, container.SchemaName)
		}
	}

	return nil
}

// DropIndexes drops all indexes for a schema (useful when deleting a container)
// collectionName should be the full project-specific collection name
func DropIndexes(ctx context.Context, collectionName string) error {
	collection := configs.GetCollection(collectionName)
	if collection == nil {
		return fmt.Errorf("failed to get collection: %s", collectionName)
	}

	// Drop all indexes except _id (which cannot be dropped)
	_, err := collection.Indexes().DropAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to drop indexes for collection %s: %w", collectionName, err)
	}

	log.Printf("Dropped all indexes for collection: %s", collectionName)
	return nil
}

// ListIndexes returns all indexes for a given schema
func ListIndexes(ctx context.Context, schemaName string) ([]bson.M, error) {
	collection := configs.GetCollection(schemaName)
	if collection == nil {
		return nil, fmt.Errorf("failed to get collection for schema: %s", schemaName)
	}

	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list indexes for schema %s: %w", schemaName, err)
	}
	defer cursor.Close(ctx)

	var indexes []bson.M
	if err = cursor.All(ctx, &indexes); err != nil {
		return nil, fmt.Errorf("failed to decode indexes for schema %s: %w", schemaName, err)
	}

	return indexes, nil
}

// RebuildIndexes drops and recreates all indexes for a schema
func RebuildIndexes(ctx context.Context, container *models.ContainerModel, tenantID, projectID string) error {
	if container == nil {
		return fmt.Errorf("container model is nil")
	}

	// Drop existing indexes (use full collection name)
	collectionName := GetProjectCollectionName(tenantID, projectID, container.SchemaName)
	if err := DropIndexes(ctx, collectionName); err != nil {
		log.Printf("Warning: Failed to drop indexes for %s: %v", collectionName, err)
	}

	// Wait a bit for indexes to be fully dropped
	time.Sleep(100 * time.Millisecond)

	// Recreate indexes
	return EnsureIndexes(ctx, container, tenantID, projectID)
}

// isIndexExistsError checks if the error is due to index already existing
func isIndexExistsError(err error) bool {
	if err == nil {
		return false
	}
	// MongoDB returns error code 85 or 86 for index already exists
	// Also check for error message containing "already exists"
	errMsg := err.Error()
	return mongo.IsDuplicateKeyError(err) ||
		containsAny(errMsg, []string{
			"already exists",
			"IndexOptionsConflict",
			"Index with name",
			"index already exists",
		})
}

// containsAny checks if a string contains any of the given substrings
func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}
