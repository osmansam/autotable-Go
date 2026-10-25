# Workflow result caching

Add optional Redis caching to manually invoked public read-only workflows, including `loadPublicPage`, to avoid repeating database reads for identical requests.

## Approach

Cache the final workflow result in AutoTable's existing Redis infrastructure. Per-step caching would still execute the rest of the workflow and introduces additional keys and invalidation rules. The existing Next.js cache remains useful but does not cover direct AutoTable requests. The user approved implementing complete-result caching in AutoTable.

## Configuration and behavior

- Add `isRedisCached` (default false) and `cacheTime` (minutes, consistent with pipeline/API settings; zero uses the existing configured default).
- Caching is opt-in. Existing workflows keep their current behavior.
- Cache only successful, public, read-only manual workflows. Database writes, notifications, outbox work, cache invalidation, and non-read HTTP calls are ineligible. Nested workflows and unresolved external operations bypass caching unless their read-only behavior can be established safely.
- Authenticated, authorized, personalized, and form-bound execution bypasses the shared cache. Existing authentication and authorization checks remain ahead of cache lookup.
- Cache keys isolate tenant, project, schema, workflow definition/version, and complete execution inputs, including page, locale and pagination. No input-selected cache-key fields may accidentally combine different responses.
- Reuse the existing Redis cache-fill lock to limit duplicate execution on simultaneous misses. Cache errors fall back to normal workflow execution. Workflow errors and partial execution failures are not cached.
- Content/configuration writes invalidate workflow results for that project through a project cache revision. This intentionally favors reliable invalidation across collections over retaining unrelated page entries. Localization changes must participate too. External sources remain subject to the configured TTL.
- Preserve the response envelope and data shape. Cache hits identify their source as `cache`, consistent with existing dynamic API behavior.

Example workflow settings:

```json
{
  "name": "loadPublicPage",
  "isRedisCached": true,
  "cacheTime": 1
}
```

These fields are added to the existing complete workflow definition; this fragment is not a replacement workflow. `cacheTime: 1` means one minute.

## Verification and delivery

Test cache hits, expiry, input/project/tenant isolation, definition changes, invalidation across collections, Redis failure, concurrent fills, and bypasses for authentication, writes, errors, and partial failures. Run relevant Go tests and the repository checks. Include usage documentation and an enabling example. Deliver local changes for review; deploying or editing production workflows is a separate action.
