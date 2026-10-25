# Workflow Context Query Optimization

**Goal:** Reduce the remaining pre-cache workflow overhead without caching security metadata across requests.

**Evidence:** Release `9c51a3c` includes request-local project reuse. Matched production request logs show 159–226 ms backend processing on cache hits; a MongoDB/Redis readiness probe took 72 ms. The current workflow route still performs a live project check and a live container fetch in sequence.

**Approach:** On a warm Redis slug mapping, fetch the active project and its tenant/project-scoped collection metadata with one MongoDB aggregation. Retain existing authentication and workflow authorization after this fresh read. Reuse verified request context in the handler as before. Cold mappings, legacy routes, and other operations retain existing behavior.

**Constraints:** No shared permission cache, no removed active-project check, no authentication changes, no production writes or deployment. URL/token mismatches must fail without overwriting auth locals. Metadata and workflow output remain unchanged.

**Files:** new `utils/workflowRequestContext.go` and matching unit tests; small workflow-only branch in `middlewares/authenticate.go`; workflow-cache docs.

- [x] Add RED tests for one scoped Mongo aggregation, token mismatch, inactive/missing project, missing schema, and cold/legacy fallback.
- [x] Implement warm-map combined lookup and use it only for workflow conditional authentication.
- [x] Verify the aggregation against existing production metadata with read-only queries and equivalent ordinary reads.
- [x] Run complete Go/race/vet/build checks and one independent final review.
- [x] Document results and leave changes local for deployment and production remeasurement.

## Progress

- Regression verified against the old sequential path using a compiler overlay (no working-tree rollback): fails because the scoped aggregate is absent. Combined query passes.
- Inactive-project regression: RED showed the old slug mapping was retained. GREEN removes it when the active-project query returns no project, matching the existing resolver and allowing a replacement project using the same slug to resolve on a later request.
- Permission-change test confirms the next HTTP request reads updated authentication metadata; no shared permission cache was introduced.
- Read-only production metadata comparison from this client: sequential 105.3 / 70.1 / 74.3 ms; combined 37.7 / 37.0 / 38.4 ms. All complete container metadata, including permissions and workflow definitions, matched exactly. These are query timings, not end-to-end HTTP predictions.
- Full unit and full race suites, vet, and Linux production build passed. Build-cache sandbox access was resolved for vet/build.
- Final independent review completed on the available reviewer model after the preferred model stopped at a usage limit before reviewing. No Critical or Important findings. Reviewer independently reran targeted workflow-context and authentication-reuse tests and whitespace checks.
- Deferred minor: optional additional tests for aggregate/decode failures, cancellation, malformed Redis mappings, and already-resolved fallback. Code inspection found no defect in those branches.
- Review boundaries: unrelated existing auth/cache architecture was unchanged; post-deployment end-to-end timing is not established by the database probe. The new code remains local, with no commit, push, production deployment, or production metadata writes by this task.
