# Translation Editor Explicit Save Design

## Goal

Make manual translation persistence visible and reliable in TenantPanel. Editing an AI-generated or manual translation must not depend on leaving the input field, and the user must see whether the save succeeded or failed.

## Interaction

Each translation row uses a controlled text input and an explicit **Save** button.

- The button is disabled while the value matches the last persisted translation.
- Editing the value enables **Save**.
- Clicking it displays **Saving…** while the PATCH request is pending.
- A successful request updates the row’s persisted value, changes its origin badge to **Manual**, disables the button, and briefly displays **Saved**.
- A failed request retains the edited text, keeps **Save** enabled, and displays the backend error through the existing toast system.
- Empty translations cannot be saved.
- Changing locale or refetching translations resets row drafts to the newly loaded persisted values.

The existing save-on-blur behavior is removed to prevent invisible requests and accidental saves.

## Data Flow

TenantPanel continues using the existing manual translation endpoint:

`PATCH /api/v1/tenant/projects/:projectId/translations/:locale/:key`

The backend decodes URL-encoded translation keys before lookup. A successful response represents a current manual translation. TanStack Query invalidates the locale translation query after success so refreshed data matches MongoDB.

## Components

Draft and save-state behavior is isolated in a focused translation-row component rather than expanding the project localization page:

- `TranslationEditorRow` owns the controlled draft value and transient saved state.
- `ProjectLocalizationSection` maps translation records to rows and supplies the existing edit mutation.
- The API mutation remains responsible for cache invalidation.

## Error Handling

- The row never replaces a failed draft with the old server value.
- The displayed error uses the backend response message when available and a generic fallback otherwise.
- Repeated clicks are prevented while the request is pending.
- A stale response cannot mark a newer draft as saved; the saved baseline is the exact value submitted.

## Testing

- A pure draft-state helper or component test verifies unchanged, changed, saving, saved, and failed behavior.
- The API mutation test verifies the encoded translation key and request body.
- TenantPanel type checking and production build must pass.
- Backend controller tests verify encoded translation keys are decoded.

## Success Criteria

- Editing `Email → E-posta` enables a visible Save button.
- Clicking Save visibly progresses through Saving to Saved.
- The row becomes Manual after success.
- Refreshing TenantPanel retains `E-posta`.
- A failed request leaves the draft editable and shows an error.
