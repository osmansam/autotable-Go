# Workflow Result Cache Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Let public read-only workflows opt into complete-result Redis caching.

**Architecture:** Reuse DynamicCache and the existing Redis fill locks. Keys include a project revision, workflow definition, and complete execution inputs. Invalidation advances the project revision after content, metadata, and localization writes.

**Tech Stack:** Go, Fiber, MongoDB, go-redis, miniredis, existing Go tests.

**Spec:** `docs/superpowers/specs/2026-10-24-workflow-result-cache-design.md` (approved by user).

## Global Constraints

- `isRedisCached` defaults to false. `cacheTime` is minutes; zero uses configured default.
- Cache only successful public read-only manual workflows; authenticated and form-bound requests bypass caching.
- Preserve existing authentication middleware and response data shape.
- Redis failures fall back to execution. Production deployment is outside this local change.
- Work in the current clean feature checkout; the managed workspace marks Git metadata read-only, so no branch/worktree/commit operations.

## Review Focus

- Shared responses must never cross tenant, project, identity, page, language, or pagination boundaries.
- Writes hidden in nested branches or workflows must not be skipped through a cache hit.
- Partial workflow failures and canceled requests must never populate the cache.
- Invalidation must follow committed writes, including localization and container metadata updates.
- Simultaneous misses and Redis loss must leave workflow execution usable.

### Task 1: Project revision and write invalidation

**Files:** `utils/workflow_cache.go`, `utils/workflow_cache_test.go`, `utils/generateRedisKey.go`, `controllers/containerController.go`, `controllers/container_cache_test.go`, `repositories/project_localization_repository.go`, `repositories/localization_repository.go`, repository localization tests.

**Interfaces:** Produce `GetProjectWorkflowCacheVersion(context.Context, string, string) (int64, error)` and `InvalidateProjectWorkflowCache(context.Context, string, string) error`. Advance revisions with schema invalidation, container mutations, and successful localization changes.

- [x] Write tests for cross-schema invalidation, project isolation, Redis failure, and localization/metadata write invalidation.
- [x] Run targeted tests and verify missing revision behavior fails.
- [x] Add revision helpers and post-write integration. Keep existing schema-cache behavior intact.
- [x] Run `go test ./utils ./repositories ./controllers ./cache` and inspect results.

### Task 2: Opt-in cached workflow execution

**Files:** `models/dynamicWorkflow.go`, `services/workflow_result_cache.go`, `services/workflow_result_cache_test.go`, `services/dynamic_service.go`, `services/dynamic_workflow.go`, `services/dynamic_workflow_validation.go`.

**Interfaces:** Consume project revision helpers and existing DynamicCache/fill locks. Produce an eligibility/key helper and integrate lookup/fill around `ExecuteWorkflow` execution. Track partial failures through the payload.

- [x] Write a behavior test enabling cache through workflow JSON and expecting the second call's source to be `cache`; verify it fails before implementation.
- [x] Test expiry, definition/input/pagination/project/tenant isolation, null and numeric output shape, disabled workflows, authenticated and form-bound bypass, writes/nested unsafe steps, partial failures, Redis loss, and simultaneous fills.
- [x] Add model fields, TTL validation, a recursive read-only classifier, input/definition fingerprints, safe Redis lookup/fill, and result restoration.
- [x] Skip unsupported nested/external operations conservatively; permit proven read-only GET calls used by the page loader.
- [x] Run `go test ./services ./models ./cache` and inspect results.

### Task 3: Public contract, documentation, and final verification

**Files:** `controllers/workflow_audit_test.go`, workflow cache tests, `docs/workflow-result-cache.md`, design and plan progress.

**Interfaces:** Existing workflow route and envelope, with `source: cache` on hits and unchanged output data.

- [x] Exercise controller behavior and persisted JSON/BSON configuration; verify authentication remains before cache lookup.
- [x] Document one-minute configuration, eligibility/bypasses, project-wide invalidation, cache-miss behavior, and deployment requirements.
- [x] Run `gofmt`, `go test ./...`, and targeted `go test -race` for cache execution/revision tests.
- [x] Request one independent final review, address material findings with regression tests, and verify the resulting changes.
- [x] Deliver local changes, passing checks, and the exact settings needed to enable caching.
