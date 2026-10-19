# AutoTable: critical improvement analysis

Reviewed the current working tree on 2026-10-18, including the existing authentication and workflow changes. This is a source review with isolated diagnostic probes, not a test against production. Application code was not changed during this review.

The first priority is to close authorization and data-access gaps. More workflow features, broad refactoring, and coverage percentages should follow those fixes.

## Priority order

| Order | Priority | Improvement | Main consequence today |
| --- | --- | --- | --- |
| 1 | Critical with default project configuration | Secure the default auth collection and registration/login rules | Anonymous account reads/writes and authentication without proof of a secret |
| 2 | Critical when TestPipeline is exposed | Restrict arbitrary aggregation to a controlled management operation | Collection isolation can be bypassed; aggregation can also perform writes |
| 3 | High | Bind export authorization to the actual schema and fields exported | A permitted schema can supply authorization for a different data collection |
| 4 | High when caching is enabled | Apply field permissions consistently on cache hits | A lower-privilege user can receive a higher-privilege cached projection |
| 5 | High | Repair login session creation and initialize session indexes | Duplicate sessions undermine refresh/revocation; an existing unique index makes login fail |
| 6 | High for scheduled writes | Honor transaction requirements for cron workflows | A failed workflow can leave data committed without its corresponding outbox event |
| 7 | High where project authors are untrusted | Share the outbound HTTP security policy with Dynamic APIs | A configured API can reach the application's local/private network |
| 8 | High for concurrent localization editing | Preserve manual translations during reconciliation | A stale reconciliation snapshot can overwrite a newer human edit |

These are conditional on the paths/configurations described below. They do not establish that a deployed instance has been exploited.

## 1. Default authentication needs safe defaults

**Evidence:** `controllers/projectController.go:283` creates the default auth collection with email as its only login credential and no hashed secret. At `:311`, its ordinary data routes, including reads, updates, deletes, and pipeline testing, are active without authentication flags. Non-template project creation uses this bootstrap at `:787`.

`controllers/authController.go:389` constructs a MongoDB filter directly from submitted non-hashed credential values. Secret comparison happens only for hashed credential fields at `:421`. Consequently, the default configuration supplies no password or other possession proof. Login also lacks scalar/type validation before constructing its filter.

Registration checks `IsAuthContainer` at `controllers/authController.go:229`, but does not enforce `IsRegisterActive`. It accepts and stores the submitted user map, and copies a submitted role into the issued token at `:325`. This is a project application-user issue; the separate management-token boundary should not be described as bypassed by these findings.

**Improve:** disable public account-management CRUD by default; require an explicitly configured password or verified identity flow; reject object/operator-shaped login credentials; enforce the registration flag; assign self-registration roles on the server. Review existing project configurations as well as fixing the bootstrap for new projects.

**Required regression tests:** a new project's anonymous account read/update/delete attempts are denied; knowing an email alone is insufficient; non-scalar login inputs fail; disabled registration fails; caller-selected privileged roles cannot be issued.

## 2. Pipeline testing needs a separate security boundary

**Evidence:** `routes/dynamicRoutes.go:18` exposes pipeline testing through a schema-configurable data-route permission, rather than the management-route contract. `controllers/dynamicController.go:918` accepts the caller's pipeline definition. `services/dynamic_service.go:2078` executes it; `utils/dynamicPipeline.go:18` parses arbitrary stages and `:47` passes them directly to MongoDB aggregation.

There is no read-only stage validation, validation of referenced collections, or record-access restriction in this execution path. Filtering result fields after execution at `services/dynamic_service.go:2088` cannot restrict what the database already read or wrote. MongoDB aggregation supports cross-collection stages and write-producing stages. Application-side project collection prefixes do not constrain a collection named inside a raw pipeline. The practical reach also depends on the database account's permissions.

**Prerequisite:** the caller can access an active TestPipeline route. This includes the unsafe default auth configuration above; a deliberately disabled/protected route reduces exposure.

**Improve:** make testing a management-authorized operation; use a read-only stage allowlist; validate nested stages and every referenced collection; enforce project and record-access policies; bound execution and results. Use database privileges that reduce the consequences of a validation mistake.

**Required regression tests:** unauthorized testing is denied; write stages and out-of-project references are rejected before execution; nested stages receive the same checks; read-only tests honor access policies and resource limits.

## 3. Export must authorize the same target it queries

**Evidence:** `middlewares/authenticate.go:134` selects and authorizes a container from the query-string schema and stores it in request locals. `controllers/dynamicController.go:1171` independently parses the export body, then passes both that request and the stored container to the service. `services/dynamic_service.go:2243` trusts the supplied container, but `:2293` queries the body request's schema.

Thus one schema supplies permissions, field definitions, and record-access rules while another supplies the data. This is a schema boundary within the resolved project, not evidence of arbitrary cross-tenant access through this endpoint alone. Exported columns still depend on the first schema; overlapping fields are sufficient for disclosure.

The normal export path also strips hashed fields but does not apply the caller's field-role projection before workbook creation (`services/dynamic_service.go:2302` and `:3208`). These checks need to be consistent with ordinary reads.

**Improve:** resolve one canonical target before authorization, reject conflicting schema inputs, verify a supplied container matches that target, and apply field permissions to the selected export columns and values.

**Required regression tests:** query/body mismatches fail before a database query; a public schema cannot export a protected schema; restricted columns remain absent from an otherwise permitted export.

**Validation:** a mock MongoDB probe supplied metadata for one schema and an export request for another. The actual service queried the second collection and returned its sample value in the workbook.

## 4. Cached records must not bypass field permissions

**Evidence:** `services/dynamic_service.go:1252` builds a cache key from schema and record identity without the caller's role. Cache hits at `:1254` and `:1273` return the item without `FilterDocuments`. A database miss applies that role filter at `:1310` and caches the resulting projection at `:1316`.

An administrator can therefore warm the cache with restricted fields that a later member receives. The reverse order can hide fields an administrator should see. Existing record-access/row-access isolation disables caching for those configured cases (`:1251`); it does not cover field-role permissions on otherwise shared records.

**Improve:** make projection a mandatory final step for every response, with a cache design that does not reuse one role's projection as another role's source. Either isolate projections by the relevant authorization policy/role or cache a suitable base record and project a detached copy for every request.

**Required regression tests:** admin then member, member then admin, anonymous reads, nested restricted fields, cache-fill wait responses, and permission changes.

**Validation:** an isolated test with emulated Redis and mock MongoDB fetched an administrator's record, then fetched it as a member. The second result came from cache and contained the administrator-only sample field.

## 5. Session creation and indexes need one lifecycle

**Evidence:** `controllers/authController.go:491` and `:494` call `startProjectAuthSession` twice with the same token pair. The helper at `controllers/authCookieHelpers.go:56` derives the same refresh family each time. `services/auth_session_service.go:43` always creates a session, and `repositories/auth_session_repository.go:38` uses InsertOne.

`repositories/auth_session_repository.go:29` defines a unique family-ID index and an expiry index, but no production call to this EnsureIndexes method was found. With the unique index installed, the second insert fails and this login path returns HTTP 500. Without it, two family documents are inserted; refresh rotation and family revocation operate with UpdateOne (`:43`, `:76`), so their single-family guarantees no longer hold. The unused expiry-index setup also leaves automatic session cleanup dependent on external provisioning.

**Improve:** create exactly one session per login, provision and verify indexes at startup/migration time, and safely reconcile existing duplicate families before enabling uniqueness. Keep production migrations separate from test setup.

**Required regression tests:** a complete successful login persists one family; refresh reuse is rejected; logout revokes that family; startup establishes the required indexes; duplicate legacy data is handled explicitly.

## 6. Cron must honor transaction requirements

**Evidence:** `services/dynamic_cron.go:215` directly calls `runWorkflowDefinition` with an ordinary context. It never invokes the transaction wrapper. By comparison, manual execution uses `workflowRequiresTransaction` and `runTransaction` at `services/dynamic_service.go:2214`. The transactional execution mode inside `services/dynamic_workflow.go:194` selects steps; it does not itself open a MongoDB transaction.

**Improve:** use a shared execution entry point that applies transaction requirements consistently to manual, scheduled, and nested execution. Commit scheduled database changes and their durable events together.

**Required regression tests:** a scheduled workflow with an explicit transaction request uses a session; injected later-step/outbox failure rolls back earlier writes on a real replica set; a successful run commits both data and events.

**Validation:** a mock command probe ran a cron workflow with `runInTransaction=true`, accepted the record insert, and injected an outbox failure. The transaction wrapper was never called and the insert command had no transaction flag. This confirms the missing transaction boundary; it is not a live-database rollback test.

## 7. Dynamic APIs need the workflow outbound-network protections

**Evidence:** workflow HTTP calls use URL checks and a validating transport, but `services/dynamic_service.go:2141` sends Dynamic API calls through the general helper. `utils/executeApiRequest.go:35` constructs a normal HTTP client; this call supplies no destination policy. The management update at `controllers/containerController.go:1042` stores the supplied Dynamic API definitions without validating their destinations.

**Prerequisite:** an actor can configure a Dynamic API, or a configured endpoint redirects/resolves into an unsafe destination. A runtime caller does not gain arbitrary URL selection through the API name alone. The risk matters when project-scoped authors must not access the host's internal network.

**Improve:** share one outbound-network policy/transport across features, including redirects and DNS resolution. If private integrations are intended, allow them through an explicit, narrowly scoped policy rather than unrestricted network access.

**Required regression tests:** private/loopback targets, redirects into private networks, changing DNS answers, proxy behavior, and explicitly approved private integrations.

**Validation:** a local HTTP test server was called successfully through ExecuteDynamicAPI, confirming that this path accepts a loopback destination. No external/internal production service was contacted.

## 8. Reconciliation must preserve concurrent manual translations

**Evidence:** `services/localization_reconciliation.go:120` reads a snapshot of translations, builds changes, and applies them later. `repositories/localization_repository.go:94` writes translated text and origin from that snapshot using only the translation's identity as its filter at `:105`.

If an editor saves a manual translation between those operations, reconciliation can replace it with older text and origin. The generated-translation path already has a manual-origin guard (`:136`), but reconciliation does not share that protection.

**Improve:** update only source/discovery fields during reconciliation and preserve human-owned content, or use versioned conditional updates with an explicit conflict policy. New placeholder creation must also avoid replacing a concurrent manual insert.

**Required regression tests:** pause reconciliation after its snapshot, save a manual edit, then resume; repeat for a newly inserted manual translation and a concurrent source change.

**Validation:** mock MongoDB command inspection confirmed that reconciliation sends stale text/origin with an identity-only filter and no version/manual-origin condition.

## Next operational priorities

- **Fence localization job ownership.** `services/localization_worker.go:17` sets a fixed 30-minute lease without renewal. Expired running jobs can be claimed again at `repositories/localization_repository.go:293`. Progress/completion updates at `:183` and `:191` filter only by job ID, and completion clears whichever lease is present. Long jobs on multiple replicas can duplicate paid translation work and let an old worker overwrite the new worker's status. Add renewal, claim tokens, ownership-checked writes, and cancellation on lost ownership. A mock command probe confirmed the unfenced completion update.
- **Make deployment preserve a working release.** `.github/workflows/production.yml:93` shuts down the stack before a fresh build. Build failure leaves the old service stopped. Build a versioned artifact first, start/check the replacement, switch traffic, and retain a tested rollback path. `/health` at `main.go:127` always reports success after startup; add dependency-aware readiness separately from process liveness.
- **Put the important checks in CI.** The deployment workflow runs ordinary Go tests at `.github/workflows/production.yml:21`. The real MongoDB tests require the integration build tag and their runner. Add PR checks for the auth/data-access scenarios above, race/vet checks, and replica-set integration tests. A passing helper-heavy test suite does not establish these endpoint security invariants.
- **Make cron missed-run behavior explicit.** The occurrence marker deliberately represents an attempt rather than completion (`services/workflow_cron_occurrence.go:132`). A crash after claiming can lose a scheduled run. For business-critical schedules, persist scheduled occurrences as retryable jobs with a defined catch-up policy. This is a documented design limitation, separate from the transaction bug above.

## Verification and scope

- `go test ./... -count=1`: passed after allowing the loopback servers used by tests.
- `go test ./... -race -count=1`: passed.
- `go vet ./...`: passed.
- Three isolated service probes confirmed cache projection reuse, export target mismatch, and unrestricted loopback Dynamic API access.
- Three isolated mock command probes confirmed the cron transaction gap, stale reconciliation writes, and unfenced localization completion.
- Probe source and logs were kept under `/private/tmp`; they are diagnostics, not permanent regression tests or production changes.
- No deployed routes, live customer data, live MongoDB indexes, TLS termination, or backup/restore operation was inspected. No dependency vulnerability scan was performed. Those unknowns should not be presented as confirmed production failures.

Recommended implementation sequence: fix and test the default auth and pipeline boundaries first; then export, cache, and session correctness; then scheduled transaction handling, outbound-network consistency, and localization concurrency; finally enforce the checks and safer rollout process in CI/deployment.
