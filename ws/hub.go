// ws/hub.go
package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/websocket/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/observability"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Event struct {
	Type      string `json:"type"`              // "invalidate", "pageChanged", "containerChanged", "notificationChanged"
	Schema    string `json:"schema"`            // schema name
	EventID   string `json:"eventId,omitempty"` // stable idempotency key for outbox-backed events
	UserId    string `json:"userId,omitempty"`  // user who triggered the event
	TenantID  string `json:"-"`                 // tenant ID (not sent to client, used for routing)
	ProjectID string `json:"-"`                 // project ID (not sent to client, used for routing)
	Timestamp int64  `json:"ts"`
}

type redisEventEnvelope struct {
	Origin    string `json:"origin"`
	Type      string `json:"type"`
	Schema    string `json:"schema"`
	EventID   string `json:"eventId,omitempty"`
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
func RunBroadcaster(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-Broadcast:
			if !ok {
				return
			}
			broadcastEvent(ev)
		}
	}
}

func broadcastEvent(ev Event) {
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
		attrs := append(observability.TenantProjectAttrs(ev.TenantID, ev.ProjectID),
			slog.String(observability.FieldSchemaName, ev.Schema),
			slog.String(observability.FieldOperation, "websocket_broadcast"),
			slog.String("event_type", ev.Type),
			slog.Int("queued_clients", queuedCount),
			slog.Int("matched_clients", matchCount))
		observability.DebugCtx(context.Background(), "websocket broadcast queued", attrs...)
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

	observability.SetWebsocketClientsConnected(remaining)
	attrs := append(observability.TenantProjectAttrs(info.TenantID, info.ProjectID),
		slog.String(observability.FieldOperation, "websocket_disconnect"),
		slog.String("reason", reason),
		slog.Int("connected_clients", remaining))
	observability.InfoCtx(context.Background(), "websocket client disconnected", attrs...)
	_ = conn.Close()
}

// RunRedisSubscriber relays websocket events published by other app instances to local clients.
func RunRedisSubscriber(ctx context.Context) {
	backoff := pubSubBackoff

	for {
		if ctx.Err() != nil {
			return
		}
		if configs.RedisClient == nil {
			if !sleepWithContext(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}

		pubsub := configs.RedisClient.Subscribe(ctx, redisWSChan)
		if _, err := pubsub.Receive(ctx); err != nil {
			if ctx.Err() != nil {
				_ = pubsub.Close()
				return
			}
			configs.RedisCircuitRecordResult(err)
			_ = pubsub.Close()
			observability.ErrorCtx(ctx, "websocket redis pubsub subscribe failed", err,
				slog.String(observability.FieldOperation, "websocket_redis_subscribe"),
				slog.String(observability.FieldStatus, "error"))
			if !sleepWithContext(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}

		configs.RedisCircuitRecordSuccess()
		backoff = pubSubBackoff
		observability.InfoCtx(ctx, "websocket redis pubsub subscribed",
			slog.String(observability.FieldOperation, "websocket_redis_subscribe"),
			slog.String(observability.FieldStatus, "success"))

		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					_ = pubsub.Close()
					return
				}
				configs.RedisCircuitRecordResult(err)
				_ = pubsub.Close()
				observability.ErrorCtx(ctx, "websocket redis pubsub receive failed", err,
					slog.String(observability.FieldOperation, "websocket_redis_receive"),
					slog.String(observability.FieldStatus, "error"))
				if !sleepWithContext(ctx, backoff) {
					return
				}
				backoff = nextBackoff(backoff)
				break
			}

			var envelope redisEventEnvelope
			if err := json.Unmarshal([]byte(msg.Payload), &envelope); err != nil {
				observability.WarnCtx(ctx, "websocket redis pubsub invalid payload",
					slog.String(observability.FieldOperation, "websocket_redis_receive"),
					slog.String(observability.FieldStatus, "invalid_payload"),
					slog.String(observability.FieldError, err.Error()))
				continue
			}
			if envelope.Origin == instanceID {
				continue
			}

			enqueueBroadcast(envelope.toEvent(), "redis pub/sub")
		}
	}
}

func sleepWithContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
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
		observability.ErrorCtx(ctx, "websocket redis pubsub marshal failed", err,
			slog.String(observability.FieldOperation, "websocket_redis_publish"),
			slog.String(observability.FieldStatus, "error"))
		return
	}

	err = configs.RedisClient.Publish(ctx, redisWSChan, payload).Err()
	configs.RedisCircuitRecordResult(err)
	if err != nil {
		observability.ErrorCtx(ctx, "websocket redis pubsub publish failed", err,
			slog.String(observability.FieldOperation, "websocket_redis_publish"),
			slog.String(observability.FieldStatus, "error"))
	}
}

func enqueueBroadcast(ev Event, source string) {
	select {
	case Broadcast <- ev:
	default:
		attrs := append(observability.TenantProjectAttrs(ev.TenantID, ev.ProjectID),
			slog.String(observability.FieldSchemaName, ev.Schema),
			slog.String(observability.FieldOperation, "websocket_broadcast_enqueue"),
			slog.String(observability.FieldStatus, "dropped"),
			slog.String("source", source),
			slog.String("event_type", ev.Type))
		observability.WarnCtx(context.Background(), "websocket broadcast queue full", attrs...)
	}
}

func newRedisEventEnvelope(ev Event) redisEventEnvelope {
	return redisEventEnvelope{
		Origin:    instanceID,
		Type:      ev.Type,
		Schema:    ev.Schema,
		EventID:   ev.EventID,
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
		EventID:   e.EventID,
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
		observability.WarnCtx(ctx, "websocket tenant slug not found",
			slog.String(observability.FieldOperation, "websocket_resolve_context"),
			slog.String(observability.FieldStatus, "not_found"),
			slog.String(observability.FieldError, err.Error()))
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
	err = projectColl.FindOne(ctx, bson.M{"slug": projectSlug, "tenantId": tenantObjID}).Decode(&projectResult)
	if err != nil {
		observability.WarnCtx(ctx, "websocket project slug not found",
			slog.String(observability.FieldOperation, "websocket_resolve_context"),
			slog.String(observability.FieldStatus, "not_found"),
			slog.String(observability.FieldError, err.Error()))
		return "", "", err
	}

	// Extract project ID as hex string
	projectObjID, ok := projectResult["_id"].(primitive.ObjectID)
	if !ok {
		return "", "", fmt.Errorf("invalid project _id")
	}
	projectID = projectObjID.Hex()

	observability.DebugCtx(ctx, "websocket tenant/project slugs resolved",
		append(observability.TenantProjectAttrs(tenantID, projectID),
			slog.String(observability.FieldOperation, "websocket_resolve_context"),
			slog.String(observability.FieldStatus, "success"))...)

	// Cache the result for future use (24 hours)
	if configs.RedisClient != nil && configs.RedisCircuitAllow() {
		err := configs.RedisClient.Set(ctx, cacheKey, tenantID+"|"+projectID, 24*time.Hour).Err()
		configs.RedisCircuitRecordResult(err)
	}

	return tenantID, projectID, nil
}

func validateTenantAndProjectIDs(tenantID, projectID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tenantObjID, err := primitive.ObjectIDFromHex(strings.TrimSpace(tenantID))
	if err != nil {
		return fmt.Errorf("invalid tenantId")
	}
	projectObjID, err := primitive.ObjectIDFromHex(strings.TrimSpace(projectID))
	if err != nil {
		return fmt.Errorf("invalid projectId")
	}

	projectColl := configs.GetCollection("projects")
	if err := projectColl.FindOne(ctx, bson.M{"_id": projectObjID, "tenantId": tenantObjID}).Err(); err != nil {
		return err
	}
	return nil
}

func websocketContextAttrs(tenantSlug, projectSlug, tenantIDParam, projectIDParam string) []slog.Attr {
	attrs := make([]slog.Attr, 0, 4)
	attrs = appendTrimmedWSAttr(attrs, "tenant_slug", tenantSlug)
	attrs = appendTrimmedWSAttr(attrs, "project_slug", projectSlug)
	attrs = appendTrimmedWSAttr(attrs, "tenant_id_param", tenantIDParam)
	attrs = appendTrimmedWSAttr(attrs, "project_id_param", projectIDParam)
	return attrs
}

func appendTrimmedWSAttr(attrs []slog.Attr, key, value string) []slog.Attr {
	value = strings.TrimSpace(value)
	if value == "" {
		return attrs
	}
	return append(attrs, slog.String(key, strings.Clone(value)))
}

// HandleWS adds clients and keeps them alive.
// Expects tenantSlug and projectSlug as query parameters: /ws?tenantSlug=xxx&projectSlug=yyy
// Also accepts tenantId and projectId for backward compatibility
func HandleWS(c *websocket.Conn) {
	tenantSlug := c.Query("tenantSlug")
	projectSlug := c.Query("projectSlug")
	tenantIDParam := c.Query("tenantId")
	projectIDParam := c.Query("projectId")

	if tenantSlug == "" {
		tenantSlug = c.Headers("X-Tenant-Slug")
	}
	if projectSlug == "" {
		projectSlug = c.Headers("X-Project-Slug")
	}
	if tenantIDParam == "" {
		tenantIDParam = c.Headers("X-Tenant-Id")
	}
	if projectIDParam == "" {
		projectIDParam = c.Headers("X-Project-Id")
	}

	var tenantID, projectID string
	var err error
	if tenantSlug != "" && projectSlug != "" {
		tenantID, projectID, err = resolveTenantAndProjectIDs(tenantSlug, projectSlug)
	} else if tenantIDParam != "" && projectIDParam != "" {
		tenantID = strings.TrimSpace(tenantIDParam)
		projectID = strings.TrimSpace(projectIDParam)
		if looksLikeObjectID(tenantID) && looksLikeObjectID(projectID) {
			err = validateTenantAndProjectIDs(tenantID, projectID)
		} else {
			tenantID, projectID, err = resolveTenantAndProjectIDs(tenantID, projectID)
		}
	} else {
		attrs := []slog.Attr{
			slog.String(observability.FieldOperation, "websocket_connect"),
			slog.String(observability.FieldStatus, "missing_context"),
		}
		attrs = append(attrs, websocketContextAttrs(tenantSlug, projectSlug, tenantIDParam, projectIDParam)...)
		observability.WarnCtx(context.Background(), "websocket connection rejected", attrs...)
		c.WriteMessage(websocket.TextMessage, []byte(`{"error":"tenantSlug and projectSlug required (or tenantId/projectId)"}`))
		c.Close()
		return
	}

	if err != nil {
		attrs := []slog.Attr{
			slog.String(observability.FieldOperation, "websocket_connect"),
			slog.String(observability.FieldStatus, "invalid_context"),
			slog.String(observability.FieldError, err.Error()),
		}
		attrs = append(attrs, websocketContextAttrs(tenantSlug, projectSlug, tenantIDParam, projectIDParam)...)
		observability.WarnCtx(context.Background(), "websocket connection rejected", attrs...)
		c.WriteMessage(websocket.TextMessage, []byte(`{"error":"Invalid tenant or project"}`))
		c.Close()
		return
	}

	observability.InfoCtx(context.Background(), "websocket client connected",
		append(observability.TenantProjectAttrs(tenantID, projectID),
			slog.String(observability.FieldOperation, "websocket_connect"),
			slog.String(observability.FieldStatus, "success"))...)

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

	observability.SetWebsocketClientsConnected(totalClients)
	observability.DebugCtx(context.Background(), "websocket client count updated",
		slog.String(observability.FieldOperation, "websocket_client_count"),
		slog.Int("connected_clients", totalClients))

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

func looksLikeObjectID(value string) bool {
	_, err := primitive.ObjectIDFromHex(strings.TrimSpace(value))
	return err == nil
}

// EmitInvalidate pushes an invalidate event to clients in the same tenant/project.
func EmitInvalidate(schema string, userId string, tenantID string, projectID string, eventID ...string) {
	idempotencyKey := ""
	if len(eventID) > 0 {
		idempotencyKey = eventID[0]
	}
	observability.DebugCtx(context.Background(), "websocket invalidate event emitted",
		append(observability.TenantProjectAttrs(tenantID, projectID),
			slog.String(observability.FieldSchemaName, schema),
			slog.String(observability.FieldOperation, "websocket_emit_invalidate"))...)

	publishEvent(Event{
		Type:      "invalidate",
		Schema:    schema,
		EventID:   idempotencyKey,
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

// EmitNotificationChanged signals clients to refetch notification state.
func EmitNotificationChanged(userId string, tenantID string, projectID string) {
	publishEvent(Event{
		Type:      "notificationChanged",
		Schema:    "notifications",
		UserId:    userId,
		TenantID:  tenantID,
		ProjectID: projectID,
		Timestamp: time.Now().Unix(),
	})
}
