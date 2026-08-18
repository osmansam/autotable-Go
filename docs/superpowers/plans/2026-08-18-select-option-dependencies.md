# Configurable Select Option Dependencies Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let schema-backed form selects fetch arbitrary source fields, render configured left/right option labels, and retain those fields for cart mappings and calculations.

**Architecture:** Extend the selection endpoint with a comma-separated `dataFields` projection whose fields pass the existing authorization and hashed-field checks. Persist optional select dependency/display metadata, derive one effective field set in each frontend, and pass rendered left/right values through the existing `OptionType.sourceItem` selection flow. Keep the closed select and all legacy configurations unchanged.

**Tech Stack:** Go, Fiber, MongoDB Go driver, React 18, TypeScript, react-select, TanStack Query, Vitest.

## Global Constraints

- All new configuration properties are optional and existing single-label selects must render unchanged.
- Templates support field interpolation only; no executable expressions.
- Missing optional display values render blank.
- Displayed dependencies are persisted into cart items only through explicit field mappings.
- Every requested source field must pass schema existence, role authorization, and hashed-field checks.
- Use TDD for every task and commit independently in the repository changed by that task.

---

## File Structure

### `autotable-Go`

- `models/containerModel.go`: persisted select option display shape.
- `models/models_test.go`: JSON/BSON compatibility coverage.
- `models/frontendValidation.go`: structural select dependency/template validation.
- `models/form_calculation_validation_test.go`: dependency and mapping validation cases.
- `controllers/dynamicController.go`: parse the `dataFields` query parameter.
- `services/dynamic_service.go`: authorize requested selection fields.
- `services/dynamic_service_test.go`: selection projection/security behavior.
- `repositories/dynamic_repository.go`: project all approved fields.
- `repositories/dynamic_repository_test.go`: Mongo projection assertion.

### `tenantPanel`

- `src/types/page.ts` and `src/utils/api/page.ts`: frontend configuration types.
- `src/utils/selectOptionConfig.ts`: template parsing/rendering and effective-field derivation.
- `src/utils/selectOptionConfig.test.ts`: pure configuration behavior.
- `src/utils/selectionQuery.ts` and `.test.ts`: stable `dataFields` request serialization.
- `src/utils/formConfig.ts` and `.test.ts`: build enriched options with retained source records.
- `src/components/forms/useFormSelectionData.ts`: request every effective field.
- `src/components/panelComponents/FormElements/SelectInput.tsx`: left/right open-option renderer.
- `src/components/PageDesigner/FormFieldEditor.tsx`: dependency and template controls.
- `src/components/PageDesigner/PageDesigner.tsx`: clean and persist the new fields.
- `src/components/PageDesigner/PageDesigner.formSave.test.ts`: refresh persistence regression.

### `react-template`

- Mirror the tenant runtime types and utilities in `src/types/page.ts`, `src/utils/selectOptionConfig.ts`, `src/utils/selectionQuery.ts`, and `src/utils/formConfig.ts` with matching tests.
- `src/components/forms/useFormSelectionData.ts`: effective-field selection requests.
- `src/components/panelComponents/FormElements/SelectInput.tsx`: left/right open-option renderer.

---

### Task 1: Multi-field selection endpoint

**Files:**
- Modify: `controllers/dynamicController.go:265-315`
- Modify: `services/dynamic_service.go:147-157,1021-1130`
- Modify: `repositories/dynamic_repository.go:391-420`
- Test: `services/dynamic_service_test.go`
- Test: `repositories/dynamic_repository_test.go`

**Interfaces:**
- Consumes: `GET /dynamic/selection?schemaName=product&fieldName=name&valueField=_id&dataFields=price,taxRate`
- Produces: `GetItemsForSelectionInput.DataFields []string`; approved fields passed to `FindForSelection(..., extraFields ...string)`.

- [ ] **Step 1: Write failing service and repository tests**

Add cases proving `price` and `taxRate` appear in the repository projection and returned documents, while an unknown, role-restricted, or hashed requested field returns an error. Use literal `DataFields: []string{"price", "taxRate"}` and assert the exact Mongo projection contains `_id`, `name`, `price`, and `taxRate`.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./services ./repositories -run 'Selection'`

Expected: FAIL because `GetItemsForSelectionInput` has no `DataFields` and additional projection fields are not forwarded.

- [ ] **Step 3: Implement parsing, validation, and projection**

Add `DataFields []string` to the input. Parse `dataFields` with `strings.Split`, trim entries, remove blanks/duplicates, and pass them to the service. Build one requested-field set from `FieldName`, `ValueField`, and `DataFields`; reject any field not present in `container.Fields` except `_id`; apply existing authorization and `IsHashed` checks to every requested field; pass the approved data fields to `FindForSelection`.

- [ ] **Step 4: Run focused and full backend tests**

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./services ./repositories ./controllers`

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./...`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controllers/dynamicController.go services/dynamic_service.go services/dynamic_service_test.go repositories/dynamic_repository.go repositories/dynamic_repository_test.go
git commit -m "feat: fetch selection dependency fields"
```

### Task 2: Persist select dependency and display configuration

**Files:**
- Modify: `models/containerModel.go:39-70`
- Modify: `models/frontendValidation.go:150-220`
- Test: `models/models_test.go`
- Test: `models/form_calculation_validation_test.go`

**Interfaces:**
- Produces: `SelectOptionDisplayConfig { LeftTemplate string; RightTemplate string }` and `ActionFormFieldConfig.SourceDataFields []string`, `ActionFormFieldConfig.OptionDisplay *SelectOptionDisplayConfig` using `sourceDataFields` and `optionDisplay` JSON/BSON keys.

- [ ] **Step 1: Write failing round-trip and validation tests**

Extend `TestFormComponentConfigRoundTrip` with:

```go
SourceDataFields: []string{"name", "price", "taxRate"},
OptionDisplay: &SelectOptionDisplayConfig{
    LeftTemplate: "{{name}}",
    RightTemplate: "{{price}} ₺",
},
```

Assert JSON and BSON round trips preserve both templates and all fields. Add validation cases for blank dependency names, malformed `{{ }}` references, and a mapping source field absent from the select's declared effective fields.

- [ ] **Step 2: Run model tests and verify RED**

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./models -run 'Form(ComponentConfig|Calculation)'`

Expected: FAIL because the new model fields/types do not exist.

- [ ] **Step 3: Implement model fields and structural validation**

Add the optional fields and a helper that extracts template tokens with `regexp.MustCompile(` + "`{{\\s*([A-Za-z_][A-Za-z0-9_.]*)\\s*}}`" + `)`. For a schema-backed select, define effective fields as value field, label field, `SourceDataFields`, and both template references. Reject blank dependency entries and malformed unmatched braces. Require every calculation mapping's `SourceField` to be in that effective set. Schema existence and field authorization remain enforced by the selection endpoint, where the source container is available.

- [ ] **Step 4: Run model and full backend tests**

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./models`

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./...`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add models/containerModel.go models/frontendValidation.go models/models_test.go models/form_calculation_validation_test.go
git commit -m "feat: persist select option dependencies"
```

### Task 3: Tenant select configuration utilities and request contract

**Files:**
- Create: `src/utils/selectOptionConfig.ts`
- Create: `src/utils/selectOptionConfig.test.ts`
- Modify: `src/types/page.ts`
- Modify: `src/utils/api/page.ts`
- Modify: `src/utils/selectionQuery.ts`
- Modify: `src/utils/selectionQuery.test.ts`
- Modify: `src/utils/formConfig.ts`
- Modify: `src/utils/formConfig.test.ts`

**Interfaces:**
- Produces: `SelectOptionDisplayConfig`, `sourceDataFields?: string[]`, `optionDisplay?: SelectOptionDisplayConfig`.
- Produces: `extractTemplateFields(template: string): string[]`, `renderOptionTemplate(template: string, record: Record<string, unknown>): string`, and `getEffectiveSelectDataFields(field: FormFieldConfig, mappings?: FormFieldMappingConfig[]): string[]`.
- Extends `getSelectionQueryConfig` with `dataFields?: string[]` serialized as a sorted, deduplicated comma-separated parameter.

- [ ] **Step 1: Write failing utility tests**

Test literal expectations:

```ts
expect(extractTemplateFields("{{code}} — {{name}} / {{price}}"))
  .toEqual(["code", "name", "price"]);
expect(renderOptionTemplate("{{price}} ₺", { price: 120 })).toBe("120 ₺");
expect(renderOptionTemplate("{{missing}}", {})).toBe("");
expect(getEffectiveSelectDataFields(field, mappings))
  .toEqual(["_id", "discountRate", "name", "price", "taxRate"]);
```

Also expect the selection URL to contain `dataFields=discountRate%2Cprice%2CtaxRate` and expect `buildFormInputs` to create an option with `label`, `leftLabel`, `rightLabel`, and the complete `sourceItem`.

- [ ] **Step 2: Run tests and verify RED**

Run: `yarn test src/utils/selectOptionConfig.test.ts src/utils/selectionQuery.test.ts src/utils/formConfig.test.ts --run`

Expected: FAIL because the types, helpers, URL parameter, and enriched labels do not exist.

- [ ] **Step 3: Implement the pure configuration boundary**

Use a safe token regex, stringify scalar values only, return blank for missing/object values, deduplicate and sort effective fields, and preserve `sourceLabelField` as the fallback left label. Extend `OptionType` with optional `leftLabel` and `rightLabel` strings without removing `sourceItem`.

- [ ] **Step 4: Run focused tests**

Run: `yarn test src/utils/selectOptionConfig.test.ts src/utils/selectionQuery.test.ts src/utils/formConfig.test.ts --run`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/types/page.ts src/utils/api/page.ts src/types/index.ts src/components/panelComponents/shared/types.ts src/utils/selectOptionConfig.ts src/utils/selectOptionConfig.test.ts src/utils/selectionQuery.ts src/utils/selectionQuery.test.ts src/utils/formConfig.ts src/utils/formConfig.test.ts
git commit -m "feat: model select option dependencies"
```

### Task 4: Tenant designer controls and save persistence

**Files:**
- Modify: `src/components/PageDesigner/FormFieldEditor.tsx:250-310`
- Modify: `src/components/PageDesigner/PageDesigner.tsx:1632-1740`
- Modify: `src/components/PageDesigner/PageDesigner.formSave.test.ts`
- Test: `src/components/PageDesigner/FormFieldEditor.test.tsx`

**Interfaces:**
- Consumes: Task 3 configuration types and template helpers.
- Produces: saved `sourceDataFields` and `optionDisplay` configuration.

- [ ] **Step 1: Write failing save and editor tests**

Extend the save regression fixture with `sourceDataFields: ["name", "price"]` and `{ leftTemplate: "{{name}}", rightTemplate: "{{price}} ₺" }`, asserting the cleaned form retains them exactly. Add an editor test that chooses `price` in the dependency multi-select, enters both templates, and observes those values in `onChange`.

- [ ] **Step 2: Run tests and verify RED**

Run: `yarn test src/components/PageDesigner/PageDesigner.formSave.test.ts src/components/PageDesigner/FormFieldEditor.test.tsx --run`

Expected: FAIL because save cleaning and editor controls do not handle these properties.

- [ ] **Step 3: Implement the designer UI**

Show dependency controls only for schema-backed selects. Populate choices from the selected container's fields. Reset dependencies/templates when the source schema changes. Render a non-interactive preview row with left text flexing and right text aligned to the end. In `cleanFormConfig`, trim/deduplicate `sourceDataFields`, trim templates, and omit an empty `optionDisplay`.

- [ ] **Step 4: Run focused tests, full tests, lint, and build**

Run: `yarn test --run`

Run: `yarn lint`

Run: `yarn build`

Expected: all tests/build PASS and lint has zero new errors or warnings.

- [ ] **Step 5: Commit**

```bash
git add src/components/PageDesigner/FormFieldEditor.tsx src/components/PageDesigner/FormFieldEditor.test.tsx src/components/PageDesigner/PageDesigner.tsx src/components/PageDesigner/PageDesigner.formSave.test.ts
git commit -m "feat: configure select option labels"
```

### Task 5: Tenant preview/runtime option rendering

**Files:**
- Modify: `src/components/forms/useFormSelectionData.ts`
- Modify: `src/components/panelComponents/FormElements/SelectInput.tsx`
- Create: `src/components/panelComponents/FormElements/SelectOptionContent.tsx`
- Create: `src/components/panelComponents/FormElements/SelectOptionContent.test.tsx`

**Interfaces:**
- Consumes: `OptionType.leftLabel`, `OptionType.rightLabel`, `getEffectiveSelectDataFields`, and `getSelectionQueryConfig({ dataFields })`.
- Produces: open-menu option rows with left/right labels; closed control continues to use `option.label`.

- [ ] **Step 1: Write failing renderer test**

Use `renderToStaticMarkup` and assert an option `{ label: "Syrup", leftLabel: "Syrup", rightLabel: "120 ₺" }` renders both literal values in separate left/right elements. Assert `{ label: "Legacy" }` renders `Legacy` once and no right element.

- [ ] **Step 2: Run tests and verify RED**

Run: `yarn test src/components/panelComponents/FormElements/SelectOptionContent.test.tsx --run`

Expected: FAIL because `SelectOptionContent` does not exist.

- [ ] **Step 3: Implement requests and open-option rendering**

Have `useFormSelectionData` derive effective dependencies from the form and its object-list mappings, pass non-value/label fields as `dataFields`, and include them in the query key. Make `CustomOption` render `SelectOptionContent`; keep react-select's `SingleValue` default so the closed select displays only `label`. Keep the selected check icon after the right region.

- [ ] **Step 4: Run tenant verification**

Run: `yarn test --run`

Run: `yarn lint`

Run: `yarn build`

Expected: all tests/build PASS and lint has zero new errors or warnings.

- [ ] **Step 5: Commit**

```bash
git add src/components/forms/useFormSelectionData.ts src/components/panelComponents/FormElements/SelectInput.tsx src/components/panelComponents/FormElements/SelectOptionContent.tsx src/components/panelComponents/FormElements/SelectOptionContent.test.tsx
git commit -m "feat: preview select option dependencies"
```

### Task 6: Production runtime parity

**Files:**
- Create: `src/utils/selectOptionConfig.ts`
- Create: `src/utils/selectOptionConfig.test.ts`
- Modify: `src/types/page.ts`
- Modify: `src/types/index.ts`
- Modify: `src/components/panelComponents/shared/types.ts`
- Modify: `src/utils/selectionQuery.ts`
- Modify: `src/utils/selectionQuery.test.ts`
- Modify: `src/utils/formConfig.ts`
- Modify: `src/utils/formConfig.test.ts`
- Modify: `src/components/forms/useFormSelectionData.ts`
- Modify: `src/components/panelComponents/FormElements/SelectInput.tsx`
- Create: `src/components/panelComponents/FormElements/SelectOptionContent.tsx`
- Create: `src/components/panelComponents/FormElements/SelectOptionContent.test.tsx`

**Interfaces:**
- Produces the same public types, helper signatures, request format, and rendering behavior as Tasks 3 and 5.

- [ ] **Step 1: Port the tenant tests before production code**

Copy the literal utility, form-option, selection-query, and static-renderer expectations into the matching `react-template` test files. Do not copy implementations yet.

- [ ] **Step 2: Run tests and verify RED**

Run: `yarn test src/utils/selectOptionConfig.test.ts src/utils/selectionQuery.test.ts src/utils/formConfig.test.ts src/components/panelComponents/FormElements/SelectOptionContent.test.tsx --run`

Expected: FAIL because runtime parity is absent.

- [ ] **Step 3: Implement production parity**

Port the pure helpers, types, effective request construction, enriched options, and `SelectOptionContent` integration. Preserve `react-template`'s existing mobile `menuPosition` behavior and other styling differences.

- [ ] **Step 4: Run runtime verification**

Run: `yarn test --run`

Run: `yarn eslint src/utils/selectOptionConfig.ts src/utils/selectOptionConfig.test.ts src/utils/selectionQuery.ts src/utils/selectionQuery.test.ts src/utils/formConfig.ts src/utils/formConfig.test.ts src/components/forms/useFormSelectionData.ts src/components/panelComponents/FormElements/SelectInput.tsx src/components/panelComponents/FormElements/SelectOptionContent.tsx src/components/panelComponents/FormElements/SelectOptionContent.test.tsx`

Run: `yarn build`

Expected: all tests/build PASS and changed-file lint has zero errors.

- [ ] **Step 5: Commit**

```bash
git add src/types/page.ts src/types/index.ts src/components/panelComponents/shared/types.ts src/utils/selectOptionConfig.ts src/utils/selectOptionConfig.test.ts src/utils/selectionQuery.ts src/utils/selectionQuery.test.ts src/utils/formConfig.ts src/utils/formConfig.test.ts src/components/forms/useFormSelectionData.ts src/components/panelComponents/FormElements/SelectInput.tsx src/components/panelComponents/FormElements/SelectOptionContent.tsx src/components/panelComponents/FormElements/SelectOptionContent.test.tsx
git commit -m "feat: render select option dependencies"
```

### Task 7: Configure and verify the Da Vinci order example

**Files:**
- Modify: `docs/examples/davinci-order-calculations.json`
- Test: all three repository suites from prior tasks.

**Interfaces:**
- Consumes the persisted configuration and runtime contracts from Tasks 1-6.
- Produces an example where `name` is left, `price` is right, and `price` maps to `unitPrice` for the existing cart total chain.

- [ ] **Step 1: Update the example configuration**

Add:

```json
"sourceDataFields": ["name", "price"],
"optionDisplay": {
  "leftTemplate": "{{name}}",
  "rightTemplate": "{{price}} ₺"
}
```

Keep `fieldMappings`, `itemCalculations`, and `summaries` from the calculation example.

- [ ] **Step 2: Run final verification in all repositories**

Backend: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./...`

Tenant: `yarn test --run && yarn lint && yarn build`

Runtime: `yarn test --run && yarn build`

Expected: every test and build exits zero; lint introduces no new errors.

- [ ] **Step 3: Commit the example**

```bash
git add docs/examples/davinci-order-calculations.json
git commit -m "docs: show product price in order options"
```
