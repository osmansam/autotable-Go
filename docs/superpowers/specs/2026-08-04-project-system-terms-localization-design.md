# Project System Terms Localization Design

## Goal

Allow each project to customize predefined application terms per enabled locale. For example, a project can display `İşlemler` instead of `Actions` in Turkish. These terms use the existing project localization workflow and remain separate from tenant-created entities and data.

## Scope

The first version provides a predefined catalog of system terms used by react-template. Tenant users can edit translations but cannot create arbitrary system keys. The initial catalog includes common navigation, table, form, action, confirmation, search, selection, validation, loading, success, and error terms.

Examples include:

- Actions, Add, Create, Edit, Delete, Save, Cancel, Close
- Search, Filter, Select, Select All, Clear, Reset
- Yes, No, Confirm, Continue, Back, Next
- Loading, No data, Success, Error

The catalog must contain only terms that react-template actually uses through i18next. Additional predefined terms can be introduced in later releases without a database migration.

## Data Model

System-term translations use the existing `project_translations` collection:

- `resourceType`: `system`
- `resourceId`: stable system key
- `translationKey`: `system:<key>`
- `sourceText`: the source-locale term used by react-template
- `translatedText`: generated or manually edited project translation
- existing locale, status, origin, hash, active, and audit fields remain unchanged

The source text is the i18next lookup key. Stable translation keys prevent display-text changes from creating ambiguous records.

## Discovery and Generation

Backend localization discovery appends the predefined system catalog to page and container source strings. Saving locale settings with AI generation enabled generates missing current translations for every non-source enabled locale.

When new predefined terms are added later, regeneration creates only missing or outdated generated records. Existing current manual translations are never overwritten.

## TenantPanel

The project Localization page adds a **System terms** section within the translation editor.

- The selected target locale controls which system translations are displayed.
- Each row shows the source term, translated value, status, and origin.
- Editing a value saves it through the existing manual translation endpoint.
- System terms can be filtered separately from tenant-created content.
- The UI does not permit adding, renaming, or deleting system keys.

## Runtime

The public runtime translation endpoint returns current active system translations with all other project translations. React-template installs them in the same i18next `translation` namespace.

Existing calls such as `t("Actions")` resolve the project-specific value. No special table or action rendering branch is required.

Runtime responses use the array-based `sourceText` and `translatedText` shape so Axios key normalization cannot alter localization keys.

## Error Handling

- Missing translations fall back to the source term through i18next.
- Failed AI generation marks only the affected records/job as failed and does not remove current translations.
- Invalid or unknown system keys cannot be written through the management API.
- Manual values remain available if later AI jobs fail.

## Testing

- Discovery tests verify representative predefined keys and stable `system:<key>` translation keys.
- Repository/service tests verify manual system translations cannot be overwritten by AI.
- Controller tests verify runtime output includes current active system terms.
- TenantPanel tests verify filtering and manual editing behavior.
- React-template tests verify `Actions` resolves to a project-specific value such as `İşlemler`.

## Success Criteria

- A tenant user can edit `Actions` to `İşlemler` for Turkish from the project Localization page.
- React-template displays `İşlemler` anywhere it calls `t("Actions")`, including table action columns.
- AI can fill missing predefined terms for newly enabled languages.
- Regeneration never overwrites manual system-term translations.
