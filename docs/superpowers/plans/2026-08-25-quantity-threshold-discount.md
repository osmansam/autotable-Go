# Quantity-Threshold Item Discount Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a tenant-configurable per-line quantity discount with an original struck-through price, a discounted payable price, and matching backend recalculation.

**Architecture:** Extend the existing operation-based cart calculation model with `quantityDiscount`, producing `originalTargetField` and `targetField`. Add a typed `priceComparison` object-list display that renders safe React markup, mirror calculation behavior in both frontend projects, and keep Go workflow calculation authoritative for submitted calculated fields.

**Tech Stack:** Go, MongoDB BSON/JSON models, React 18, TypeScript, Vitest, Tailwind CSS, Yarn 4.

**Spec:** `docs/superpowers/specs/2026-08-25-quantity-threshold-discount-design.md`

## Global Constraints

- The threshold is evaluated independently per cart line; quantities from different products are never combined.
- The discount applies when `quantity >= minimumQuantity`.
- The supplied Da Vinci configuration uses minimum quantity `6`, discount percentage `30`, currency `TRY`, and precision `2`.
- Existing `multiply` calculations and `rightTemplate` displays remain backward compatible.
- Tenant-authored display text is not interpreted as HTML.
- Preserve pre-existing uncommitted changes in all three repositories, especially overlapping model and `src/types/page.ts` files.

## File Structure

- `models/pageModel.go`: Go persistence types for calculation and price-comparison configuration.
- `models/frontendValidation.go`: operation-specific and display-reference validation.
- `models/form_calculation_validation_test.go`: model round-trip and validation cases.
- `services/form_cart_calculations.go`: decimal-safe server calculation.
- `services/form_cart_calculations_test.go`: backend threshold and overwrite fixtures.
- `tenantPanel/src/types/page.ts` and `tenantPanel/src/utils/api/page.ts`: designer/runtime and API types.
- `tenantPanel/src/utils/formCalculations.ts`: preview calculation engine.
- `tenantPanel/src/components/PageDesigner/formCalculationEditor.ts`: normalization and designer validation.
- `tenantPanel/src/components/PageDesigner/FormComponentEditor.tsx`: operation and price-comparison controls.
- `tenantPanel/src/components/forms/DynamicFormObjectList.tsx`: preview price comparison.
- `react-template/src/types/page.ts`: production runtime types.
- `react-template/src/utils/formCalculations.ts`: production calculation engine.
- `react-template/src/components/forms/DynamicFormObjectList.tsx`: production price comparison.
- `docs/examples/davinci-orders-page.json`, `docs/examples/davinci-orders-page-second-project.json`, and `docs/examples/davinci-order-form-calculations.json`: working examples.

---

### Task 1: Persist and Validate Discount Configuration in Go

**Files:**
- Modify: `models/pageModel.go`
- Modify: `models/frontendValidation.go`
- Modify: `models/form_calculation_validation_test.go`

**Interfaces:**
- Produces: optional `OriginalTargetField string`, `MinimumQuantity *float64`, and `DiscountPercentage *float64` JSON/BSON fields on `FormItemCalculationConfig`.
- Produces: `FormPriceComparisonConfig` and optional `PriceComparison *FormPriceComparisonConfig` on `FormObjectListDisplayConfig`.
- Consumes: existing ordered available-field validation and `validateFormPrecision`.

- [ ] **Step 1: Inspect overlapping user changes before editing**

Run: `git diff -- models/pageModel.go models/frontendValidation.go models/models_test.go`

Expected: record the existing hunks and edit around them; do not restore or rewrite them.

- [ ] **Step 2: Write failing JSON/BSON round-trip assertions**

Change the calculation fixture to include:

```go
minimumQuantity := 6.0
discountPercentage := 30.0
FormItemCalculationConfig{
    Operation: "quantityDiscount", Inputs: []string{"unitPrice", "quantity"},
    OriginalTargetField: "originalLineTotal", TargetField: "lineTotal",
    MinimumQuantity: &minimumQuantity, DiscountPercentage: &discountPercentage,
    Precision: intPointer(2),
}
```

Add a display fixture whose `PriceComparison` references `originalLineTotal` and `lineTotal`, then assert every field survives both JSON and BSON round trips.

- [ ] **Step 3: Run the model test and verify RED**

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./models -run TestPageModelFormCalculationJSONAndBSONRoundTrip -count=1`

Expected: FAIL because the new fields and types do not exist.

- [ ] **Step 4: Add the minimal persistence types**

Add pointer numeric fields so omitted values remain distinguishable from zero:

```go
type FormItemCalculationConfig struct {
    Operation          string   `bson:"operation" json:"operation"`
    Inputs             []string `bson:"inputs" json:"inputs"`
    OriginalTargetField string  `bson:"originalTargetField,omitempty" json:"originalTargetField,omitempty"`
    TargetField        string   `bson:"targetField" json:"targetField"`
    MinimumQuantity   *float64 `bson:"minimumQuantity,omitempty" json:"minimumQuantity,omitempty"`
    DiscountPercentage *float64 `bson:"discountPercentage,omitempty" json:"discountPercentage,omitempty"`
    Precision          *int     `bson:"precision,omitempty" json:"precision,omitempty"`
}

type FormPriceComparisonConfig struct {
    OriginalField   string `bson:"originalField" json:"originalField"`
    DiscountedField string `bson:"discountedField" json:"discountedField"`
    Currency        string `bson:"currency,omitempty" json:"currency,omitempty"`
    Precision       *int   `bson:"precision,omitempty" json:"precision,omitempty"`
}
```

Attach `PriceComparison *FormPriceComparisonConfig` to the existing object-list display type.

- [ ] **Step 5: Run the round-trip test and verify GREEN**

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./models -run TestPageModelFormCalculationJSONAndBSONRoundTrip -count=1`

Expected: PASS.

- [ ] **Step 6: Write failing validation table cases**

Add cases for missing/distinct output fields, input overwrite, unknown inputs, threshold `0`, percentage `0` and `101`, output collisions, unknown price-comparison fields, partial price comparison, lowercase currency, and precision `7`. Keep the valid fixture at threshold `6` and percentage `30`.

- [ ] **Step 7: Run validation tests and verify RED**

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./models -run TestValidateFormCalculationConfig -count=1`

Expected: FAIL because `quantityDiscount` is unsupported and display references are not validated.

- [ ] **Step 8: Implement operation-specific validation**

Branch on `calculation.Operation`: preserve `multiply`, and for `quantityDiscount` require two available inputs, distinct collision-free outputs, `minimumQuantity > 0`, `0 < discountPercentage <= 100`, and valid precision. Add both outputs to the ordered available-field set. Validate price comparison only after all item calculations so both referenced outputs are available.

- [ ] **Step 9: Run backend model tests and format**

Run: `gofmt -w models/pageModel.go models/frontendValidation.go models/form_calculation_validation_test.go`

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./models -run 'Test(PageModelFormCalculationJSONAndBSONRoundTrip|ValidateFormCalculationConfig)' -count=1`

Expected: PASS.

- [ ] **Step 10: Commit the backend model slice**

```bash
git add models/pageModel.go models/frontendValidation.go models/form_calculation_validation_test.go
git commit -m "feat: validate quantity discount form config"
```

### Task 2: Calculate Discounted Lines Authoritatively in Go

**Files:**
- Modify: `services/form_cart_calculations.go`
- Modify: `services/form_cart_calculations_test.go`

**Interfaces:**
- Consumes: the Task 1 `FormItemCalculationConfig` fields.
- Produces: server-calculated numeric `originalTargetField` and `targetField` values.

- [ ] **Step 1: Write failing threshold tests**

Use unit price `100`, threshold `6`, and percentage `30`. Assert quantities `5`, `6`, and `7` produce `{originalLineTotal,lineTotal}` values `{500,500}`, `{600,420}`, and `{700,490}`. Seed incorrect client values in the records and assert they are overwritten. Assert a summary over `lineTotal` returns `1410`.

- [ ] **Step 2: Run the service tests and verify RED**

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./services -run TestEvaluateFormCartQuantityDiscount -count=1`

Expected: FAIL with unsupported item calculation.

- [ ] **Step 3: Implement minimal decimal-safe discount evaluation**

For `quantityDiscount`, parse unit price and quantity through existing numeric guards, compute and round the original multiplication, and when the threshold is met multiply its rational value by `(100-discountPercentage)/100` before rounding. Write both outputs on every evaluation. Keep invalid quantities on the existing `FORM_INVALID_QUANTITY` path.

- [ ] **Step 4: Add boundary and rounding coverage**

Test a 100 percent discount and a decimal price such as `19.99 * 6 * 0.70 == 83.96` at precision two. Assert input records remain immutable.

- [ ] **Step 5: Format and run backend regression tests**

Run: `gofmt -w services/form_cart_calculations.go services/form_cart_calculations_test.go`

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./...`

Expected: PASS.

- [ ] **Step 6: Commit backend evaluation**

```bash
git add services/form_cart_calculations.go services/form_cart_calculations_test.go
git commit -m "feat: calculate quantity discounts on form carts"
```

### Task 3: Add Tenant Panel Types, Preview Logic, and Builder Controls

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/types/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/formCalculations.test.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/formCalculations.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/formCalculationEditor.test.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/formCalculationEditor.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/PageDesigner/FormComponentEditor.tsx`

**Interfaces:**
- Produces: TypeScript `quantityDiscount` fields and `FormPriceComparisonConfig` matching Go JSON names.
- Produces: designer normalization/validation and form controls for every required property.
- Produces: preview items containing both `originalLineTotal` and `lineTotal`.

- [ ] **Step 1: Inspect overlapping frontend type changes**

Run: `git -C /Users/osmansamilerdogan/Desktop/tenantPanel diff -- src/types/page.ts`

Expected: preserve the user's unrelated date/table changes while extending the same interfaces.

- [ ] **Step 2: Write failing mirrored calculation tests**

Add the `100 × [5,6,7]` fixture and expect original/payable totals `[500/500, 600/420, 700/490]`. Add the `19.99 × 6` rounding assertion and a test proving recalculation does not mutate its input.

- [ ] **Step 3: Run calculation tests and verify RED**

Run from `tenantPanel`: `yarn test src/utils/formCalculations.test.ts`

Expected: FAIL because the operation is unsupported.

- [ ] **Step 4: Extend types and implement calculation**

Use a discriminated union or optional operation fields that serializes exactly as the spec. In `calculateObjectListItem`, retain the current `multiply` branch and add `quantityDiscount`; always write the rounded original output, then write either the same value or the rounded discounted value.

- [ ] **Step 5: Run calculation tests and verify GREEN**

Run from `tenantPanel`: `yarn test src/utils/formCalculations.test.ts`

Expected: PASS.

- [ ] **Step 6: Write failing editor-helper tests**

Assert normalization trims both output names and uppercases price-comparison currency. Assert validation rejects threshold `0`, percentage `101`, duplicate outputs, unknown comparison fields, lowercase currency before normalization, and partial comparison configuration. Assert switching a new calculation to `quantityDiscount` can hold defaults `6`, `30`, and precision `2`.

- [ ] **Step 7: Run editor tests and verify RED**

Run from `tenantPanel`: `yarn test src/components/PageDesigner/formCalculationEditor.test.ts`

Expected: FAIL on the new validation and normalization expectations.

- [ ] **Step 8: Implement helper validation and normalization**

Add both quantity-discount outputs to the available fields in declaration order. Normalize `originalTargetField`, comparison fields, and currency. Emit explicit messages for each invalid threshold, percentage, output, or comparison property.

- [ ] **Step 9: Add builder controls**

Replace the disabled operation control with a `Multiply`/`Quantity discount` selector. Conditionally render original output, minimum quantity, and percentage inputs for the new operation. Add a price-comparison editor near the current `rightTemplate`; disabling it removes the object rather than saving a partial configuration.

- [ ] **Step 10: Run Tenant Panel tests, lint, and build**

Run from `tenantPanel`: `yarn test src/utils/formCalculations.test.ts src/components/PageDesigner/formCalculationEditor.test.ts src/components/PageDesigner/PageDesigner.formSave.test.ts`

Run from `tenantPanel`: `yarn lint`

Run from `tenantPanel`: `yarn build`

Expected: all commands pass with no new warnings.

- [ ] **Step 11: Commit the Tenant Panel configuration slice**

```bash
git -C /Users/osmansamilerdogan/Desktop/tenantPanel add src/types/page.ts src/utils/api/page.ts src/utils/formCalculations.ts src/utils/formCalculations.test.ts src/components/PageDesigner/formCalculationEditor.ts src/components/PageDesigner/formCalculationEditor.test.ts src/components/PageDesigner/FormComponentEditor.tsx
git -C /Users/osmansamilerdogan/Desktop/tenantPanel commit -m "feat: configure quantity discounts in form builder"
```

### Task 4: Render Accessible Price Comparisons in Both Frontends

**Files:**
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/forms/DynamicFormObjectList.test.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/forms/DynamicFormObjectList.tsx`
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/components/forms/DynamicFormObjectList.test.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/components/forms/DynamicFormObjectList.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/types/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/formCalculations.test.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/formCalculations.ts`

**Interfaces:**
- Consumes: `display.priceComparison` and the two calculated item fields.
- Produces: accessible `<del>` original price plus emphasized discounted price when discounted value is lower.

- [ ] **Step 1: Inspect overlapping production type changes**

Run: `git -C /Users/osmansamilerdogan/Desktop/react-template diff -- src/types/page.ts`

Expected: preserve the user's unrelated changes while adding matching form types.

- [ ] **Step 2: Add production calculation tests and verify RED**

Copy the exact Tenant Panel threshold and rounding fixtures into `react-template/src/utils/formCalculations.test.ts`.

Run from `react-template`: `yarn test src/utils/formCalculations.test.ts`

Expected: FAIL because `quantityDiscount` is unsupported.

- [ ] **Step 3: Implement matching types and calculation logic**

Mirror the Tenant Panel public field names and calculation semantics exactly. Do not import code across repositories.

- [ ] **Step 4: Run production calculation tests and verify GREEN**

Run from `react-template`: `yarn test src/utils/formCalculations.test.ts`

Expected: PASS with fixtures identical to Tenant Panel.

- [ ] **Step 5: Write failing component tests in each frontend**

Render an item with equal totals and assert only `500.00 TRY` is visible. Render an item with `originalLineTotal: 600` and `lineTotal: 420` and assert a `<del>` contains `600.00 TRY` while the emphasized payable value contains `420.00 TRY`. Also assert a malformed/nonnumeric comparison falls back to `rightTemplate` without throwing.

- [ ] **Step 6: Run component tests and verify RED**

Run from each frontend: `yarn test src/components/forms/DynamicFormObjectList.test.tsx`

Expected: FAIL because structured comparison rendering does not exist.

- [ ] **Step 7: Implement a focused price renderer in each component**

Format with `Intl.NumberFormat` using fixed configured precision and currency. Render `<del className="text-neutral-400">` only when `discounted < original`, followed by a visually emphasized discounted value. Use the existing right-side text when the structured configuration is absent or unusable.

- [ ] **Step 8: Run both frontend test/build suites**

Run from each frontend: `yarn test src/utils/formCalculations.test.ts src/components/forms/DynamicFormObjectList.test.tsx && yarn build`

Expected: PASS.

- [ ] **Step 9: Commit each runtime independently**

```bash
git -C /Users/osmansamilerdogan/Desktop/tenantPanel add src/components/forms/DynamicFormObjectList.tsx src/components/forms/DynamicFormObjectList.test.tsx
git -C /Users/osmansamilerdogan/Desktop/tenantPanel commit -m "feat: preview discounted cart prices"

git -C /Users/osmansamilerdogan/Desktop/react-template add src/types/page.ts src/utils/formCalculations.ts src/utils/formCalculations.test.ts src/components/forms/DynamicFormObjectList.tsx src/components/forms/DynamicFormObjectList.test.tsx
git -C /Users/osmansamilerdogan/Desktop/react-template commit -m "feat: render quantity-discounted cart lines"
```

### Task 5: Update Examples and Perform Cross-Project Verification

**Files:**
- Modify: `docs/examples/davinci-orders-page.json`
- Modify: `docs/examples/davinci-orders-page-second-project.json`
- Modify: `docs/examples/davinci-order-form-calculations.json`

**Interfaces:**
- Consumes: all configuration fields introduced by Tasks 1–4.
- Produces: copy-ready Da Vinci page JSON with the approved six-item, 30-percent behavior.

- [ ] **Step 1: Update the example configurations**

Replace the Da Vinci `multiply` calculation with:

```json
{
  "operation": "quantityDiscount",
  "inputs": ["unitPrice", "quantity"],
  "originalTargetField": "originalLineTotal",
  "targetField": "lineTotal",
  "minimumQuantity": 6,
  "discountPercentage": 30,
  "precision": 2
}
```

Keep input ordering consistent with the implemented contract (unit price first, quantity second). Add `display.priceComparison` with TRY and precision two. Keep the total summary sourced from `lineTotal`.

- [ ] **Step 2: Validate JSON and backend acceptance**

Run: `jq empty docs/examples/davinci-orders-page.json docs/examples/davinci-orders-page-second-project.json docs/examples/davinci-order-form-calculations.json`

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./...`

Expected: JSON parses and Go tests pass.

- [ ] **Step 3: Run complete frontend verification**

Run from `tenantPanel`: `yarn test && yarn lint && yarn build`

Run from `react-template`: `yarn test && yarn lint && yarn build`

Expected: every command passes. Do not delete or stage unrelated generated/user files.

- [ ] **Step 4: Review fixture parity and workspace hygiene**

Run: `git diff --check`

Run in each frontend: `git diff --check`

Compare the `100 × [5,6,7]` and `19.99 × 6` expectations in both frontend test files and the Go service test. Confirm `git status --short` shows only intended feature files plus the pre-existing user changes identified at the start.

- [ ] **Step 5: Commit examples**

```bash
git add docs/examples/davinci-orders-page.json docs/examples/davinci-orders-page-second-project.json docs/examples/davinci-order-form-calculations.json
git commit -m "docs: configure Da Vinci quantity discount"
```
