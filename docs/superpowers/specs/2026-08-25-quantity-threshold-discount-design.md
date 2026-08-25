# Quantity-Threshold Item Discount Design

## Goal

Allow a tenant administrator to configure a percentage discount that applies to one cart line when that line's quantity reaches a configured threshold. The order form must show the undiscounted line total struck through beside the discounted total, and the backend must recalculate the same values before the workflow persists the order.

## Scope

This change extends the existing configurable form-cart calculation flow across the three related projects:

- `autotable-Go` persists and validates the configuration and performs server-side workflow calculations.
- `tenantPanel` provides form-builder controls and a live preview.
- `react-template` renders and submits the production form.

The feature is a per-object-list-item quantity discount. It does not total quantities across different products, create discount tiers, stack discounts, or introduce a general-purpose expression engine.

## Configuration

### Quantity discount calculation

`FormItemCalculationConfig` gains a `quantityDiscount` operation and operation-specific fields:

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

The first input is the unit price and the second is the line quantity. The operation calculates the undiscounted value as `unitPrice * quantity` and writes it to `originalTargetField`. When `quantity >= minimumQuantity`, it calculates `original total * (1 - discountPercentage / 100)` and writes that value to `targetField`. Below the threshold, `targetField` equals the original total.

Both output fields are numeric. Rounding occurs at each output using the existing configured precision rules. The existing `multiply` operation remains unchanged.

For a unit price of 100 TRY, a quantity of 6, and a 30 percent discount, `originalLineTotal` is 600 and `lineTotal` is 420. A quantity of 5 produces 500 in both fields. A quantity greater than 6 receives the same 30 percent discount; this feature does not multiply or tier the discount.

### Price-comparison display

`FormObjectListDisplayConfig` gains an optional structured display:

```json
{
  "priceComparison": {
    "originalField": "originalLineTotal",
    "discountedField": "lineTotal",
    "currency": "TRY",
    "precision": 2
  }
}
```

When both fields are numeric and the discounted value is lower, the cart row renders the original formatted price with a line through it and the discounted formatted price beside it with stronger emphasis. When the values are equal, it renders only the regular final price. This avoids presenting a discount below the configured threshold.

The renderer builds React elements from typed configuration. It does not interpret HTML from tenant-authored templates. Existing `rightTemplate` behavior remains available and is used when `priceComparison` is absent.

## Tenant Panel Experience

The item-calculation operation control becomes a selector containing `Multiply` and `Quantity discount`. Selecting `Quantity discount` displays:

- Unit-price input field.
- Quantity input field.
- Original-total output field.
- Discounted-total output field.
- Minimum quantity.
- Discount percentage.
- Precision.

The object-list display editor adds an optional price-comparison section containing the original field, discounted field, currency, and precision. Field selectors use outputs available from the object list where possible. The live preview uses the same behavior and visual treatment as production.

For the supplied `Sipariş Girişi` page, the builder configuration uses `unitPrice`, `quantity`, `originalLineTotal`, `lineTotal`, threshold `6`, percentage `30`, currency `TRY`, and precision `2`. Its existing summary continues to sum `lineTotal`, so the payable order total includes line discounts automatically.

## Runtime Data Flow

1. Selecting a product retains the existing configured unit-price snapshot.
2. Adding a line calculates both the original and payable line totals.
3. Editing, incrementing, or decrementing a line recalculates both totals and all summaries.
4. The object-list row compares the two configured price fields and renders either one regular price or the original struck-through price plus the discounted price.
5. Submission includes the line inputs and calculated outputs through the existing form payload path.
6. The Go service resolves the stored form configuration and recalculates the original total, discounted total, and summaries before the workflow continues.

Calculations are deterministic in their declared order. A later calculation may use an earlier output, subject to existing field-reference validation.

## Validation and Errors

Configuration validation for `quantityDiscount` requires:

- Exactly two known numeric calculation inputs.
- A nonempty `originalTargetField` and `targetField` that are distinct, do not overwrite inputs, and do not collide with existing item fields or calculation outputs.
- A finite `minimumQuantity` greater than zero.
- A finite `discountPercentage` greater than zero and no greater than 100.
- Precision from zero through six, matching existing calculation rules.

Price-comparison validation requires both fields to reference available item fields, a three-letter uppercase ASCII currency code, and precision from zero through six. The entire price-comparison object is optional; a partially configured object is rejected rather than silently ignored.

At runtime, missing or nonnumeric inputs use the existing calculation-error path and preserve the current cart state. The backend rejects unsupported or invalid stored configurations rather than trusting submitted totals.

## Backend Authority and Numeric Semantics

The backend adds the operation to `EvaluateFormCart`, which already recalculates configured cart outputs for workflow submissions. It computes both output fields from the submitted item inputs using decimal-safe rational arithmetic and the existing rounding method. Summaries then consume the server-calculated discounted field.

Client-calculated `originalLineTotal`, `lineTotal`, and summary values are overwritten by server-calculated values. The calculation configuration comes from the persisted page/component reference, not from the request body. This feature does not change the existing source or trust model for `unitPrice`; authoritative product-price lookup is outside this change.

## Compatibility

All new properties are optional. Existing forms using `multiply`, text templates, or no calculations retain their behavior. Existing saved `FormItemCalculationConfig` records deserialize without migration. The API uses the same JSON field names in Go, Tenant Panel types, and production-template types.

Because the frontend repositories do not share a runtime package, the calculation and formatting behavior remains mirrored in `tenantPanel` and `react-template`. Equivalent fixtures in both projects protect against drift.

## Testing

Backend tests cover JSON/BSON round trips, valid configuration, each validation failure, quantities below/at/above the threshold, a 100 percent discount, decimal rounding, nonnumeric values, server overwrite of submitted calculated fields, and summaries based on discounted totals.

Each frontend calculation suite uses the same threshold fixtures and verifies immutable recalculation below/at/above the threshold. Display tests verify one regular price below the threshold and accessible original-plus-discounted markup at or above it. Tenant Panel editor tests verify operation switching, normalization, validation messages, serialization, and preview behavior.

The supplied Da Vinci order example is updated to demonstrate the calculation and price-comparison configuration. Targeted tests and production builds run in all three repositories before completion.

## Success Criteria

- A tenant administrator can configure the threshold and percentage without editing JSON.
- A discount is evaluated independently for each product line.
- Quantity 6 or greater receives 30 percent off in the supplied form; lower quantities do not.
- A discounted row displays the full line price struck through beside the payable price.
- A nondiscounted row displays one regular price.
- Cart totals and submitted values use the discounted line total.
- Backend and both frontend runtimes produce matching values.
- Existing forms remain unchanged.
