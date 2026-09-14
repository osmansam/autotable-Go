# Public development bulk container importer

`POST /api/v1/{tenantSlug}/{projectSlug}/container/create-multiple`

This endpoint intentionally requires no authentication. It is registered before the authenticated container route group. It is always available while that route registration exists; there is no automatic environment gate or one-use lock. Remove the `create-multiple` registration in `routes/containerRoutes.go` after your development import.

The project must already exist and be active. Tenant/project scope comes from the URL slugs. Request body IDs and query parameters cannot select a different project.

From the repository directory, replace the host and slugs and run:

```sh
curl --request POST \
  'http://localhost:3000/api/v1/YOUR_TENANT_SLUG/YOUR_PROJECT_SLUG/container/create-multiple' \
  --header 'Content-Type: application/json' \
  --data-binary @docs/examples/miywo/all-containers.json
```

Use your configured server port if it differs from 3000. No Authorization header is needed.

## Request

Send a JSON array of 1–100 complete container definitions, using the same shape as single-container creation:

```json
[
  {
    "schemaName": "profiles",
    "fields": [{"name": "id", "type": "string", "tag": "required"}]
  },
  {
    "schemaName": "pets",
    "fields": [{"name": "name", "type": "string", "tag": "required"}]
  }
]
```

The minimal example leaves dynamic routes inactive. The Miywo file includes its route and index settings.

Malformed JSON, empty/oversized arrays, blank names, or repeated names within a request return HTTP 400 before any creation begins. The existing request-body size and write rate limits apply. Missing/inactive projects return HTTP 404.

## Processing and response

Definitions are created sequentially in array order, with a 10-second timeout per definition. Native `objectId`/`objectIdArray` references require their target containers to exist already; list referenced containers first. The Miywo definitions preserve UUID references as strings, so their order does not matter.

The endpoint reuses single-container validation, index creation, reference checks, cache defaults and cache invalidation. It continues after individual errors. This is not an atomic transaction: successful inserts are retained, and existing container definitions are never overwritten.

HTTP 201 means every definition succeeded. HTTP 207 means at least one definition returned an error, including when all failed. Inspect each result:

```json
{
  "created": 1,
  "failed": 1,
  "results": [
    {"schemaName": "profiles", "status": 201, "created": true, "id": "0123456789abcdef01234567"},
    {"schemaName": "pets", "status": 404, "created": false, "error": "The specified schema already exists in containers"}
  ]
}
```

Duplicate-container errors retain the existing single-create status (404). `created` counts persisted containers; `failed` counts results with errors. A container can be counted in both if insertion succeeds but subsequent reference-cache synchronization fails: that result retains `created: true`, its ID, and its error. Check the existing container before retrying such a result. A disconnected or timed-out client also does not imply rollback.
