# Project Localization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add project-scoped system and dynamic-content localization across `autotable-Go`, `tenantPanel`, and `react-template`, including persistent per-user preferences, editable translations, and OpenAI-assisted bulk generation.

**Architecture:** Use a hybrid model. Static application copy stays in versioned `react-i18next` catalogs; project-created presentation metadata stays in its existing source-language fields and receives sidecar MongoDB translations. The Go backend owns project locale configuration, translation discovery/status, user preferences, cached runtime bundles, and leased asynchronous AI jobs.

**Tech Stack:** Go, Fiber, MongoDB, Redis, React 18, TypeScript, TanStack Query, `i18next`, `react-i18next`, OpenAI Responses API with Structured Outputs.

## Global Constraints

- Ordinary entity records are never translated.
- Project presentation metadata includes page/menu names, labels, placeholders, help and description text, action text, validation text, column display names, and static option labels.
- Each project has independent `sourceLocale`, `defaultLocale`, and `enabledLocales` settings.
- Each authenticated user has one saved locale preference per project; unauthenticated startup uses local storage.
- Manual translations are never overwritten by bulk or automatic AI generation.
- A source-text change marks every matching translation outdated; bulk regeneration replaces only AI translations unless a user explicitly confirms replacement of a manual translation.
- Adding a language is free of AI side effects by default at the API level. `tenantPanel` offers “Generate all translations with AI” selected by default in the add-language confirmation, shows the discoverable item count, and sends an explicit generation request only after confirmation.
- Only backend code can read `OPENAI_API_KEY`; neither frontend receives or stores it.
- AI translation is enabled only when both `OPENAI_API_KEY` and `OPENAI_TRANSLATION_MODEL` name a model available to the deployment account; the application has no hardcoded model fallback.
- Translation endpoints use the project's existing read/edit authorization. This feature does not add a role system.
- Use BCP 47 language tags normalized to canonical casing, such as `en`, `tr`, and `pt-BR`.

---

### Task 1: Backend locale domain and persistence

**Files:**
- Modify: `models/tenantModel.go`
- Create: `models/localizationModel.go`
- Create: `repositories/localization_repository.go`
- Create: `repositories/localization_repository_test.go`
- Modify: `configs/setup.go`

**Interfaces:**
- Produces `Project.SourceLocale string`, `Project.DefaultLocale string`, `Project.EnabledLocales []string`, and `Project.LocalizationVersion int64`.
- Produces `TranslationEntry`, `ProjectLocalePreference`, and `TranslationJob` models.
- Produces repository operations used by all later backend tasks.

- [ ] Add project locale fields with defaults `sourceLocale=en`, `defaultLocale=en`, and `enabledLocales=[en]`; validate that source and default locales are enabled.
- [ ] Define translation identity as `(tenantId, projectId, locale, translationKey)` and store `resourceType`, `resourceId`, `propertyPath`, `sourceText`, `sourceHash`, `translatedText`, `origin` (`ai|manual`), `status` (`current|outdated|failed`), `isActive`, `orphanedAt`, `lastDiscovered`, audit fields, and timestamps.
- [ ] Define preference identity as `(userId, tenantId, projectId)` and job fields for status, progress, lease owner/expiry, retry count, error summary, and requested operation.
- [ ] Add unique MongoDB indexes for translation identity and user preference identity plus job polling indexes for `(status, nextAttemptAt, leaseExpiresAt)`.
- [ ] Write repository tests proving tenant/project isolation, idempotent upserts, manual-origin preservation, preference replacement, and exclusive job leasing.
- [ ] Run `go test ./repositories ./models` and commit the backend domain independently.

### Task 2: Translation-key discovery and source-change reconciliation

**Files:**
- Create: `services/localization_discovery.go`
- Create: `services/localization_discovery_test.go`
- Modify: `models/pageModel.go`
- Modify: `models/containerModel.go`
- Modify: `controllers/pageController.go`
- Modify: `controllers/containerController.go`

**Interfaces:**
- Produces `DiscoverProjectStrings(ctx, tenantID, projectID) ([]SourceString, error)`.
- Produces `ReconcileResourceStrings(ctx, tenantID, projectID, resourceType, resourceID) error` for normal write paths.
- Produces stable keys such as `page:<id>.name`, `component:<id>.column:<field>.displayName`, and `action:<id>.label`.

- [ ] Inventory all presentation-only string fields in pages, filters, components, containers, fields, actions, nested rows, validation configuration, and static options.
- [ ] Add stable IDs to editable nested objects that currently can only be identified by array position; use stable field names or option values only where uniqueness is already enforced.
- [ ] Implement deterministic discovery that excludes schema names, binding keys, field values, templates, expressions, URLs, and ordinary records.
- [ ] Define `sourceHash` as SHA-256 of `canonicalizeLocale(locale) + "\x00" + exactSourceText`; do not trim, lowercase, collapse whitespace/punctuation, or change quotation marks. If NFC Unicode normalization is adopted, apply it explicitly and test it.
- [ ] Reconcile only the changed resource after page/container create, update, or delete; increment the project localization version after a material reconciliation change. Never run whole-project discovery from a runtime bundle read.
- [ ] Mark changed entries outdated while preserving manual text. Mark undiscovered keys `isActive=false` with `orphanedAt` instead of deleting them; reactivate the same key with its translation intact if it returns.
- [ ] Reserve full-project discovery for migration, repair/manual rescan, locale generation, and scheduled consistency checks.
- [ ] Add table-driven tests covering reorder stability, duplicate labels with different keys, exact source hashing, source edits, orphan/reactivation, static option labels, resource-write reconciliation, and exclusion of entity data.
- [ ] Run `go test ./services ./models` and commit discovery separately.

### Task 3: Project locale settings and user preferences API

**Files:**
- Modify: `controllers/projectController.go`
- Modify: `routes/projectRoutes.go`
- Create: `controllers/localizationController.go`
- Create: `controllers/localization_controller_test.go`
- Create: `routes/localizationRoutes.go`
- Modify: `main.go`

**Interfaces:**
- Produces `PATCH /api/v1/tenant/projects/:id/locales`.
- Produces `GET|PUT /api/v1/t/:tenantSlug/p/:projectSlug/localization/preference`.
- Locale settings response contains `sourceLocale`, `defaultLocale`, and `enabledLocales`.

- [ ] Validate canonical BCP 47 tags, reject duplicates, prevent removing source/default locales, and require at least one enabled locale.
- [ ] Make locale-setting updates atomic. Accept `generateWithAI` as an explicit request flag and enqueue a `translate_locale` job only when it is true; return the discoverable item count so the client can show the scope before confirmation.
- [ ] Implement authenticated project-scoped preference read/write and reject preferences for disabled locales.
- [ ] Reuse existing tenant/project context and authorization middleware instead of adding localization-specific roles.
- [ ] Test malformed locales, authorization, cross-project access, removal constraints, idempotent updates, opt-in job creation, and enabling a locale without AI.
- [ ] Run controller and route tests, then commit API configuration independently.

### Task 4: Translation management and runtime bundle API

**Files:**
- Create: `services/localization_service.go`
- Create: `services/localization_service_test.go`
- Modify: `controllers/localizationController.go`
- Modify: `routes/localizationRoutes.go`

**Interfaces:**
- Produces `GET /api/v1/tenant/projects/:id/translations?locale=&status=&search=` for management.
- Produces `PATCH /api/v1/tenant/projects/:id/translations/:locale/:key` for manual edits.
- Produces `GET /api/v1/t/:tenantSlug/p/:projectSlug/localization/bundle/:locale` for runtime.

- [ ] Return management rows with source text, translated text, origin, status, resource type, and update audit data.
- [ ] On manual edit, set `origin=manual`, recompute the current source hash, set `status=current`, and invalidate the locale bundle cache.
- [ ] Build compact runtime bundles shaped as `{ locale, sourceLocale, version, direction, messages: Record<string,string> }`; derive `direction` server-side (`ltr|rtl`) and fall back per key to source text when translation is absent or outdated.
- [ ] Use one Redis key per tenant/project/locale (`localization:<tenantId>:<projectId>:<locale>`), delete it after relevant mutations, and increment `localizationVersion` on the project. Do not put the version in the Redis key.
- [ ] Expose an ETag/version so `react-template` can avoid downloading unchanged bundles.
- [ ] Test manual protection, missing keys, outdated fallback, cache invalidation, disabled locales, and project isolation.
- [ ] Run service/controller tests and commit runtime delivery separately.

### Task 5: AI provider and reliable background translation jobs

**Files:**
- Create: `services/translation_provider.go`
- Create: `services/openai_translation_provider.go`
- Create: `services/openai_translation_provider_test.go`
- Create: `services/localization_worker.go`
- Create: `services/localization_worker_test.go`
- Modify: `configs/setup.go`
- Modify: `main.go`

**Interfaces:**
- Consumes required `OPENAI_API_KEY` and `OPENAI_TRANSLATION_MODEL` when AI translation is enabled; there is no default model.
- Produces `TranslateBatch(ctx, sourceLocale, targetLocale string, items []TranslationInput) ([]TranslationOutput, error)`.

- [ ] Implement the provider behind an interface and use the Responses API with a strict structured-output schema returning each input key exactly once.
- [ ] Add `resourceType`, `propertyPath`, and a minimal `context` field to each AI input. Derive context from resource type, parent page, field type, nearby label, option group, or validation purpose without sending ordinary entity records or unrelated project data.
- [ ] Prompt the model to preserve placeholders, interpolation tokens, HTML boundaries, product names, abbreviations, and option values while translating only supplied display text.
- [ ] Batch by item count and estimated prompt size; never put a whole large project into one request.
- [ ] Implement a MongoDB-leased worker safe for multiple backend replicas, with bounded retries, exponential backoff, cancellation, progress counters, and partial-failure summaries.
- [ ] Before every AI upsert, re-read the entry and skip it when `origin=manual`; this protects edits made while a job is running.
- [ ] Treat identical retries idempotently using job ID, locale, translation key, and source hash.
- [ ] Test with a fake provider for retry, lease recovery, cancellation, source edits during a job, manual edits during a job, and malformed AI responses.
- [ ] Keep the provider disabled with a clear configuration error until both the API key and a deployment-verified model ID are supplied; optionally validate model availability at startup or through an administrative health check. Run all tests without a live OpenAI request.
- [ ] Run backend tests and commit the AI boundary independently.

### Task 6: Regeneration and job-control API

**Files:**
- Modify: `controllers/localizationController.go`
- Modify: `routes/localizationRoutes.go`
- Modify: `services/localization_service.go`
- Test: `controllers/localization_controller_test.go`

**Interfaces:**
- Produces `POST /api/v1/tenant/projects/:id/translations/jobs` with scopes `key`, `locale`, `outdated`, or `project`.
- Produces `GET /api/v1/tenant/projects/:id/translations/jobs/:jobId` and `POST .../:jobId/cancel`.

- [ ] Add explicit regeneration requests with target locales and `manualPolicy=preserve` as the mandatory default.
- [ ] Require `manualPolicy=replace` plus explicit confirmation only for a single selected manual entry; disallow bulk manual replacement.
- [ ] Return job totals, completed, skipped-manual, failed, current key/locale, and a bounded error list.
- [ ] Deduplicate equivalent active jobs and make cancellation cooperative between batches.
- [ ] Test all scopes, deduplication, protected manual entries, status polling, and cancellation.
- [ ] Run controller/service tests and commit job control separately.

### Task 7: `tenantPanel` locale and translation management

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/project.ts`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/localization.ts`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/localization.test.ts`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/localization/ProjectLocaleSettings.tsx`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/localization/TranslationEditor.tsx`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/localization/TranslationJobProgress.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/pages/ProjectManagementPage.tsx`

**Interfaces:**
- Consumes Tasks 3, 4, and 6 APIs.
- Produces project settings, searchable translation review, manual editing, and job progress UI.

- [ ] Extend TypeScript project types with source, default, and enabled locale fields.
- [ ] Add settings controls that prevent invalid source/default removal. The add-language confirmation shows the translatable item count and a “Generate all translations with AI” checkbox selected by default; clearing it enables manual translation without creating a job.
- [ ] Add a translation grid filtered by language, resource type, status, and search text; show AI/manual badges and outdated warnings.
- [ ] Add inline manual editing with optimistic updates and invalidation of translation queries.
- [ ] Add actions for one entry, one language, all outdated entries, and the whole project; preserve manual translations by default.
- [ ] Poll active job status with TanStack Query and display completed, skipped, failed, and cancelled counts.
- [ ] Add component/API tests for validation, language addition, manual edits, protected regeneration, and progress display.
- [ ] Run `yarn test` and `yarn build` in `tenantPanel`, then commit frontend management independently.

### Task 8: `react-template` runtime locale selection and metadata overlay

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/i18n.ts`
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/api/localization.ts`
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/context/Localization.context.tsx`
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/hooks/useLocalizedProjectMetadata.ts`
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/components/header/LanguageSelector.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/components/header/Header.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/App.tsx`

**Interfaces:**
- Consumes locale settings, preference, and runtime bundle endpoints.
- Produces `translateMetadata(key, sourceText)` and the navbar selector.

- [ ] Resolve startup locale in order: backend project preference, project-scoped local-storage preference, enabled browser locale on first visit, project default, then project source locale. Use `en` only as a legacy fallback when locale configuration is absent.
- [ ] Register static UI catalogs with `react-i18next`; replace English-sentence keys with stable namespaced keys as screens are migrated.
- [ ] Load the dynamic bundle before rendering project metadata, then overlay translated presentation fields without mutating the cached source API objects.
- [ ] Add the navbar language selector using only enabled project locales; switch immediately, cache locally, persist when authenticated, and update document `lang` and `dir`.
- [ ] Use `Intl` or locale-aware date/number utilities for formatting; do not translate stored values.
- [ ] Fall back safely to source metadata when a bundle request fails, with no blank labels.
- [ ] Test resolution precedence, per-project isolation, browser fallback, persistence, bundle failure, metadata overlay, and language switching.
- [ ] Run `yarn test` and `yarn build` in `react-template`, then commit runtime localization independently.

### Task 9: Static UI catalogs and stable API errors

**Files:**
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/locales/en/*.json`
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/locales/en/*.json`
- Modify: `utils/helpers.go`
- Modify: relevant backend error response call sites incrementally
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/axiosClient.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/api/errorMessage.ts`

**Interfaces:**
- Backend errors become `{ code, params, message }`, retaining English `message` during migration.
- Frontends translate `errors.<code>` and interpolate `params`, falling back to `message`.

- [ ] Split static catalogs into focused namespaces such as `common`, `auth`, `projects`, `pages`, `tables`, and `errors`.
- [ ] Migrate visible English literals and sentence-based `t()` calls in bounded screen groups; configure missing-key warnings in development only.
- [ ] Introduce stable backend error codes without breaking current clients, then migrate call sites incrementally.
- [ ] Add CI checks that parse locale JSON, verify English keys exist, detect placeholder mismatches, and report untranslated literals without initially blocking unrelated work.
- [ ] Test error-code translation and fallback behavior in both frontends.
- [ ] Run all frontend tests/builds and backend tests, then commit catalog/error migration independently.

### Task 10: Migration, observability, and rollout

**Files:**
- Create: `cmd/localization-migrate/main.go`
- Create: `cmd/localization-migrate/main_test.go`
- Modify: deployment environment documentation
- Modify: observability configuration used by background workers

**Interfaces:**
- Produces an idempotent migration command with `--dry-run`, tenant/project filters, and summary output.

- [ ] Backfill existing legacy projects with `en` source/default/enabled settings without changing existing labels; configured projects always use their own source locale as the final fallback.
- [ ] Discover and seed source keys without calling OpenAI; AI jobs start only after an explicit confirmed generation or regeneration request.
- [ ] Add a scheduled purge for translations orphaned for at least 90 days, with a dry-run mode and metrics; never purge active or newly rediscovered keys.
- [ ] Emit structured metrics/logs for queued jobs, duration, translated/skipped/failed counts, retry counts, provider latency, and estimated token usage; never log the API key or full sensitive metadata payloads.
- [ ] Roll out backend schema/API first, `tenantPanel` management second, and `react-template` runtime last so every deployment remains backward compatible.
- [ ] Pilot one project and two locales, manually verify source edits and protected translations, then enable additional projects.
- [ ] Run `go test ./...`, both frontend test suites, and both frontend production builds before release.

## Acceptance Scenarios

- Adding Turkish with “Generate all translations with AI” confirmed creates one job and translates all configured presentation metadata; clearing the option enables Turkish with source-text fallbacks and no AI cost.
- A user selecting Turkish in Project A returns to Turkish on another authenticated device, while Project B retains its independent preference.
- An unauthenticated first visit uses an enabled browser locale and remembers the selection locally per project.
- Changing `Customer` to `Client` marks every corresponding translation outdated.
- “Regenerate all languages” refreshes AI translations but preserves a manually edited German value and reports it as skipped/outdated.
- A Turkish-source project falls back to Turkish, not English, when a requested translation is missing.
- Runtime bundle or OpenAI failures never remove source labels or prevent the project from opening.
- Entity rows and lookup record values are unchanged by every localization operation.

## Recommended Delivery Order

1. Release 1 — Tasks 1–4 plus the runtime subset of Task 8: manual localization, write-time reconciliation, bundles, source fallback, and language selection.
2. Release 2 — Task 7 plus preference synchronization from Task 8: project settings, translation editor, missing/outdated filtering, and saved per-project preferences.
3. Release 3 — Tasks 5–6 plus AI progress UI from Task 7: explicit AI generation, worker leasing/retries, manual protection, usage metrics, and regeneration.
4. Release 4 — Tasks 9–10: full static UI catalogs, stable API errors, migration tooling, additional locales, RTL testing, and controlled rollout.
