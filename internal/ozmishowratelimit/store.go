package ozmishowratelimit

import (
	"context"
	"errors"
	"github.com/go-redis/redis/v8"
)

// Constant script: callers cannot supply code, limits, expiry, or Redis keys.
// Check before incrementing to keep exhausted counters bounded under abuse.
const consumeScript = `local n = redis.call('GET', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
if n then
 if ttl < 0 or not tonumber(n) then return redis.error_reply('invalid rate limit state') end
 if tonumber(n) >= tonumber(ARGV[2]) then return {0, ttl} end
end
local count = redis.call('INCR', KEYS[1])
if count == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return {1, redis.call('PTTL', KEYS[1])}`

func consume(ctx context.Context, client redis.Cmdable, key string, policy Policy) (Result, error) {
	value, err := client.Eval(ctx, consumeScript, []string{key}, policy.Window.Milliseconds(), policy.Max).Result()
	if err != nil {
		return Result{}, err
	}
	values, ok := value.([]interface{})
	if !ok || len(values) != 2 {
		return Result{}, errors.New("invalid budget response")
	}
	allowed, ok1 := values[0].(int64)
	ttl, ok2 := values[1].(int64)
	if !ok1 || !ok2 || (allowed != 0 && allowed != 1) || ttl < 0 || ttl > policy.Window.Milliseconds() {
		return Result{}, errors.New("invalid budget response")
	}
	result := Result{Allowed: allowed == 1}
	if !result.Allowed {
		result.RetryAfterSeconds = (ttl + 999) / 1000
		if result.RetryAfterSeconds < 1 {
			result.RetryAfterSeconds = 1
		}
	}
	return result, nil
}
