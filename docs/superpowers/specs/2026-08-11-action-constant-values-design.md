# Action Constant Values Design

## Goal

Allow every record-submitting table action to define values such as `status: "ACTIVE"` without requiring the value to be shown to the user. A configured value acts as a default when its field is visible and as an enforced hidden value when its field is absent from the action form.

The behavior must remain consistent across `autotable-Go`, `tenantPanel`, and `react-template`, including paginated and unpaginated table runtimes.

## Canonical Configuration

Continue using the existing action-level map:

```json
{
  "constantValues": {
    "status": "ACTIVE"
  }
}
```

`constantValues` remains the only persisted configuration for this feature. Do not introduce a second hidden-field or submit-level representation. Values may be any JSON-compatible value: string, number, boolean, array, object, or `null`.

The Tenant Panel action editor provides a structured key/value editing experience and preserves the stored `constantValues` object. Invalid entries, including blank keys or malformed structured values, must be shown to the designer and must not be silently converted to an empty object.

## Effective Form Fields

Each runtime derives the effective rendered form keys after applying the action's explicit form fields, selected/generated schema fields, exclusions, and other existing field-selection rules. Constant behavior is decided against this final set rather than only against the raw `formFields` configuration.

For each key in `constantValues`:

- If the key is an effective rendered form key, the configured value is a default.
- If the key is not an effective rendered form key, the configured value is hidden and enforced in the outgoing record.

No synthetic input is generated for a hidden constant.

## Payload Precedence

### Visible constant field

For a create form, merge in this order:

1. configured constant default;
2. user input.

For an edit form, merge in this order:

1. configured constant default;
2. existing record value;
3. user input.

Therefore, `status: "ACTIVE"` preselects `ACTIVE` when a visible create field has no other value. On edit, an existing status is displayed instead. In both cases, the user's final choice is submitted.

### Hidden constant field

Build the normal action record first, then merge hidden constants last. A hidden `status: "ACTIVE"` is therefore submitted as `ACTIVE` even when the source record previously contained another status.

The runtime must partition constants into visible defaults and hidden enforced values before constructing initial state and the final payload. It must not merge the entire constant map last, because that would incorrectly overwrite user choices for visible fields.

## Action Coverage

Apply the behavior to every action that submits or mutates record data:

- table Add;
- table Edit;
- custom form and Update actions;
- workflow-backed versions of those actions;
- bulk Edit.

Both direct dynamic-item mutations and workflow records use the same precedence rules. Paginated and unpaginated tables must behave identically in `tenantPanel` previews and in `react-template` runtime pages.

Delete and Link actions do not submit editable records, so `constantValues` has no operational effect for those action kinds. Their existing behavior remains unchanged.

## Backend Contract and Validation

`autotable-Go` continues to serialize action-level `constantValues`. Validation rejects keys that are empty after trimming. JSON-compatible values, including explicit `null`, are accepted.

The backend does not decide whether a key is visible because effective form fields depend on frontend field generation. The frontend runtimes own the visible-default versus hidden-enforced partition.

Existing saved actions without `constantValues` retain their current behavior. Existing actions with `constantValues` use the clarified precedence rules without requiring a migration.

## Components and Boundaries

Introduce or extend a small pure frontend helper that accepts an action's constants and effective form keys and returns:

```ts
{
  visibleDefaults: Record<string, unknown>;
  hiddenValues: Record<string, unknown>;
}
```

The helper owns only classification. Form initialization continues to own default and existing-record precedence, while submission code owns merging `hiddenValues` last. This separation keeps the same rule reusable across Add, Edit, custom actions, bulk Edit, and both table data modes.

Tenant Panel owns configuration editing and cleaning. The runtime projects consume the cleaned action contract and do not parse a designer-only representation.

## Error Handling

- The designer reports blank constant keys and malformed structured values at the action being edited.
- A configuration error prevents that invalid action configuration from being saved.
- Runtime helpers treat a missing `constantValues` map as empty.
- Runtime code must not discard valid falsy values such as `false`, `0`, an empty string, or `null` merely because they are falsy.

## Testing

Backend tests verify action constant serialization and rejection of blank keys while accepting JSON-compatible values.

Frontend pure-helper tests verify:

- classification using effective rendered form keys;
- preservation of falsy and `null` values;
- visible create defaults yielding to user input;
- visible edit defaults yielding to existing values and user input;
- hidden constants overriding the normal outgoing record.

Tenant Panel designer tests verify structured editing, cleaning, validation feedback, and round-trip persistence.

Runtime integration tests cover Add, Edit, custom direct mutation, custom workflow submission, and bulk Edit. Equivalent tests cover paginated and unpaginated behavior in both frontend projects where those paths exist.

## Success Criteria

- A configured value for a rendered field appears as a default and remains user-editable.
- An existing visible edit value is not overwritten by the configured default.
- A configured value for a non-rendered field is never shown and is included in the outgoing record.
- Hidden configured values override an existing record value.
- Visible user selections override configured defaults.
- Direct mutations and workflows receive equivalent records.
- Existing action configurations without constants remain unchanged.
