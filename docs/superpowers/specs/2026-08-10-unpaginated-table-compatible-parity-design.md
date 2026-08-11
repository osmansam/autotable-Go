# Unpaginated Table Compatible Parity Design

## Goal

Give schema-backed all-items tables every paginated-table capability that is compatible with the existing `GET /dynamic` contract, in both Tenant Panel preview and React Template runtime.

## Scope

All-items tables gain compatible support for:

- custom titles;
- constant-filter create/edit behavior and protected constant fields;
- configured actions and bulk actions;
- selection state;
- table runtime outputs;
- export;
- source-revision-aware query keys;
- existing columns, toggles, nested rows, filters, generated relation columns, and drag behavior.

All-items tables do not gain pagination controls, paginated response metadata, or server-only search/sort/filter semantics. `GET /dynamic` continues to read `schemaName` only and remains subject to the backend's maximum unbounded-read limit.

## Component Contract

`GenericUnpaginatedPage` accepts the compatible superset of table props used by `GenericPaginatedPage`:

```text
schemaName
includeFields
excludeFields
actionsEnabled
isHeader
constantFilter
customTitle
tableConfig
dataBinding
componentId
outputs
resolvedParams
sourceRevision
```

Tenant Panel uses the subset it supports today: `constantFilter`, `customTitle`, `tableConfig`, and `dataBinding`. React Template additionally uses `componentId`, `outputs`, `resolvedParams`, and `sourceRevision` for runtime integration.

`dataBinding.kind` must remain `schema` for an all-items table. Pipeline and workflow dispatch never reaches `GenericUnpaginatedPage`.

`resolvedParams` is accepted for contract consistency and stable request/query-key composition, but it does not create new server-side filtering behavior. Values are normalized through the existing JSON-safe request utilities before contributing to query identity.

## Shared Runtime Outputs

Move the current paginated-only `TableOutputPublisher` into a focused shared module under the table runtime area. Both table components use the same publisher and `resolveTableOutput` adapter.

The publisher receives:

- `componentId`;
- configured output definitions;
- table output state containing current rows, selected IDs, and current page metadata where applicable.

For unpaginated tables, current rows are the full rendered all-items result and page metadata is omitted. Selected IDs continue to come from the shared selection context. Unmounting or losing required state marks outputs unavailable exactly as paginated tables do.

## Constant Values and Actions

`constantFilter` remains a legacy caller-supplied constant-value map distinct from `tableConfig.constantFilters`. The unpaginated component merges both maps using the paginated component's precedence:

1. editable form values;
2. configured create values;
3. `tableConfig.constantFilters` where currently used by the table action contract;
4. caller `constantFilter` last.

Constant keys are excluded from editable update payloads and from generated filter inputs. Creates include the constant values. This parity is local to create/edit/action behavior; it does not claim that `GET /dynamic` filters its returned rows.

## Export

All-items schema tables expose the existing export control and `ExportModal`. Export uses the existing schema export API rather than converting the currently rendered array into a new client-generated file.

The modal receives the current schema, available fields, and supported filter/search state. It must not display pagination-only choices that cannot be honored by all-items mode. No new export endpoint is introduced.

## Query Identity and Refresh

The all-items hook already uses the `['dynamic', schemaName, 'all', ...]` query-key family. Unpaginated runtime passes `sourceRevision` and normalized compatible request identity into `useGetDynamicItems`, preserving separation from paginated `table-source` keys.

Mutations and websocket schema invalidation continue invalidating the schema prefix, so both table modes refresh after changes.

## Rendering and Controls

Unpaginated tables render the same compatible table chrome as paginated tables:

- configured/custom title;
- action and bulk-action controls;
- selection controls;
- filter-panel and display-toggle controls;
- Excel upload/export where already authorized and supported;
- nested rows and drag handles.

They never receive `pagination`, `outsideSortProps`, or paginated `outsideSearchProps`. Existing local table search may remain available, but the feature does not describe it as server search.

## Implementation Boundaries

Use targeted parity changes plus small shared helpers. Do not refactor the two large table components into a single component in this delivery. Shared extraction is limited to behavior that must remain identical, especially output publication and constant-value merging where practical.

Do not change backend routes, page models, pipeline/workflow bindings, or pagination response shapes.

## Testing

Tenant Panel tests cover:

- unpaginated prop forwarding from preview;
- custom-title rendering contract;
- constant-value merge precedence and update-field protection;
- compatible selection/export controls remain available;
- no pagination props are produced.

React Template tests cover:

- schema/all runtime forwards `componentId`, `outputs`, `resolvedParams`, and `sourceRevision`;
- shared output publication resolves unpaginated rows and selected IDs;
- publisher cleanup marks outputs unavailable;
- constant values affect create/edit behavior without claiming list filtering;
- export is available for authorized schema tables;
- source revision separates all-items query identity;
- all-items rendering does not produce pagination or server outside-sort/search props.

Full frontend suites and production builds must pass in both repositories.

## Acceptance Criteria

- Switching a schema table to All items does not remove compatible table properties or controls.
- The same custom title, configured actions, bulk actions, selection, toggles, nested rows, generated columns, drag, and export behavior remain available.
- Runtime table outputs work in all-items mode.
- Constant values are preserved in create/edit behavior and cannot be overwritten through editable update fields.
- Source revisions refresh all-items queries without colliding with paginated queries.
- No pagination controls appear in all-items mode.
- No new server-side filtering, sorting, pagination, or endpoint behavior is implied or added.
