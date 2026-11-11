# Central page requests

Date: 2026-11-06
Status: approved by the user on 2026-11-06; implementation plan pending review.

## Intent

Configure page data requests centrally in Tenant Panel and connect any data-consuming page component to them. The React runtime makes one read for components whose effective requests match. A component with different filters, sorting, pagination, parameters, or returned data runs a separate read. Returning to the same effective request rejoins the shared result.

The user's explicit requirements are centralized page connections, support beyond tables and visuals, and deduplication according to actual data/filter/sort requirements. The details below are proposed implementation choices, rather than additional user requirements.

## Approaches

1. **Named page requests plus canonical effective-request caching (selected direction).** Central definitions supply shared defaults; consumers may override controls. Cache identity comes from the fully resolved transport request, independently of component type or request name. This provides central configuration and independent component behavior.
2. **Deduplicate existing component requests only.** Smaller change, but repeats configuration throughout the page and does not provide the requested central connections.
3. **Publish a table's rows to other components.** Suitable for a details panel, but makes a table the owner of the dataset and prevents independent filter/sort variants. Keep the existing selected-record connection as its own feature.

## Existing implementation and scope

- `react-template/src/pageRuntime/PageRuntimeProvider.tsx` owns page filters, parameter resolution and component outputs. It does not currently own a shared data-request contract.
- Tables use dynamic query keys; visual repeaters use `visual-collection` keys and their own result/error adapter, despite using the same `/dynamic/table-source` transport for schema lists.
- Info/distribution blocks use table-source or workflow reads. Charts use pipeline/workflow reads. Unpaginated tables and relation matrices also use dynamic list reads. These transports must retain their existing semantics.
- Tenant Panel maintains page types in both `src/utils/api/page.ts` and `src/types/page.ts`; page saves, copies, reordering and visual-placement saves must preserve the new configuration.
- Go persists a typed PageModel and validates its runtime graph. A frontend-only field would not reliably survive saving, so backend persistence and validation are part of this feature.

This is one feature spanning `autotable-Go`, `tenantPanel`, and `react-template`. Existing unrelated working-tree edits must be preserved.

## Page model and connections

Add optional `requests` to the page model. A definition has a stable ID, a human-readable name, an existing supported read source, static parameters, parameter bindings, filters, sorting, pagination defaults and an explicit result contract. Source definitions reuse existing schema/pipeline/workflow read capabilities; this feature does not invent execution support for currently unsupported API/function sources.

Add optional request connections to data-consuming components and visual repeaters. A connection references a page request and supplies optional local overrides. Components with several reads expose named connections, such as a relation matrix's rows and columns or a field's options. Components consume the appropriate result path without issuing an additional request. A display mapping or result selector does not change the request identity; a server-side projection does.

The central request owns returned-field configuration. Connecting a table and visual to it does not implicitly narrow the visual to the table's visible columns or broaden backend access. If a consumer explicitly requests a different projection, it has a different effective request.

Named requests are local to their page; reusable visual-library designs store a connection slot, not a particular page's request ID. Page placement supplies that connection. Standalone editor/sample previews continue to use sample data and do not execute runtime requests.

## Tenant Panel authoring

Provide a Page Requests editor in the page workspace to add/edit/remove definitions and see connected consumers. Each compatible data-consuming component exposes a request selector and its supported local overrides. Visual repeaters can select a page request when editing a placed visual; library designs can retain their existing direct schema source.

Shared page filters and request defaults affect all inheriting consumers. Table search/filter/sort/page controls are local overrides by default, so changing one table does not silently change another component. Authors who want synchronized behavior bind the consumers to the same page filters/defaults. Equal effective values share automatically regardless of whether they came from shared or local controls.

Reject deletion of a request that still has consumers until they are disconnected or reassigned. Show missing/incompatible connections as actionable validation errors. Preserve connections through normal edits, page copying, visual saving, metadata changes and ordering changes. If component/request IDs are regenerated on copy, rewrite their references together.

## Runtime identity and sharing

Resolve request defaults, page bindings and consumer overrides before creating the descriptor. Precedence is central defaults, resolved central parameters, then explicit consumer overrides. Omitted overrides inherit; explicit valid false/zero/empty values must not be lost through truthiness checks. Clearing a local override restores inheritance. Override semantics must be represented explicitly rather than relying on accidental object-spread order.

Canonical identity includes page scope, verified tenant/project/application identity, method, endpoint/transport, source, normalized wire parameters, filters, sorting, page/limit, projection and any response-affecting body or header. Source transport and result contract must be compatible before sharing. Request IDs and component IDs do not distinguish otherwise identical reads within the same page.

Canonicalization is deterministic for unordered object keys and encoded parameter names. Preserve ordered arrays, sort priority, repeated query values, typed values and other distinctions that affect the endpoint. Use the same normalized descriptor to build both the key and the outgoing request. Do not merge requests merely because they use the same schema.

Use the existing TanStack Query infrastructure, with one shared transport result and consumer-specific adapters for rows, totals, charts, info blocks and visual collection states. Consumers subscribe to the same pending operation and cached result. Attaching a matching consumer while a successful result is fresh does not make another request. A failed refresh hides data when the existing access/error rules require it.

Unmounting one subscriber must not cancel a request still needed by others. Authentication/project changes cancel or discard the former scope and prevent late results from appearing in the new scope. Invalidation and successful mutations refresh active matching descriptors once per distinct request; a component with differing filters still refreshes its own distinct descriptor. Preserve bounded retries, cancellation and current authorization-error behavior.

Only data reads participate. Create/update/delete, form submissions, action workflows and any execution that changes data keep their existing mutation behavior. Deduplication must never collapse two user actions. Existing workflow/pipeline read adapters must keep their actual endpoint semantics; transports that return different data are not interchangeable.

## Coverage and compatibility

All existing page read consumers must be audited and routed through the common descriptor/cache layer: paginated/unpaginated tables, visual repeaters, charts, info/distribution blocks, relation-matrix feeds and configurable lookup/options reads. Static components need no request. Metadata/session prerequisites remain their own shared queries and do not count as a duplicate dataset read.

Legacy pages without `requests` continue to work without manual migration. Their inline sources compile through compatible adapters into the same effective-request machinery where semantics match. No forced conversion of saved page/library documents is required. Preserve existing selected-record bindings, component outputs, table actions, local filtering behavior, layouts and authentication rules.

Backend and editor validation cover duplicate/missing request IDs, invalid source/result combinations, malformed overrides and unresolved parameter references. Integrate request-to-component dependencies into the existing runtime graph and reject cycles, including a request depending on the output of its own consumer. Waiting dependencies do not fetch prematurely. Do not add unrelated stricter validation to legacy pages.

## Verification requirements

- A mounted table and visual consumer with the same effective request produce exactly one dataset network call, including simultaneous and staggered mounting.
- Different filters or sorting produce two calls and correct distinct results. Once effective controls match again, consumers rejoin the same cache; parameter insertion order alone does not split it.
- Different pagination, projection, source transport or response-affecting parameters remain separate. Presentation-only selectors do not split the underlying read.
- Cover matching consumers across the other supported component types and multi-source components; no connected component also runs its legacy fetch.
- Shared filter changes, local overrides, clear-to-inherit, false/zero values, empty results, failures/retry, concurrent invalidation and unmounting preserve correct states and request counts.
- Project/user changes and late responses cannot cross scopes. Existing visual/table mutations refresh every relevant variant once without merging actions.
- Definitions and connections round-trip through Go JSON/BSON persistence and Tenant Panel save/copy/reorder/visual-placement paths. Explicitly clearing the request list is persisted rather than lost to `omitempty`.
- Missing references, incompatible connections and dependency cycles are rejected with useful errors. Legacy pages and editor/sample previews retain their existing behavior.
- Run targeted regression tests, repository type/build checks and isolated browser verification with request-count fixtures. Broaden to affected repository suites before reporting completion.

## Boundaries and remaining builder decisions

No new third-party dependency, database collection or dataset endpoint is required. This feature does not combine differently filtered/sorted server requests into a larger download or perform unauthorized client-side equivalents. It does not turn arbitrary mutation workflows into reads.

The builder can choose focused module/file names, adapter boundaries and editor layout using existing repository conventions. Request signature correctness, whole-page read coverage, page persistence and backwards compatibility are required outcomes, not optional later phases.
