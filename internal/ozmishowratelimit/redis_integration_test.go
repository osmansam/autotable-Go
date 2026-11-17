package ozmishowratelimit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

// This fixture cannot select or reach an existing Redis. It starts its own
// child with TCP disabled, a unique temporary Unix socket and a random password.
func disposableRedis(t *testing.T) *redis.Client {
	t.Helper()
	binary, err := exec.LookPath("redis-server")
	if err != nil {
		t.Skip("redis-server not installed; native integration fixture unavailable")
	}
	dir, err := os.MkdirTemp("/tmp", "ozmi-redis-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "r.sock")
	secretBytes := make([]byte, 32)
	if _, err = rand.Read(secretBytes); err != nil {
		t.Fatal(err)
	}
	password := hex.EncodeToString(secretBytes)
	cmd := exec.Command(binary, "--port", "0", "--unixsocket", socket, "--unixsocketperm", "700", "--requirepass", password, "--save", "", "--appendonly", "no", "--maxmemory", "64mb", "--maxmemory-policy", "noeviction", "--dir", dir, "--logfile", filepath.Join(dir, "redis.log"))
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: socket, Password: password, MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	t.Cleanup(func() {
		_ = client.Close()
		_ = cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("test Redis process did not exit")
		}
	})
	for i := 0; i < 100; i++ {
		if client.Ping(context.Background()).Err() == nil {
			return client
		}
		select {
		case err := <-done:
			t.Fatalf("disposable Redis exited: %v", err)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("disposable Redis did not become ready")
	return nil
}

func TestDisposableRedisIsolationAndOOM(t *testing.T) {
	first := disposableRedis(t)
	original := disposableRedis(t)
	options := *first.Options()
	second := redis.NewClient(&options)
	defer second.Close()
	cfg := Config{Secret: testSecret, Namespace: "real-test", Policies: DefaultPolicies(), MaxConcurrent: 128}
	appA, appB := testApp(t, cfg, first), testApp(t, cfg, second)
	var accepted, denied atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 80; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			app := appA
			if i%2 == 1 {
				app = appB
			}
			status, result := call(t, app, body("login"), testSecret)
			if status == 200 && result.Allowed {
				accepted.Add(1)
			} else if status == 429 && !result.Allowed {
				denied.Add(1)
			} else {
				t.Errorf("concurrent Redis response: %d %+v", status, result)
			}
		}(i)
	}
	wg.Wait()
	if accepted.Load() != 10 || denied.Load() != 70 {
		t.Fatalf("atomic native Redis budgets: accepted=%d denied=%d", accepted.Load(), denied.Load())
	}

	if status, _ := call(t, appB, body("login"), testSecret); status != 429 {
		t.Fatal(status)
	}
	// These commands can only touch the child processes created above.
	if err := original.FlushAll(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	if status, _ := call(t, appA, body("login"), testSecret); status != 429 {
		t.Fatal("AutoTable flush affected limiter", status)
	}
	if err := first.ConfigSet(context.Background(), "maxmemory", "1").Err(); err != nil {
		t.Fatal(err)
	}
	if status, result := call(t, appB, body("admin"), testSecret); status != 503 || result.Allowed {
		t.Fatalf("OOM must fail closed: %d %+v", status, result)
	}
	t.Log("Independent clients share budgets; separate Redis flush is isolated; real noeviction OOM returns 503")
}
