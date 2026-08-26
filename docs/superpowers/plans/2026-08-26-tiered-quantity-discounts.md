# Tiered Quantity Discounts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add progressive row-level quantity discount tiers and a compact next-tier action to the cart in both frontend applications.

**Architecture:** Extend the shared JSON contract with `discountTiers`, keeping the existing threshold fields as a legacy single-tier fallback. Centralize tier selection in small calculation helpers, keep Go submission calculation authoritative, and let each cart row use the configured quantity input plus its existing `onAdjust` callback to jump to the next tier.

**Tech Stack:** Go, MongoDB/BSON models, TypeScript, React, Tailwind CSS, Vitest, React server rendering tests

**Spec:** `docs/superpowers/specs/2026-08-26-tiered-quantity-discounts-design.md`

## Global Constraints

- The highest reached tier applies to every unit in that object-list row only.
- Tier quantities and percentages are finite, greater than zero, unique, and strictly ascending; percentages cannot exceed 100.
- `discountTiers` is authoritative when non-empty; legacy `minimumQuantity` plus `discountPercentage` remains a supported one-tier fallback.
- The UI offers only the next tier with compact copy in the form `+3 → %30` and hides the action at the highest tier.
- Clicking the offer sets the row quantity exactly to the next threshold and uses the existing recalculation path.
- The Go workflow calculation remains authoritative for submitted values.
- Keep `tenantPanel` and `react-template` runtime behavior identical.
- Do not modify the unrelated existing changes in `services/dynamic_array_service.go` or `services/dynamic_array_service_test.go`.

---

### Task 1: Backend Tier Contract and Validation

**Files:**
- Modify: `models/pageModel.go`
- Modify: `models/frontendValidation.go`
- Test: `models/form_calculation_validation_test.go`
- Test: `models/models_test.go`

**Interfaces:**
- Produces: `models.FormQuantityDiscountTierConfig` with `MinimumQuantity *float64` and `DiscountPercentage *float64`.
- Produces: `FormItemCalculationConfig.DiscountTiers []FormQuantityDiscountTierConfig` serialized as `discountTiers,omitempty`.
- Consumes: existing legacy `MinimumQuantity` and `DiscountPercentage` fields.

- [ ] **Step 1: Write failing model and validation tests**

Add a round-trip assertion in `models/models_test.go` for:

```go
DiscountTiers: []FormQuantityDiscountTierConfig{
    {MinimumQuantity: float64Ptr(6), DiscountPercentage: float64Ptr(30)},
    {MinimumQuantity: float64Ptr(10), DiscountPercentage: float64Ptr(40)},
},
```

Extend `models/form_calculation_validation_test.go` with table cases covering a valid two-tier calculation, empty tiers without a valid legacy pair, partial tier values, zero/non-finite quantities, percentages outside `(0, 100]`, duplicate or descending quantities, and equal or descending percentages. Assert contextual errors such as `discount tier 1 minimumQuantity must be greater than 0` and `discount tiers must have strictly ascending minimumQuantity`.

- [ ] **Step 2: Run the focused tests and verify failure**

Run: `go test ./models -run 'Test.*(Form|Calculation)'`

Expected: FAIL because `FormQuantityDiscountTierConfig` and `DiscountTiers` do not exist.

- [ ] **Step 3: Add the model and a focused validator**

In `models/pageModel.go`, add:

```go
type FormQuantityDiscountTierConfig struct {
    MinimumQuantity    *float64 `bson:"minimumQuantity" json:"minimumQuantity"`
    DiscountPercentage *float64 `bson:"discountPercentage" json:"discountPercentage"`
}
```

Add `DiscountTiers []FormQuantityDiscountTierConfig` to `FormItemCalculationConfig`. In `models/frontendValidation.go`, extract a helper that validates either the non-empty tier list or the complete legacy pair. Iterate in supplied order, validate finite ranges, and compare each entry with its predecessor for strictly ascending quantities and percentages. Keep the existing output-field validation unchanged.

- [ ] **Step 4: Run and format the backend model tests**

Run: `gofmt -w models/pageModel.go models/frontendValidation.go models/form_calculation_validation_test.go models/models_test.go`

Run: `go test ./models -run 'Test.*(Form|Calculation)'`

Expected: PASS.

- [ ] **Step 5: Commit the backend contract**

```bash
git add models/pageModel.go models/frontendValidation.go models/form_calculation_validation_test.go models/models_test.go
git commit -m "feat: validate tiered quantity discounts"
```

### Task 2: Authoritative Backend Tier Calculation

**Files:**
- Modify: `services/form_cart_calculations.go`
- Test: `services/form_cart_calculations_test.go`

**Interfaces:**
- Consumes: `FormItemCalculationConfig.DiscountTiers` from Task 1.
- Produces: `formCartDiscountPercentage(calculation models.FormItemCalculationConfig, quantity float64) (float64, bool)`.

- [ ] **Step 1: Write failing calculation tests**

Add cases with tiers `6/30` and `10/40` asserting:

```go
// unit price 100
quantity 3  => original 300, discounted 300
quantity 6  => original 600, discounted 420
quantity 8  => original 800, discounted 560
quantity 10 => original 1000, discounted 600
quantity 12 => original 1200, discounted 720
```

Include two cart rows at different quantities to prove row isolation, plus the existing legacy one-tier case.

- [ ] **Step 2: Run the focused service tests and verify failure**

Run: `go test ./services -run TestEvaluateFormCart`

Expected: FAIL because only the legacy threshold is evaluated.

- [ ] **Step 3: Implement highest-reached-tier selection**

Add a helper that walks the already validated ascending list and retains the last percentage whose minimum is `<= quantity`. If `DiscountTiers` is empty, evaluate the legacy pair. Replace the direct legacy check in `EvaluateFormCart` with:

```go
percentage, qualified := formCartDiscountPercentage(calculation, right)
if qualified {
    itemCopy[calculation.TargetField] = formCartApplyPercentageDiscount(original, percentage, precision)
}
```

Return `FORM_CONFIG_NOT_FOUND` if neither a valid tier list nor a valid legacy pair is available at runtime.

- [ ] **Step 4: Format and run service tests**

Run: `gofmt -w services/form_cart_calculations.go services/form_cart_calculations_test.go`

Run: `go test ./services -run TestEvaluateFormCart`

Expected: PASS.

- [ ] **Step 5: Commit authoritative calculation support**

```bash
git add services/form_cart_calculations.go services/form_cart_calculations_test.go
git commit -m "feat: calculate tiered row discounts"
```

### Task 3: React Template Types and Calculation Helpers

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/types/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/formCalculations.ts`
- Test: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/formCalculations.test.ts`

**Interfaces:**
- Produces: `FormQuantityDiscountTierConfig { minimumQuantity: number; discountPercentage: number }`.
- Produces: optional `FormItemCalculationConfig.discountTiers`.
- Produces: `getQuantityDiscountTiers(calculation): FormQuantityDiscountTierConfig[]` and `getNextQuantityDiscountTier(calculation, quantity): FormQuantityDiscountTierConfig | undefined`.
- Preserves: legacy single-tier calculations.

- [ ] **Step 1: Write failing tier-helper and calculation tests**

Test that `getQuantityDiscountTiers` returns configured tiers and converts a valid legacy pair into one tier. Test `getNextQuantityDiscountTier` at quantities 3, 6, 8, and 10. Extend calculation cases to assert the totals from Task 2 and that the full row receives the selected percentage.

- [ ] **Step 2: Run the focused tests and verify failure**

Run from `/Users/osmansamilerdogan/Desktop/react-template`: `yarn test src/utils/formCalculations.test.ts`

Expected: FAIL because the tier types and exported helpers do not exist.

- [ ] **Step 3: Add types and minimal shared helpers**

Add the interface and optional array to `src/types/page.ts`. In `formCalculations.ts`, implement:

```ts
export const getQuantityDiscountTiers = (calculation: FormItemCalculationConfig) =>
  calculation.discountTiers?.length
    ? calculation.discountTiers
    : calculation.minimumQuantity !== undefined && calculation.discountPercentage !== undefined
      ? [{ minimumQuantity: calculation.minimumQuantity, discountPercentage: calculation.discountPercentage }]
      : [];

export const getNextQuantityDiscountTier = (
  calculation: FormItemCalculationConfig,
  quantity: number,
) => getQuantityDiscountTiers(calculation).find((tier) => tier.minimumQuantity > quantity);
```

Update `calculateObjectListItem` to walk the normalized tiers and apply the percentage from the last tier reached.

- [ ] **Step 4: Run focused tests and the TypeScript build**

Run: `yarn test src/utils/formCalculations.test.ts`

Run: `yarn build`

Expected: both PASS.

- [ ] **Step 5: Commit template calculation support**

```bash
git add src/types/page.ts src/utils/formCalculations.ts src/utils/formCalculations.test.ts
git commit -m "feat: calculate tiered quantity discounts"
```

### Task 4: React Template Compact Next-Tier Action

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/components/forms/DynamicFormObjectList.tsx`
- Test: `/Users/osmansamilerdogan/Desktop/react-template/src/components/forms/DynamicFormObjectList.test.ts`

**Interfaces:**
- Consumes: `getNextQuantityDiscountTier` from Task 3.
- Consumes: existing `onAdjust(index, field, delta, min?, max?)` callback.
- Produces: one next-tier button per eligible row with visible label `+N → %P`.

- [ ] **Step 1: Write failing cart-row rendering tests**

Configure `inputs: ["unitPrice", "quantity"]` with tiers `6/30` and `10/40`. Assert quantity 3 renders `+3 → %30`, quantity 6 renders `+4 → %40`, quantity 8 renders `+2 → %40`, and quantity 10 renders no tier action. Assert the button title/accessible name describes adding the missing units for the resulting discount.

Use an interactive Vitest render or invoke the button handler from the rendered element tree to assert that clicking at quantity 3 calls:

```ts
onAdjust(0, "quantity", 3, undefined, undefined)
```

- [ ] **Step 2: Run the component test and verify failure**

Run: `yarn test src/components/forms/DynamicFormObjectList.test.ts`

Expected: FAIL because no tier action is rendered.

- [ ] **Step 3: Render the next tier through the existing adjust path**

For each row, locate the first `quantityDiscount` calculation with two inputs, read the quantity from its second input, and call `getNextQuantityDiscountTier`. Render a `type="button"` beneath the secondary text using compact neutral/emerald Tailwind classes and visible copy:

```tsx
<span>+{missingQuantity} → %{tier.discountPercentage}</span>
```

Give it a full English accessible label such as `Add 3 items to unlock 30% discount`. On click call `onAdjust(index, quantityField, missingQuantity)`. Do not render for non-finite quantity, non-positive gap, or absent next tier.

- [ ] **Step 4: Run component tests and build**

Run: `yarn test src/components/forms/DynamicFormObjectList.test.ts src/utils/formCalculations.test.ts`

Run: `yarn build`

Expected: both PASS.

- [ ] **Step 5: Commit the template interaction**

```bash
git add src/components/forms/DynamicFormObjectList.tsx src/components/forms/DynamicFormObjectList.test.ts
git commit -m "feat: offer next discount tier in cart rows"
```

### Task 5: Tenant Panel Runtime Parity

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/types/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/index.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/formCalculations.ts`
- Test: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/formCalculations.test.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/forms/DynamicFormObjectList.tsx`
- Test: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/forms/DynamicFormObjectList.test.ts`

**Interfaces:**
- Mirrors: Task 3 tier types and helper signatures in all tenant-panel API/runtime type layers.
- Mirrors: Task 4 cart-row behavior and copy exactly.

- [ ] **Step 1: Port the Task 3 and Task 4 tests before runtime code**

Add the same tier-selection, calculated-total, legacy fallback, compact-label, click-delta, accessibility, and highest-tier hiding cases to the tenant-panel test files.

- [ ] **Step 2: Run tenant-panel focused tests and verify failure**

Run from `/Users/osmansamilerdogan/Desktop/tenantPanel`: `yarn test src/utils/formCalculations.test.ts src/components/forms/DynamicFormObjectList.test.ts`

Expected: FAIL because tenant-panel types and runtime code do not support tiers.

- [ ] **Step 3: Mirror the proven runtime implementation**

Add `FormQuantityDiscountTierConfig` and `discountTiers` to `src/types/page.ts`, `src/utils/api/page.ts`, and the legacy/public types in `src/utils/index.ts`. Port the Task 3 helpers and calculation branch to `src/utils/formCalculations.ts`, then port the Task 4 row action to `DynamicFormObjectList.tsx`. Keep function names, fallback rules, copy, and styling identical.

- [ ] **Step 4: Run tenant-panel focused tests and build**

Run: `yarn test src/utils/formCalculations.test.ts src/components/forms/DynamicFormObjectList.test.ts`

Run: `yarn build`

Expected: both PASS.

- [ ] **Step 5: Commit tenant runtime parity**

```bash
git add src/types/page.ts src/utils/api/page.ts src/utils/index.ts src/utils/formCalculations.ts src/utils/formCalculations.test.ts src/components/forms/DynamicFormObjectList.tsx src/components/forms/DynamicFormObjectList.test.ts
git commit -m "feat: support tiered discounts in tenant cart"
```

### Task 6: Tenant Page Designer Tier Editing

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/formCalculationEditor.ts`
- Test: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/formCalculationEditor.test.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/FormComponentEditor.tsx`
- Test: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/PageDesigner.formSave.test.ts`

**Interfaces:**
- Consumes: tenant `FormQuantityDiscountTierConfig` from Task 5.
- Produces: `addDiscountTier`, `updateDiscountTier`, and `removeDiscountTier` immutable editor helpers.
- Produces: normalized and validated `discountTiers` in saved page JSON.

- [ ] **Step 1: Write failing editor-helper tests**

Test that switching a calculation to `quantityDiscount` initializes `discountTiers` with `{ minimumQuantity: 6, discountPercentage: 30 }`. Test adding a tier defaults beyond the prior tier (10/40), editing either value, removing a tier, and leaving the original form object unchanged. Test normalization preserves numeric values and save serialization includes both tiers.

Add validation cases for empty arrays, invalid ranges, duplicate/descending quantities, and equal/descending percentages. Keep a valid legacy pair accepted.

- [ ] **Step 2: Run editor tests and verify failure**

Run: `yarn test src/components/PageDesigner/formCalculationEditor.test.ts src/components/PageDesigner/PageDesigner.formSave.test.ts`

Expected: FAIL because tier editor helpers and serialization do not exist.

- [ ] **Step 3: Implement immutable helpers, normalization, and validation**

Add helpers with signatures:

```ts
addDiscountTier(form, listIndex, calculationIndex): FormComponentConfig
updateDiscountTier(form, listIndex, calculationIndex, tierIndex, patch): FormComponentConfig
removeDiscountTier(form, listIndex, calculationIndex, tierIndex): FormComponentConfig
```

Update the operation-switch helper to initialize the first tier. Extend `validateDesignerCalculations` with the backend rules and contextual messages. Preserve `discountTiers` in `normalizeDesignerCalculations`; do not synthesize tiers for untouched legacy calculations.

- [ ] **Step 4: Add compact tier controls to the calculation editor**

Inside the existing `quantityDiscount` branch in `FormComponentEditor.tsx`, render each tier as two labeled numeric inputs and a remove icon, followed by an `Add discount tier` button. Use the immutable helpers for every change. Retain legacy field inputs only when the loaded calculation has no tier array so old pages remain editable without an automatic representation change.

- [ ] **Step 5: Run editor tests and tenant build**

Run: `yarn test src/components/PageDesigner/formCalculationEditor.test.ts src/components/PageDesigner/PageDesigner.formSave.test.ts`

Run: `yarn build`

Expected: both PASS.

- [ ] **Step 6: Commit designer support**

```bash
git add src/components/PageDesigner/formCalculationEditor.ts src/components/PageDesigner/formCalculationEditor.test.ts src/components/PageDesigner/FormComponentEditor.tsx src/components/PageDesigner/PageDesigner.formSave.test.ts
git commit -m "feat: edit quantity discount tiers"
```

### Task 7: Da Vinci Example Configuration

**Files:**
- Modify: `docs/examples/davinci-order-form-calculations.json`
- Modify: `docs/examples/davinci-orders-page.json`
- Modify: `docs/examples/davinci-orders-page-second-project.json`

**Interfaces:**
- Consumes: `discountTiers` JSON contract from Task 1.
- Produces: example pages with 6/30 and 10/40 row discount tiers.

- [ ] **Step 1: Replace the legacy threshold pair in all examples**

Use:

```json
"discountTiers": [
  { "minimumQuantity": 6, "discountPercentage": 30 },
  { "minimumQuantity": 10, "discountPercentage": 40 }
]
```

Remove the top-level `minimumQuantity` and `discountPercentage` from those example calculations while preserving inputs, output fields, precision, comparison display, and summary source.

- [ ] **Step 2: Validate JSON and run backend validation tests**

Run: `jq empty docs/examples/davinci-order-form-calculations.json docs/examples/davinci-orders-page.json docs/examples/davinci-orders-page-second-project.json`

Run: `go test ./models ./services`

Expected: both PASS.

- [ ] **Step 3: Commit updated examples**

```bash
git add docs/examples/davinci-order-form-calculations.json docs/examples/davinci-orders-page.json docs/examples/davinci-orders-page-second-project.json
git commit -m "docs: configure Da Vinci discount tiers"
```

### Task 8: Cross-Project Verification

**Files:**
- Verify only; modify only if a test exposes a defect in files already listed above.

**Interfaces:**
- Consumes: all completed tasks.
- Produces: verified backend, designer, and storefront behavior with no unrelated changes included.

- [ ] **Step 1: Run the complete backend suite**

Run from `/Users/osmansamilerdogan/Desktop/autotable-Go`: `go test ./...`

Expected: PASS.

- [ ] **Step 2: Run the complete react-template checks**

Run from `/Users/osmansamilerdogan/Desktop/react-template`: `yarn test`

Run: `yarn build`

Expected: both PASS.

- [ ] **Step 3: Run the complete tenant-panel checks**

Run from `/Users/osmansamilerdogan/Desktop/tenantPanel`: `yarn test`

Run: `yarn build`

Expected: both PASS.

- [ ] **Step 4: Review working trees and the example diff**

Run in each repository: `git status --short`

Run in `autotable-Go`: `git diff HEAD~3 -- docs/examples models services/form_cart_calculations.go`

Expected: implementation commits contain only scoped tier-discount changes; the pre-existing dynamic-array service edits remain untouched and uncommitted.

- [ ] **Step 5: Perform a manual cart smoke test**

Open the Da Vinci order form and add one row at quantity 3. Confirm `+3 → %30`; click and confirm quantity 6, a 30% row price, recalculated total, and `+4 → %40`; click again and confirm quantity 10, a 40% row price, recalculated total, and no further badge. Add another product and confirm its quantity and price are unaffected.

- [ ] **Step 6: Record any verification-only fixes**

If verification required scoped fixes, stage only the already listed source and test files that were corrected, inspect `git diff --cached`, and commit them in the repository they affect with:

```bash
git commit -m "fix: align tiered discount behavior"
```

If all checks pass without changes, do not create an empty commit.
