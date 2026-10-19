# AutoTable operational upgrade

## What is ready

Scheduled runs are persisted in MongoDB before execution and delivered through the existing outbox. A cursor and its occurrence commit together. Restarts recover the previous 24 hours, at most 100 runs per workflow and 500 per poll; polling rotates workflows to prevent starvation. Earlier history is skipped with `skippedBefore` recorded on the cursor. First discovery and schedule/definition changes start in the future. Unchanged reloads retain the cursor. Current metadata is fenced during scheduling so a stale replica cannot restore an old definition or enqueue obsolete code.

Database changes, child outbox jobs and the root completion marker commit together. Retry after an acknowledgement failure does not repeat committed database work. Cron roots use a 50-second transaction budget below MongoDB's default transaction lifetime, covered by a 90-second lease reserved before dispatch. Heartbeats do not concurrently modify the claim during that transaction. Other outbox operations retain renewable leases. External requests remain separate outbox effects and retain existing provider idempotency limitations. Retries use the existing bounded ten-attempt/backoff policy; terminal failures remain visible in `dynamic_outbox` with `operation: "cron_workflow"` and `status: "failed"`. Investigate/repair the cause before explicitly requeuing a failed job; do not delete completion markers to force replay.

Use MongoDB's replica-set/transaction-capable topology. Startup provisions outbox uniqueness/execution-marker indexes before any producer or worker starts. Do not mix old Redis-only cron producers with new producers during rollout: stop/recreate the old application as the deployment script does. A crash before this version has established its first durable cursor cannot be recovered retrospectively. Pending roots recheck the source workflow is active; disabled/removed roots complete without effects. Current permissions remain authoritative while executable snapshots are pinned to their occurrence.

## Build the upgrade tool

```bash
go build -o /private/tmp/autotable-upgrade ./cmd/autotable-upgrade
/private/tmp/autotable-upgrade --help
```

The tool never loads `.env` automatically. Select an exact database and provide its connection URI through an environment variable from the existing secret store. Avoid passing a URI/password as a command argument or saving it in shell history. The examples below assume `AUTOTABLE_UPGRADE_MONGO_URI` is already supplied securely and `AUTOTABLE_UPGRADE_DATABASE` names the intended database.

## Audit before any live change

```bash
/private/tmp/autotable-upgrade --database "$AUTOTABLE_UPGRADE_DATABASE"
```

The default action is read-only. It reports transaction support, authentication configuration, account enrollment counts, legacy refresh-session counts, duplicate identity/family counts, session-index readiness and queued/failed cron counts. It does not return account values, passwords, token hashes, refresh-family IDs or translation text. Database/query failure is a failed audit, not evidence that no problems exist. Exit zero means the audit ran successfully; inspect the findings before rollout.

Resolve duplicate localization identities deliberately, preserving human edits; this tool does not delete or merge them. A missing sparse session-key/TTL index will be provisioned by application startup if the deployment account has index permissions. Legacy refresh families are intentionally excluded from rotation and expire naturally: users sign in once again. No bulk session deletion is needed.

## Prepare one legacy email-only auth schema

After confirming the exact project and backing up its metadata, add a hashed password credential with explicit scope:

```bash
/private/tmp/autotable-upgrade --action add-password-field \
  --database "$AUTOTABLE_UPGRADE_DATABASE" \
  --tenant "$TENANT_ID" --project "$PROJECT_ID" --schema "$AUTH_SCHEMA"
```

This adds a required hashed password field and turns public registration off. It preserves other metadata, stops on a conflicting password field or concurrent field edit, and is idempotent when a hashed secret credential already exists. It does not invent or backfill passwords. Existing OAuth-only accounts can continue through their configured verified identity flow; decide whether they need password enrollment. Refresh the selected collection's metadata cache through the normal management update flow, or let its existing cache TTL expire; login credential configuration itself reads live metadata.

## Enroll/reset one verified account

Verify the person's identity through an established trusted process first. Do not grant access merely because someone knows an email address. Use an exact application-user ObjectID within the selected project. Pass a strong, owner-chosen password through stdin, using the terminal's hidden-input facility or a secret-manager pipe:

```bash
/private/tmp/autotable-upgrade --action set-password --password-stdin \
  --database "$AUTOTABLE_UPGRADE_DATABASE" \
  --tenant "$TENANT_ID" --project "$PROJECT_ID" --schema "$AUTH_SCHEMA" \
  --user "$VERIFIED_APPLICATION_USER_ID"
```

Input is not echoed by the tool; provide it through a hidden-input facility rather than typing into an echoing stdin terminal. The tool stores only a bcrypt hash and revokes that user's refresh sessions in the exact tenant/project in one transaction. A revocation failure rolls back the password update. Other users/projects and unrelated fields remain untouched. Existing access tokens expire normally. Rerun the audit to distinguish configured credential fields from accounts that still need enrollment. A database operator runs this tool; it is not a public enrollment endpoint and sends no email/messages.

## Release and verification

Run the complete local checks and hosted CI before releasing. Keep the source changes together: outbox schema, cursor producer, root consumer, startup indexes, and the compatible tenant panel account-creation endpoint. Use a unique release tag and the existing staged `.env.next` mechanism with `scripts/deploy-production.sh`. The script builds first, replaces the single application instance, checks `/ready` and containment routes, refreshes nginx, and restores the previous image/environment on failure. Preserve `.deploy/ready-image` and retained images. Expect brief recreation downtime.

After deployment, check dependency readiness, management/application credential separation, one successful enrolled-user login/refresh/logout, one disposable project cron run, and absence of failed/stuck cron roots. Confirm the new producer creates `workflow_cron_schedules` cursors and durable outbox occurrences; a static `/health` response is not sufficient verification.

## Production access still needed

No production server, application database or account was changed during this work. The configured GitHub CLI login is unavailable, and the deployment guide contains example SSH values. Re-establish the repository's GitHub deployment access or identify an existing authenticated server connection before attempting a real rollout. No password, private key or guessed production host is required to finish local preparation.
