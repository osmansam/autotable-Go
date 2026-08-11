# Table Additional Data Fields Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Return non-column row fields required by action conditions and custom table logic without rendering those fields as columns.

**Architecture:** Persist explicit `dataFields` on table configuration, then extend the shared frontend field-projection resolver to union explicit fields with fields discovered from filter inputs, actions, overrides, rows, and toggles. Paginated table bindings send that union as `fields`; rendering continues to use only `columns`.

**Tech Stack:** Go models and validation; React 18 and TypeScript; Vitest; Yarn 4.

## Global Constraints

- Preserve all pre-existing uncommitted changes in all three repositories.
- `columns` remains presentation-only; do not add a hidden-column flag.
- `constantFilters` remains independent of returned projection fields.
- Explicit `dataFields` is trimmed, deduplicated, and never rendered automatically.
- Known condition fields are discovered using the existing condition parser and available schema field names.
- Backend field authorization remains authoritative.
- Paginated and unpaginated configurations remain compatible in both frontend projects.

---

### Task 1: Persist and validate backend `dataFields`

**Files:**
- Modify: `models/pageModel.go`
- Modify: `models/frontendValidation.go`
- Modify: `models/models_test.go`

**Interfaces:**
- Produces: `TableComponentConfig.DataFields []string` serialized as `dataFields` in BSON and JSON.
- Produces: `ValidateTableComponentConfig` rejects blank entries and duplicate trimmed names.

- [ ] **Step 1: Write failing round-trip and validation tests**

Extend a table configuration round-trip fixture with:

```go
DataFields: []string{"status", "internalCategory"},
```

Assert both values survive BSON decoding. Add table-driven validation cases for `[]string{"status", " "}` and `[]string{"status", " status "}`, expecting precise blank and duplicate errors. The production mutations caught are omitting persistence or accepting ambiguous configuration.

- [ ] **Step 2: Run the focused tests and verify RED**

Run:

```bash
GOCACHE=/private/tmp/autotable-data-fields-go-cache go test ./models -run 'TestPageTableAdditionalDataFieldsRoundTrip|TestValidateTableComponentConfigDataFields' -count=1
```

Expected: compilation fails because `DataFields` does not exist.

- [ ] **Step 3: Add the model property and minimal validation**

Add beside `Columns`:

```go
DataFields []string `bson:"dataFields,omitempty" json:"dataFields,omitempty"`
```

In `ValidateTableComponentConfig`, trim each entry for comparison, reject blank values with `table dataFields requires non-empty fields`, and reject duplicates with `table dataFields field '<name>' is duplicated`.

- [ ] **Step 4: Format and run model tests**

Run:

```bash
gofmt -w models/pageModel.go models/frontendValidation.go models/models_test.go
GOCACHE=/private/tmp/autotable-data-fields-go-cache go test ./models -count=1
```

Expected: PASS.

---

### Task 2: Extend the shared field-projection resolver in Tenant Panel

**Files:**
- Modify: `../tenantPanel/src/types/page.ts`
- Modify: `../tenantPanel/src/utils/api/page.ts`
- Modify: `../tenantPanel/src/utils/tableConfig.ts`
- Modify: `../tenantPanel/src/utils/tableConfig.test.ts`

**Interfaces:**
- Adds `dataFields?: string[]` to both designer/runtime table interfaces.
- Extends `getTableDataFieldNames(tableConfig, availableFieldNames)` without changing its return type.

- [ ] **Step 1: Write failing field-resolution tests**

Add literal tests proving:

```ts
expect(getTableDataFieldNames({
  columns: [{ field: "name" }],
  constantFilters: { status: "ACTIVE" },
  dataFields: ["internalCategory", "status", "status"],
  filterPanel: { inputs: [{ formKey: "owner", type: "text" }] },
  actions: [{
    kind: "edit",
    disabledCondition: "status != 'ACTIVE'",
    hiddenCondition: "owner == 'system'",
    requiredCondition: "approved == true",
    fieldOverrides: [{ field: "price", disabledCondition: "locked == true" }],
  }],
}, ["_id", "name", "status", "internalCategory", "owner", "approved", "locked", "price"]))
  .toEqual(["name", "owner", "status", "approved", "locked", "internalCategory"]);
```

Add a separate assertion that the returned fields do not mutate or append to `columns`. The production mutations caught are requiring hidden columns, missing action conditions, requesting keywords, or returning duplicates.

- [ ] **Step 2: Run the focused test and verify RED**

Run: `yarn test src/utils/tableConfig.test.ts`

Expected: FAIL because explicit data fields and action/filter consumers are absent.

- [ ] **Step 3: Implement consumer-driven field discovery**

Reuse `extractConditionFieldCandidates`. Add a local `addKnownField` helper that trims, checks `availableFieldNames` when supplied, and inserts into the existing `Set`.

Add fields in this stable order:

1. existing columns and column behavior;
2. filter-panel form keys;
3. action conditions and field-override conditions for Add, row actions, and bulk actions;
4. row class conditions;
5. toggle condition/effect fields supported by current toggle types;
6. explicit `dataFields`.

Do not add `constantFilters` keys solely because they filter the server; they are included only if another row consumer or explicit `dataFields` requires their returned values.

- [ ] **Step 4: Run focused tests and build**

Run:

```bash
yarn test src/utils/tableConfig.test.ts
yarn build
```

Expected: PASS.

---

### Task 3: Mirror resolver behavior in React Template

**Files:**
- Modify: `../react-template/src/types/page.ts`
- Modify: `../react-template/src/utils/tableConfig.ts`
- Modify: `../react-template/src/utils/tableConfig.test.ts`

**Interfaces:**
- Matches the Task 2 `dataFields` and `getTableDataFieldNames` contract exactly.

- [ ] **Step 1: Copy the behavioral tests with project-appropriate imports**

Use the same literal configuration and expected ordered field list as Tenant Panel. Add a test showing a `status` action condition requests status while the only rendered column remains `name`.

- [ ] **Step 2: Run the focused test and verify RED**

Run: `yarn test src/utils/tableConfig.test.ts`

Expected: FAIL because React Template has the old resolver.

- [ ] **Step 3: Implement the identical resolver contract**

Mirror the focused helper logic from Tenant Panel rather than creating component-local projection rules. Retain all existing array-source, nested-row, generated-relation, progress, template, and drag behavior.

- [ ] **Step 4: Run focused tests and build**

Run:

```bash
yarn test src/utils/tableConfig.test.ts
yarn build
```

Expected: PASS.

---

### Task 4: Add Tenant Panel “Additional data fields” authoring

**Files:**
- Modify: `../tenantPanel/src/utils/pageDesignerTableConfig.ts`
- Modify: `../tenantPanel/src/utils/pageDesignerTableConfig.test.ts`
- Modify: `../tenantPanel/src/components/PageDesigner/PageDesigner.tsx`

**Interfaces:**
- Produces: `cleanDesignerTableDataFields(fields?: string[]): string[] | undefined`.
- Tenant Panel Request tab edits `tableConfig.dataFields` through a schema-backed multi-select.

- [ ] **Step 1: Write failing cleaner tests**

Assert:

```ts
expect(cleanDesignerTableDataFields([" status ", "status", "", "owner"]))
  .toEqual(["status", "owner"]);
expect(cleanDesignerTableDataFields([])).toBeUndefined();
```

Also extend table hydration/cleaning coverage to prove `dataFields` survives editing and saving without becoming columns.

- [ ] **Step 2: Run the focused test and verify RED**

Run: `yarn test src/utils/pageDesignerTableConfig.test.ts`

Expected: FAIL because the cleaner does not exist.

- [ ] **Step 3: Implement the cleaner and persistence**

Implement trimming and first-occurrence deduplication. In `cleanTableConfig` within `PageDesigner.tsx`, persist `dataFields` only when the cleaner returns a non-empty list. Include existing `dataFields` when hydrating an edited table.

- [ ] **Step 4: Add the Request-tab selector**

Below Constant Filters, render an **Additional data fields** multi-select using the bound container’s complete field list, excluding `_id` only if the existing selector conventions do so. Selected values update `tableConfig.dataFields` without modifying `tableConfig.columns`.

Helper copy:

> Fetch these values with every row without displaying them as table columns. Fields referenced by known action conditions are added automatically.

- [ ] **Step 5: Run focused tests and build**

Run:

```bash
yarn test src/utils/pageDesignerTableConfig.test.ts src/utils/tableConfig.test.ts
yarn build
```

Expected: PASS.

---

### Task 5: Cross-project request and rendering verification

**Files:**
- Modify only implementation or tests required by a demonstrated parity failure.

**Interfaces:**
- Verifies `docs/superpowers/specs/2026-08-11-table-additional-data-fields-design.md` end to end.

- [ ] **Step 1: Verify paginated binding construction in both frontends**

Add or extend tests around table field projection showing that the binding passed to the table-source query contains `status` when it is referenced only by `disabledCondition`. Assert `columns` remains `[{ field: "name" }]`.

- [ ] **Step 2: Run complete backend verification**

Run with loopback permission:

```bash
GOCACHE=/private/tmp/autotable-data-fields-go-cache go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 3: Run complete Tenant Panel verification**

Run:

```bash
yarn test
yarn build
```

Expected: PASS.

- [ ] **Step 4: Run complete React Template verification**

Run:

```bash
yarn test
yarn build
```

Expected: PASS.

- [ ] **Step 5: Audit the target scenario**

Use this configuration:

```json
{
  "columns": [{ "field": "name" }],
  "constantFilters": { "status": "ACTIVE" },
  "actions": [{
    "kind": "edit",
    "disabledCondition": "status != 'ACTIVE'"
  }]
}
```

Verify the request query contains `status` in `fields`, the server query contains `status=ACTIVE`, returned rows expose `status` to action evaluation, and the rendered table has no Status column.

- [ ] **Step 6: Review worktree boundaries**

Run `git diff --check` and `git status --short` in all repositories. Do not stage unrelated existing changes or generated `react-template/dist/index.html` output.
