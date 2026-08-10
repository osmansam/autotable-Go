# Table Row Drag Reordering Design

## Goal

Allow selected dynamic tables to reorder the rows currently visible on a page by drag and drop, persisting the result in a configured integer field.

## Configuration

Add optional table drag configuration:

```json
{
  "drag": {
    "enabled": true,
    "orderField": "order"
  }
}
```

Tenant Panel exposes an **Enable row dragging** control and an **Order field** selector. The selector lists integer-compatible fields from the table container. Dragging is inactive until both values are configured.

The backend persists the configuration and validates that enabled dragging has a non-empty order field. Tenant Panel additionally ensures that the selected field is integer-compatible.

## Reordering Semantics

Reordering operates only on rows displayed on the current page.

Dropping a row at order 30 onto the row at order 10 moves the dragged row to position 10 and shifts the previous positions 10 through 29 down one place. Downward movement uses the corresponding inverse shift.

After each successful drop calculation, visible rows receive consecutive integer order values in their new visual sequence. Existing gaps such as 10, 20, 30 are normalized. Missing order values are generated during the first drop.

For paginated tables, the starting value uses the page offset:

```
start = (currentPage - 1) * rowsPerPage + 1
```

Unpaginated tables start at 1.

Dropping a row onto itself is a no-op.

## Persistence and UI

React Template and Tenant Panel preview pass the existing `isDraggable` and `onDragEnter` props to `GenericTable`.

A pure reorder helper accepts the visible rows, dragged identity, target identity, order field, and starting value. It returns the reordered visible rows and update payloads only for rows whose order value changed or was missing.

Changed rows are sent through the existing dynamic bulk-update mutation. The local query cache is updated optimistically and rolled back on failure through the existing mutation behavior. A successful mutation invalidates the table query.

Dragging is disabled while another explicit sort is active because sorted display order conflicts with manual order. When drag configuration is active and no conflicting sort exists, the table request uses the configured order field ascending so persisted order is visible after reload.

## Error Handling

- Missing row identities make a drop a no-op.
- Missing order values are generated rather than rejected.
- A missing/invalid configured order field disables dragging in the frontend.
- Bulk update failures use the existing error toast, rollback, and query invalidation behavior.

## Testing

- Configuration round trip and backend validation.
- Tenant cleaning preserves drag settings and restricts order choices to integer fields.
- Upward move shifts the range.
- Downward move shifts the range.
- Missing and non-contiguous values normalize.
- Paginated starting offset is applied.
- Self-drop and missing identities are no-ops.
- Paginated and unpaginated table integrations pass the correct drag props.
- Full backend and frontend suites and production builds remain green.
