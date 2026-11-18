# Collaboration verification ledger

## Phase 1: authorization foundation

Implemented scope-specific management permission resolution, ordinary tenant members, assigned-project discovery, current-role session reads, management family liveness and refresh adoption, identity-specific revocation, normalized-email preflight, and guarded authorization transactions. Existing Go controller/route/middleware/service/repository suites passed in task checkpoints. The panel baseline passed 28 tests before frontend changes.

## Phase 2: existing member administration

Implemented project/tenant admin member list, versioned role changes, explicit legacy-role conversion, explicit project-membership reactivation, removal, last-usable-admin protection, durable audit/revocation events, approved API contracts, and feature/migration readiness gating. Features default disabled.

Evidence: `go test ./services ./repositories ./controllers ./routes ./middlewares -count=1`; `scripts/test-collaboration-mongo-integration.sh` (task-owned local replica set). Tests include reciprocal concurrent admin removal, a grant paused against legacy project deactivation, missing/rolled-back projects, audit-failure rollback, revocation retry with runtime/other-project sessions preserved, lifecycle revocation of both identities, strict request parsing, and the fixed-role API matrix. Runtime credentials and mismatched project-management tokens cannot access member APIs. A readiness fence prevents an in-flight request from committing through a migration pause.

Recovery is deferred until independent account verification is implemented. Invitation and recovery flags remain false by default. No production migrations, deployments, SMTP configuration, or real invitations have been performed.

## Later checkpoints

The phase 3 and phase 4 implementation evidence follows below. Final independent review is the remaining release gate.

## Phase 3: invitation delivery, ownership, acceptance and recovery

Implemented mandatory TLS SMTP, dedicated AES-GCM encrypted delivery outbox, stable message identity and bounded retry, hashed single-use invitations, opaque browser continuation, independent account/password/email proofs, transactional acceptance, postcommit target sessions, and explicit verified administrator recovery. Existing passwords, other tenant roles and other project/runtime sessions are preserved. Account identity versions are separate from grant-fencing revisions.

Executed `env GOCACHE=/private/tmp/autotable-collaboration/gocache go test ./...` successfully at Task 8, plus repeated task-owned replica-set runs via `scripts/test-collaboration-mongo-integration.sh`. Real MongoDB cases cover concurrent acceptance, inviter demotion, normalized account collision, disabled tenant membership rejection, audit failure rollback, existing-account role/password/session preservation, continuation expiry, independent verification, and administrator recovery. The HTTP test checks actual response envelopes, secure-cookie boundaries, real target session persistence and replay denial.

## Phase 4: panel and complete HTTP journeys

Panel implementation includes live selected-scope session refresh, actual tenant switching, scoped query caches, fixed-role member administration, explicit legacy conversion/reactivation, invitation delivery states, verified recovery, cookie-backed join, separate account proof and read-only/stale-editor write checks. All 879 panel tests in 170 files passed after final context checks. Production build passed. Lint passed with 117 warnings and zero errors; warnings are shared form/fast-refresh guidance. Bundle budget passed: 433 JavaScript chunks, largest TemplatePanel 1,889,394 of 2,000,000 bytes.

`TestCollaborationMongoHTTPJourneys` uses a task-owned MongoDB replica set, seeded management parents in two tenants/three projects, real strict route middleware/live authorization/session cookies, real collaboration endpoints, and captured mail transport. Developer, Editor and Viewer invitations are created through HTTP, captured delivery is exchanged, existing accounts prove passwords, an unverified account completes a separate email challenge, a new account joins without creating a tenant, permission probes verify role/scope denials, member removal denies live access, and the durable worker revokes the stale family. Same-ID runtime credentials remain rejected. Permission probes use actual registered security contracts with inert success handlers; they do not mutate unrelated schemas/pages. Existing lifecycle tests independently cover project initialization/rollback and concurrent deactivation.

A recovery pagination regression was reproduced RED and fixed GREEN: verified candidates now carry their current project membership version, including candidates outside the first member-directory page. Full verification and final independent-review outcomes are recorded below when completed.

## Scope of the evidence

No production services, database credentials or real recipient addresses are used. The isolated script validates its mongod process/data-directory ownership and deletes only its own temporary database/process. SMTP tests use task-owned loopback servers/captured transport. Browser components are exercised through jsdom; a deployed browser/SMTP-provider staging acceptance remains an operator rollout step. No external mail, push, merge or deploy was performed.

## Final commands before independent review

- Backend: `env GOCACHE=/private/tmp/autotable-collaboration/gocache go test ./... -count=1` — passed all packages.
- MongoDB/HTTP: `env GOCACHE=/private/tmp/autotable-collaboration/gocache scripts/test-collaboration-mongo-integration.sh` — passed 54 top-level collaboration tests, including the three-role HTTP journeys; real task-owned replica set, no integration skips.
- Panel: `yarn test` — 170 files / 879 tests passed.
- Panel: `yarn lint` — passed, 0 errors / 117 warnings.
- Panel: `yarn build` — TypeScript and production build passed.
- Panel: `yarn check:bundle` — passed, 433 chunks; largest 1,889,394 / 2,000,000 bytes.
- Both worktrees: `git diff --check` — passed.

Self-review checks reproduced and fixed cached-role flashes and mismatched capability responses (RED→GREEN). Cached user identity cannot display privileged roles until live verification, and capability IDs/scope must match the selected workspace.

## Final review and fix-pass verification (November 18, 2026)

The fresh independent review identified no Critical findings. Six Important findings were accepted, each reproduced with a failing regression and fixed in one pass. See `review.md` for findings, test names, corrections, two deferred minor UI/status issues, and operational review limits; `decisions.md` preserves every execution ruling.

After the fixes, the complete Go suite (`go test ./... -count=1`) passed. The task-owned replica-set script passed 56 top-level tests with no integration skips, including refreshable wrong-account rejection, fresh-generation session preservation through audit retry, and migration role/tenant eligibility. The complete panel suite passed 170 files / 883 tests; lint passed with 0 errors / 117 warnings, TypeScript/production build passed, and bundle budget passed. Both worktrees passed `git diff --check`. These results describe the original reviewed snapshot; follow-up changes and verification are recorded below.

Paired worktrees remain on `codex/project-collaboration`: `/private/tmp/autotable-collaboration/backend` and `/private/tmp/autotable-collaboration/tenantPanel`. Original branches remain `feat-boolean-switch` (backend) and `feat-tenant-auth-register` (panel). Integration, push and deployment were not performed. All collaboration flags default false pending the documented migration and staging configuration.

## Follow-up review verification

The current uncommitted changes on `feat-boolean-switch` (backend) and `feat-tenant-auth-register` (panel) include the follow-up fixes described in `review.md`. To avoid inheriting the application's .env/database configuration, backend tests ran from the isolated matching checkout with explicit task-owned fixtures; changed source/test files were mirrored from the original working tree and checked for byte equality. No database migration or SMTP-provider message was performed.

- `go test ./... -count=1` with the isolated Go cache — all packages passed.
- `scripts/test-collaboration-mongo-integration.sh` — 62 top-level collaboration tests passed, including concurrent idempotency, privileged project visibility, and reactivation invariants; no integration skips.
- `yarn test` — 170 files / 885 tests passed.
- `yarn lint` — 0 errors / 117 warnings.
- `yarn build` and `yarn check:bundle` — passed.
- Working-tree whitespace checks — passed.

These results are local evidence only. CI, real SMTP-provider delivery, deployed reverse-proxy/browser cookie attributes and production-like migration data remain unexecuted staging checks. All collaboration feature flags remain disabled by default. The unfinished unrelated backend cherry-pick sequence and both original indexes/branch heads were preserved, with no new commits.

## Deferred-minor cleanup verification

The two original deferred minors are resolved. The panel can resend an expired invitation using its current version; it cannot resend accepted/revoked invitations. Expiry cleanup uses bounded per-job majority transactions to reconcile the failed delivery label, remove expired encrypted payloads and record one durable audit event. An `expiryFinalizedAt` marker allows safe retries/concurrent workers and repair of terminal-expiry records left by the old implementation. The additive `invitation_expiry_cleanup` index is part of the reviewed migration's index set; no production index application was performed.

Executed locally after the fixes: `go test ./... -count=1` (all packages), the isolated replica-set collaboration script (65 top-level tests, no integration skips), `yarn test` (170 files / 888 tests), `yarn lint` (0 errors / 117 warnings), `yarn build`, `yarn check:bundle`, and working-tree whitespace checks. The isolated source/test files match the originals byte for byte. No new commits or index changes were made to either original repository; the existing unrelated cherry-pick sequence was preserved.
