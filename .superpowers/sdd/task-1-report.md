status: DONE_WITH_CONCERNS

changed files:
- Generated patch for `src/utils/api/container.ts`.
- Generated patch for `src/utils/api/container.test.ts`.
- No files under tenantPanel were modified.

commits created:
- None. The patch is unapplied and tenantPanel is read-only in this environment.

tests/commands run with outputs summarized:
- `git apply --check /Users/osmansamilerdogan/Desktop/autotable-Go/.superpowers/sdd/task-1.patch` from `/Users/osmansamilerdogan/Desktop/tenantPanel`: passed with exit code 0.
- `yarn build`: not run because the patch could not be applied in this read-only environment.
- Test suite: not run because the patch could not be applied in this read-only environment.

self-review notes:
- The DynamicWorkflow replacement is an explicit full replacement hunk based on the current source, with no duplicated interface declaration.
- The patch includes the requested workflow types, normalization helper, loader normalization, update payload, update hook, and focused normalization test.
- TenantPanel source and git state were left unchanged.

concerns:
- Build and runtime tests remain unexecuted until the patch is applied in tenantPanel.
- No commit was created because tenantPanel is outside the writable workspace.

## Fix

status: COMPLETE

patch path:
- `/Users/osmansamilerdogan/Desktop/autotable-Go/.superpowers/sdd/task-1-fix.patch`

changed files:
- `src/utils/api/container.test.ts` (in patch only)
- `.superpowers/sdd/task-1-fix.patch`
- `.superpowers/sdd/task-1-report.md`

commands run:
- `git apply --check /Users/osmansamilerdogan/Desktop/autotable-Go/.superpowers/sdd/task-1-fix.patch` from `/Users/osmansamilerdogan/Desktop/tenantPanel`: passed with exit code 0.

self-review:
- The added test invokes the captured `useUpdateWorkflows` mutation function and asserts `axiosClient.patch` receives `/acme/retail/container/workflows/container-1` and exactly `{ Workflows: [{ name: "notify", isActive: true }] }`.
- The test mocks the hook dependencies and request client locally; no production code is changed.
- TenantPanel remains unmodified because it is read-only.

concerns:
- Vitest was not run because applying the patch to the read-only tenantPanel checkout is prohibited.
