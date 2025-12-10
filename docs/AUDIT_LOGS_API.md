# Audit Logs API Documentation

## Overview

The Audit Logs API allows you to retrieve audit trail information with advanced filtering, sorting, and pagination capabilities.

## Endpoint

```
GET /api/v1/audit-logs/
```

## Authentication & Authorization

This endpoint requires authentication and authorization.

### Authentication

Include a valid JWT token in the request headers.

### Authorization

By default (if not configured in settings), any authenticated user can access audit logs. To restrict access to specific roles, configure the authorization in the `settings` collection.

#### Database Configuration

The audit logs authorization is stored in the `settings` collection with the key `"audit_logs"`:

```json
{
  "key": "audit_logs",
  "auditLogs": {
    "isAuthorized": true,
    "authorizeRole": ["admin", "auditor"]
  },
  "createdAt": "2024-12-09T10:00:00Z",
  "updatedAt": "2024-12-09T10:00:00Z"
}
```

#### Configuration Fields

- `key` (string): Must be `"audit_logs"` for the system to recognize this setting
- `auditLogs.isAuthorized` (boolean): When `true`, role-based authorization is enforced
- `auditLogs.authorizeRole` (array): List of roles that are allowed to access audit logs

#### Examples:

**Allow only admin role (default):**

```json
{
  "key": "audit_logs",
  "auditLogs": {
    "isAuthorized": true,
    "authorizeRole": ["admin"]
  }
}
```

**Allow multiple roles:**

```json
{
  "key": "audit_logs",
  "auditLogs": {
    "isAuthorized": true,
    "authorizeRole": ["admin", "auditor", "security_officer"]
  }
}
```

**Disable authorization (any authenticated user):**

```json
{
  "key": "audit_logs",
  "auditLogs": {
    "isAuthorized": false,
    "authorizeRole": []
  }
}
```

#### Managing from Frontend

You can create API endpoints to manage the settings collection, allowing administrators to:

1. View current audit logs authorization settings
2. Update the `isAuthorized` flag
3. Add or remove roles from `authorizeRole`

**Note:** The configuration is loaded once when the server starts. To apply changes, restart the server or implement a dynamic reload mechanism.

## Query Parameters

### Filtering Parameters

| Parameter    | Type   | Description                                 | Example                                                                                      |
| ------------ | ------ | ------------------------------------------- | -------------------------------------------------------------------------------------------- |
| `action`     | string | Filter by action type                       | `create`, `update`, `delete`, `login`, `logout`, `bulk_create`, `bulk_update`, `bulk_delete` |
| `schemaName` | string | Filter by schema name                       | `users`, `products`, `orders`                                                                |
| `userEmail`  | string | Filter by user email                        | `user@example.com`                                                                           |
| `userId`     | string | Filter by user ID (ObjectID hex string)     | `507f1f77bcf86cd799439011`                                                                   |
| `documentId` | string | Filter by document ID (ObjectID hex string) | `507f1f77bcf86cd799439012`                                                                   |
| `startDate`  | string | Filter by start date (RFC3339 format)       | `2024-01-01T00:00:00Z`                                                                       |
| `endDate`    | string | Filter by end date (RFC3339 format)         | `2024-12-31T23:59:59Z`                                                                       |
| `ip`         | string | Filter by IP address                        | `192.168.1.1`                                                                                |
| `role`       | string | Filter by user role                         | `admin`, `user`, `editor`                                                                    |

### Sorting Parameters

| Parameter | Type    | Default     | Description                                 |
| --------- | ------- | ----------- | ------------------------------------------- |
| `sort`    | string  | `timestamp` | Field to sort by                            |
| `asc`     | boolean | `false`     | Sort ascending (true) or descending (false) |

### Pagination Parameters

| Parameter | Type    | Required | Description               |
| --------- | ------- | -------- | ------------------------- |
| `page`    | integer | Yes\*    | Page number (starts at 1) |
| `limit`   | integer | Yes\*    | Number of items per page  |

\*Both `page` and `limit` must be provided together to enable pagination.

## Response Format

### Without Pagination

When pagination parameters are not provided, the response is a simple array of audit log items:

```json
[
  {
    "_id": "507f1f77bcf86cd799439011",
    "timestamp": "2024-12-09T10:30:00Z",
    "userId": "507f1f77bcf86cd799439010",
    "userEmail": "user@example.com",
    "roles": ["admin"],
    "schemaName": "products",
    "documentIds": ["507f1f77bcf86cd799439012"],
    "action": "update",
    "description": "Updated product details",
    "before": {...},
    "after": {...},
    "ip": "192.168.1.1",
    "userAgent": "Mozilla/5.0..."
  }
]
```

### With Pagination

When pagination parameters are provided, the response includes pagination metadata:

```json
{
  "items": [
    {
      "_id": "507f1f77bcf86cd799439011",
      "timestamp": "2024-12-09T10:30:00Z",
      "userId": "507f1f77bcf86cd799439010",
      "userEmail": "user@example.com",
      "roles": ["admin"],
      "schemaName": "products",
      "documentIds": ["507f1f77bcf86cd799439012"],
      "action": "update",
      "description": "Updated product details",
      "before": {...},
      "after": {...},
      "ip": "192.168.1.1",
      "userAgent": "Mozilla/5.0..."
    }
  ],
  "totalItems": 150,
  "totalPages": 15,
  "page": 1,
  "limit": 10
}
```

## Example Requests

### Example 1: Get all audit logs for a specific schema (paginated)

```bash
GET /api/v1/audit-logs/?schemaName=products&page=1&limit=20
```

### Example 2: Get all create actions by a specific user

```bash
GET /api/v1/audit-logs/?action=create&userEmail=user@example.com&page=1&limit=10
```

### Example 3: Get audit logs within a date range

```bash
GET /api/v1/audit-logs/?startDate=2024-01-01T00:00:00Z&endDate=2024-12-31T23:59:59Z&page=1&limit=50
```

### Example 4: Get all delete actions sorted by timestamp ascending

```bash
GET /api/v1/audit-logs/?action=delete&sort=timestamp&asc=true&page=1&limit=10
```

### Example 5: Get audit logs for a specific document

```bash
GET /api/v1/audit-logs/?documentId=507f1f77bcf86cd799439012&page=1&limit=10
```

### Example 6: Get all login attempts from a specific IP

```bash
GET /api/v1/audit-logs/?action=login&ip=192.168.1.1&page=1&limit=10
```

### Example 7: Get all actions by users with admin role

```bash
GET /api/v1/audit-logs/?role=admin&page=1&limit=25
```

### Example 8: Complex query - Updates on products schema in December 2024 by admins

```bash
GET /api/v1/audit-logs/?action=update&schemaName=products&role=admin&startDate=2024-12-01T00:00:00Z&endDate=2024-12-31T23:59:59Z&page=1&limit=20
```

## Error Responses

### 400 Bad Request

```json
{
  "status": 400,
  "message": "Error fetching audit logs: invalid pagination parameters",
  "data": null
}
```

### 401 Unauthorized

```json
{
  "status": 401,
  "message": "Unauthorized",
  "data": null
}
```

### 403 Forbidden

Returned when the authenticated user does not have sufficient permissions to access audit logs.

```json
{
  "status": 403,
  "message": "Access denied: Insufficient permissions to access audit logs",
  "data": null
}
```

## Notes

- All timestamps are in UTC and follow the RFC3339 format
- ObjectID parameters must be valid MongoDB ObjectID hex strings (24 characters)
- The default sort is by timestamp in descending order (most recent first)
- If pagination is not enabled (no page/limit), all matching results are returned
- The `documentIds` field in the response is an array that may contain multiple document IDs for bulk operations
- By default, only users with the `admin` role can access audit logs (configurable via environment variables)
