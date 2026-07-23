# Tenant Panel Workflow Management Design

Date: 2026-07-23

## Goal

Add workflow management to the container details modal in `tenantPanel`, next to the existing Pipelines, Dynamic APIs, Permissions, Routes, Structured, and JSON views. Admin users should be able to add a workflow to a container, see existing workflows, edit or delete them, and manage workflow authentication and role authorization from the same place.

## Existing Context

`autotable-Go` already persists workflows on `ContainerModel.Workflows` and exposes `PATCH /:tenantSlug/:projectSlug/container/workflows/:id` through `controllers.UpdateWorkflows`. That endpoint validates workflow definitions with `services.ValidateWorkflows` and updates only the `workflows` field.

Runtime workflow access is already enforced by `middlewares.ConditionalAuthentication("ExecuteWorkflow")`. For `POST /dynamic/workflow/:workflowName`, the middleware reads the matching workflow's `isAuthenticated`, `isAuthorized`, `isActive`, and `authorizeRole` fields. This means the tenantPanel UI only needs to edit those fields correctly; backend route behavior is already aligned.

tenantPanel already has a pattern for item-level auth controls in `AddPipelineModal` and `AddDynamicApiModal`, and container route permissions are handled in `RoutePermissions`. Workflow management should follow those patterns.

## Scope

Implement a workflow tab in `ContainerDetailsModal` with list, add, edit, and delete operations. The workflow editor will be JSON-based for workflow body details, not a full visual builder.

The workflow editor will support:

- `name`
- `trigger`
- `mode`
- `isActive`
- `isAuthenticated`
- `isAuthorized`
- `authorizeRole`
- `description`
- `returnStep`
- `outputFields`
- `timeoutSec`
- `stopOnError`
- `runInTransaction`
- JSON sections for `payload`, `conditions`, and `steps`

The initial implementation will not include a drag-and-drop or wizard-based workflow builder. That can be added later without changing the backend contract.

## UX

The container details modal will add a `Workflows` tab near `Pipelines`. The tab header shows the workflow count and an add button.

Each workflow row shows:

- Name and description when present
- Active or inactive status
- Trigger and mode
- Auth Required and Role Check badges
- Output fields count when configured
- Expandable JSON preview for `steps`, `conditions`, and `payload`
- Edit and delete actions
- Allowed roles when authorization is enabled

The add/edit modal mirrors the pipeline modal style:

- Name is required and immutable while editing
- Trigger and mode use select controls
- Auth and authorization use switches
- Authorized roles use the existing role selector
- JSON textareas validate their expected shapes before save
- Submit builds a `DynamicWorkflow` object that matches the Go model's camelCase JSON tags

## Data Flow

tenantPanel will add an `UpdateWorkflowsPayload` type and `useUpdateWorkflows` hook in `src/utils/api/container.ts`. The hook calls:

`PATCH /:tenantSlug/:projectSlug/container/workflows/:containerId`

with:

```json
{
  "Workflows": []
}
```

This PascalCase key matches `controllers.WorkflowsUpdate`.

`useContainers` and `useContainer` will normalize workflow fields, including auth fields, so list and editor state stay consistent whether the backend returns PascalCase or camelCase.

## Backend Changes

No backend behavior change is required for the first pass. Backend workflow storage, validation, cache invalidation, WebSocket change emission, and runtime authorization checks already exist.

If testing reveals tenantPanel sends only camelCase `workflows` to the workflow update endpoint, the frontend hook must keep sending `Workflows` rather than changing backend parsing.

## Error Handling

The workflow modal validates:

- Required workflow name
- Valid JSON for payload, conditions, and steps
- `conditions` and `steps` must be arrays
- `payload` must be an object when provided
- `outputFields` input is split into non-empty field names

Backend validation errors from `UpdateWorkflows` are surfaced through the existing toast pattern in container API hooks.

## Testing

Frontend verification:

- TypeScript build for tenantPanel
- Focused tests if existing test setup covers container API normalization or workflow helper logic

Backend verification:

- Existing Go workflow validation and route auth tests should continue to pass
- No new backend tests are required unless backend code changes become necessary

Manual UI verification:

- Open container details
- Switch to Workflows
- Add a manual workflow with one simple step
- Edit auth/role settings
- Confirm it appears in the list with badges and allowed roles
- Delete the workflow

