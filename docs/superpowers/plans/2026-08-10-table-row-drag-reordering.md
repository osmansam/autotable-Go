# Table Row Drag Reordering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Configure dynamic tables to reorder visible rows by drag and drop and persist consecutive values in a selected integer field.

**Architecture:** Persist a small `table.drag` configuration, calculate reorder effects with a pure shared helper, and connect the existing `GenericTable` drag callback to the existing bulk dynamic update mutation. Paginated tables normalize only the current page using its global offset; unpaginated tables start at one.

**Tech Stack:** Go, MongoDB BSON, React, TypeScript, Vite, Vitest, TanStack Query

## Global Constraints

- Reordering affects only rows displayed on the current page.
- Dragging order 30 onto 10 shifts the intervening range rather than swapping two values.
- Visible rows normalize to consecutive integers after every drop.
- Missing order values are generated on the first drop.
- Paginated order starts at `(currentPage - 1) * rowsPerPage + 1`.
- Unpaginated order starts at 1.
- Self-drop and missing identities are no-ops.
- Dragging requires an enabled configuration and a selected integer field.
- A conflicting explicit sort disables dragging.

---

### Task 1: Backend drag configuration persistence and validation

**Files:**
- Modify: `models/pageModel.go`
- Modify: `models/models_test.go`
- Modify: `models/frontendValidation.go`
- Test: `models/frontendValidation_create_test.go`

**Interfaces:**
- Produces: `TableDragConfig { Enabled bool; OrderField string }`.
- Produces: `TableComponentConfig.Drag *TableDragConfig`.

- [ ] **Step 1: Write failing round-trip and validation tests**

Add `Drag: &TableDragConfig{Enabled: true, OrderField: "order"}` to the page table fixture and assert BSON round-trip equality. Add a validation case for `Enabled: true` with a blank `OrderField`.

- [ ] **Step 2: Verify RED**

Run: `go test ./models -run 'TestPageTableTogglesRoundTrip|TestValidateTableComponentConfig' -count=1`

Expected: compilation fails because `TableDragConfig` and `Drag` do not exist.

- [ ] **Step 3: Implement the minimal model and validation**

```go
type TableDragConfig struct {
    Enabled    bool   `bson:"enabled" json:"enabled"`
    OrderField string `bson:"orderField" json:"orderField"`
}
```

Add `Drag *TableDragConfig` to `TableComponentConfig`. Reject enabled drag settings whose trimmed order field is empty.

- [ ] **Step 4: Verify GREEN**

Run the focused command, then `go test ./...`.

### Task 2: Pure current-page reorder calculation

**Files:**
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/tableRowReorder.ts`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/tableRowReorder.test.ts`
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/tableRowReorder.ts`
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/tableRowReorder.test.ts`

**Interfaces:**
- Produces: `reorderCurrentPageRows<T>(rows, draggedRow, targetRow, orderField, startOrder): { rows: T[]; updates: Array<{ _id: string | number; updates: Record<string, number> }> }`.

- [ ] **Step 1: Write failing tests in both projects**

Cover:

```ts
const rows = Array.from({ length: 50 }, (_, index) => ({
  _id: String(index + 1),
  order: index + 1,
}));
const result = reorderCurrentPageRows(rows, rows[29], rows[9], "order", 1);
expect(result.rows.map((row) => row._id).slice(9, 12)).toEqual(["30", "10", "11"]);
expect(result.rows.map((row) => row.order)).toEqual(
  Array.from({ length: 50 }, (_, index) => index + 1),
);
```

Also cover downward movement, missing/non-contiguous values, `startOrder: 51`, self-drop, and missing identities.

- [ ] **Step 2: Verify RED**

Run `yarn test src/utils/tableRowReorder.test.ts` in each frontend.

- [ ] **Step 3: Implement minimal deterministic calculation**

Resolve identity from `_id`, falling back to `id`. Remove the dragged row and splice it at the target's original index; this puts it before the target when moving upward and after it when moving downward. Map the reordered array to `startOrder + index`. Return updates only where the existing numeric value differs.

- [ ] **Step 4: Verify GREEN**

Run the focused tests in both projects.

### Task 3: Tenant configuration editor and cleaning

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/types/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/pageDesignerTableConfig.ts`
- Test: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/pageDesignerTableConfig.test.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/PageDesigner.tsx`

**Interfaces:**
- Produces: `TableDragConfig { enabled: boolean; orderField: string }`.
- Produces: `cleanDesignerTableDrag(drag): TableDragConfig | undefined`.

- [ ] **Step 1: Write failing cleaner tests**

```ts
expect(cleanDesignerTableDrag({ enabled: true, orderField: " sortOrder " }))
  .toEqual({ enabled: true, orderField: "sortOrder" });
expect(cleanDesignerTableDrag({ enabled: true, orderField: " " }))
  .toBeUndefined();
```

- [ ] **Step 2: Verify RED**

Run: `yarn test src/utils/pageDesignerTableConfig.test.ts`.

- [ ] **Step 3: Add types, cleaner, hydration, and save behavior**

Add optional `drag` to table config types and API types. Preserve it in `cleanTableConfig` and hydrate it into editor state.

- [ ] **Step 4: Add Request-tab controls**

Add **Enable row dragging** and an **Order field** selector near constant sorting. Filter options to integer-compatible types: `int`, `integer`, `int32`, `int64`, and `autoIncrementId`. Clear `orderField` when drag is disabled.

- [ ] **Step 5: Verify Tenant configuration**

Run focused tests, then `yarn test` and `yarn build`.

### Task 4: React Template paginated runtime integration

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/types/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/components/panelComponents/FormElements/GenericPaginatedPage.tsx`

**Interfaces:**
- Consumes: Task 2 helper and `updateMultipleDynamicItem(updates)`.
- Produces: `GenericTable.isDraggable` and `onDragEnter` for valid drag config.

- [ ] **Step 1: Add runtime type**

Add optional `drag?: { enabled: boolean; orderField: string }` to `TableComponentConfig`.

- [ ] **Step 2: Build the drop callback**

Call `reorderCurrentPageRows(rows, dragged, target, orderField, (currentPage - 1) * rowsPerPage + 1)`. If updates are non-empty, call `updateMultipleDynamicItem(updates)`.

- [ ] **Step 3: Wire GenericTable**

Pass `isDraggable={dragEnabledWithoutConflictingSort}` and the callback. Ensure the configured order field ascending contributes to request sorting when there is no explicit conflicting sort.

- [ ] **Step 4: Verify**

Run the reorder tests, full tests, and `yarn build`.

### Task 5: Tenant preview and unpaginated runtime integration

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/panelComponents/FormElements/GenericPaginatedPage.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx`

**Interfaces:**
- Consumes: Task 2 helper, configured drag field, current visible rows, and existing bulk update mutations.
- Produces: current-page drag persistence for all generic table variants.

- [ ] **Step 1: Integrate Tenant paginated preview**

Use page offset and bulk-update only changed rows.

- [ ] **Step 2: Integrate unpaginated variants**

Use `startOrder = 1` and bulk-update only changed rows.

- [ ] **Step 3: Verify all projects**

Run `go test ./...`; run `yarn test`, `yarn build`, and `git diff --check` in both frontends.
