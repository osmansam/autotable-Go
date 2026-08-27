# Relation Matrix Row Filters Design

## Goal

Allow page authors to configure an interactive filter panel for relation-matrix rows using the same filter contract and user experience as a table. Filters affect only the row-schema request; the column set and relation membership behavior remain unchanged.

## Configuration Contract

`RelationMatrixConfig` gains an optional `filterPanel` property of the existing `TableFilterPanelConfig` type:

```json
{
  "rowSchemaName": "users",
  "rowIdField": "_id",
  "rowLabelField": "name",
  "columnSchemaName": "roles",
  "columnIdField": "_id",
  "columnLabelField": "name",
  "targetArrayField": "users",
  "targetItemMatchField": "userId",
  "columnLimit": 100,
  "filterPanel": {
    "inputs": [
      {
        "formKey": "status",
        "type": "select",
        "label": "Status",
        "optionsSource": "static",
        "staticOptionsJson": "[{\"value\":\"active\",\"label\":\"Active\"}]"
      }
    ]
  }
}
```

The property is optional. Existing relation-matrix configurations without it continue to load and render unchanged. No new filter format is introduced.

## Runtime Data Flow

The relation matrix continues to issue two independent paginated requests:

1. The row request uses `rowSchemaName`, the existing row limit, and the applied filter-panel values.
2. The column request uses `columnSchemaName`, `columnLimit`, and no row filter values.

The runtime initializes filter form state from configured defaults using the same table-filter helpers. Users edit draft values in the shared filter panel. Applying filters updates the applied row-filter state and therefore the row query key and request. Clear All Filters empties the values using the existing table behavior and refreshes rows. Columns are not refetched because their request parameters do not change.

Empty filter values follow the existing table serialization rules and are omitted from the row request. Schema-backed select filters load their options through the established table filter option-query behavior.

Changing row filters must not clear local relation membership overrides or edit-toggle state. Rows that disappear because of a filter may reappear with the current server membership state when the filter is reset.

## Runtime UI

The customer-facing `RelationMatrix` renders the existing table `FilterPanel` through `GenericTable` in the same location and visual style as a configured table filter panel. The matrix does not introduce a separate filter toolbar.

The filter panel supports the input types already supported by `TableFilterPanelConfig`, including text, number, select, date, and date-range behavior where available. Existing Apply and Reset interactions, labels, placeholders, option display, and responsive styling are reused.

The matrix keeps search, pagination, rows-per-page, Excel export, column filters, and orientation controls disabled unless separately configured in a future feature.

## Page Designer

The relation-matrix editor in `tenantPanel` gains a Row Filters section. It reuses the existing table filter-input editing conventions instead of exposing raw JSON.

Authors can:

- add and remove row-filter inputs;
- select a field from the configured row schema;
- set the supported filter input type, label, placeholder, and default value;
- configure static or schema-backed select options using the existing filter field contract;
- save, reopen, import, and export the configuration without data loss.

Changing the row schema clears filter inputs that reference the previous schema, matching how other dependent relation-matrix fields are reset.

## Backend Model and Validation

The Go `RelationMatrixConfig` adds `FilterPanel *TableFilterPanelConfig` with `filterPanel,omitempty` JSON and BSON names.

`ValidateRelationMatrixConfig` calls the existing `ValidateFilterPanelConfig`. Validation errors are wrapped as `relationMatrix filterPanel: <reason>` so API consumers can identify the failing component contract. Existing validation of row, column, target-array, and column-limit fields remains unchanged.

## Frontend Type Synchronization

Both `tenantPanel` and `react-template` add `filterPanel?: TableFilterPanelConfig` to `RelationMatrixConfig`. The JSON property name and nested input fields must exactly match the Go model.

## Error Handling

Invalid relation-matrix filter configuration is rejected when the page is saved. At runtime, row-query errors continue through the existing dynamic-query error behavior; a column request failure is not disguised as a row-filter problem.

If a schema-backed filter option request fails, the shared filter control retains its established empty/error behavior. Applying other valid filters remains possible.

## Testing

Backend tests cover JSON/BSON round trips, valid filter panels, and wrapped validation failures for malformed filter inputs.

Tenant-panel tests cover relation-matrix filter initialization, editing, dependent reset when the row schema changes, cleaning, and save/import persistence.

React-template tests cover:

- default filter-state initialization;
- omission of empty values;
- applied values passed to the row query;
- an unchanged empty filter object passed to the column query;
- Apply and Reset behavior;
- schema-backed select option handling through the shared filter machinery;
- unchanged relation membership editing and toggle behavior.

Full backend tests, both frontend test suites, and both production builds must pass before completion.

## Non-Goals

- Filtering relation-matrix columns.
- Adding matrix search, pagination, export, or column-filter controls.
- Creating a new filter configuration format.
- Sharing one matrix filter state with other page components.
- Filtering relation membership values independently of row records.
