# Schema Table All-Items Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let Tenant Panel authors choose paginated or all-items loading for schema-backed tables and honor the choice in preview and production runtime.

**Architecture:** Persist an optional `table.dataMode`, normalize it fail-closed through a pure helper in each frontend, and dispatch schema/all tables to the existing unpaginated component. All other combinations retain the existing paginated component and request path.

**Tech Stack:** Go/Fiber model validation, React 18, TypeScript, Vitest, React Query, Yarn 4.

## Global Constraints

- Only schema-backed tables may activate all-items mode.
- Missing or unknown runtime values normalize with `value === "all" ? "all" : "paginated"`.
- Pipeline and workflow behavior remains unchanged and keeps dormant `table.dataMode` metadata.
- `GET /dynamic` remains unfiltered and subject to the existing maximum unbounded-read limit.
- No new backend endpoint, threshold, confirmation dialog, or client-side pagination.

---

### Task 1: Backend persisted contract and validation

**Files:**
- Modify: `models/pageModel.go`
- Modify: `models/frontendValidation.go`
- Test: `models/frontendValidation_create_test.go`
- Test: `models/models_test.go`

**Interfaces:**
- Produces: `TableComponentConfig.DataMode string` serialized as `dataMode,omitempty`.
- Produces: validation accepting `""`, `"paginated"`, and `"all"`, rejecting every other value.

- [ ] **Step 1: Write failing validation and JSON round-trip tests**

Add a table-driven validation test using `DataMode` values `""`, `"paginated"`, `"all"`, and `"future"`; add a JSON marshal/unmarshal assertion that preserves `"all"`.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./models -run 'TestValidateTableComponentConfigDataMode|TestTableComponentConfigDataModeJSON'`

Expected: compilation fails because `TableComponentConfig.DataMode` does not exist.

- [ ] **Step 3: Implement the model and strict validator**

Add:

```go
DataMode string `bson:"dataMode,omitempty" json:"dataMode,omitempty"`
```

At the start of `ValidateTableComponentConfig`, reject any non-empty value other than `paginated` and `all` with `invalid table dataMode` in the message.

- [ ] **Step 4: Format and verify GREEN**

Run: `gofmt -w models/pageModel.go models/frontendValidation.go models/frontendValidation_create_test.go models/models_test.go`

Run: `go test ./models -run 'TestValidateTableComponentConfigDataMode|TestTableComponentConfigDataModeJSON'`

- [ ] **Step 5: Commit backend contract**

```bash
git add models/pageModel.go models/frontendValidation.go models/frontendValidation_create_test.go models/models_test.go
git commit -m "feat: validate schema table data mode"
```

### Task 2: Frontend mode resolver and types

**Files:**
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/tableDataMode.ts`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/tableDataMode.test.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/types/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/page.ts`
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/tableDataMode.ts`
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/tableDataMode.test.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/types/page.ts`

**Interfaces:**
- Produces: `TableDataMode = "paginated" | "all"`.
- Produces: `resolveTableDataMode(value: unknown): TableDataMode`.
- Produces: `shouldUseAllItemsTable(kind: BindingKind | undefined, value: unknown): boolean`.

- [ ] **Step 1: Write identical failing resolver tests in both frontends**

Assert `all` remains `all`, missing/invalid values become `paginated`, schema/all returns true, and pipeline/all plus workflow/all return false.

- [ ] **Step 2: Run tests and verify RED**

Run in each frontend: `yarn test src/utils/tableDataMode.test.ts`

Expected: module-not-found failure.

- [ ] **Step 3: Implement minimal pure helpers and type fields**

Use:

```ts
export const resolveTableDataMode = (value: unknown): TableDataMode =>
  value === "all" ? "all" : "paginated";

export const shouldUseAllItemsTable = (kind: BindingKind | undefined, value: unknown) =>
  kind === "schema" && resolveTableDataMode(value) === "all";
```

Add `dataMode?: TableDataMode` to every persisted `TableComponentConfig` type copy.

- [ ] **Step 4: Run both focused tests and builds**

Run in each frontend: `yarn test src/utils/tableDataMode.test.ts`

Run in each frontend: `yarn build`

- [ ] **Step 5: Commit each frontend helper separately**

Commit message in both repositories: `feat: add schema table data mode resolver`.

### Task 3: Tenant Panel editor persistence

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/PageDesigner.tsx`
- Test: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/pageDesignerTableConfig.test.ts`

**Interfaces:**
- Consumes: `resolveTableDataMode` from Task 2.
- Produces: cleaner persistence of `dataMode` and schema-only Data loading selector.

- [ ] **Step 1: Write failing cleaner regression tests**

Cover `all` preservation, invalid normalization to `paginated`, and dormant `all` preservation while a table source changes away from schema and back.

- [ ] **Step 2: Run the focused test and verify RED**

Run: `yarn test src/utils/pageDesignerTableConfig.test.ts`

Expected: assertions fail because the cleaner omits `dataMode`.

- [ ] **Step 3: Persist normalized mode and add the schema-only selector**

Make `cleanTableConfig` include:

```ts
dataMode: resolveTableDataMode(tableConfig.dataMode),
```

In the Request tab, render the selector only when `tableSourceType === "schema"`, bind it to the normalized mode, and show the approved large-schema warning. Do not clear the field in source-type change handlers.

- [ ] **Step 4: Run test and Tenant Panel build**

Run: `yarn test src/utils/pageDesignerTableConfig.test.ts src/utils/tableDataMode.test.ts`

Run: `yarn build`

- [ ] **Step 5: Commit editor behavior**

```bash
git add src/components/PageDesigner/PageDesigner.tsx src/utils/pageDesignerTableConfig.test.ts
git commit -m "feat: configure schema table loading mode"
```

### Task 4: Preview and runtime dispatch

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/pages/PagePreviewPage.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/components/DynamicPageSections.tsx`
- Test: `/Users/osmansamilerdogan/Desktop/react-template/src/components/DynamicPageSections.test.ts`
- Test: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/tableDataMode.test.ts`

**Interfaces:**
- Consumes: `shouldUseAllItemsTable` from Task 2.
- Produces: schema/all rendering through `GenericUnpaginatedPage`; all remaining source/mode pairs through `GenericPaginatedPage`.

- [ ] **Step 1: Add failing dispatch regression tests**

Cover schema/all, schema/missing, schema/paginated, pipeline/all, workflow/all, and the schema → pipeline → schema dormant-mode sequence through the pure resolver and runtime section tests.

- [ ] **Step 2: Run focused tests and verify RED**

Run in React Template: `yarn test src/components/DynamicPageSections.test.ts src/utils/tableDataMode.test.ts`

Expected: schema/all still resolves to the paginated rendering path.

- [ ] **Step 3: Dispatch to the existing unpaginated component**

Import `GenericUnpaginatedPage` and `shouldUseAllItemsTable` in both rendering files. For valid table bindings, render `GenericUnpaginatedPage` only when the helper returns true; pass `schemaName`, `tableConfig`, title/header props, and actions. Keep the existing paginated props for every other case.

- [ ] **Step 4: Verify focused tests, all tests, and builds**

Run in Tenant Panel: `yarn test && yarn build`

Run in React Template: `yarn test && yarn build`

Run in backend: `go test ./...`

- [ ] **Step 5: Commit runtime integration in each repository**

Commit message in Tenant Panel: `feat: preview schema all-items tables`.

Commit message in React Template: `feat: render schema all-items tables`.

### Task 5: Final cross-project verification

**Files:**
- Verify only.

- [ ] **Step 1: Inspect diffs and worktree state**

Run `git diff --check` and `git status --short` in all three repositories; confirm no unrelated files are staged.

- [ ] **Step 2: Run complete verification fresh**

Run backend `go test ./...`, Tenant Panel `yarn test && yarn build`, and React Template `yarn test && yarn build`.

- [ ] **Step 3: Check acceptance criteria against the final diff**

Confirm default pagination, strict backend validation, cleaner normalization, dormant metadata, schema-only UI, unpaginated schema dispatch, unchanged pipeline/workflow dispatch, query-key separation, warning copy, and no new endpoint.
