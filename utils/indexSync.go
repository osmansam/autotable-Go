package utils

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// SyncContainerIndexes builds desired indexes before removing retired definitions.
// Conflicting definitions return an error; existing constraints are never dropped
// to make a conflicting index build succeed. Unmanaged database indexes are kept.
func SyncContainerIndexes(ctx context.Context, existing, updated *models.ContainerModel, tenantID, projectID string) error {
	if updated == nil {
		return fmt.Errorf("container model is nil")
	}
	collection := dynamicCollectionProvider(tenantID, projectID, updated.SchemaName)
	if collection == nil {
		return fmt.Errorf("missing collection for schema %s", updated.SchemaName)
	}
	before := map[string]bool{}
	cursor, err := collection.Indexes().List(ctx)
	if err != nil && !isNamespaceNotFoundError(err) {
		return err
	}
	if err == nil {
		var indexes []bson.M
		if err := cursor.All(ctx, &indexes); err != nil {
			return err
		}
		for _, index := range indexes {
			if name, ok := index["name"].(string); ok {
				before[name] = true
			}
		}
	}
	if err := EnsureIndexes(ctx, updated, tenantID, projectID); err != nil {
		// A build can partially succeed. Remove only attempted names absent
		// before this operation, leaving all pre-existing constraints intact.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		for _, name := range containerIndexNames(updated) {
			if before[name] || name == "_id_" {
				continue
			}
			if _, dropErr := collection.Indexes().DropOne(cleanupCtx, name); dropErr != nil {
				var commandErr mongo.CommandError
				if isNamespaceNotFoundError(dropErr) || errors.As(dropErr, &commandErr) && commandErr.Code == 27 {
					continue
				}
				err = errors.Join(err, fmt.Errorf("failed to roll back new index %s: %w", name, dropErr))
			}
		}
		return err
	}
	// A renamed schema addresses another collection. Never remove the source's indexes.
	if existing == nil || existing.SchemaName != updated.SchemaName {
		return nil
	}
	desired := containerIndexNames(updated)
	for _, name := range containerIndexNames(existing) {
		keep := false
		for _, current := range desired {
			if current == name {
				keep = true
				break
			}
		}
		if keep || name == "_id_" {
			continue
		}
		if _, err := collection.Indexes().DropOne(ctx, name); err != nil {
			var commandErr mongo.CommandError
			if errors.As(err, &commandErr) && commandErr.Code == 27 {
				continue
			}
			return fmt.Errorf("failed to remove retired index %s: %w", name, err)
		}
	}
	return nil
}

func containerIndexNames(container *models.ContainerModel) []string {
	var names []string
	seen := map[string]bool{}
	add := func(name string) {
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	for _, field := range container.Fields {
		if field.Unique {
			add("idx_" + field.Name + "_unique")
		}
	}
	for _, index := range container.Indexes {
		if len(index.Fields) == 0 {
			continue
		}
		name := index.Name
		if name == "" {
			var parts []string
			for _, field := range index.Fields {
				order := field.Order
				if order != 1 && order != -1 {
					order = 1
				}
				parts = append(parts, field.FieldName, strconv.Itoa(order))
			}
			name = strings.Join(parts, "_")
		}
		add(name)
	}
	return names
}
