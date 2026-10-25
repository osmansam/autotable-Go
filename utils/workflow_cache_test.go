package utils

import (
	"context"
	"testing"

	"github.com/osmansam/autotableGo/configs"
)

func TestSchemaInvalidationAdvancesProjectWorkflowRevision(t *testing.T) {
	setupMiniRedis(t)
	ctx := context.Background()
	key := "tenant:tenant:project:project:workflow-cache:revision"
	other := "tenant:tenant:project:other:workflow-cache:revision"
	if err := configs.RedisClient.Set(ctx, key, 7, 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := configs.RedisClient.Set(ctx, other, 20, 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := IncrementSchemaCacheVersion(ctx, "tenant", "project", "visualBlocks"); err != nil {
		t.Fatal(err)
	}
	if got, err := configs.RedisClient.Get(ctx, key).Int64(); err != nil || got != 8 {
		t.Fatalf("project workflow revision = %d, %v; want 8 after another schema changes", got, err)
	}
	if got, _ := configs.RedisClient.Get(ctx, other).Int64(); got != 20 {
		t.Fatalf("unrelated project revision = %d", got)
	}
}

func TestWorkflowCacheInvalidationSurvivesCanceledWriteContext(t *testing.T) {
	setupMiniRedis(t)
	key := "tenant:tenant:project:project:workflow-cache:revision"
	if err := configs.RedisClient.Set(context.Background(), key, 7, 0).Err(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	InvalidateWorkflowResultsAfterWrite(ctx, "tenant", "project")
	if got, _ := configs.RedisClient.Get(context.Background(), key).Int64(); got != 8 {
		t.Fatalf("committed write's revision=%d; want 8 even after cancellation", got)
	}
}
