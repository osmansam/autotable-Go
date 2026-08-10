# Table Display Toggle Placement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let each table display toggle render in the upper or lower controls while keeping `Show Filters` as the rightmost upper control.

**Architecture:** Extend the shared frontend config with optional `isUpperSide`, preserve it through Tenant Panel cleaning/editor state, and resolve missing values to `true` in React Template. Both generic page variants will build display-toggle controls first and append `Show Filters` last through the existing `GenericTable` placement mechanism.

**Tech Stack:** React, TypeScript, Vite, Vitest, Tailwind CSS

## Global Constraints

- Placement is per toggle.
- `true` means upper; `false` means lower beside Excel.
- Missing values resolve to `true`.
- `Show Filters` is always upper and last among upper controls.
- Existing toggle state, effects, and pagination reset remain unchanged.

---

### Task 1: Tenant configuration contract

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/types/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/pageDesignerTableConfig.ts`
- Test: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/pageDesignerTableConfig.test.ts`

**Interfaces:**
- Consumes: `TableToggleConfig`, `cleanDesignerTableToggles(toggles)`.
- Produces: `TableToggleConfig.isUpperSide?: boolean`; cleaning preserves explicit false.

- [ ] **Step 1: Write the failing cleaning test**

```ts
expect(cleanDesignerTableToggles([{
  id: "lower", label: "Lower", defaultValue: false, isUpperSide: false,
}])).toEqual([{
  id: "lower", label: "Lower", defaultValue: false, isUpperSide: false,
}]);
```

- [ ] **Step 2: Verify RED**

Run: `yarn test src/utils/pageDesignerTableConfig.test.ts`

Expected: FAIL because cleaning drops `isUpperSide`.

- [ ] **Step 3: Implement the minimal contract**

```ts
// TableToggleConfig
isUpperSide?: boolean;

// cleanDesignerTableToggles result
...(toggle.isUpperSide !== undefined
  ? { isUpperSide: toggle.isUpperSide }
  : {}),
```

- [ ] **Step 4: Verify GREEN**

Run: `yarn test src/utils/pageDesignerTableConfig.test.ts`

- [ ] **Step 5: Commit**

```bash
git add src/types/page.ts src/utils/pageDesignerTableConfig.ts src/utils/pageDesignerTableConfig.test.ts
git commit -m "feat: preserve table toggle placement"
```

### Task 2: Tenant per-toggle editor

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/PageDesigner.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/pageDesignerTableConfig.ts`
- Test: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/pageDesignerTableConfig.test.ts`

**Interfaces:**
- Consumes: optional `isUpperSide` from Task 1.
- Produces: `createDesignerTableToggle(id)` with upper default and an editor checkbox.

- [ ] **Step 1: Write the failing factory test**

```ts
expect(createDesignerTableToggle("toggle2")).toEqual({
  id: "toggle2",
  label: "Display toggle",
  defaultValue: false,
  isUpperSide: true,
});
```

- [ ] **Step 2: Verify RED**

Run: `yarn test src/utils/pageDesignerTableConfig.test.ts`

Expected: FAIL because the factory is missing.

- [ ] **Step 3: Implement factory and UI**

```ts
export const createDesignerTableToggle = (id: string): TableToggleConfig => ({
  id,
  label: "Display toggle",
  defaultValue: false,
  isUpperSide: true,
});
```

Use the factory in `addTableToggle`. Add an `Upper side` checkbox whose checked value is `toggle.isUpperSide !== false` and whose change handler calls `updateTableToggle(toggleIndex, { isUpperSide: event.target.checked })`. Expand the editor grid for the added control.

- [ ] **Step 4: Verify Tenant Panel**

Run: `yarn test`

Run: `yarn build`

- [ ] **Step 5: Commit**

```bash
git add src/components/PageDesigner/PageDesigner.tsx src/utils/pageDesignerTableConfig.ts src/utils/pageDesignerTableConfig.test.ts
git commit -m "feat: configure display toggle placement"
```

### Task 3: React Template placement resolver

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/types/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/tableToggles.ts`
- Test: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/tableToggles.test.ts`

**Interfaces:**
- Consumes: persisted `TableToggleConfig.isUpperSide?: boolean`.
- Produces: `isTableToggleUpperSide(toggle: TableToggleConfig): boolean`.

- [ ] **Step 1: Write failing resolver tests**

```ts
expect(isTableToggleUpperSide({ ...toggle, isUpperSide: false })).toBe(false);
expect(isTableToggleUpperSide({ ...toggle, isUpperSide: true })).toBe(true);
expect(isTableToggleUpperSide(toggle)).toBe(true);
```

- [ ] **Step 2: Verify RED**

Run: `yarn test src/utils/tableToggles.test.ts`

Expected: FAIL because the resolver is missing.

- [ ] **Step 3: Implement resolver**

```ts
export const isTableToggleUpperSide = (toggle: TableToggleConfig): boolean =>
  toggle.isUpperSide !== false;
```

- [ ] **Step 4: Verify GREEN**

Run: `yarn test src/utils/tableToggles.test.ts`

- [ ] **Step 5: Commit**

```bash
git add src/types/page.ts src/utils/tableToggles.ts src/utils/tableToggles.test.ts
git commit -m "feat: resolve table toggle placement"
```

### Task 4: Mixed runtime placement and Show Filters ordering

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/components/panelComponents/FormElements/GenericPaginatedPage.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/tableToggles.ts`
- Test: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/tableToggles.test.ts`

**Interfaces:**
- Consumes: `isTableToggleUpperSide(toggle)`.
- Produces: `appendShowFiltersControl<T>(displayControls, showFiltersControl?)`; ordered controls for `GenericTable`.

- [ ] **Step 1: Write the failing order test**

```ts
expect(appendShowFiltersControl([upper, lower], showFilters)).toEqual([
  upper,
  lower,
  showFilters,
]);
expect(appendShowFiltersControl([upper], undefined)).toEqual([upper]);
```

- [ ] **Step 2: Verify RED**

Run: `yarn test src/utils/tableToggles.test.ts`

Expected: FAIL because the helper is missing.

- [ ] **Step 3: Implement helper and integrate both variants**

```ts
export const appendShowFiltersControl = <T>(
  displayControls: T[],
  showFiltersControl?: T,
): T[] =>
  showFiltersControl ? [...displayControls, showFiltersControl] : displayControls;
```

Map each display toggle with `isUpperSide: isTableToggleUpperSide(toggle)`. Append the optional `Show Filters` entry afterward with `isUpperSide: true`. Preserve the existing callbacks, including paginated reset-to-page-one behavior.

- [ ] **Step 4: Verify React Template**

Run: `yarn test src/utils/tableToggles.test.ts`

Run: `yarn test`

Run: `yarn build`

- [ ] **Step 5: Commit**

```bash
git add src/components/panelComponents/FormElements/GenericPaginatedPage.tsx src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx src/utils/tableToggles.ts src/utils/tableToggles.test.ts
git commit -m "feat: position table display toggles"
```

### Task 5: Cross-project verification

**Files:**
- Verify only; no intended production edits.

**Interfaces:**
- Consumes: Tasks 1-4.
- Produces: final test/build/diff evidence.

- [ ] **Step 1: Verify Tenant Panel**

Run in `/Users/osmansamilerdogan/Desktop/tenantPanel`: `yarn test`, `yarn build`, and `git diff --check`.

- [ ] **Step 2: Verify React Template**

Run in `/Users/osmansamilerdogan/Desktop/react-template`: `yarn test`, `yarn build`, and `git diff --check`.

- [ ] **Step 3: Inspect scoped diffs**

Confirm only planned placement/type/editor/runtime/test files changed and all unrelated working-tree changes remain untouched.

- [ ] **Step 4: Report behavior**

Report per-toggle placement, backward-compatible upper defaults, lower placement beside Excel, and rightmost upper `Show Filters`.
