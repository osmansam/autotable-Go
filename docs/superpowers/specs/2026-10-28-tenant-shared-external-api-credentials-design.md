# Tenant-shared external API credentials and template creation

Date: 2026-10-28
Status: Approved by the user's “move on”; implemented and locally verified.
Repositories: `autotable-Go` and `tenantPanel`.

## Intent and agreed scope

The user wants to reuse an external API credential across projects and select or add the credential when creating a project from a template. The user selected **same-tenant sharing with explicit project access**. Cross-tenant/platform-managed credentials are excluded.

This concerns outbound external API keys used by workflow `call_api` steps. It does not change inbound AutoTable integration tokens, provider billing/credit allocation, or Ozmishow.

## Current behavior and constraints

- `models.ExternalAPICredential` stores an encrypted secret with tenant and project IDs in the global `external_api_credentials` collection.
- Credential creation/list/revocation are project-management endpoints. `GetExternalAPICredential` requires the credential ID, tenant ID and project ID to match.
- `call_api` reads a literal `config.credentialId`, resolves it for the executing project, checks expiry/revocation and allowed hosts, and decrypts it server-side.
- Templates may be tenant-local or global. Project cloning copies containers and workflows without copying credentials or remapping their references.
- Project creation runs under a tenant-management identity. Project management permissions do not automatically imply tenant privileges.
- Deleting a project removes project-tagged credential records. Shared ownership must not remain tied to the originating project.

## Approaches

1. **Tenant-owned credential plus explicit project grants — selected direction.** One encrypted key can serve several authorized projects; each execution checks the grant. This implements the user's chosen tenant boundary without duplicating secrets.
2. **Copy credentials into every project.** This retains existing ownership but duplicates secrets and makes shared revocation and updates inconsistent. Rejected because the user wants one reusable credential.
3. **Platform-owned, cross-tenant integrations.** This requires a different ownership and authorization model. Excluded by the user's answer.

The storage, API and UI details below are implementation proposals, not additional decisions attributed to the user.

## Credential ownership and grants

Keep the existing credential collection and IDs. Add an explicit `scope` of `project` or `tenant`; absent scope means legacy `project`. Tenant credentials have no owning `projectId`. Preserve existing encryption, auth types, allowed-domain validation, expiration and revocation fields.

Use `external_api_credential_grants` records containing tenant ID, credential ID, project ID, grantor and creation time. Enforce uniqueness on `(tenantId, credentialId, projectId)` and indexes for runtime lookup and project cleanup. Grants are references, never secret copies.

Runtime resolution permits either:

- An existing project credential belonging to the exact executing tenant/project; or
- A tenant credential belonging to the executing tenant with a matching project grant.

No missing scope, malformed ID, missing grant or ownership mismatch broadens access. The workflow still rejects revoked/expired credentials, disallowed destinations and unsafe redirects. Do not cache positive grant/credential authorization across executions; disconnection or revocation must affect subsequent outbound calls. A request already sent cannot be recalled.

## Management permissions and API

Add tenant integration read/manage permissions to the existing management permission matrix. For this feature both are restricted to active tenant owners/admins. All mutations validate live membership and ownership server-side; client project/tenant IDs never establish authority.

Provide tenant-management endpoints under `/api/v1/tenant/integrations/external-api-credentials` for listing metadata, creating, revoking, and managing individual project grants. Use an idempotent grant operation and a separate disconnect operation. Validate that grant targets are active projects in the same tenant. The implementation plan will fix exact route suffixes and contracts before coding.

Provide an explicit tenant-admin action to promote an existing project credential to tenant scope **without changing its credential ID or secret**. Atomically remove project ownership and grant the original project access, so existing workflow references continue working. Promotion must not leave an intermediate state that breaks the original project's access. It is not automatic and cannot be requested using only project-management authority.

Existing project credential endpoints remain compatible. Project listings include legacy project credentials and granted tenant credentials, with scope and usable/revoked/expired status. Project-only credentials keep their existing revoke behavior. A project endpoint must never globally revoke or mutate a tenant credential. Tenant admins manage shared grants through the tenant endpoints; project-only users cannot grant themselves or other projects access.

Metadata responses never include plaintext or ciphertext. Global revocation of a shared credential affects every connected project. Disconnecting a project affects only that project's use. Secret rotation/editing is outside this change; the design must not imply an unimplemented rotation control.

## Template requirements and binding

Expose a tenant-authorized template connection-requirements endpoint. Discover every literal `credentialId` in `call_api` steps, including inactive workflows/steps and nested `steps`, `elseSteps`, and branch steps. Deduplicate repeated references while retaining where each is used. Do not rewrite arbitrary strings, headers or unrelated IDs.

Return safe requirement labels, workflow/step usage and static destination hosts where available. A global template from another tenant must not reveal that tenant's credential metadata or confer access to its key; use generic labels when credential metadata is not owned by the requesting tenant.

Extend project creation with:

- An optional list of tenant credential IDs to connect to the new project, including credentials not currently referenced by workflows.
- Explicit bindings from each source template credential reference to a selected tenant credential owned by the target tenant.

Every copied credential reference requires a binding. The UI can preselect the same tenant credential for an eligible shared reference, but access is granted only when the user submits creation. Project-only source credentials require promotion or replacement; their secrets are never silently copied. Cross-tenant/global templates always bind to credentials owned by the target tenant.

Validate bindings, tenant ownership, non-revoked/non-expired state, duplicates, and template requirements before inserting a new project. Reject missing or unknown bindings with actionable errors. Validate selected credentials against statically known destination hosts; templated/dynamic destinations retain execution-time host checks. No user-provided credential ID bypasses ownership checks.

During creation, grant the selected tenant credentials to the new project and rewrite only the discovered workflow credential references before containers are persisted. Preserve unrelated workflow configuration and ordinary template data copying. A failed create must remove any partial new-project grants and project resources, while preserving pre-existing shared credentials. Duplicate grants are harmless. Template credentials and the source project's grants remain untouched.

## Tenant Panel

Add a tenant-level External APIs management view, available without selecting a project. Tenant owners/admins can create a shared key, see connected projects, connect/disconnect projects, revoke the shared key, and explicitly promote an existing project key. Reuse the existing external credential form and validation where practical.

The project Integrations view shows each connected credential's scope and status. Shared keys clearly indicate tenant ownership; project-only controls cannot perform tenant-wide revocation.

In Create Project, provide an External APIs section for selecting shared credentials. When a template is selected, show each required connection and its destination/usage, with a credential selector. Changing templates clears stale bindings. Support **Add new credential** using the existing name/auth/header/secret/domain/expiry fields.

Proposed UI behavior: adding a new key explicitly saves it to the tenant credential store first and then selects it. If project creation fails, retain the selected key and form values so the user can retry. Explain that the saved key remains in the tenant store; never delete an existing/shared key during project rollback. The project-create payload contains IDs/bindings, not secret values.

Show precise failures for missing connections, insufficient access, revoked keys and invalid host permissions. Disable repeat submission while saving. Do not add a separate client-side secret persistence mechanism.

## Lifecycle and compatibility

- Existing workflows and project credentials work without a migration or changed IDs.
- Existing clients creating projects without external credential requirements retain their current contract.
- A template with credential references now requires explicit binding rather than creating workflows with unusable inherited credentials. Document this intentional validation change.
- Project deletion removes its grants and project-owned keys, but never tenant-owned credentials or other projects' access.
- Tenant credentials stay tenant-owned after promotion, even if the originating project is deleted.
- Revoked credentials remain visible as metadata for diagnosis; granting/binding them is rejected.
- No hosted migrations, provider requests, production data changes or deployments are included in implementation verification.

## Verification and acceptance

Backend tests must cover legacy ownership, tenant grants, denied cross-tenant/cross-project use, revoked/expired credentials, host restrictions, permission/route contracts, redaction, grant uniqueness, promotion atomicity, nested template binding, and cleanup after failed creation or project deletion.

Use the existing disposable Mongo integration patterns for grant/promotion/clone state transitions and failures. Runtime tests must demonstrate an actual local outbound call from two separately authorized projects using the same credential, and rejection from an ungranted third project. Disconnecting one project must leave the other working; tenant revocation must block both. No external provider or production key is required.

Tenant Panel tests cover tenant permissions, credential scope display, grants/revocation, promotion, selecting a template, requirement mapping, adding a new key, changing templates, actionable errors and retry behavior. Run relevant type/lint/build checks and the affected backend suites; complete broader required repository checks after focused verification passes.

Acceptance example: a tenant admin creates one provider credential, connects project A, creates project B from a template with the same credential selected, and both workflows succeed against a local provider fixture. Project C cannot use it without a grant. Deleting A leaves B working. Restoring neither templates nor projects can expose or duplicate the secret.

## Builder discretion

The implementation plan may choose concrete helper/file names, exact route suffixes, pagination within existing conventions, and UI component placement. It must preserve the stated ownership, live authorization, cloning and cleanup guarantees. Any requirement for cross-tenant sharing, automatic access to every project, provider billing controls or changing secret encryption returns to design review.
