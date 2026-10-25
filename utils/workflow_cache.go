package utils

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/osmansam/autotableGo/configs"
)

func projectWorkflowCacheVersionKey(tenantID, projectID string) string {
	return fmt.Sprintf("tenant:%s:project:%s:workflow-cache:revision", tenantID, projectID)
}

// GetProjectWorkflowCacheVersion separates results after a write to any schema
// in this project. Revision keys do not expire with the cached responses.
func GetProjectWorkflowCacheVersion(ctx context.Context, tenantID, projectID string) (int64, error) {
	if configs.RedisClient == nil || !configs.RedisCircuitAllow() {
		return 0, configs.ErrRedisCircuitOpen
	}
	key := projectWorkflowCacheVersionKey(tenantID, projectID)
	version, err := configs.RedisClient.Get(ctx, key).Int64()
	configs.RedisCircuitRecordResult(err)
	if err == nil {
		return version, nil
	}
	if err != redis.Nil {
		return 0, err
	}
	created, err := configs.RedisClient.SetNX(ctx, key, int64(1), 0).Result()
	configs.RedisCircuitRecordResult(err)
	if err != nil {
		return 0, err
	}
	if created {
		return 1, nil
	}
	version, err = configs.RedisClient.Get(ctx, key).Int64()
	configs.RedisCircuitRecordResult(err)
	return version, err
}

func InvalidateProjectWorkflowCache(ctx context.Context, tenantID, projectID string) error {
	if deferProjectWorkflowInvalidation(ctx, tenantID, projectID) {
		return nil
	}
	return invalidateProjectWorkflowCacheNow(ctx, tenantID, projectID)
}

func invalidateProjectWorkflowCacheNow(ctx context.Context, tenantID, projectID string) error {
	if tenantID == "" || projectID == "" || configs.RedisClient == nil || !configs.RedisCircuitAllow() {
		return nil
	}
	err := configs.RedisClient.Incr(ctx, projectWorkflowCacheVersionKey(tenantID, projectID)).Err()
	configs.RedisCircuitRecordResult(err)
	return err
}

// InvalidateWorkflowResultsAfterWrite preserves a committed write's successful
// response when Redis is unavailable; TTL remains the fallback freshness bound.
func InvalidateWorkflowResultsAfterWrite(ctx context.Context, tenantID, projectID string) {
	// A successful write still needs invalidation if its HTTP request expires.
	bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := InvalidateProjectWorkflowCache(bounded, tenantID, projectID); err != nil {
		log.Printf("Workflow cache invalidation failed tenant=%s project=%s: %v", tenantID, projectID, err)
	}
}

type workflowInvalidationBatchKey struct{}
type workflowInvalidationBatch struct {
	sync.Mutex
	projects map[[2]string]struct{}
}

// BeginWorkflowCacheInvalidation defers revision changes until the surrounding
// transaction commits. Schema-cache behavior remains unchanged. A read-only or
// rolled-back transaction never advances the workflow revision.
func BeginWorkflowCacheInvalidation(ctx context.Context) (context.Context, func(bool)) {
	if batch, _ := ctx.Value(workflowInvalidationBatchKey{}).(*workflowInvalidationBatch); batch != nil {
		return ctx, func(bool) {}
	}
	batch := &workflowInvalidationBatch{projects: map[[2]string]struct{}{}}
	return context.WithValue(ctx, workflowInvalidationBatchKey{}, batch), func(committed bool) {
		if !committed {
			return
		}
		batch.Lock()
		projects := make([][2]string, 0, len(batch.projects))
		for project := range batch.projects {
			projects = append(projects, project)
		}
		batch.Unlock()
		bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		for _, project := range projects {
			if err := invalidateProjectWorkflowCacheNow(bounded, project[0], project[1]); err != nil {
				log.Printf("Post-commit workflow cache invalidation failed tenant=%s project=%s: %v", project[0], project[1], err)
			}
		}
	}
}

func deferProjectWorkflowInvalidation(ctx context.Context, tenantID, projectID string) bool {
	batch, _ := ctx.Value(workflowInvalidationBatchKey{}).(*workflowInvalidationBatch)
	if batch == nil || tenantID == "" || projectID == "" {
		return false
	}
	batch.Lock()
	batch.projects[[2]string{tenantID, projectID}] = struct{}{}
	batch.Unlock()
	return true
}
