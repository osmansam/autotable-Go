# Tenant Panel Nested Array Fields Design

## Goal

Tenant panel users can define nested fields for container schema fields of type `object` and `array`.

The immediate need is schema editing only. Runtime create/edit forms in tenant applications are out of scope for this pass.

## Current State

The backend model already supports nested field metadata through `Field.Children`.

Backend validation already understands:

- `object` fields with `children`, validating the value as an object.
- `array` fields with `children`, validating each array element as an object with the configured child fields.

The tenant panel `Field` TypeScript type already includes `children?: Field[]`, and `AddFieldModal` already has `childFields` state. The missing part is the visible interface for creating, editing, removing, and reviewing those child fields.

## User Experience

When adding or editing a field in a container:

- Selecting `object` shows a "Child Fields" section.
- Selecting `array` shows the same "Child Fields" section, with copy indicating each child field describes one object inside the array.
- The user can add child fields with the same core properties used by normal fields:
  - name
  - type
  - validation tag
  - enum values when applicable
  - common boolean flags where appropriate
- The user can edit and delete child fields before saving the parent field.
- Saved container details display nested child fields beneath their parent so the structure is visible without reopening the modal.

Example saved schema shape:

```json
{
  "name": "orders",
  "type": "array",
  "children": [
    { "name": "product", "type": "string" },
    { "name": "quantity", "type": "int", "tag": "positive" }
  ]
}
```

This represents values like:

```json
{
  "orders": [
    { "product": "Board Game", "quantity": 2 },
    { "product": "Sleeves", "quantity": 5 }
  ]
}
```

## Implementation Approach

Use an inline child-field builder inside `AddFieldModal`.

This is preferred over stacked modals because the current modal already owns child field state and the user can see the parent field context while editing the nested schema.

The child-field builder should be scoped and simple:

- It reuses the existing `Field` shape.
- It avoids page/runtime form behavior changes.
- It supports one nested level for the UI in this pass. Existing data structures can still represent deeper nesting, but the interface should not try to solve unlimited recursive editing until there is a clear need.

## Data Flow

1. User opens a container in tenantPanel.
2. User adds or edits a field.
3. If the field type is `object` or `array`, user defines `children`.
4. `AddFieldModal` submits the parent `Field` with `children`.
5. `ContainerDetailsModal` updates the container fields using the existing update flow.
6. Backend persists the schema in the existing `fields.children` shape.

## Error Handling

The child-field editor should prevent:

- missing child field names
- missing child field types
- duplicate child field names under the same parent

Changing a parent field away from `object` or `array` clears or omits `children` before save.

## Testing

Tenant panel verification:

- Build tenantPanel successfully.
- Add an `array` field with child fields and confirm the payload includes `children`.
- Edit the same field and confirm child fields are preloaded.
- Confirm container details display child fields under the parent.

Backend verification is limited because nested validation already has test coverage and no backend behavior change is expected.
