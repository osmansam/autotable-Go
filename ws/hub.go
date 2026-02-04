// ws/hub.go
package ws

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/websocket/v2"
	"github.com/osmansam/autotableGo/configs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Event struct {
	Type      string `json:"type"`   // "invalidate", "pageChanged", "containerChanged"
	Schema    string `json:"schema"` // schema name
	UserId    string `json:"userId,omitempty"` // user who triggered the event
	TenantID  string `json:"-"` // tenant ID (not sent to client, used for routing)
	ProjectID string `json:"-"` // project ID (not sent to client, used for routing)
	Timestamp int64  `json:"ts"`
}

// ClientInfo stores metadata about each WebSocket client
type ClientInfo struct {
	Conn      *websocket.Conn
	TenantID  string
	ProjectID string
}

var (
	clients   = make(map[*websocket.Conn]*ClientInfo)
	clientsMu sync.RWMutex
	Broadcast = make(chan Event, 128)
)

// RunBroadcaster keeps sending events to connected clients in the same tenant/project.
func RunBroadcaster() {
	for ev := range Broadcast {
		payload, _ := json.Marshal(ev)

		clientsMu.RLock()
		matchCount := 0
		for conn, info := range clients {
			// Only send to clients in the same tenant and project
			if info.TenantID == ev.TenantID && info.ProjectID == ev.ProjectID {
				matchCount++
				if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
					conn.Close()
					clientsMu.RUnlock()
					clientsMu.Lock()
					delete(clients, conn)
					clientsMu.Unlock()
					clientsMu.RLock()
				}
			}
		}
		clientsMu.RUnlock()
		
		if matchCount > 0 {
			log.Printf("WebSocket: Broadcast sent to %d client(s) - type: %s, schema: %s, tenantID: %s, projectID: %s", 
				matchCount, ev.Type, ev.Schema, ev.TenantID, ev.ProjectID)
		}
	}
}

// resolveTenantAndProjectIDs converts slugs to actual database IDs
// First checks Redis cache, then falls back to database lookup
func resolveTenantAndProjectIDs(tenantSlug, projectSlug string) (tenantID, projectID string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try Redis cache first
	cacheKey := "slug_mapping:" + tenantSlug + ":" + projectSlug
	cachedValue, err := configs.RedisClient.Get(ctx, cacheKey).Result()
	if err == nil && cachedValue != "" {
		parts := strings.Split(cachedValue, "|")
		if len(parts) == 2 {
			return parts[0], parts[1], nil
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
	tenantID = tenantResult["_id"].(primitive.ObjectID).Hex()

	// Find project by slug and tenant
	var projectResult bson.M
	err = projectColl.FindOne(ctx, bson.M{"slug": projectSlug, "tenantId": tenantID}).Decode(&projectResult)
	if err != nil {
		log.Printf("WebSocket: Project not found for slug '%s': %v", projectSlug, err)
		return "", "", err
	}
	
	// Extract project ID as hex string
	projectID = projectResult["_id"].(primitive.ObjectID).Hex()
	
	log.Printf("WebSocket: Resolved slugs - tenant '%s' -> %s, project '%s' -> %s", 
		tenantSlug, tenantID, projectSlug, projectID)

	// Cache the result for future use (24 hours)
	configs.RedisClient.Set(ctx, cacheKey, tenantID+"|"+projectID, 24*time.Hour)

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
	}

	clientsMu.Lock()
	clients[c] = clientInfo
	totalClients := len(clients)
	clientsMu.Unlock()
	
	log.Printf("WebSocket: Total connected clients: %d", totalClients)

	defer func() {
		clientsMu.Lock()
		delete(clients, c)
		remaining := len(clients)
		clientsMu.Unlock()
		log.Printf("WebSocket: Client disconnected - tenantID: %s, projectID: %s (remaining: %d)", tenantID, projectID, remaining)
		c.Close()
	}()

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
	
	Broadcast <- Event{
		Type:      "invalidate",
		Schema:    schema,
		UserId:    userId,
		TenantID:  tenantID,
		ProjectID: projectID,
		Timestamp: time.Now().Unix(),
	}
}

// EmitPageChanged pushes a pageChanged event to clients in the same tenant/project.
func EmitPageChanged(userId string, tenantID string, projectID string) {
	Broadcast <- Event{
		Type:      "pageChanged",
		Schema:    "pages",
		UserId:    userId,
		TenantID:  tenantID,
		ProjectID: projectID,
		Timestamp: time.Now().Unix(),
	}
}

// EmitContainerChanged pushes a containerChanged event to clients in the same tenant/project.
func EmitContainerChanged(userId string, tenantID string, projectID string) {
	Broadcast <- Event{
		Type:      "containerChanged",
		Schema:    "containers",
		UserId:    userId,
		TenantID:  tenantID,
		ProjectID: projectID,
		Timestamp: time.Now().Unix(),
	}
}
