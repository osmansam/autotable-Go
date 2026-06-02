package events

import (
	"testing"
	"time"

	"github.com/osmansam/autotableGo/ws"
)

func TestNewDynamicEvents(t *testing.T) {
	if NewDynamicEvents() == nil {
		t.Fatal("NewDynamicEvents() = nil")
	}
}

func TestEmitInvalidate(t *testing.T) {
	events := NewDynamicEvents()
	events.EmitInvalidate("orders", "user", "tenant", "project", "event")
	select {
	case got := <-ws.Broadcast:
		if got.Type != "invalidate" || got.Schema != "orders" || got.EventID != "event" || got.UserId != "user" || got.TenantID != "tenant" || got.ProjectID != "project" {
			t.Fatalf("Broadcast event = %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broadcast event")
	}
}
