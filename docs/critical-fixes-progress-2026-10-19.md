# Critical fixes: plan and progress

Requirements: [critical improvement analysis](critical-improvements-analysis-2026-10-18.md). The user authorized implementing these fixes without approval pauses. Preserve the dirty feature branch and paused dynamic-function code; no commits, deployment, or live customer-data migration.

Design: make authorization and resource selection consistent at the request/service boundary; reuse existing transaction and HTTP protections; use ownership-checked background writes. Keep compatibility where it does not preserve an insecure path. Test each behavior before changing it, then run full Go/race/vet and real MongoDB integration checks.

Pre-edit snapshot: `/private/tmp/autotable-critical-fixes-before-2026-10-19`.

| Task | Scope and acceptance | Status |
| --- | --- | --- |
| 1 | Auth defaults, scalar credentials, registration flag/server-controlled role; existing unsafe auth routes require management permission. | Implemented; auth/middleware/routes tests pass |
| 2 | Exactly one login session; startup provisions session indexes; duplicate legacy families excluded from refresh without deleting stored sessions. | Verified; full race suite and relevant integration checks pass |
| 3 | Export schema binding and field-role projection; cache hits and misses respect roles and nested fields. | Verified; full race suite and relevant integration checks pass |
| 4 | Pipeline testing requires management permission; aggregation validates stages/nested collections and applies bounds; query substitution cannot change structure. | Verified; full race suite and relevant integration checks pass |
| 5 | Cron applies the same transaction decision as manual workflows, with real rollback tests. | Verified; full race suite and relevant integration checks pass |
| 6 | Dynamic APIs share safe destination resolution, redirect handling, deadlines and transport with workflows. | Verified; full race suite and relevant integration checks pass |
| 7 | Reconciliation preserves concurrent manual edits; localization workers renew/fence claims and cancel stale owners. | Implemented; atomic content/progress fencing and real takeover test pass |
| 8 | Dependency-aware readiness; build before stopping/replacing release, rollback on failed readiness, PR/race/vet/real-Mongo CI. | Implemented; five isolated deployment scenarios pass |
| 9 | Final combined tests, independent review, compatibility notes and remaining operational limits. | Complete; review findings corrected and final verification passes |

Cross-task interfaces: auth/session startup and readiness share `main.go`; update sequentially. Export/cache/pipeline/API changes share `services/dynamic_service.go`; update sequentially. Localization changes affect repository interfaces and mocks together. Existing management route inventory tests must include the pipeline test contract.

Ruling: work in the existing feature checkout, preserving its uncommitted changes with a source snapshot, because the user's existing changes are required context and the user requested continuous execution. Cost if wrong: a clean worktree would omit the changes this fix depends on; snapshot/diff review protects unrelated work.

## Verification ledger

Record each task's failing regression tests, passing checks, implementation decisions, and residual constraints here as work proceeds.

Task 1: credential/registration/default tests RED (missing helpers) → GREEN; account management token/anonymous/membership tests RED → GREEN; `go test ./controllers ./middlewares ./routes -count=1` passed. Tenant panel account creation now targets management-only `/admin/auth/users` and does not sign the operator into the created account.

Task 2: full password-login regression reproduced two session writes before the fix. Ruling: introduce a sparse unique `sessionKey` for newly issued families; old unkeyed refresh sessions fail closed and require one re-login, while their records are preserved for TTL cleanup. This avoids deleting or rewriting customer session data or blocking index creation on historical duplicates. Cost: existing sessions must log in again when their access token needs refreshing.

Task 2: single-login, sparse unique fresh-session key, and legacy-rotation exclusion tests passed; startup index provisioning added. Auth-related package suites passed.
Task 3: cache/target-binding/nested-projection/Excel-column regressions reproduced leaks RED → GREEN; full service and utility suites passed.
Task 4: pipeline validation/parameter literal tests RED → GREEN; testing endpoint moved to an approved management route contract; raw testing joins denied, source access filter applied; stored pipelines retain same-project read-only joins.
Task 5: explicit cron transaction regression RED (wrapper not invoked) → GREEN; real replica-set rollback/commit regression added for final integration run.
Task 6: network regression RED (loopback request reached server) → GREEN; Dynamic APIs now use the same validating transport and redirect checks as workflow HTTP. Existing HTTP integration fixtures now configure a public address and explicitly inject a local test transport.
Ruling: private-address Dynamic API URLs fail closed; no implicit private-network exception is introduced. Cost: existing private integrations need an explicitly designed network policy before re-enabling; public integrations retain behavior.

Task 7: reconciliation uses an atomic update pipeline that reads the current text/origin rather than the discovery snapshot. Existing/new concurrent manual edits and changed source hashes pass against real MongoDB. Claims include unique tokens and expiry, renew periodically, and fence progress/completion. Content mutations take a conditional job write and translation write in one bounded snapshot/majority transaction. An increment guarantees the claim write acquires a write conflict even within one millisecond. Explicit caller sessions are rejected for worker content writes because a session alone does not establish a transaction. The real integration test pauses the content mutation, verifies takeover cannot commit through it, then rejects both stale content and failure-status writes.

Task 8: `/health` remains process liveness; `/ready` probes MongoDB and Redis with a two-second budget and no private diagnostic response. Startup provisions session and localization indexes before workers/listening. Deployment builds a unique image before replacing the app, retains the previous image and environment, and restores them on readiness or security-check failure. Nginx refreshes its app address after replacement/rollback. The first predecessor can use its legacy `/health`; successful newer images record their `/ready` contract, including Compose healthchecks. Secrets are staged with restrictive permissions and excluded from Docker build context. Production deployment waits for reusable PR/race/vet/Linux-build/replica-set CI. Hosted CI and an actual server rollout have not been executed here.

Review follow-up: a fresh read-only reviewer found account execution routes needed schema-write permissions and activation checks, content fencing needed one transaction, rollback needed the old health contract, and the password tag needed `minlength=8`. These were corrected with regressions. The same reviewer checked the corrections and identified the explicit-session edge case; that was reproduced and closed. Additional cache tests cover both role orders, anonymous reads, field-policy edits, and waiting for another cache fill. Nested BSON arrays and exports whose entire child schema is restricted also have regressions.

## Upgrade and compatibility notes

- Existing email-only auth configurations now reject password login until a hashed secret credential is configured. Plan a password enrollment/reset or verified identity flow; no passwords or customer configuration were silently invented or backfilled. New default projects require email and a password of at least eight characters and leave public registration off.
- Account/role collection routes require a matching project management token and current membership. The tenant panel creates accounts through `/admin/auth/users`, which never replaces the operator's login session. Public registration cannot select privileges and assigns its configured basic role on the server.
- Historical unkeyed refresh families require one re-login. Their records remain for TTL expiry, and family revocation updates every legacy duplicate. New sessions have a sparse unique session key. Existing access tokens retain their normal expiration.
- Localization worker content writes require a MongoDB replica set/transaction-capable deployment. Startup requires permission to create the declared indexes. If historical duplicate localization identities prevent a unique index, startup fails rather than deleting/merging customer translations; inspect and resolve such data deliberately before rollout.
- Dynamic APIs reject local/private destinations, including unsafe redirects/DNS results, and bypass environment proxies. There is no private-network exception. Existing private integrations need a separately designed allow policy.
- Pipeline testing requires project schema-write permission and is limited to the authorized source collection. Read-only source access rules are prepended. Saved pipelines may use validated same-project joins with fully qualified collection names or project collection placeholders; write stages and server-side JavaScript are rejected, execution is bounded, and results are capped.
- Use `scripts/deploy-production.sh` with a unique release tag and staged `.env.next` for rollout. This is a single-instance replacement with brief recreation downtime, not a zero-downtime switch. Keep `.deploy` metadata and prior images for rollback. A rollback to the first older image can verify only that artifact's existing liveness endpoint.

## Remaining operational limits

Cron occurrence markers still represent an attempted run, so a process crash after claiming can lose that occurrence. A durable schedule queue, retry/idempotency rules, and a catch-up policy remain a separate scheduler design change. Paid translation requests already in flight cannot be undone on lease loss; stale workers are canceled and cannot commit generated-content/job updates after another owner takes over.

No deployment, live-data cleanup, secret rotation, database privilege migration, backup restoration, or TLS verification was performed. Those require evidence from the target environment. Dynamic-function code and its disabled routes/UI remain preserved.

## Final verification (2026-10-19)

| Check | Result |
| --- | --- |
| `go test -race ./... -count=1` | Passed: 749 top-level tests and 1,313 subtests across 15 tested packages; zero failures/race reports |
| `go vet ./...` | Passed |
| `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build` | Passed; binary written under `/private/tmp` |
| `scripts/test-workflow-mongo-integration.sh -race` | Passed: 14 real MongoDB integration tests plus subtests, using a disposable task-owned loopback replica set |
| Tenant panel `vitest run` | Passed: 587 tests across 117 files |
| Tenant panel TypeScript and Vite production build | Passed; existing bundle-size warning remains |
| Deployment black-box regression tests | Passed: five scenarios, with fake Docker/curl only |
| Shell syntax, workflow/Compose YAML parsing, both repositories' `git diff --check` | Passed |

No commits, pushes, actual deployment, or live customer-data modifications were made. Test logs and task-only review artifacts are under `/private/tmp/autotable-critical-*`.
