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
