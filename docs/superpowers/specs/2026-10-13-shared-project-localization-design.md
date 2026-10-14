# Shared Project Localization Design

**Date:** 2026-10-13

## Goal

AutoTable will become the single localization service for an AutoTable project. Tenant Panel and Ozmishow will both manage the same enabled languages and translation records, while Ozmishow continues to render one shared component structure for every locale.

An owner can enable a language and request AI generation from Tenant Panel. Editors can then review or change the same translations from either Tenant Panel or the Ozmishow admin. The public Ozmishow site reads those translations from AutoTable and falls back safely to the source language when a translation is missing or unusable.

The OpenAI integration remains server-side in the Go backend. Ozmishow will not require its own OpenAI API key or call OpenAI directly.

## Existing Systems

### AutoTable Go backend

The backend already provides project locale settings, centralized translation records, manual translation updates, AI translation jobs, and public runtime settings and translation routes. Project locale fields are stored on the project record:

- `sourceLocale`
- `defaultLocale`
- `enabledLocales`
- `localizationVersion`

The current AI discovery process scans legacy page definitions and container metadata. It does not yet discover Ozmishow's dynamic `sitePages`, `visualBlocks`, or `reusableBlocks` records.

### Tenant Panel

Tenant Panel already has a Localization area with Languages and Translations tabs. It calls the tenant-scoped project locale and translation APIs, can request AI generation while saving language settings, and permits manual editing of generated translations.

### Ozmishow

Ozmishow already uses AutoTable as its backend and resolves projects from the tenant and project route context. It has a unified localization model in which components are locale-independent and text is represented by stable keys in resource-owned locale dictionaries.

Ozmishow currently reads locale settings from its `siteConfig` record and writes target translations into the owning resource's embedded locale dictionary. The recently added Next.js OpenAI translation route duplicates functionality that belongs in AutoTable.

## Selected Architecture

AutoTable is the authoritative source for project language settings and target-language translations.

- The AutoTable project record is authoritative for source, default, and enabled locales.
- Ozmishow resource dictionaries remain authoritative for source-language text because they define the original content attached to each component structure.
- AutoTable `TranslationEntry` records are authoritative for target-language text.
- Tenant Panel and Ozmishow use the same localization services and repository.
- The public Ozmishow application reads current target translations from AutoTable.
- Existing embedded Ozmishow target dictionaries are retained temporarily as migration fallback data but are not written after cutover.

This avoids two-way synchronization between independent translation stores. Both editors operate on one target-translation record, so the latest successful edit is immediately visible through either interface.

Ozmishow keeps the locale fields in `siteConfig` for schema compatibility during the transition. Its repository overlays the authoritative AutoTable settings before routing or rendering. Stored `siteConfig` values are used only as a temporary availability fallback and are not presented as a second administrative source of truth.

## Translation Identity

AutoTable's unique translation index is scoped by tenant, project, locale, and `translationKey`. Ozmishow dictionary keys are stable within their owning resource but are not assumed to be globally unique across every resource.

Discovered Ozmishow translations therefore use a canonical backend key composed from:

```text
ozmishow:<resource-type>:<resource-id>:<local-dictionary-key>
```

The original Ozmishow dictionary key is stored as the translation entry's property path. The resource type and resource ID identify the owning `sitePages`, `visualBlocks`, or `reusableBlocks` record. Ozmishow groups returned entries by owner and reconstructs the in-memory locale dictionary expected by its existing localization resolver.

Canonical keys must remain stable when blocks are reordered. They use the stable resource, block, node, and repeated-item identifiers already produced by Ozmishow rather than array positions or source text.

## Translation States

The translation model will distinguish the following conditions:

- `missing`: the source key exists but no target text has been supplied.
- `current`: the translated text corresponds to the current source hash.
- `outdated`: the source text changed after the target text was created.
- `failed`: generation was attempted but failed for this entry.

An untranslated origin is added for placeholder entries. AI generation changes the origin to `ai`; a user edit changes it to `manual`.

Source reconciliation creates or updates placeholder records for every enabled target locale. It preserves non-empty translated text. When the source hash changes, the entry becomes outdated rather than losing its existing translation.

Automatic generation fills missing, outdated AI, and failed entries. It never overwrites a non-empty manual translation. An explicit future force-regeneration feature is outside this design.

Only active, non-empty, current translations are returned by the public runtime endpoint. Editors can see missing, outdated, and failed entries.

## Backend Components

### Ozmishow source discovery

The localization discovery service will scan these project dynamic collections in addition to the existing legacy sources:

- `sitePages`
- `visualBlocks`
- `reusableBlocks`

For each record it reads `localization[sourceLocale]`. Each string entry becomes a `SourceString` using the canonical backend key, owning resource metadata, original local dictionary key, source value, and source hash.

The backend reads only declared dictionary entries. It does not recursively translate arbitrary strings in dynamic records, which prevents URLs, CSS values, identifiers, data bindings, and structural configuration from being treated as user-facing copy.

Discovery remains compatible with projects that do not contain Ozmishow schemas. A missing Ozmishow collection is treated as an empty source rather than a job failure.

### Reconciliation service

A shared reconciliation service compares discovered sources with existing translation entries. It:

1. Creates missing placeholders for every enabled target locale.
2. Refreshes resource metadata and discovery timestamps.
3. Marks changed-source entries outdated.
4. Marks removed keys inactive and records `orphanedAt`.
5. Preserves current manual translations and all non-empty outdated text.

Reconciliation runs before an AI generation job and can also be requested by an authenticated Ozmishow editor after source-language content is saved.

### Translation service

The existing OpenAI provider and batching logic remain in the Go backend. Generation operates on reconciled entries and records per-entry failures without making the source site unavailable.

Project translation jobs persist their source locale, optional translation-key filter, state, and counts so clients can show pending, running, completed, and failed progress. Existing job leases are used to make pending work recoverable after a process restart rather than relying only on an in-process goroutine.

### Localization version

`localizationVersion` increments after any change that can alter runtime locale output:

- language settings change;
- manual translation update;
- generated translation batch persistence;
- source reconciliation changes current/outdated/active state.

Clients use the version in cache keys or invalidation tags. Multiple writes in one generation batch should coalesce version increments where practical; correctness is more important than minimizing the numeric increment.

## API Design

### Tenant-scoped administration

Existing Tenant Panel routes remain the administrative surface:

```text
PATCH /api/v1/tenant/projects/:id/locales
GET   /api/v1/tenant/projects/:id/translations?locale=<locale>
PATCH /api/v1/tenant/projects/:id/translations/:locale/:key
```

The locale settings route continues accepting `generateWithAI`. Saving settings first reconciles project sources, then queues translation for enabled non-source locales when generation is requested.

Job status and explicit regeneration are added:

```text
GET  /api/v1/tenant/projects/:id/translation-jobs/:jobId
POST /api/v1/tenant/projects/:id/translations/generate
```

The generation request accepts one or more enabled target locales and an optional list of canonical translation keys. It defaults to every missing/outdated/failed entry in those locales without overwriting current manual entries. Tenant Panel omits the key filter for project-wide generation; Ozmishow can send the keys visible in a component editor.

### Project-scoped Ozmishow administration

Ozmishow uses project-scoped routes backed by the same services:

```text
GET   /api/v1/:tenantSlug/:projectSlug/localization/settings
GET   /api/v1/:tenantSlug/:projectSlug/localization/translations?locale=<locale>
GET   /api/v1/:tenantSlug/:projectSlug/localization/editor/translations?locale=<locale>
PATCH /api/v1/:tenantSlug/:projectSlug/localization/translations/:locale/:key
POST  /api/v1/:tenantSlug/:projectSlug/localization/reconcile
POST  /api/v1/:tenantSlug/:projectSlug/localization/translations/generate
GET   /api/v1/:tenantSlug/:projectSlug/localization/translation-jobs/:jobId
```

The existing settings and runtime translation reads remain backward compatible and public. The separate editor read is authenticated and includes translation identity, resource metadata, source text, translated text, origin, and status. Public runtime reads expose only active, current values required to render translations.

The settings response also includes `localizationVersion`, allowing Ozmishow to key or invalidate locale caches without another project lookup.

Project-scoped mutation routes use project authentication and require an editor or administrator role. Tenant routes retain tenant authentication and scope checks. Neither client can supply tenant or project database IDs independently of its authenticated route context.

## Tenant Panel Workflow

1. The owner opens Localization and selects Languages.
2. The owner enables a target language, chooses the default language if needed, and decides whether to generate missing translations with AI.
3. Tenant Panel saves project locale settings through the existing endpoint.
4. AutoTable reconciles current sources and, when requested, queues generation.
5. Tenant Panel polls only the returned job ID while the job is active.
6. On completion it invalidates the translations query and shows generated, missing, outdated, and failed counts.
7. The Translations tab permits edits. Saving an edit marks that entry manual and current.

The empty state differentiates between no target language and a target language whose source synchronization or generation has not completed.

## Ozmishow Admin Workflow

Ozmishow loads editor locale choices from AutoTable project settings rather than treating `siteConfig` locale fields as authoritative. During transition, legacy `siteConfig` values may be used only if the settings endpoint is unavailable.

Selecting a target locale loads the centralized translation entries for that locale. The Ozmishow adapter maps entries back to the owning resource's dictionary shape, allowing the existing localization resolver, field descriptors, fallback badges, and coverage calculations to remain in place.

Target-language saves call the project-scoped translation update route. They do not patch the embedded target dictionary. Tenant Panel sees the updated manual entry on its next query refresh.

Source-language saves continue updating the owning Ozmishow resource. After a successful source write, Ozmishow requests reconciliation. Reconciliation failure does not roll back the saved source content; the editor displays a synchronization warning and offers retry.

The existing Ozmishow AI button calls the AutoTable generation endpoint. It does not call OpenAI or hold an OpenAI key. Generated values remain editable in both applications.

### Source-locale changes

Adding, removing, or changing a target language is supported. Changing the source locale after an Ozmishow project contains localized content is not part of this initial integration because source dictionaries define the canonical component copy. AutoTable rejects that change for a populated Ozmishow project with an actionable validation error. A future source-locale migration can materialize a selected target locale into every owning Ozmishow resource before changing the project setting.

## Public Ozmishow Runtime

For each request, Ozmishow resolves the project from the trusted host and loads:

1. the existing site structure and source dictionaries;
2. AutoTable runtime locale settings;
3. current translations for the selected target locale.

The translation adapter groups entries by resource and overlays them through the existing Ozmishow localization pipeline. Component type, order, style, layout, media, links, data bindings, and other structure remain shared across locales.

If localization settings or target translations cannot be loaded, the source-language site remains renderable. A target-locale request uses the source fallback rather than failing the page. Cached locale data is keyed or tagged with `localizationVersion` so successful edits become visible without a deployment.

## Error Handling and Observability

- OpenAI configuration and API errors are returned as translation-job failures, never exposed as raw secrets, and never prevent source rendering.
- Invalid or disabled target locales return a client error before generation or mutation.
- Unknown translation keys cannot be created without source and ownership metadata established by reconciliation.
- Protected placeholders and identifiers remain validated by the backend provider before generated text is stored.
- Manual entries skipped by generation contribute to the job's `skippedManual` count.
- Job error summaries are bounded and safe to display in administrative clients.
- Requests and jobs carry tenant, project, job, and request identifiers in structured logs.
- Concurrent manual updates use the repository's single authoritative entry. A completed manual edit wins over a concurrent AI upsert because generated writes exclude manual entries.

## Migration

Migration is additive, dry-run first, and non-destructive.

### Discovery and report

The migration reads source and target dictionaries from Ozmishow `sitePages`, `visualBlocks`, and `reusableBlocks`. Its dry-run report includes:

- resources and source keys discovered;
- translations found per locale;
- canonical backend keys to create;
- duplicate or invalid keys;
- missing resource identifiers;
- entries that would be inserted, updated, skipped, or marked inactive;
- deterministic counts and checksum.

### Backfill

Apply mode upserts existing non-source dictionary values as manual translation entries so migration never allows later AI generation to overwrite reviewed text. Blank target values become missing placeholders. Existing AutoTable manual entries win over migrated values.

The migration does not delete or rewrite Ozmishow dictionaries. It records enough ownership metadata to map every inserted entry back to its resource and local key.

### Compatibility window

During rollout, Ozmishow reads centralized translations first and falls back to an embedded target value only when no backend entry exists. Ozmishow stops writing target dictionaries as soon as centralized writes are enabled. This is a temporary dual-read period, not dual-write synchronization.

After parity checks pass, the fallback remains available for one release as recovery support. Removing old embedded target dictionaries is outside this implementation and requires a separate explicit cleanup decision.

## Rollout Order

1. Add backend discovery, reconciliation, project-scoped mutation routes, generation/status routes, and localization-version updates.
2. Update Tenant Panel to show job progress and the richer translation states.
3. Add the AutoTable localization adapter to Ozmishow while retaining legacy reads.
4. Run and review the localization migration dry run.
5. Apply the translation backfill.
6. Switch Ozmishow admin target writes to AutoTable.
7. Switch public rendering to centralized translations with embedded fallback.
8. Verify cross-editor parity and public routes.
9. Remove the duplicate Next.js OpenAI route, helper, tests, and Next-side OpenAI configuration.

Each phase remains backward compatible with the previous phase so deployments do not require an all-at-once release.

## Testing

### AutoTable Go backend

- Ozmishow resource discovery from each supported dynamic collection.
- Canonical key uniqueness across resource types and resource IDs.
- Missing collections and malformed records.
- Placeholder creation for newly enabled locales.
- Current, missing, outdated, failed, inactive, AI, and manual transitions.
- Preservation of manual translations during reconciliation and generation.
- Protected token validation and incomplete OpenAI responses.
- Tenant and project authentication boundaries; project-role authorization is temporarily deferred.
- Job status, restart recovery, bounded errors, and progress counts.
- `localizationVersion` changes after every runtime-affecting operation.
- Runtime responses exclude missing, failed, outdated, empty, and inactive entries.

### Tenant Panel

- Enabling a language with and without AI generation.
- Job progress and completion refresh.
- Editing AI, outdated, and missing translations.
- Clear empty, configuration-error, generation-error, and retry states.
- Query invalidation after locale, job, or translation changes.

### Ozmishow

- Locale settings loaded from AutoTable with transition fallback.
- Backend entries mapped to the correct page, visual, and reusable resource dictionaries.
- Stable keys across block and item reordering.
- Source fallback for every non-current target state.
- Target edits update AutoTable without changing component structure or source dictionaries.
- Source edits request reconciliation and report synchronization failures.
- Cache invalidation by localization version.
- No browser or Next.js server path calls OpenAI directly.

### End-to-end acceptance flow

1. Enable Turkish in Tenant Panel and request AI generation.
2. Observe job completion and translations in Tenant Panel.
3. Open the Ozmishow Turkish editor and see the same values.
4. Change one translation in Tenant Panel and another in Ozmishow.
5. Refresh both editors and confirm both edits appear in both places.
6. Visit the public Turkish route and confirm the translations render on the same component structure.
7. Change the English source text and confirm the Turkish entry becomes outdated without being deleted.
8. Regenerate missing/outdated AI entries and confirm the manual translation remains unchanged.
9. Disable or simulate OpenAI failure and confirm source fallback and manual editing continue to work.

## Out of Scope

- Translating images, links, CSS, component structure, or data bindings.
- Supporting a second translation provider in this implementation.
- Automatically deleting legacy embedded target dictionaries.
- Automatically overwriting current manual translations.
- Changing the source locale of a populated Ozmishow project.
- A general-purpose localization system for arbitrary dynamic schemas that do not expose explicit locale dictionaries.

## Success Criteria

- Tenant Panel is the single place to enable or disable project languages.
- Both Tenant Panel and Ozmishow edit the same target translation records.
- AI generation runs only in AutoTable and requires no Ozmishow API key.
- Enabling a language discovers Ozmishow text and makes it available to both editors.
- The public Ozmishow site renders one component structure with locale-specific text overlays.
- Missing, failed, or outdated translations fall back to source text without breaking a page.
- Manual translations survive source changes, reconciliation, migration, and AI generation.
- Existing translations migrate without destructive updates and remain recoverable during rollout.
- Translation changes appear without redeploying Ozmishow.

