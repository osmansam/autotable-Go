# Table Bulk Actions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make table bulk edit and bulk delete configurable from PageDesigner and persisted in page table configuration.

**Architecture:** Add a `bulkActions` object beside `addButton`, `actions`, and `filterPanel`. The editor writes `bulkActions.edit` and `bulkActions.delete`; runtimes read those configs to decide visibility, labels, fields, and optional workflow submit behavior while preserving built-in bulk endpoints as the default.

**Tech Stack:** Go page model validation, React/TypeScript tenantPanel designer/runtime, React/TypeScript react-template runtime.

## Global Constraints

- Keep existing saved pages backward compatible: absent `table.bulkActions` means current bulk edit/delete behavior stays enabled.
- Use existing `TableActionConfig` and `ActionConfig` shapes rather than creating a second action schema.
- Direct bulk update/delete endpoints remain the fallback when no workflow submit is configured.
- Store selectable dynamic functions in config fields, but do not invent a runtime function execution endpoint.

---

### Task 1: Backend Schema And Validation

**Files:**
- Modify: `models/pageModel.go`
- Modify: `models/frontendValidation.go`
- Test: `models/frontendValidation_create_test.go`

**Interfaces:**
- Produces: `TableBulkActionsConfig` with `Edit *ActionConfig` and `Delete *ActionConfig`.
- Produces: `TableComponentConfig.BulkActions *TableBulkActionsConfig`.

- [ ] Add failing validation test that an invalid bulk action kind is rejected.
- [ ] Add Go model fields.
- [ ] Validate both configured bulk actions with `ValidateActionConfig`.
- [ ] Run focused Go tests.

### Task 2: Shared Frontend Types And Cleaning

**Files:**
- Modify: `../tenantPanel/src/types/page.ts`
- Modify: `../tenantPanel/src/types/layout.ts`
- Modify: `../react-template/src/types/page.ts`
- Modify: `../react-template/src/types/layout.ts`
- Modify: `../tenantPanel/src/components/PageDesigner/PageDesigner.tsx`

**Interfaces:**
- Produces: `TableBulkActionsConfig` with optional `edit` and `delete` table action configs.
- Produces: clean designer serialization for `table.bulkActions`.

- [ ] Add TypeScript bulk action types.
- [ ] Add `bulkActions` to table config interfaces.
- [ ] Extend `cleanTableConfig` to serialize configured bulk actions.

### Task 3: Designer UI

**Files:**
- Modify: `../tenantPanel/src/components/PageDesigner/PageDesigner.tsx`

**Interfaces:**
- Consumes: `tableConfig.bulkActions`.
- Produces: editable bulk edit/delete enable flags, labels, icons, form fields, and workflow/function submit names.

- [ ] Add `Bulk Actions` table settings tab.
- [ ] Add update helpers for `bulkActions.edit`, `bulkActions.delete`, and bulk edit fields.
- [ ] Render controls following existing action editor patterns.

### Task 4: Runtime Behavior

**Files:**
- Modify: `../tenantPanel/src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx`
- Modify: `../react-template/src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx`

**Interfaces:**
- Consumes: `tableConfig.bulkActions`.
- Uses: `useDynamicCrud().executeWorkflow`.

- [ ] Hide selection actions when their config is disabled.
- [ ] Use configured bulk edit fields when present.
- [ ] Submit to configured workflow when `submit.workflowName` is set; otherwise use existing bulk endpoints.

### Task 5: Verification

**Commands:**
- `go test ./models`
- `yarn test` or `yarn build` in each frontend project if local dependencies are present.

