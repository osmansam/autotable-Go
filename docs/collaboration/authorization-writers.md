# Management authorization writer inventory

All new collaboration mutations use `ManagementCollaborationRepository.WithAuthorizationTransaction`. It writes the active actor user, tenant, and project documents before reading memberships; recipient users are guarded in the same transaction. Snapshot-isolation write conflicts then re-evaluate live policy on retry.

| Existing writer | Disposition |
| --- | --- |
| TenantRegister, tenantAuthController.go | Creates new user/tenant/owner membership; normalized email is checked/stored by the shared normalization policy. No existing membership update. |
| CreateProject, projectController.go | New project is `initializing`; authorization denies it until initialization completes. Creator membership is the first admin. Final project write makes it available; failed initialization invokes rollback. |
| UpdateProject | Deactivation writes the same project document as collaboration guards. Every request setting active=true uses a tenant-authorized majority transaction, locks the project, finds a usable administrator using the shared eligibility pipeline, and guards that administrator before commit. Adminless reactivation rolls back with a recovery error. Updates preserve initializing state. |
| DeleteProject | First deactivates the project with an atomic project-document write, which conflicts with grant guards; cleanup then removes memberships/scoped records. Inactive/missing projects cannot receive grants. |
| rollbackProjectCreateFromDatabase | Same deactivate-before-cleanup pattern; no grants into missing/inactive projects. |
| Project localization/branding/template/credential writes | Write the same project document or retain existing narrow permissions; never overwrite authorization fields or memberships. |
| Collaboration role/removal/recovery/invitation acceptance | Shared guarded transactions; usable-admin checks, version checks, audit and exact-scope revocation. |
| Collaboration migration | Explicit offline CLI, all collaboration flags paused, reviewed conflict-free preflight hash, readiness false until all indexes/backfill succeed. No automatic account merge/removal. |
| Runtime auth/container upgrades | Operate on project-prefixed application-user collections, not global management users/memberships. No management authority supplied. |
| Direct MongoDB maintenance / future user or tenant disabling | Unsupported online. Pause grants first, preserve/replace last admins or deactivate projects, increment account identity version for credential/email changes, revoke affected sessions, invalidate continuations, and reconcile before restoring readiness. Any future HTTP mutation must use the same user/tenant/project guards. |

Current WebSocket is runtime application-only; management identity is not accepted there. Integration tests cover live legacy project deactivation/deletion and transaction-local snapshots. Concurrency tests for membership changes and invitation acceptance extend this inventory in later tasks.
