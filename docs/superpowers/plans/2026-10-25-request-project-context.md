# Request Project Context Reuse Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan. Track progress below; no commits or deployment are requested.

**Goal:** Avoid resolving the same URL project twice during a workflow cache hit without weakening tenant/project scope checks.

**Architecture:** Store successful URL resolution in private Fiber request locals keyed by tenant/project slugs. Both existing project-context entry points reuse it only during that HTTP request. `GetTenantAndProjectContext` still compares current authentication locals against the resolved IDs on every call. New requests still check the project is active; errors are never retained.

**Tech Stack:** Go, Fiber, existing MongoDB and Redis clients, miniredis, Mongo driver's mock transport.

**Spec:** User approved removing duplicate project lookups and reusing verified request context while preserving authentication. This extends no cache lifetime across requests and does not skip the initial live check or collection permissions.

## Constraints and review focus

- Preserve URL/token tenant and project mismatch rejection, including tokens/locals set after initial resolution.
- Keep query/local legacy fallback behavior.
- Resolution must not survive request boundaries, failure, or a change in URL tenant/project slugs.
- Both slug resolver entry points may share the verified URL identity, but their authentication responsibilities stay unchanged.
- No production database/config changes, deployment, shared metadata cache, or authentication changes in this task.
- Continue in the user's existing clean feature checkout; retain local changes for review.

### Task 1: Regression coverage and request-local reuse

**Files:** `utils/tenantContext.go`, new `utils/tenant_context_request_test.go`.
**Interfaces:** Existing `GetTenantAndProjectContext` and `GetTenantAndProjectFromSlugs` signatures remain unchanged; memoization key/type stay private.

- [x] Add tests for one active-project lookup across repeated resolution and both resolver entry points, cache misses, and current auth mismatch checks.
- [x] Add tests for new-request activity checks, retry after lookup failures, changed URL slugs, and legacy fallback (existing fallback tests retained).
- [x] Run targeted tests and confirm duplicate lookups fail before implementation.
- [x] Consolidate slug resolution and store only successful request-local URL resolution; reapply auth validation each call.
- [x] Run targeted tests and existing project-context tests.

### Task 2: Route-level validation and final verification

**Files:** workflow HTTP tests if practical; documentation of behavior in `docs/workflow-result-cache.md`.

- [x] Exercise real conditional middleware plus public workflow service with fixture metadata, mock Mongo cold lookup, and Redis to verify a cache hit still authorizes correctly and resolves project once.
- [x] Run `go test ./...`, targeted and full race checks, `go vet ./...`, Linux production build, and `git diff --check`.
- [x] Run a local benchmark with the same simulated database latency before/after; report measured limits without promising production timing.
- [x] Obtain one fresh-context final code review and address material issues.

## Progress

- Baseline `go test ./...` passed before editing code.
- RED: four resolver-pair tests observed two activity checks instead of one.
- GREEN: new and existing utils project-context tests pass.
- Rerouting regression caught Fiber's borrowed parameter strings; cloning memoized slug names fixed it without changing auth locals or URI handling.
- The shared cold lookup uses the existing global collection provider (same production collection) to exercise real Mongo query generation with mock driver transport. Background Redis population captures its client so it cannot accidentally use a later test/request's replaced global client.
- Before benchmark (same 3 ms simulated database read): 6.94 ms/request, 2 project lookups/request.
- Route validation uses the existing conditional-auth helper with fixture metadata and the real public workflow service; authenticated success stops before unrelated user-data loading. Existing service and controller tests retain their coverage.
- Full `go test ./... -count=1`, targeted race checks on utils/middleware/controllers/services, and `go vet ./...` passed.
- After benchmark: 3.82 ms/request, 1 project lookup/request (same simulated 3 ms read). This isolates context lookup overhead and does not predict end-to-end production latency.
- Final independent review: ready, no critical/important/minor findings. Reviewer independently reran targeted resolver/auth/fallback/workflow tests and whitespace checks.
- Full `go test -race ./... -count=1` and `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build` passed. No production data/configuration changed.
- Review scope excludes predictions of production latency from the simulated benchmark, unrelated existing auth/cache architecture, and deployment behavior. Production timing must be measured after deployment; this task leaves the verified local changes ready for that step.
