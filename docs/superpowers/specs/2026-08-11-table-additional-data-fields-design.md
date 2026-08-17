# Table Additional Data Fields Design

## Goal

Allow a table to receive row fields that are needed by filters, actions, conditions, or custom logic without rendering those fields as columns. A field such as `status` can therefore be used simultaneously as a server-side constant filter and as client-side action state while remaining invisible in the table.

The behavior must remain consistent across `autotable-Go`, `tenantPanel`, and `react-template`.

## Separation of Responsibilities

Table configuration has three distinct concepts:

- `columns` controls presentation only.
- `constantFilters` controls fixed server-side query constraints.
- `dataFields` requests additional values in returned row records without displaying them.

Do not add a hidden-column mode. A column always represents rendered table output.

Example:

```json
{
  "columns": [
    { "field": "name" },
    { "field": "price" }
  ],
  "constantFilters": {
    "status": "ACTIVE"
  },
  "dataFields": ["status"],
  "actions": [
    {
      "kind": "edit",
      "disabledCondition": "status != 'ACTIVE'"
    }
  ]
}
```

The backend filters for active records. Returned records include `status`, allowing the runtime to evaluate the Edit action, but the table renders only `name` and `price`.

## Automatic Required-Field Discovery

The shared frontend field resolver builds the requested field projection as a union of all known consumers:

1. rendered non-computed columns;
2. existing column rules, templates, progress fields, nested rows, generated relations, array sources, and drag order fields;
3. configured filter-panel `formKey` values;
4. action form fields and field overrides where row data is required;
5. fields referenced by action-level `disabledCondition`, `hiddenCondition`, and `requiredCondition` expressions;
6. fields referenced by field-override conditions;
7. fields referenced by row-class and toggle conditions or request effects;
8. explicit `dataFields`;
9. `_id` where the request path requires row identity.

Condition discovery uses the existing condition-field parser and the available container field names. Keywords, operators, literals, and unknown identifiers are not requested as fields.

If `status` appears in an action condition, it is requested automatically. The designer does not require users to duplicate it in `dataFields`.

## Explicit Additional Data Fields

Add the following table property:

```ts
dataFields?: string[];
```

`dataFields` is a fallback for consumers that cannot be statically inspected, including external custom code or future runtime behavior. Tenant Panel exposes it in table Request settings as an **Additional data fields** multi-select populated from the bound schema fields.

The editor excludes duplicates, trims names, and saves no empty entries. It may show fields that are already automatically required, but the persisted configuration remains deduplicated.

## Data Flow

For schema-backed paginated tables, the resolved union becomes the `fields` query projection passed to the table-source endpoint. Filters remain independent of projection: `constantFilters.status = "ACTIVE"` filters on the server whether or not `status` is returned.

For pipeline- and workflow-backed tables, the same resolved union is sent as the requested `fields` contract. The source may return additional values, but runtimes must not turn them into columns automatically.

Unpaginated schema tables currently receive full records. They continue doing so, while sharing the configuration and resolver contract so future projection support cannot diverge. Rendering remains driven exclusively by `columns`.

## Backend Contract and Validation

`autotable-Go` persists `TableComponentConfig.DataFields []string` using `dataFields` in BSON and JSON.

Validation rejects entries that are blank after trimming. Duplicate values are normalized by Tenant Panel and tolerated by backend deserialization, but backend validation may reject duplicates if it can provide a precise field-specific message. Schema-aware page validation rejects unknown fields only when the page component has an unambiguous schema binding available; it must not reject pipeline or workflow output fields merely because they are absent from a container schema.

Existing table configurations without `dataFields` remain valid and use automatic discovery only.

## Security and Authorization

Requesting a field does not bypass existing field authorization. Backend role-based response filtering remains authoritative. A configured additional field that the current user cannot access is omitted by the normal authorization layer.

Hidden presentation is not a security boundary. Sensitive fields must still be protected by backend authorization rather than relying on their absence from `columns`.

## Error Handling

- Tenant Panel prevents blank additional field entries.
- The backend returns a table-configuration validation error for invalid persisted entries.
- Unknown identifiers in condition expressions are ignored by projection discovery rather than sent as arbitrary field names.
- A missing or empty `dataFields` property behaves as an empty list.
- A condition that references an authorized, known schema field automatically includes it even when no visible column uses it.

## Testing

Backend tests verify JSON/BSON round-trip persistence, blank-entry validation, and backward compatibility when `dataFields` is absent.

Shared frontend resolver tests verify:

- a field referenced only by an action disabled condition is requested;
- action hidden and required conditions are included;
- field-override, row, and toggle condition fields are included;
- filter-panel keys and explicit `dataFields` are included;
- duplicates collapse to one field;
- keywords and unknown identifiers are excluded when available schema names are supplied;
- requested fields do not create rendered columns.

Tenant Panel tests verify editor cleaning, persistence, hydration, and schema-field options.

Runtime tests verify that paginated requests include additional and automatically discovered fields, action conditions receive those row values, and unpaginated rendering remains column-driven.

## Success Criteria

- `constantFilters.status = "ACTIVE"` filters records server-side.
- An action condition referencing `status` receives `row.status` without a status column.
- Users can explicitly add non-displayed fields through Additional data fields.
- Automatically discoverable fields require no duplicate manual configuration.
- Additional row data never becomes a rendered column unless separately added to `columns`.
- Existing tables continue to behave as before.
