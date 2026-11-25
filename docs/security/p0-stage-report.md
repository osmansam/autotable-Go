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
