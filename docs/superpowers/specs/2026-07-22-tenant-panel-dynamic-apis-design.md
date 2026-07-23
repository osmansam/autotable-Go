# Tenant Panel Dynamic APIs Design

## Goal

Add per-container Dynamic API management to tenantPanel and prevent general container edits, such as adding fields, from breaking existing Dynamic API configuration.

## Scope

Dynamic APIs are managed inside each container details modal as a new tab next to Pipelines. The feature does not add a global cross-container Dynamic API dashboard.

## Root Cause

TenantPanel field edits use the general container PATCH endpoint. The backend `UpdateContainer` handler preserves existing pipelines, dynamic functions, and workflows before saving the updated container, but it does not preserve `DynamicApis`. When tenantPanel sends a field-focused payload that omits `dynamicApis`, the backend can overwrite the existing Dynamic API list with an empty value.

## Backend Design

Add `UpdateDynamicApis` to the container controller using the same narrow-update pattern as `UpdatePipelines` and `UpdateDynamicFunctions`.

The endpoint accepts:

```json
{
  "dynamicApis": [
    {
      "name": "status",
      "url": "https://example.com/status",
      "method": "GET",
      "dependencies": ["id"],
      "isAuthenticated": false,
      "isAuthorized": false,
      "authorizeRole": [],
      "isActive": true,
      "isRedisCached": false,
      "cacheTime": 0
    }
  ]
}
```

It updates only the container `dynamicApis` field, invalidates container cache keys, emits the existing container change websocket event, and returns the same response shape as pipeline/function update handlers.

Also update `UpdateContainer` to preserve `existingContainer.DynamicApis`, so older frontend paths and partial general edits cannot erase Dynamic APIs.

## TenantPanel Design

Add a `Dynamic APIs` view mode to `ContainerDetailsModal`.

The tab shows:

- API name, HTTP method, URL
- active/inactive state
- auth/role/cache badges
- dependency list
- edit and delete actions
- empty state with an add button

Add `AddDynamicApiModal` following the existing `AddPipelineModal` conventions:

- name, URL, method, dependencies
- active toggle
- require authentication toggle
- require authorization toggle with role multiselect
- redis cache toggle and cache time

Add `useUpdateDynamicApis()` to `tenantPanel/src/utils/api/container.ts`, using:

```text
PATCH /:tenantSlug/:projectSlug/container/dynamicApis/:id
```

with payload `{ dynamicApis: DynamicApiModel[] }`.

## Error Handling

Frontend validation blocks missing name, missing URL, duplicate name on add, and missing HTTP method. Backend invalid ObjectID and missing container behavior follows the existing narrow update handlers.

## Testing

Backend tests cover:

- `UpdateContainer` preserves existing Dynamic APIs during a field-focused update.
- `UpdateDynamicApis` rejects invalid container IDs through the existing controller error-path style.

TenantPanel verification covers:

- TypeScript/build succeeds after adding the hook, modal, and tab.
- Manual UI path: open a container, add/edit/delete Dynamic APIs in the tab, add a field, and confirm Dynamic APIs remain present in the JSON tab.
