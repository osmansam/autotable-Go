# Relation Matrix Row Filters Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add table-style interactive filters that affect only relation-matrix row records.

**Architecture:** Reuse `TableFilterPanelConfig` across the Go page model and both TypeScript models. The tenant designer authors row filters, while the customer runtime builds the established `GenericTable` filter panel and passes only applied values to the row query; the column request remains independent and unfiltered.

**Tech Stack:** Go, BSON/JSON models, TypeScript, React, TanStack Query, Tailwind CSS, Vitest

**Spec:** `docs/superpowers/specs/2026-08-27-relation-matrix-row-filters-design.md`

## Global Constraints

- Filters affect only the relation-matrix row-schema request.
- The column request always receives an empty filter object.
- Reuse `TableFilterPanelConfig`; do not introduce another persisted filter format.
- Existing matrices without `filterPanel` remain backward compatible.
- Apply and Clear All Filters behave like the existing table filter panel.
- Relation membership overrides and toggle state survive row-filter changes.
- Keep `tenantPanel` and `react-template` type contracts synchronized with Go.
- Preserve unrelated user changes in all three repositories.

---

### Task 1: Backend Relation-Matrix Filter Contract

**Files:**
- Modify: `models/pageModel.go`
- Modify: `models/frontendValidation.go`
- Test: `models/models_test.go`

**Interfaces:**
- Consumes: existing `TableFilterPanelConfig` and `ValidateFilterPanelConfig`.
- Produces: `RelationMatrixConfig.FilterPanel *TableFilterPanelConfig` serialized as `filterPanel,omitempty` in JSON and BSON.

- [ ] **Step 1: Write failing round-trip and validation tests**

Extend the existing relation-matrix tests in `models/models_test.go`. Add a valid filter input:

```go
inputs := []ActionFormFieldConfig{{
    FormKey: "status",
    Type: "select",
    Label: "Status",
    OptionsSource: "static",
    StaticOptionsJson: `[{"value":"active","label":"Active"}]`,
}}
validConfig.FilterPanel = &TableFilterPanelConfig{Inputs: &inputs}
```

Assert the input survives JSON and BSON round trips. Add invalid cases with an empty `formKey` and empty `type`, expecting errors containing `relationMatrix filterPanel: filter input`.

- [ ] **Step 2: Run the model tests and verify failure**

Run: `GOCACHE=/private/tmp/autotable-matrix-filters-red go test ./models -run 'Test.*RelationMatrix'`

Expected: FAIL because `RelationMatrixConfig` has no `FilterPanel` field and malformed matrix filter panels are not validated.

- [ ] **Step 3: Add the model field and reuse validation**

Add to `RelationMatrixConfig`:

```go
FilterPanel *TableFilterPanelConfig `bson:"filterPanel,omitempty" json:"filterPanel,omitempty"`
```

At the end of `ValidateRelationMatrixConfig`, before returning success, add:

```go
if err := ValidateFilterPanelConfig(config.FilterPanel); err != nil {
    return fmt.Errorf("relationMatrix filterPanel: %w", err)
}
```

- [ ] **Step 4: Format and verify the backend contract**

Run: `gofmt -w models/pageModel.go models/frontendValidation.go models/models_test.go`

Run: `GOCACHE=/private/tmp/autotable-matrix-filters-green go test ./models -run 'Test.*RelationMatrix'`

Expected: PASS.

- [ ] **Step 5: Commit the backend contract**

```bash
git add models/pageModel.go models/frontendValidation.go models/models_test.go
git commit -m "feat: validate relation matrix row filters"
```

### Task 2: Frontend Types and Tenant Configuration Cleaning

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/types/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/types/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/relationMatrixConfig.ts`
- Test: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/relationMatrixConfig.test.ts`

**Interfaces:**
- Produces: `RelationMatrixConfig.filterPanel?: TableFilterPanelConfig` in both frontends.
- Produces: `cleanRelationMatrixFilterPanel(filterPanel): TableFilterPanelConfig | undefined` in tenant configuration utilities.
- Preserves: complete existing relation-matrix cleaning behavior.

- [ ] **Step 1: Write failing tenant cleaning tests**

Extend `relationMatrixConfig.test.ts` with a matrix whose filter inputs contain surrounding whitespace. Assert cleaning produces:

```ts
filterPanel: {
  inputs: [{
    formKey: "status",
    type: "select",
    formKeyType: "string",
    label: "Status",
    placeholder: "Choose status",
    optionsSource: "static",
    staticOptionsJson: "[]",
  }],
}
```

Assert blank `formKey` entries are removed and an empty resulting panel is omitted. Assert the original object is unchanged.

- [ ] **Step 2: Run the focused tenant test and verify failure**

Run from `/Users/osmansamilerdogan/Desktop/tenantPanel`: `yarn test src/utils/relationMatrixConfig.test.ts`

Expected: FAIL because the relation-matrix type and cleaner do not preserve `filterPanel`.

- [ ] **Step 3: Synchronize types and add focused cleaning**

Add the optional property to both `RelationMatrixConfig` interfaces:

```ts
filterPanel?: TableFilterPanelConfig;
```

In `tenantPanel/src/utils/relationMatrixConfig.ts`, add a focused cleaner that filters blank keys, trims string metadata, retains supported option/source/validation fields, and returns `undefined` when no inputs remain. Include its result from `cleanRelationMatrixConfig` without changing existing toggle cleaning.

- [ ] **Step 4: Run the focused tests and both type-checking builds**

Run: `yarn test src/utils/relationMatrixConfig.test.ts`

Run in `tenantPanel`: `yarn build`

Run in `react-template`: `yarn build`

Expected: all PASS.

- [ ] **Step 5: Commit each repository’s type/config change**

In `tenantPanel`:

```bash
git add src/types/page.ts src/utils/relationMatrixConfig.ts src/utils/relationMatrixConfig.test.ts
git commit -m "feat: preserve relation matrix row filters"
```

In `react-template`:

```bash
git add src/types/page.ts
git commit -m "feat: type relation matrix row filters"
```

### Task 3: Focused Tenant Row-Filter Editor

**Files:**
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/RelationMatrixFilterEditor.tsx`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/relationMatrixFilterEditor.ts`
- Test: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/relationMatrixFilterEditor.test.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/PageDesigner.tsx`

**Interfaces:**
- Produces: `buildRelationMatrixFilterInputs(fields: Field[]): TableFilterPanelInputConfig[]`.
- Produces: immutable helpers `addRelationMatrixFilterInput`, `updateRelationMatrixFilterInput`, and `removeRelationMatrixFilterInput`.
- Produces: `RelationMatrixFilterEditor` with props `{ value, rowFields, onChange }`.
- Consumes: `RelationMatrixConfig.filterPanel` from Task 2.

- [ ] **Step 1: Write failing helper tests**

Use row fields containing `_id`, text, Boolean, enum, image, and schema-backed ObjectID fields. Assert `buildRelationMatrixFilterInputs` excludes IDs/images, maps Boolean to a static True/False select, preserves enums as static options, and maps object references to schema-backed selects.

Assert add/update/remove helpers return new matrix objects, preserve the original, and use this default new input:

```ts
{
  formKey: "",
  type: "text",
  formKeyType: "string",
  label: "",
  placeholder: "",
  required: false,
  optionsSource: "static",
  staticOptionsJson: "[]",
  sourceValueField: "_id",
}
```

- [ ] **Step 2: Run helper tests and verify failure**

Run: `yarn test src/components/PageDesigner/relationMatrixFilterEditor.test.ts`

Expected: FAIL because the helper module does not exist.

- [ ] **Step 3: Implement immutable helpers**

Create `relationMatrixFilterEditor.ts`. Follow the existing `actionInputTypeFromField`, `formKeyTypeForActionInput`, and static/schema option conventions already used in `PageDesigner.tsx`. Keep all helpers pure and return a new `filterPanel.inputs` array.

- [ ] **Step 4: Build the focused editor component**

Create `RelationMatrixFilterEditor.tsx`. Render a `Row Filters` card with:

- an Enable default filters checkbox using `buildRelationMatrixFilterInputs(rowFields)`;
- an Add filter button;
- one compact row per input with row-field selection, type, label, placeholder, and remove action;
- select-only controls for static/schema option source, static options JSON, source schema, value field, label field, request filters, and source condition;
- existing numeric/multiple/default settings supported by `TableFilterPanelInputConfig`.

Use the immutable helpers for changes and expose no raw whole-panel JSON editor.

- [ ] **Step 5: Integrate with the relation-matrix section**

In `PageDesigner.tsx`, derive `rowFields` from `relationMatrixConfig.rowSchemaName`, render `RelationMatrixFilterEditor` below the existing relation contract fields, and update `relationMatrixConfig` through its `onChange` prop. When row schema changes, also set `filterPanel: undefined` so stale field references cannot survive.

- [ ] **Step 6: Run focused tests and tenant build**

Run: `yarn test src/components/PageDesigner/relationMatrixFilterEditor.test.ts src/utils/relationMatrixConfig.test.ts`

Run: `yarn build`

Expected: both PASS.

- [ ] **Step 7: Commit designer support**

```bash
git add src/components/PageDesigner/RelationMatrixFilterEditor.tsx src/components/PageDesigner/relationMatrixFilterEditor.ts src/components/PageDesigner/relationMatrixFilterEditor.test.ts src/components/PageDesigner/PageDesigner.tsx
git commit -m "feat: edit relation matrix row filters"
```

### Task 4: Runtime Row-Request Filter Construction

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/relationMatrix.ts`
- Test: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/relationMatrix.test.ts`

**Interfaces:**
- Produces: `compactRelationMatrixRowFilters(values: Record<string, unknown>): Record<string, unknown>`.
- Produces: `buildRelationMatrixRequests(config, appliedRowFilters)` returning row and column request descriptors.
- Consumes: `RelationMatrixConfig.filterPanel` from Task 2.

- [ ] **Step 1: Write failing request-construction tests**

Assert values `""`, whitespace-only strings, `null`, `undefined`, and empty arrays are omitted while `false`, `0`, populated arrays, dates, and non-empty strings are retained. Then assert:

```ts
buildRelationMatrixRequests(config, { status: "active" })
```

returns:

```ts
{
  row: { page: 1, limit: 100, schemaName: "product", filters: { status: "active" } },
  column: { page: 1, limit: 40, schemaName: "countList", filters: {} },
}
```

Mutating the row filters must not change `column.filters`.

- [ ] **Step 2: Run the relation utility tests and verify failure**

Run: `yarn test src/utils/relationMatrix.test.ts`

Expected: FAIL because the request helpers do not exist.

- [ ] **Step 3: Implement the pure helpers**

Add the two exported functions. Clamp the column limit exactly as the component does today, use a fixed row limit of 100, copy retained filter values into a new object, and always create a fresh empty `column.filters` object.

- [ ] **Step 4: Run relation utility tests**

Run: `yarn test src/utils/relationMatrix.test.ts`

Expected: PASS.

- [ ] **Step 5: Commit request construction**

```bash
git add src/utils/relationMatrix.ts src/utils/relationMatrix.test.ts
git commit -m "feat: build filtered relation matrix row requests"
```

### Task 5: Runtime Table-Style Filter Panel

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/components/RelationMatrix.tsx`
- Test: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/relationMatrix.test.ts`
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/tableFilters.test.tsx`

**Interfaces:**
- Consumes: `buildRelationMatrixRequests` from Task 4.
- Consumes: `buildConfiguredFilterInputs`, `getFilterDefaultValues`, and `useFilterPanelSelectionData` from `src/utils/tableFilters.tsx`.
- Produces: a `PanelFilterType` passed to the existing `GenericTable.filterPanel` prop.

- [ ] **Step 1: Add failing default/filter-input behavior tests**

In `tableFilters.test.tsx`, add a relation-matrix-shaped fixture proving configured text/select inputs become `GenericInputType` entries, schema-backed selections use their configured value/label fields, and defaults produce literal initial applied values. Keep these tests against the real shared helper functions.

- [ ] **Step 2: Run focused tests and verify failure where behavior is missing**

Run: `yarn test src/utils/tableFilters.test.tsx src/utils/relationMatrix.test.ts`

Expected: relation request tests pass from Task 4; any newly exposed missing shared behavior fails before component integration. If all shared-helper assertions already pass, record that they characterize reused behavior and continue without changing those helpers.

- [ ] **Step 3: Wire applied filter state into `RelationMatrix`**

In `RelationMatrix.tsx`:

```ts
const configuredFilters = config.filterPanel?.inputs;
const defaultFilters = useMemo(
  () => getFilterDefaultValues(configuredFilters),
  [configuredFilters],
);
const [appliedRowFilters, setAppliedRowFilters] = useState(defaultFilters);
const selectionData = useFilterPanelSelectionData(configuredFilters || []);
const filterInputs = useMemo(
  () => buildConfiguredFilterInputs(configuredFilters, [], selectionData),
  [configuredFilters, selectionData],
);
const requests = buildRelationMatrixRequests(config, appliedRowFilters);
```

Call `useGetPaginatedItems` with the row descriptor’s filters and the column descriptor’s empty filters. Preserve `overrides`, `pending`, and toggle state exactly as separate component state.

- [ ] **Step 4: Pass the shared filter panel to `GenericTable`**

When `configuredFilters` exists, pass:

```ts
filterPanel={{
  inputs: filterInputs,
  formElements: appliedRowFilters,
  setFormElements: setAppliedRowFilters,
  isFilterPanelActive: filterInputs.length > 0,
  isApplyButtonActive: true,
  isCloseButtonActive: false,
}}
```

Keep all currently disabled matrix controls disabled. Clear All Filters will use the shared component’s established empty-value behavior.

- [ ] **Step 5: Run focused tests and production build**

Run: `yarn test src/utils/relationMatrix.test.ts src/utils/tableFilters.test.tsx`

Run: `yarn build`

Expected: both PASS.

- [ ] **Step 6: Commit runtime filtering**

```bash
git add src/components/RelationMatrix.tsx src/utils/relationMatrix.test.ts src/utils/tableFilters.test.tsx
git commit -m "feat: filter relation matrix rows"
```

### Task 6: Cross-Project Verification

**Files:**
- Verify only; modify only files already listed if a regression exposes a scoped defect.

**Interfaces:**
- Consumes: all completed tasks.
- Produces: a verified backend/editor/runtime feature.

- [ ] **Step 1: Run the complete backend suite**

Run from `/Users/osmansamilerdogan/Desktop/autotable-Go`: `GOCACHE=/private/tmp/autotable-matrix-filters-final go test ./...`

Expected: PASS.

- [ ] **Step 2: Run complete tenant-panel checks**

Run from `/Users/osmansamilerdogan/Desktop/tenantPanel`: `yarn test`

Run: `yarn build`

Expected: both PASS.

- [ ] **Step 3: Run complete customer-template checks**

Run from `/Users/osmansamilerdogan/Desktop/react-template`: `yarn test`

Run: `yarn build`

Expected: both PASS.

- [ ] **Step 4: Inspect all three working trees**

Run: `git status --short` in each repository.

Expected: only pre-existing unrelated changes remain; verification-generated tracked build artifacts are restored before handoff.

- [ ] **Step 5: Perform a manual matrix smoke test**

Configure text and schema-backed select row filters. Open the runtime matrix, apply each filter, and confirm rows change while column headers remain stable. Clear all filters and confirm rows return. Toggle one visible relation, filter it out, clear filters, and confirm its server-backed membership is still correct.
