# Table Template Column Design

## Goal

Allow a table column to display a value composed from multiple row fields without requiring a matching backend schema field. For example, a `Full Name` column can render `{{name}} {{surname}}`.

## Configuration

Add `template` as a table column type and add an optional `template` string to the column configuration.

```json
{
  "field": "fullName",
  "displayName": "Full Name",
  "type": "template",
  "template": "{{name}} {{surname}}"
}
```

The configured `field` is a stable, synthetic column key. It does not need to exist in the backend schema.

## Tenant Panel

The table column type selector will include `Template`. Selecting it will show a text input for the template string. The designer will persist a trimmed, non-empty template only for template columns.

The editor will explain that placeholders use `{{fieldName}}` syntax and provide `{{name}} {{surname}}` as an example.

## Runtime Rendering

For each row, the runtime will replace every `{{fieldName}}` placeholder with the corresponding row value.

- Missing, `null`, and `undefined` values become an empty string.
- The rendered result is trimmed.
- Repeated whitespace is collapsed to one space, so a missing surname renders `John`, not `John `.
- Zero and `false` remain visible rather than being treated as missing.
- The same behavior applies to paginated and unpaginated tables and to Tenant Panel preview tables.

The template syntax is deliberately limited to field interpolation. It will not evaluate JavaScript, functions, or conditional expressions.

## Data Fields

The table data-field resolver will parse placeholder names from template columns and request those fields automatically. Source fields do not need to be visible table columns or manually listed in `dataFields`.

The synthetic column key itself will not be requested from the backend.

## Limitations

Template columns are presentation-only. Server-side filtering and sorting by the synthetic field are not supported in this version. Filtering or sorting should use the underlying schema fields instead.

## Validation

Frontend configuration cleanup will discard blank template strings. Backend page validation will reject a template column with a blank template so invalid configuration cannot be persisted through another client.

## Testing

Tests will cover:

- Template interpolation with two fields.
- Missing values and whitespace normalization.
- Preservation of zero and false values.
- Automatic source-field discovery.
- Exclusion of the synthetic field from requested backend fields.
- Designer cleanup and persistence.
- Paginated and unpaginated rendering paths.
- Backend configuration validation and serialization.
