# Tenant Panel project collaboration

Status: revision 2 approved by the user on 2026-11-17 ("ok move on"); implementation planning in progress.
Date: 2026-11-17.
Repositories: `autotable-Go` and sibling `tenantPanel`.

## Intent and agreed scope

Let a business invite teammates and external contributors into particular AutoTable projects with different access levels. A collaborator can develop one project, view another, and have no visibility into a third.

The user selected the first release scope: **Tenant Panel and visual builders, using the existing fixed project roles**. Published-application users remain a separate identity and permission context.

## Approaches

1. **Extend existing management memberships and fixed project roles.** Reuses live backend authorization, management sessions, and frontend permission helpers. Delivers predictable permissions with limited configuration. This is the selected approach.
2. **Deliver panel collaboration and published-application collaboration together.** Supports more user journeys but requires separate runtime policies and onboarding, increasing scope and risk. Deferred.

Custom roles, per-person permission overrides, delegated invitation flags, team/group inheritance, billing seats, and simultaneous editing/conflict resolution are also deferred.

## Existing behavior and constraints

- `models/tenantModel.go` defines tenant/project memberships, an invitation model, and project admin/developer/editor/viewer/support roles.
- `services/management_authorization.go` checks live active user, tenant, tenant membership, project, and project membership. Token role claims do not establish permission.
- `routes/management_contracts.go` declares approved identity/scope/permission boundaries. New management routes must use this system.
- `services/auth_session_service.go` exposes project session revocation hooks.
- `controllers/projectController.go:GetAllProjects` currently lists all projects in the tenant. Existing project read permissions have no ordinary tenant member role.
- `TenantRegister` creates an account AND a tenant. `TenantLogin` requires an active tenant membership. Invited newcomers cannot simply use that registration route.
- Frontend management permissions mirror backend role mappings; navigation and project/template fetching also need to work for ordinary members.
- The inspected management account model has no email-verification state, and no email transport was found in the Go source. Both need explicit support for safe email invitations.
- Existing project editor permissions include page and localization editing. This release preserves that behavior.
- The existing `containsAnyRole` authorization behavior permits any recognized role within the selected membership scope. It does not combine tenant roles with project roles. This behavior must be made explicit, not replaced by a numeric role hierarchy.
- Existing project creation, creation rollback, update/deactivation, and deletion paths write authorization-relevant state outside the proposed collaboration service. They must be included in the concurrency integration audit.

## Roles and access boundaries

Keep the existing project role keys. New invitations choose exactly one of these four roles; existing multi-role memberships and support memberships remain valid.

| Role | Management access in this release |
| --- | --- |
| Project Admin | Existing admin configuration permissions plus project member/invitation management |
| Developer | Existing schema/API/workflow/page/integration development permissions; no membership administration |
| Editor | Existing page and translation editing; no schema or membership administration |
| Viewer | Existing permitted read operations; no management writes or membership administration |

Do not silently broaden existing schema delete, project delete, audit, integration, or credential permissions. Existing destructive operations retain their current role restrictions. Read operations must not return raw secrets or invitation token hashes.

Introduce `tenant_member` as a basic tenant role for project collaborators. It permits account/session access and a list of assigned projects, without tenant configuration, billing, member administration, project creation, or integration access. Reusing billing/auditor/admin roles for ordinary collaborators would grant unrelated permissions.

Tenant owner/admin can administer project memberships through tenant-scoped, target-project endpoints. This is an explicit membership-management capability, not automatic access to project configuration. Other tenant roles retain their existing privileges. Project admin can manage members only in their own project, including assigning another project admin. No project invitation can grant a tenant role.

An active tenant membership is required for project access. Accepting an invitation creates a basic tenant membership only if none exists; it must not downgrade or overwrite existing tenant roles, or reactivate a disabled tenant membership.

### Effective permission resolution

Extend the existing `ManagementAuthorizationService` as the single management permission resolver. Route contracts choose the permission and target; controllers and invitation acceptance invoke the same policy using current repository state. The frontend mirrors its results for presentation only.

Apply these rules in order:

1. Deny an unknown permission, malformed/cross-tenant target, inactive user/tenant, or absent/disabled tenant membership before evaluating any role. A project target must also exist in the tenant and be active. Disabled status wins over all roles; fail closed when live state cannot be loaded.
2. For a tenant permission, evaluate only the active tenant membership's recognized tenant roles. For a project permission, additionally require an active membership in that exact project and evaluate only its recognized project roles. Never combine tenant and project role arrays or treat tenant ownership as project membership.
3. Within one membership, preserve existing additive semantics: a permission is allowed if at least one recognized role is in that permission's allowlist. There is no "highest role wins" ordering; in particular, Support is not a rank below/above Editor or Developer. Unknown role keys grant nothing and are preserved for explicit migration review.
4. Account/session reads require active identity/membership eligibility, independently of tenant-wide project-list privileges. Basic member project discovery and direct project details require active project membership. When a membership also contains a privileged tenant role, that role retains only the explicitly listed tenant capabilities below.
5. Token role claims, `Tenant.OwnerUserID`, client state, a matching email, and runtime application roles do not supply missing management capabilities. `OwnerUserID` is organizational metadata, not a permission bypass.

Enumerated tenant-level capabilities and exceptions:

| Tenant roles | Tenant-wide capabilities retained or added |
| --- | --- |
| Owner, Admin, Billing, Auditor | Existing tenant-wide project list/detail and tenant branding read |
| Owner, Admin | Existing project creation/update, tenant branding management, and tenant integration read/manage |
| Owner | Existing project deletion |
| Owner, Admin | New target-project member/invitation administration and admin recovery, without project configuration access |
| Member | Own account/session and assigned-project discovery/detail only |

No other privileged tenant-to-project exceptions are introduced. Existing per-project allowlists in `managementPermissionRoles` remain the source of truth, including narrower deletion, credential, integration, and audit restrictions. Unknown tenant roles alone do not qualify for account access. Add table-driven tests for each tenant role, all project roles, mixed tenant/project roles, legacy multi-role combinations, and disabled/missing entities.

## Member administration

Add a project Members area using existing panel components, with current collaborators and pending invitations. Authorized administrators can invite, resend, cancel, change a member's project role, and remove membership. Other project roles do not receive the member directory or invitation list in this first release.

Show the role's actual allowed actions when selecting it. Do not describe Editor as records-only. Published-app data access continues to depend on its existing application identity, row, field, and workflow rules; project membership does not create a runtime identity or bypass those rules.

Removing project access leaves other projects and the tenant membership intact. A basic member with no projects sees an empty project list. Do not add tenant-wide member removal or ownership transfer to this release.

Protect the last active project admin against removal/demotion and reject self-demotion/removal when it would leave no project admin. Existing tenant ownership is never changed by these endpoints. Administrators can add a replacement admin before removing the previous one.

For an existing membership with multiple roles, Support, or unknown roles, the ordinary single-role update endpoint must return a conflict rather than replace the role array. Display the complete current set and preserve it until an administrator explicitly requests conversion to one of the four supported invitation roles. Conversion must include the expected current membership version and full role set, `convertLegacyRoleSet: true`, and confirmation showing every role to be removed. Check the same live administrative authority, last-admin safeguard, audit, and session-revocation requirements as any role mutation; reject stale versions. Listing, resending invitations, or editing unrelated metadata never converts roles. New invitations cannot merge or replace an existing role set, including a disabled project membership; an administrator must handle that existing membership explicitly.

Tenant Owner/Admin has an explicit recovery operation through the tenant-scoped member administration API: appoint a replacement Project Admin from an active, verified tenant member even when the project has zero usable admins. It grants only that target's project membership, records a recovery-specific audit event/reason, and does not give the caller project configuration access. If the replacement is not yet a tenant member, use the ordinary secure invitation flow. Count a "usable" project admin only when user, tenant membership, project membership, and project are active and its role set contains Project Admin. Disabling a user/tenant membership through an operational or future administrative path must either arrange replacement admins in affected active projects or explicitly deactivate those projects in the same guarded lifecycle operation.

## Invitation lifecycle and onboarding

1. An authorized administrator selects a project, email, and allowed project role.
2. Normalize email consistently with management-account lookup, validate role/scope, check active entities and caller authority, and reject an already active project member with a clear instruction to use role management.
3. Generate at least 32 cryptographically random bytes. The invitation record contains only a SHA-256 token hash, a token version, and scoped invitation metadata. Proposed lifetime: seven days. A delivery outbox stores a short-lived encrypted copy of the secret as described below, never plaintext.
4. Commit the invitation, audit event, and delivery job together. Return a queued state, not a sent state. A worker sends through a configurable server-side SMTP transport with required TLS and a configured sender/panel base URL. A delivery failure is visible and retryable. No tokens or passwords enter logs or audit payloads.
5. The invitation link opens a public join page with the secret in the URL fragment. Exchange it in a protected POST for a server-side continuation, then clear the fragment and raw secret. Preview returns only the minimal tenant/project name, offered role, and expiry needed for consent, without revealing account existence. Apply no-store/referrer controls and exclude the join page from analytics.
6. A new user enters name and password and joins the existing tenant/project without creating a tenant. An existing user authenticates with the invited account. A mismatched signed-in account is asked to switch accounts; acceptance never silently changes accounts.
7. **Existing-account credentials and mailbox possession are separate proofs.** For every existing account, require fresh password authentication for that account (within five minutes of acceptance) plus possession of the current emailed invitation. A stored email match or an old session alone is insufficient. For legacy unverified accounts, issue an independent purpose-bound email-verification challenge after that reauthentication; completing it verifies the email but never changes credentials. A project invitation/continuation cannot reset a password, create a login session for an existing account by itself, attach an identity provider, replace an email, or revoke sessions across unrelated tenants. If the person cannot authenticate, stop and direct them to a separate account-recovery/support process with its own ownership checks; invitation APIs cannot substitute for recovery. Existing passwordless management accounts likewise require an independently supported account-authentication method; runtime Google login is not such a method. New-account creation is permitted only when no normalized management account already exists, uses a new password, and verifies mailbox possession through the emailed invitation. If an account appears concurrently, reject creation and require the existing-account path; do not link or overwrite it.
8. In a MongoDB transaction, recheck expiry, pending status, email/account match, active tenant/project, and the inviter's current grant authority; establish memberships, consume the invitation, and write audit events atomically. Concurrent acceptance must yield one grant only. If another operation already granted membership, do not overwrite its role with the invitation's older role.
9. Issue fresh management sessions only after commit using the existing secure cookie machinery. Refresh frontend session context and open the accepted project. Existing users invited into another tenant must receive a real session for the target tenant; changing local storage alone does not switch tenants.

Invitations have their own states: pending, accepted, revoked, and expired. Expiry is checked at read and acceptance time; do not depend on TTL deletion. Retain terminal records for audit. Delivery status is separate from invitation state.

Resend rotates the secret and expiry, invalidating older links. Do not expose a previously stored raw token. Permit only one pending invitation per normalized tenant/project/email. Rotation and delivery failure must leave a clearly retryable state, not a misleading successful send. Revoking or losing the inviter's authority prevents acceptance; another authorized admin can replace the invitation under their own authority.

Invite token possession proves mailbox ownership only when delivered through the configured email transport. Production does not offer an admin-visible copy-link fallback that could bypass that proof. Local testing uses a captured test transport.

### Durable email delivery and retry semantics

MongoDB commits are not atomic with SMTP. Use a transactional invitation-delivery outbox from the first invitation release; the audit/revocation outbox may share worker infrastructure, but delivery jobs have their own state and payload contract.

- Create/resend accepts an idempotency key bound to actor, tenant, project, normalized email, requested role, and request digest. Repeating that request returns its original result without creating another invitation or rotating its secret again. Reusing the key with a different digest is a conflict.
- In the invitation transaction, persist a job keyed by `(invitationId,tokenVersion)` and encrypt the delivery payload using authenticated encryption with a dedicated deployment-managed key and stored key ID. Include invitation ID/version as authenticated associated data. The invitation itself never stores a recoverable secret. Missing/unavailable keys fail closed; retain old keys until their pending jobs drain.
- Jobs have queued, leased, retryable-failure, SMTP-accepted, terminal-failure, and obsolete states; lease expiry recovers worker crashes. Use a stable message ID per job and exponential retry delays capped at one hour, at most ten attempts, and never beyond invitation expiry. Exhaustion is visible as failed; an authorized resend creates a new token version/job.
- Before each attempt, recheck current invitation version/status/expiry, active scope, and inviter authority. Suppress obsolete work. Resend atomically replaces the hash/version and invalidates older links/continuations; revoke suppresses remaining jobs. An already in-flight SMTP request can still send an obsolete message, whose link must fail safely.
- A crash after SMTP acceptance but before acknowledgement can send the same email twice. Retry the same job and link, never create an extra grant. SMTP has no guaranteed exactly-once delivery. UI wording "sent" means the SMTP server accepted the message, not confirmed inbox delivery; show queued, retrying, failed, and sent distinctly.
- Delete encrypted delivery secrets after successful acknowledgement or a terminal/obsolete/expired outcome. Never return ciphertext, plaintext secrets, SMTP credentials, or token hashes in admin/public APIs. Log only non-secret IDs, delivery state, and sanitized error categories.

### Server-side sign-in continuation

Raw invitation tokens must not be placed in local storage, session storage, query strings, OAuth state, analytics, or React Query keys. On the initial token exchange, create a server-side continuation record containing invitation ID/token version, purpose, expiry, optional authenticated user ID, and a hash of a separate cryptographically random browser handle. Set the handle only in a production Secure, HttpOnly, host-only cookie using the existing same-site management-cookie policy. The cookie is opaque, not the invitation token. Expiry is the earlier of thirty minutes or invitation expiry; TTL cleanup never substitutes for an explicit expiry check.

The join page reloads its progress from the continuation API. Sign-in forms and any separately supported management authentication redirect preserve the browser cookie and resume the server-recorded internal join route; redirect state carries no invite secret and cannot choose an arbitrary return URL. Runtime OAuth cannot satisfy management authentication. The panel/API deployment must be same-site as required by the existing Lax cookie policy; test this with the configured hosts rather than weakening cookies or falling back to client token storage.

All continuation mutations enforce trusted-origin/CSRF checks, rate limits, and no-store headers, including unauthenticated exchange. Successful reauthentication rotates the handle and binds the record to that account; a mismatched account cannot accept or silently rebind it. Invite rotation/revocation, account mismatch recovery, cancellation, or thirty-minute expiry clears the continuation. An expired continuation requires reopening a still-valid emailed link. Each browser has one active continuation; opening another link replaces it explicitly. Acceptance consumes it transactionally with the invitation, and replay cannot issue further sessions or memberships. If a successful response is lost, the member can log in normally; do not re-enable the consumed secret to recover the response.

## Backend shape and consistency

Extend existing authorization and route contracts with member/invitation read/manage capabilities. Keep controller parsing, service authorization/business rules, and repository persistence separate. Exact route naming is left to the builder; required operations are:

- Tenant-scoped target-project member/invitation administration for tenant owner/admin.
- Project-scoped member/invitation administration for project admins.
- Public, rate-limited invitation preview and new/existing-account acceptance, protected by invitation secret and the required account authentication rather than an existing target-tenant membership.
- Ordinary-member-safe current-account and assigned-project reads.

All mutations resolve tenant/project from trusted session/route context; payload IDs, role claims, and client permission checks cannot override it. Preserve management/runtime credential separation, origin/CSRF restrictions, body limits, and rate limits. Redact account existence in public errors while giving administrators actionable delivery errors.

Use MongoDB transactions for acceptance and membership mutations. A replica set or transaction-capable MongoDB deployment is required; fail clearly rather than using partial-write fallbacks. Serialize all project membership/grant-authority mutations through an updated project collaboration revision in the same transaction, so concurrent last-admin changes and inviter revocation/acceptance cannot both pass stale checks. Apply equivalent tenant and user authorization revisions to tenant membership/role/status and global user identity/status changes; acceptance must touch those same guard documents, not merely read them. Concurrent transactions then conflict and re-evaluate live policy on retry.

Before enabling collaboration writes, inventory **every writer** of management users, tenants, projects, and memberships: existing controllers, project creation/rollback/delete/deactivation, repository helpers, migrations, maintenance scripts, and future administrative APIs. Route supported mutations through shared guarded repository/service operations, including legacy routes. Preserve idempotent project cleanup: deactivate under the guard before deleting scoped records; an inactive/missing project cannot accept invitations or receive recovery grants. The last-admin invariant applies to active projects; intentional deletion/rollback first deactivates the project. Existing project configuration updates must not reset authorization revision fields. Unsupported direct-database operations must run offline with collaboration grants paused, then execute reconciliation before re-enabling them. Deploying only new fenced APIs while old writers remain active is not acceptable. Record the writer inventory and the disposition of each path in implementation review evidence.

Required uniqueness: tenant membership `(tenantId,userId)`, project membership `(tenantId,projectId,userId)`, invitation token hash, and one pending invitation `(tenantId,projectId,normalizedEmail)`. Expired pending invitations are retired before replacement. Before enabling unique membership/email indexes, report legacy conflicts and resolve them explicitly; never delete or merge accounts automatically.

Keep normalized management email identity unique. Add normalized-email/verification metadata through a controlled migration, preserving existing sign-in compatibility. Preflight existing mixed-case/duplicate accounts; do not equate application-user emails with management accounts.

Permission downgrades/removals invalidate affected project sessions using the existing revocation hooks; disabling a tenant membership revokes that user's sessions within that tenant. Invitations never change existing credentials or trigger account-wide recovery/revocation. A separate account-recovery operation is responsible for its own credential/session rules. Persist a revocation event with the membership mutation and retry failures; until processed, live membership checks must deny removed access and must never rely on stale token claims. Inspect existing long-lived management connections and ensure they reauthorize or close when access changes; runtime connections remain separate.

Record invitation creation/delivery failure/resend/revocation/acceptance, role changes, and member removals in audit logs with actor, target, scope, before/after role, and time. Persist mutation audit records in the transaction or a transactional outbox, so audit failure cannot silently lose a security event.

Project discovery must be filtered on the server for `tenant_member`. Direct project detail lookups must enforce the same membership boundary. Tenant owner/admin and existing privileged tenant roles retain their existing project-list behavior. A project token cannot be used for another project or to enumerate the tenant.

## Frontend integration

Reuse existing UI components, React Query, translations, auth context, and permission helpers. Add the Members navigation entry only for authorized administrators. Show resend/cancel and delivery/expiry states, confirmation before removal, and clear last-admin errors. Keep invitation secrets out of React Query keys and persisted storage.

Add a public invitation join route with new-account and existing-account paths, accessible error states for expired/revoked links, account mismatch recovery, and successful project entry. Resume through the server-side continuation after reload/sign-in without allowing arbitrary post-login redirects. Existing-account reauthentication must also work when that account has no active tenant membership; it creates only a short-lived account proof for invitation acceptance, not an otherwise unauthorized tenant session. The invitation cannot reactivate disabled accounts or memberships.

Update project discovery, route guards, dashboard/current-account access, and switch-back-to-tenant for `tenant_member`. Avoid fetching tenant templates/integrations for users who cannot use them. Make builders genuinely read-only for Viewer, including shortcuts, save/autosave, imports, and direct mutation calls. Backend denials remain authoritative. Refresh current role/context on denied or changed sessions and clear stale project data from client caches.

## Verification and release criteria

Use focused Go authorization/service/repository/route tests and frontend integration tests, plus existing relevant suites and a production panel build. Run transaction/concurrency cases against a real test Mongo replica set; mocks alone cannot establish atomicity.

Required cases:

- Same user has different roles in two projects and cannot discover/access an unassigned third project, including direct URLs/API calls.
- Viewer cannot mutate any management surface; Editor cannot mutate schemas; Developer cannot manage collaborators; Project Admin cannot grant tenant access.
- Tenant owner/admin can manage memberships without being silently granted project configuration access.
- Mixed tenant/project and multi-role memberships follow only their own scope's allowlists; Billing/Auditor keep enumerated read access, while Member cannot enumerate projects. Unknown roles, stale token claims, and `OwnerUserID` cannot grant privileges.
- Legacy multi-role/Support memberships survive ordinary updates; single-role conversion requires explicit full-set confirmation, current version, proper authority, last-admin protection, and an audit record. Tenant Owner/Admin can recover an adminless project without receiving project configuration access themselves.
- Invited newcomer joins without a new tenant; existing user joins a second tenant or joins while lacking any current tenant membership; wrong-email/disabled accounts are rejected. Invitation-link possession alone cannot authenticate, reset, relink, or change an existing account. Unverified legacy users must supply fresh existing credentials and a separate verification challenge; inability to authenticate goes to independent recovery, with no credential/session changes through the invite API. A concurrently registered normalized email cannot be overwritten.
- Expired, revoked, rotated, reused, and simultaneously redeemed links do not grant access; inviter removal/demotion blocks pending grants.
- Concurrent last-admin changes leave a usable active admin. Concurrent invite and role updates never restore removed privileges. Exercise races with legacy project delete/deactivate/rollback paths and tenant/user disabling, not just new collaboration endpoints.
- Crash before/after invitation/outbox commit never loses a queued job or creates an orphan grant. Crash after SMTP acceptance may duplicate email but not membership; retry uses the same version/link. SMTP failure/attempt exhaustion and encryption-key failure are visible. Resend idempotency prevents repeated rotation; stale workers send only links that cannot grant access after revocation/rotation.
- Reload and supported authentication redirects retain only the opaque cookie continuation; wrong browser/account, expired/replayed continuation, changed invitation version, CSRF attempts, and arbitrary return URLs fail safely. Secrets never appear in list/preview/error/audit responses, URL query strings, OAuth state, or persisted JavaScript storage.
- Demotion/removal denies stale sessions and direct API requests; revocation retry and audit failure paths fail safely.
- Existing owners/admins/support users, project creation, ordinary login, runtime users, and management/runtime separation keep working.

Before rollout: run legacy identity/membership preflight, apply reviewed migrations/indexes, configure SMTP/TLS and panel URL, verify transaction support, and exercise one real invitation in a test environment. No deployment or real invitations are part of design preparation.

## Implementation phases and acceptance checkpoints

1. **Authorization foundation.** Add `tenant_member`, the explicit resolver policy, own-account/assigned-project reads, management-session compatibility, and the complete authorization-writer inventory/guard integration. Checkpoint: direct API tests prove project isolation and exact mixed-role behavior; legacy creation/delete/deactivation tests pass. Do not enable invitation APIs.
2. **Existing member management.** Deliver member list/update/remove, explicit legacy-role conversion, last-admin serialization, audit, and revocation for projects with usable admins. Keep recovery of adminless projects unavailable, including promotion through ordinary role-update endpoints, until phase 3 supplies verified account state. Checkpoint: real MongoDB replica-set tests prove concurrent last-admin and legacy writer safety, and stale sessions lose removed privileges.
3. **Secure invitations and verified admin recovery.** Deliver verification metadata and independent account proofs first; then enable tenant-admin recovery for verified active tenant members. Also deliver continuation storage/cookies, invitation transactions, encrypted delivery outbox/SMTP worker, rotation/idempotency, and target-tenant session establishment. Checkpoint: verified recovery without caller project access, account takeover negatives, reload/redirect/CSRF cases, crash/retry tests, and concurrent acceptance all pass before invitations/recovery can be enabled.
4. **Frontend and release validation.** Finish Members/join screens, role-aware navigation/builders, read-only Viewer behavior, integration coverage, migrations/preflight, and staged rollout controls. Checkpoint: frontend tests/build and end-to-end invitation/member journeys pass against test email capture and a real MongoDB replica set; existing runtime behavior remains isolated.

Each checkpoint produces reviewable evidence before moving to the dependent phase. Keep incomplete operations unavailable rather than exposing partially enforced collaboration features. These are implementation acceptance gates, not permission to deploy or send real invitations.

## Review disposition

| Review issue | Revision |
| --- | --- |
| 1. Invitation becomes legacy account recovery | Removed invitation-authorized reset/enrollment for existing accounts; require fresh existing-account authentication plus separate verification for unverified accounts; recovery remains independent |
| 2. Effective permission precedence | One live resolver, deny-first eligibility, scope-specific additive allowlists, enumerated tenant exceptions, no tenant/project role union |
| 3. Legacy multi-role memberships | Preserve by default; reject implicit replacement; explicit version-checked conversion with full-role confirmation/audit |
| 4. SMTP/database consistency | Transactional encrypted delivery outbox, idempotency, leases/retries, accurate delivery states, documented duplicate-email limit |
| 5. Invitation survives authentication | Thirty-minute server continuation with opaque secure HttpOnly cookie, reauthentication binding, redirect/reload rules, no raw secret in browser storage |

The review's additional requests are captured in the tenant recovery operation, all-writer concurrency integration, real replica-set coverage, and four acceptance checkpoints above. This revision addresses design requirements; it is not an implementation or penetration-test result.

## Builder decisions and proposals to review

The user's confirmed decisions are the management-only scope and reuse of fixed project roles. The user supplied a conditional review requiring the five revisions above before implementation. Proposed defaults remain: seven-day invitations, administrators-only invitation management, project admins may grant project admin, a new minimally privileged `tenant_member` role, SMTP delivery with an encrypted transactional outbox, five-minute fresh account proof, and thirty-minute server-side continuations. These are design proposals, not previous user commitments.

The builder chooses exact route names, repository/interface names, UI layout within the established component system, migration tooling, SMTP library, transaction/outbox implementation, and test filenames while meeting the boundaries above.
