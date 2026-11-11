# Dependent Select Request Filters Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Configure dependent select requests once per field and apply them consistently to forms, table actions, bulk actions, and filter panels.
**Architecture:** A shared typed rule resolver produces equality scalars or membership arrays. A runtime input adapter loads options from current form/draft state using the centralized request layer. Tenant Panel uses one rule editor across the existing select source editors; backend selection reads add typed repeated-value membership.
**Tech Stack:** React, TypeScript, TanStack Query, existing shared controls, Go/Fiber/MongoDB.
**Spec:** docs/superpowers/specs/2026-11-07-dependent-select-request-filters-design.md

## Global Constraints

- User approved the design and implementation; proceed through all tasks without another approval round.
- Preserve all existing uncommitted work; work in current feature checkouts, capture pre-change files, no resets/stashes/commits/merges.
- Rules are an optional `sourceFilterBindings` array; existing fields without rules retain behavior.
- Missing references wait without a request; false/zero remain valid; clear dependent descendants on actual source changes, not initialization.
- Use the same effective request identity for wire and cache; preserve request and record keys.
- No live record writes in verification. No arbitrary expression evaluation or Mongo operator inputs.

## Review Focus

- Prefilled `_id` with a paginated parent absent: use stored ID only for `_id`-valued selects; other paths stay unavailable.
- Apply-mode table filters: lookup queries use draft state, table requests stay applied until Apply.
- A sole returned option must not auto-select a new dependent value after source changes.
- Named request connections must honor missing-reference readiness and merge local filters.
- Older config normalization and action/form serializers must retain rule identifiers and case-sensitive field/path values.

## Shared interfaces

Both frontends: `src/types/selectFilters.ts` exports `SelectFilterBinding` with `{ id: string; field: string; operator: "eq" | "in"; value: { source: "literal"; value: string | number | boolean | (string | number | boolean)[] } | { source: "field"; fieldKey: string } | { source: "record"; fieldKey: string; path: string[] } }`.

`src/utils/selectFilters.ts` exports `validateSelectFilterBindings(fields)`, `resolveSelectFilters(field, fields, values, optionsByField)`, `clearDependentSelectValues(fields, previous, next)`, `referencedSelectRecordFields(fields, key)`. Fields structurally include formKey/type/sourceValueField/sourceRequestFilters/sourceFilterBindings. Resolver returns `{ready, filters, message?}`; `in` produces an array, serialized as repeated query parameters. Options retain `sourceItem`.

Backend wire: `filter.location=id1&filter.location=id2` becomes typed `$in`; single values preserve equality. Existing schema table-source reads already support repeated field query values. Rules are AND across target fields.

### Task 1: Typed rules and resolver

Files: create mirrored `src/types/selectFilters.ts`, `src/utils/selectFilters.ts` and tests; extend shared field interfaces (`src/types/page.ts`, Tenant `src/utils/api/page.ts`, both generic input types).
- [x] Write resolver/validation/clearing tests covering all value sources, nested own paths, false/zero, missing, multiple values, duplicate targets, missing/self/cycle references and edit `_id` fallback. Run RED.
- [x] Implement shared interfaces/functions and mirrored files. Run focused tests GREEN.
- [x] Capture baseline and ledger outcomes. Tests also pin no clearing of unchanged initialized defaults.

### Task 2: Backend membership and config persistence

Files: `models/containerModel.go`, new model roundtrip test, `utils/buildFilterFromQuery.go`, focused membership tests.
- [x] Write RED tests for repeated ObjectID/number/string selection filters, one-element equality, malformed values, hashed-field omission, and rule JSON/BSON persistence.
- [x] Add optional typed SelectFilterBinding config and narrow repeated-value parsing in BuildSelectionFilterFromQuery; preserve existing equality and BuildFilterFromQuery behavior.
- [x] Run focused Go tests then models/utils suites. No unrelated controller changes.

### Task 3: Tenant Panel rule editing and serialization

Files: create `src/components/PageDesigner/SelectFilterBindingsEditor.tsx` and tests; modify `FormFieldEditor.tsx`, `PageDesigner.tsx`, `RelationMatrixRowFilterEditor.tsx`, related shared types/adapters, `src/visualBuilder/VisualActionTools.tsx` if its separate form editor needs integration, backend-shaped field types.
- [x] Write RED editor/persistence tests for adding/removing rules, source field / peer input / record path / typed literal choices and invalid dependencies.
- [x] Reuse existing UI controls. Wire the shared editor into every configured select field family. Carry `sourceFilterBindings` through all clean/serialization paths, and validate it on saves using Task 1 helpers.
- [x] Run editor/persistence tests GREEN and focused lint/typecheck. Add concise user-facing configuration guide.

### Task 4: Runtime shared option loading and clearing

Files: new `src/components/forms/useDependentSelectInputs.ts`; modify `pageRuntime/requests/selectionOptions.ts`, `utils/selectionQuery.ts`, `components/forms/{DynamicForm,DynamicFormField,useFormSelectionData}.tsx/ts`, `components/panelComponents/FormElements/{GenericAddEditPanel,GenericPaginatedPage,GenericUnpaginatedPage}.tsx`, `components/panelComponents/Tables/FilterPanel.tsx`, `utils/{formConfig,tableFilters}.ts/tsx`, schema-derived input adapters and visual action validation.
- [x] Write mounted RED tests for missing references, outgoing matching filters, source changes clearing descendant values, defaults/record paths, multiple values, stale responses, errors/retry, sole option auto-fill suppression, named request merge/readiness and identical-read sharing.
- [x] Add one shared query adapter driven by current local values and source metadata carried on GenericInputType. Remove eager reads for live-dependent fields. Keep non-dependent legacy options compatible and record field projections complete.
- [x] Use draft values in FilterPanel, apply clearing in actual event-driven value updates, prevent submitting while active dependent fields are waiting/error/loading, and preserve edit/default hydration.
- [x] Run mounted and existing form/filter/action/request regressions GREEN.

### Task 5: Integration, browser checks, review

- [x] Full frontend suites, builds and focused lint; Go models/utils and other required relevant checks.
- [x] Isolated browser setup verifies rule authoring/save/reload plus two arbitrary schema pairs, changing/clearing sources and filter Apply behavior with in-memory responses.
- [x] Generate task diff against captured baseline; fresh independent review of spec compliance and correctness. Address important findings with RED/GREEN tests.
- [x] Record evidence and any material limitations; finish with a short usage example. Leave work uncommitted.
