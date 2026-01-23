// ws/hub.go
package ws

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gofiber/websocket/v2"
)

type Event struct {
	Type      string `json:"type"`   // "invalidate", "pageChanged", "containerChanged"
	Schema    string `json:"schema"` // schema name
	UserId    string `json:"userId,omitempty"` // user who triggered the event
	Timestamp int64  `json:"ts"`
}

var (
	clients   = make(map[*websocket.Conn]struct{})
	clientsMu sync.RWMutex
	Broadcast = make(chan Event, 128) // <-- declare it here
)

// RunBroadcaster keeps sending events to all connected clients.
func RunBroadcaster() {
	for ev := range Broadcast {
		payload, _ := json.Marshal(ev)

		clientsMu.RLock()
		for c := range clients {
			if err := c.WriteMessage(websocket.TextMessage, payload); err != nil {
				c.Close()
				clientsMu.RUnlock()
				clientsMu.Lock()
				delete(clients, c)
				clientsMu.Unlock()
				clientsMu.RLock()
			}
		}
		clientsMu.RUnlock()
	}
}

// HandleWS adds clients and keeps them alive.
func HandleWS(c *websocket.Conn) {
	clientsMu.Lock()
	clients[c] = struct{}{}
	clientsMu.Unlock()

	defer func() {
		clientsMu.Lock()
		delete(clients, c)
		clientsMu.Unlock()
		c.Close()
	}()

	for {
		if _, _, err := c.ReadMessage(); err != nil {
			return
		}
	}
}

// EmitInvalidate pushes an invalidate event to all clients.
func EmitInvalidate(schema string, userId string) {
	Broadcast <- Event{
		Type:      "invalidate",
		Schema:    schema,
		UserId:    userId,
		Timestamp: time.Now().Unix(),
	}
}

// EmitPageChanged pushes a pageChanged event to all clients.
func EmitPageChanged(userId string) {
	Broadcast <- Event{
		Type:      "pageChanged",
		Schema:    "pages",
		UserId:    userId,
		Timestamp: time.Now().Unix(),
	}
}

// EmitContainerChanged pushes a containerChanged event to all clients.
func EmitContainerChanged(userId string) {
	Broadcast <- Event{
		Type:      "containerChanged",
		Schema:    "containers",
		UserId:    userId,
		Timestamp: time.Now().Unix(),
	}
}
