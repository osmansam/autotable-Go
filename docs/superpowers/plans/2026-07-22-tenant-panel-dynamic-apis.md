# Tenant Panel Dynamic APIs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-container Dynamic API management to tenantPanel and prevent field edits from erasing Dynamic APIs.

**Architecture:** The backend gets a narrow `dynamicApis` update endpoint and general container updates preserve existing Dynamic APIs. TenantPanel mirrors the existing Pipelines tab/modal pattern with a new Dynamic APIs tab and a typed update hook.

**Tech Stack:** Go/Fiber/MongoDB backend, React/TypeScript/Vite tenantPanel, React Query API hooks.

## Global Constraints

Use existing container controller patterns; do not redesign container storage.
Use the tenantPanel `GenericButton`, `TextInput`, `SelectInput`, and `CheckSwitch` conventions.
Dynamic API management lives inside each container details modal, not a top-level dashboard.
No unrelated backend model or validator changes.

---

### Task 1: Backend Dynamic API Preservation and Endpoint

**Files:**
- Modify: `controllers/containerController.go`
- Modify: `routes/containerRoutes.go`
- Test: `controllers/error_paths_test.go`

**Interfaces:**
- Consumes: `models.DynamicApiModel` and existing container cache/websocket helpers.
- Produces: `UpdateDynamicApis(c *fiber.Ctx) error` and route `PATCH /dynamicApis/:id`.

- [ ] **Step 1: Write the failing tests**

Add an error-path test row to `controllers/error_paths_test.go`:

```go
{name: "dynamic apis", method: http.MethodPatch, path: "/id", handler: UpdateDynamicApis},
```

If a focused preservation test already exists for `UpdateContainer`, add this assertion to it; otherwise add a controller or helper-level test that creates an existing container with `DynamicApis`, applies a field-only update path, and expects `DynamicApis` to remain unchanged.

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./controllers
```

Expected: failure because `UpdateDynamicApis` is undefined or the route behavior is missing.

- [ ] **Step 3: Implement backend changes**

In `UpdateContainer`, after preserving functions/workflows, add:

```go
updatedContainer.DynamicApis = existingContainer.DynamicApis
```

Add a request type if one is not already present:

```go
type DynamicApisUpdate struct {
	DynamicApis []models.DynamicApiModel `json:"dynamicApis"`
}
```

Add `UpdateDynamicApis` by copying the narrow update structure from `UpdateDynamicFunctions`, changing the parsed type and Mongo `$set` field to `dynamicApis`.

In `routes/containerRoutes.go`, register before `/:id`:

```go
containerGroup.Patch("/dynamicApis/:id",
	middlewares.DefaultBodySizeLimit(),
	middlewares.WriteRateLimit(),
	controllers.UpdateDynamicApis,
)
```

- [ ] **Step 4: Run backend tests**

Run:

```bash
go test ./controllers
```

Expected: PASS.

### Task 2: TenantPanel API Hook

**Files:**
- Modify: `../tenantPanel/src/utils/api/container.ts`

**Interfaces:**
- Consumes: `DynamicApiModel`.
- Produces: `UpdateDynamicApisPayload` and `useUpdateDynamicApis()`.

- [ ] **Step 1: Add payload type**

Add:

```ts
export interface UpdateDynamicApisPayload {
  dynamicApis: DynamicApiModel[];
}
```

- [ ] **Step 2: Add update hook**

Add a hook matching `useUpdatePipelines`, with path:

```ts
buildContainerPath(tenantSlug, projectSlug, `/dynamicApis/${id}`)
```

The mutation sends `UpdateDynamicApisPayload`, invalidates `["containers", tenantSlug, projectSlug]` and `["container", tenantSlug, projectSlug, variables.id]`, and shows success/error toasts.

- [ ] **Step 3: Run tenantPanel type/build verification**

Run from `../tenantPanel`:

```bash
yarn build
```

Expected: no TypeScript errors from the new hook.

### Task 3: TenantPanel Dynamic API Modal

**Files:**
- Create: `../tenantPanel/src/components/panelComponents/Modals/AddDynamicApiModal.tsx`

**Interfaces:**
- Consumes: `DynamicApiModel` and role options from `useRoleItems`.
- Produces: `AddDynamicApiModal` with props `{ isOpen, onClose, onAddDynamicApi, editDynamicApi }`.

- [ ] **Step 1: Implement modal**

Create the modal using the `AddPipelineModal` layout. Validate:

```ts
if (!apiData.name?.trim()) toast.error(t("API name is required"));
if (!apiData.url?.trim()) toast.error(t("API URL is required"));
if (!apiData.method?.trim()) toast.error(t("HTTP method is required"));
```

Submit:

```ts
const dynamicApi: DynamicApiModel = {
  name: apiData.name!,
  url: apiData.url!,
  method: apiData.method!,
  dependencies: normalizeDependencies(apiData.dependenciesText || ""),
  isAuthenticated: apiData.isAuthenticated || false,
  isAuthorized: apiData.isAuthorized || false,
  authorizeRole: apiData.authorizeRole || [],
  isActive: apiData.isActive !== undefined ? apiData.isActive : true,
  isRedisCached: apiData.isRedisCached || false,
  cacheTime: apiData.cacheTime || 0,
};
```

- [ ] **Step 2: Run tenantPanel build**

Run:

```bash
yarn build
```

Expected: PASS.

### Task 4: TenantPanel Container Details Tab

**Files:**
- Modify: `../tenantPanel/src/components/panelComponents/Modals/ContainerDetailsModal.tsx`

**Interfaces:**
- Consumes: `AddDynamicApiModal`, `DynamicApiModel`, `useUpdateDynamicApis`.
- Produces: Dynamic APIs tab with add/edit/delete actions.

- [ ] **Step 1: Add state and handlers**

Add `apis` to `viewMode`, add state for modal/edit/delete, and use:

```ts
const { updateDynamicApis, isUpdating: isDynamicApisUpdating } = useUpdateDynamicApis();
```

Add/edit/delete should update the full `container.dynamicApis || []` array through `updateDynamicApis({ id: container.id, payload: { dynamicApis: updatedDynamicApis } })`.

- [ ] **Step 2: Add tab button and content**

Add a tab button labelled `Dynamic APIs`. The content mirrors the Pipelines tab with method, URL, active/cache/auth/dependency badges and empty state.

- [ ] **Step 3: Preserve container sub-configs in general field edits**

If field edit payloads remain broad, include these fields in the payload type and payload object:

```ts
dynamicApis: container.dynamicApis || [],
dynamicFunctions: container.dynamicFunctions || [],
workflows: container.workflows || [],
frontend: container.frontend,
```

The backend preservation fix is still required.

- [ ] **Step 4: Run tenantPanel build**

Run:

```bash
yarn build
```

Expected: PASS.

### Task 5: Full Verification

**Files:**
- No new files.

**Interfaces:**
- Consumes: all previous tasks.
- Produces: verified backend and tenantPanel changes.

- [ ] **Step 1: Format Go files**

Run:

```bash
gofmt -w controllers/containerController.go routes/containerRoutes.go controllers/error_paths_test.go
```

- [ ] **Step 2: Run backend tests**

Run:

```bash
go test ./controllers
```

Expected: PASS.

- [ ] **Step 3: Run tenantPanel build**

Run from `../tenantPanel`:

```bash
yarn build
```

Expected: PASS.
