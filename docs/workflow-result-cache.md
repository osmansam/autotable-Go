# Workflow result cache

Manual public read-only workflows can cache their complete result in Redis. This avoids rerunning the workflow's database and HTTP reads for repeated inputs.

Add these fields to the existing workflow definition after deploying a backend version containing this feature:

```json
{
  "isRedisCached": true,
  "cacheTime": 1
}
```

`cacheTime` is **minutes**, matching existing pipeline and dynamic API settings. `1` means one minute; `5` means five minutes. Zero or omission uses AutoTable's configured default cache TTL. Negative values and durations that overflow Go's supported duration are rejected. Existing workflows default to caching disabled.

Keep the workflow's existing name, steps, permissions, and return configuration. The fragment above is not a complete replacement workflow.

## Execution

The existing workflow endpoint and request body stay the same. For example:

```text
POST /api/v1/acme/law-test/dynamic/workflow/loadPublicPage?schemaName=siteConfig
Content-Type: application/json

{"record":{"sitePath":["contact"]}}
```

The first request runs the workflow. Subsequent eligible requests reuse the stored result until its TTL expires or the project revision changes. A hit returns the existing response envelope with `source: "cache"`; a normal execution returns `source: "workflow"`. The output data is unchanged, including large numeric values. Simultaneous misses use the existing Redis cache-fill lock and bounded wait. Expiry, Redis failure, or a wait timeout falls back to normal execution.

## Eligibility and authentication

Shared caching requires an active manual public workflow and an explicitly anonymous caller execution. Authentication middleware still runs before workflow execution. Calls with a user ID, roles, audit identity, system/unknown identity, form configuration, or an enclosing database transaction bypass the shared cache. Authentication-container workflows also bypass it.

Read operations (`get_record`, `find_records`, `count_records`, `distinct`, `aggregate`, `run_pipeline`), local transformations, conditions, loops, and return steps are eligible. `query_dynamic_api` is eligible because it enforces HTTP GET. All active branches are checked, including branches not taken by the current request.

Database writes (including array updates), audit writes, notifications, cache invalidation, outbox work, `execute_workflow`, generic `call_api`, and unrestricted `execute_dynamic_api` make the whole workflow ineligible. Unsupported operations execute normally without caching. The existing runtime treats `call_api` as outbox work even with GET; use `query_dynamic_api` for supported read-only HTTP queries. This release deliberately does not infer whether a nested workflow or arbitrary API call is safe to skip. Workflow errors, canceled requests, and any step failure continued by `continueOnError` or `stopOnError: false` do not populate the cache.

## Isolation and freshness

Keys isolate tenant, project, schema, workflow name, workflow/container definition, and complete record/query/oldRecord/stepOutputs/pagination inputs. Page and locale values therefore get separate entries. Definitions and input data are hashed in key names.

Writes through AutoTable's schema invalidation path advance the project's workflow cache revision. For transactions, these changes are collected and applied after a successful commit; read-only and rolled-back transactions do not advance it. Container metadata changes, translation changes, and locale-setting changes advance it too, including committed metadata writes followed by a secondary maintenance error. Post-write invalidation uses a bounded context that survives HTTP request cancellation. This invalidates workflow results across collections within that project; unrelated projects retain their entries. Old entries expire naturally by TTL.

Project-wide invalidation is conservative: editing one visual block also invalidates unrelated cached pages in that project. Changes made directly to MongoDB or to external GET services are discovered on the next cache miss/expiry. If Redis invalidation is unavailable after a committed write, the write remains successful and TTL bounds the stale period when Redis recovers.

The Next.js page-data cache is a separate layer with its own lifetime and invalidation. Backend caching does not remove either layer's first-request execution cost.

## Request overhead

Middleware and handlers reuse a successfully resolved URL project within the same HTTP request. This avoids repeating its Redis slug lookup and live MongoDB activity check before a workflow cache hit. Every new request still verifies the project, and each context lookup still validates the current authenticated tenant/project scope. Nothing is shared across requests by this optimization; collection permissions and authentication remain ahead of workflow result lookup.
