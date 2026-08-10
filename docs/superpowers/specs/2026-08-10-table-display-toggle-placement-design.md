# Table Display Toggle Placement Design

## Goal

Allow every configured table display toggle to choose between the existing upper toolbar and the lower table controls beside the Excel action. Keep the `Show Filters` control on the upper toolbar at the far right.

## Configuration Contract

Add an optional `isUpperSide` boolean to `TableToggleConfig` in both `tenantPanel` and `react-template`.

- `true`: render the display toggle in the upper toolbar.
- `false`: render the display toggle in the lower controls beside the Excel action.
- Missing: treat as `true` for backward compatibility with saved table configurations.

The setting is per toggle, so one table may contain a mix of upper and lower display toggles.

## Tenant Panel

The Display Toggles editor will expose an `Upper side` checkbox for each toggle. New toggles default to `isUpperSide: true`. Cleaning and saving the table configuration will preserve the explicit boolean value, including `false`.

## React Template

Paginated and unpaginated generic tables will pass each configured toggle's resolved placement to `GenericTable`. The `Show Filters` entry will always use `isUpperSide: true` and will be appended after upper display toggles so it remains the rightmost upper control. Lower display toggles use the existing lower-filter rendering path, which places them beside the Excel action.

Changing placement does not change toggle state, request effects, column visibility effects, Boolean edit effects, or pagination-reset behavior.

## Compatibility and Validation

No backend persistence migration is required because page table configuration is JSON-compatible and the field is optional. Existing configurations render exactly as before because an omitted placement resolves to upper.

## Tests

- Tenant configuration cleaning preserves `isUpperSide: false` and new-toggle defaults use `true`.
- Runtime placement defaults missing values to upper and respects explicit `false`.
- Mixed upper/lower toggles are passed to the correct toolbar regions.
- `Show Filters` is always upper and ordered after all upper display toggles.
- Existing toggle effects and page-reset tests remain green.
