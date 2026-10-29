# Tenant-shared External API Credentials Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Reuse encrypted external API credentials within a tenant through explicit project grants, including template-based project creation.

**Architecture:** Preserve legacy project credentials, add tenant ownership and grant records, and enforce grants during every workflow credential lookup. Template creation validates and maps credential references before persisting cloned workflows. Backend and frontend implementation can proceed independently against the API contract below.

**Tech Stack:** Go/Fiber/MongoDB; React/TypeScript/TanStack Query/Vitest.

**Spec:** `docs/superpowers/specs/2026-10-28-tenant-shared-external-api-credentials-design.md` (approved by the user's “move on”).

## Global Constraints

- Same-tenant sharing with explicit project access; no cross-tenant credentials.
- No plaintext or encrypted secrets in metadata, templates, browser persistence or project-create payloads.
- Legacy project credentials and workflow IDs remain compatible.
- Only live tenant owners/admins can create/promote shared credentials or change grants.
- Preserve host restrictions, expiration, revocation, redirect protection and tenant isolation.
- No hosted data changes, provider requests, deployment, pushes or unrelated edits.
- Work in the existing task checkouts and preserve unrelated state; leave implementation changes reviewable without committing or merging.

## Review Focus

- A project-only user must not obtain tenant authority by changing request paths or submitted IDs.
- Nested/inactive workflow credential references must be mapped without altering unrelated authored JSON.
- A clone failure or project deletion must remove its grants without deleting shared keys.
- A promotion or revocation racing with execution must not create broader access or persistently break the originating project.
- A failed project submission must retain the selected saved credential and visible form state without storing its secret.

## API Contract

Tenant base: `/api/v1/tenant/integrations/external-api-credentials`.

- `GET /`: `{data:{credentials:[...]}}`, including tenant credentials and legacy credentials eligible for promotion.
- `POST /`: existing external credential input; creates tenant scope, returns `{data:{credential:...}}`.
- `POST /:id/revoke`: global shared revocation.
- `POST /:id/promote`: converts an owned project credential to shared scope, preserving ID and original access.
- `PUT /:id/projects/:projectId`: idempotent grant, no body required.
- `DELETE /:id/projects/:projectId`: disconnect one project.
- Metadata extends the existing fields with `scope: "tenant" | "project"` and `grantedProjectIds: string[]`; `projectId` is optional for tenant scope. Secrets never appear.
- `GET /api/v1/tenant/projects/templates/:id/external-api-requirements`: `{data:{requirements:[{sourceCredentialId,name,hosts:string[],usages:string[]}]}}`. Generic names for foreign/global-template requirements; no foreign credential metadata.
- Project-create payload gains `externalAPICredentialIds?: string[]` and `externalAPICredentialBindings?: Record<string,string>` (source template ID → target tenant credential ID).
- Existing project credential routes list connected shared credentials but cannot globally revoke them.

### Task 1: Credential ownership, grants and management (backend)

**Files:** `models/externalAPICredential.go`, focused repository/controller files, `repositories/dynamic_repository.go`, `routes/integrationRoutes.go`, `routes/projectRoutes.go`, `routes/management_contracts.go`, `services/management_authorization.go`, corresponding tests and runtime credential resolution.

**Interfaces:** Preserve `GetExternalAPICredential(ctx, tenantID, projectID, credentialID)` as the runtime entry point. Add tenant lookup/grant/disconnect/promotion methods with tenant/project ownership enforcement. Use the API contract above for UI consumers.

- [x] Write and observe failing tests for granted/denied runtime use, legacy compatibility, tenant permission contracts, redaction, revocation and promotion.
- [x] Implement additive model/grants, unique grant index, tenant CRUD and promotion, and live runtime authorization.
- [x] Verify project users cannot globally revoke shared keys and deleting a project preserves shared keys.
- [x] Run focused repository/controller/service/route tests; keep full output in temporary logs.

### Task 2: Template requirements, binding and cleanup (backend)

**Files:** `controllers/projectController.go`, new focused template-credential helpers/tests, `controllers/project_cleanup.go`, template route/contracts and disposable Mongo tests.

**Interfaces:** Consume tenant credential management from Task 1; produce requirements endpoint and project-create fields in the shared API contract.

- [x] Write failing tests for all nested workflow branches, missing/unknown bindings, tenant mismatch, expired/revoked keys, host checks and unchanged unrelated workflow config.
- [x] Implement requirements/preflight before project insertion, explicit grants and binding-aware container cloning.
- [x] Test and implement rollback of partial grants and safe project deletion. Promotion must preserve original access with no unsafe intermediate state.
- [x] Demonstrate two authorized projects using one key against a local HTTP fixture, denied third-project use, one-project disconnect and shared revocation.
- [x] Run affected backend suites and disposable Mongo integration checks where supported.

### Task 3: Tenant management and project creation (Tenant Panel)

**Files:** new tenant External APIs view/components/hooks and tests; `src/utils/api/integration.ts`, `src/utils/api/project.ts`, `src/pages/ProjectsPage.tsx`, `src/pages/IntegrationsPage.tsx`, navigation constants/routes and corresponding tests.

**Interfaces:** Consume the exact API contract; keep credential secrets confined to transient credential form submission.

- [x] Write failing API/UI tests for shared scope, permissions, grants, promotion, template binding and failed-submit retry.
- [x] Add tenant External APIs navigation/view for owners/admins, with create, promote, project connection/disconnection and global revoke controls.
- [x] Update project Integrations to identify shared credentials and prevent project-wide controls from revoking tenant keys.
- [x] Add credential selection and inline explicit key creation to Create Project; fetch template requirements, clear stale bindings on template changes, and send only selected IDs/bindings.
- [x] Run focused frontend tests and type/build checks.

### Task 4: Integration, review and delivery

- [x] Check backend/frontend contract parity and document endpoint usage, promotion semantics and template compatibility.
- [x] Run appropriate full backend suites and Tenant Panel tests/build, investigate failures without weakening assertions.
- [x] Obtain a fresh review of the implementation against the approved design, especially the five Review Focus cases.
- [x] Fix important findings with regression coverage and rerun affected checks.
- [x] Report implementation, verification evidence and any external verification limits; do not deploy or alter hosted credentials.

## Execution ledger

- Ruling: User approved implementation with “move on”; proceed without another approval round. Use parallel backend/frontend workers with exclusive repository ownership and a fixed API contract.
- Ruling: Keep changes in the existing clean feature checkouts and uncommitted for review; no merges, pushes or worktree changes are needed for this coordinated local task.

## Final verification

- `go test ./...`: passed after the cleanup fixes (`/private/tmp/shared-api-final-go-root.log`).
- `bash scripts/test-workflow-mongo-integration.sh`: 28 integration tests passed, including four shared-credential lifecycle scenarios (`/private/tmp/shared-api-final-mongo-root.log`).
- Tenant Panel `npm test`: 643 tests across 117 files passed, no type errors (`/private/tmp/tenant-full-test-final3.log`).
- Tenant Panel `npm run build`: passed; existing bundle/Browserslist/icon warnings remain (`/private/tmp/tenant-build-final3.log`).
- Changed frontend production-file ESLint and both repositories’ whitespace checks passed.
- Browser fixtures verified tenant key promotion, connecting another project, template requirements blocking submission until bound, inline key creation and successful mobile project creation. Captured project payload contains only IDs/bindings, no secret.
- Independent review found two P2 issues (actionable nested errors and rollback cleanup failure); both fixed with RED→GREEN regressions and rereviewed. No remaining important findings.
- No commits, pushes, deployments, hosted data writes or real provider calls were performed.
