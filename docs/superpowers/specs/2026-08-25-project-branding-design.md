# Tenant and Project Branding Design

## Summary

AutoTable will support consistent branding across projects. A tenant may define defaults and each project may override individual values. The backend resolves the effective branding; `react-template` renders it in appropriate identity surfaces; `tenantPanel` provides upload, preview, inheritance, and reset controls.

The initial release covers names, logos, favicon, one accent color, and optional login-page branding. It deliberately excludes arbitrary CSS or HTML, external asset URLs, custom domains, email branding, and role-based management restrictions.

## Goals

- Let users with access to a tenant configure tenant-wide branding defaults.
- Let users with access to a project override branding for that project.
- Render the correct identity when moving between projects without rebuilding a frontend.
- Brand public login surfaces without exposing management metadata.
- Preserve usable AutoTable defaults for existing and partially configured records.
- Leave a stable authorization boundary where role restrictions can be introduced later.

## Non-goals

- Custom CSS, HTML, fonts, layouts, or arbitrary scripts.
- User-supplied external asset URLs.
- Custom domains, email templates, or social sharing metadata.
- Branding revisions or scheduled publication.
- Subscription- or role-based feature gating.

## Branding Model

Both `Tenant` and `Project` embed an optional, typed `branding` object. Fields are optional so inheritance works per field rather than as an all-or-nothing object.

```text
Branding {
  displayName?: string
  logo?: BrandingAsset
  compactLogo?: BrandingAsset
  favicon?: BrandingAsset
  logoAlt?: string
  primaryColor?: string
  loginBrandingEnabled?: boolean
  version?: integer
}

BrandingAsset {
  url: string
  provider: "cloudinary"
  assetId: string
  width: integer
  height: integer
  format: string
  bytes: integer
}
```

`assetId` is management-only metadata and is never returned by the public runtime endpoint. `version` increments when branding changes and supports cache invalidation. Storage fields may use a provider-neutral name even though Cloudinary is the initial provider.

### Effective-value precedence

For every field independently:

```text
project override -> tenant default -> AutoTable platform default
```

An absent project field inherits. Removing an override unsets that project field; it does not copy or delete the tenant value. Empty strings are not stored as meaningful overrides.

Fallbacks within the resolved result are:

- `compactLogo` falls back to `logo`.
- `favicon` falls back to `compactLogo` where its format is browser-compatible, otherwise to the AutoTable favicon.
- `logoAlt` falls back to the effective display name.
- `displayName` falls back to the project name before the AutoTable product name.

## Backend Architecture (`autotable-Go`)

### Models and resolution

Add the typed branding structures to the tenant/project models and a pure resolver that accepts platform defaults, tenant branding, and project branding. All public consumers receive a resolved view model rather than raw database objects.

Existing tenant and project records need no data migration. Missing `branding` fields naturally resolve through the fallback chain.

Project cloning and templates copy the project branding overrides as metadata. A cloned project can subsequently remove overrides to inherit its tenant defaults. Cloudinary assets are referenced, not duplicated, during cloning.

### Management API

Authenticated management routes support:

- Reading raw tenant branding, including inheritance state.
- Partially updating tenant branding.
- Uploading/replacing/removing each tenant branding asset.
- Reading raw project branding plus the effective resolved result.
- Partially updating project branding.
- Uploading/replacing/removing each project branding asset.

Updates use PATCH semantics. Omitted fields remain unchanged. Explicit reset/unset operations remove an override. Asset upload endpoints accept one named asset type (`logo`, `compactLogo`, or `favicon`) and do not accept client-provided Cloudinary identifiers.

Any authenticated member with access to a tenant may manage tenant branding. Any authenticated member with access to a project may manage project branding. Existing tenant/project scope middleware remains responsible for isolation. The handlers should expose a clear authorization seam so future role checks can be added without changing request or response contracts.

### Public runtime API

A public, rate-limited endpoint resolves a project by tenant and project slug and returns only renderable fields:

```json
{
  "data": {
    "displayName": "Acme Inventory",
    "logoUrl": "https://...",
    "compactLogoUrl": "https://...",
    "faviconUrl": "https://...",
    "logoAlt": "Acme Inventory",
    "primaryColor": "#2563EB",
    "loginBrandingEnabled": true,
    "version": 4
  }
}
```

It must never expose provider names, asset IDs, file sizes, internal tenant/project IDs, or raw override state. It must return the same generic not-found response for unknown or inactive tenant/project combinations.

The response supports ETag or equivalent version-based caching. Management updates invalidate any backend cache entry for that tenant/project. Public caching must remain short enough that a branding update is visible promptly; conditional requests avoid unnecessary payload transfer.

### Upload lifecycle and validation

The browser uploads through the backend. The backend:

1. Confirms tenant/project access and the requested asset slot.
2. Enforces request and file-size limits.
3. Validates the actual file signature, decoded image, dimensions, and allowed format.
4. Uploads to a tenant/project-scoped Cloudinary folder using a generated name.
5. Persists the new asset metadata atomically with the branding version increment.
6. Deletes the superseded Cloudinary asset only after the database update succeeds.

If upload or persistence fails, the previous branding remains active. If persistence fails after upload, the newly uploaded orphan is deleted on a best-effort basis and the error is logged. Failure to delete an old asset does not roll back a successful branding update; it is logged for operational cleanup.

The first release accepts PNG, JPEG, and WebP. SVG is deferred until a robust sanitization pipeline is implemented. Recommended constraints are configurable, with conservative defaults such as a 2 MB maximum and decoded-dimension limits that prevent image bombs. TenantPanel communicates recommended aspect ratios but the backend does not require an exact ratio.

Cloudinary delivery URLs use HTTPS and are publicly fetchable because branding must render before login. Upload, replacement, removal, and raw metadata remain authenticated operations.

### Validation and security

- Trim display name and alt text; enforce practical length limits.
- Accept colors only in canonical six-digit hex form and normalize their case.
- Reject malformed images, misleading extensions, unsupported MIME signatures, and oversized decoded images.
- Generate provider paths server-side and prevent cross-tenant asset operations.
- Do not accept arbitrary URLs, HTML, CSS, or scripts.
- Preserve CSRF, authentication, rate-limit, and request-size protections used by comparable management routes.
- Emit structured audit records when the existing audit infrastructure supports an appropriate event without coupling branding correctness to audit availability.

## Tenant Management UI (`tenantPanel`)

Add a reusable Branding editor used in tenant and project contexts.

The tenant editor manages defaults. The project editor displays both the stored project override and the resolved effective value for each field. Each project control indicates whether it is inherited or overridden and provides a `Use tenant default` action.

Capabilities include:

- Display-name and logo-alt text inputs.
- Validated hex input and color picker for the primary color.
- Upload, preview, replace, and remove/reset actions for each asset.
- Separate preview states for desktop header, collapsed sidebar, login page, and browser favicon.
- Clear file requirements and inline validation errors.
- A single `Save branding` action for text, boolean, and color changes.
- Immediate persistence for completed asset uploads, followed by query invalidation and refreshed effective values.

The UI keeps unsaved scalar edits locally and warns before abandoning them. Upload failure leaves the previous preview and value intact. Removing a project override visibly reveals the inherited tenant value.

The editor does not display role controls. Access is determined by successful backend authorization.

## Runtime Rendering (`react-template`)

Introduce a project-branding query keyed by tenant and project slug and a provider/hook that exposes the resolved branding to components. The API remains the single source of truth for inheritance.

Apply effective branding to:

- Header: primary logo and display name.
- Expanded sidebar: primary logo and display name where space allows.
- Collapsed sidebar: compact logo.
- Login screen: project identity when `loginBrandingEnabled` is true.
- Loading, empty, and error surfaces where a product identity is already present.
- Browser title and favicon.

Do not insert logos into business content or every page merely because space exists. Branding is limited to identity and navigation surfaces.

The runtime shows AutoTable defaults during initial loading to avoid blocking the application shell. When branding arrives it updates the identity surfaces, document title, favicon, and a CSS custom property for the primary color. A failed branding request or broken image silently uses the next safe fallback and must not prevent login or navigation.

When navigating between projects, the query key changes, stale project branding is not treated as the new project's branding, and document-level state is updated or restored. Components should use a shared branded-image wrapper or error handler so broken-asset behavior is consistent.

The primary color is initially limited to selected identity accents via a controlled CSS variable. It does not rewrite arbitrary component colors, preserving contrast and avoiding unexpected theme regressions.

## Data Flow

### Management

```text
tenantPanel -> authenticated branding API -> validate -> Cloudinary/MongoDB
            <- raw overrides + effective preview <- resolve/invalidate
```

### Runtime

```text
react-template -> public runtime-branding API -> tenant/project lookup
                                               -> resolve project/tenant/default
react-template <- safe effective branding + version/ETag
```

## Failure Handling

- Unknown/inactive projects receive a generic not-found response.
- A failed upload or database write keeps the previous effective branding.
- Failed old-asset cleanup is logged and retried operationally without breaking the saved setting.
- A broken delivery URL triggers component fallback and never breaks the application shell.
- If the public endpoint is unavailable, react-template uses bundled AutoTable defaults.
- Conflicting edits follow last-write-wins for the initial release; versioning is retained so optimistic concurrency can be added later.

## Testing Strategy

### Backend

- Unit tests for field-by-field precedence and every fallback.
- Validation tests for text, colors, file signatures, sizes, formats, and dimensions.
- Scope tests proving users cannot manage branding outside their tenant/project access.
- Route tests for authenticated management and public filtered responses.
- Upload service tests for success, upload failure, persistence failure cleanup, replacement cleanup, and removal.
- Tests confirming provider metadata never enters the public response.
- Tests confirming legacy records resolve to platform defaults.
- Clone/template tests confirming metadata copy and later inheritance reset.
- Cache/ETag tests covering version changes and invalidation.

### Tenant panel

- Editor tests for inherited versus overridden fields.
- Reset-to-tenant behavior and effective preview refresh.
- File validation, upload progress/failure, and preservation of prior assets.
- Dirty-form navigation warning and successful scalar saves.
- Tenant and project editor integration tests against mocked API contracts.

### Runtime frontend

- Provider/hook tests for tenant/project query isolation.
- Header and sidebar selection of primary versus compact assets.
- Login branding toggle behavior.
- Broken-image and failed-request fallbacks.
- Document title/favicon setup, project transition, and cleanup.
- Primary-color CSS variable application without unrelated theme mutation.

## Rollout

1. Add backward-compatible backend models, resolver, management endpoints, runtime endpoint, and tests.
2. Add tenantPanel management UI and verify tenant/project inheritance.
3. Add react-template provider and apply branding to identity surfaces.
4. Deploy backend before either frontend; both frontends remain compatible while branding is absent.
5. Configure representative tenant and project records and verify login, navigation, fallback, and cross-project switching in a production-like environment.

## Future Extensions

- Tenant/project role restrictions using the preserved authorization seam.
- Sanitized SVG support.
- Dark-mode assets and a richer accessible theme token set.
- Custom domains, email branding, and social share images.
- Optimistic concurrency and branding revision history.
- Plan-based white-label controls if product packaging later requires them.
