# Shared Project Localization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make AutoTable the shared localization service used by Tenant Panel, Ozmishow admin, and the public Ozmishow runtime.

**Architecture:** The Go backend owns project locale settings, target-language records, reconciliation, AI jobs, and versioning. Tenant Panel keeps its tenant-scoped administration surface; Ozmishow uses project-scoped APIs and adapts centralized entries into its existing resource dictionaries at runtime. Ozmishow source dictionaries remain canonical, while target dictionaries migrate to AutoTable with a temporary read-only fallback.

**Tech Stack:** Go 1.x, Fiber, MongoDB; React 18/Vite/TanStack Query/Vitest in Tenant Panel; Next.js 15/React 19/TanStack Query/Vitest in Ozmishow.

**Spec:** `docs/superpowers/specs/2026-10-13-shared-project-localization-design.md`

## Global Constraints

- Repositories: `/Users/osmansamilerdogan/Desktop/autotable-Go`, `/Users/osmansamilerdogan/Desktop/tenantPanel`, and `/Users/osmansamilerdogan/Desktop/ozmishow-nextjs`.
- Preserve all unrelated dirty-worktree changes. Inspect diffs before every edit and do not overwrite user work.
- Do not stage or commit any change. Replace commit steps with explicit review checkpoints.
- AutoTable is authoritative for project locale settings and target-language translations.
- Ozmishow resource dictionaries remain authoritative only for source-language text after cutover.
- OpenAI credentials and calls exist only in the Go backend.
- AI generation never overwrites a non-empty manual translation.
- Public responses include only active, current, non-empty translations; editors can see all lifecycle states.
- Changing the source locale of a populated Ozmishow project is rejected in this implementation.
- Keep Ozmishow's existing stable text-field descriptors and component structure; never infer localization from arbitrary string properties.
- Existing embedded target dictionaries are retained as temporary read fallback data and are not deleted.

## Review Focus

- Canonical keys containing spaces, colons, slashes, Unicode, or percent escapes must survive storage and URL round trips without selecting a different entry; Task 2 and Task 4 tests pin this behavior.
- Missing or duplicate Ozmishow resource IDs must be reported and skipped without collapsing two resources into one translation identity; Task 2 tests pin this behavior.
- Disabling and later re-enabling a target locale must retain matching translations while preventing the disabled locale from appearing at runtime; Task 4 tests pin this behavior.
- A manual edit racing with AI persistence must win, and later generated batches must count it as skipped; Task 1 and Task 3 tests pin this behavior.
- When AutoTable localization reads fail, Ozmishow must render source text and must not silently resume writing embedded target dictionaries; Task 7 and Task 8 tests pin this behavior.

---

### Task 1: Translation lifecycle and repository primitives

**Files:**
- Modify: `autotable-Go/models/localizationModel.go`
- Modify: `autotable-Go/models/localizationModel_test.go`
- Modify: `autotable-Go/repositories/localization_repository.go`
- Modify: `autotable-Go/repositories/localization_repository_test.go`
- Create: `autotable-Go/repositories/project_localization_repository.go`
- Create: `autotable-Go/repositories/project_localization_repository_test.go`

**Interfaces:**
- Produces: `TranslationOriginUntranslated`, `TranslationStatusMissing`, `TranslationJob.SourceLocale string`, and optional `TranslationJob.TranslationKeys []string`.
- Produces: `(*LocalizationRepository).UpsertPlaceholder(ctx, entry) error`.
- Produces: `(*LocalizationRepository).ApplyReconciledEntries(ctx, entries) error`.
- Produces: `(*LocalizationRepository).UpsertGeneratedTranslation(ctx, entry) (stored bool, err error)`, where `stored=false` means an existing manual entry won.
- Produces: `(*LocalizationRepository).GetJob(ctx, tenantID, projectID, jobID) (*models.TranslationJob, error)`.
- Produces: `ProjectLocalizationRepository` methods `GetProject`, `IncrementVersion`, and `UpdateLocaleSettings` scoped by tenant and project IDs.

- [ ] **Step 1: Write failing model and repository tests**

Add tests for serialization of the new lifecycle values, placeholder upsert semantics, reconciliation persistence, project-scoped job lookup, and project localization-version increments. Extend the generated-upsert race test to assert that a duplicate insert after a manual write is treated as a skipped manual entry rather than an overwrite.

- [ ] **Step 2: Run the focused tests and verify failure**

Run: `GOCACHE=/private/tmp/autotable-localization-plan go test ./models ./repositories`

Expected: FAIL because the new lifecycle constants and repository methods do not exist.

- [ ] **Step 3: Implement the model and repository primitives**

Keep the existing unique identity `(tenantId, projectId, locale, translationKey)`. Placeholder writes set empty text, untranslated origin, missing status, active ownership metadata, and source hash without replacing non-empty manual text. Project version updates must filter by both tenant and project ID.

- [ ] **Step 4: Run the focused tests**

Run: `GOCACHE=/private/tmp/autotable-localization-plan go test ./models ./repositories`

Expected: PASS.

- [ ] **Step 5: Review checkpoint**

Run `gofmt` on touched Go files, run `git diff --check`, and inspect only the Task 1 diff. Do not stage or commit.

### Task 2: Ozmishow discovery and source reconciliation

**Files:**
- Create: `autotable-Go/services/ozmishow_localization_discovery.go`
- Create: `autotable-Go/services/ozmishow_localization_discovery_test.go`
- Create: `autotable-Go/services/localization_reconciliation.go`
- Create: `autotable-Go/services/localization_reconciliation_test.go`
- Modify: `autotable-Go/services/localization_discovery.go`
- Modify: `autotable-Go/services/localization_discovery_test.go`
- Modify: `autotable-Go/services/localization_job.go`

**Interfaces:**
- Consumes: Task 1 placeholder and reconciliation repository methods.
- Produces: `CanonicalOzmishowTranslationKey(resourceType, resourceID, localKey string) string`.
- Produces: `DiscoverOzmishowRecordStrings(resourceType string, record bson.M, sourceLocale string) ([]models.SourceString, error)`.
- Produces: `DiscoverAllProjectStrings(ctx, tenantID, projectID primitive.ObjectID, sourceLocale string) ([]models.SourceString, error)` covering legacy and Ozmishow resources.
- Produces: `ReconcileProjectTranslations(ctx, tenantID, projectID primitive.ObjectID, sourceLocale string, enabledLocales []string) (models.TranslationReconciliationSummary, error)`.
- Produces: `HasOzmishowLocalizedContent(ctx, tenantID, projectID primitive.ObjectID) (bool, error)` for the source-locale change guard.

- [ ] **Step 1: Write failing pure discovery tests**

Cover `sitePages`, `visualBlocks`, and `reusableBlocks`; canonical key stability; local property-path preservation; Unicode and reserved characters; non-string dictionary values; missing IDs; duplicate IDs; absent collections; and ignoring strings outside `localization[sourceLocale]`.

- [ ] **Step 2: Run discovery tests and verify failure**

Run: `GOCACHE=/private/tmp/autotable-localization-plan go test ./services -run 'Test(DiscoverOzmishow|CanonicalOzmishow|ReconcileProject)'`

Expected: FAIL because Ozmishow discovery and reconciliation are undefined.

- [ ] **Step 3: Implement discovery**

Read project dynamic collections through `utils.GetDynamicCollectionForProject`. Treat namespace-not-found as empty. Reject records without a stable `_id` or `id`; report duplicates instead of deriving identity from array position or source text.

- [ ] **Step 4: Write failing reconciliation tests**

Assert placeholder creation for every enabled non-source locale, source-hash changes marking entries outdated, removed keys becoming inactive, rediscovered keys becoming active, disabled locale retention, and preservation of manual text.

- [ ] **Step 5: Implement reconciliation and integrate project discovery**

Move the legacy job's project-wide discovery behind `DiscoverAllProjectStrings`. Reconciliation must be idempotent and return deterministic counts for discovered, created, updated, outdated, inactive, and failed items.

- [ ] **Step 6: Run service tests**

Run: `GOCACHE=/private/tmp/autotable-localization-plan go test ./services`

Expected: PASS.

- [ ] **Step 7: Review checkpoint**

Run `gofmt`, `git diff --check`, and inspect the Task 2 diff. Do not stage or commit.

### Task 3: Durable filtered AI jobs and progress

**Files:**
- Modify: `autotable-Go/services/localization_job.go`
- Create: `autotable-Go/services/localization_job_test.go`
- Create: `autotable-Go/services/localization_worker.go`
- Create: `autotable-Go/services/localization_worker_test.go`
- Modify: `autotable-Go/services/openai_translation_provider.go`
- Modify: `autotable-Go/services/openai_translation_provider_test.go`
- Modify: `autotable-Go/repositories/localization_repository.go`
- Modify: `autotable-Go/main.go`

**Interfaces:**
- Consumes: Task 2 `ReconcileProjectTranslations` and `DiscoverAllProjectStrings`.
- Produces: `QueueProjectTranslationJob(tenantID, projectID, requestedBy primitive.ObjectID, sourceLocale string, targetLocales, translationKeys []string) (primitive.ObjectID, error)`.
- Produces: `ProcessProjectTranslationJob(ctx context.Context, job *models.TranslationJob) error`.
- Produces: `StartLocalizationWorker(ctx context.Context, owner string, pollInterval time.Duration)`.

- [ ] **Step 1: Write failing job-selection tests**

Assert project-wide jobs translate missing, outdated-AI, and failed entries; optional canonical keys restrict work; current manual entries are skipped and counted; disabled locales are rejected; protected-token failures affect only their entries; and successful persistence increments project localization version.

- [ ] **Step 2: Run job tests and verify failure**

Run: `GOCACHE=/private/tmp/autotable-localization-plan go test ./services -run 'Test(ProjectTranslationJob|LocalizationWorker|TranslateWithOpenAI)'`

Expected: FAIL on the new queue signature and worker behavior.

- [ ] **Step 3: Refactor job processing**

Reconcile first, read the persisted job source locale, filter deterministically by locale and optional key list, batch at 50 source strings, preserve manual entries through the repository result, update progress counts, emit structured tenant/project/job logs, bound error summaries, and bump the project version after persisted runtime changes.

- [ ] **Step 4: Implement lease-based recovery**

Start one cancellable worker from `main.go`. It leases pending or expired jobs and processes them through the same exported function. Queueing may wake the worker but must not rely solely on an untracked goroutine.

- [ ] **Step 5: Run service and repository tests**

Run: `GOCACHE=/private/tmp/autotable-localization-plan go test ./services ./repositories`

Expected: PASS.

- [ ] **Step 6: Review checkpoint**

Run `gofmt`, `git diff --check`, and inspect Task 3 for cancellation leaks and duplicate job processing. Do not stage or commit.

### Task 4: Tenant and project localization APIs

**Files:**
- Modify: `autotable-Go/controllers/localizationController.go`
- Modify: `autotable-Go/controllers/project_locales_test.go`
- Create: `autotable-Go/controllers/localization_controller_test.go`
- Modify: `autotable-Go/routes/projectRoutes.go`
- Modify: `autotable-Go/routes/localizationRoutes.go`
- Modify: `autotable-Go/routes/localizationRoutes_test.go`
- Modify: `autotable-Go/routes/routes_test.go`
- Modify: `autotable-Go/middlewares/tenantAuthenticate.go`
- Modify: `autotable-Go/middlewares/middlewares_test.go`

**Interfaces:**
- Consumes: Tasks 1-3 repository, reconciliation, queue, and job-status APIs.
- Produces tenant endpoints for job status and explicit generation.
- Produces project endpoints for authenticated editor reads, manual updates, reconciliation, generation, and job status.
- Project editor endpoints require project authentication. Project-role authorization is temporarily deferred by the user's 2026-10-14 follow-up.

- [ ] **Step 1: Write failing middleware and route-registration tests**

Assert public settings/runtime reads remain public; editor reads and every mutation require project authentication without a role gate; tenant endpoints retain tenant/project scope; static generation/job paths register before parameterized translation-key paths.

- [ ] **Step 2: Run route tests and verify failure**

Run: `GOCACHE=/private/tmp/autotable-localization-plan go test ./middlewares ./routes`

Expected: FAIL because the new middleware and routes are absent.

- [ ] **Step 3: Write failing controller contract tests**

Cover settings including `localizationVersion`; public filtering; editor lifecycle rows; locale validation; URL-escaped canonical keys; rejection of unknown keys without reconciled ownership metadata; create-or-update manual text after reconciliation; optional generation keys; project-scoped job lookup; source-locale change rejection on populated Ozmishow projects; disabled/re-enabled locale behavior; and version increments.

- [ ] **Step 4: Implement controllers and routes**

Use one service layer for tenant and project handlers. Keep runtime responses backward compatible while adding canonical identity and ownership fields needed by Ozmishow. Never accept tenant/project IDs from the body.

- [ ] **Step 5: Run focused backend tests**

Run: `GOCACHE=/private/tmp/autotable-localization-plan go test ./controllers ./middlewares ./routes ./services ./repositories ./models`

Expected: PASS.

- [ ] **Step 6: Review checkpoint**

Run `gofmt`, `git diff --check`, and inspect API error bodies for secret leakage. Do not stage or commit.

### Task 5: Tenant Panel job progress and richer translation states

**Files:**
- Modify: `tenantPanel/src/utils/api/localization.ts`
- Modify: `tenantPanel/src/utils/api/localization.test.ts`
- Modify: `tenantPanel/src/components/localization/ProjectLocalizationSection.tsx`
- Modify: `tenantPanel/src/components/localization/ProjectLocalizationSection.test.tsx`
- Modify: `tenantPanel/src/types/index.ts`

**Interfaces:**
- Consumes: Task 4 tenant locale, translation, generation, and job-status responses.
- Produces: `TranslationRow` with `missing | current | outdated | failed` status and `untranslated | ai | manual` origin.
- Produces: `TranslationJobResult`, `useTranslationJob`, and `useGenerateTranslations` hooks.

- [ ] **Step 1: Write failing API-hook tests**

Assert save settings returns `translationJobId`, job polling is enabled only for pending/running jobs, polling stops on completed/failed, explicit generation omits a key filter for project-wide generation, and translation queries do not poll when no job is active.

- [ ] **Step 2: Run API tests and verify failure**

Run: `yarn test src/utils/api/localization.test.ts`

Expected: FAIL on missing job APIs and response types.

- [ ] **Step 3: Implement localization API hooks**

Keep all calls under `/tenant/projects/:id`. Return normalized response data rather than raw Axios responses so the component can store the returned job ID.

- [ ] **Step 4: Write failing component tests**

Assert saving a new language shows progress, completion refreshes translations, failure shows bounded errors and retry, explicit regeneration starts a project-wide job, missing/outdated badges render, untranslated inputs remain editable, and disabled/re-enabled locale selection remains valid.

- [ ] **Step 5: Implement the Tenant Panel workflow**

Preserve the existing Languages and Translations tabs. Add job progress and a regenerate-missing/outdated action near the translation editor, distinguish no target language from not-yet-generated content, and keep manual save-on-blur behavior.

- [ ] **Step 6: Run Tenant Panel localization tests**

Run: `yarn test src/utils/api/localization.test.ts src/components/localization/ProjectLocalizationSection.test.tsx src/components/localization/localeSettings.test.ts`

Expected: PASS.

- [ ] **Step 7: Review checkpoint**

Run `yarn build`, record unrelated pre-existing failures separately, run `git diff --check`, and inspect only localization files. Do not stage or commit.

### Task 6: Ozmishow AutoTable localization client and settings overlay

**Files:**
- Create: `ozmishow-nextjs/data/project-localization.ts`
- Create: `ozmishow-nextjs/data/adapters/autotable-project-localization.ts`
- Create: `ozmishow-nextjs/data/server-project-localization.ts`
- Create: `ozmishow-nextjs/tests/data/autotable-project-localization.test.ts`
- Modify: `ozmishow-nextjs/data/server-site-repository.ts`
- Modify: `ozmishow-nextjs/tests/data/autotable-site-repository.test.ts`
- Modify: `ozmishow-nextjs/site/domain/types.ts`

**Interfaces:**
- Consumes: Task 4 public settings and runtime translation contracts.
- Produces: `ProjectLocaleSettings`, `RuntimeTranslationEntry`, `EditorTranslationEntry`, and `TranslationJob` TypeScript types.
- Produces: `createAutoTableProjectLocalizationClient(baseUrl, fetcher)` with `getSettings`, `getRuntimeTranslations`, `getEditorTranslations`, `updateTranslation`, `reconcile`, `generate`, and `getJob`.
- Produces: `mergeProjectLocaleSettings(config, settings): SiteConfig`.

- [ ] **Step 1: Write failing client contract tests**

Assert endpoint paths, URL encoding of canonical keys, response unwrapping, no-store reads, project bearer forwarding for authenticated operations, public reads without credentials, and normalized backend errors.

- [ ] **Step 2: Run client tests and verify failure**

Run: `npm test -- tests/data/autotable-project-localization.test.ts`

Expected: FAIL because the client does not exist.

- [ ] **Step 3: Implement the focused client and types**

Keep dynamic site transport unchanged. Localization endpoints are a separate concern and must not be represented as dynamic schemas.

- [ ] **Step 4: Write failing settings-overlay tests**

Assert AutoTable settings override `siteConfig` source/default/enabled locales, `localizationVersion` is retained, valid stored settings are used when the localization request fails, and invalid settings never corrupt an otherwise valid config.

- [ ] **Step 5: Integrate the settings overlay**

Wrap the server repository's `getSiteConfig` result with authoritative settings. Preserve current `SiteConfig` validation and treat stored locale fields only as availability fallback.

- [ ] **Step 6: Run data tests**

Run: `npm test -- tests/data/autotable-project-localization.test.ts tests/data/autotable-site-repository.test.ts`

Expected: PASS.

- [ ] **Step 7: Review checkpoint**

Run `npm run type-check` and record unrelated baseline diagnostics separately, then run `git diff --check`. Do not stage or commit.

### Task 7: Ozmishow admin centralized translation workflow

**Files:**
- Create: `ozmishow-nextjs/lib/admin/project-localization-backend.ts`
- Create: `ozmishow-nextjs/tests/admin/project-localization-backend.test.ts`
- Modify: `ozmishow-nextjs/lib/admin/localization-backend.ts`
- Modify: `ozmishow-nextjs/lib/admin/localized-containers.ts`
- Modify: `ozmishow-nextjs/app/admin/page-source.ts`
- Modify: `ozmishow-nextjs/app/admin/dashboard-source.ts`
- Modify: `ozmishow-nextjs/app/admin/localization/types.ts`
- Modify: `ozmishow-nextjs/app/admin/queries/containerQueries.ts`
- Modify: `ozmishow-nextjs/app/admin/localization/RegisteredBlockLocaleEditor.tsx`
- Create: `ozmishow-nextjs/app/api/admin/localization/generate/route.ts`
- Create: `ozmishow-nextjs/app/api/admin/localization/jobs/[id]/route.ts`
- Modify tests: `ozmishow-nextjs/tests/admin/localization-route.test.ts`, `tests/admin/localized-containers.test.ts`, `tests/admin/page-source.test.ts`, `tests/admin/dashboard-source.test.ts`, `tests/admin/locale-editor.test.tsx`, `tests/admin/admin-query-hooks.test.tsx`

**Interfaces:**
- Consumes: Task 6 client and Task 4 authenticated editor contracts.
- Produces: `overlayEditorTranslations(resource, entries, locale)` and local-key-to-canonical-key mapping on each `LocalizationTarget`.
- Produces admin helpers `loadProjectLocalizationSettings`, `loadEditorTranslations`, `saveProjectTranslations`, `reconcileProjectLocalization`, `generateProjectTranslations`, and `loadTranslationJob`.

- [ ] **Step 1: Write failing admin-backend tests**

Assert target-locale reads overlay centralized entries onto page, visual, and reusable resources; source-locale saves patch the resource then reconcile; target-locale saves call only AutoTable translation endpoints; target backend failure does not write embedded dictionaries; and canonical keys are taken from reconciled editor rows rather than reconstructed from source text.

- [ ] **Step 2: Run admin backend tests and verify failure**

Run: `npm test -- tests/admin/project-localization-backend.test.ts tests/admin/localization-route.test.ts tests/admin/localized-containers.test.ts`

Expected: FAIL on missing centralized helpers.

- [ ] **Step 3: Implement admin loading and saving**

Use authoritative project settings in dashboard and page loaders. Preserve existing source dictionary writes and optimistic resource versions. For a target locale, return the existing resource version unchanged because centralized translation edits do not mutate the resource.

- [ ] **Step 4: Write failing AI/job UI tests**

Assert the AI button sends the selected component's canonical keys, displays pending/running state, refetches localized blocks on completion, leaves results editable, displays bounded failure details, and never calls `/api/admin/localization/translate`.

- [ ] **Step 5: Implement generation proxies and editor behavior**

The Next routes only authenticate the Ozmishow admin session and proxy to AutoTable; they never read an OpenAI key. Poll only the returned job ID and invalidate localization queries on terminal status.

- [ ] **Step 6: Run focused Ozmishow admin tests**

Run: `npm test -- tests/admin/project-localization-backend.test.ts tests/admin/localization-route.test.ts tests/admin/localized-containers.test.ts tests/admin/page-source.test.ts tests/admin/dashboard-source.test.ts tests/admin/locale-editor.test.tsx tests/admin/admin-query-hooks.test.tsx tests/admin/page-editor.test.tsx`

Expected: PASS.

- [ ] **Step 7: Review checkpoint**

Run `git diff --check` and confirm a simulated AutoTable outage preserves source display while target saves fail visibly. Do not stage or commit.

### Task 8: Public Ozmishow translation overlay and cache versioning

**Files:**
- Create: `ozmishow-nextjs/site/localization/backend-overlay.ts`
- Create: `ozmishow-nextjs/tests/site/backend-localization-overlay.test.ts`
- Modify: `ozmishow-nextjs/site/localization/index.ts`
- Modify: `ozmishow-nextjs/app/[[...sitePath]]/page.tsx`
- Modify: `ozmishow-nextjs/app/sitemap.ts`
- Modify: `ozmishow-nextjs/app/robots.ts`
- Modify: `ozmishow-nextjs/lib/metadata.ts`
- Modify tests: `ozmishow-nextjs/tests/site/resource-localization.test.tsx`, `tests/site/site-page-renderer.test.tsx`, and `tests/site/localization-architecture.test.ts`

**Interfaces:**
- Consumes: Task 6 public localization client and runtime entry types.
- Produces: `applyBackendTranslations<T extends LocalizedResource>(resourceType, resource, locale, entries): T`.
- Produces: resource-grouping helpers keyed by resource type and stable resource ID.

- [ ] **Step 1: Write failing overlay tests**

Cover page, visual, and reusable ownership; property-path reconstruction; centralized value precedence; embedded target fallback only when no backend entry exists; source fallback for missing/outdated/failed entries; disabled locales; duplicate owners; and immutability of stored structures.

- [ ] **Step 2: Run overlay tests and verify failure**

Run: `npm test -- tests/site/backend-localization-overlay.test.ts tests/site/resource-localization.test.tsx`

Expected: FAIL because the backend overlay is absent.

- [ ] **Step 3: Implement resource overlay**

Apply translations to in-memory copies through the existing dictionary resolver. Do not alter shared structural values or persist public-read results.

- [ ] **Step 4: Write failing public-route tests**

Assert target pages, metadata, shared header/footer, and visual blocks use centralized text; AutoTable read failure renders source content; and `localizationVersion` participates in request-cache keys or invalidation tags.

- [ ] **Step 5: Integrate public loading**

Load settings before route resolution, resolve the requested locale, fetch only that locale's runtime entries, then overlay pages and referenced resources before metadata and rendering. Keep source rendering available when translation reads fail.

- [ ] **Step 6: Run public localization tests**

Run: `npm test -- tests/site/backend-localization-overlay.test.ts tests/site/resource-localization.test.tsx tests/site/site-page-renderer.test.tsx tests/site/localization-architecture.test.ts tests/site/localization.test.ts`

Expected: PASS.

- [ ] **Step 7: Review checkpoint**

Run `npm run type-check`, record unrelated baseline diagnostics separately, and run `git diff --check`. Do not stage or commit.

### Task 9: Existing Ozmishow translation backfill

**Files:**
- Create: `ozmishow-nextjs/scripts/migrate-autotable-localization.ts`
- Create: `ozmishow-nextjs/tests/migration/autotable-localization.test.ts`
- Modify: `ozmishow-nextjs/package.json`
- Modify: `ozmishow-nextjs/docs/autotable/ozmishow/unified-localization-migration.md`

**Interfaces:**
- Consumes: Task 4 project reconciliation and translation update endpoints.
- Produces CLI command `npm run migrate:autotable-localization -- --mode=dry-run|apply ...`.
- Produces deterministic `migration-report.json` and, in apply mode, `apply-report.json` plus raw backups.

- [ ] **Step 1: Write failing migration planner tests**

Assert canonical key planning, existing target values classified as manual, blanks classified missing, backend manual values winning, duplicate/missing IDs reported, deterministic checksums, idempotent reruns, and no network writes in dry-run mode.

- [ ] **Step 2: Run migration tests and verify failure**

Run: `npm test -- tests/migration/autotable-localization.test.ts`

Expected: FAIL because the migration command is absent.

- [ ] **Step 3: Implement dry-run planning**

Reuse the existing unified-localization export parsing where possible. Require explicit input and output paths; never infer a broad directory. The report must account for every localized resource and dictionary entry.

- [ ] **Step 4: Implement guarded apply mode**

Require `AUTOTABLE_ENDPOINT`, `AUTOTABLE_PROJECT`, and `AUTOTABLE_TOKEN`; write backups before the first mutation; reconcile first; URL-encode every canonical key; stop on the first rejected write; and never delete embedded dictionaries.

- [ ] **Step 5: Run migration tests and a fixture dry run**

Run: `npm test -- tests/migration/autotable-localization.test.ts`

Run: `npm run migrate:autotable-localization -- --mode=dry-run --site-pages=docs/autotable/ozmishow/migrated-records/sitePages.records.json --visual-blocks=docs/autotable/ozmishow/migrated-records/visualBlocks.records.json --reusable-blocks=docs/autotable/ozmishow/migrated-records/reusableBlocks.records.json --output=/private/tmp/ozmishow-autotable-localization-dry-run`

Expected: tests PASS and the dry run writes reports only under the explicit temporary output.

- [ ] **Step 6: Review checkpoint**

Inspect report counts and `git diff --check`. Do not run apply against live data, stage, or commit.

### Task 10: Remove duplicate Ozmishow OpenAI integration and update configuration docs

**Files:**
- Delete: `ozmishow-nextjs/lib/admin/ai-translation.ts`
- Delete: `ozmishow-nextjs/app/api/admin/localization/translate/route.ts`
- Delete: `ozmishow-nextjs/tests/admin/ai-translation.test.ts`
- Delete: `ozmishow-nextjs/tests/admin/ai-translation-route.test.ts`
- Modify: `ozmishow-nextjs/.env.example`
- Modify: `ozmishow-nextjs/README.md`
- Modify: `autotable-Go/.env.example` if present
- Modify: `autotable-Go/README.md`

**Interfaces:**
- Consumes: Tasks 3, 4, and 7 backend-only AI generation path.
- Produces: one documented location for `OPENAI_API_KEY` and `OPENAI_TRANSLATION_MODEL`: the Go backend environment.

- [ ] **Step 1: Write or update the architecture guard test**

Extend `ozmishow-nextjs/tests/site/localization-architecture.test.ts` to assert no Ozmishow production file reads `OPENAI_API_KEY`, imports the deleted helper, or calls `api.openai.com`.

- [ ] **Step 2: Run the guard test and verify failure**

Run: `npm test -- tests/site/localization-architecture.test.ts`

Expected: FAIL while the direct Next.js integration exists.

- [ ] **Step 3: Remove direct integration and update docs**

Document backend environment setup, Tenant Panel generation, Ozmishow proxy behavior, and the requirement to restart the Go service after changing environment variables. Remove Next-side OpenAI variables without exposing any local secret value.

- [ ] **Step 4: Run guard and focused localization suites**

Run: `npm test -- tests/site/localization-architecture.test.ts tests/admin/locale-editor.test.tsx tests/admin/localization-route.test.ts`

Expected: PASS.

- [ ] **Step 5: Review checkpoint**

Run `git diff --check` in both repositories and confirm no `.env.local` or secret was added. Do not stage or commit.

### Task 11: Cross-application acceptance and final verification

**Files:**
- Create or modify only the smallest existing integration-test files needed in each repository.
- Do not add production behavior in this task; return to the owning task if a failure exposes a missing implementation.

**Interfaces:**
- Consumes: all prior tasks.
- Produces: evidence that both editors and the public runtime use the same AutoTable translation entries.

- [ ] **Step 1: Add a contract fixture shared by the three test suites**

Represent one project with English source content, Turkish enabled, a page resource, a visual resource, a reusable resource, one AI translation, one manual translation, one missing translation, and one outdated translation. Keep equivalent JSON fixtures in the repository-local test directories rather than adding runtime coupling between repositories.

- [ ] **Step 2: Run complete focused suites**

Run in `autotable-Go`: `GOCACHE=/private/tmp/autotable-localization-plan go test ./models ./repositories ./services ./controllers ./middlewares ./routes`

Run in `tenantPanel`: `yarn test src/utils/api/localization.test.ts src/components/localization/ProjectLocalizationSection.test.tsx src/components/localization/localeSettings.test.ts`

Run in `ozmishow-nextjs`: `npm test -- tests/data/autotable-project-localization.test.ts tests/admin/project-localization-backend.test.ts tests/admin/localization-route.test.ts tests/admin/localized-containers.test.ts tests/admin/locale-editor.test.tsx tests/site/backend-localization-overlay.test.ts tests/site/resource-localization.test.tsx tests/site/site-page-renderer.test.tsx tests/site/localization-architecture.test.ts tests/migration/autotable-localization.test.ts`

Expected: all focused suites PASS.

- [ ] **Step 3: Run repository-wide verification**

Run in `autotable-Go`: `GOCACHE=/private/tmp/autotable-localization-plan go test ./...`

Run in `tenantPanel`: `yarn test` and `yarn build`.

Run in `ozmishow-nextjs`: `npm test` and `npm run type-check`.

Expected: no new failure attributable to localization. Record pre-existing unrelated failures with command output rather than describing a full suite as passing.

- [ ] **Step 4: Execute the manual acceptance flow against a non-production project**

Enable Turkish in Tenant Panel, generate, inspect job completion, edit one value in each editor, verify cross-editor parity, verify the public Turkish route, change one English source value, verify outdated/source fallback, regenerate, and verify the manual value is preserved.

- [ ] **Step 5: Final safety review**

In all three repositories run `git diff --check`, `git status --short`, and `git diff --cached --name-only`. Confirm there are no staged changes or commits, no secrets, no destructive migration output in the worktrees, and no unrelated user changes overwritten.

