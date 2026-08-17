# Display Toggle Column Targets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let each display toggle hide selected static columns and generated relation groups while off and reveal them while on.

**Architecture:** Keep visibility bindings canonical on each target. Extend generated relation groups with the existing `ToggleBinding` shape, add pure Tenant Panel helpers that assign/remove bindings from a reverse target selector, and filter hidden generated groups before descriptor creation in both frontends.

**Tech Stack:** Go, MongoDB BSON, React, TypeScript, Vite, Vitest

## Global Constraints

- Selected targets use `visibilityToggle: { toggleId, when: true }`.
- Toggle on reveals selected targets; toggle off hides them.
- Static columns and generated groups may both be selected.
- Targets without bindings remain visible.
- Missing toggle references fail open.
- Existing Boolean edit behavior remains independent.
- The backend stores one canonical target-side binding.

---

### Task 1: Backend generated-group visibility contract

**Files:**
- Modify: `models/pageModel.go`
- Modify: `models/models_test.go`
- Modify: `models/frontendValidation.go`
- Test: `models/frontendValidation_create_test.go`

**Interfaces:**
- Consumes: existing `ToggleBinding` and `GeneratedRelationColumnsConfig`.
- Produces: `GeneratedRelationColumnsConfig.VisibilityToggle *ToggleBinding`.

- [ ] **Step 1: Write failing round-trip and validation tests**

Set `VisibilityToggle: &ToggleBinding{ToggleID: "showLocations", When: true}` in the existing page round-trip fixture and assert it survives BSON. Add an invalid generated-group case whose `VisibilityToggle.ToggleID` is `"missing"` and assert validation fails.

- [ ] **Step 2: Verify RED**

Run: `go test ./models -run 'TestPageTableTogglesRoundTrip|TestValidateTableComponentConfigRejectsInvalidGeneratedRelationColumns' -count=1`

Expected: compilation fails because generated groups do not expose `VisibilityToggle`.

- [ ] **Step 3: Implement model and validation**

```go
VisibilityToggle *ToggleBinding `bson:"visibilityToggle,omitempty" json:"visibilityToggle,omitempty"`
```

Validate it exactly like `BooleanEditToggle`, with an error naming `visibilityToggle`.

- [ ] **Step 4: Verify GREEN**

Run the focused command from Step 2, then `go test ./...`.

### Task 2: Shared frontend types and generated descriptor visibility

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/types/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/generatedRelationColumns.ts`
- Test: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/generatedRelationColumns.test.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/types/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/generatedRelationColumns.ts`
- Test: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/generatedRelationColumns.test.ts`

**Interfaces:**
- Consumes: `GeneratedRelationColumnsConfig.visibilityToggle?: ToggleBinding`, `isTableColumnVisible(binding, state, toggles)`.
- Produces: descriptor arrays containing only groups visible in current toggle state.

- [ ] **Step 1: Write failing descriptor tests in both projects**

```ts
expect(buildGeneratedRelationColumnDescriptors(
  [{ ...group, visibilityToggle: { toggleId: "editLocations", when: true } }],
  { location: [{ _id: "1", name: "Main" }] },
  { editLocations: false },
  [{ id: "editLocations", label: "Edit locations", defaultValue: false }],
)).toEqual([]);
```

Repeat with `editLocations: true` and expect one descriptor.

- [ ] **Step 2: Verify RED**

Run `yarn test src/utils/generatedRelationColumns.test.ts` in each frontend.

Expected: off-state assertion fails because descriptors are still produced.

- [ ] **Step 3: Add types and minimal filtering**

Add `visibilityToggle?: ToggleBinding` to generated group types and API types. At the start of each `groups.flatMap`, return `[]` when:

```ts
!isTableColumnVisible(group.visibilityToggle, toggleState, toggles)
```

- [ ] **Step 4: Verify GREEN**

Run the focused test in both projects.

### Task 3: Tenant cleaning and target assignment helpers

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/pageDesignerTableConfig.ts`
- Test: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/pageDesignerTableConfig.test.ts`

**Interfaces:**
- Produces: `DesignerVisibilityTarget = `column:${string}` | `group:${string}``.
- Produces: `getDesignerToggleVisibilityTargets(config, toggleId): DesignerVisibilityTarget[]`.
- Produces: `setDesignerToggleVisibilityTargets(config, toggleId, targets): TableComponentConfig`.

- [ ] **Step 1: Write failing helper tests**

Create a config with two static columns and two generated groups. Assert getter returns targets bound to the requested toggle. Assert setter:

```ts
expect(setDesignerToggleVisibilityTargets(config, "editLocations", [
  "column:active",
  "group:locations",
])).toMatchObject({
  columns: [
    { field: "active", visibilityToggle: { toggleId: "editLocations", when: true } },
    { field: "name" },
  ],
  generatedRelationColumns: [
    { id: "locations", visibilityToggle: { toggleId: "editLocations", when: true } },
    { id: "warehouses" },
  ],
});
```

Also assert deselection removes only bindings owned by `editLocations`, and assignment transfers a target from another toggle.

- [ ] **Step 2: Verify RED**

Run: `yarn test src/utils/pageDesignerTableConfig.test.ts`

Expected: imports fail because helpers do not exist.

- [ ] **Step 3: Implement helpers and cleaner**

Use exact keys `column:<field>` and `group:<id>`. Selected targets receive `{ toggleId, when: true }`; unselected targets lose the binding only when its current `toggleId` matches. Extend `cleanDesignerGeneratedRelationColumns` to preserve a cleaned `visibilityToggle`.

- [ ] **Step 4: Verify GREEN**

Run the focused test.

### Task 4: Tenant Display Toggle target UI

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/PageDesigner.tsx`

**Interfaces:**
- Consumes: getter/setter from Task 3.
- Produces: per-toggle “Visible columns and groups” checkboxes.

- [ ] **Step 1: Add target controls**

Inside each Display Toggle card, render a bordered section titled `Visible columns and groups`. Render checkboxes for configured static columns using `column:<field>`, and generated groups using `group:<id>`. Checked values come from `getDesignerToggleVisibilityTargets(tableConfig, toggle.id)`.

- [ ] **Step 2: Wire immutable updates**

On checkbox change, compute the next selected target list and call:

```ts
setTableConfig((current) =>
  setDesignerToggleVisibilityTargets(current, toggle.id, nextTargets),
);
```

Update toggle removal so generated-group `visibilityToggle` bindings are cleared along with edit bindings.

- [ ] **Step 3: Verify Tenant Panel**

Run `yarn test`, `yarn build`, and `git diff --check`.

### Task 5: Cross-project verification

**Files:**
- Verify only.

**Interfaces:**
- Consumes: Tasks 1-4.
- Produces: evidence for persistence, validation, editor configuration, preview behavior, and generated runtime behavior.

- [ ] **Step 1: Run backend verification**

Run: `gofmt -w models/pageModel.go models/models_test.go models/frontendValidation.go models/frontendValidation_create_test.go`

Run: `go test ./...`

- [ ] **Step 2: Run Tenant Panel verification**

Run: `yarn test`, `yarn build`, and `git diff --check`.

- [ ] **Step 3: Run React Template verification**

Run: `yarn test`, `yarn build`, and `git diff --check`.

- [ ] **Step 4: Confirm the requested flow**

Verify the same display toggle can be selected as a generated group's visibility target and Boolean edit toggle. On produces visible editable generated columns; off produces no generated columns.
