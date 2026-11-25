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
