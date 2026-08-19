# Configurable Form Cart Calculations Design

## Goal

Add reusable, designer-configurable price snapshots and cart totals to dynamic forms. The initial order form calculates `unitPrice * quantity`, displays live line and order totals, persists those values, and has the backend independently verify all financial values. The model must support later discount, tax, and shipping operations without redesigning the cart.

## Scope

This change spans the three related projects:

- `autotable-Go` owns persisted page configuration, configuration validation, and authoritative workflow calculation.
- `tenantPanel` owns page-designer controls and its live form preview.
- `react-template` owns the production dynamic-form runtime.

The first release supports field snapshots, item multiplication, list sums, and a direct summary-field copy. Arbitrary expressions, discounts, taxes, shipping, and promotions are explicitly deferred. The configuration shape reserves an operation-based extension point for those additions.

## Configuration Model

### Field mappings

Each `FormObjectListConfig` may declare source-record fields that must be snapshotted when an item is added:

```json
{
  "fieldMappings": [
    {
      "sourceFormKey": "productId",
      "sourceField": "price",
      "targetField": "unitPrice",
      "required": true
    }
  ]
}
```

`sourceFormKey` identifies the schema-backed select whose selected record supplies the value. `sourceField` is read from that selected source record. `targetField` is written to the embedded cart item. A required mapping prevents the item from being added when the selected source record has no valid value.

Snapshots are deliberate: changing a product price after an order is placed must not change the historical order.

### Item calculations

Each object list may define structured item calculations:

```json
{
  "itemCalculations": [
    {
      "operation": "multiply",
      "inputs": ["unitPrice", "quantity"],
      "targetField": "lineTotal",
      "precision": 2
    }
  ]
}
```

The initial release supports only `multiply`. It requires exactly two numeric inputs. Calculations run in declaration order, which provides an explicit dependency order for later operations.

### Summaries

The form may define summaries derived from an object list:

```json
{
  "summaries": [
    {
      "key": "subtotal",
      "label": "Ara Toplam",
      "operation": "sum",
      "objectListKey": "items",
      "sourceField": "lineTotal",
      "targetField": "subtotal",
      "format": {
        "style": "currency",
        "currency": "TRY",
        "precision": 2
      }
    },
    {
      "key": "total",
      "label": "Toplam",
      "operation": "copy",
      "sourceField": "subtotal",
      "targetField": "total",
      "format": {
        "style": "currency",
        "currency": "TRY",
        "precision": 2
      }
    }
  ]
}
```

The initial summary operations are `sum` and `copy`. `sum` totals one numeric item field across an object list. `copy` copies an earlier summary value and provides a stable extension point for a future final-total chain such as subtotal, discount, tax, shipping, and total.

Summary calculations run in declaration order. Summary keys and target fields must be unique.

## Money Semantics

All calculations use decimal-safe arithmetic and explicit precision. Inputs are normalized to the configured precision, multiplication is rounded once at the calculated output, and a sum adds normalized item outputs before rounding its result. The initial default is two decimal places.

Formatting is separate from arithmetic. Currency defaults to `TRY` for the supplied order form but is configurable per summary. Persisted values remain numeric and do not contain currency symbols or localized separators.

## Designer Experience

The `tenantPanel` form object-list editor adds three focused configuration sections:

1. Field mappings select the schema-backed form field, source field, target item field, and whether the value is required.
2. Item calculations select an operation, its item inputs, output field, and precision.
3. Form summaries select an operation, list and source field when applicable, output field, label, currency, and precision.

Controls expose only valid fields where possible. Removing or renaming a referenced field leaves a visible validation error and prevents the page configuration from being saved. The designer preview uses the same runtime behavior as the production template.

## Runtime Data Flow

1. A schema-backed product select loads complete product records, including mapped fields such as `price`.
2. When the user adds an item, the runtime copies configured form fields, snapshots configured source-record fields, and evaluates item calculations.
3. Every add, edit, remove, increment, and decrement operation recalculates the affected item and all form summaries.
4. The cart renders configured item templates. The supplied order page shows product name, quantity, unit price, and line total.
5. A summary panel in the configured form area renders subtotal and total.
6. Submission includes embedded item snapshots, calculated item fields, and summary target fields.

For the supplied order page, one submitted item has this logical shape:

```json
{
  "productId": "product-id",
  "quantity": 3,
  "unitPrice": 100,
  "lineTotal": 300
}
```

The initial order includes `subtotal` and `total`. `discountTotal` is added only when discount calculations are implemented later.

## Backend Authority

Client totals are previews and must never be treated as authoritative. A workflow submission includes the stored page ID and component ID as calculation-configuration references. The backend resolves those references to the persisted page, verifies that the component is the requested workflow form, and uses its stored mappings and calculations. It never accepts calculation configuration from the request body. A reusable backend calculation step invoked by `create-davinci-order` then performs these operations before persistence:

1. Validate that the cart is non-empty and quantities are valid positive numbers.
2. Load every referenced product through the trusted schema repository.
3. Read the current authoritative price and calculate each `unitPrice` and `lineTotal` using the same operation and precision semantics.
4. Calculate `subtotal` and `total` from the authoritative lines.
5. Compare submitted snapshots and totals with authoritative results.
6. On any difference, reject the request with a structured stale-price error that identifies the affected item; do not silently place an order at a different price.
7. When all values match, discard client financial values, persist the server-calculated values, and continue the workflow.

This comparison protects against tampering and also makes genuine price changes visible to the user before purchase.

## Validation and Error Handling

Configuration validation rejects:

- A mapping without `sourceFormKey`, `sourceField`, or `targetField`.
- A mapping whose source form field is not a schema-backed select.
- Duplicate mapping target fields in one object list.
- Unsupported operations or the wrong number of inputs.
- Calculations that reference unknown item fields or overwrite an input field.
- Duplicate calculation or summary targets.
- A summary that references an unknown object list, item field, or earlier summary.
- Currency codes that are not three uppercase ASCII letters.
- Precision outside the supported range of zero through six decimal places.
- A summary target that collides with a normal persisted parent form field.

At runtime, a required missing or nonnumeric mapped price prevents adding the item and displays a field-level or cart-level error. Invalid quantities are rejected rather than coerced. Missing calculation inputs never silently become zero. An empty cart displays zero summaries but submission is rejected for this order form.

Backend errors use stable machine-readable codes for invalid quantity, missing product, missing price, and stale price. The frontend translates these into actionable messages and preserves the cart so the user can review it.

## Compatibility

All new configuration properties are optional. Existing forms and object lists retain their current behavior. Existing source-record enrichment remains supported, but only explicitly mapped fields are guaranteed calculation inputs and persisted financial snapshots.

The calculation utilities are mirrored deliberately in `tenantPanel` and `react-template`, because the repositories do not currently use a shared frontend package. Both copies expose equivalent APIs and run the same fixtures to detect drift.

## Testing

### Backend

- JSON/BSON model round trips for the new configuration.
- Validation acceptance for the supplied configuration and rejection for every invalid case above.
- Decimal and rounding cases, including fractional prices and quantities where permitted.
- Workflow tests for correct totals, tampered totals, stale prices, missing products, missing prices, invalid quantities, and an empty cart.
- Persistence tests proving server-calculated snapshots and summaries are stored.

### Frontend utilities

- Mapping a selected source record to a cart snapshot.
- Item calculation and summary ordering.
- Decimal rounding behavior matching backend fixtures.
- Recalculation for add, edit, remove, increment, and decrement.
- Submission payload construction.
- Failure behavior for missing and invalid inputs.

### Designer and runtime

- Designer edits serialize and reload without losing configuration.
- Invalid references block save with a visible error.
- Preview and production runtime render identical unit prices, line totals, and summaries.
- The supplied `Sipariş Girişi` configuration exercises the complete flow.

## Success Criteria

- A page designer can configure product-price snapshots and cart totals without editing JSON.
- Selecting a product and quantity produces a visible unit price and line total.
- All cart mutations update subtotal and total immediately.
- The submitted and persisted order contains authoritative `unitPrice`, `lineTotal`, `subtotal`, and `total` values.
- Tampered or stale values cannot be persisted and produce a useful review message.
- Existing forms continue to work unchanged.
- The configuration can later add fixed or percentage discount operations without changing the mapping, item-calculation, or summary architecture.
