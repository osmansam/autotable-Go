# Table Template Column Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add display-only table columns that interpolate row fields with templates such as `{{name}} {{surname}}`.

**Architecture:** Persist `type: "template"` and a `template` string in the shared table-column configuration. Keep interpolation and placeholder discovery in focused frontend utilities shared by the designer preview and runtime rendering paths; the backend only serializes and validates configuration.

**Tech Stack:** Go, TypeScript, React, Vitest, Vite

## Global Constraints

- Template syntax supports field interpolation only; it never evaluates JavaScript or expressions.
- Missing, null, and undefined values render as empty strings; zero and false remain visible.
- Collapse repeated whitespace and trim the final output.
- Template columns are display-only and cannot be server-sorted or server-filtered by their synthetic key.
- Placeholder source fields are automatically added to fetched table fields.

---

### Task 1: Backend Configuration Contract

**Files:**
- Modify: `models/pageModel.go`
- Modify: `models/frontendValidation.go`
- Test: `models/frontendValidation_create_test.go`
- Test: `models/models_test.go`

**Interfaces:**
- Produces: `TableColumnConfig.Template string` serialized as `template`.
- Validation rule: a column whose type is `template` requires non-blank `Field` and `Template`.

- [ ] **Step 1: Write failing validation and round-trip tests**

Add tests that reject `{Field: "fullName", Type: "template", Template: " "}` and preserve `{Field: "fullName", Type: "template", Template: "{{name}} {{surname}}"}` through JSON/BSON model serialization.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./models -run 'Test.*TemplateColumn' -count=1`

Expected: FAIL because `Template` is absent and blank templates are accepted.

- [ ] **Step 3: Implement persistence and validation**

Add:

```go
Template string `bson:"template,omitempty" json:"template,omitempty"`
```

to `TableColumnConfig`. In table validation, return a specific validation error when `column.Type == "template" && strings.TrimSpace(column.Template) == ""`.

- [ ] **Step 4: Run focused tests**

Run: `go test ./models -run 'Test.*TemplateColumn' -count=1`

Expected: PASS.

### Task 2: Template Interpolation and Field Discovery

**Files:**
- Create: `tenantPanel/src/utils/tableColumnTemplate.ts`
- Create: `tenantPanel/src/utils/tableColumnTemplate.test.ts`
- Create: `react-template/src/utils/tableColumnTemplate.ts`
- Create: `react-template/src/utils/tableColumnTemplate.test.ts`
- Modify: `tenantPanel/src/utils/tableConfig.ts`
- Modify: `tenantPanel/src/utils/tableConfig.test.ts`
- Modify: `react-template/src/utils/tableConfig.ts`
- Modify: `react-template/src/utils/tableConfig.test.ts`

**Interfaces:**
- Produces: `getTableTemplateFields(template?: string): string[]`.
- Produces: `renderTableColumnTemplate(template: string | undefined, row: Record<string, unknown>): string`.

- [ ] **Step 1: Write failing utility tests**

Test that `renderTableColumnTemplate("{{name}} {{surname}}", {name: "Ada", surname: "Lovelace"})` returns `Ada Lovelace`; missing surname returns `Ada`; `{count: 0, active: false}` preserves `0 false`; duplicate placeholders are deduplicated by `getTableTemplateFields`.

- [ ] **Step 2: Run tests to verify failure**

Run in each frontend: `yarn test src/utils/tableColumnTemplate.test.ts`

Expected: FAIL because the utility does not exist.

- [ ] **Step 3: Implement the utilities**

Use `/\{\{\s*([^}]+?)\s*\}\}/g` to extract and interpolate trimmed field names. Convert only nullish values to `""`, then normalize output with `.replace(/\s+/g, " ").trim()`.

- [ ] **Step 4: Add failing resolver tests**

For a synthetic `fullName` template column, assert `getTableDataFieldNames` includes `name` and `surname` but excludes `fullName`.

- [ ] **Step 5: Integrate placeholder discovery**

In both `tableConfig.ts` resolvers, branch on `column.type === "template"`, add every result from `getTableTemplateFields(column.template)`, and do not add `column.field`.

- [ ] **Step 6: Run utility and resolver tests**

Run in each frontend: `yarn test src/utils/tableColumnTemplate.test.ts src/utils/tableConfig.test.ts`

Expected: PASS.

### Task 3: Tenant Panel Types and Designer

**Files:**
- Modify: `tenantPanel/src/types/page.ts`
- Modify: `tenantPanel/src/utils/api/page.ts`
- Modify: `tenantPanel/src/utils/pageDesignerTableConfig.ts`
- Modify: `tenantPanel/src/utils/pageDesignerTableConfig.test.ts`
- Modify: `tenantPanel/src/components/PageDesigner/PageDesigner.tsx`

**Interfaces:**
- Extends `TableColumnType` with `"template"`.
- Extends `TableColumnConfig` with `template?: string`.

- [ ] **Step 1: Write a failing designer cleanup test**

Assert saving trims `"  {{name}} {{surname}}  "` to `"{{name}} {{surname}}"` and omits blank templates.

- [ ] **Step 2: Run the test to verify failure**

Run: `yarn test src/utils/pageDesignerTableConfig.test.ts`

Expected: FAIL because template cleanup is absent.

- [ ] **Step 3: Add shared types and cleanup**

Add `"template"` and `template?: string` to both Tenant type contracts. Add `Template` to `TABLE_COLUMN_TYPE_OPTIONS`. Make `cleanTableConfig` persist a trimmed template only when the type is `template`.

- [ ] **Step 4: Add designer controls**

When a column type is `template`, render an input bound to `column.template` with placeholder `{{name}} {{surname}}` and helper text explaining placeholder syntax. Keep the normal field input as the synthetic key and display-name input as its heading.

- [ ] **Step 5: Run focused tests and typecheck build**

Run: `yarn test src/utils/pageDesignerTableConfig.test.ts src/utils/tableConfig.test.ts src/utils/tableColumnTemplate.test.ts && yarn build`

Expected: PASS with no TypeScript errors.

### Task 4: Preview and Runtime Rendering

**Files:**
- Modify: `tenantPanel/src/components/panelComponents/FormElements/GenericPaginatedPage.tsx`
- Modify: `tenantPanel/src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx`
- Modify: `react-template/src/types/page.ts`
- Modify: `react-template/src/components/panelComponents/FormElements/GenericPaginatedPage.tsx`
- Modify: `react-template/src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx`

**Interfaces:**
- Consumes: `renderTableColumnTemplate(template, row)` from Task 2.

- [ ] **Step 1: Add `template` to runtime types**

Extend runtime `TableColumnType` and `TableColumnConfig` with the exact fields used by Tenant Panel.

- [ ] **Step 2: Render template columns in all four table paths**

Next to the existing `computedLabel` branch, add a `template` branch whose cell node returns:

```tsx
<span>{renderTableColumnTemplate(columnConfig.template, row)}</span>
```

Pass the content through the existing `renderLinkedCellContent` wrapper so links and cell presentation remain compatible.

- [ ] **Step 3: Run frontend tests**

Run in both projects: `yarn test`

Expected: all tests pass with no type errors.

### Task 5: Full Verification

**Files:**
- Verify all modified files; do not alter unrelated dirty-worktree changes.

- [ ] **Step 1: Run backend verification**

Run: `go test ./... -count=1`

Expected: all packages pass.

- [ ] **Step 2: Run Tenant Panel verification**

Run: `yarn test && yarn build`

Expected: all tests and production build pass.

- [ ] **Step 3: Run runtime verification**

Run: `yarn test && yarn build`

Expected: all tests and production build pass.

- [ ] **Step 4: Review diffs**

Run `git diff --check` and `git status --short` in all three repositories. Confirm only intended feature files plus pre-existing user changes remain.
