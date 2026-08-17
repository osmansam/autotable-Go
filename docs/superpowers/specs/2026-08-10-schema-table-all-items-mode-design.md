# Schema Table All-Items Mode Design

## Goal

Allow an administrator to choose whether a schema-backed table loads a paginated result or all dynamic model items. The choice is configured while creating or editing a table in Tenant Panel and is honored by Tenant Panel preview and the React Template runtime.

## Scope

- Add the choice only to table components whose data-binding kind is `schema`.
- Keep pipeline and workflow table behavior unchanged.
- Reuse the existing backend routes:
  - `GET /dynamic/page` for paginated schema tables.
  - `GET /dynamic` for all-items schema tables.
- Preserve paginated behavior for existing saved pages.

The feature does not add a new dynamic-data endpoint or change pipeline and workflow execution semantics.

## Persisted Contract

Add this optional field to `TableComponentConfig` in the Go model and both frontend type copies:

```text
dataMode?: "paginated" | "all"
```

Every runtime resolver uses the fail-closed rule `dataMode === "all" ? "all" : "paginated"`. The absent, malformed, future, or otherwise unrecognized runtime value therefore resolves to `paginated`. This makes existing page documents backward-compatible and prevents malformed metadata from silently issuing an unbounded request.

Tenant Panel saves `dataMode` only as part of table configuration. The frontend table-configuration cleaner preserves valid values and normalizes invalid values to `paginated`. Backend frontend-metadata validation does not normalize persisted input: it accepts `paginated`, `all`, or an omitted value and rejects any other non-empty value.

## Tenant Panel Editing

The table create/edit interface includes a **Data loading** selector when the table source is `schema`:

- **Paginated** — the default; retrieves one page and displays pagination controls.
- **All items** — retrieves the schema's all-items response in one request and displays no pagination controls. Its help text says: “Loads the all-items response in one request. Use with care for large schemas.” The backend's existing maximum unbounded-read limit still applies.

The selector is hidden for pipeline and workflow sources. Changing a table from schema to pipeline or workflow does not delete or overwrite `table.dataMode`; the field becomes dormant. If the component later returns to a schema source, its prior schema-table choice is restored. While a non-schema source is active, runtime dispatch always uses the existing paginated source component.

The selector belongs in the table request settings because it controls the data request rather than table presentation.

## Runtime Dispatch and Data Flow

Tenant Panel preview and React Template use the same resolution rule, centralized in one small pure dispatch helper within each frontend rather than repeated as nested component checks:

1. Resolve the component data binding.
2. Normalize the mode with `table.dataMode === "all" ? "all" : "paginated"`.
3. If the binding kind is `schema` and the normalized mode is `all`, render `GenericUnpaginatedPage`.
4. Otherwise render `GenericPaginatedPage`.

`GenericUnpaginatedPage` uses the existing `useGetDynamicItems` hook and `GET /dynamic`. It intrinsically renders without pagination controls; callers do not hide controls around a paginated component. It receives the existing table configuration and schema name so current unpaginated columns, actions, nested rows, toggles, and row behavior remain available.

`GenericPaginatedPage` remains responsible for schema pagination and for all pipeline/workflow table sources. No pipeline or workflow request is routed to `GET /dynamic`.

Both data modes continue using schema-scoped query keys and the existing mutation invalidation behavior so create, edit, delete, bulk actions, and websocket invalidation refresh the active table data. The current keys cannot collide: the all-items hook uses the `['dynamic', schemaName, 'all', ...]` family, while paginated table-source requests use the `['dynamic', schemaName, 'table-source', ...]` family.

## Filtering, Search, and Sort

The existing `GET /dynamic` controller reads `schemaName` but does not parse filter, search, or sort query parameters. All-items mode therefore loads the route's existing unfiltered response and does not promise request-feature parity with `/dynamic/page` or `/dynamic/table-source`. This feature does not add backend filter/search/sort support or client-side pagination.

The existing unpaginated component may retain its current client presentation controls, but tests for this feature must not claim that configured request filters, server search, or server sort affect the all-items response. Extending that endpoint contract is separate work.

Switching modes in saved configuration uses distinct query-key families and therefore cannot reuse a paginated response as an all-items response.

## Error Handling and Safety

- Existing loading, empty, and request-error states remain in their respective table components.
- Invalid or missing `dataMode` values fall back to paginated behavior at runtime.
- Backend validation rejects explicitly invalid persisted values.
- The UI labels all-items mode clearly because it can issue a large response for a large schema.
- This delivery does not add a row-count threshold or confirmation dialog; administrators explicitly opt into the unbounded route.

## Testing

Backend tests cover:

- omitted `dataMode` remains valid;
- `paginated` and `all` are valid;
- any other non-empty mode is rejected;
- JSON/BSON page-model serialization preserves the field.

Tenant Panel tests cover:

- valid cleaning/default behavior;
- the selector is available for schema tables and excluded from pipeline/workflow behavior;
- preview dispatches schema `all` to the unpaginated component;
- omitted mode and schema `paginated` dispatch to the paginated component;
- pipeline/workflow dispatch remains paginated even if stale metadata contains `all`;
- a schema table saved as `all` can change to a pipeline source without deleting `dataMode`, uses the existing pipeline path while dormant, and resumes all-items mode after changing back to schema.

React Template tests cover the same runtime dispatch rules, the fail-closed normalization helper, and the distinct query-key families. They verify that the selected path uses the existing unfiltered all-items query contract without changing pipeline/workflow requests.

## Acceptance Criteria

- An administrator can select Paginated or All items while creating or editing a schema-backed table.
- The setting persists with the page component.
- Existing tables continue to use paginated requests.
- A schema table configured for All items calls the all-items dynamic route and shows the route's returned rows without pagination controls.
- Schema tables configured for Paginated continue to call the paginated route.
- Pipeline and workflow table behavior is unchanged.
- Tenant Panel preview and React Template runtime behave consistently.
