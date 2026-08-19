# Configurable Select Option Dependencies Design

## Goal

Allow a schema-backed dynamic-form select to fetch any configured fields from each source record, display chosen values on the left and right sides of an open option, and expose those same values to later cart mappings and calculations. The initial order form will show the product name on the left and its price on the right.

## Scope

This change spans the related projects:

- `autotable-Go` persists and validates the select display/dependency configuration.
- `tenantPanel` lets designers configure dependencies and left/right option templates and previews the result.
- `react-template` renders the configured options and retains their source records for calculations.

It extends the existing calculation configuration. It does not add discount operations, redesign the select control, or introduce arbitrary executable expressions.

## Configuration Model

A schema-backed form field may declare fields required from each option and two display templates:

```json
{
  "formKey": "productId",
  "type": "select",
  "optionsSource": "schema",
  "sourceSchemaName": "product",
  "sourceValueField": "_id",
  "sourceLabelField": "name",
  "sourceDataFields": ["name", "price", "taxRate", "discountRate"],
  "optionDisplay": {
    "leftTemplate": "{{name}}",
    "rightTemplate": "{{price}} ₺"
  }
}
```

`sourceDataFields` is the allowlist of source-record values that must be retained for display or downstream use. The value and legacy label fields are fetched automatically even when not repeated in this list.

`leftTemplate` and `rightTemplate` use the project's existing safe field-template syntax. They support field interpolation only; functions and executable expressions are not allowed. Either template may reference any automatically fetched or explicitly configured source field.

For backward compatibility, a missing `optionDisplay.leftTemplate` falls back to the existing `sourceLabelField`. A missing right template renders no right-side content.

## Designer Experience

For schema-backed select fields, `tenantPanel` adds:

1. A multi-select for additional source data fields.
2. A left option template input, initially populated from the label field.
3. A right option template input, optional.
4. A preview row showing the left/right alignment.

The designer can include as many schema fields as needed. Fields referenced by either template or by a cart field mapping are automatically included in the effective fetch set, preventing configuration drift when a designer forgets to select a dependency separately.

The existing object-list mapping editor continues to define downstream values independently:

```text
productId.price        -> unitPrice
productId.taxRate      -> taxRate
productId.discountRate -> discountRate
```

Display does not imply persistence. A dependency enters a cart item only through an explicit field mapping.

## Runtime Data Flow

1. The select derives the effective source field set from the value field, legacy label field, configured dependencies, display-template references, and object-list mappings that reference this form field.
2. The schema request asks for those fields while preserving existing filters and pagination behavior.
3. Each option stores its value, fallback label, and retained source-record data.
4. While the menu is open, each option renders the left template in the flexible left region and the right template in a right-aligned region.
5. Selecting an option keeps the retained source-record data in the form's existing selected-record state. The closed select remains compatible with its current single-label presentation.
6. When an object-list item is added, field mappings read from that retained record, item calculations run, and summaries update in their configured area.

For the supplied order form, an open option appears logically as:

```text
Da Vinci Syrup                                      ₺120.00
```

The selected product's `price` can then be mapped to `unitPrice`; `unitPrice * quantity` produces `lineTotal`; the right-side summary sums line totals.

## Validation and Error Handling

Configuration validation rejects:

- Dependency fields that do not exist in the selected source schema.
- Display templates that reference fields unavailable from the source schema.
- Empty or malformed template references.
- A field mapping whose source field does not exist on its schema-backed select's source schema.

At runtime, absent optional template values render as blank. They do not break or remove the option. An unavailable required mapping continues to prevent adding the item through the existing calculation validation behavior. Failed option requests retain the select's current error behavior.

Changing a label or dependency configuration invalidates cached option data so stale records are not reused.

## Compatibility

All new properties are optional. Existing selects continue to fetch and render their value and label fields exactly as before. Existing forms do not acquire a right label or new persisted cart values.

The configuration remains generic: it works for product price, stock, SKU, tax, discount metadata, or fields from any other schema. It does not hard-code currency or product semantics.

## Testing

### Backend

- JSON/BSON round trips retain dependencies and display templates.
- Validation accepts valid schema fields and rejects unknown dependency/template/mapping fields.
- Existing select configurations remain valid.

### Tenant designer

- Dependency fields and templates serialize and survive refresh.
- Template field references contribute to the effective source field set.
- The preview aligns rendered left and right values.

### Frontend runtimes

- Option requests include the complete effective field set.
- Open options render left and right templates from real source records.
- Missing optional values render safely.
- Selection retains every configured dependency.
- Cart mappings can consume retained dependencies and update calculations and summaries.
- Legacy single-label selects render unchanged.

## Success Criteria

- A designer can choose any number of source fields for a schema-backed select.
- A designer can independently choose or template the open option's left and right labels.
- The order form displays product name on the left and price on the right while selecting.
- Any fetched dependency can be explicitly mapped into a cart item and used by later calculations.
- Refreshing the designer or runtime does not lose the configuration.
- Existing forms remain backward-compatible.
