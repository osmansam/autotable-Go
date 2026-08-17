# Unpaginated Table Compatible Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve all paginated-table capabilities compatible with schema all-items mode in Tenant Panel preview and React Template runtime.

**Architecture:** Add small shared pure helpers for constant-value behavior and extract runtime output publication from the paginated component. Extend the unpaginated prop contract and wire compatible controls without importing pagination or server-only request behavior.

**Tech Stack:** React 18, TypeScript, Vitest, React Query, Yarn 4.

## Global Constraints

- `GET /dynamic` remains schema-only, unpaginated, and unfiltered.
- Do not add pagination, outside server sort/search, or a new endpoint.
- Pipeline and workflow rendering remains paginated.
- Use tests before production edits and run full suites/builds in both frontends.

---

### Task 1: Shared constant-value behavior

**Files:**
- Create: both frontends `src/utils/tableConstantValues.ts`
- Create: both frontends `src/utils/tableConstantValues.test.ts`
- Modify: both `GenericUnpaginatedPage.tsx` files

**Interfaces:**
- `mergeTableConstantValues(editable, configured, tableConstants, callerConstants)` applies caller constants last.
- `omitTableConstantKeys(updates, tableConstants, callerConstants)` removes protected keys.

- [ ] Write failing tests for precedence and protected update keys.
- [ ] Run `yarn test src/utils/tableConstantValues.test.ts` in both frontends and verify missing-module failures.
- [ ] Implement the helpers and integrate them into unpaginated create/update form submission.
- [ ] Run focused tests and both builds.
- [ ] Commit each frontend with `feat: preserve all-items table constants`.

### Task 2: Shared table runtime output publisher

**Files:**
- Create: React Template `src/pageRuntime/TableOutputPublisher.tsx`
- Create: React Template `src/pageRuntime/TableOutputPublisher.test.ts`
- Modify: React Template paginated and unpaginated table components

**Interfaces:**
- `TableOutputPublisher({ componentId, outputs, state })` publishes through the current page runtime store.

- [ ] Write failing publisher lifecycle tests using the real output adapter/store boundary.
- [ ] Extract the current paginated publisher unchanged into the shared module.
- [ ] Use it from both components; unpaginated state includes rendered rows and selected IDs without page metadata.
- [ ] Run focused runtime tests and build.
- [ ] Commit with `feat: publish all-items table outputs`.

### Task 3: Compatible props and query identity

**Files:**
- Modify: both `GenericUnpaginatedPage.tsx` files
- Modify: Tenant Panel `src/pages/PagePreviewPage.tsx`
- Modify: React Template `src/components/DynamicPageSections.tsx`
- Modify/Test: both `src/utils/tableDataMode.test.ts` or focused forwarding helpers

**Interfaces:**
- Unpaginated props accept `constantFilter`, `customTitle`, and `dataBinding` in both frontends.
- React Template additionally accepts `componentId`, `outputs`, `resolvedParams`, and `sourceRevision`.

- [ ] Add failing pure prop-resolution/forwarding tests.
- [ ] Extend props and forward them from preview/runtime dispatch.
- [ ] Pass `sourceRevision` into `useGetDynamicItems`; include normalized compatible params in query identity without claiming backend filtering.
- [ ] Verify focused tests and builds.
- [ ] Commit Tenant Panel with `feat: forward all-items table properties` and React Template with `feat: complete all-items runtime properties`.

### Task 4: Export and compatible chrome

**Files:**
- Modify: React Template `GenericUnpaginatedPage.tsx`
- Test: focused pure control-resolution tests under `src/utils`

**Interfaces:**
- Authorized schema tables expose the existing `ExportModal` through `GenericTable.onExcelExport`.
- No pagination props or outside server search/sort props are passed.

- [ ] Write a failing control-resolution test for authorized export and absent pagination-only controls.
- [ ] Add the existing export modal/hook wiring to the unpaginated runtime component.
- [ ] Verify focused tests, full frontend suites, and both builds.
- [ ] Commit with `feat: export all-items tables`.

### Task 5: Final cross-project verification

- [ ] Run Tenant Panel `yarn test && yarn build`.
- [ ] Run React Template `yarn test && yarn build`.
- [ ] Restore any generated tracked build artifacts.
- [ ] Run `git diff --check` and confirm clean status in all repositories.
- [ ] Audit every acceptance criterion against committed diffs.
