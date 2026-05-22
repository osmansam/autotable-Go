# WebSocket Multi-Tenancy Implementation

## Overview

The WebSocket implementation now supports multi-tenancy, ensuring that events are only broadcast to users within the same tenant and project. This prevents data leakage between different tenants.

## How It Works

### Server-Side Changes

1. **Connection Isolation**: Each WebSocket connection is now associated with a `tenantID` and `projectID`
2. **Event Routing**: Events are only sent to clients that belong to the same tenant/project combination
3. **Client Metadata**: Server stores client metadata including tenant and project IDs
4. **Redis Pub/Sub Fan-Out**: Each app instance publishes websocket events to Redis channel `websocket:events`; other instances subscribe and deliver matching events to their local clients
5. **Buffered Client Sends**: Broadcasts enqueue messages into each client's buffered send channel; slow clients are disconnected if their buffer fills

### Client-Side Connection

To connect to the WebSocket, clients must provide their `tenantId` and `projectId` as query parameters:

```javascript
// Example: Connecting to WebSocket with tenant/project context
const tenantId = "your-tenant-id";
const projectId = "your-project-id";

const ws = new WebSocket(
  `ws://localhost:3000/ws?tenantId=${tenantId}&projectId=${projectId}`,
);

ws.onopen = () => {
  console.log("WebSocket connected");
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log("Received event:", data);

  // Handle different event types
  switch (data.type) {
    case "invalidate":
      // Refresh data for the specified schema
      console.log(`Invalidate schema: ${data.schema}`);
      break;
    case "pageChanged":
      // Refresh page list
      console.log("Pages changed");
      break;
    case "containerChanged":
      // Refresh container list
      console.log("Containers changed");
      break;
  }
};

ws.onerror = (error) => {
  console.error("WebSocket error:", error);
};

ws.onclose = () => {
  console.log("WebSocket disconnected");
};
```

### Alternative: Using Headers

If query parameters are not feasible, you can also pass the tenant and project slugs via headers (when establishing the WebSocket connection):

```javascript
const ws = new WebSocket("ws://localhost:3000/ws", {
  headers: {
    "X-Tenant-Slug": tenantSlug,
    "X-Project-Slug": projectSlug,
  },
});
```

**Note**: Not all WebSocket clients support custom headers. Query parameters are the recommended approach.

## Event Types

### 1. Invalidate Event

Sent when data in a specific schema is created, updated, or deleted.

```json
{
  "type": "invalidate",
  "schema": "users",
  "userId": "user-id-who-triggered",
  "ts": 1234567890
}
```

### 2. Page Changed Event

Sent when pages are created, updated, or deleted.

```json
{
  "type": "pageChanged",
  "schema": "pages",
  "userId": "user-id-who-triggered",
  "ts": 1234567890
}
```

### 3. Container Changed Event

Sent when containers (schemas) are created, updated, or deleted.

```json
{
  "type": "containerChanged",
  "schema": "containers",
  "userId": "user-id-who-triggered",
  "ts": 1234567890
}
```

## React Example

```jsx
import { useEffect, useState } from "react";

function useWebSocket(tenantSlug, projectSlug) {
  const [ws, setWs] = useState(null);
  const [lastEvent, setLastEvent] = useState(null);

  useEffect(() => {
    if (!tenantSlug || !projectSlug) return;

    const websocket = new WebSocket(
      `ws://localhost:3000/ws?tenantSlug=${tenantSlug}&projectSlug=${projectSlug}`,
    );

    websocket.onmessage = (event) => {
      const data = JSON.parse(event.data);
      setLastEvent(data);
    };

    websocket.onerror = (error) => {
      console.error("WebSocket error:", error);
    };

    websocket.onclose = () => {
      console.log("WebSocket closed, attempting to reconnect...");
      // Implement reconnection logic here
    };

    setWs(websocket);

    return () => {
      websocket.close();
    };
  }, [tenantSlug, projectSlug]);

  return { ws, lastEvent };
}

// Usage in a component
function MyComponent({ tenantSlug, projectSlug }) {
  const { lastEvent } = useWebSocket(tenantSlug, projectSlug);

  useEffect(() => {
    if (lastEvent) {
      console.log("Received event:", lastEvent);
      // Refresh your data based on the event type
      if (lastEvent.type === "invalidate") {
        // Refetch data for the affected schema
        refetchData(lastEvent.schema);
      }
    }
  }, [lastEvent]);

  return <div>Your component</div>;
}
```

## Security Considerations

1. **Authentication**: The WebSocket endpoint accepts tenant and project slugs from query parameters and resolves them to database IDs. The slugs are validated against the database to ensure they exist.
2. **Authorization**: Consider adding middleware to verify that the user has access to the specified tenant/project before accepting the WebSocket connection.
3. **Caching**: Slug-to-ID mappings are cached in Redis for 24 hours to improve performance.

## Future Enhancements

1. Add JWT token validation in the WebSocket connection handler
2. Implement automatic reconnection on the client side
3. Add heartbeat/ping-pong to detect stale connections
4. Consider using rooms/channels for more granular event broadcasting
