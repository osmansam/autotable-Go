# Implementation decisions

These are the complete recorded implementation rulings, in execution order. Each records the tradeoff or ongoing requirement. Final independent-review findings and their current resolution status are in `review.md`.

1. paired worktrees under /private/tmp/autotable-collaboration — keeps both repositories isolated without modifying original ignore rules — original checkouts will not show implementation until integration.

2. project discovery uses the specialized repository directly rather than a pass-through service — filtering and live snapshot validation stay together without duplicate policy — repository tests and HTTP contracts must stay aligned.

3. legacy project lifecycle writes already touch the same parent document as transactional grant guards — preserve their atomic deactivate-before-cleanup flow rather than wrapping cleanup in a large transaction — real replica-set race verifies retry/denial if lifecycle wins.

4. account identity version is separate from authorization guard counters — future email/credential changes invalidate account proofs without ordinary grant fencing invalidating its own proof — every future account recovery path must increment accountVersion.

5. per-user collaborator revocation affects management sessions only, while existing project lifecycle revocation affects both identity contexts — preserves project deletion/deactivation semantics — separate real-Mongo tests pin both behaviors.

6. added a project-scoped capability endpoint alongside the tenant endpoint — project UI must not inherit tenant-owner capability from the ambient tenant cookie — client selects its actual scope and validates returned IDs.

7. feature middleware tags write context for a transactional readiness fence; migration rechecks its report after draining that fence — stops in-flight writes racing a migration pause — all future collaboration write routes must use this middleware.

8. Continuation cancellation stays available with invitations disabled — browser-owned logout semantics must remain usable — cancellation still requires trusted Origin.

9. Recovery may explicitly target the tenant administrator themselves and preserves existing role sets by adding Admin — no implicit configuration grant occurs — incorrect selection can grant the explicitly chosen person project access.

10. HTTP journeys seed management parent projects and use inert success handlers behind actual strict schema/page security contracts — existing lifecycle tests separately cover project bootstrap/rollback, while the new journeys exercise real collaboration endpoints without production configuration — full deployed browser/schema execution remains staging work.

11. real SMTP delivery, actual production migration data and deployed browser/proxy cookie attributes remain operator staging checks — local fixtures and source review do not establish production integration — wrong deployment configuration can prevent email or cookie-based joining despite passing local tests.

12. durable member revocation uses membership generations rather than wall-clock cutoffs — fresh families are tagged with the live project-membership version and mutation events revoke only older generations, including legacy untagged families — future management family creation must continue through the tagging repository.

