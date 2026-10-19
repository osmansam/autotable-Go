# Workflow audit — 2026-10-18

Dynamic functions were subsequently paused at the user's request. `GET /dynamic/execute`, the container `dynamicFunctions` update route, their controllers, the plugin compiler/loader, function cache helpers, and the middleware branch are commented out. The original implementation and its test cases remain in comments for future work; stored `DynamicFunctions` model fields remain compatible. Workflows, pipelines, and dynamic API routes remain enabled. The full test suite and `go vet ./...` pass after this change.

Added 138 top-level unit tests, nine live MongoDB integration tests, and one fuzz target across both audit rounds. The new files contain 642 passing unit leaf cases (including fuzz seeds) and 15 live database cases. Existing tests also pass.

Statement coverage across the workflow execution, validation, cron, outbox, and extracted workflow helper files increased from **55.3% to 70.0%**. This is scoped workflow coverage, not coverage of the entire application. The original audit reached 67.6%; the follow-up addressed its remaining implementation and integration gaps.

## Verified

| Check | Result |
| --- | --- |
| `go test ./... -count=1 -coverprofile=coverage.out` | Pass; all 16 packages containing tests |
| `go test -race ./... -count=1` | Pass; no detected data races |
| `go vet ./...` | Pass; the configuration test copy-lock warnings are fixed |
| `./scripts/test-workflow-mongo-integration.sh -race` | Pass; nine tests / 15 cases on MongoDB 5.0.27 |
| Template fuzz target, 10 seconds, two workers | Pass; 57,896 executions |
| `gofmt` and `git diff --check` on changed files | Pass |

The macOS race builds emit an `LC_DYSYMTAB` linker warning but finish successfully. The configuration test helper now initializes configuration once and restores only the configuration pointer, avoiding copies/resets of `sync.Once`. Existing authentication, route, and deployment edits were preserved.

## Regression fixes

- **Execution and cancellation:** canceled or expired contexts stop execution before mutations and between steps/iterations. Tests cover step ordering, skipped steps, continuation policies, business errors, and nested returns.
- **Snapshots:** workflow inputs, step outputs, nested records, queued definitions, and replay state no longer share mutable nested maps or slices. BSON scalar types and binary values retain their types. Whole-object template references are detached before assignment, preventing ancestor assignments from creating cyclic workflow state.
- **Templates and conditions:** inserted record text is treated literally rather than interpreted as another template. Typed BSON arrays resolve and traverse correctly. Query references work in conditions and expression defaults. Missing fields cannot satisfy ordering/string conditions; added and removed fields count as changes.
- **Validation:** unknown condition operators and unsafe outbox references inside BSON maps or typed slices are rejected. Inactive returns no longer prevent valid later steps. Unsupported `dynamic_function` definitions are rejected when validated instead of being accepted and failing during execution. Tests cover triggers, schedules, timeouts, modes, nested branches, return placement, limits, and step configuration.
- **Protected updates:** final-document previews preserve BSON distinctions and integer precision, validate paths, and reject unsupported modifiers/selectors instead of authorizing an inaccurately simulated document. This includes raw BSON and regex selectors. Unprotected updates retain native MongoDB behavior.
- **HTTP calls:** redirects retain a bounded chain length and revalidate full destination URLs. Resolution is checked again at actual connection time, and the dialer receives only validated literal IP addresses. Host headers and TLS hostname/certificate verification retain the original hostname. Environment proxies are bypassed for workflow requests. Preflight resolution and HTTP execution share a deadline bounded by the step timeout and existing ten-second request limit. Tests include DNS rebinding, mixed/malformed DNS answers, IPv4/IPv6 fallback, proxy bypass, TLS, methods, headers, response bounds, and cancellation.
- **Outbox ownership:** long-running jobs renew their leases. Renewal, acknowledgement, and retry writes check claim ownership; a stale worker cannot acknowledge a newer claim. Expired final attempts reach a terminal failed state. Retry delays cannot overflow.
- **Transactional completion:** database write steps and captured database-only `if`/`for_each` groups commit their data, post-write outbox events, and completion marker together. The marker is rechecked inside the transaction. Live tests deliberately reject marker inserts and prove that data and queued events roll back; concurrent delivery tests prove one committed mutation.
- **Replay and idempotency:** default keys identify durable events, so later updates to the same record are not suppressed forever. Scope components are escaped so separators cannot collapse distinct workflow/step identities. Descendant keys distinguish steps, loops, and branches and remain stable across replay. Captured children execute locally, conditional groups retain their conditions, and nested audit steps receive distinct stable identities. Query data and audit identity survive enqueue/replay.
- **Legacy replay:** new events persist snapshot version 1. Unknown future formats and ambiguous old snapshots are refused before execution with a requeue message. This includes missing root conditions, missing referenced query inputs, and delegated old workflows whose query inputs were not captured. Compatible old jobs still run. Audit identity can be reconstructed from trusted persisted user fields without fetching changed live identity data.
- **Cron:** container changes refresh captured execution context and shutdown cancels running work. Each nominal scheduled occurrence and its overlap lease are claimed atomically in Redis; the occurrence marker survives lease release for 24 hours, preventing sequential replicas from running the same tick. Timezone/DST, delayed callbacks, and `@every` reloads are tested. Unqualified schedules use UTC and interval schedules use a shared Unix-epoch anchor.
- **Live database coverage:** a database-scoped repository constructor supports isolated clients. Tests exercise real CRUD, combined row/policy/project filters, denied ownership transfer, Mongo operator semantics, transaction rollback, queued events, repeated updates, concurrent replay, unique indexes, and lease ownership. The runner starts a temporary loopback replica set, verifies its PID and database path before initialization, and cleans up its own process and data.
- **Bulk endpoint:** array requests no longer fail while parsing optional form-reference metadata. Endpoint tests execute a complete bulk request through the controller and service.
- **Aggregation:** a result limit is enforced after expanding stages, so an earlier `$limit` followed by `$unwind` cannot bypass the output cap.

## Compatibility and remaining limits

- Configured idempotency keys remain deterministic and scoped. Scope components and configured values containing separators are now escaped to prevent collisions. Already stored keys are replayed as stored. Default keys apply per event; repeated independent executions on the same record are allowed.
- Protected update previews intentionally reject Mongo modifiers, selector forms, or document comparisons whose result cannot be simulated safely. They also reject unsafe overflow and Decimal128 increments. Native unprotected updates are unaffected by these preview restrictions.
- Cron jobs skip execution when Redis cannot confirm a claim. Each occurrence permits at most one attempt: failures and cancellation after claiming are not automatically retried, and a crash after claiming can lose that occurrence. Callbacks older than 24 hours or dated in the future are skipped. UTC defaults and epoch-aligned `@every` intervals can change schedules previously anchored to a machine's local timezone or startup time; set `timezone` explicitly when needed.
- External side effects and groups mixing database writes with external effects still have at-least-once semantics across a crash. Optional provider idempotency headers propagate a stable, opaque, scoped retry key, but the remote provider must implement deduplication. This is not an exactly-once guarantee for arbitrary external APIs.
- Old snapshots cannot recover original query values or conditions that were never stored. Ambiguous jobs now fail safely and require requeueing with the original inputs rather than silently choosing behavior.
- Workflow HTTP calls intentionally bypass `HTTP_PROXY`, `HTTPS_PROXY`, and related environment proxy settings to prevent proxy-side resolution from bypassing IP validation. Ordinary API helper calls retain their previous proxy behavior.
- `dynamic_function` is unavailable in this processor and is now rejected by workflow validation. This audit does not add request-context-dependent code execution to background workers.

## Enabling provider idempotency

For a remote API that supports an idempotency header, opt in on the `call_api` step:

```json
{
  "type": "call_api",
  "executionMode": "outbox",
  "timeoutSec": 10,
  "config": {
    "url": "https://api.example.com/charges",
    "method": "POST",
    "idempotencyHeader": "Idempotency-Key"
  }
}
```

The configured header receives the same hash on retries of that scoped step and a different hash for other steps/scopes. It requires a durable outbox identity, overrides a caller-supplied value for that opted-in header, and cannot replace authentication or transport headers. Leaving the option absent preserves existing provider request headers.

## Reproducing live integration checks

```sh
./scripts/test-workflow-mongo-integration.sh
./scripts/test-workflow-mongo-integration.sh -race
```

The runner requires local `mongod` and `mongo`/`mongosh` executables. `MONGOD_BIN` and `MONGO_SHELL_BIN` can select explicit paths. Tests use a fresh database per case and never use the application's configured MongoDB database. Explicitly running the integration tests without a reachable replica set fails instead of silently skipping them.

No deployment, commit, or external publication was performed.
