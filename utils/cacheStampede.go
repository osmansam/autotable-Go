package utils

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/osmansam/autotableGo/configs"
)

func BuildCacheFillLockKey(cacheKey string) string {
	return "lock:cache-fill:" + HashCacheQuery(cacheKey)
}

func AcquireCacheFillLock(ctx context.Context, cacheKey string) (string, bool) {
	lockID := uuid.NewString()
	locked, err := configs.RedisClient.SetNX(ctx, BuildCacheFillLockKey(cacheKey), lockID, configs.GetCacheFillLockTTL()).Result()
	if err != nil {
		return "", false
	}
	return lockID, locked
}

func ReleaseCacheFillLock(ctx context.Context, cacheKey, lockID string) {
	luaScript := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`
	_ = configs.RedisClient.Eval(ctx, luaScript, []string{BuildCacheFillLockKey(cacheKey)}, lockID).Err()
}

func WaitForCacheFill(ctx context.Context, checkCache func() bool) bool {
	deadline := time.NewTimer(configs.GetCacheFillWaitTimeout())
	defer deadline.Stop()

	ticker := time.NewTicker(configs.GetCacheFillPollInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return checkCache()
		case <-ticker.C:
			if checkCache() {
				return true
			}
		}
	}
}
