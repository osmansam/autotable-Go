# Operational Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the scheduler crash/recovery gap and prepare safe legacy-auth upgrades and deployment.

**Architecture:** Persist scoped schedule cursors and root workflow occurrences transactionally into the existing outbox. Reuse its retry/lease machinery and execute database work with the completion marker in one transaction. Supply read-only upgrade diagnostics and an explicit scoped auth metadata migration.

**Tech Stack:** Go 1.24, MongoDB replica-set transactions, existing robfig cron parser and outbox.

**Spec:** `docs/superpowers/specs/2026-10-19-durable-cron-design.md`

## Global Constraints

- Native execution; user authorization overrides approval pauses. Preserve prior dirty changes and disabled dynamic functions. No commits/pushes without an integration need and a concrete reviewed artifact.
- Recovery horizon 24 hours; 100 occurrences/workflow and 500/poll; first discovery starts in the future; 50-second transaction budget and 90-second reserved lease.
- Reuse the outbox's bounded retries and ownership checks. Passwords, hashes, tokens, connection strings never appear in diagnostics.
- Production target and working authentication are required for actual rollout; no guessed host or live database.

## Review Focus

- Concurrent replicas: only one durable occurrence and cursor advancement.
- Crash between cursor/event writes: transaction rolls both back; restart recovers the run.
- Retried root after database commit/ack failure: no duplicate mutation or child events.
- Removed/disabled workflow or expired claim: queued root cannot execute/commit.
- Legacy metadata with conflicting credential fields: report/stop, never overwrite fields or invent passwords.

### Task 1: Durable schedule cursor and occurrence materialization

**Files:** `models/workflowCron.go`, `repositories/workflow_cron.go`, `services/workflow_cron_durable.go`, `services/dynamic_cron.go`, `services/workflow_cron_occurrence.go`, cron tests.
**Interfaces:** repository `EnsureCronSchedule(ctx, id, revision, first)`, `AdvanceCronSchedule(ctx, cursor, next, skippedBefore, event)`; scheduler uses existing metadata loader.

- [ ] Add failing clock/restart/bounded recovery tests and real Mongo concurrency/rollback tests.
- [ ] Implement cursor/event transaction, stable scope identity and bounded fair materialization.
- [ ] Replace Redis attempt dispatch with durable materialization; update obsolete inline-execution tests while retaining timezone/DST coverage.
- [ ] Run affected suites and real Mongo integration tests.

### Task 2: Durable cron root execution

**Files:** outbox payload/operation, `services/workflow_cron_outbox.go`, dispatch/startup wiring, real Mongo tests.
**Interfaces:** `processCronWorkflowOutbox(ctx, repository, event)`; consumes Task 1 root snapshots and existing owned outbox claim.

- [ ] Add failing replay/rollback/disable/claim-loss tests.
- [ ] Execute root and marker atomically, fence scope and claim, preserve async side-effect idempotency.
- [ ] Provision indexes before workers, validate snapshots and cap execution.
- [ ] Run service, repository and real Mongo suites.

### Task 3: Compatibility audit and explicit scoped auth metadata upgrade

**Files:** `internal/upgrade`, `cmd/autotable-upgrade`, tests and upgrade runbook.
**Interfaces:** audit takes injected Mongo database; CLI defaults to read-only and requires explicit tenant/project/schema for metadata writes.

- [ ] Add failing audit classification, no-secret-output and migration conflict/idempotency tests.
- [ ] Implement bounded diagnostics and scoped metadata upgrade without password/account writes.
- [ ] Verify on disposable MongoDB fixtures; document enrollment and refresh behavior.

### Task 4: Final verification and deployment handoff

**Files:** progress/runbook and existing deploy/CI integration as needed.

- [ ] Run full Go race/vet/Linux build, Mongo tests, shell/CLI/deployment checks and diff hygiene.
- [ ] Request one fresh read-only final review; correct important findings with regressions.
- [ ] Inspect authenticated deployment availability. If unavailable, report that exact remaining blocker with a concrete release/runbook ready to use.
