package utils

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/osmansam/autotableGo/configs"
)

// Acquire Redis lock
func AcquireLock(lockKey string, expiration time.Duration) (string, bool) {
	ctx := context.Background()
	if !configs.RedisCircuitAllow() {
		return "", true
	}

	uniqueID := uuid.NewString() // Generate unique lock identifier

	success, err := configs.RedisClient.SetNX(ctx, lockKey, uniqueID, expiration).Result()
	configs.RedisCircuitRecordResult(err)
	if err != nil {
		return "", true
	}
	return uniqueID, success
}

// Release Redis lock only if the current process owns it
func ReleaseLock(lockKey, uniqueID string) {
	if uniqueID == "" || !configs.RedisCircuitAllow() {
		return
	}

	luaScript := `
        if redis.call("GET", KEYS[1]) == ARGV[1] then
            return redis.call("DEL", KEYS[1])
        else
            return 0
        end
    `
	err := configs.RedisClient.Eval(context.Background(), luaScript, []string{lockKey}, uniqueID).Err()
	configs.RedisCircuitRecordResult(err)
}
