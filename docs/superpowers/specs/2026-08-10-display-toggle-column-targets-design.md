# Display Toggle Column Targets Design

## Goal

Allow each table display toggle to control the visibility of selected static columns and generated relation column groups. A selected target is visible while its toggle is on and hidden while it is off.

## Configuration Contract

Static columns continue using their existing `visibilityToggle: { toggleId, when }` binding. Generated relation groups gain the same optional `visibilityToggle` binding.

The Display Toggles editor provides the convenient reverse view: for each toggle, users select the static columns and generated relation groups controlled by that toggle. Selecting a target writes `{ toggleId: currentToggle.id, when: true }` to the target. Deselecting it removes that binding only when it belongs to the current toggle.

The target-side binding remains canonical. No duplicate target list is persisted on the toggle.

## Tenant Panel

Each Display Toggle card gains a multi-select area with:

- Static table columns, identified by field name and display label.
- Generated relation groups, identified by stable group ID and source/array label.

Selections reflect the current target bindings. Assigning a target already controlled by another toggle transfers it to the current toggle. Removing or renaming a toggle continues to clean or update references through the existing binding-maintenance behavior.

The table preview applies the same visibility behavior as React Template.

## Runtime Behavior

Static columns retain their existing visibility evaluation. Generated relation descriptors are not created when their group's visibility binding does not match the current toggle state.

For the requested Edit Locations flow, the generated group may use the same toggle for both:

- `visibilityToggle: { toggleId: "editLocations", when: true }`
- `booleanEditToggle: { toggleId: "editLocations", when: true }`

The result is:

- Toggle on: generated location columns are visible and editable.
- Toggle off: generated location columns are absent.

Targets without a visibility binding remain visible. Missing or invalid toggle references fail open for backward compatibility.

## Backend

Add optional `visibilityToggle` persistence to generated relation groups and validate that its toggle ID exists, matching existing static-column validation.

## Tests

- Backend JSON/BSON round trip preserves generated-group visibility bindings.
- Backend validation rejects an unknown generated-group visibility toggle.
- Tenant cleaning preserves the binding.
- Display-toggle target selection assigns, transfers, and removes canonical bindings.
- Generated relation descriptors hide and reveal the entire group based on toggle state.
- Static-column visibility and generated-column editability continue working.
- Full backend and frontend suites and production builds remain green.
