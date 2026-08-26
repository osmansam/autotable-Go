# Tiered Quantity Discounts Design

## Goal

Extend the existing row-level `quantityDiscount` calculation so one product row can offer progressive quantity thresholds. The highest threshold reached applies its discount percentage to every unit in that row. The cart UI offers the next threshold with a compact action that raises the row quantity exactly to that threshold.

For example, with tiers at 6 units for 30% and 10 units for 40%:

- Quantity 3 has no discount and offers `+3 → %30`.
- Quantity 6 receives 30% off and offers `+4 → %40`.
- Quantity 8 receives 30% off and offers `+2 → %40`.
- Quantity 10 receives 40% off and shows no further offer.

Discounts are isolated to the qualifying object-list row. They do not affect other rows or the entire cart.

## Configuration Contract

`quantityDiscount` gains an optional ordered `discountTiers` array:

```json
{
  "operation": "quantityDiscount",
  "inputs": ["unitPrice", "quantity"],
  "originalTargetField": "originalLineTotal",
  "targetField": "lineTotal",
  "discountTiers": [
    { "minimumQuantity": 6, "discountPercentage": 30 },
    { "minimumQuantity": 10, "discountPercentage": 40 }
  ],
  "precision": 2
}
```

Each tier contains:

- `minimumQuantity`: a finite number greater than zero.
- `discountPercentage`: a finite number greater than zero and no greater than 100.

Tier quantities must be unique and strictly ascending. Discount percentages must also be strictly ascending so reaching a higher tier can never make the row more expensive through a weaker discount.

The existing top-level `minimumQuantity` and `discountPercentage` fields remain supported as a legacy single tier. A calculation must provide either a non-empty `discountTiers` array or both legacy fields, but not a partial legacy pair. When `discountTiers` is present, it is authoritative. Existing saved pages therefore remain valid without migration.

## Calculation Behavior

The first calculation input is unit price and the second is row quantity. The calculation always writes the undiscounted `unitPrice * quantity` value to `originalTargetField`.

The calculator selects the tier with the greatest `minimumQuantity` less than or equal to the row quantity. If a tier qualifies, its percentage is applied to the complete original total for that row. If no tier qualifies, the discounted target equals the original total. Existing precision and numeric error behavior remain unchanged.

The Go workflow calculation remains authoritative for submitted values. Both frontend projects mirror the same algorithm for immediate UI feedback.

## Cart Row Interaction

Each visible row derives its current quantity and the calculation whose quantity input matches that field. It selects the first configured tier above the current quantity and renders one compact button beneath the row's secondary text:

```text
+3 → %30
```

The accessible label describes the full action, including how many units will be added and the resulting discount. Clicking the button sets the row quantity to the next tier's `minimumQuantity`, then runs the existing form-state recalculation so the row price and cart summary update together.

Only the next tier is shown. After a click, the following tier becomes the offer. No offer is rendered at or above the highest tier, for missing or nonnumeric quantities, or for malformed calculation configuration. The edit and remove actions remain unchanged.

The visual treatment uses a small, restrained badge/button consistent with the existing neutral cart styling. The compact text avoids making rows substantially taller or wider.

## Page Designer

The calculation editor exposes tier rows with minimum quantity and discount percentage inputs plus add and remove controls. New `quantityDiscount` calculations default to one tier matching today's defaults: 6 units and 30%.

Designer normalization trims existing field names and preserves tier order. Designer validation reports empty tiers, non-finite values, invalid ranges, duplicate or non-ascending quantities, and non-ascending percentages before save. Loading a legacy single-tier calculation continues to work; editing it may retain the legacy representation until tiers are explicitly added, avoiding unrelated saved-page churn.

## Project Changes

### `autotable-Go`

- Add the tier model to the page configuration types.
- Validate tier and legacy configurations.
- Select and apply the highest reached tier in workflow calculations.
- Update the Da Vinci example page to include 6/30 and 10/40 tiers.

### `tenantPanel`

- Mirror the API and runtime TypeScript types.
- Add tier controls, normalization, and validation to the page designer.
- Update the dynamic form calculation and cart-row UI.

### `react-template`

- Mirror the runtime TypeScript types.
- Update the dynamic form calculation and cart-row UI.

The two frontend renderers should remain behaviorally identical.

## Error Handling

Backend page validation rejects invalid persisted configuration with contextual object-list and calculation indexes. Workflow calculation continues to use the existing structured errors for invalid price or quantity values.

The frontend renderer does not show an offer when it cannot safely derive a next tier. It does not guess at malformed configuration. Page designer validation prevents newly authored malformed tier configurations from being saved.

## Testing

Backend tests cover no qualifying tier, each qualifying tier, quantities above the maximum, row isolation, precision, legacy single-tier behavior, and every new validation rule.

Both frontend projects test highest-tier selection, applying the percentage to the complete row, legacy behavior, and recalculated totals. Cart component tests cover the compact next-tier label, setting quantity exactly to the threshold, advancing to the following offer, accessible labeling, and hiding the action at the highest tier.

Tenant-panel designer tests cover adding/removing tiers, defaults, normalization, validation, API serialization, and loading legacy configurations.

## Non-Goals

- Discounts distributed across multiple product rows.
- Applying a tier to the entire cart.
- Marginal pricing where only units beyond a threshold receive the higher discount.
- Product-specific tier lookup from a separate promotion service.
- Date ranges, coupons, customer groups, or other promotion rules.
