# Project collaboration operations

This release covers management users in Tenant Panel and its builders. Runtime application users, generated applications, arbitrary custom roles, and password recovery remain separate. Administrator, Developer, Editor, and Viewer are the invitation roles. Existing Support/multiple/unknown role sets remain until an administrator explicitly confirms conversion.

## Prerequisites and migration

Use a MongoDB replica set or supported sharded deployment with transactions and majority writes. Back up the management database. Verify TLS SMTP connectivity and the panel/API cookie topology in staging. Stop legacy tenant/project lifecycle maintenance during migration; never repair memberships directly while grants are enabled.

The following flags default to `false` on every application instance:

```
COLLABORATION_MEMBERS_ENABLED=false
COLLABORATION_INVITATIONS_ENABLED=false
COLLABORATION_RECOVERY_ENABLED=false
```

Build the explicit management migration CLI with `go build ./cmd/autotable-collaboration`. Run its read-only preflight against an explicitly selected database:

```
./autotable-collaboration preflight --uri '<explicit MongoDB URI>' --database '<management database>'
```

Review and resolve every duplicate/case-colliding email, duplicate membership, and unusable administrator finding through an authorized maintenance process. Never merge accounts or discard memberships automatically. Re-run preflight and retain the report and its hash in the release record. With all collaboration flags paused, apply exactly that reviewed hash:

```
./autotable-collaboration apply --uri '<explicit MongoDB URI>' --database '<management database>' --report-hash '<reviewed hash>'
```

URI credentials must be handled by the operator's secret-management process; these examples contain no real credentials. The CLI does not load the application's database URI. Migration schema version 1 writes `management_collaboration_migrations/{_id: "schema", version: 1, ready: true}` only after normalization and indexes succeed. A readiness write fence drains in-flight collaboration transactions before report revalidation. A failed apply leaves readiness false; repair the reported conflict/index issue and re-run preflight/apply. Keep the existing unique indexes and normalized fields; rollback does not delete collaboration or audit history.

New collections: `management_invitations`, `management_invitation_deliveries`, `management_invitation_continuations`, `management_email_challenges`, `management_collaboration_events`, `management_collaboration_requests`, `management_collaboration_migrations`. Unique indexes cover normalized management email, tenant/user and tenant/project/user membership, invitation hash/pending project email, continuation/challenge hashes, delivery job and idempotency keys. TTL indexes clean expired continuation/challenge/idempotency records; authorization checks expiry itself rather than relying on TTL timing. Internal parent revision fields are preserved by existing project writers.

## Mail and browser configuration

Set these through managed secrets/configuration when enabling invitations or verified recovery:

```
INVITATION_SMTP_HOST=<SMTP hostname>
INVITATION_SMTP_PORT=587
INVITATION_SMTP_TLS_MODE=starttls
INVITATION_SMTP_FROM=AutoTable <noreply@example.invalid>
INVITATION_SMTP_USERNAME=<optional username>
INVITATION_SMTP_PASSWORD=<optional password paired with username>
INVITATION_PANEL_URL=https://panel.example.invalid
INVITATION_DELIVERY_KEYS_JSON={"key-2026-11":"<base64 of 32 random bytes>"}
INVITATION_DELIVERY_ACTIVE_KEY_ID=key-2026-11
```

Port 465 with `implicit` TLS is supported. Plain SMTP and missing STARTTLS are rejected; certificate validation is mandatory and AUTH occurs only after TLS. The HTTPS panel setting is an origin, without path/query/fragment. Configure the exact panel origin in the API CORS/trusted-origin whitelist, allow credentials, and forward `Idempotency-Key`. Use same-site HTTPS panel/API hosts: production cookies are Secure, HttpOnly, host-only and SameSite=Lax; cross-site embedding/third-party cookies are not supported. Tenant/project management cookie signing configuration remains required.

Invitation delivery uses a dedicated AES-256-GCM keyring, never a JWT/SMTP password as an encryption key. Rotate by adding the new key to every worker, switching the active key, then keeping old decrypt keys until all jobs using them have completed/expired and backups are handled. Losing a key makes its queued mail undecryptable; cancel/resend using the active key. Avoid printing decrypted payloads. Tokens are 256-bit random secrets; only hashes are stored in invitation/challenge/continuation records. Encrypted delivery payloads are purged on terminal delivery/expiry/obsolescence. Invitation links use a fragment; the join page removes fragments/query data immediately and keeps continuation only in an opaque HttpOnly cookie. Reverse proxies, analytics, traces and support tools must not capture request bodies or cookies on these authentication endpoints.

## Workers and recovery

The application starts durable collaboration audit/revocation and invitation delivery workers. They lease work and retry after crashes. Delivery has at most ten attempts, exponential delay capped at one hour, a stable Message-ID, and explicit queued/retrying/sent/failed states. SMTP acceptance is not inbox delivery. A crash after DATA acceptance can cause duplicate email; the database transaction still grants project access at most once. Resend rotates the invitation version and invalidates older links/continuations/jobs. Revocation is checked live even before the durable session-family revocation worker finishes. Management project families store the membership generation at issuance; security events invalidate only earlier generations, so delayed audit/revocation retries preserve fresh authorized sessions. Legacy pending events without a generation invalidate only legacy untagged families. Never bypass the normal session repository when creating management project families.

Monitor queued age, retries/terminal failures, obsolete payload cleanup, revoked session families, and durable security-event processing. Never log raw email bodies, invite/challenge tokens, browser handles, passwords, key material or cookies. Infrastructure failure denies grants instead of falling back to token claims. Do not bypass a failed mail job by manually assigning an existing account's password or mailbox verification state.

Existing accounts must prove their current password within five minutes; unverified accounts also complete an independent email challenge. Invitations never replace a password, merge accounts or reactivate a disabled tenant membership. Accepted membership is committed before target tenant/project sessions are issued. If session issuance fails, the response explicitly directs ordinary sign-in; consuming the link again cannot issue sessions.

If no usable project administrator remains, a tenant owner/admin may explicitly select an active, verified existing tenant member and provide a reason. Recovery adds Admin while preserving legacy role sets and audits the explicit target. It gives no implicit project configuration access to the caller. Ordinary role/removal operations protect the last usable administrator.

## Staged enablement and rollback

1. Deploy the foundation with all flags false. Complete reviewed schema/index migration and verify session, identity separation and project discovery.
2. Enable members in staging, then the intended fleet after member administration/last-admin/revocation checks pass.
3. Configure tested TLS mail and encryption keys. Enable invitations and recovery only after ownership-proof, delivery-retry and verified-recovery checks pass.
4. Roll out the paired panel build after live context, join flow, read-only builder and cache-boundary checks pass. UI capability queries follow the server flags and selected authority scope.

To pause, turn invitation/recovery flags off first, then members if necessary. Keep workers, keys and durable history available for revocation, audit and cleanup. In-flight transactions already authorized may finish when changing process environment; restart/drain the fleet for a full stop, and set migration readiness false through the reviewed maintenance procedure before schema work. Do not drop collections/indexes, remove accepted memberships, reactivate disabled accounts, or restore stale signing/session state as a feature rollback. Re-enable only after reconciliation and the relevant staging checks.

No production migration, deployment or real email has been performed by this implementation.

## Project visibility, reactivation and session availability

Privileged tenant roles (Owner, Admin, Billing, Auditor) retain visibility of inactive project metadata for administration. Initializing projects are hidden from discovery and direct repository lookup for every role. Basic tenant Members discover only their active assigned projects. Inactive project cards do not offer switching, runtime opening or membership management. Visibility never supplies a project configuration role.

Every project-update request containing `isActive: true` uses a guarded majority transaction, including requests whose initial read saw the project active. This closes a concurrent-deactivation race. Activation requires a usable existing administrator: active account, known active tenant membership, and active project Admin membership. The update and any other submitted metadata roll back together if that requirement fails. A project-only administrator cannot perform tenant lifecycle updates. If an intentionally inactive project has lost its last usable administrator, arrange verified administrator access through authorized maintenance with grants paused, as described in the writer inventory, before requesting activation; ordinary project recovery endpoints still require an active project.

Management authentication deliberately reads the session-family store on each request. Keep its existing session indexes healthy, deploy session-family support before enabling collaboration, and include this database round trip in staging latency/capacity checks. Store failure returns 503 and blocks the operation; the panel shows an access-verification failure and preserves local identity instead of turning a 503 into logout. This release does not claim a production latency benchmark.

Concurrent identical invitation create/resend intents are serialized by the shared actor/tenant/project authorization writes before checking the idempotency record. A six-caller replica-set regression verifies that all callers receive the cached result, create one invitation, and rotate once on resend. Reusing the key with a different digest remains a conflict. No unguarded duplicate-key replay fallback is used.

## Expired invitation reconciliation

Expired invitations can be resent from the panel, creating a new secret/version through the existing authorized API. Cleanup processes at most 100 expired delivery jobs per tick and never needs decryption keys or a mail sender. Each job's terminal status, current-version invitation delivery label, encrypted-payload removal and durable audit record commit together. `expiryFinalizedAt` prevents duplicate audit events and allows reconciliation of older terminal-expiry records. An audit-write failure rolls back the job update for a later retry; expired tokens remain invalid throughout. Expiry of an old invitation version or a verification delivery cannot overwrite a current invitation's delivery label.

The reviewed index set includes the additive nonunique `invitation_expiry_cleanup` index on `expiryFinalizedAt`, `expiresAt`, and `_id`, so the worker can skip finalized history. Apply the current reviewed migration/index set during staging rollout; no database/index change was applied to production here.
