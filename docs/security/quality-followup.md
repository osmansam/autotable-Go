# Quality/release gate follow-up — 2026-11-24

Continues P0 branch from 7c54e11. No monolith refactoring, application dependency upgrades, license changes, production-data operations, deployment or history rewriting.

## Stage 1 — cleanup and reproducible module graph

- Removed tracked controller backup and generated temp_Sum/temp_Multiple source. Existing arithmetic handler examples are retained only as test fixtures; no production route registered them.
- Removed four unreachable WebSocket helper functions left by authentication hardening. Live authentication, registration and teardown remain unchanged.
- `go mod tidy -diff` initially failed on fasthttp/websocket's direct classification; tidy now passes with no version/checksum change.
- Full race suite PASS before and after cleanup. No remaining known Go advisories from the previous final scan; a fresh scan is required at the end of this follow-up. Docker remains stopped; release/operator limits still apply.

## Measured baseline

Package-local statement coverage: 53.0%. Cross-package coverage (`-coverpkg=./...`): 54.3%, correctly counting security middleware exercised by root request tests. Staticcheck v0.8.2 (Go >=1.26) is pinned from official module metadata. Existing legacy error text must not change merely for style, and Mongo/telemetry API migrations are outside this follow-up's architectural scope; exact counted findings will be documented rather than disabling whole lint categories.

## Stage 2 — lint-discovered malformed delete validation

Staticcheck exposed a discarded ObjectID parse error in container deletion: a malformed ID could reach database initialization, with the parse error overwritten. The existing intended invalid-ID response was unreachable. Reproduced in an isolated subprocess with an explicit unreachable loopback URI; fixed by returning the existing 400/message before initializing/querying Mongo. Valid delete behavior is unchanged. Regression and complete race suite PASS. No real database was accessed; no dependency versions changed.

## Stage 3 — actionable lint findings

Removed unreachable private helpers/constant and unused test helpers; removed a redundant nil check and a transaction-client assignment always overwritten before use. Replaced a self-comparison test with a fixed independently derived SHA-256 fixture. Retained three intentional nil-context compatibility tests with narrowly documented Staticcheck annotations. No API messages or routes changed; the monoliths were not split or restructured.

Full race suite PASS. Staticcheck now reports only 79 ST1005 legacy capitalization findings and 31 SA1019 deprecated API usages. They will be exact-count/source baselined; no unused-code, discarded-error or other new findings are permitted. Deprecations and legacy style remain explicit technical debt, not a claim of a pristine lint result.

## Stage 4 — enforced quality and coverage gates

`make check` now runs vet, tidy-diff, pinned Staticcheck v0.8.2 with an exact-source/message/count baseline, full race tests with cross-package coverage, Python tooling regressions, and pinned govulncheck source/module scans. CI uses the same target and retains coverage/lint reports. No lint category is globally disabled; only the 110 reviewed ST1005/SA1019 findings are retained, and new, changed, duplicate or stale entries fail the gate. Only those two legacy codes may be baselined.

Measured weighted statement coverage is 54.28%, with a 54% total floor and no source exclusions. Critical file floors are JWT parsers 90%, key validation 95%, and diagnostics/recovery/panic guard/WebSocket context 100%. This is a regression floor, not an assertion that overall coverage is high. The checker merges duplicate cross-package coverage blocks and fails closed on empty/malformed/missing critical records; percentages are weighted from raw statements rather than rounded display values.

`make check` PASS, including 35 Python tests (seven gate regressions plus 28 deployment tests), full race suite, zero new Staticcheck findings, clean tidy and zero known Go vulnerabilities. Workflow validation and diff checks PASS. Makefile contains local build/run/integration/quality targets; onboarding and executable image/staging gates follow separately. Tool/report outputs are ignored and excluded from Docker context.

## Stage 5 — onboarding/configuration documentation

Added root README and secret-free, trackable .env.example. Documented Go/native Mongo/Redis prerequisites, local replica-set isolation, config precedence and JSON-owned Redis/CORS settings, startup/index writes, required and optional feature credentials, diagnostics, cleartext OTLP limits, all Make targets, measured coverage/lint debt and manual release boundaries. No automatic local or production service was started.

Actual godotenv parsing of the example PASS; required credential fields are empty; README local links, Make help/target resolution and whitespace checks PASS. Existing quality gate commands are the same commands documented, already verified in Stage 4. Keys remain operator-provisioned; Docker, staging mount, ingress and customer acceptance checks are not represented as completed.

## Stage 6 — executable image/runtime/OS and mount gates

Added shared `make image-check`, `make image-scan`, and `make staging-mounts` commands. CI builds the pinned image, checks UID/GID 10001 and writable temp/log/read-only binary/config in a network-isolated shell, then scans its exported archive for OS HIGH/CRITICAL vulnerabilities. Scanner gets no Docker socket or app secrets. Trivy v0.76.2's official image is digest-pinned from Docker Hub (`sha256:8f6bb947dbc528902e11f2f5be52f7b519f6a39d5d148f41166f2040499768fb`); its Go 1.27 requirement stays inside that tool container and does not upgrade application Go. CLI options verified against official Trivy documentation.

Scan errors/findings, EOL OS, missing/freshly invalid OS reports, runtime permission failures and missing Docker fail closed. No ignore-unfixed suppression or repository ignore file is used. The staging mount gate requires existing explicit absolute directories and reads access metadata without chown, mkdir or writing customer files. It does not replace real upload/log acceptance.

Six controlled Docker-transport regression tests, shell and workflow validation PASS. **Real local `make image-check` fails with Docker engine unavailable**; image build/runtime/OS scan have NOT passed locally. OrbStack remains stopped and its general start resumes unrelated machines, so it was not started. Actual image/scan results must come from CI or a running appropriate test engine; no clean OS report is claimed. No deployment or production volume was accessed.

## Stage 7 — read-only key and staging ingress validation

Added `make config-check` / `autotable-secrets --check` to validate JWT, diagnostics and optional encryption-key formats without generating or printing credentials, changing files, rotating sessions or connecting databases. Added staging HTTPS health/readiness/frontend checks with system certificate validation and same-origin HTTPS-only redirects. The API and frontend must provide nosniff/HSTS (180-day minimum); the frontend must provide enforced minimum CSP directives. Checks do not impose runtime headers or a frame policy on the application/Swagger/customer embeds and do not claim a full CSP or customer-flow audit.

Added explicit-variable `make ingress-check` and a verification-only `make release-check`; documented real mount, key-rotation and acceptance prerequisites. Before the final expanded regressions, `make check` PASS (54.30% weighted coverage, unchanged critical floors, no new lint findings, 45 Python tests, zero Go advisory findings). Broader image report fixtures and redirect tests also PASS. Full isolated integration runner PASS across all tagged packages after source cleanup/fix.

No real staging configuration, URLs or credentials were supplied, so actual key/mount/ingress/customer acceptance gates were not executed. Docker remains stopped; the recorded real image attempt failed as a prerequisite error. These operator/runtime checks remain release blockers until run against the intended staging environment. Go 1.26.11 compatibility, manual-only deployment and customer API contracts are preserved.

## Review fix — side-effect-free key checks and credential-safe initialization

Independent review found the new CLI's import of utils could initialize Cloudinary before validation/generation and expose credential-bearing URL parse errors. Reproduced both CLI exposure and the pre-existing application initializer exposure with subprocess tests using synthetic markers (no real credentials). Moved shared external-key decoding/validation to a small initializer-free internal package while preserving the utils public validator/error identity and encryption semantics. CLI no longer imports utils; Cloudinary startup failure now uses a generic credential-free panic.

Both regressions and full `make check` PASS, including coverage floors, exact lint baseline, race/tooling tests and zero known Go findings. No database/cloud calls, credential rotation or environment mutation occurred. Review findings on shell predicates and ingress parsing are addressed separately.

## Review fix — mandatory shell assertions and complete OS inventory

Review identified that `sh -e` does not stop on a failed left side of `&&`, and that recognized OS metadata alone did not prove a package scan occurred. New regressions execute the actual emitted shell probes under controlled failed UID/GID/access predicates (not just inspect Docker transport arguments); they failed on the old script. Every required assertion is now independent. OS reports now require named/versioned packages matching the detected distribution, reject EOL/missing severities, and still fail HIGH/CRITICAL findings and tool errors. Positive report fixtures now include real package inventory shape.

Ten release-gate regressions and shell validation PASS. Parallel release verification waits for image/scan completion before mount and ingress checks. Documentation now accurately calls AUTOTABLE_CHECK_IMAGE a build output tag that is replaced, not an existing-image inspection option. These checks still have not run against real containers/staging while Docker is stopped; no OS cleanliness or mount acceptance is claimed.

## Review fix — browser-effective HSTS and explicit CSP sources

Review reproduced malformed HSTS fields (unbalanced quotes/repeated malformed max-age) and wildcard CSP hosts being accepted. Replaced substring matching with parsing of the entire first STS field: balanced quoted values, valid directive syntax, single case-insensitive directives, numeric max-age and valueless includeSubDomains. Behavior follows RFC 6797 section 6.1 (https://www.rfc-editor.org/rfc/rfc6797.html#section-6.1). Unknown syntactically valid extensions remain allowed; no includeSubDomains/preload policy is imposed.

Default CSP sources now reject wildcard hosts and unrestricted scheme-only values while preserving explicit approved HTTPS hosts and frontend-specific policies. Seven ingress regression tests cover malformed/duplicate STS, first-field handling, quote/extension parsing, CSP host/scheme wildcards, report-only policy and redirect/TLS constraints. They failed before correction and PASS after. Full `make check` PASS, including 52 Python regressions, unchanged security coverage floors, 110 exact legacy lint findings and zero known Go findings. These are offline policy checks; actual staging ingress and browser acceptance remain unexecuted until supplied.

## Final verification and remaining release conditions

Final committed implementation passes:

- `make check`: full Go race/coverage suite, vet/tidy, exact Staticcheck accounting, 52 Python regressions, and govulncheck source/module scans with no known Go findings.
- Weighted statement coverage 54.31% (54% overall regression floor); JWT parsers 90.91%/91.23%, key validation 96.97%, and diagnostics/recovery/panic guard/WebSocket context 100%. Overall coverage remains modest; the 79 legacy capitalization and 31 deprecation findings remain explicit debt.
- `make integration`: every integration-tagged package against a fresh task-owned loopback Mongo replica set, race-enabled; no production URI or data used.
- `make build`, `go mod verify`, `go mod tidy -diff`, actionlint, shell syntax, source/diff and documentation checks.

Independent review requested changes on the earlier follow-up revision for five important issues: credential-bearing initialization, shell predicate failures, empty OS inventory, malformed HSTS and CSP wildcard defaults. Each was reproduced and corrected with regressions. Parallel release ordering and output-tag documentation were also corrected. The review's old verdict is not represented as a second independent approval of the fixes.

Final real `make image-scan` attempt exits nonzero with Docker engine unavailable. No local image build/runtime/OS scan result is claimed, and no engine or unrelated VM was started. CI now enforces the executable image/OS checks; no workflow was remotely dispatched by this task. Real staging mounts, TLS/CSP responses, credential provisioning/rotation and customer acceptance remain explicit operator release conditions.

The original checkout/cherry-pick state is untouched. Follow-up commits stay on security/p0-hardening in the isolated worktree. No changes were pushed, merged or deployed; no production data/credentials were changed, no license was added and Git history remains intact. Architecture/monolith splitting remains out of scope.

## CI follow-up — actionable OS scan failures (2026-11-25)

The branch was subsequently pushed. [CI run 45648525018](https://github.com/osmansam/autotable-Go/actions/runs/45648525018) built the real image and passed its isolated non-root permission probe, then correctly rejected five HIGH package findings in Alpine 3.24.1. Its uploaded JSON identifies OpenSSL CVE-2026-14456 (libcrypto3/libssl3), musl CVE-2026-107891 (musl/musl-utils), and zlib CVE-2026-85091. This confirms the build/probe only; the OS gate did not pass.

Previously, shell fail-fast stopped before reporting findings to the console. The scanner status is now captured so the report can print each HIGH/CRITICAL advisory, package, installed version and fixed version before rejecting it. Missing/invalid reports, EOL distributions and scanner errors still fail closed, including scanner errors with otherwise clean reports. No severities, ignore policy or release requirements were relaxed.

Both new diagnostic regressions failed before the fix and pass after it; all 12 container gate regressions, shell syntax and diff checks pass. The full existing Go race suite passes before the separate runtime dependency update. These diagnostic changes do not resolve the five OS findings; remediation is a separate commit. Go compatibility, application behavior and deployment requirements are unchanged. Local Docker remains stopped; no application or production data was accessed.

## CI follow-up — runtime remediation and verification status

Separate commits `4347888` (diagnostics) and `5692b45` (runtime remediation) were pushed to `origin/security/p0-hardening`. The runtime now pins the verified official Alpine 3.24.4 digest and applies stable-branch package upgrades; advisory/fixed-version evidence and Go compatibility are recorded in `p0-advisories.md`. No application code, Go dependencies, API contracts or deployment configuration changed in this follow-up.

Full existing Go race tests pass before and after the runtime dependency group. Final `make check` passes: vet/tidy, exact Staticcheck accounting, race/coverage (54.31%), all 54 Python tooling regressions and both govulncheck scans with no known Go vulnerabilities. Shell syntax and diff checks pass.

Real updated-image verification is pending: the local Docker engine is stopped. Dispatching verification-only `ci.yml` on the hardening branch failed with GitHub HTTP 500; a single retry returned an incomplete JSON response, and no new run appeared. The production workflow was not dispatched. The five findings in the old image have verified patched targets, but zero remaining OS findings cannot be claimed until the updated image builds and its unchanged HIGH/CRITICAL/EOL gate passes. Staging volume access, ingress policy, signing-key rotation and customer acceptance remain operator release conditions. Nothing was merged or deployed, no production data was modified, and no license was added.
