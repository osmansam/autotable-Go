# Dependent select request filters

Status: approved by user for implementation. User chose filtering the source request over filtering already loaded options.

## Intent

Allow any configured select field to request options using values from its own form or filter panel. For example, Count list can request records whose `location` equals the selected Location record's `_id`. Names and schemas are configurable; this example is not built into runtime code.

This applies to React Template selects configured through Tenant Panel: standalone forms, table create/edit/custom actions, bulk action forms, table filter panels (paginated and unpaginated), relation matrix filters, and record-action forms opened by visual buttons. Schema-derived selects using the same generic input model must carry the settings too. Navigation menus and unrelated settings selectors are not data-entry fields in this scope.

## Existing behavior and constraints

- `sourceRequestFilters` already supplies fixed selection filters; it currently receives no live form state.
- `sourceFilterCondition` is a separate condition over loaded option records. It has inconsistent handling between forms, action modals, and table filters. Preserve its existing purpose; the new feature filters requests.
- Table action options are currently loaded outside their modal's local state. Filter panels may have draft values distinct from applied table filters. Reading only the parent's state would produce incorrect requests.
- Central page requests already define effective request identities and share matching reads. Select dependencies must use that layer and include resolved filters in the cache key.
- Selected values are commonly scalar IDs, while option `sourceItem` carries the full selected record when requested. Both are useful and must remain distinct.

## Approaches

1. **Filter source requests — chosen by user.** Fetch matching options as dependencies change. This works with large sources and avoids relying on records already loaded. Requires sharing the current field state with option queries.
2. **Filter loaded options — considered.** Less network activity but can only match records/fields already loaded. User did not choose this approach; retain existing option-condition behavior without using it to implement the new rules.

## Configuration

Add an optional `sourceFilterBindings` array to the shared select configuration. Each rule has a stable ID, target source field, comparison, and value reference. Keep rules as an array so response normalization cannot rename dynamic source-field or input identifiers.

The Tenant Panel select editor shows a reusable **Dependent request filters** section alongside the existing source configuration. Authors add/remove rules and choose:

- **Source field:** a field of the select's source schema (with a manual field path fallback where source metadata is unavailable).
- **Comparison:** equals, or one of for multiple values.
- **Value from:** a fixed value, another input's current value, or a field of another select's selected record.
- **Input / record field:** the peer input and optional selected-record path, such as `_id`.

The editor explicitly distinguishes the option source field being filtered from the input supplying the value. It shows a readable summary, such as `Count list.location equals Location → _id`.

Current values include typed, selected, prefilled, default, and edit-mode values. There is no requirement to interact with a select first. A fixed rule requires no peer input. More than one rule is supported and rules are combined with AND. Chains across several inputs are supported.

The shared editor is reused by standalone form fields, table filter inputs, table row/create/bulk action fields, relation matrix filters, and visual record-action field editors. Existing schema/action configuration serialization must retain the new property.

## Runtime behavior

Implement one resolver and option-query path for dependent selects, used where current input state actually lives: DynamicForm, GenericAddEditPanel, and FilterPanel. Carry source configuration and stable page-request slot through GenericInputType rather than adding separate request logic for each table implementation.

Resolve fixed values, field values, and selected-record paths without evaluating JavaScript. Use own properties, bounded paths, and existing blocked-key rules. Preserve value types: `0` and `false` are valid; null, undefined, an empty string, or an empty reference list means the dependency is not ready.

Request behavior:

1. Combine existing fixed request filters with resolved rules. A conflicting duplicate target is rejected in the editor instead of silently overwriting another rule.
2. If a required reference is missing, show an empty, disabled dependent select with a short explanation and do not send an unfiltered request.
3. When references become available, fetch matching options. Loading and errors belong to that field and provide a retry path.
4. When a source input changes or clears, clear dependent values and their dependent descendants. Apply this on actual input changes, not on initial edit/default hydration. This prevents submitting selections belonging to the previous source value.
5. Do not display previous-filter results while a new request is pending. Late responses from an older query cannot replace the current options.
6. Existing fields without rules retain their behavior. Existing explicit invalidate-key settings still work; dependency clearing is deduplicated and does not form an effect loop.

FilterPanel uses its draft state for dependent option requests when Apply mode is enabled. The table's applied data query changes only when Apply is pressed. Clear All clears the dependent selections as well.

Selected-record field references use the selected option's `sourceItem`. Automatically include referenced record fields in configurable selection/schema projections where supported. For workflow/pipeline results whose shape is user-defined, missing fields produce a contained field message; do not invent extra per-record requests. Parent selected values and projected record fields are resolved within the same form or filter panel, not a global shared mutable form state.

For prefilled/edit values, use initial record data when present. A selected-record `_id` reference may use the stored scalar value only when that select explicitly uses `_id` as its value field (including the default). If an arbitrary selected-record field is absent from a paginated parent result and initial data, show that the selected record is not available in the loaded parent options; do not guess, run an unbounded query, or add an implicit per-record read. Ordinary current-value references continue to work for prefilled IDs. This fallback/unavailable policy is an implementation proposal settled after the builder check.

## Request sharing and source support

Extend the existing selection option adapter to accept resolved local filters and readiness. For a named page request, merge local filters with definition defaults, page bindings, and connection overrides through the existing resolver. For an inline source, build the filtered selection descriptor. Effective source, parameters, projection and filters determine identity, so equal requests share cache/in-flight work and different filters produce separate requests.

Do not let a connected page request force a query to run while its input dependencies are unresolved. Remove duplicate eager requests for dependent fields from action/filter option prefetch hooks. Keep the field configuration's slot stable for named request mapping.

Inline schema selections use the existing selection endpoint. Add a narrow typed membership representation for the one-of operator, validated against schema field metadata, while retaining existing equality query behavior. Named schema, pipeline and workflow request sources continue to use their existing transport semantics; filters are passed as resolved request values. A pipeline or workflow must consume those parameters in its own definition to filter its result.

Preserve option record keys and request slot keys at API boundaries. This feature must not repeat the earlier camel-cased slot problem.

## Validation and persistence

Mirror the optional config in Tenant Panel types, React Template types/generic input adapters, and backend typed ActionFormFieldConfig. Existing page/component map payloads must preserve it. Validate target paths, references, operators and literal types. Reject missing peer inputs, self-dependencies, cycles, and invalid source paths before saving; defend again at runtime for imported/older malformed definitions. A bad select must not blank the entire page.

Keep authorization and field restrictions on the existing server read paths. Rules supply query values, not arbitrary Mongo operators or executable expressions. No database migration or changes to existing pages are required.

## Proposed defaults and boundaries

These are implementation proposals, not additional user requirements:

- Clear dependent values whenever their referenced input changes; do not silently auto-select the first replacement option.
- Wait for all references in a rule set; do not broaden the query when one is empty.
- Equality and one-of cover single and multiple source values. General expression builders, OR groups, cross-page input references, and automatic rewriting of pipeline/workflow definitions are outside this change.
- Prefer the existing shared form controls and per-field configuration; no new separate workspace/dialog is introduced.

## Verification

- Resolver tests for scalar IDs, selected `_id` and nested paths, defaults/edit values, fixed values, false/zero, missing references, multiple values, and transitive clearing.
- Query tests for exact outgoing filters, no request while missing, distinct filter identities, identical-request sharing, and named request merging/readiness.
- Mounted flows for standalone forms, table action/create/bulk modals, table filters including Apply mode, and relation matrix filters. Include source changes, clears, initial edit preservation, stale response isolation, retry, and unaffected unrelated fields.
- Tenant Panel tests for rule creation/edit/removal, peer/schema choices, cycle errors, and save/reload retention across every field editor family.
- Backend membership/equality and typed config round-trip tests.
- Isolated browser checks using in-memory data for Location → Count list and an arbitrary second schema pair. No live record writes.
- Full frontend suites and builds, focused lint, and relevant backend checks before completion.
