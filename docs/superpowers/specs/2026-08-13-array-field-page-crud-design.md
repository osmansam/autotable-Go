# Array Field Page CRUD Design

## Summary

Add first-class page support for displaying and mutating one embedded array on a single parent record. A route such as `checklist/:id` supplies the parent record ID through a binding such as `{{route.id}}`. The page designer selects the parent schema and array field, generates a table and CRUD actions from the array child definitions, and permits customization without losing the generated defaults.

The feature spans `autotable-Go` (metadata validation and atomic array mutation APIs), `tenantPanel` (page design and preview), and `react-template` (generated-page runtime). Existing work for table `dataMode: "arrayField"`, `arraySource`, and `generatedRelationColumns` is the foundation and must be extended rather than replaced.

## Goals

- Bind a page component to one parent record using a route value such as `{{route.id}}`.
- Turn a selected embedded `array` field on that record into table rows.
- Automatically generate columns, Add, Edit, Delete, and optional Reorder behavior from the array's child fields.
- Let the page designer customize or disable generated columns, form fields, and actions.
- Generate Boolean membership columns for relational child fields such as `duties.locations`.
- Mutate individual array rows atomically instead of sending a stale copy of the entire parent array.
- Keep page metadata and runtime behavior consistent in `tenantPanel` and `react-template`.

## Non-goals

- Treating primitive arrays such as `stringArray` as editable row tables.
- Making embedded array rows independent top-level resources or schemas.
- Supporting arbitrary MongoDB paths or accepting MongoDB operators from clients.
- Automatically migrating existing array data to add child `_id` values.
- Replacing normal top-level schema tables or expandable read-only nested rows.

## Page and data binding

The page route uses the existing `:param` convention, for example `checklist/:id`. Component parameters use the existing binding expression syntax, for example `{{route.id}}`.

An array-backed table binds to:

1. A parent schema, such as `checklist`.
2. A parent record ID expression, such as `{{route.id}}`.
3. An embedded field of type `array`, such as `duties`.
4. A configured child identity field, such as `duty` or `order`.

The parent ID identifies the one document that every operation updates. The child identity field identifies the one array entry affected by Edit, Delete, relation membership edits, and Reorder. The designer must only offer child scalar fields suitable for stable comparison. It must warn that the chosen value must be unique within the parent array. The backend rejects an operation with `409 Conflict` if the configured identity matches multiple rows.

The table configuration extends the existing array source metadata rather than adding a second competing model:

```json
{
  "dataMode": "arrayField",
  "binding": {
    "kind": "schema",
    "schemaName": "checklist",
    "parameters": {
      "id": {
        "source": "route",
        "field": "id"
      }
    }
  },
  "arraySource": {
    "enabled": true,
    "field": "duties",
    "rowIdentityField": "duty"
  }
}
```

The exact persisted representation of the route parameter must follow the project's existing parameter-binding parser. The designer may display `{{route.id}}`, but it must save the same normalized binding structure used by other component source parameters.

## Generated configuration and customization

When Array field rows is selected, `tenantPanel` presents this sequence:

1. Select the parent schema.
2. bind the parent record ID, normally to `{{route.id}}`.
3. Select an eligible embedded `array` field.
4. Select its row identity child field.
5. Generate table columns and form inputs from the selected array's children.
6. Review and customize generated behavior.

Generation defaults are:

- One table column for each supported child field.
- One Add action with an empty generated form.
- One Edit action using the same field definitions and selected-row values.
- One Delete action with confirmation.
- Reorder disabled unless explicitly enabled and mapped to a numeric child order field.
- `objectId` and `objectIdArray` children rendered with the existing population settings when available.
- Generated relation columns offered for relational child fields, but not enabled implicitly when they could create a very wide table.

The designer can change labels, ordering, visibility, table rendering, form input types, defaults, validation presentation, action labels, confirmation copy, and whether each generated action is enabled. Generated configuration is ordinary persisted page configuration after creation; runtime code does not infer a different UI on every render.

Regeneration reconciles by child field name:

- Preserve existing customization for fields that still exist.
- Add newly discovered fields using current defaults.
- Mark configuration for removed or incompatible fields as invalid for user review rather than silently deleting it.
- Never re-enable an action the designer explicitly disabled.

## Generated relation columns

A relation group converts records from a source schema into Boolean membership columns backed by an array child field. For example, every `location` record becomes a column and each `duties.locations` value determines whether the cell is checked.

```json
{
  "id": "duty-locations",
  "arrayField": "locations",
  "sourceSchemaName": "location",
  "sourceIdField": "_id",
  "sourceLabelField": "name",
  "sourceLimit": 100
}
```

The existing `generatedRelationColumns` model remains authoritative. In an array-backed table, `arrayField` refers to a field on the child row. Source records are fetched using the configured schema, ID field, label field, and limit. A cell edit calls the array-row Edit operation with only the changed relation field. Optimistic display is allowed, but failures must restore the confirmed membership and show the server error.

## Backend API

Add narrow dynamic routes for embedded array mutations. They use the same tenant, project, authentication, authorization, schema route configuration, workflows, cache invalidation, and auditing conventions as existing dynamic-item mutations.

```http
POST   /dynamic/:schema/:id/array/:field
PATCH  /dynamic/:schema/:id/array/:field/:rowIdentity
DELETE /dynamic/:schema/:id/array/:field/:rowIdentity
PATCH  /dynamic/:schema/:id/array/:field/reorder
```

`rowIdentity` is URL encoded. Edit and Delete also receive the configured `rowIdentityField` in a validated request property or query parameter; the server does not trust a client-provided arbitrary path. The field must be a direct child of the selected array in the container metadata.

### Add

The request body contains the new child object. The service converts and validates values from the array's child definitions, checks any required uniqueness constraints, and appends the new child atomically. It returns the updated child and enough parent metadata for the client cache to refresh.

### Edit

The request body contains a partial child update. The service locates exactly one child using the configured identity field and value, merges the patch with that child, performs full child validation on the merged result, and atomically replaces that array entry. Changing the identity field is allowed only when the resulting identity is unique.

### Delete

The service locates exactly one child using the configured identity and atomically removes it. Deletion cannot remove every matching value when the data is accidentally duplicated; duplicated matches produce a conflict that must be corrected explicitly.

### Reorder

The request contains an ordered list of child identity values and the configured numeric order field. It must contain every current child identity exactly once. The service writes zero-based order values atomically. Add defaults a configured order field to the next position when the client omits it.

### Atomicity and concurrency

Mutations use a MongoDB update constrained by the parent `_id`, validated array field, and child identity. When necessary, the service uses a transaction or compare-and-update filter so validation and persistence cannot silently overwrite a concurrent change. No endpoint performs an unrestricted client-driven replacement of the complete array for a one-row mutation.

## Validation and errors

The backend validates all metadata-derived paths before building a database update:

- The schema exists in the current tenant and project.
- The parent ID is valid and the parent record exists.
- The selected field is a direct container field of type `array` with children.
- The identity field is a supported direct child field.
- Add and merged Edit values satisfy the child types, required tags, enum rules, lengths, relation formats, and configured uniqueness rules.
- Referenced IDs belong to the child field's configured schema where reference validation is enabled.
- Edit and Delete match exactly one child.
- Reorder identities are complete, unique, and refer to current rows.

Responses use:

- `400 Bad Request` for malformed values, invalid configuration, or incomplete reorder input.
- `404 Not Found` for a missing schema, parent, array row, or referenced record where appropriate.
- `409 Conflict` for duplicate child identities or concurrent modification.
- Existing authentication and authorization status behavior for access failures.

Errors use the existing dynamic API error envelope and include a user-readable message. The frontend keeps an Add/Edit form open with entered values after a validation error. A conflict prompts a parent-record refresh and does not present the stale optimistic state as saved.

## Runtime behavior

Both `tenantPanel` preview and `react-template` runtime resolve the parent ID from route bindings and fetch one parent record. The selected array is flattened with the existing array-source row metadata. Synthetic table IDs remain presentation identifiers only; API mutations use the parent ID plus configured child identity.

- Add opens the generated empty form and appends the returned child after success.
- Edit opens the generated form populated from the selected child and merges the server response after success.
- Delete shows confirmation using the configured display or identity field and removes the row after success.
- Generated relation switches issue a partial Edit for their one child relation field.
- Reorder sends the complete ordered identity list and updates visible order only after confirmation, or rolls back optimistic ordering on failure.
- Successful mutations invalidate and refresh the parent source while preserving compatible table state such as toggles and visible columns.

The two frontends share equivalent helpers, types, and tests. Changes should be ported deliberately between them rather than depending on build artifacts copied from one repository.

## Authorization, workflows, and cache behavior

Array operations are mutations of the parent schema. They require the equivalent parent Create/Update/Delete permissions selected by action policy: Add and Edit/Reorder use update permission, while Delete additionally honors the configured delete action permission where the route model distinguishes it. The implementation must document and test the final mapping against existing dynamic route authorization.

The operations run the same applicable before/after workflow hooks, audit recording, Redis invalidation, and triggered-schema invalidation as a parent update. Hooks receive both the previous and updated parent values plus array mutation context (`operation`, `arrayField`, `rowIdentityField`, and `rowIdentity`) so behavior is inspectable.

## Testing

### `autotable-Go`

- Page model BSON/JSON round trips for parent binding, `arraySource`, generated actions, and generated relation groups.
- Frontend validation accepts complete array-table configuration and rejects missing or incompatible schema, field, and identity selections.
- Service and repository tests cover Add, partial Edit, identity change, Delete, relation membership edit, and Reorder.
- Tests cover invalid parent IDs, absent parents, non-array fields, invalid child values, zero matches, duplicate matches, duplicate reorder identities, incomplete reorder sets, and concurrent conflicts.
- Route/controller tests cover status codes, auth behavior, workflow context, audit behavior, and cache invalidation.

### `tenantPanel`

- Designer tests cover eligible field options and automatic generation.
- Reconciliation tests prove customization and explicit action disabling survive regeneration.
- Validation tests cover missing parent binding, array field, identity field, and invalid relation group configuration.
- Preview tests cover `{{route.id}}` resolution and all mutations.

### `react-template`

- Unit tests cover array flattening, mutation request construction, synthetic-ID separation, route binding, form-value mapping, and relation membership updates.
- Component tests cover Add, Edit, Delete confirmation, Reorder, loading, empty arrays, validation failures, and conflict rollback/refresh.
- Equivalent fixtures must exercise the supplied checklist example: `checklist/:id`, `duties` rows, and generated `location` columns backed by each duty's `locations` array.

## Compatibility and rollout

Existing paginated, all-items, and nested-row tables remain unchanged. Existing `arrayField` page configurations continue to render. If they lack generated CRUD metadata, they remain read-only until the designer saves enabled actions. This avoids silently granting mutation behavior to existing pages.

The feature should be delivered in this order: backend model/API support, tenant designer generation and validation, tenant preview mutation behavior, then matching generated-runtime behavior. Each stage must keep current page JSON readable by older runtime code through optional fields and `omitempty` serialization.

## Acceptance criteria

- A designer can create a page at `checklist/:id`, bind the source ID to `{{route.id}}`, select `checklist.duties`, and generate a usable table without hand-writing page JSON.
- The generated table can add, edit, and delete one duty while updating only the checklist identified by the route.
- The designer can customize generated columns/forms/actions and those changes survive regeneration.
- A generated column exists for each selected location record and editing a switch changes only that duty's `locations` membership.
- Optional drag reorder persists the configured `order` child field.
- Invalid or ambiguous child identities never update or delete multiple rows.
- Backend and both frontend test suites cover the configuration and runtime behavior.
