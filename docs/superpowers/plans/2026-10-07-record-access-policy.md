# Record Access Policy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the uncommitted route-personalization prototype with a generic, route-scoped record access policy that is enforced consistently for HTTP CRUD operations and workflow data steps, and expose the policy in tenantPanel.

**Architecture:** Add an optional `access` contract to each record-backed route, build one trusted execution context and one policy compiler/evaluator, and combine its predicates with request filters and existing `rowAccess` using MongoDB `$and`. Create assignments are reapplied around workflows, updates are checked before and after mutation and written with the same constrained selector, deletes are atomic, and workflows carry caller/system identity through outbox execution. tenantPanel edits the structured policy through a dedicated dialog and omits the property when no policy is configured.

**Tech Stack:** Go, Fiber, MongoDB Go driver, Go tests/mtest; React, TypeScript, Vite, Vitest, Testing Library, Yarn 4.

**Spec:** [Record Access Policy Design](../specs/2026-10-07-record-access-policy-design.md)

## Global Constraints

- Preserve unrelated user changes in both dirty worktrees. Replace only the current personalization prototype and extend the already-added current-user workflow support where it remains useful.
- Remove `isPersonalized`, `personalizedField`, `IsPersonalized`, `PersonalizedField`, `services/personalization.go`, and their terminology completely. Do not add compatibility aliases or migration behavior.
- Treat policy state as trusted server state. Never resolve `auth.*` from a request body, query value, workflow variable, or caller-provided map.
- Keep existing route activation, role authorization, field authorization, and container `rowAccess` cumulative with the new policy.
- Do not apply route access to aggregation/test-pipeline routes; reject policy configuration on them.
- Keep workflow template behavior such as `{{user.*}}` intact. `{{auth.*}}` is a policy expression namespace, not a general workflow-template namespace in this change.
- Prefer focused helpers and tests over adding more branching to `dynamic_service.go` and `dynamic_workflow.go`.
- Use `apply_patch` for manual edits and `gofmt -w` for Go formatting.

## Review Focus

Review every task against these failure cases:

1. **Cross-user cache leakage:** a protected route must never read or populate a shared response/item cache without identity/policy scoping.
2. **Mixed `any` semantics:** a true context-only rule removes the record restriction; an unavailable context value makes only that rule non-matching and must not suppress another usable rule.
3. **Update ownership transfer/races:** both the pre-update and post-`before_update` documents must pass, and the final write must reuse an access-constrained selector.
4. **Bulk partial outcomes:** one inaccessible/invalid item must follow existing per-item failure semantics without authorizing, rolling back, or exposing another item incorrectly.
5. **Workflow identity/bypass:** caller and system identities must survive nesting/outbox restoration, and no workflow repository call may bypass the target route policy.

---

### Task 1: Replace the backend route contract and validation

**Files:**
- Modify: `models/containerModel.go`
- Modify: `models/frontendValidation.go`
- Modify: `models/models_test.go`
- Modify: `controllers/containerController.go`
- Delete after replacement: `services/personalization.go`
- Delete after replacement: `services/personalization_test.go`

- [ ] **Step 1: Write model serialization tests for the new contract**

Add table-driven tests in `models/models_test.go` that marshal and unmarshal a route with:

```go
Access: &RecordAccessPolicy{
    Assign: map[string]interface{}{"ownerId": "{{auth.user.id}}"},
    Any: []RecordAccessRule{{
        Field: "ownerId", Operator: "eq", Value: "{{auth.user.id}}",
    }},
}
```

Assert the BSON/JSON keys are exactly `access`, `assign`, `any`, `field`, `context`, `operator`, and `value`, and assert the legacy keys never appear.

- [ ] **Step 2: Write validation tests before implementation**

Replace the personalization validation tests with `TestValidateRecordAccessPolicies` subtests covering:

- valid assignment-only create, rule-only create, and combined create policies;
- valid read/update/delete `any` policies;
- empty access object;
- assignment on a non-create route;
- assignment to `id`, `_id`, unknown, equation, and hashed fields;
- rule with neither/both `field` and `context`;
- unknown record field, unsupported operator, invalid `auth` path;
- scalar right-hand value for `in`/`nin`;
- access on pipeline/test-pipeline routes;
- access causing authentication to be required without mutating persisted `isAuthenticated`.

- [ ] **Step 3: Run the focused tests and confirm they fail**

Run:

```bash
go test ./models -run 'Test(RouteSpecAccess|ValidateRecordAccessPolicies)' -count=1
```

Expected: compile/test failure because the new policy types and validator do not exist.

- [ ] **Step 4: Add the policy model and validator**

In `models/containerModel.go`, replace the two prototype fields on `RouteSpec` with:

```go
Access *RecordAccessPolicy `bson:"access,omitempty" json:"access,omitempty"`
```

Add `RecordAccessPolicy` and `RecordAccessRule` using the approved shape. In `models/frontendValidation.go`, replace `ValidateRoutePersonalization` with `ValidateRecordAccessPolicies`, using schema field lookup and explicit allowlists for routes, operators, and `auth` context roots. Treat a whole-string `{{auth...}}` token as a context reference; literals remain allowed subject to field/operator compatibility.

- [ ] **Step 5: Wire validation into container create/update**

Change both controller call sites in `controllers/containerController.go` to invoke `models.ValidateRecordAccessPolicies`. Keep the existing validation response shape.

- [ ] **Step 6: Remove the prototype implementation**

Delete `services/personalization.go` and `services/personalization_test.go`, then remove all calls to `applyRoutePersonalization` and `finalizeRoutePersonalization`. Later tasks will add policy enforcement back test-first.

- [ ] **Step 7: Run and format the focused backend tests**

Run:

```bash
gofmt -w models/containerModel.go models/frontendValidation.go models/models_test.go controllers/containerController.go
go test ./models ./controllers -count=1
```

Expected: PASS, with no references to legacy personalization names in those packages.

- [ ] **Step 8: Commit the contract change**

```bash
git add models/containerModel.go models/frontendValidation.go models/models_test.go controllers/containerController.go
git commit -m "feat: define route record access policies"
```

### Task 2: Build the trusted context, evaluator, and MongoDB compiler

**Files:**
- Create: `services/record_access.go`
- Create: `services/record_access_test.go`
- Modify: `services/dynamic_service.go`
- Modify: `services/dynamic_workflow.go`
- Modify: `services/workflow_user_test.go`

- [ ] **Step 1: Write context and sanitization tests**

Cover a caller context containing tenant, project, schema, operation, identity kind, user ID/`_id`, role/roles, and nested sanitized user fields. Reuse and rename the existing recursive workflow-user sanitizer tests to prove hashed top-level and nested fields are absent and the repository document is not mutated. Add a system context test with no user.

- [ ] **Step 2: Write rule evaluation and compilation tests**

Add table-driven tests for `eq`, `ne`, `in`, and `nin` with strings, arrays, and ObjectIDs. Include these security cases:

- field rule compiles to the expected BSON predicate;
- multiple field rules compile under `$or`;
- a true context-only rule returns no record restriction;
- a false/missing context-only rule does not discard a usable record predicate;
- no true context rule and no usable field rule returns an explicit deny-all selector;
- missing assignment context returns an error;
- ObjectID record fields convert a resolved user ID before comparison;
- an invalid ObjectID fails closed rather than comparing the raw string.

- [ ] **Step 3: Run the new tests and confirm they fail**

Run:

```bash
go test ./services -run 'Test(RecordAccess|BuildAccess|ResolveAccess|SanitizeAccess)' -count=1
```

Expected: compile failure for the missing access engine.

- [ ] **Step 4: Implement focused access primitives**

In `services/record_access.go`, introduce unexported primitives with explicit inputs, for example:

```go
type accessIdentityKind string

const (
    accessIdentityCaller accessIdentityKind = "caller"
    accessIdentitySystem accessIdentityKind = "system"
)

type recordAccessContext struct {
    TenantID     string
    ProjectID    string
    Schema       string
    Operation    string
    IdentityKind accessIdentityKind
    UserID       string
    UserRole     string
    UserRoles    []string
    User         map[string]interface{}
}
```

Implement helpers to:

- build the trusted `auth` tree;
- resolve only complete `{{auth.*}}` references;
- normalize values based on the target field type;
- apply trusted assignments without mutating the caller map unexpectedly;
- evaluate `any` against a prepared record;
- compile record rules into BSON and distinguish unrestricted from deny-all;
- combine non-empty filters with `$and` without overwriting `$or` clauses.

Use stable sentinel values such as `bson.M{"_id": bson.M{"$exists": false}}` for deny-all rather than returning `nil`.

- [ ] **Step 5: Consolidate current-user loading**

Retain the useful prototype work in `dynamic_workflow.go`, but rename generic pieces away from workflow-only terminology where appropriate (`loadAccessUser`, `sanitizeAccessUser`). Keep workflow `CurrentUser` behavior compatible. Ensure the loader obtains the authentication container server-side and strips hashed fields recursively.

- [ ] **Step 6: Run the focused tests**

Run:

```bash
gofmt -w services/record_access.go services/record_access_test.go services/dynamic_service.go services/dynamic_workflow.go services/workflow_user_test.go
go test ./services -run 'Test(RecordAccess|BuildAccess|ResolveAccess|SanitizeAccess|Workflow.*CurrentUser)' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit the policy engine**

```bash
git add services/record_access.go services/record_access_test.go services/dynamic_service.go services/dynamic_workflow.go services/workflow_user_test.go
git commit -m "feat: add trusted record access engine"
```

### Task 3: Require HTTP identity and pass it through every protected input

**Files:**
- Modify: `middlewares/authenticate.go`
- Modify: `middlewares/middlewares_test.go`
- Modify: `controllers/dynamicController.go`
- Modify: `controllers/error_paths_test.go`
- Modify: `services/dynamic_service.go`

- [ ] **Step 1: Replace middleware prototype tests**

Rename the conditional-authentication cases to use `hasAccess`. Prove a route with `access != nil` requires a token even when `isAuthenticated` and `isAuthorized` are false, while inactive or unprotected public routes retain current behavior.

- [ ] **Step 2: Add controller input propagation tests**

Add or extend controller tests to prove all protected CRUD/read/export inputs receive the authenticated user ID, role/roles, and audit user/current-user source needed to construct the trusted context. Cover get-all, selection, get-one, and export because their current input structs lack user identity.

- [ ] **Step 3: Run focused tests and confirm failure**

Run:

```bash
go test ./middlewares ./controllers -run 'Test.*(ConditionalAuthentication|Access|Export|Selection)' -count=1
```

Expected: failures until the access flag and input fields are added.

- [ ] **Step 4: Update authentication routing**

In `middlewares/authenticate.go`, derive `hasAccess := route.Access != nil` and pass it to `conditionalAuthenticationRequiresToken`. Remove every personalization variable and comment.

- [ ] **Step 5: Add a common identity payload to service inputs**

Avoid duplicating identity fields further by introducing a small embedded input value, for example:

```go
type AccessIdentityInput struct {
    IdentityKind string
    UserID       string
    UserRole     string
    UserRoles    []string
    AuditUser    *models.AuditUser
}
```

Export constants for `caller` and `system`, then embed this value in every record-backed create/read/update/delete/export input. HTTP controllers set `caller`; cron/workflow code sets or preserves the appropriate kind. Keep existing public field names temporarily only where needed to minimize unrelated churn, but ensure the engine has one canonical identity source.

- [ ] **Step 6: Run the package tests**

Run:

```bash
gofmt -w middlewares/authenticate.go middlewares/middlewares_test.go controllers/dynamicController.go controllers/error_paths_test.go services/dynamic_service.go
go test ./middlewares ./controllers ./services -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit identity propagation**

```bash
git add middlewares/authenticate.go middlewares/middlewares_test.go controllers/dynamicController.go controllers/error_paths_test.go services/dynamic_service.go
git commit -m "feat: propagate route access identity"
```

### Task 4: Enforce trusted assignments and create policies

**Files:**
- Modify: `services/dynamic_service.go`
- Create: `services/record_access_create_test.go`

- [ ] **Step 1: Write single-create lifecycle tests**

Test the approved order: parse, assign, `before_create`, reassign, prepare/finalize, evaluate, insert. Specifically prove:

- caller-supplied `ownerId` is overwritten;
- a `before_create` workflow cannot spoof the assigned owner;
- string user ID becomes an ObjectID for an ObjectID schema field;
- assignment-only create succeeds;
- failed `any` returns 403 and inserts nothing;
- missing assignment reference returns a structured service error and inserts nothing.

- [ ] **Step 2: Write bulk-create partial-result tests**

Use at least two records: one passing and one failing final policy evaluation. Assert each item follows the service's existing bulk success/failure contract, no denied item is inserted, and the successful item retains trusted assignments.

- [ ] **Step 3: Run focused tests and confirm failure**

Run:

```bash
go test ./services -run 'Test(Create|BulkCreate).*RecordAccess' -count=1
```

Expected: tests fail because create services do not yet apply access.

- [ ] **Step 4: Implement create enforcement through shared helpers**

Update `CreateDynamicItem`, `CreateMultipleDynamicItems`, and their per-item helpers to use the same access context and assignment/evaluation helpers. Reapply assignments inside the transaction after `before_create` workflows and before insert. Do not duplicate policy logic in bulk loops.

- [ ] **Step 5: Run focused and package tests**

Run:

```bash
gofmt -w services/dynamic_service.go services/record_access_create_test.go
go test ./services -run 'Test(Create|BulkCreate).*RecordAccess' -count=1
go test ./services -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit create enforcement**

```bash
git add services/dynamic_service.go services/record_access_create_test.go
git commit -m "feat: enforce access policies on create"
```

### Task 5: Enforce access in all supported read queries and protect caches

**Files:**
- Modify: `services/dynamic_service.go`
- Create: `services/record_access_read_test.go`
- Modify: `controllers/dynamicController.go`

- [ ] **Step 1: Write query-composition tests**

Cover `GetAllDynamicItems`, `GetItemsForSelection`, `GetDynamicItem`, `SearchDynamicItems`, `FilterDynamicItems`, `GetAllDynamicItemsWithPagination`, and `ExportDynamicItems`. For each, assert the repository receives a MongoDB filter containing request filter/search clauses, existing `rowAccess`, and route `access` combined with `$and`.

For get-one, assert an inaccessible ID is queried with the combined selector and returns the existing not-found response, not an authorization response. For export, assert only permitted records reach workbook generation.

- [ ] **Step 2: Write cache-isolation regression tests**

Prime the item and paginated caches with one user's result, invoke the same URL/ID as a second user, and assert the second request cannot receive the first result. The initial implementation should assert protected routes skip shared cache reads and writes entirely; identity-scoped caching can be introduced later as a measured optimization.

- [ ] **Step 3: Write mixed-rule read tests**

Use an `any` policy with an owner field rule plus an admin context rule. Assert:

- ordinary user gets the owner predicate;
- admin gets no additional access predicate;
- system/no-user context does not make the admin rule error and still uses any viable field rule or deny-all;
- an all-unresolvable policy generates deny-all.

- [ ] **Step 4: Run focused tests and confirm failure**

Run:

```bash
go test ./services -run 'Test(Read|Get|Search|Filter|Selection|Export|Cache).*RecordAccess' -count=1
```

Expected: protected reads are currently unconstrained and cache regression tests fail.

- [ ] **Step 5: Add one read-filter composition path**

Create a helper that selects the route spec for the operation, constructs access context, compiles access, obtains existing `rowAccess`, and returns the combined filter. Call it before every repository query/count. Do not post-filter policy results. Explicitly leave `GetPipeline` and test-pipeline behavior unchanged.

- [ ] **Step 6: Disable shared caching for protected routes**

Make `shouldCache` false whenever the selected route has `Access != nil` for item and paginated/all read paths. Apply the decision before any cache lookup, wait, or set. Keep existing cache behavior for unprotected routes.

- [ ] **Step 7: Run focused and package tests**

Run:

```bash
gofmt -w services/dynamic_service.go services/record_access_read_test.go controllers/dynamicController.go
go test ./services -run 'Test(Read|Get|Search|Filter|Selection|Export|Cache).*RecordAccess' -count=1
go test ./services ./controllers -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit read enforcement**

```bash
git add services/dynamic_service.go services/record_access_read_test.go controllers/dynamicController.go
git commit -m "feat: enforce access policies on reads"
```

### Task 6: Make update and delete enforcement atomic

**Files:**
- Modify: `repositories/dynamic_repository.go`
- Modify: `repositories/dynamic_repository_test.go`
- Modify: `services/dynamic_service.go`
- Create: `services/record_access_mutation_test.go`

- [ ] **Step 1: Write repository selector tests**

Add tests for a new `DeleteByFilter` method and the existing `UpdateByFilter`, asserting the exact supplied filter reaches MongoDB unchanged and matched/deleted counts are returned. Keep `DeleteByID`/`UpdateByID` for unprotected internal uses if still required, but protected service paths must use the filter variants.

- [ ] **Step 2: Write single-update security tests**

Cover:

- existing record is fetched with ID + `rowAccess` + route access;
- inaccessible existing record returns not found;
- a user cannot transfer ownership through the request body;
- a `before_update` workflow cannot transfer ownership outside policy;
- an admin context rule can authorize a transfer because the final document still satisfies the admin rule;
- the write uses the same original constrained selector, so a concurrent ownership change results in matched count zero/not found.

- [ ] **Step 3: Write delete and bulk regression tests**

Assert delete uses one access-constrained database selector and never performs an unconstrained delete after an application check. For bulk update/delete, include one authorized and one unauthorized item and assert the existing per-item result shape, no record-existence disclosure, and no unauthorized mutation.

- [ ] **Step 4: Run focused tests and confirm failure**

Run:

```bash
go test ./repositories ./services -run 'Test.*(Update|Delete).*RecordAccess|TestDeleteByFilter' -count=1
```

Expected: failure because current protected paths fetch/write by ID alone.

- [ ] **Step 5: Add filtered repository deletion**

Implement:

```go
func (r *DynamicRepository) DeleteByFilter(
    ctx context.Context,
    tenantID, projectID, schemaName string,
    filter bson.M,
) (*mongo.DeleteResult, error)
```

Instrument it consistently with other repository operations.

- [ ] **Step 6: Refactor update enforcement**

Build the selector once from ID, row access, and route access. Fetch the existing record with that selector, prepare/merge, run `before_update`, evaluate the final document, then call `UpdateByFilter` with the same selector. Return not found when either fetch or final matched count shows no permitted row. Apply the same per-item helper in bulk update.

- [ ] **Step 7: Refactor delete enforcement**

Use the constrained selector for the final `DeleteByFilter`. Any reference checks may inspect only the already-authorized record/ID and must not weaken the final selector. Apply the same helper to bulk delete.

- [ ] **Step 8: Run focused and package tests**

Run:

```bash
gofmt -w repositories/dynamic_repository.go repositories/dynamic_repository_test.go services/dynamic_service.go services/record_access_mutation_test.go
go test ./repositories ./services -run 'Test.*(Update|Delete).*RecordAccess|TestDeleteByFilter' -count=1
go test ./repositories ./services -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit atomic mutation enforcement**

```bash
git add repositories/dynamic_repository.go repositories/dynamic_repository_test.go services/dynamic_service.go services/record_access_mutation_test.go
git commit -m "feat: enforce atomic record access mutations"
```

### Task 7: Route workflow data operations through the access engine

**Files:**
- Modify: `models/dynamicOutbox.go`
- Modify: `services/dynamic_workflow.go`
- Modify: `services/dynamic_cron.go`
- Modify: `services/dynamic_outbox_test.go`
- Modify: `services/workflow_user_test.go`
- Create: `services/record_access_workflow_test.go`

- [ ] **Step 1: Write identity persistence tests**

Extend outbox round-trip tests to assert `identityKind`, user ID, roles, and a sanitized `currentUser` survive enqueue and restore. Assert request/manual/nested workflows preserve `caller`, while cron-created payloads use `system` with no invented user.

- [ ] **Step 2: Write workflow policy tests**

For `create_record`, `update_record`, `delete_record`, `get_record`, `find_records`, `count_records`, `distinct`, and array mutation steps, prove the mapping in the spec:

- create uses `CreateDynamicModelItem` access;
- update/unset/array changes use `UpdateDynamicModelItem` access;
- delete uses `DeleteDynamicModelItem` access;
- get/find/count/distinct use `GetAllDynamicModelItems` access.

Include caller-owner success, caller denial, explicit system-rule success, and system denial. Assert workflow variables/config cannot inject `auth.*` values.

- [ ] **Step 3: Add a bypass regression test**

Configure a restrictive target route and execute each workflow data step. Fail the test if any step calls raw `Insert`, `UpdateMany`, collection `DeleteMany`, `FindOne`, `Count`, or `Distinct` without a compiled access predicate. This is the main Review Focus #5 guard.

- [ ] **Step 4: Run focused tests and confirm failure**

Run:

```bash
go test ./services -run 'Test(Workflow|Outbox|Cron).*RecordAccess|Test.*IdentityKind' -count=1
```

Expected: failures for missing identity kind and direct repository bypasses.

- [ ] **Step 5: Persist and restore identity kind**

Add `IdentityKind` and roles as needed to `DynamicOutboxPayload` and `workflowExecutionPayload`. Populate them when building outbox events and restore them when processing. Set cron payloads to system explicitly; never infer system merely because `UserID` is empty.

- [ ] **Step 6: Introduce workflow-safe service helpers**

Refactor shared service internals so workflow operations can pass an explicit `recordAccessContext` without Fiber. Use those helpers from HTTP services and workflows. For workflow multi-record update/delete, compile the access predicate into the operation filter; where final-document update evaluation cannot be represented safely for arbitrary operators, load permitted candidates and apply the same per-record atomic update helper rather than using unconstrained `UpdateMany`.

- [ ] **Step 7: Preserve workflow user features**

Keep existing `{{user.*}}` template and condition support and sanitized `CurrentUser` behavior. Do not expose hashed values or make the policy `auth` namespace mutable through workflow state.

- [ ] **Step 8: Run focused and service tests**

Run:

```bash
gofmt -w models/dynamicOutbox.go services/dynamic_workflow.go services/dynamic_cron.go services/dynamic_outbox_test.go services/workflow_user_test.go services/record_access_workflow_test.go
go test ./services -run 'Test(Workflow|Outbox|Cron).*RecordAccess|Test.*IdentityKind' -count=1
go test ./services -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit workflow enforcement**

```bash
git add models/dynamicOutbox.go services/dynamic_workflow.go services/dynamic_cron.go services/dynamic_outbox_test.go services/workflow_user_test.go services/record_access_workflow_test.go
git commit -m "feat: enforce record access in workflows"
```

### Task 8: Replace tenantPanel route types and normalization

**Repository:** `/Users/osmansamilerdogan/Desktop/tenantPanel`

**Files:**
- Modify: `src/utils/api/container.ts`
- Modify: `src/utils/containerRoutes.ts`
- Modify: `src/utils/containerRoutes.test.ts`
- Modify: `src/components/panelComponents/Modals/CreateContainerModal.tsx`
- Create: `src/components/panelComponents/Modals/CreateContainerModal.test.tsx`

- [ ] **Step 1: Write TypeScript normalization tests**

Replace personalization cases with tests that normalize camelCase and PascalCase policy payloads into one canonical shape, preserve literal values/arrays, omit `access` when absent, and do not emit either legacy field. Test immutable route updates and clean policy removal.

- [ ] **Step 2: Write new-container payload tests**

Assert all default routes omit `access`, `isPersonalized`, and `personalizedField` entirely.

- [ ] **Step 3: Run focused tests and confirm failure**

Run from `/Users/osmansamilerdogan/Desktop/tenantPanel`:

```bash
yarn test src/utils/containerRoutes.test.ts src/components/panelComponents/Modals/CreateContainerModal.test.tsx
```

Expected: failures until the contract is replaced.

- [ ] **Step 4: Define the UI contract**

Add `RecordAccessPolicy`, `RecordAccessRule`, and `AccessValue` types in `src/utils/api/container.ts`, and replace the two legacy `RouteSpec` properties with `access?: RecordAccessPolicy`.

- [ ] **Step 5: Normalize the new policy without legacy aliases**

Update `normalizeContainerRouteSpec` to accept Go/PascalCase serialization for `Access`, `Assign`, `Any`, `Field`, `Context`, `Operator`, and `Value`, while producing canonical camelCase. Do not recognize `isPersonalized` or `personalizedField`.

- [ ] **Step 6: Remove prototype defaults**

Remove legacy properties from `CreateContainerModal.tsx`; do not add empty `access` defaults.

- [ ] **Step 7: Run focused tests and build**

```bash
yarn test src/utils/containerRoutes.test.ts src/components/panelComponents/Modals/CreateContainerModal.test.tsx
yarn build
```

Expected: PASS.

- [ ] **Step 8: Commit the tenant contract**

```bash
git add src/utils/api/container.ts src/utils/containerRoutes.ts src/utils/containerRoutes.test.ts src/components/panelComponents/Modals/CreateContainerModal.tsx src/components/panelComponents/Modals/CreateContainerModal.test.tsx
git commit -m "feat: add route access policy contract"
```

### Task 9: Build and integrate the tenantPanel policy editor

**Repository:** `/Users/osmansamilerdogan/Desktop/tenantPanel`

**Files:**
- Create: `src/components/route-access/recordAccessPolicy.ts`
- Create: `src/components/route-access/recordAccessPolicy.test.ts`
- Create: `src/components/route-access/RouteAccessPolicyEditor.tsx`
- Create: `src/components/route-access/RouteAccessPolicyEditor.test.tsx`
- Modify: `src/components/RoutePermissions.tsx`
- Modify: `src/components/RoutePermissions.test.tsx`
- Modify: `src/common/CheckSwitch.tsx` only if its existing `disabled` prototype change is still needed

- [ ] **Step 1: Write pure helper tests**

In `recordAccessPolicy.test.ts`, cover:

- record-backed route eligibility and create-only assignment eligibility;
- exclusion of `_id`, `id`, equation, and hashed fields from assignment choices;
- context option lists from the approved `auth` namespace;
- operator/value validation, including arrays for `in`/`nin`;
- readable summaries for assignment and OR rules;
- empty/invalid policy rejection;
- clean removal returning `undefined` rather than `{}`.

- [ ] **Step 2: Write editor interaction tests**

Test opening with no policy, adding/removing assignments, adding record/context rules, switching operators, editing literals/context references, saving a valid policy, blocking an invalid policy, canceling without mutation, and removing an existing policy.

- [ ] **Step 3: Replace RoutePermissions prototype tests**

Assert the table shows `No policy` or a concise summary plus `Edit Policy`. In edit mode, assert saving updates only that route and forces `isAuthenticated: true`; while a policy exists, the route authentication switch is disabled and the bulk-authentication-off action leaves protected routes authenticated. After policy removal, authentication can be disabled normally.

- [ ] **Step 4: Run focused tests and confirm failure**

Run:

```bash
yarn test src/components/route-access/recordAccessPolicy.test.ts src/components/route-access/RouteAccessPolicyEditor.test.tsx src/components/RoutePermissions.test.tsx
```

Expected: compile/test failure because the editor and helpers do not exist.

- [ ] **Step 5: Implement pure policy helpers**

Keep route capability checks, field eligibility, validation, input parsing, and summary formatting outside the React component. Support literal strings/numbers/booleans and JSON arrays, plus selectable complete `{{auth.*}}` references. Do not offer raw arbitrary template paths.

- [ ] **Step 6: Implement the policy editor dialog**

Use the panel's existing modal/dialog primitives and accessible labels. Show Trusted assignments only for create/bulk-create routes. Show Allow when any rule matches for every supported record route. Work on local draft state and emit a normalized policy only on Save.

- [ ] **Step 7: Integrate with RoutePermissions**

Remove `PERSONALIZED_WRITE_ROUTES`, personalization fields/handlers/columns, and related terminology. Add the Access Policy column and editor state. Continue using the existing serialized update queue so rapid edits do not lose prior route changes.

- [ ] **Step 8: Run focused tests and build**

```bash
yarn test src/components/route-access/recordAccessPolicy.test.ts src/components/route-access/RouteAccessPolicyEditor.test.tsx src/components/RoutePermissions.test.tsx
yarn build
```

Expected: PASS.

- [ ] **Step 9: Commit the policy editor**

```bash
git add src/components/route-access src/components/RoutePermissions.tsx src/components/RoutePermissions.test.tsx src/common/CheckSwitch.tsx
git commit -m "feat: add route access policy editor"
```

### Task 10: Remove all legacy names and verify both repositories

**Repositories:**
- `/Users/osmansamilerdogan/Desktop/autotable-Go`
- `/Users/osmansamilerdogan/Desktop/tenantPanel`

**Files:**
- Modify only files revealed by the audits below

- [ ] **Step 1: Audit for forbidden legacy contract names**

Run in both repositories:

```bash
rg -n -i 'isPersonalized|personalizedField|route personalization|personalization' . --glob '!docs/superpowers/**' --glob '!node_modules/**' --glob '!dist/**'
```

Expected: no product-code or test matches. Historical design/plan prose may mention removal.

- [ ] **Step 2: Audit raw workflow repository operations**

Run in autotable-Go:

```bash
rg -n 'repository\.(Insert|InsertMany|UpdateByID|UpdateMany|DeleteByID|DeleteByFilter|FindByID|FindOne|FindAll|Query|Count|Distinct)|GetCollection\(.*\)\.(Delete|Update|Find)' services/dynamic_workflow.go
```

Review every result and ensure record CRUD/query steps pass a policy-composed selector or call the shared access-enforcing helper. Pipeline-only operations remain excluded by design.

- [ ] **Step 3: Run the full backend suite**

```bash
GOCACHE=/private/tmp/autotable-go-record-access-cache go test ./...
```

Expected: PASS.

- [ ] **Step 4: Run backend race tests for affected packages**

```bash
GOCACHE=/private/tmp/autotable-go-record-access-race-cache go test -race ./services ./repositories ./middlewares ./controllers ./models
```

Expected: PASS.

- [ ] **Step 5: Run the full tenant suite, lint, and build**

From `/Users/osmansamilerdogan/Desktop/tenantPanel`:

```bash
yarn test
yarn lint
yarn build
```

Expected: all tests and production build pass. If lint reports unrelated pre-existing failures, record them separately and prove no new failures originate in changed files.

- [ ] **Step 6: Inspect the final diffs and working trees**

In each repository, run:

```bash
git status --short
git diff --check
git diff --stat
```

Confirm only intended files changed, no legacy fields remain, no debug logging/secrets were added, and the design's eight security invariants are each represented by at least one automated test.

- [ ] **Step 7: Request code review before declaring completion**

Use the `superpowers:requesting-code-review` skill, providing the spec, this plan, both repository diffs, and the five Review Focus cases. Resolve findings with `superpowers:receiving-code-review`, then rerun the affected verification commands.

- [ ] **Step 8: Commit any verification fixes separately**

```bash
git add <only-files-changed-by-review>
git commit -m "fix: harden record access policy enforcement"
```

Do not create an empty commit if review required no changes.
