# Array Field Page CRUD Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build configurable pages whose route-bound parent record exposes one embedded array as generated, customizable table rows with atomic Add, Edit, Delete, Reorder, and relation-membership mutations.

**Architecture:** Extend the existing `dataMode: "arrayField"`, `arraySource`, action, and generated-relation page metadata. Add a focused Go array mutation service that validates container metadata and performs parent-scoped compare-and-update writes; both React applications call this API rather than replacing a parent array from stale client state.

**Tech Stack:** Go 1.x, Fiber, MongoDB Go driver, React 18, TypeScript, TanStack Query, Axios, Vitest.

## Global Constraints

- Parent routes use `:param`, and component source values support `{{route.id}}`.
- Only direct container fields with type `array` and declared children are eligible.
- Every mutation is scoped by tenant, project, parent schema, parent ID, array field, row identity field, and row identity value.
- Clients never submit MongoDB paths or operators.
- Existing paginated, all-items, nested-row, and read-only array-field pages remain compatible.
- Existing uncommitted work in all three repositories belongs to the user and must be preserved.
- No existing array page becomes writable merely by loading newer code; generated actions must be persisted and enabled.

---

## File map

### `autotable-Go`

- `models/pageModel.go`: add persisted array-source parent-ID binding and generation metadata.
- `models/frontendValidation.go`: validate complete and safe array-table configuration.
- `models/models_test.go`, `models/frontendValidation_create_test.go`: model round-trip and validation coverage.
- `requests/dynamic_array_request.go`: decode narrow JSON payloads for array mutations.
- `requests/dynamic_array_request_test.go`: request validation tests.
- `repositories/dynamic_repository.go`: expose one compare-and-update operation for parent documents.
- `repositories/dynamic_repository_test.go`: verify Mongo filters and updates.
- `services/dynamic_array_service.go`: metadata lookup, child merge/validation, concurrency checks, workflows, audit, and cache/outbox integration.
- `services/dynamic_array_service_test.go`: service behavior and failure coverage.
- `controllers/dynamicArrayController.go`: Fiber adapters and status/error envelopes.
- `controllers/error_paths_test.go`: controller failure coverage.
- `routes/dynamicRoutes.go`, `routes/routes_test.go`: register array endpoints before generic `/:id` routes.

### `tenantPanel`

- `src/types/page.ts`, `src/types/layout.ts`, `src/utils/api/page.ts`: matching page metadata types.
- `src/utils/pageDesignerArraySource.ts`: focused generation/reconciliation and eligible-field helpers.
- `src/utils/pageDesignerArraySource.test.ts`: generation and preservation tests.
- `src/utils/pageDesignerTableConfig.ts`, `src/utils/pageDesignerTableConfig.test.ts`: normalize persisted settings.
- `src/components/PageDesigner/PageDesigner.tsx`: guided array source controls and Generate/Regenerate UI.
- `src/utils/api/dynamicArray.ts`, `src/utils/api/dynamicArray.test.ts`: typed array mutation requests.
- `src/utils/tableConfig.ts`, `src/utils/tableConfig.test.ts`: retain parent/child mutation metadata on flattened rows.
- `src/components/panelComponents/FormElements/GenericPaginatedPage.tsx`: preview CRUD and reorder integration.
- `src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx`: preview CRUD and reorder integration.
- `src/utils/useGeneratedRelationTableColumns.tsx`: use row-scoped Edit and roll back failed optimistic switches.
- `src/pages/PagePreviewPage.tsx`: route-bound single-parent fetch behavior.

### `react-template`

- `src/types/page.ts`, `src/types/layout.ts`, `src/components/panelComponents/shared/types.ts`: runtime metadata types.
- `src/utils/api/dynamicArray.ts`, `src/utils/api/dynamicArray.test.ts`: matching typed request layer.
- `src/utils/tableConfig.ts`, `src/utils/tableConfig.test.ts`: matching flattened-row metadata.
- `src/utils/dynamicPageTableConfig.ts`, `src/utils/dynamicPageTableConfig.test.ts`: runtime config resolution for a route-bound parent.
- `src/components/panelComponents/FormElements/GenericPaginatedPage.tsx`: generated runtime CRUD and reorder.
- `src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx`: generated runtime CRUD and reorder.
- `src/utils/useGeneratedRelationTableColumns.tsx`: row-scoped relation edits with rollback.
- `src/utils/tableActions.ts`, `src/utils/tableActions.test.ts`: preserve explicitly disabled generated actions.

---

### Task 1: Persist and validate complete array-page metadata

**Files:**
- Modify: `models/pageModel.go`
- Modify: `models/frontendValidation.go`
- Test: `models/models_test.go`
- Test: `models/frontendValidation_create_test.go`

**Interfaces:**
- Produces: `TableArraySourceConfig.ParentID ParameterBinding`
- Produces: `TableArraySourceConfig.AutoGenerate *TableArrayAutoGenerateConfig`
- Consumes: existing `DataBinding`, `ParameterBinding`, `TableColumnConfig`, `ActionConfig`, and `TableDragConfig`.

- [ ] **Step 1: Write failing BSON/JSON round-trip tests**

Add a page fixture whose table contains:

```go
ArraySource: &TableArraySourceConfig{
    Enabled:          true,
    Field:            "duties",
    RowIdentityField: "duty",
    ParentID: ParameterBinding{
        Source: "static",
        Value:  "{{route.id}}",
    },
    AutoGenerate: &TableArrayAutoGenerateConfig{
        Columns: true,
        Add:     true,
        Edit:    true,
        Delete:  true,
        Reorder: false,
    },
},
```

Assert the decoded page retains every field and existing array-source fixtures still decode when the new fields are absent.

- [ ] **Step 2: Run the model tests and confirm failure**

Run: `go test ./models -run 'TestPageTableArraySource'`

Expected: FAIL because `ParentID`, `AutoGenerate`, and `TableArrayAutoGenerateConfig` do not exist.

- [ ] **Step 3: Add the metadata structs**

Implement:

```go
type TableArrayAutoGenerateConfig struct {
    Columns bool `bson:"columns" json:"columns"`
    Add     bool `bson:"add" json:"add"`
    Edit    bool `bson:"edit" json:"edit"`
    Delete  bool `bson:"delete" json:"delete"`
    Reorder bool `bson:"reorder" json:"reorder"`
}

type TableArraySourceConfig struct {
    Enabled          bool                          `bson:"enabled,omitempty" json:"enabled,omitempty"`
    Field            string                        `bson:"field,omitempty" json:"field,omitempty"`
    RowIdentityField string                        `bson:"rowIdentityField,omitempty" json:"rowIdentityField,omitempty"`
    ParentID         *ParameterBinding             `bson:"parentId,omitempty" json:"parentId,omitempty"`
    AutoGenerate     *TableArrayAutoGenerateConfig `bson:"autoGenerate,omitempty" json:"autoGenerate,omitempty"`
}
```

Use a pointer for `ParentID` so legacy configurations remain distinguishable and read-only.

- [ ] **Step 4: Write failing configuration validation tests**

Cover missing parent binding, empty array field, empty identity field, reorder without `drag.orderField`, and generated mutation actions without a parent binding. Retain acceptance of legacy read-only array sources.

- [ ] **Step 5: Implement metadata validation**

Extend `ValidateTableComponentConfig` so writable/generated array sources require a non-empty parent binding, and `AutoGenerate.Reorder` requires enabled drag configuration with an order field. Keep schema/field compatibility validation in the designer and mutation service, where container metadata is available.

- [ ] **Step 6: Run tests**

Run: `gofmt -w models/pageModel.go models/frontendValidation.go models/models_test.go models/frontendValidation_create_test.go`

Run: `go test ./models`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add models/pageModel.go models/frontendValidation.go models/models_test.go models/frontendValidation_create_test.go
git commit -m "feat: model array page CRUD configuration"
```

---

### Task 2: Parse and validate array mutation requests

**Files:**
- Create: `requests/dynamic_array_request.go`
- Create: `requests/dynamic_array_request_test.go`

**Interfaces:**
- Produces: `ArrayRowMutationRequest`, `ArrayReorderRequest`, `ParseArrayRowMutation`, and `ParseArrayReorder`.
- Consumes: JSON request bodies from the controller; does not consume arbitrary update operators.

- [ ] **Step 1: Write failing parser tests**

Test valid payloads and rejection of blank identity fields, missing values, empty updates, unknown top-level properties, duplicate reorder identities, and reorder lists with mixed JSON scalar types.

Use these request shapes:

```json
{"rowIdentityField":"duty","item":{"duty":"Clean","order":0}}
```

```json
{"rowIdentityField":"duty","updates":{"description":"Closing task"}}
```

```json
{"rowIdentityField":"duty","orderField":"order","rowIdentities":["Open","Clean"]}
```

- [ ] **Step 2: Run tests and confirm failure**

Run: `go test ./requests -run 'TestParseArray'`

Expected: FAIL because the parser API does not exist.

- [ ] **Step 3: Implement strict request types and parsers**

Implement exported request values:

```go
type ArrayRowMutationRequest struct {
    RowIdentityField string                 `json:"rowIdentityField"`
    Item             map[string]interface{} `json:"item,omitempty"`
    Updates          map[string]interface{} `json:"updates,omitempty"`
}

type ArrayReorderRequest struct {
    RowIdentityField string        `json:"rowIdentityField"`
    OrderField       string        `json:"orderField"`
    RowIdentities    []interface{} `json:"rowIdentities"`
}
```

Decode with `json.Decoder.DisallowUnknownFields()`, require the operation-appropriate object, and reject duplicate normalized identities before service execution.

- [ ] **Step 4: Run tests**

Run: `gofmt -w requests/dynamic_array_request.go requests/dynamic_array_request_test.go`

Run: `go test ./requests`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add requests/dynamic_array_request.go requests/dynamic_array_request_test.go
git commit -m "feat: parse embedded array mutations"
```

---

### Task 3: Add compare-and-update repository support

**Files:**
- Modify: `repositories/dynamic_repository.go`
- Test: `repositories/dynamic_repository_test.go`

**Interfaces:**
- Produces: `UpdateByFilter(ctx, tenantID, projectID, schemaName string, filter bson.M, update bson.M) (*mongo.UpdateResult, error)`.
- Consumes: service-built, metadata-validated Mongo filters and updates.

- [ ] **Step 1: Write a failing mocked Mongo repository test**

Assert this call targets the tenant/project schema collection and forwards both a parent-and-prior-array filter and `$set` update unchanged:

```go
filter := bson.M{"_id": parentID, "duties": priorDuties}
update := bson.M{"$set": bson.M{"duties": nextDuties}}
```

- [ ] **Step 2: Run the focused test and confirm failure**

Run: `go test ./repositories -run 'TestDynamicRepositoryUpdateByFilter'`

Expected: FAIL because `UpdateByFilter` does not exist.

- [ ] **Step 3: Implement the narrow repository method**

Use the same collection resolver as `UpdateByID`:

```go
func (r *DynamicRepository) UpdateByFilter(
    ctx context.Context,
    tenantID, projectID, schemaName string,
    filter bson.M,
    update bson.M,
) (*mongo.UpdateResult, error) {
    return r.GetCollection(tenantID, projectID, schemaName).UpdateOne(ctx, filter, update)
}
```

Wrap the call with the same `observability.StartSpan`, `MongoTraceAttrs`, and `EndSpan` pattern used by `UpdateByID`.

- [ ] **Step 4: Run repository tests**

Run: `gofmt -w repositories/dynamic_repository.go repositories/dynamic_repository_test.go`

Run: `go test ./repositories`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add repositories/dynamic_repository.go repositories/dynamic_repository_test.go
git commit -m "feat: support compare-and-update dynamic writes"
```

---

### Task 4: Implement atomic array mutation service behavior

**Files:**
- Create: `services/dynamic_array_service.go`
- Create: `services/dynamic_array_service_test.go`
- Modify: `services/dynamic_service.go`

**Interfaces:**
- Produces: `AddArrayRow`, `UpdateArrayRow`, `DeleteArrayRow`, and `ReorderArrayRows` methods on `DynamicService`.
- Consumes: `requests.ArrayRowMutationRequest`, `requests.ArrayReorderRequest`, container metadata, `DynamicRepository.UpdateByFilter`, existing workflow/audit/outbox helpers.
- Returns: updated parent document and changed row, or `ServiceError` with status 400/404/409.

- [ ] **Step 1: Write failing metadata and identity tests**

Create a checklist container fixture with `duties` children (`duty`, `description`, `locations`, `order`). Test rejection of a missing schema, invalid parent ID, non-array field, unknown identity field, unsupported structured identity, zero identity matches, and duplicate identity matches.

- [ ] **Step 2: Write failing Add/Edit/Delete tests**

Cover:

- Add validates a child and appends it.
- Add defaults `order` to the current length only when an order field is supplied by configuration/request context.
- Edit merges a partial patch and validates the complete child.
- Edit may change identity when the replacement is unique.
- Delete removes exactly one child.
- Relation Edit replaces only the selected child's `locations` value.
- A stale compare filter with `MatchedCount == 0` returns `409`.

- [ ] **Step 3: Write failing Reorder tests**

Require every current identity exactly once, reject duplicates/missing/foreign identities, and assert persisted zero-based order values.

- [ ] **Step 4: Run service tests and confirm failure**

Run: `go test ./services -run 'TestDynamicArray'`

Expected: FAIL because the service methods do not exist.

- [ ] **Step 5: Implement focused metadata helpers**

Add private helpers with these responsibilities:

```go
func embeddedArrayField(container *models.ContainerModel, fieldName string) (*models.Field, error)
func embeddedChildField(arrayField *models.Field, childName string) (*models.Field, error)
func normalizeArrayRows(value interface{}) ([]map[string]interface{}, error)
func matchingArrayRowIndexes(rows []map[string]interface{}, identityField string, identity interface{}) []int
```

Only accept direct names found in container metadata. Reuse existing field conversion and validation helpers on a temporary container whose `Fields` are the array's `Children`; do not duplicate type rules.

- [ ] **Step 6: Implement one shared mutation transaction**

Build a private method that:

1. Acquires the existing parent update lock convention.
2. Resolves the container and parent.
3. Copies the prior array.
4. Produces and validates the next array.
5. Runs before-update workflows.
6. Calls `UpdateByFilter` with `_id` and the prior array as the compare condition.
7. Maps zero matches to `409 Conflict`.
8. Runs after-update workflows and existing audit/outbox/cache invalidation logic.
9. Strips hashed values before returning the parent.

Include mutation context in workflow `StepOutputs` under a reserved `arrayMutation` key:

```go
map[string]interface{}{
    "operation":        operation,
    "arrayField":       arrayField,
    "rowIdentityField": identityField,
    "rowIdentity":      identity,
}
```

- [ ] **Step 7: Implement the four public service methods**

Keep each method responsible only for its array transformation, then delegate persistence to the shared transaction. Return the updated parent and affected child in a stable response structure:

```go
type DynamicArrayMutationResult struct {
    Parent map[string]interface{} `json:"parent"`
    Row    map[string]interface{} `json:"row,omitempty"`
}
```

- [ ] **Step 8: Run service and race tests**

Run: `gofmt -w services/dynamic_array_service.go services/dynamic_array_service_test.go services/dynamic_service.go`

Run: `go test ./services`

Run: `go test -race ./services -run 'TestDynamicArray'`

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add services/dynamic_array_service.go services/dynamic_array_service_test.go services/dynamic_service.go
git commit -m "feat: mutate embedded array rows atomically"
```

---

### Task 5: Expose authenticated array mutation routes

**Files:**
- Create: `controllers/dynamicArrayController.go`
- Modify: `controllers/error_paths_test.go`
- Modify: `routes/dynamicRoutes.go`
- Modify: `routes/routes_test.go`

**Interfaces:**
- Produces:
  - `POST /dynamic/:schema/:id/array/:field`
  - `PATCH /dynamic/:schema/:id/array/:field/:rowIdentity`
  - `DELETE /dynamic/:schema/:id/array/:field/:rowIdentity`
  - `PATCH /dynamic/:schema/:id/array/:field/reorder`
- Consumes: Task 2 parsers and Task 4 service methods.

- [ ] **Step 1: Write failing route-registration tests**

Register `DynamicRoutes("/dynamic", app)` and assert each endpoint is routed to a handler rather than the generic `/:id` endpoint. Ensure `/reorder` is registered before `/:rowIdentity`.

- [ ] **Step 2: Write failing controller tests**

Cover project-context failure, malformed JSON, service status propagation, URL-decoded identities, idempotency behavior, and the existing response envelope.

- [ ] **Step 3: Run tests and confirm failure**

Run: `go test ./routes ./controllers -run 'Array|array'`

Expected: FAIL because routes and handlers do not exist.

- [ ] **Step 4: Implement controller adapters**

Each handler must obtain tenant/project/user context, parse `schema`, `id`, `field`, and optional `rowIdentity`, call `beginDynamicIdempotency`, parse the strict request body, invoke the service, and finish through `sendIdempotentResponse`/`sendDynamicServiceError`.

Use `ConditionalAuthentication("UpdateDynamicModelItem")` for Add, Edit, and Reorder. Use `ConditionalAuthentication("DeleteDynamicModelItem")` for Delete, matching the approved permission mapping without adding container route flags.

- [ ] **Step 5: Register routes before generic item routes**

Add all four routes above `Delete("/:id")`, `Patch("/:id")`, and `Get("/:id")`, with existing default body, public, general, and write rate-limit middleware.

- [ ] **Step 6: Run controller and route tests**

Run: `gofmt -w controllers/dynamicArrayController.go controllers/error_paths_test.go routes/dynamicRoutes.go routes/routes_test.go`

Run: `go test ./controllers ./routes`

Expected: PASS.

- [ ] **Step 7: Run the backend suite**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add controllers/dynamicArrayController.go controllers/error_paths_test.go routes/dynamicRoutes.go routes/routes_test.go
git commit -m "feat: expose embedded array mutation routes"
```

---

### Task 6: Generate and reconcile customizable array tables in `tenantPanel`

**Files:**
- Modify: `../tenantPanel/src/types/page.ts`
- Modify: `../tenantPanel/src/types/layout.ts`
- Modify: `../tenantPanel/src/utils/api/page.ts`
- Create: `../tenantPanel/src/utils/pageDesignerArraySource.ts`
- Create: `../tenantPanel/src/utils/pageDesignerArraySource.test.ts`
- Modify: `../tenantPanel/src/utils/pageDesignerTableConfig.ts`
- Modify: `../tenantPanel/src/utils/pageDesignerTableConfig.test.ts`
- Modify: `../tenantPanel/src/components/PageDesigner/PageDesigner.tsx`

**Interfaces:**
- Produces: `eligibleArrayFields`, `eligibleIdentityFields`, `generateArrayTableDefaults`, and `reconcileArrayTableDefaults`.
- Consumes: container `Field` metadata, existing table columns/actions/form inputs, population settings, and Task 1 page types.

- [ ] **Step 1: Add matching TypeScript types and failing normalization tests**

Represent `parentId` and `autoGenerate` exactly as backend JSON. Assert normalization trims names, retains a static value of `{{route.id}}`, and omits incomplete writable generation metadata while preserving legacy read-only `arraySource`.

- [ ] **Step 2: Write failing generator tests with the checklist fixture**

Given the supplied `checklist.duties` metadata, assert generation produces columns for `duty`, `locations`, `order`, and `description`; Add/Edit/Delete actions; form inputs inferred from child types; and Reorder only when selected with `order`.

Expected core call:

```ts
generateArrayTableDefaults({
  schemaName: "checklist",
  parentId: { source: "static", value: "{{route.id}}" },
  arrayField: dutiesField,
  rowIdentityField: "duty",
  enabled: { columns: true, add: true, edit: true, delete: true, reorder: false },
});
```

- [ ] **Step 3: Write failing reconciliation tests**

Assert field-name matching preserves custom labels, ordering, input types, hidden columns, confirmation text, and `enabled: false`; adds a new child field; and returns a warning entry for a removed child field.

- [ ] **Step 4: Implement focused generator helpers**

Keep schema inspection and reconciliation outside `PageDesigner.tsx`. Reuse existing input-type and population-settings utilities. Return `{ table, warnings }` so the UI cannot silently drop stale customization.

- [ ] **Step 5: Run helper tests**

Run from `../tenantPanel`: `yarn test src/utils/pageDesignerArraySource.test.ts src/utils/pageDesignerTableConfig.test.ts`

Expected: PASS.

- [ ] **Step 6: Add the guided designer controls**

In the existing Array field rows section, add parent-ID binding (default text `{{route.id}}`), eligible identity selection, generation switches, and Generate/Regenerate. Disable generation until schema, parent binding, array field, and identity are valid. Show reconciliation warnings inline and require Save to persist the result.

- [ ] **Step 7: Verify designer build and tests**

Run from `../tenantPanel`: `yarn test src/utils/pageDesignerArraySource.test.ts src/utils/pageDesignerTableConfig.test.ts src/utils/pageBindings.test.ts`

Run from `../tenantPanel`: `yarn build`

Expected: PASS.

- [ ] **Step 8: Commit in `tenantPanel`**

```bash
git -C ../tenantPanel add src/types/page.ts src/types/layout.ts src/utils/api/page.ts src/utils/pageDesignerArraySource.ts src/utils/pageDesignerArraySource.test.ts src/utils/pageDesignerTableConfig.ts src/utils/pageDesignerTableConfig.test.ts src/components/PageDesigner/PageDesigner.tsx
git -C ../tenantPanel commit -m "feat: generate customizable array field tables"
```

---

### Task 7: Add the shared frontend array mutation client

**Files:**
- Create: `../tenantPanel/src/utils/api/dynamicArray.ts`
- Create: `../tenantPanel/src/utils/api/dynamicArray.test.ts`
- Create: `../react-template/src/utils/api/dynamicArray.ts`
- Create: `../react-template/src/utils/api/dynamicArray.test.ts`

**Interfaces:**
- Produces: `addDynamicArrayRow`, `updateDynamicArrayRow`, `deleteDynamicArrayRow`, `reorderDynamicArrayRows`, and `useDynamicArrayMutations`.
- Consumes: Axios clients and Task 5 endpoint response.

- [ ] **Step 1: Write identical failing request tests in both frontends**

Assert URL encoding for schema, parent ID, field, and identity, and exact payloads. Example:

```ts
await updateDynamicArrayRow({
  schemaName: "checklist",
  parentId: "abc",
  arrayField: "duties",
  rowIdentityField: "duty",
  rowIdentity: "Close/store",
  updates: { locations: [1, 3] },
});
```

Expected URL: `/dynamic/checklist/abc/array/duties/Close%2Fstore`.

- [ ] **Step 2: Run both focused tests and confirm failure**

Run from `../tenantPanel`: `yarn test src/utils/api/dynamicArray.test.ts`

Run from `../react-template`: `yarn test src/utils/api/dynamicArray.test.ts`

Expected: FAIL because the clients do not exist.

- [ ] **Step 3: Implement typed request functions**

Use a shared argument shape in each repository:

```ts
export interface DynamicArrayTarget {
  schemaName: string;
  parentId: string | number;
  arrayField: string;
  rowIdentityField: string;
}
```

Return `{ parent, row? }` from each mutation. The hook invalidates `['dynamic', schemaName]` queries on success or conflict and exposes async mutation functions so forms stay open until confirmed.

- [ ] **Step 4: Run tests and builds**

Run from each frontend: `yarn test src/utils/api/dynamicArray.test.ts`

Run from each frontend: `yarn build`

Expected: PASS.

- [ ] **Step 5: Commit each repository**

```bash
git -C ../tenantPanel add src/utils/api/dynamicArray.ts src/utils/api/dynamicArray.test.ts
git -C ../tenantPanel commit -m "feat: add dynamic array mutation client"
git -C ../react-template add src/utils/api/dynamicArray.ts src/utils/api/dynamicArray.test.ts
git -C ../react-template commit -m "feat: add dynamic array mutation client"
```

---

### Task 8: Wire array CRUD into `tenantPanel` preview

**Files:**
- Modify: `../tenantPanel/src/utils/tableConfig.ts`
- Modify: `../tenantPanel/src/utils/tableConfig.test.ts`
- Modify: `../tenantPanel/src/components/panelComponents/FormElements/GenericPaginatedPage.tsx`
- Modify: `../tenantPanel/src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx`
- Modify: `../tenantPanel/src/utils/useGeneratedRelationTableColumns.tsx`
- Modify: `../tenantPanel/src/pages/PagePreviewPage.tsx`
- Create: `../tenantPanel/src/utils/arrayTableMutations.test.ts`

**Interfaces:**
- Consumes: Task 6 metadata and Task 7 mutations.
- Produces: preview Add/Edit/Delete/Reorder and relation edits scoped to one parent and child.

- [ ] **Step 1: Replace whole-array helper expectations with row mutation targets**

Change the table helper contract from `buildArraySourceParentUpdate` to:

```ts
export interface ArraySourceMutationTarget {
  parentId: string | number;
  arrayField: string;
  rowIdentityField: string;
  rowIdentity: unknown;
  index: number;
}

export const getArraySourceMutationTarget = (
  row: GenericTableRow | undefined,
): ArraySourceMutationTarget | undefined;
```

Keep `applyTableArraySource`, but ensure it never treats the synthetic `_id` as a server resource ID.

- [ ] **Step 2: Write failing orchestration tests**

Test that Edit and relation switches call row-scoped PATCH; Delete calls row-scoped DELETE; Add calls POST using the route-bound parent; Reorder sends all identities; and a missing `parentId` leaves legacy pages read-only.

- [ ] **Step 3: Resolve and fetch exactly one parent in preview**

Use existing `resolveRouteParamValue` before rendering table pages. When `dataMode === "arrayField"`, require the resolved parent ID and use the existing get-one dynamic query rather than fetching all parents and flattening them.

- [ ] **Step 4: Wire Add, Edit, and Delete**

Branch existing handlers on `isTableArraySourceEnabled(tableConfig)`. Pass the selected row separately from synthetic display ID. Await mutation success before closing forms; on failure preserve form state and let the shared API error toast display the backend message.

- [ ] **Step 5: Wire generated relation columns**

Change the hook callback to return a promise. Snapshot membership before mutation, render the optimistic value, clear it on success, and restore the confirmed row on rejection.

- [ ] **Step 6: Wire reorder**

When drag is enabled for an array table, derive ordered raw identities from row metadata and call `reorderDynamicArrayRows`. Restore the last confirmed order on rejection.

- [ ] **Step 7: Run preview tests and build**

Run from `../tenantPanel`: `yarn test src/utils/tableConfig.test.ts src/utils/arrayTableMutations.test.ts src/utils/generatedRelationColumns.test.ts`

Run from `../tenantPanel`: `yarn build`

Expected: PASS.

- [ ] **Step 8: Commit in `tenantPanel`**

```bash
git -C ../tenantPanel add src/utils/tableConfig.ts src/utils/tableConfig.test.ts src/components/panelComponents/FormElements/GenericPaginatedPage.tsx src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx src/utils/useGeneratedRelationTableColumns.tsx src/pages/PagePreviewPage.tsx src/utils/arrayTableMutations.test.ts
git -C ../tenantPanel commit -m "feat: preview embedded array CRUD pages"
```

---

### Task 9: Port metadata and CRUD behavior to `react-template`

**Files:**
- Modify: `../react-template/src/types/page.ts`
- Modify: `../react-template/src/types/layout.ts`
- Modify: `../react-template/src/components/panelComponents/shared/types.ts`
- Modify: `../react-template/src/utils/tableConfig.ts`
- Modify: `../react-template/src/utils/tableConfig.test.ts`
- Modify: `../react-template/src/utils/dynamicPageTableConfig.ts`
- Create: `../react-template/src/utils/dynamicPageTableConfig.test.ts`
- Modify: `../react-template/src/components/panelComponents/FormElements/GenericPaginatedPage.tsx`
- Modify: `../react-template/src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx`
- Modify: `../react-template/src/utils/useGeneratedRelationTableColumns.tsx`
- Modify: `../react-template/src/utils/tableActions.ts`
- Modify: `../react-template/src/utils/tableActions.test.ts`
- Create: `../react-template/src/utils/arrayTableMutations.test.ts`

**Interfaces:**
- Consumes: page JSON created by Task 6 and mutation client from Task 7.
- Produces: generated application behavior equivalent to Task 8 preview.

- [ ] **Step 1: Add matching runtime types and config tests**

Copy the backend JSON contract exactly. Assert `{{route.id}}` is resolved before the table query, generated actions remain disabled when persisted with `enabled: false`, and legacy array pages remain read-only.

- [ ] **Step 2: Port row metadata and orchestration tests**

Use the same checklist fixture and behavioral assertions as Task 8. Do not assert implementation-specific React component internals; assert mutation targets and visible action behavior.

- [ ] **Step 3: Implement single-parent loading and array CRUD**

Port the tested preview behavior into the corresponding runtime files. Preserve runtime-only concerns already present in `dynamicPageTableConfig.ts` and `tableActions.ts`.

- [ ] **Step 4: Port relation rollback and reorder behavior**

Use the same promise-returning relation edit callback and confirmed-order rollback rules as `tenantPanel`.

- [ ] **Step 5: Run focused tests**

Run from `../react-template`: `yarn test src/utils/dynamicPageTableConfig.test.ts src/utils/tableConfig.test.ts src/utils/tableActions.test.ts src/utils/arrayTableMutations.test.ts src/utils/generatedRelationColumns.test.ts`

Expected: PASS.

- [ ] **Step 6: Run build**

Run from `../react-template`: `yarn build`

Expected: PASS. Do not stage generated `dist` changes unless the repository's established release workflow requires them and the user explicitly asks.

- [ ] **Step 7: Commit in `react-template`**

```bash
git -C ../react-template add src/types/page.ts src/types/layout.ts src/components/panelComponents/shared/types.ts src/utils/tableConfig.ts src/utils/tableConfig.test.ts src/utils/dynamicPageTableConfig.ts src/utils/dynamicPageTableConfig.test.ts src/components/panelComponents/FormElements/GenericPaginatedPage.tsx src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx src/utils/useGeneratedRelationTableColumns.tsx src/utils/tableActions.ts src/utils/tableActions.test.ts src/utils/arrayTableMutations.test.ts
git -C ../react-template commit -m "feat: render embedded array CRUD pages"
```

---

### Task 10: Verify the integrated checklist scenario

**Files:**
- Modify if needed: tests already introduced in Tasks 1–9 only.
- Do not modify: user-supplied checklist data attachments.

**Interfaces:**
- Verifies: `checklist/:id` + `{{route.id}}` + `duties` + `duty` identity + `locations` generated relation columns.

- [ ] **Step 1: Run all backend tests**

Run from `autotable-Go`: `go test ./...`

Expected: PASS.

- [ ] **Step 2: Run backend race tests**

Run from `autotable-Go`: `go test -race ./...`

Expected: PASS.

- [ ] **Step 3: Run all `tenantPanel` tests and build**

Run from `../tenantPanel`: `yarn test`

Run from `../tenantPanel`: `yarn build`

Expected: PASS.

- [ ] **Step 4: Run all `react-template` tests and build**

Run from `../react-template`: `yarn test`

Run from `../react-template`: `yarn build`

Expected: PASS.

- [ ] **Step 5: Verify repository scopes**

Run:

```bash
git status --short
git -C ../tenantPanel status --short
git -C ../react-template status --short
```

Confirm that pre-existing user changes remain present and no unrelated files were staged or committed.

- [ ] **Step 6: Perform a manual smoke test**

Create or preview a page with route `checklist/:id`, parent ID `{{route.id}}`, array field `duties`, identity `duty`, and a generated relation group for `locations`. Verify Add, Edit, Delete, a location switch, and optional order drag each change only the selected checklist and duty.

If verification exposes a defect, return to the responsible task, add a failing regression test, implement the smallest correction, rerun that task's focused and full verification, and commit the exact files named by that task.
