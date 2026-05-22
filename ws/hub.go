// ws/hub.go
package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/websocket/v2"
	"github.com/osmansam/autotableGo/configs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Event struct {
	Type      string `json:"type"`             // "invalidate", "pageChanged", "containerChanged"
	Schema    string `json:"schema"`           // schema name
	UserId    string `json:"userId,omitempty"` // user who triggered the event
	TenantID  string `json:"-"`                // tenant ID (not sent to client, used for routing)
	ProjectID string `json:"-"`                // project ID (not sent to client, used for routing)
	Timestamp int64  `json:"ts"`
}

type redisEventEnvelope struct {
	Origin    string `json:"origin"`
	Type      string `json:"type"`
	Schema    string `json:"schema"`
	UserId    string `json:"userId,omitempty"`
	TenantID  string `json:"tenantId"`
	ProjectID string `json:"projectId"`
	Timestamp int64  `json:"ts"`
}

// ClientInfo stores metadata about each WebSocket client
type ClientInfo struct {
	Conn      *websocket.Conn
	TenantID  string
	ProjectID string
	Send      chan []byte
}

var (
	clients        = make(map[*websocket.Conn]*ClientInfo)
	clientsMu      sync.RWMutex
	Broadcast      = make(chan Event, 128)
	redisWSChan    = "websocket:events"
	instanceID     = newInstanceID()
	clientSendSize = 64
	writeWait      = 10 * time.Second
	pubSubBackoff  = 1 * time.Second
	pubSubMaxDelay = 30 * time.Second
)

type staleClient struct {
	conn *websocket.Conn
	info *ClientInfo
}

// RunBroadcaster keeps sending events to connected clients in the same tenant/project.
func RunBroadcaster() {
	for ev := range Broadcast {
		payload, _ := json.Marshal(ev)
		var staleClients []staleClient

		clientsMu.RLock()
		matchCount := 0
		queuedCount := 0
		for conn, info := range clients {
			// Only send to clients in the same tenant and project
			if info.TenantID == ev.TenantID && info.ProjectID == ev.ProjectID {
				matchCount++
				select {
				case info.Send <- payload:
					queuedCount++
				default:
					staleClients = append(staleClients, staleClient{conn: conn, info: info})
				}
			}
		}
		clientsMu.RUnlock()

		for _, stale := range staleClients {
			unregisterClient(stale.conn, stale.info, "send buffer full")
		}

		if matchCount > 0 {
			log.Printf("WebSocket: Broadcast queued to %d/%d client(s) - type: %s, schema: %s, tenantID: %s, projectID: %s",
				queuedCount, matchCount, ev.Type, ev.Schema, ev.TenantID, ev.ProjectID)
		}
	}
}

func writePump(info *ClientInfo) {
	for payload := range info.Send {
		_ = info.Conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := info.Conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			unregisterClient(info.Conn, info, "write failed")
			return
		}
	}
}

func unregisterClient(conn *websocket.Conn, info *ClientInfo, reason string) {
	clientsMu.Lock()
	if current, ok := clients[conn]; !ok || current != info {
		clientsMu.Unlock()
		return
	}

	delete(clients, conn)
	remaining := len(clients)
	close(info.Send)
	clientsMu.Unlock()

	log.Printf("WebSocket: Client disconnected - tenantID: %s, projectID: %s, reason: %s (remaining: %d)", info.TenantID, info.ProjectID, reason, remaining)
	_ = conn.Close()
}

// RunRedisSubscriber relays websocket events published by other app instances to local clients.
func RunRedisSubscriber() {
	backoff := pubSubBackoff

	for {
		if configs.RedisClient == nil {
			time.Sleep(backoff)
			backoff = nextBackoff(backoff)
			continue
		}

		ctx := context.Background()
		pubsub := configs.RedisClient.Subscribe(ctx, redisWSChan)
		if _, err := pubsub.Receive(ctx); err != nil {
			configs.RedisCircuitRecordResult(err)
			_ = pubsub.Close()
			log.Printf("WebSocket: Redis pub/sub subscribe failed: %v", err)
			time.Sleep(backoff)
			backoff = nextBackoff(backoff)
			continue
		}

		configs.RedisCircuitRecordSuccess()
		backoff = pubSubBackoff
		log.Printf("WebSocket: Redis pub/sub subscribed to channel %q", redisWSChan)

		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				configs.RedisCircuitRecordResult(err)
				_ = pubsub.Close()
				log.Printf("WebSocket: Redis pub/sub receive failed: %v", err)
				time.Sleep(backoff)
				backoff = nextBackoff(backoff)
				break
			}

			var envelope redisEventEnvelope
			if err := json.Unmarshal([]byte(msg.Payload), &envelope); err != nil {
				log.Printf("WebSocket: Redis pub/sub invalid payload: %v", err)
				continue
			}
			if envelope.Origin == instanceID {
				continue
			}

			enqueueBroadcast(envelope.toEvent(), "redis pub/sub")
		}
	}
}

func publishEvent(ev Event) {
	enqueueBroadcast(ev, "local publish")

	if configs.RedisClient == nil {
		return
	}
	if !configs.RedisCircuitAllow() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	payload, err := json.Marshal(newRedisEventEnvelope(ev))
	if err != nil {
		log.Printf("WebSocket: Failed to marshal Redis pub/sub event: %v", err)
		return
	}

	err = configs.RedisClient.Publish(ctx, redisWSChan, payload).Err()
	configs.RedisCircuitRecordResult(err)
	if err != nil {
		log.Printf("WebSocket: Failed to publish Redis pub/sub event: %v", err)
	}
}

func enqueueBroadcast(ev Event, source string) {
	select {
	case Broadcast <- ev:
	default:
		log.Printf("WebSocket: Dropped broadcast event from %s because queue is full - type: %s, schema: %s, tenantID: %s, projectID: %s",
			source, ev.Type, ev.Schema, ev.TenantID, ev.ProjectID)
	}
}

func newRedisEventEnvelope(ev Event) redisEventEnvelope {
	return redisEventEnvelope{
		Origin:    instanceID,
		Type:      ev.Type,
		Schema:    ev.Schema,
		UserId:    ev.UserId,
		TenantID:  ev.TenantID,
		ProjectID: ev.ProjectID,
		Timestamp: ev.Timestamp,
	}
}

func (e redisEventEnvelope) toEvent() Event {
	return Event{
		Type:      e.Type,
		Schema:    e.Schema,
		UserId:    e.UserId,
		TenantID:  e.TenantID,
		ProjectID: e.ProjectID,
		Timestamp: e.Timestamp,
	}
}

func newInstanceID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown-host"
	}
	return fmt.Sprintf("%s:%d:%d", hostname, os.Getpid(), time.Now().UnixNano())
}

func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > pubSubMaxDelay {
		return pubSubMaxDelay
	}
	return next
}

// resolveTenantAndProjectIDs converts slugs to actual database IDs
// First checks Redis cache, then falls back to database lookup
func resolveTenantAndProjectIDs(tenantSlug, projectSlug string) (tenantID, projectID string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try Redis cache first
	cacheKey := "slug_mapping:" + tenantSlug + ":" + projectSlug
	if configs.RedisClient != nil && configs.RedisCircuitAllow() {
		cachedValue, err := configs.RedisClient.Get(ctx, cacheKey).Result()
		configs.RedisCircuitRecordResult(err)
		if err == nil && cachedValue != "" {
			parts := strings.Split(cachedValue, "|")
			if len(parts) == 2 {
				return parts[0], parts[1], nil
			}
		}
	}

	// Not in cache, query database
	tenantColl := configs.GetCollection("tenants")
	projectColl := configs.GetCollection("projects")

	// Find tenant by slug
	var tenantResult bson.M
	err = tenantColl.FindOne(ctx, bson.M{"slug": tenantSlug}).Decode(&tenantResult)
	if err != nil {
		log.Printf("WebSocket: Tenant not found for slug '%s': %v", tenantSlug, err)
		return "", "", err
	}

	// Extract tenant ID as hex string
	tenantObjID, ok := tenantResult["_id"].(primitive.ObjectID)
	if !ok {
		return "", "", fmt.Errorf("invalid tenant _id")
	}
	tenantID = tenantObjID.Hex()

	// Find project by slug and tenant
	var projectResult bson.M
	err = projectColl.FindOne(ctx, bson.M{"slug": projectSlug, "tenantId": tenantID}).Decode(&projectResult)
	if err != nil {
		log.Printf("WebSocket: Project not found for slug '%s': %v", projectSlug, err)
		return "", "", err
	}

	// Extract project ID as hex string
	projectObjID, ok := projectResult["_id"].(primitive.ObjectID)
	if !ok {
		return "", "", fmt.Errorf("invalid project _id")
	}
	projectID = projectObjID.Hex()

	log.Printf("WebSocket: Resolved slugs - tenant '%s' -> %s, project '%s' -> %s",
		tenantSlug, tenantID, projectSlug, projectID)

	// Cache the result for future use (24 hours)
	if configs.RedisClient != nil && configs.RedisCircuitAllow() {
		err := configs.RedisClient.Set(ctx, cacheKey, tenantID+"|"+projectID, 24*time.Hour).Err()
		configs.RedisCircuitRecordResult(err)
	}

	return tenantID, projectID, nil
}

// HandleWS adds clients and keeps them alive.
// Expects tenantSlug and projectSlug as query parameters: /ws?tenantSlug=xxx&projectSlug=yyy
// Also accepts tenantId and projectId for backward compatibility
func HandleWS(c *websocket.Conn) {
	// Extract tenant and project slugs from query parameters
	// Try new parameter names first, fall back to old names
	tenantSlug := c.Query("tenantSlug")
	if tenantSlug == "" {
		tenantSlug = c.Query("tenantId") // backward compatibility
	}

	projectSlug := c.Query("projectSlug")
	if projectSlug == "" {
		projectSlug = c.Query("projectId") // backward compatibility
	}

	// If not provided, try to get from headers (for backward compatibility)
	if tenantSlug == "" {
		tenantSlug = c.Headers("X-Tenant-Slug")
		if tenantSlug == "" {
			tenantSlug = c.Headers("X-Tenant-Id")
		}
	}
	if projectSlug == "" {
		projectSlug = c.Headers("X-Project-Slug")
		if projectSlug == "" {
			projectSlug = c.Headers("X-Project-Id")
		}
	}

	// Require both slugs for multi-tenancy isolation
	if tenantSlug == "" || projectSlug == "" {
		log.Printf("WebSocket: Connection rejected - missing tenant or project slug")
		c.WriteMessage(websocket.TextMessage, []byte(`{"error":"tenantSlug and projectSlug required (or tenantId/projectId)"}`))
		c.Close()
		return
	}

	// Resolve slugs to actual database IDs
	tenantID, projectID, err := resolveTenantAndProjectIDs(tenantSlug, projectSlug)
	if err != nil {
		log.Printf("WebSocket: Failed to resolve tenant/project IDs from slugs '%s/%s': %v", tenantSlug, projectSlug, err)
		c.WriteMessage(websocket.TextMessage, []byte(`{"error":"Invalid tenant or project"}`))
		c.Close()
		return
	}

	log.Printf("WebSocket: Client connected - tenantSlug: %s (%s), projectSlug: %s (%s)",
		tenantSlug, tenantID, projectSlug, projectID)

	// Register client with tenant/project context
	clientInfo := &ClientInfo{
		Conn:      c,
		TenantID:  tenantID,
		ProjectID: projectID,
		Send:      make(chan []byte, clientSendSize),
	}

	clientsMu.Lock()
	clients[c] = clientInfo
	totalClients := len(clients)
	clientsMu.Unlock()

	log.Printf("WebSocket: Total connected clients: %d", totalClients)

	defer func() {
		unregisterClient(c, clientInfo, "read closed")
	}()

	go writePump(clientInfo)

	// Keep connection alive and handle any incoming messages
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			return
		}
	}
}

// EmitInvalidate pushes an invalidate event to clients in the same tenant/project.
func EmitInvalidate(schema string, userId string, tenantID string, projectID string) {
	log.Printf("WebSocket: Emitting invalidate event - schema: %s, userId: %s, tenantID: %s, projectID: %s", schema, userId, tenantID, projectID)

	publishEvent(Event{
		Type:      "invalidate",
		Schema:    schema,
		UserId:    userId,
		TenantID:  tenantID,
		ProjectID: projectID,
		Timestamp: time.Now().Unix(),
	})
}

// EmitPageChanged pushes a pageChanged event to clients in the same tenant/project.
func EmitPageChanged(userId string, tenantID string, projectID string) {
	publishEvent(Event{
		Type:      "pageChanged",
		Schema:    "pages",
		UserId:    userId,
		TenantID:  tenantID,
		ProjectID: projectID,
		Timestamp: time.Now().Unix(),
	})
}

// EmitContainerChanged pushes a containerChanged event to clients in the same tenant/project.
func EmitContainerChanged(userId string, tenantID string, projectID string) {
	publishEvent(Event{
		Type:      "containerChanged",
		Schema:    "containers",
		UserId:    userId,
		TenantID:  tenantID,
		ProjectID: projectID,
		Timestamp: time.Now().Unix(),
	})
}
