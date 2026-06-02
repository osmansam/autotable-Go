package ws

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/osmansam/autotableGo/configs"
)

func TestRedisEventEnvelopeRoundTrip(t *testing.T) {
	event := Event{
		Type:      "invalidate",
		Schema:    "orders",
		EventID:   "event",
		UserId:    "user",
		TenantID:  "tenant",
		ProjectID: "project",
		Timestamp: time.Now().Unix(),
	}
	envelope := newRedisEventEnvelope(event)
	if envelope.Origin != instanceID {
		t.Fatalf("Origin = %q, want %q", envelope.Origin, instanceID)
	}
	if got := envelope.toEvent(); !reflect.DeepEqual(got, event) {
		t.Fatalf("toEvent() = %#v, want %#v", got, event)
	}
}

func TestNextBackoff(t *testing.T) {
	tests := []struct {
		current time.Duration
		want    time.Duration
	}{
		{current: time.Second, want: 2 * time.Second},
		{current: pubSubMaxDelay, want: pubSubMaxDelay},
	}
	for _, tt := range tests {
		if got := nextBackoff(tt.current); got != tt.want {
			t.Fatalf("nextBackoff(%s) = %s, want %s", tt.current, got, tt.want)
		}
	}
}

func TestNewInstanceID(t *testing.T) {
	if got := newInstanceID(); got == "" || !strings.Contains(got, ":") {
		t.Fatalf("newInstanceID() = %q", got)
	}
}

func TestEmitEvents(t *testing.T) {
	tests := []struct {
		name       string
		emit       func()
		wantType   string
		wantSchema string
	}{
		{name: "invalidate", emit: func() { EmitInvalidate("orders", "user", "tenant", "project", "event") }, wantType: "invalidate", wantSchema: "orders"},
		{name: "page changed", emit: func() { EmitPageChanged("user", "tenant", "project") }, wantType: "pageChanged", wantSchema: "pages"},
		{name: "container changed", emit: func() { EmitContainerChanged("user", "tenant", "project") }, wantType: "containerChanged", wantSchema: "containers"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.emit()
			select {
			case got := <-Broadcast:
				if got.Type != tt.wantType || got.Schema != tt.wantSchema || got.TenantID != "tenant" || got.ProjectID != "project" {
					t.Fatalf("Broadcast event = %#v", got)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for broadcast event")
			}
		})
	}
}

func TestResolveTenantAndProjectIDsFromCache(t *testing.T) {
	server := miniredis.RunT(t)
	oldClient := configs.RedisClient
	configs.RedisClient = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = configs.RedisClient.Close()
		configs.RedisClient = oldClient
	})
	if err := configs.RedisClient.Set(context.Background(), "slug_mapping:tenant:project", "tenant-id|project-id", time.Minute).Err(); err != nil {
		t.Fatalf("Redis Set() error = %v", err)
	}
	tenantID, projectID, err := resolveTenantAndProjectIDs("tenant", "project")
	if err != nil || tenantID != "tenant-id" || projectID != "project-id" {
		t.Fatalf("resolveTenantAndProjectIDs() = %q, %q, %v", tenantID, projectID, err)
	}
}

func TestEnqueueBroadcastDropsWhenQueueIsFull(t *testing.T) {
	oldBroadcast := Broadcast
	Broadcast = make(chan Event, 1)
	t.Cleanup(func() { Broadcast = oldBroadcast })

	first := Event{Type: "first"}
	enqueueBroadcast(first, "test")
	enqueueBroadcast(Event{Type: "dropped"}, "test")
	if got := <-Broadcast; !reflect.DeepEqual(got, first) {
		t.Fatalf("Broadcast event = %#v, want %#v", got, first)
	}
	select {
	case got := <-Broadcast:
		t.Fatalf("unexpected queued event = %#v", got)
	default:
	}
}

func TestRunBroadcasterWithoutClients(t *testing.T) {
	oldBroadcast := Broadcast
	Broadcast = make(chan Event, 1)
	t.Cleanup(func() { Broadcast = oldBroadcast })

	Broadcast <- Event{Type: "invalidate", TenantID: "tenant", ProjectID: "project"}
	close(Broadcast)
	RunBroadcaster()
}
