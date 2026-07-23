# Boolean Switch Table Column Design

## Goal

Add a table column type that renders boolean fields as editable inline switches, so users can toggle fields such as `active` directly from the table.

## Scope

The new column type is `booleanSwitch`.

It is configured in tenantPanel Page Designer table column settings and rendered by both tenantPanel preview/admin tables and react-template end-user tables.

## Behavior

- `boolean` remains display-only and renders as the current Yes/No badge.
- `booleanSwitch` renders a switch in the cell.
- Toggling the switch sends the existing dynamic row update request with `{ [fieldName]: nextBoolean }`.
- The initial checked state accepts boolean-like values: `true`, `"true"`, `1`, and `"1"` are true; all other values are false.
- No backend model migration is required because table column type is stored as a string.

## Validation

No backend validation change is required. The backend already stores table column `type` as an unrestricted string and validates only unrelated table link/filter properties.

## Testing

TenantPanel tests verify `TABLE_COLUMN_TYPE_OPTIONS` includes `booleanSwitch`.

React-template tests should verify helper-level behavior if a table-cell helper exists; otherwise build verification covers TypeScript integration for the renderer path.
