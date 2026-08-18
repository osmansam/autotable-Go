# Configurable Form Cart Calculations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add reusable product-price snapshots, line totals, live order summaries, designer controls, and backend-authoritative verification to dynamic forms.

**Architecture:** Persist structured mappings and operation-based calculations in the page component configuration. Mirror a small deterministic calculation utility in both frontend repositories for live behavior, while `autotable-Go` resolves the stored page/component configuration and recalculates financial values before workflow persistence. The client submits only a page/component reference and record data; calculation configuration is always loaded from trusted storage.

**Tech Stack:** Go 1.x, Fiber, MongoDB driver, Go tests; React 18, TypeScript, TanStack Query, Tailwind CSS, Vitest, Yarn 4.

## Global Constraints

- New configuration properties are optional; existing forms must retain current behavior.
- Initial operations are `multiply`, `sum`, and `copy`; do not add arbitrary expression evaluation.
- Supported precision is an integer from `0` through `6`, defaulting to `2`.
- Currency codes are exactly three uppercase ASCII letters; the supplied order form uses `TRY`.
- Missing or nonnumeric required inputs must fail; they must never silently become zero.
- Backend calculations are authoritative and stale or tampered financial values are rejected before persistence.
- The backend must load calculation configuration from the persisted page/component, never from a submitted configuration object.
- `tenantPanel` and `react-template` use equivalent utility APIs and identical calculation fixtures.

---

## File Structure

### `autotable-Go`

- `models/pageModel.go`: persisted configuration structs.
- `models/frontendValidation.go`: form calculation reference and operation validation.
- `models/models_test.go`: JSON/BSON round-trip coverage.
- `models/form_calculation_validation_test.go`: focused invalid/valid configuration cases.
- `services/form_cart_calculations.go`: decimal-safe item and summary evaluation independent of HTTP and MongoDB.
- `services/form_cart_calculations_test.go`: shared fixtures, rounding, and stale-value cases.
- `services/form_cart_verifier.go`: persisted page/component resolution, authoritative product reads, and workflow record replacement.
- `services/form_cart_verifier_test.go`: repository-backed verification cases.
- `services/dynamic_service.go`: call the verifier before the selected workflow executes.
- `controllers/dynamicController.go`: parse trusted configuration references separately from record data.
- `controllers/error_paths_test.go`: workflow request parsing coverage.

### `tenantPanel`

- `src/types/page.ts` and `src/utils/api/page.ts`: calculation configuration contracts.
- `src/utils/formCalculations.ts`: preview calculation engine.
- `src/utils/formCalculations.test.ts`: shared fixture coverage.
- `src/utils/formConfig.ts`: payload building and calculated field integration.
- `src/components/forms/DynamicForm.tsx`: snapshot, recalculate, submit-reference, and error flow.
- `src/components/forms/DynamicFormObjectList.tsx`: unit-price and line-total rendering.
- `src/components/forms/DynamicFormSummary.tsx`: summary presentation.
- `src/components/PageDesigner/FormComponentEditor.tsx`: mapping, calculation, and summary controls.
- `src/components/PageDesigner/formCalculationEditor.ts`: immutable editor update and validation helpers.
- `src/components/PageDesigner/formCalculationEditor.test.ts`: designer serialization/validation tests.
- `src/pages/PagePreviewPage.tsx`: pass page/component identity into preview forms.

### `react-template`

- `src/types/page.ts`: calculation configuration contracts.
- `src/utils/formCalculations.ts`: production calculation engine mirroring `tenantPanel`.
- `src/utils/formCalculations.test.ts`: identical shared fixtures.
- `src/utils/formConfig.ts`: payload building and calculated field integration.
- `src/components/forms/DynamicForm.tsx`: snapshot, recalculate, submit-reference, and stale-price flow.
- `src/components/forms/DynamicFormObjectList.tsx`: unit-price and line-total rendering.
- `src/components/forms/DynamicFormSummary.tsx`: summary presentation.
- `src/components/DynamicPageSections.tsx`: pass page/component identity into forms.
- `src/utils/dynamic.ts`: include form configuration references in workflow requests.

---

### Task 1: Persist and validate calculation configuration in `autotable-Go`

**Files:**
- Modify: `models/pageModel.go`
- Modify: `models/frontendValidation.go`
- Modify: `models/models_test.go`
- Create: `models/form_calculation_validation_test.go`

**Interfaces:**
- Produces: `FormFieldMappingConfig`, `FormItemCalculationConfig`, `FormSummaryConfig`, and `FormValueFormatConfig`.
- Produces: `validateFormCalculationConfig(form FormComponentConfig) error`, called from existing form validation.

- [ ] **Step 1: Write failing JSON/BSON round-trip tests**

Add a form fixture containing one `price -> unitPrice` mapping, one `multiply` item calculation, and `sum`/`copy` summaries. Assert every field survives both JSON and BSON round trips in `models/models_test.go`.

- [ ] **Step 2: Run the model test and verify it fails**

Run: `go test ./models -run 'TestPageModel.*Form.*Calculation' -count=1`

Expected: FAIL because the new fields and structs do not exist.

- [ ] **Step 3: Add the persisted model types**

Add these exact contracts and fields:

```go
type FormFieldMappingConfig struct {
    SourceFormKey string `bson:"sourceFormKey" json:"sourceFormKey"`
    SourceField   string `bson:"sourceField" json:"sourceField"`
    TargetField   string `bson:"targetField" json:"targetField"`
    Required      bool   `bson:"required,omitempty" json:"required,omitempty"`
}

type FormItemCalculationConfig struct {
    Operation   string   `bson:"operation" json:"operation"`
    Inputs      []string `bson:"inputs" json:"inputs"`
    TargetField string   `bson:"targetField" json:"targetField"`
    Precision   *int     `bson:"precision,omitempty" json:"precision,omitempty"`
}

type FormValueFormatConfig struct {
    Style     string `bson:"style,omitempty" json:"style,omitempty"`
    Currency  string `bson:"currency,omitempty" json:"currency,omitempty"`
    Precision *int   `bson:"precision,omitempty" json:"precision,omitempty"`
}

type FormSummaryConfig struct {
    Key           string                 `bson:"key" json:"key"`
    Label         string                 `bson:"label,omitempty" json:"label,omitempty"`
    Area          string                 `bson:"area,omitempty" json:"area,omitempty"`
    Order         int                    `bson:"order,omitempty" json:"order,omitempty"`
    Operation     string                 `bson:"operation" json:"operation"`
    ObjectListKey string                 `bson:"objectListKey,omitempty" json:"objectListKey,omitempty"`
    SourceField   string                 `bson:"sourceField" json:"sourceField"`
    TargetField   string                 `bson:"targetField" json:"targetField"`
    Format        *FormValueFormatConfig `bson:"format,omitempty" json:"format,omitempty"`
}
```

Add `FieldMappings []FormFieldMappingConfig` and `ItemCalculations []FormItemCalculationConfig` to `FormObjectListConfig`, and `Summaries []FormSummaryConfig` to `FormComponentConfig`.

- [ ] **Step 4: Write table-driven validation tests**

Cover valid order configuration plus missing mapping fields, non-schema select sources, duplicate targets, unknown inputs, wrong multiply arity, unsupported operations, overwritten inputs, unknown list/source fields, forward summary references, duplicate summary targets, invalid currency, invalid precision, and parent-field collisions.

- [ ] **Step 5: Run validation tests and verify they fail**

Run: `go test ./models -run 'TestValidateFormCalculationConfig' -count=1`

Expected: FAIL because validation is absent.

- [ ] **Step 6: Implement ordered, reference-aware validation**

Build the available item-field set from `itemFields`, mapping targets, and each earlier calculation target. Build the available summary set only from earlier summary targets. Call the validator from the existing `ValidateFormComponentConfig` function.

- [ ] **Step 7: Run the focused and package tests**

Run: `go test ./models -count=1`

Expected: PASS.

- [ ] **Step 8: Commit the backend model contract**

```bash
git add models/pageModel.go models/frontendValidation.go models/models_test.go models/form_calculation_validation_test.go
git commit -m "feat: add form cart calculation configuration"
```

### Task 2: Build the deterministic frontend calculation engine in `tenantPanel`

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/types/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/page.ts`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/formCalculations.ts`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/formCalculations.test.ts`

**Interfaces:**
- Produces: `snapshotMappedFields`, `calculateObjectListItem`, `calculateFormSummaries`, and `recalculateFormState`.
- Produces: `FormCalculationError` with codes `missing_mapping`, `invalid_number`, and `invalid_operation`.

- [ ] **Step 1: Add failing shared-fixture tests**

Use this fixture in the test file:

```ts
const items = [
  { unitPrice: 19.99, quantity: 3 },
  { unitPrice: 5.25, quantity: 2 },
];
// line totals: 59.97 and 10.50; subtotal and total: 70.47
```

Test required mapping failure, multiplication, ordered `sum` then `copy`, empty-list zero summaries, and precision values `0`, `2`, and `6`.

- [ ] **Step 2: Run the tests and verify failure**

Run from `tenantPanel`: `yarn test src/utils/formCalculations.test.ts`

Expected: FAIL because the module does not exist.

- [ ] **Step 3: Add matching TypeScript configuration types**

Use discriminated operation unions:

```ts
export type FormItemCalculationOperation = "multiply";
export type FormSummaryOperation = "sum" | "copy";
export interface FormCalculationError extends Error {
  code: "missing_mapping" | "invalid_number" | "invalid_operation";
  field?: string;
}
```

Mirror the Go JSON names exactly.

- [ ] **Step 4: Implement decimal-safe integer scaling**

Implement `toScaledInteger(value, precision)` and `fromScaledInteger(value, precision)` without adding a dependency. Reject non-finite inputs. Multiply two scaled integers and round once to the target precision; sum already-rounded line totals.

```ts
export function recalculateFormState(
  form: FormComponentConfig,
  state: FormElementsState,
): FormElementsState;
```

The function returns a new state and does not mutate items or the input state.

- [ ] **Step 5: Run tests and type-check**

Run from `tenantPanel`: `yarn test src/utils/formCalculations.test.ts && yarn build`

Expected: PASS.

- [ ] **Step 6: Commit the preview calculation engine**

```bash
git -C /Users/osmansamilerdogan/Desktop/tenantPanel add src/types/page.ts src/utils/api/page.ts src/utils/formCalculations.ts src/utils/formCalculations.test.ts
git -C /Users/osmansamilerdogan/Desktop/tenantPanel commit -m "feat: add form calculation engine"
```

### Task 3: Mirror and lock the calculation engine in `react-template`

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/types/page.ts`
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/formCalculations.ts`
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/formCalculations.test.ts`

**Interfaces:**
- Consumes: the exact contracts and fixtures from Task 2.
- Produces: the same four exported functions and error codes for production runtime use.

- [ ] **Step 1: Copy the exact Task 2 fixtures into a failing runtime test**

Do not weaken or rewrite expected values; identical fixtures are the drift detector.

- [ ] **Step 2: Run the focused runtime test and verify failure**

Run from `react-template`: `yarn test src/utils/formCalculations.test.ts`

Expected: FAIL because the module does not exist.

- [ ] **Step 3: Mirror the contracts and utility implementation**

Keep function signatures, operation names, rounding semantics, and errors byte-for-byte equivalent where repository imports permit.

- [ ] **Step 4: Run focused tests and build**

Run from `react-template`: `yarn test src/utils/formCalculations.test.ts && yarn build`

Expected: PASS.

- [ ] **Step 5: Commit the production calculation engine**

```bash
git -C /Users/osmansamilerdogan/Desktop/react-template add src/types/page.ts src/utils/formCalculations.ts src/utils/formCalculations.test.ts
git -C /Users/osmansamilerdogan/Desktop/react-template commit -m "feat: add form calculation engine"
```

### Task 4: Integrate calculations and summaries into `react-template`

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/formConfig.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/formConfig.test.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/components/forms/DynamicForm.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/components/forms/DynamicFormObjectList.tsx`
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/components/forms/DynamicFormSummary.tsx`

**Interfaces:**
- Consumes: `snapshotMappedFields(...)` and `recalculateFormState(...)` from Task 3.
- Produces: live calculated state and persisted summary fields in `buildFormSubmitRequestBody`.

- [ ] **Step 1: Add failing utility tests for calculated payloads**

Assert that mapped/calculated item fields and `subtotal`/`total` are included, while transient picker fields remain excluded.

- [ ] **Step 2: Run the focused test and verify failure**

Run from `react-template`: `yarn test src/utils/formConfig.test.ts`

Expected: FAIL because calculated targets are not included.

- [ ] **Step 3: Recalculate on every cart mutation**

In `handleAddObject`, obtain the selected option's `sourceItem`, call `snapshotMappedFields`, evaluate the new item, update the list, then call `recalculateFormState`. Wrap edit, remove, increment, and decrement state transitions with the same recalculation. Convert `FormCalculationError` into a toast and preserve existing cart state on failure.

- [ ] **Step 4: Render item money and the summary panel**

Add `DynamicFormSummary` with this public contract:

```ts
type Props = {
  summaries: FormSummaryConfig[];
  values: FormElementsState;
  area: FormAreaKey;
};
```

Use `Intl.NumberFormat` with each summary's currency and precision. Extend object-list display templates to render `{{unitPrice}}` and `{{lineTotal}}`; do not add hard-coded product field names.

- [ ] **Step 5: Run tests and production build**

Run from `react-template`: `yarn test src/utils/formConfig.test.ts src/utils/formCalculations.test.ts && yarn build`

Expected: PASS.

- [ ] **Step 6: Commit runtime behavior**

```bash
git -C /Users/osmansamilerdogan/Desktop/react-template add src/utils/formConfig.ts src/utils/formConfig.test.ts src/components/forms/DynamicForm.tsx src/components/forms/DynamicFormObjectList.tsx src/components/forms/DynamicFormSummary.tsx
git -C /Users/osmansamilerdogan/Desktop/react-template commit -m "feat: render live form cart totals"
```

### Task 5: Integrate identical preview behavior into `tenantPanel`

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/formConfig.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/formConfig.test.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/forms/DynamicForm.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/forms/DynamicFormObjectList.tsx`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/forms/DynamicFormSummary.tsx`

**Interfaces:**
- Consumes: Task 2's engine.
- Produces: preview behavior equivalent to Task 4.

- [ ] **Step 1: Add the same calculated-payload assertions used in Task 4**

- [ ] **Step 2: Run the preview utility tests and verify failure**

Run from `tenantPanel`: `yarn test src/utils/formConfig.test.ts`

Expected: FAIL on missing snapshot or summary fields.

- [ ] **Step 3: Mirror Task 4 runtime integration**

Use the same mutation sequence, error codes, `DynamicFormSummary` props, and `Intl.NumberFormat` behavior. Keep preview-only visual wrappers unchanged.

- [ ] **Step 4: Run focused tests and build**

Run from `tenantPanel`: `yarn test src/utils/formConfig.test.ts src/utils/formCalculations.test.ts && yarn build`

Expected: PASS.

- [ ] **Step 5: Commit preview behavior**

```bash
git -C /Users/osmansamilerdogan/Desktop/tenantPanel add src/utils/formConfig.ts src/utils/formConfig.test.ts src/components/forms/DynamicForm.tsx src/components/forms/DynamicFormObjectList.tsx src/components/forms/DynamicFormSummary.tsx
git -C /Users/osmansamilerdogan/Desktop/tenantPanel commit -m "feat: preview live form cart totals"
```

### Task 6: Add designer controls and client-side configuration validation

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/FormComponentEditor.tsx`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/formCalculationEditor.ts`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/formCalculationEditor.test.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/PageDesigner.tsx`

**Interfaces:**
- Produces: `validateDesignerCalculations(form): string[]` and immutable add/update/remove helpers.
- Consumes: schema field metadata already supplied to `FormComponentEditor`.

- [ ] **Step 1: Write failing editor-helper tests**

Test adding/removing mappings, preserving zero precision, operation defaults, ordered summary references, duplicate targets, stale references after a field rename, currency validation, and normalized save output.

- [ ] **Step 2: Run the helper test and verify failure**

Run from `tenantPanel`: `yarn test src/components/PageDesigner/formCalculationEditor.test.ts`

Expected: FAIL because the helper module does not exist.

- [ ] **Step 3: Implement focused immutable helpers**

Keep list manipulation and validation outside the already-large editor component. Export exact functions:

```ts
addFieldMapping(form, objectListIndex): FormComponentConfig
updateFieldMapping(form, objectListIndex, mappingIndex, patch): FormComponentConfig
removeFieldMapping(form, objectListIndex, mappingIndex): FormComponentConfig
addItemCalculation(form, objectListIndex): FormComponentConfig
addSummary(form): FormComponentConfig
validateDesignerCalculations(form): string[]
normalizeDesignerCalculations(form): FormComponentConfig
```

- [ ] **Step 4: Add the mapping, item-calculation, and summary panels**

Use selects for known schema/form/item fields, operation selectors limited to supported values, numeric precision inputs constrained to `0..6`, a three-character uppercase currency input, inline errors, and accessible add/remove buttons. Disable page save when calculation validation returns errors.

- [ ] **Step 5: Run tests, lint, and build**

Run from `tenantPanel`: `yarn test src/components/PageDesigner/formCalculationEditor.test.ts && yarn lint && yarn build`

Expected: PASS.

- [ ] **Step 6: Commit designer support**

```bash
git -C /Users/osmansamilerdogan/Desktop/tenantPanel add src/components/PageDesigner/FormComponentEditor.tsx src/components/PageDesigner/formCalculationEditor.ts src/components/PageDesigner/formCalculationEditor.test.ts src/components/PageDesigner/PageDesigner.tsx
git -C /Users/osmansamilerdogan/Desktop/tenantPanel commit -m "feat: configure form cart calculations"
```

### Task 7: Carry trusted page/component references through workflow submission

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/components/DynamicPageSections.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/components/forms/DynamicForm.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/dynamic.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/pages/PagePreviewPage.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/forms/DynamicForm.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/dynamic.ts`
- Modify: `controllers/dynamicController.go`
- Modify: `controllers/error_paths_test.go`

**Interfaces:**
- Produces frontend request field: `formConfigRef: { pageId: string; componentId: string }`.
- Produces backend value object: `FormConfigReference{PageID, ComponentID string}` separated from the workflow record.

- [ ] **Step 1: Add failing backend request-parser tests**

Assert a wrapped workflow request extracts `formConfigRef`, keeps it out of `record`, accepts requests without it for backward compatibility, and rejects a partial reference.

- [ ] **Step 2: Run the parser tests and verify failure**

Run: `go test ./controllers -run 'TestParseWorkflowRequestBody.*FormConfigRef' -count=1`

Expected: FAIL because the parser does not expose the reference.

- [ ] **Step 3: Extend workflow request parsing**

Introduce:

```go
type FormConfigReference struct {
    PageID      string `json:"pageId"`
    ComponentID string `json:"componentId"`
}
```

Return it separately from `parseWorkflowRequestBody`; thread it into `services.ExecuteWorkflowInput`. Keep existing callers working with `nil`.

- [ ] **Step 4: Pass identity from both form renderers**

Change `DynamicForm` props to include optional `pageId` and required `componentId` when calculation config exists. Include `formConfigRef` only for workflow submissions whose form declares mappings, calculations, or summaries. Preview and production derive IDs from their existing page and component objects.

- [ ] **Step 5: Add frontend request tests and run all focused tests**

Run from each frontend: `yarn test src/utils/formConfig.test.ts && yarn build`

Run backend: `go test ./controllers ./services -count=1`

Expected: PASS.

- [ ] **Step 6: Commit reference transport in each repository**

```bash
git -C /Users/osmansamilerdogan/Desktop/react-template add src/components/DynamicPageSections.tsx src/components/forms/DynamicForm.tsx src/utils/dynamic.ts
git -C /Users/osmansamilerdogan/Desktop/react-template commit -m "feat: reference form config in workflow requests"
git -C /Users/osmansamilerdogan/Desktop/tenantPanel add src/pages/PagePreviewPage.tsx src/components/forms/DynamicForm.tsx src/utils/dynamic.ts
git -C /Users/osmansamilerdogan/Desktop/tenantPanel commit -m "feat: reference preview form config"
git add controllers/dynamicController.go controllers/error_paths_test.go services/dynamic_service.go
git commit -m "feat: parse workflow form config references"
```

### Task 8: Recalculate and verify authoritative cart values in `autotable-Go`

**Files:**
- Create: `services/form_cart_calculations.go`
- Create: `services/form_cart_calculations_test.go`
- Create: `services/form_cart_verifier.go`
- Create: `services/form_cart_verifier_test.go`
- Modify: `services/dynamic_service.go`

**Interfaces:**
- Consumes: `ExecuteWorkflowInput.FormConfigRef` from Task 7 and model contracts from Task 1.
- Produces: `EvaluateFormCart(form models.FormComponentConfig, record map[string]interface{}) (map[string]interface{}, error)`.
- Produces stable errors: `FORM_INVALID_QUANTITY`, `FORM_PRODUCT_NOT_FOUND`, `FORM_PRODUCT_PRICE_MISSING`, `FORM_STALE_PRICE`, and `FORM_CONFIG_NOT_FOUND`.

- [ ] **Step 1: Write failing pure evaluator tests**

Use the same `19.99 * 3 + 5.25 * 2 = 70.47` fixture as both frontends. Test precision, empty lists, missing values, negative/zero quantity, and nonmutation of the input record.

- [ ] **Step 2: Run the evaluator test and verify failure**

Run: `go test ./services -run 'TestEvaluateFormCart' -count=1`

Expected: FAIL because the evaluator does not exist.

- [ ] **Step 3: Implement scaled-integer evaluation**

Use `math/big.Rat` or decimal-string-to-scaled-integer helpers so binary floating-point does not determine persisted prices. Apply mappings before calculations, round at each configured target, and execute declarations in order.

- [ ] **Step 4: Write failing verifier tests**

Build repository fakes for page lookup and product lookup. Cover valid replacement, client tampering, current-price change, missing page/component, wrong workflow component, missing product, and missing product price.

- [ ] **Step 5: Run verifier tests and verify failure**

Run: `go test ./services -run 'TestVerifyWorkflowFormCart' -count=1`

Expected: FAIL because persisted config/product resolution is absent.

- [ ] **Step 6: Resolve trusted configuration and products**

Load the page from `utils.GetPageCollectionForProject`, locate the exact component ID via the existing page-component traversal pattern, verify its workflow schema/name match the request, batch-query referenced products through `DynamicRepository.Query`, replace mapped source fields with database values, evaluate the authoritative record, compare every configured financial target, and return the authoritative clone.

- [ ] **Step 7: Invoke verification before workflow execution**

In `DynamicService.ExecuteWorkflow`, after tenant/project/container resolution and before `runWorkflowDefinition`, call the verifier when `FormConfigRef != nil`. On success replace `input.Record`; on difference return HTTP 409 semantics with `FORM_STALE_PRICE` and affected item indexes. Other validation failures return 422; missing trusted configuration returns 404.

- [ ] **Step 8: Run service and full backend tests**

Run: `gofmt -w models/pageModel.go models/frontendValidation.go models/form_calculation_validation_test.go services/form_cart_calculations.go services/form_cart_calculations_test.go services/form_cart_verifier.go services/form_cart_verifier_test.go controllers/dynamicController.go controllers/error_paths_test.go services/dynamic_service.go`

Run: `go test ./... -count=1`

Expected: PASS.

- [ ] **Step 9: Commit authoritative verification**

```bash
git add services/form_cart_calculations.go services/form_cart_calculations_test.go services/form_cart_verifier.go services/form_cart_verifier_test.go services/dynamic_service.go
git commit -m "feat: verify workflow cart totals"
```

### Task 9: Exercise the supplied order configuration and complete cross-repository verification

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/formCalculationEditor.test.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/formCalculations.test.ts`
- Modify: `services/form_cart_verifier_test.go`
- Create: `docs/examples/davinci-order-form-calculations.json`

**Interfaces:**
- Consumes: all prior task contracts.
- Produces: copyable configuration fragment for `Sipariş Girişi`.

- [ ] **Step 1: Add the supplied form as a cross-project fixture**

Create the example fragment with:

```json
{
  "objectListKey": "items",
  "fieldMappings": [{
    "sourceFormKey": "productId",
    "sourceField": "price",
    "targetField": "unitPrice",
    "required": true
  }],
  "itemCalculations": [{
    "operation": "multiply",
    "inputs": ["unitPrice", "quantity"],
    "targetField": "lineTotal",
    "precision": 2
  }],
  "summaries": [
    {"key":"subtotal","label":"Ara Toplam","area":"right","operation":"sum","objectListKey":"items","sourceField":"lineTotal","targetField":"subtotal","format":{"style":"currency","currency":"TRY","precision":2}},
    {"key":"total","label":"Toplam","area":"right","operation":"copy","sourceField":"subtotal","targetField":"total","format":{"style":"currency","currency":"TRY","precision":2}}
  ]
}
```

Use the same logical fixture in the designer normalization test, frontend runtime test, and backend verifier test.

- [ ] **Step 2: Run repository test suites**

Run backend: `go test ./... -count=1`

Run from `tenantPanel`: `yarn test && yarn lint && yarn build`

Run from `react-template`: `yarn test && yarn lint && yarn build`

Expected: all commands PASS.

- [ ] **Step 3: Verify clean diffs and configuration parity**

Run in each repository: `git status --short` and `git diff --check`.

Expected: no uncommitted files from the feature and no whitespace errors. Compare the two `formCalculations.test.ts` fixture blocks and confirm their inputs and expected values are identical.

- [ ] **Step 4: Commit the example and final fixture coverage**

```bash
git add docs/examples/davinci-order-form-calculations.json services/form_cart_verifier_test.go
git commit -m "docs: add davinci order calculation example"
git -C /Users/osmansamilerdogan/Desktop/tenantPanel add src/components/PageDesigner/formCalculationEditor.test.ts
git -C /Users/osmansamilerdogan/Desktop/tenantPanel commit -m "test: cover davinci order calculation config"
git -C /Users/osmansamilerdogan/Desktop/react-template add src/utils/formCalculations.test.ts
git -C /Users/osmansamilerdogan/Desktop/react-template commit -m "test: cover davinci order calculations"
```
