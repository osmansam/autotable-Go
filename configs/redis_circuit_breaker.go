package configs

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

var ErrRedisCircuitOpen = errors.New("redis circuit breaker is open")

var redisCircuit = struct {
	sync.Mutex
	failures         int
	openedUntil      time.Time
	halfOpenInFlight bool
}{}

func RedisCircuitAllow() bool {
	if !GetRedisCircuitBreakerEnabled() {
		return true
	}

	redisCircuit.Lock()
	defer redisCircuit.Unlock()

	now := time.Now()
	if redisCircuit.openedUntil.IsZero() {
		return true
	}
	if now.Before(redisCircuit.openedUntil) {
		return false
	}
	if redisCircuit.halfOpenInFlight {
		return false
	}

	redisCircuit.halfOpenInFlight = true
	return true
}

func RedisGetString(ctx context.Context, key string) (string, error) {
	if !RedisCircuitAllow() {
		return "", ErrRedisCircuitOpen
	}

	value, err := RedisClient.Get(ctx, key).Result()
	RedisCircuitRecordResult(err)
	return value, err
}

func RedisSetValue(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if !RedisCircuitAllow() {
		return nil
	}

	err := RedisClient.Set(ctx, key, value, ttl).Err()
	RedisCircuitRecordResult(err)
	return err
}

func RedisDelKeys(ctx context.Context, keys ...string) error {
	if len(keys) == 0 || !RedisCircuitAllow() {
		return nil
	}

	err := RedisClient.Del(ctx, keys...).Err()
	RedisCircuitRecordResult(err)
	return err
}

func RedisFlushAll(ctx context.Context) error {
	if !RedisCircuitAllow() {
		return ErrRedisCircuitOpen
	}

	err := RedisClient.FlushAll(ctx).Err()
	RedisCircuitRecordResult(err)
	return err
}

func RedisCircuitRecordResult(err error) {
	if err == nil || err == redis.Nil {
		RedisCircuitRecordSuccess()
		return
	}
	RedisCircuitRecordFailure(err)
}

func RedisCircuitRecordSuccess() {
	if !GetRedisCircuitBreakerEnabled() {
		return
	}

	redisCircuit.Lock()
	defer redisCircuit.Unlock()

	wasOpen := !redisCircuit.openedUntil.IsZero()
	redisCircuit.failures = 0
	redisCircuit.openedUntil = time.Time{}
	redisCircuit.halfOpenInFlight = false
	if wasOpen {
		log.Println("Redis circuit breaker closed after successful probe")
	}
}

func RedisCircuitRecordFailure(err error) {
	if !GetRedisCircuitBreakerEnabled() {
		return
	}

	redisCircuit.Lock()
	defer redisCircuit.Unlock()

	redisCircuit.halfOpenInFlight = false
	redisCircuit.failures++
	if redisCircuit.failures < GetRedisCircuitFailureThreshold() && redisCircuit.openedUntil.IsZero() {
		return
	}

	redisCircuit.failures = 0
	redisCircuit.openedUntil = time.Now().Add(GetRedisCircuitOpenDuration())
	log.Printf("Redis circuit breaker opened for %s after Redis error: %v", GetRedisCircuitOpenDuration(), err)
}

func GetRedisCircuitBreakerEnabled() bool {
	return GetAppConfig().Redis.CircuitBreaker.Enabled
}

func GetRedisCircuitFailureThreshold() int {
	threshold := GetAppConfig().Redis.CircuitBreaker.FailureThreshold
	if threshold < 1 {
		return RedisCircuitFailureThreshold
	}
	return threshold
}

func GetRedisCircuitOpenDuration() time.Duration {
	seconds := GetAppConfig().Redis.CircuitBreaker.OpenDurationSeconds
	if seconds < 1 {
		seconds = RedisCircuitOpenDurationSeconds
	}
	return time.Duration(seconds) * time.Second
}
