# Final independent review

A fresh `gpt-6.2-astra` reviewer examined backend `7eef0d1..da904d6` and panel `7fc0978..9fe8a15` against the approved spec, plan, ledger rulings and failure-mode focus. Review was read-only. No Critical findings were identified. The six Important findings were accepted and fixed in one test-driven pass, without a second reviewer.

| Finding | Regression evidence and correction |
| --- | --- |
| Expired project access could redirect a refreshable user to login | `recovers an expired project session through its project refresh cookie` failed before the fix and passes after scoped refresh selection includes project management session reads. |
| Expired tenant access hid a still-refreshable wrong account | `TestCollaborationHTTPAcceptanceJourney` reproduced anonymous handling. Browser identity now checks a signed, current/grace refresh token against its live management family without issuing or rotating sessions; mismatch requires explicit account switching. |
| Another tab's old project could overwrite shared workspace context | `does not publish a pending refresh after another tab replaces the workspace` failed before the fix. A generation and persisted workspace snapshot now protect asynchronous publication; storage changes adopt identity without trusting cached roles, abort stale writes, clear caches and reverify. Write guards also compare selected tenant/project metadata. |
| Delayed/retried revocation invalidated fresh sessions | `TestCollaborationMongoRevocationPreservesFreshGeneration` failed before the fix. Session creation reads live membership generation; mutation events revoke earlier generations only, independently of clock skew and audit retries. |
| Migration certified empty/unknown tenant roles or a cross-tenant project mismatch | `TestCollaborationPreflightUsableAdministrator` failed for all three defects. Preflight now reuses live session-role eligibility and counts administrators by tenant plus project. |
| Adminless project recovery hid secure invitations | `offers tenant-authorized invitations for an adminless project` failed after waiting for the actual recovery state. Authorized tenant administrators can now invite a replacement while ordinary member promotion remains recovery-gated. |

The focused regression runs passed after each fix. Final complete-suite results are in `verification.md`.

## Resolved minor findings

- Expired invitations now offer Resend through the existing versioned API. Cancellation remains available only for pending invitations; accepted/revoked invitations cannot be resent.
- Expiry cleanup now atomically marks the current expired invitation's delivery failed, removes the encrypted payload/lease and writes a durable delivery-failure audit event. A per-job finalization marker prevents duplicate events across concurrent cleanup/retries and repairs old terminal-expiry records. Rotated invitations and unrelated verification-delivery expiry preserve the current invitation's state. Cleanup batches are bounded and use the additive expiry cleanup index.

These findings were originally deferred and were subsequently resolved at the user's request; the staging limits below still apply.

## Operational limits of review

Real SMTP-provider delivery, migration on actual production data, and deployed browser/proxy cookie attributes were not exercised. These remain the documented staging/release checks, with flags disabled until readiness/configuration checks pass. No production credentials or services were used. There are no unresolved Critical or Important review findings.

## Follow-up pasted review (November 18, 2026)

The follow-up was verified against the current uncommitted working trees. Changes remain uncommitted by the user's explicit request; the recommendation to create review commits does not override that request.

- Concurrent duplicate idempotency conflict was not reproduced. `TestCollaborationConcurrentInvitationIdempotency` executes six simultaneous creates and six simultaneous resends against a real replica set; all callers return the same invitation/version, with one create and one rotation. Shared authorization parent writes serialize the transaction before replay lookup. Existing guarded replay logic was preserved.
- Privileged inactive visibility is intentional. Initializing-project visibility was reproduced and fixed for list/direct lookup. The panel now also suppresses the Members link on inactive cards; open/switch were already blocked. `TestCollaborationPrivilegedProjectVisibility` covers Owner/Admin/Billing/Auditor; the inactive card test failed before the Members-link correction and passes after it.
- Adminless reactivation was reproduced through the actual HTTP update handler (200 before the fix). `ReactivateProject` now checks tenant update authority and administrator eligibility inside a transaction, fences the project and selected administrator, and rolls back all changes on failure. Tests cover rejection, rollback, a usable administrator, project-only authority denial, and the actual HTTP 409 recovery response.
- The unused local-only `useAuth.logout` helper was removed after confirming the only production caller of `useAuth` is PrivateRoutes. UI logout continues through the existing backend-backed hook.
- Existing session-store outage handling was verified by `TestManagementSessionUnavailableFailsClosed` and the panel outage regression: 503 is preserved as an availability failure, not logout. Existing recovery errors already have a recovery-specific code and message; an HTTP regression pins that contract.
- The original backend has an unfinished `--no-commit` cherry-pick sequence for unrelated page-runtime-binding commits. Its sequence metadata, index and branch HEAD were left untouched. There is no `CHERRY_PICK_HEAD`, but the sequence is still reported by `git status`; no continue/abort/reset/commit was run.

Complete follow-up checks passed locally: all Go packages, 62 collaboration tests in the task-owned replica-set harness, 885 panel tests, lint (0 errors / 117 warnings), production build and bundle budget. These are local test results, not CI results. Real SMTP delivery, reverse-proxy/browser cookie behavior, and a production-like data migration remain staging release checks. No production rollout was performed.

## Minor-fix verification

The expired Resend UI regression and database expiry-audit regressions failed before their fixes and pass afterward. Integration tests cover queued/retrying/leased jobs, legacy terminal expiry, concurrent cleanup, replay, audit-insert rollback, rotated invitations and verification-job isolation. `GetAllProjects` still validates the tenant ID but no longer creates/discards an unused ObjectID variable.

All Go packages passed; the replica-set harness passed 65 top-level collaboration tests; the panel passed 170 files / 888 tests; lint, build, bundle budget and working-tree whitespace checks passed. Results are local evidence, not CI/staging execution. All fixes remain uncommitted on the existing branches, and the unrelated backend cherry-pick sequence remains untouched.
