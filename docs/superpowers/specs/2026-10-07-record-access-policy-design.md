# Record Access Policy Design

**Date:** 2026-10-07

**Status:** Approved

**Repositories:** `autotable-Go`, `tenantPanel`

## Purpose

Replace the route-level `isPersonalized` and `personalizedField` prototype with a general record access policy. The new model must support identity-aware record authorization without trusting caller-supplied identity fields, and it must be enforced consistently for HTTP routes and workflow data operations.

This is a clean contract replacement. The old personalization fields will be removed rather than retained as aliases.

## Goals

- Express route-specific record access as declarative `any` rules.
- Allow create routes to assign trusted values such as the authenticated user ID.
- Compile record rules into MongoDB selectors wherever possible.
- Prevent check-then-write authorization races for updates and deletes.
- Apply the same policy evaluator to REST and workflow operations.
- Distinguish caller and system workflow identities without giving system work an implicit bypass.
- Provide a tenant-panel editor for policies without exposing raw JSON as the primary interface.
- Preserve existing route authentication, role authorization, field authorization, and container-level `rowAccess` behavior.

## Non-goals

- Reusable named policy resources.
- Arbitrary policy scripts or user-provided executable code.
- Policy enforcement on arbitrary aggregation pipelines or test-pipeline routes.
- An implicit administrator or system bypass.
- Backward compatibility for `isPersonalized` or `personalizedField`.
- A migration utility for persisted prototype configurations.

## Route Contract

Record-backed route specifications gain an optional `access` property:

```json
{
  "access": {
    "assign": {
      "ownerId": "{{auth.user.id}}"
    },
    "any": [
      {
        "field": "ownerId",
        "operator": "eq",
        "value": "{{auth.user.id}}"
      },
      {
        "context": "{{auth.user.role}}",
        "operator": "eq",
        "value": "admin"
      }
    ]
  }
}
```

The Go model is conceptually:

```go
type RouteSpec struct {
    // Existing route properties...
    Access *RecordAccessPolicy `bson:"access,omitempty" json:"access,omitempty"`
}

type RecordAccessPolicy struct {
    Assign map[string]interface{} `bson:"assign,omitempty" json:"assign,omitempty"`
    Any    []RecordAccessRule     `bson:"any,omitempty" json:"any,omitempty"`
}

type RecordAccessRule struct {
    Field    string      `bson:"field,omitempty" json:"field,omitempty"`
    Context  string      `bson:"context,omitempty" json:"context,omitempty"`
    Operator string      `bson:"operator" json:"operator"`
    Value    interface{} `bson:"value" json:"value"`
}
```

Each rule has exactly one left-hand source:

- `field`: a field on the target record.
- `context`: a trusted execution-context expression.

`value` is the right-hand operand and may be a literal, array, or trusted context expression.

Initially supported operators are:

- `eq`
- `ne`
- `in`
- `nin`

An `access` object is an allow policy. When `any` is non-empty, access is denied unless at least one rule matches. An empty `access` object is invalid; an assignment-only policy is valid only on create routes.

### Trusted assignments

`assign` is supported only on create and bulk-create routes. It overwrites client-provided values before validation and workflow execution. Assigned values are finalized to the target schema type before persistence, including conversion of valid user ID strings to MongoDB `ObjectID` fields.

A create policy may contain only `assign`, only `any`, or both. When `any` is present, the final prepared record must satisfy at least one rule. Missing values required by an assignment fail closed.

Update routes do not use `assign`. Instead, the policy is checked against both the existing record and the final record after `before_update` workflows. This prevents an authorized user from changing an ownership field so the final document no longer satisfies the policy, while still permitting a separately authorized administrator rule to do so.

## Execution Context

The policy engine receives a trusted context created by the server:

```text
tenant.id
project.id
schema
operation
identity.kind
user.id
user._id
user.role
user.roles
user.<nested field>
```

Template references use the `auth` namespace:

```text
{{auth.tenant.id}}
{{auth.project.id}}
{{auth.schema}}
{{auth.operation}}
{{auth.identity.kind}}
{{auth.user.id}}
{{auth.user._id}}
{{auth.user.role}}
{{auth.user.roles}}
{{auth.user.profile.departmentId}}
```

The user document is loaded from the project's authentication container and recursively stripped of hashed fields before it enters the execution context. Client payloads, query parameters, and workflow variables cannot override the `auth` namespace.

Missing context references make an individual `any` rule non-matching. This allows, for example, a system rule to match even though no human user exists. Missing assignment references are execution errors because persisting an unresolved trusted assignment would be unsafe.

## Identity Kinds

The first implementation supports:

- `caller`: an HTTP caller or a workflow inheriting an initiating caller.
- `system`: scheduled work with no human caller.

System identity is not privileged by default. A policy must explicitly allow it:

```json
{
  "context": "{{auth.identity.kind}}",
  "operator": "eq",
  "value": "system"
}
```

Manual workflows and request-triggered workflows inherit caller identity. Scheduled workflows use system identity. Nested and outbox workflow steps preserve the originating identity kind and sanitized user snapshot. A future `service` identity may be added without changing the policy shape.

## Authentication and Existing Authorization

For external HTTP routes, the presence of `access` requires authentication even if `isAuthenticated` is false. Existing route checks remain in force:

1. Route activation.
2. Authentication when explicitly enabled or implied by access.
3. Existing role authorization.
4. Record access policy.
5. Existing field-level and container-level row access controls.

These layers are cumulative. Record access does not replace role or field authorization.

## Policy Validation

Container create and update reject invalid policies before persistence.

Validation includes:

- `access` must contain at least one assignment or rule.
- `assign` is allowed only on create and bulk-create routes.
- Assigned fields must exist and cannot be `id`, `_id`, equation fields, or hashed fields.
- Assigned values must be resolvable trusted-context expressions or supported literals compatible with the field type.
- `any` must not contain empty rules.
- Each rule must specify exactly one of `field` or `context`.
- Record fields must exist.
- Operators must be one of `eq`, `ne`, `in`, or `nin`.
- Context expressions must use an allowed `auth` path.
- `in` and `nin` must receive an array-compatible right-hand value.
- Record access is rejected for pipeline and test-pipeline routes.

No `isPersonalized` or `personalizedField` fields remain in backend or tenant-panel models.

## Policy Compilation

The engine separates context-only decisions from record predicates.

For each rule in `any`:

- A context rule is evaluated immediately.
- A record-field rule is resolved into a MongoDB predicate when its right-hand value is available.

The resulting allow behavior is:

- If any context-only rule is true, the route policy contributes no record restriction.
- Otherwise, record predicates are combined with `$or`.
- If there are no matching context rules and no usable record predicates, the engine produces a deny-all selector.

Container-level `rowAccess` and request filters are combined with the route policy using `$and`.

Schema field types guide value normalization. For example, a rule comparing an `objectId` field to `{{auth.user.id}}` converts the resolved string to `primitive.ObjectID` before building the MongoDB filter.

## CRUD Enforcement

### Create and bulk create

1. Parse input.
2. Apply trusted assignments, overwriting caller values.
3. Run `before_create` workflows.
4. Reapply trusted assignments.
5. Prepare and validate the record.
6. Finalize assignment types.
7. Evaluate `any` against the final record and context.
8. Insert.

Each bulk item is evaluated independently. A failed item follows the operation's existing bulk error semantics.

### Read routes

Record access is compiled into the MongoDB query for:

- Get one.
- Get all.
- Paginated get.
- Search.
- Filter.
- Selection data.
- Export.

The policy is never implemented as post-query filtering. An unauthorized single-record lookup returns not found; collection reads return only matching rows.

### Update and bulk update

1. Build an atomic selector combining record ID, tenant/project scope, row access, and route access.
2. Fetch the existing record using that selector so unauthorized records are not exposed.
3. Merge and prepare the update.
4. Run `before_update` workflows.
5. Evaluate the final document against the policy.
6. Persist using the same access-constrained selector.

No match returns not found, avoiding record-existence disclosure. Bulk items are independently constrained.

### Delete and bulk delete

Delete selectors combine IDs, row access, and route access in the database operation. Unauthorized IDs behave as not found and are never deleted after an application-only ownership check.

## Workflow Enforcement

Workflow data steps use the same service-level policy engine and target-container route configuration:

| Workflow operation | Target route policy |
| --- | --- |
| Create record | `CreateDynamicModelItem` |
| Update record | `UpdateDynamicModelItem` |
| Delete record | `DeleteDynamicModelItem` |
| Find/count/distinct records | `GetAllDynamicModelItems` |

Workflow code must not call a repository mutation that bypasses policy enforcement. Internal helpers accept the execution context explicitly.

Outbox events persist:

- Identity kind.
- User ID and roles.
- Sanitized current-user snapshot.
- Tenant, project, schema, and operation data needed to rebuild context.

Protected workflow steps without an allowed identity fail closed and produce the existing workflow/outbox error and audit records.

## Tenant Panel

The route-permissions table replaces the prototype Personalized and Personalized Field columns with an Access Policy column.

Record-backed CRUD routes show:

- A concise summary when a policy exists.
- `No policy` when absent.
- An Edit Policy action in edit mode.

The policy editor contains two sections.

### Trusted assignments

Available only for create and bulk-create routes:

- Target schema field.
- Literal or allowed `auth` context value.
- Add/remove assignment controls.

Reserved, equation, and hashed fields are excluded.

### Allow when any rule matches

Each rule provides:

- Source type: Record field or Execution context.
- Field/context selector.
- Operator: `eq`, `ne`, `in`, or `nin`.
- Literal or context-aware comparison value.
- Add/remove rule controls.

The editor displays a readable summary such as:

```text
ownerId equals current user OR user role equals admin
```

The UI blocks empty or invalid policies, but backend validation remains authoritative. Existing route authentication and role controls remain visible. When `access` exists, authentication is shown as required and cannot be disabled until the policy is removed.

New-container defaults omit `access` entirely.

## Error Behavior

- External request lacks required authentication: `401 Unauthorized`.
- Authenticated create fails its policy: `403 Forbidden`.
- Read/update/delete selector finds no permitted record: existing not-found response.
- Invalid stored or submitted policy configuration: container create/update validation error.
- Missing trusted assignment value: fail closed with a structured service error.
- Workflow denial: workflow step error, captured by existing transactional or outbox behavior.

Error messages must not reveal protected record contents or whether an inaccessible ID exists.

## Migration

This change replaces an uncommitted prototype in both repositories:

- Remove `IsPersonalized` and `PersonalizedField` from the Go route model.
- Remove personalization-specific middleware, validation, service helpers, tests, and terminology.
- Replace them with `Access`, policy validation, execution context, compiler, and enforcement tests.
- Remove `isPersonalized` and `personalizedField` from tenant-panel models and normalization.
- Replace the personalization table controls and tests with the access-policy editor.
- Remove prototype defaults from new-container payloads; access remains omitted until configured.

Because compatibility was explicitly declined, persisted documents using the prototype fields are not interpreted by the new engine.

## Testing Strategy

Backend tests cover:

- BSON/JSON model round trips.
- Policy validation and invalid reference/operator combinations.
- Context resolution and hashed-field exclusion.
- `eq`, `ne`, `in`, and `nin` evaluation.
- MongoDB filter compilation, OR behavior, context short-circuit, and deny-all behavior.
- ObjectID normalization.
- Trusted create assignments and spoofed-client-value replacement.
- Final-record update evaluation.
- Atomic single and bulk update/delete selectors.
- Read, search, filter, selection, and export query composition.
- Interaction with existing row access and role authorization.
- Caller/system workflow behavior and outbox restoration.

Tenant-panel tests cover:

- CamelCase and PascalCase normalization.
- Route eligibility.
- Creating, editing, summarizing, and removing policies.
- Assignment field filtering.
- Rule validation and operator-specific value controls.
- Authentication locking while access exists.
- Saved container payload shape.
- Legacy route objects remaining unchanged when no access policy exists.

Both repositories must pass their full test suites and production builds before completion.

## Security Invariants

1. The client cannot provide or override `auth` context values.
2. Hashed user fields never enter policy or workflow context.
3. System identity has no implicit access.
4. Record policies are applied in database selectors for reads, updates, and deletes.
5. Updates must satisfy policy before and after mutation.
6. Workflow operations cannot bypass the policy path used by HTTP routes.
7. Missing identity data fails closed.
8. Access-denied responses do not disclose record existence.
