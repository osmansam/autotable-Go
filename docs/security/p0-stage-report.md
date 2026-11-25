# P0 hardening stage report

No production deployment, data operation, license addition, or history rewrite is part of this work.

## Stage 1 — verified inventory and Go toolchain

- Baseline and isolated-worktree baseline: `go test ./... -count=1` PASS, all 23 package entries (19 tested, four without tests).
- Go 1.24.10 → minimum 1.26.0, pinned toolchain 1.26.11. Complete suite PASS after upgrade.
- 126 unique baseline advisory findings; full verified inventory in `p0-advisories.md`. Go 1.24 patches cannot resolve current standard-library and dependency findings.
- Compatibility: builds/tests under Go 1.26.11; minimum build Go version intentionally increases. Application contracts unchanged in this stage.
- Remaining: third-party advisory fixes and application hardening pending. No clean-scan claim at this point.
- Deployment risk: CI/build agents must download/use Go 1.26.11; Docker builder is updated in its own stage. Do not deploy an intermediate stage with the old Docker builder.

## Stage 2 — JWT v5 migration

- `dgrijalva/jwt-go` removed; verified maintained target `golang-jwt/jwt/v5 v5.3.1` (Go >=1.21).
- Tests before upgrade PASS. New negative tests failed on missing/malformed expiry and empty project identifiers, then PASS after requiring expiration, HS256, issued-at validation, audience/type/scope and nonempty identifiers.
- Full suite after migration PASS, including login, refresh rotation, session recovery, middleware authorization and domain-crossing regressions with intentionally identical test keys.
- Compatibility: tenant `aud` remains a JSON string, claim names/expiry periods/cookies unchanged; v5 registered claims are internal. Missing expiry or session identifiers are intentionally rejected; normal issued tokens already have both.
- Remaining: non-JWT dependency advisories and application hardening pending. Existing v3 audience advisory no longer present as a dependency.
- Deployment risk: clients with malformed/custom tokens must reauthenticate; ordinary previously issued HS256 tokens retain wire compatibility if secrets are retained. Strong-key enforcement/rotation is a later stage.

## Stage 3 — HTTP and compression dependency group

- Fiber v2.52.16, fasthttp v1.80.0, brotli v1.3.0, compress v1.20.5. Required transitive x/crypto v0.59.0, x/net v0.62.0, x/text v0.44.0, x/sys v0.50.0, x/sync v0.25.0 are Go 1.26 compatible, verified by full build/tests.
- Complete tests before group PASS. First post-upgrade run found one assertion tied to obsolete cookie-header text. fasthttp emits `Max-Age=0` to delete cookies; test now checks parsed deletion semantics and matching path. No application-cookie logic changed. Complete suite then PASS.
- Module scan: 38 unique remaining advisories (telemetry/gRPC, Excelize/image, Mongo driver, gorilla/schema). Scan level differs from baseline reachability; no exploitability claim.
- Compatibility: retain Fiber v2 despite deprecation notice; no v3 architectural migration. Cookie deletion remains standards compliant; external consumers must parse cookie attributes rather than match old header text.
- Deployment risk: Go 1.26 required; exercise ingress/proxy/cookie flows in staging before release. No deployment performed.

## Stage 4 — telemetry, gRPC and form decoder dependencies

- OTel family v1.47.0, gRPC v1.85.0, x/net v0.63.0, gorilla/schema v1.4.1; selected transitive protobuf v1.36.12 and OAuth v0.36.0. Published targets and Go requirements checked before upgrade; retain existing APIs.
- Complete existing suite before and after group PASS. No application code changes required.
- Remaining advisories are limited to Mongo driver, Excelize and x/image; count recorded by the module rescan (see verification log). Application controls remain pending.
- Deployment risk: verify telemetry collection/exporter interoperability and form input in staging. Toolchain remains Go 1.26.11; API contracts unchanged.

## Stage 5 — Mongo, spreadsheet and image dependencies

- Mongo v1.17.10 (retain v1 API), Excelize v2.11.1, x/image v0.48.0; published Go requirements verified first.
- Complete suite before group PASS. Initial post-upgrade compile failed because Mongo's test helper removed `mt.Close`; confirmed upstream now registers cleanup with `testing.T.Cleanup`, and removed only obsolete test cleanup calls. Production Mongo behavior unchanged.
- Complete suite after adaptations PASS. Official module vulnerability scan: **zero findings**, down from 27 after telemetry group (38 after HTTP group, 126 at baseline).
- Compatibility: Mongo v1 is deprecated in favor of v2; v2 migration intentionally excluded. Spreadsheet tests pass; real customer workbook staging checks remain advisable.
- Deployment risk: Mongo and spreadsheet libraries changed across minor releases; isolated transaction/session integration tests are still required before release. Local Docker engine is stopped; investigate existing task-owned native Mongo runner before declaring this unavailable.

## Stage 6 — HTTP and background panic recovery

- HTTP recovery sanitizes 500 responses and clears partial bodies. Test without middleware crashes as expected; restored middleware passes and serves subsequent requests.
- Child localization and outbox goroutines recover inside the goroutine and complete result channels with an error; cron dispatch gets the same task boundary. Outbox panic is retried and never acknowledged as success. Localization leaves the durable lease for expiry/recovery instead of falsely completing.
- Essential worker panic cancels application context, stops HTTP with a deadline and exits nonzero for supervisor restart, including a failure before the listener starts. Optional cache fill and websocket read/write have local boundaries; writer cleanup completes before pooled connection reuse.
- Initial child-panic regression crashed before implementation; all recovery regressions and complete suite PASS after implementation. Panic logs retain stacks but omit panic values to avoid leaking credentials/customer payloads.
- Remaining: module scan remains zero known advisories; secret strength, headers and diagnostics controls still pending. Recovery cannot contain fatal runtime faults/OOM and does not undo already committed external side effects.
- Deployment risk: an essential worker panic now restarts the service rather than leaving it silently degraded; durable claims retry under existing semantics and external idempotency requirements.

## Stage 7 — integration-compatible security headers

- Add nosniff, strict-origin-when-cross-origin referrer policy, permissions policy and disable obsolete browser XSS filtering globally, including error responses.
- Complete suite PASS; actual local WebSocket 101 handshake/message, credentialed CORS, preflight with authorization/idempotency/client headers and real Swagger HTML all PASS.
- Compatibility: no global CSP, frame denial or cross-origin resource isolation that could break Swagger CDN/inline scripts or customer embeds. HSTS belongs at trusted TLS ingress (app also serves HTTP); broad HSTS/includeSubDomains is not imposed by the API.
- Remaining: zero known Go dependency findings; strong-key and diagnostics changes pending. Document-specific CSP/frame policy remains a frontend/ingress decision.
- Deployment risk: browser integration checks are automated at protocol level, not a full staging browser/customer acceptance run. No API response schema changes.

## Stage 8 — cryptographic generation and signing-key policy

- Add `go run ./cmd/autotable-secrets`: generates independent 256-bit keys via crypto/rand, prints assignments, never applies/rotates them.
- Startup rejects missing, undersized, unencoded, whitespace-padded, repetitive and reused signing material; compares decoded bytes to detect reuse across encodings. Optional legacy CONTAINER_JWT_SECRET is validated when present. Literal HMAC key interpretation is preserved.
- Weak/equal-key regressions failed before implementation; full suite and generator uniqueness/format tests PASS after implementation. No real keys were inspected, generated into repository files, or changed.
- Limitation: externally supplied string randomness cannot be proven by validation. URL-base64 can encode text-like strings; operators must use the cryptographic generator/secret manager. Tests do not claim a mathematical randomness/provenance proof.
- Remaining: zero known Go advisories; diagnostics access and container/CI work pending.
- Deployment risk: weak existing secrets intentionally prevent startup. Key rotation invalidates access/refresh tokens and requires coordinated replicas/customer reauthentication; see `p0-rollout.md`. No production rotation performed.

## Stage 9 — restricted diagnostics

- Metrics default-deny when OBSERVABILITY_TOKEN is unset; configured metrics require a dedicated strong bearer credential. Invalid/reused diagnostics credentials fail startup; handler also fails closed.
- Development pprof is opt-in via ENABLE_PPROF=true plus diagnostics authorization; production pprof remains unmounted even with a valid token.
- Default-development profiling regression failed before changes. Complete suite PASS after changes; tests cover metrics case/slash variants, pprof subpaths, missing/wrong/correct credentials and forged forwarded loopback headers.
- Prometheus example reads a mounted secret file; production environment generation forwards optional diagnostics credential. No real credential is committed.
- Remaining: zero known Go advisories; Docker/CI/artifact cleanup and final checks pending.
- Deployment risk: existing unauthenticated scrapers stop working until their independent token/file mount is configured. Diagnostics must travel only on trusted networks/TLS. Customer API authentication is unchanged.

## Stage 10 — non-root, pinned Docker image

- Pin official builder Go 1.26.11/Alpine 3.24 and runtime Alpine 3.24.1 by verified immutable Docker Hub manifest digests. Run as UID/GID 10001 with owned upload/log directories, root-owned binary/config JSON. Exclude local build/development outputs and secrets from context.
- Linux amd64 CGO-disabled cross-build PASS. Official registry manifest requests verified both digests. Deployment regression suite previously PASS (27 tests).
- Local Docker runtime/build validation is pending: installed engine reports stopped. It was not started because its start command resumes unrelated machines. CI image validation is added in the next stage.
- Remaining: zero known Go dependency findings; OS package vulnerabilities are not covered by govulncheck or digest pinning and require an image scan before release. No claim of a clean OS scan.
- Compatibility/deployment risk: staging/production bind mounts must permit UID/GID 10001 writes; image ownership cannot override host volume permissions. Operator action and staging smoke test required. No host volume ownership/data was changed.

## Stage 11 — vulnerability CI and manual production release

- CI installs pinned official govulncheck v1.10.0 and runs both source reachability and strict module scans. Pin Go 1.26.11 in CI to avoid using the unpatched minimum 1.26.0 from go.mod.
- Add image build and non-root runtime permission smoke checks in an isolated CI container, without starting the app or connecting databases. Local container check remains pending while Docker is stopped.
- Production push events now verify only; deploy job requires explicit workflow_dispatch. No workflow was dispatched, pushed or deployed by this task.
- Local `go vet ./...`, `go test -race ./... -count=1`, deployment regression tests (27 tests), shell syntax and full govulncheck source scan PASS. Source scan reports zero known vulnerabilities.
- Compatibility: CI requires module/DB network access and Docker on runner. An old Go dispatcher on the developer PATH caused scanner parsing errors; using the actual Go 1.26.11 bin on PATH resolves it. GitHub setup-go installs that exact toolchain directly.
- Remaining: OS image scan, local container execution, staging bind mounts and customer acceptance checks are release gates; no known Go dependency vulnerabilities remain.

## Stage 12 — tracked build artifact cleanup

- Untrack generated autotable/autotableGo binaries, CPU/heap/goroutine profiles and .DS_Store metadata. Add ignore rules for regenerated binaries/profiles and local tooling output. Keep source, fixtures and recorded baseline JSON.
- `git ls-files` confirms no tracked binaries/profiles/desktop metadata remain. Complete source suite PASS; `git diff --check` PASS.
- Compatibility: source builds remain the deployment path; binary files remain locally via `git rm --cached` and previous history is intact. No history rewrite or license addition.
- Remaining/release risks unchanged: zero known Go advisories; container runtime/OS scan, bind-mount permissions, signing-key rotation and monitoring configuration require staging/operator verification.

## Review fix — diagnostics credential lifecycle

- Independent review found deployment rsync could delete the operator's mounted Prometheus credential, and diagnostics key separation omitted the still-accepted previous service key.
- Reproduced both with failing tests. Exclude observability/secrets from release rsync and reject previous service credential reuse. Real local rsync regression now preserves the credential while deleting stale source.
- Complete Go suite PASS; deployment suite PASS (28 tests). No known new Go vulnerabilities; operator configuration/TLS requirements unchanged. No real secret files or remote deployment touched.

## Review fix — essential worker exit ownership

- Review identified that Fiber Listen can return before concurrent Shutdown completes, allowing main to exit successfully before the worker's nonzero exit.
- Publish an atomic failure flag before shutdown; main checks it before and after listening and owns the nonzero exit as well. Keep the worker fallback for startup-before-listen failures.
- Reproduced successful-exit race with a controlled shutdown regression, then PASS after correction; complete suite PASS. Normal shutdown remains successful. No new vulnerability findings or deployment configuration changes.

## Review fix — authenticated WebSocket subscription scope

- Critical pre-existing issue reproduced: a cookie authenticating one route/project could select a different subscription through query/header context. Independent local Redis/WebSocket regression demonstrated registration in the wrong tenant/project before the fix.
- Carry verified JWT IDs/slugs into a typed server-only websocket local. Resolve only authenticated slugs, require resolved IDs to match, and reject all conflicting legacy mirrors; matching query/header forms still work.
- Cross-project query/ID/mixed-context rejection, legitimate event delivery, route-scope and refresh-token rejection tests PASS. Complete suite PASS. No known Go dependency findings.
- Compatibility: clients supplying contradictory context are intentionally rejected; ordinary matching legacy fields and project cookie contracts remain supported. No production data access or migration.

## Review fix — WebSocket transport teardown

- Reproduced server eviction leaving the reader blocked because fasthttp's hijacked-connection Close may be a no-op.
- Capture underlying transport while registry owns the wrapper, expire net.Conn read/write deadlines, and signal teardown completion. Concurrent unregister callers wait for completion; the handler also waits for its writer before adapter pool reuse. No wrapper access occurs after teardown releases it.
- Real connection eviction regression PASS; authenticated subscription and eviction tests PASS for 10 race-enabled repetitions; complete suite PASS. No change to valid websocket messages or routes.
- Deployment risk: evicted/failed clients are disconnected promptly as intended and may reconnect through normal authentication. No production connection was touched.

## Review follow-up — remaining generated artifacts

- Confirmed retired Go plugin loader is commented out; untrack temp_Multiple.so/temp_Sum.so, Python bytecode and generated browser logs/screenshots. Ignore future regeneration and exclude shared objects/bytecode from Docker context.
- Retain earlier tracked .superpowers review patches as historical review evidence, not executable build output. No unrelated source/fixture cleanup.
- Tracked-artifact inventory and diff whitespace checks PASS; source-only full race suite PASS. Git history and local generated files remain intact. No license added, production data operation or deployment.

## Review follow-up — complete integration coverage and final verification

This final status supersedes earlier pending test statements.

- Broaden CI and the task-owned native Mongo runner to every integration-tagged package, with both workflow and collaboration test URIs. The runner verifies the fresh replica set's PID/dbpath before initialization and never uses production data. Includes the JWT-affected collaboration HTTP developer/editor/viewer journeys.
- Full task-owned `bash scripts/test-workflow-mongo-integration.sh -race` PASS across all integration packages, with no skips. Full `go test -race ./... -count=1` PASS; focused real WebSocket scope/eviction regressions PASS for ten race-enabled repetitions, including conflicting headers and missing authenticated context.
- `go vet ./...`, Linux amd64 CGO-disabled build, `go mod verify`, actionlint v1.7.12 for both workflows, shell syntax and deployment suite (28 tests) PASS.
- Official govulncheck v1.10.0 source and module scans both report **No vulnerabilities found** on Go 1.26.11. Baseline was 126 unique advisory findings; reachability/platform limitations are documented in the inventory.
- Independent review initially identified a critical pre-existing websocket subscription scope bug plus worker exit, websocket teardown and diagnostics credential lifecycle issues. All were reproduced and fixed with regressions in separate follow-up commits; the reviewer's original hold verdict applied to the earlier revision. No second independent review was represented as having occurred. Required cleanup and integration-coverage follow-ups were also handled.
- Branch/worktree retained for review; original `feat-boolean-switch` checkout and unfinished cherry-pick state untouched. Nothing pushed, merged or deployed. No open-source license added; no Git history rewritten.

Remaining release gates: build/run the pinned Docker image in CI or a running local engine, scan its OS packages, validate UID/GID 10001 permissions with actual staging mounts, provision independent JWT/diagnostics secrets and Prometheus token file, coordinate any signing-key rotation/customer reauthentication, and perform customer staging acceptance. External key provenance cannot be proved from a supplied string; the provided generator is cryptographic. These are documented limitations and operator actions, not a claim that production is ready.
