package utils

import (
	"context"
	"encoding/json"

	"github.com/go-redis/redis/v8"
)

// TryFetchFromCache attempts to fetch the cached data for a given key from Redis.
// If data is found, it returns the data and a boolean indicating a successful fetch.
func TryFetchFromCache(ctx context.Context, rdb *redis.Client, key string) ([]map[string]interface{}, bool, error) {
    cachedData, err := rdb.Get(ctx, key).Result()
    if err != nil {
        return nil, false, err
    }

    var items []map[string]interface{}
    if err := json.Unmarshal([]byte(cachedData), &items); err != nil {
        return nil, false, err
    }

    return items, true, nil
}
