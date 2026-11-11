# Central Page Requests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Configure page reads centrally and share one network request between every compatible consumer with identical effective inputs, while fetching differing inputs separately.

**Architecture:** Persist named request definitions and component connections in the page. Compile central defaults, resolved bindings and explicit local overrides into a transport descriptor; use the descriptor for both network execution and a common TanStack Query key. Consumer adapters select records, options or chart values from the shared response and never issue a second read.

**Tech Stack:** Existing Go/Fiber/MongoDB backend; React 18, TypeScript, Axios, TanStack Query 5 and Vitest in both frontend repositories. No new dependency.

**Spec:** `docs/superpowers/specs/2026-11-06-page-requests-design.md` in `autotable-Go` (approved 2026-11-06).

## Global Constraints

- No new third-party dependency, database collection or dataset endpoint is required.
- Existing unrelated working-tree edits must be preserved.
- Request IDs and component IDs do not distinguish otherwise identical reads within the same page.
- Unmounting one subscriber must not cancel a request still needed by others.
- Create/update/delete, form submissions, action workflows and any execution that changes data keep their existing mutation behavior.
- Legacy pages without `requests` continue to work without manual migration.
- Metadata/session prerequisites remain their own shared queries and do not count as a duplicate dataset read.
- Standalone editor/sample previews continue to use sample data and do not execute runtime requests.
- Whole-page read coverage, persistence and backwards compatibility are required outcomes, not optional later phases.

## Review Focus

1. Typed values and transport normalization: `false`, `0`, repeated values, ordered arrays, empty controls and equivalent ascending-sort encodings must not be merged or split incorrectly (Tasks 1–3).
2. React subscriptions: a staggered mount, one subscriber unmounting, filter divergence/rejoining and repeated invalidations must maintain correct network counts (Tasks 2, 4, 5, 8).
3. Data scope changes: an unresolved identity, a late former-project response or an authorization failure must not show another scope's cached rows (Tasks 2, 8).
4. Less visible reads: generated relation columns, table lookups, action/form options and grouped tabs must use the same machinery without accidentally running both new and old hooks (Tasks 4, 5, 7).
5. Whole-page preservation: nested tabs/subpages, copy operations and a visual save during page editing must retain definitions/connections; clearing the final unused request must persist (Tasks 1, 6–8).

## Repository and execution boundaries

- Backend root: `/Users/osmansamilerdogan/Desktop/autotable-Go` (`B` below).
- Runtime root: `/Users/osmansamilerdogan/Desktop/react-template` (`R`).
- Authoring root: `/Users/osmansamilerdogan/Desktop/tenantPanel` (`T`).
- All three currently have substantial pre-existing edits. At execution, record baseline diffs and run the worktree skill's environment detection before deciding how to isolate work. A clean checkout of HEAD alone would omit runtime/visual features this change depends on; preserve the working baseline. Never stash, reset or include unrelated edits in task commits.
- Each task follows red test → confirmed failure → implementation → confirmed pass. Commit only task-owned changes when they can be staged without absorbing existing edits; otherwise retain the verified changes in place and report that boundary.
- The recommended execution method is native in this session, followed by an independent final review, because the model contract, descriptor and consumers are sequentially dependent.

## Shared interfaces fixed by this plan

Create equivalent TypeScript contracts in `R/src/types/pageRequests.ts` and `T/src/types/pageRequests.ts`, importing `ParameterBinding` as a type from the existing page contract. Re-export them through each existing page type/API module. Mirror persisted fields in `B/models/page_requests.go` using JSON/BSON names below.

- `PageRequestSource`: discriminated by `transport: "table-source" | "list" | "page" | "pipeline" | "workflow" | "selection"`. Every source has `schemaName`; table-source also has `kind: "schema" | "pipeline" | "workflow"` and the corresponding pipeline/workflow name; direct pipeline/workflow transports require their respective names. Selection sources have `fieldName`, optional `valueField` and `dataFields`. Sources are fixed by the definition, not editable runtime parameters.
- `PageRequestControls`: optional `page: number`, `limit: number`, `sort: string | null`, `asc: boolean`, `search: string | null`. Validate page/limit as positive integers and preserve existing transport-specific limits. Only controls supported by that transport may be configured.
- `PageRequestValues`: optional `params: Record<string, JsonValue>`, `filters: Record<string, JsonValue>`, `controls: PageRequestControls`, `fields: string[]`. Reserved source/transport names cannot be passed through params or filters.
- `PageRequestParameterBindings`: optional `params` and `filters` maps of existing `ParameterBinding`, and optional `controls` map of supported control names to `ParameterBinding`. Reuse existing supported parameter sources; do not introduce new page-variable/system binding support as a side effect.
- `PageRequestDefinition`: `{ id: string; name: string; source: PageRequestSource; defaults?: PageRequestValues; bindings?: PageRequestParameterBindings; result: PageRequestResult }`.
- `PageRequestResult`: `{ kind: "records" | "value"; path?: string[]; totalPath?: string[] }`. Paths are safe own-property selectors relative to the transport's decoded payload. They do not alter the fetch signature. The transport fixes its response envelope; record consumers require arrays of records, and paginated consumers require a usable total. Validate incompatible connections rather than silently guessing.
- `PageRequestConnection`: `{ requestId: string; overrides?: PageRequestValues; result?: PageRequestResult }`. Per-property absence means inherit. An explicit null filter/sort/search clears that value; it is omitted from transports that cannot represent it. Page/limit/asc cannot be null. Removing an override restores inheritance. Arrays remain ordered unless an endpoint explicitly defines a set.
- Add `requests?: PageRequestDefinition[]` to PageModel/create/update contracts and `requestBindings?: Record<string, PageRequestConnection>` to ComponentBlock.
- Consumer slot names: `data`, `rows`, `columns`, `groupOptions`, `options:<formKey>`, `filterOptions:<filterId-or-formKey>`, `lookup:<lookup-key>`, `relationOptions:<groupId>`, `actionOptions:<actionId>:<formKey>`, and `collection:<slot>`. Treat each suffix as an opaque identifier, never as a property path. Central `listComponentRequestSlots(component)` inventories slots using existing stable IDs/keys and supplies their required result shape to validation and editor selectors.
- Add optional `requestSlot: string` to existing visual repeater sources in both engine copies. The source's normal schema settings remain a backwards-compatible fallback only when no slot is authored. A configured missing slot is an error, never an invitation to run a fallback request. Placement bindings live in ComponentBlock under `collection:<slot>`; library documents retain only slot names.
- `PageRequestScope`: `{ tenant: string; project: string; identity: string; pageKey: string }`, from a verified session and runtime page identity. A page without a persisted ID receives one stable provider-local key. An unready scope cannot execute reads.
- `EffectivePageRequest`: `{ method: "GET" | "POST"; path: string; query: readonly [string, string][]; body?: JsonValue; headers: Record<string, string>; schemaName: string; transport: PageRequestSource["transport"]; envelope: string }`. It describes the outgoing request and expected transport envelope, independently of consumer presentation.
- `PageRequestSnapshot`: discriminated pending/error/success state, decoded `data`, `error`, actual `queryKey` and `refetch`. Expose shared transport snapshots; select presentation with pure adapters.

### Task 1: Persist and validate definitions and connections

**Files:** Create `B/models/page_requests.go`, `B/models/page_requests_validation.go`, `B/models/page_requests_test.go`, `R/src/types/pageRequests.ts`, `T/src/types/pageRequests.ts`. Modify `B/models/pageModel.go`, `B/models/page_runtime_validation.go`, `B/controllers/pageController.go`, `B/controllers/page_update_document_test.go`, `R/src/types/page.ts`, `T/src/types/page.ts`, `T/src/utils/api/page.ts`. Add `T/src/utils/pageRequests.ts` and its `.test.ts`.

**Interfaces:** Produces the persisted contracts above, `listComponentRequestSlots(component)` and `validatePageRequests(page): PageBindingIssue[]` in Tenant Panel, and `validatePageRequests(page *PageModel, graph pageRuntimeGraph) error` in Go.

- [ ] Write Go round-trip tests for every transport, bindings, explicit false/zero/null, connections in nested components, and JSON→BSON→JSON preservation. Extend update-document tests so `requests: []` clears stored requests while an omitted field remains omitted.
- [ ] Write editor validation tests for duplicate/missing IDs, missing source names, reserved parameter collisions, unsafe result paths, unknown slots, incompatible results and referenced-request deletion. Explicitly verify a legacy page without new fields passes unchanged.
- [ ] Run `go test ./models ./controllers -run 'PageRequest|PageUpdateDocument'` in B and `yarn vitest run src/utils/pageRequests.test.ts` in T; confirm failures identify missing contracts/behavior.
- [ ] Implement the shared model and focused validators. Include request-only pages in runtime-validation detection; handle explicit empty requests in `pageUpdateDocument` like empty filters. Keep result selectors out of backend query execution. Do not tighten unrelated legacy validation.
- [ ] Run both targeted commands and `yarn tsc --noEmit` in R and T. Fix new type failures, record any pre-existing failures, then stage only task-owned changes for `feat: persist central page requests`.

### Task 2: Canonical descriptors, scoped transport and shared cache

**Files:** Create `R/src/pageRuntime/requests/descriptor.ts`, `transport.ts`, `query.ts`, `PageRequestScope.tsx`, and corresponding `.test.ts` files including `query.mounted.test.ts`. Modify `R/src/utils/dynamicQueryKeys.ts`, `R/src/pageRuntime/PageRuntimeProvider.tsx`, `R/src/visualBuilder/data/useVisualCollections.ts`.

**Interfaces:** Produces `compilePageRequest(source, values): EffectivePageRequest`, `pageRequestQueryOptions(scope, descriptor, enabled)`, `fetchPageRequest(scope, descriptor, signal): Promise<unknown>`, `usePageRequestScope()` and `usePageRequest(descriptor | undefined, enabled): PageRequestSnapshot`. Query key is `["page-request", tenant, project, identity, pageKey, signature]`; meta carries schema/source scope for invalidation.

- [ ] Test equal filter-object insertion orders, ascending `true`/`1` inputs from legacy adapters, and repeated-value preservation against the actual serialized request. Pin differences in sort direction, page/limit, fields, source endpoint, typed JSON bodies and response-affecting headers. Request/component names and result selectors must not split otherwise equal requests.
- [ ] Add a mounted real QueryClient/Axios-adapter test: two identical descriptors cause one pending read; a later fresh subscriber adds none; unmounting one leaves the other read alive. Verify an unresolved scope sends nothing, project/user transitions hide old data, and late old responses never populate the new scope.
- [ ] Run `yarn vitest run src/pageRuntime/requests/descriptor.test.ts src/pageRuntime/requests/query.mounted.test.ts` in R and confirm failures.
- [ ] Implement descriptors by reusing the existing endpoint encoders and actual endpoint semantics. `GET /dynamic`, `/dynamic/page`, `/dynamic/table-source`, `/dynamic/pipeline`, `/dynamic/selection` and the existing workflow-data POST remain distinct. Normalize legacy ascending encodings using the backend's accepted boolean semantics. Execute from the exact descriptor used to key the cache. Keep JSON body types; preserve repeated query values and array order. Store a raw transport payload once and decode envelopes consistently.
- [ ] Extract the existing verified visual-session scope into the common scope provider without changing authentication meaning. Explicit runtimeScope, abort signals, 15-second read timeout, at most one transient retry and zero auth/config retries apply to shared page reads. Reuse BASE_QUERY_STALE_TIME and BASE_QUERY_GC_TIME. Standalone legacy consumers without a page provider retain their original operational context.
- [ ] Run descriptor, scope, transport and mounted tests; commit task-owned changes as `feat: share page reads by effective request`.

### Task 3: Resolve page bindings and detect request dependency cycles

**Files:** Create `R/src/pageRuntime/requests/resolver.ts`, `useResolvedPageRequest.ts`, `graph.ts` and tests; `T/src/utils/pageRequestGraph.ts` and tests. Modify `R/src/pageRuntime/pageBindingCompiler.ts`, `PageRuntimeProvider.tsx`, `types.ts`, `T/src/utils/pageBindings.ts`, `B/models/page_requests_validation.go` and its tests.

**Interfaces:** Produces `resolvePageRequest(definition, resolvedBindings, connection?, localOverrides?): PageRequestValues`, `useResolvedPageRequest(componentId, slot, localOverrides?)` returning ready/pending/error plus descriptor/result contract, and `validatePageRequestGraph(page)` in both frontend validation paths and equivalent Go validation. Extend parameter compilation with `compileRequestParameters(page, requestId, connection?)`; reuse resolver semantics rather than creating fake ComponentBlocks with colliding IDs.

- [ ] Test precedence: central defaults → central resolved bindings → saved connection overrides → actual local interaction overrides. Verify false/zero survive, null clears a filter/sort, deleting an override restores inheritance, and untouched default table controls do not accidentally override central defaults.
- [ ] Test waiting page filters/outputs, invalid referenced outputs, nested-page request lookup, duplicate request IDs and dependency cycles: request → consumer → its own output, and longer cycles across multiple components/requests. Use dependency nodes `request:<id>` and `component:<id>` internally and keep public IDs unchanged.
- [ ] Run the new resolver/graph tests in R/T and `go test ./models -run PageRequest`; confirm failures before implementation.
- [ ] Implement scoped request lookup, grouped parameter compilation for params/filters/controls, and subscription only to relevant runtime dependencies. Reuse existing supported ParameterBinding semantics; central bindings resolve once per definition/runtime snapshot. Connect consumer dependency edges even when it has no direct dataBinding parameters. Block incomplete/invalid resolutions before query creation.
- [ ] Run new tests plus existing pageBindingCompiler/pageParameterResolver/pageBindings tests; commit task-owned changes as `feat: resolve page request connections and dependencies`.

### Task 4: Share table and visual collection reads

**Files:** Modify `R/src/components/panelComponents/FormElements/GenericPaginatedPage.tsx`, `GenericUnpaginatedPage.tsx`, `GenericTabPage.tsx`, `R/src/components/DynamicPageSections.tsx`, `R/src/utils/dynamic.ts`, `R/src/visualBuilder/VisualBlock.tsx`, `R/src/visualBuilder/data/collectionPlan.ts`, `collectionQuery.ts`, `collectionFetch.ts`, `useVisualCollections.ts`, both copies of `src/visual-engine/model/nodes.ts` and `validation.ts`, and `T/src/visualBuilder/data/collectionPlan.ts`. Create `R/src/pageRuntime/requests/adapters.ts` and `R/src/pageRuntime/requests/tableVisual.mounted.test.ts`.

**Interfaces:** Produces pure `selectPageRequestResult(payload, result)`, `toDynamicPayload(...)`, and `toCollectionPage(...)`. Table components accept their owning component ID/connections; visual host resolves `collection:<slot>` before instantiating renderer IDs. Both consume Task 3's hook and Task 2's query options.

- [ ] Mount a real table and visual with matching source, fields, page, limit, filters and sorting. Assert one dataset call for simultaneous/staggered mounts, correct content in both, and no legacy fetch. Change filters then sorting on one consumer: assert two effective requests and distinct displayed results. Reset overrides and assert rejoining the fresh cached result adds no read.
- [ ] Cover a schema-only legacy table/repeater pair whose normalized descriptors match, differing projections/pages that remain separate, empty/malformed results, missing visual slot errors, multiple repeater placements and unchanged selected-record details/actions.
- [ ] Run `yarn vitest run src/pageRuntime/requests/tableVisual.mounted.test.ts` in R; confirm failure with the existing separate query paths.
- [ ] Route connected reads through the common snapshot and pure adapters, disabling their old hooks. Compile compatible legacy table-source/repeater reads into the same raw transport query. Preserve supported visual filtering metadata and local region states. A page request may supply records from any compatible existing read transport; invalid result shapes show errors. Carry actual pagination into visual captions rather than claiming every connected request is first-page data.
- [ ] Track table interaction overrides explicitly; untouched control initializers inherit request defaults. Expose a reset to shared defaults. Pass the actual shared query key to CRUD cache updates, replacing assumptions based on the old table-source key. Keep record selection/bulk actions based on the current visible result.
- [ ] Run new mounted tests and existing table/visual collection, selected-record, repeater and action tests; validate both engine copies accept/preserve requestSlot. Commit task-owned changes as `feat: connect tables and visuals to shared page reads`.

### Task 5: Connect all remaining page reads and invalidation

**Files:** Modify R's `src/components/charts/DynamicChart.tsx`, `src/components/RelationMatrix.tsx`, `src/components/forms/DynamicForm.tsx`, `src/components/forms/useFormSelectionData.ts`, `src/components/panelComponents/FormElements/InfoBlocks.tsx`, `DistributionBlocks.tsx`, `GenericPaginatedPage.tsx`, `GenericUnpaginatedPage.tsx`, `GenericAddEditPanel.tsx`, `src/components/DynamicPageSections.tsx`, `src/utils/tableFilters.tsx`, `tableLookupSelection.ts`, `generatedRelationSelection.ts`, `src/utils/dynamic.ts`, `dynamicQueryKeys.ts`, `src/hooks/useWebSocket.ts`, and `src/visualBuilder/recordActions.ts`. Create `src/pageRuntime/requests/readInventory.md`, `legacyAdapters.ts`, `invalidation.ts`, `consumers.mounted.test.ts`, `invalidation.mounted.test.ts`.

**Interfaces:** Legacy adapters convert each existing read to EffectivePageRequest; `pageRequestInvalidationPredicate(scope, schemaName)` matches new query metadata. Multi-read hooks accept optional owner/component connections and derive slots from Task 1's inventory. Standalone call signatures remain backwards compatible.

- [ ] Write a table-driven mounted test for chart/info/distribution consumers, relation-matrix rows/columns, grouped tabs, form/filter options, action options, lookup labels and generated relation options. For each compatible pair assert one matching read, different controls/projections separate, and a connected slot never also invokes its inline source. Cover pending/error/retry where the old data-only hook masked those states.
- [ ] Write an invalidation regression: two matching observers plus one differing-filter descriptor, no WebSocket notification on save; successful mutation refreshes each descriptor once and unrelated schemas/projects remain untouched. Repeat for socket events, one observer unmounting, and a late pre-save read. Two create/update actions must still make two mutation calls.
- [ ] Run new mounted tests to confirm failures.
- [ ] Inventory every page data read with `rg` for useQuery/useQueries/queryFn/get/axiosClient/fetch and record source→slot→adapter ownership in readInventory.md. Include selections inside table action forms and grouped-tab sources. Exclude document metadata, authentication, file download/export and explicit user mutation executions, with a concrete reason for each exclusion.
- [ ] Implement adapters and named-slot routing. A chart can select rows from a table-source result without rerunning its former pipeline read. A relation matrix can bind rows and columns independently. Normalize all compatible legacy page reads through the same descriptor/options factory; do not collapse semantically different transports merely to improve counts. Preserve legacy outside-page use.
- [ ] Update mutation/socket invalidation and cancellation to target new scoped metadata alongside legacy keys during compatibility. Update any direct cache writes to use actual active keys. Run the inventory tests plus existing workflow/chart/form/selection/matrix/action tests; commit task-owned changes as `feat: centralize remaining page data consumers`.

### Task 6: Author and save central page requests in Tenant Panel

**Files:** Create `T/src/components/PageDesigner/PageRequestsEditor.tsx`, `PageRequestSourceEditor.tsx`, `PageRequestValuesEditor.tsx`, their `.test.tsx` files, and `T/src/utils/pageRequestEditor.ts` with tests. Modify `T/src/components/panelComponents/common/PagesSection.tsx`, `PagesSection.designer.test.tsx`, `T/src/utils/pageBindings.ts`, `T/src/components/PageDesigner/ParameterBindingsEditor.tsx`, `T/src/utils/api/page.ts` and `T/src/utils/pageEditorMetadata.test.ts`.

**Interfaces:** `PageRequestsEditor({page, onChange, disabled})` edits definitions; the source/value editors accept typed contracts. Reuse ParameterBindingsEditor for each binding bag, with an owner that can be a request and an explicit output-exclusion set from the dependency graph. Never manufacture a real component ID to make its current API fit.

- [ ] Add user-interaction tests for creating a request, selecting a supported source, configuring filters/sort/fields/pagination and bound values, renaming it, listing consumers, blocking referenced deletion, and deleting the final unused definition. Unsupported transport controls and incompatible values must not save.
- [ ] Extend workspace persistence tests: requests participate in dirty tracking, failed saves retain drafts, requests are included in Save Page, metadata updates and reordering, and in-flight save completion does not overwrite a newer draft. Include nested pages and editing existing pages without requests.
- [ ] Run editor/persistence tests and confirm failures.
- [ ] Add a Page Requests tab to the existing page workspace, using current UI controls and form patterns. Expose human labels for existing read source types; keep cache-key/transport implementation details out of ordinary user flows. Provide filters/parameters/control bindings in distinct sections and typed inputs for common values. Surface central validation issues before dispatching saves.
- [ ] Audit all explicit page create/update/copy/reparent/order/JSON-editor payload builders and dirty snapshots. Preserve requests unless explicitly changed, including when editing a different workspace tab. Run new tests plus existing PagesSection/pageBindings/API persistence tests; commit task-owned changes as `feat: manage page requests in tenant panel`.

### Task 7: Author component and visual connections without leaking page references

**Files:** Create `T/src/components/PageDesigner/PageRequestConnectionEditor.tsx` and tests. Modify `T/src/components/PageDesigner/PageDesigner.tsx`, `FormComponentEditor.tsx`, `FormFieldEditor.tsx`, `ActionSettingsTabs.tsx`, `T/src/visualBuilder/VisualDataBuilder.tsx`, `VisualDataTools.tsx`, `PlacedVisualBuilderPage.tsx`, `pageAdapter.ts`, `pageRepository.ts`, `componentLibrary.ts`, `types.ts`, and related `.test.ts`/`.test.tsx` files. Add `T/src/visualBuilder/VisualRequestConnectionEditor.tsx` and tests.

**Interfaces:** `PageRequestConnectionEditor({page, component, slot, value, onChange, disabled})` filters compatible requests and edits overrides/result selectors. `VisualRequestConnectionEditor` manages repeater slots on the placement; page repository target updates carry an optional `requestBindings` change and verify its saved round trip. Existing record-connection editing remains independent.

- [ ] Test connecting every enumerated slot to a compatible request, showing current inherited controls, changing local overrides and clearing to inherit. Unknown/missing references stay visible as errors; they are never silently replaced by another request. Removing fields/actions/columns with connections must either remove their now-unused slot safely or show the dependency before saving.
- [ ] Test a placed repeater connection through save/reload, preserving unrelated requests/connections and concurrent-editor conflict behavior. Save the visual to the library and assert its document contains slot names but no page request IDs or fetched records; placing it on another page requires selecting that page's request. Clone operations that regenerate IDs must rewrite related slot identifiers and parameter references together.
- [ ] Run the new connection/editor/repository tests and confirm failures.
- [ ] Integrate the common connection editor into each data-capable component/slot with compatible result filtering. Do not require authors to edit JSON to connect options/lookups/matrix feeds. Add a page-request choice to repeater authoring and a placement-level slot mapping editor. Keep sample previews local and preserve the existing selected-record Data connection feature.
- [ ] Update placement persistence to merge connections against the latest page with explicit baseline conflict checks and post-save verification. Keep new page-level data out of reusable library payloads. Run new tests and existing designer persistence/pageAdapter/pageRepository/componentLibrary/VisualDataBuilder tests; commit task-owned changes as `feat: connect page components to named requests`.

### Task 8: Verify end to end, document behavior and review

**Files:** Add `R/scripts/verify-page-requests.js`, `T/scripts/verify-page-requests.js`, `R/docs/page-requests.md`, `T/docs/page-requests.md`, and `B/docs/verification/2026-11-06-page-requests.md`. Update this plan's task checkboxes and approved spec status when justified.

**Interfaces:** Isolated browser fixtures intercept metadata/session/dataset responses and count requests by effective descriptor. Follow the existing browser-fixture patterns; no live data writes. Use the available browser skill at execution and temporary build directories.

- [ ] Add browser fixtures that exercise central request creation, table/visual connection, independent filter/sort overrides, matching again, saving/reloading and a chart or info-block consumer. Assert one dataset call for equivalent reads, separate calls for different controls, correct displayed rows and no runtime data requests from sample editors. Include failure/retry and project transition.
- [ ] Run targeted browser checks against local apps, inspect screenshots, and record actual counts and limitations. Verify keyboard-accessible labels, modal/tab navigation and visible error states for the new editor controls.
- [ ] Run `go test ./models ./controllers ./routes` in B; `yarn test` and `yarn tsc --noEmit` in R/T; production builds with `yarn vite build --outDir /private/tmp/page-requests-runtime-dist` in R and `yarn vite build --outDir /private/tmp/page-requests-tenant-dist` in T. Run ESLint on changed TypeScript files using each repository's existing config. Do not rewrite tracked dist outputs or unrelated generated files.
- [ ] Document source/result compatibility, shared defaults versus local overrides, pagination/projection differences, reset-to-inherit, component/slot coverage and read-versus-mutation boundaries. Record exact executed checks, pre-existing failures and remaining limitations; do not claim a check passed without fresh output.
- [ ] Perform an independent final code review of task-owned changes for request identity, scope isolation, full consumer coverage, persistence and legacy regressions. Fix findings and rerun only relevant affected checks. Commit remaining task-owned documentation as `docs: explain central page requests` when safe, then report the user-visible outcome and verification evidence.

## Plan self-review

- Every approved design section maps to Tasks 1–8: model/persistence (1), key/scope/transport (2), bindings/dependencies (3), core sharing (4), whole-page reads/invalidation (5), central authoring (6), per-consumer/library connections (7), browser/build/regression verification (8).
- Defaults/override precedence, selector-versus-projection distinctions, named slot references, raw transport snapshots and actual query keys are fixed once above and reused by all tasks.
- Review Focus cases are included in named task tests. Standalone legacy usage, nested pages, disappearing option fields and concurrent saves are explicitly covered.
- No implementation has started. Approval of the design is recorded; plan review and execution-method selection are the next required workflow step.
