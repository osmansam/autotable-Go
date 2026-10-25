package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestWorkflowCacheInvalidationFollowsTransactionCommit(t *testing.T) {
	for _, mode := range []string{"commit", "rollback", "panic", "commit-after-cancel", "read-only"} {
		t.Run(mode, func(t *testing.T) {
			server := miniredis.RunT(t)
			previous := configs.RedisClient
			configs.RedisClient = redis.NewClient(&redis.Options{Addr: server.Addr()})
			t.Cleanup(func() { _ = configs.RedisClient.Close(); configs.RedisClient = previous })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			key := "tenant:tenant:project:project:workflow-cache:revision"
			if err := configs.RedisClient.Set(ctx, key, 7, 0).Err(); err != nil {
				t.Fatal(err)
			}
			repository := &DynamicRepository{withTransaction: func(tx context.Context, fn func(mongo.SessionContext) error) error {
				err := fn(mongo.NewSessionContext(tx, nil))
				if got, _ := configs.RedisClient.Get(context.Background(), key).Int64(); got != 7 {
					t.Errorf("revision advanced to %d before commit; concurrent readers could cache uncommitted state", got)
				}
				if mode == "commit-after-cancel" {
					cancel()
				}
				return err
			}}
			sentinel := errors.New("rollback")
			var err error
			run := func() {
				err = repository.WithTransaction(ctx, func(tx mongo.SessionContext) error {
					if mode != "read-only" {
						if err := utils.IncrementSchemaCacheVersion(tx, "tenant", "project", "visualBlocks"); err != nil {
							return err
						}
					}
					if mode == "rollback" {
						return sentinel
					}
					if mode == "panic" {
						panic(sentinel)
					}
					return nil
				})
			}
			if mode == "panic" {
				func() {
					defer func() {
						if recovered := recover(); recovered != sentinel {
							t.Errorf("unexpected panic: %v", recovered)
						}
					}()
					run()
				}()
			} else {
				run()
			}
			if mode == "rollback" && !errors.Is(err, sentinel) {
				t.Fatalf("rollback error=%v", err)
			}
			if mode != "rollback" && err != nil {
				t.Fatal(err)
			}
			want := int64(8)
			if mode == "rollback" || mode == "panic" || mode == "read-only" {
				want = 7
			}
			if got, _ := configs.RedisClient.Get(context.Background(), key).Int64(); got != want {
				t.Fatalf("revision after %s=%d, want %d", mode, got, want)
			}
		})
	}
}
