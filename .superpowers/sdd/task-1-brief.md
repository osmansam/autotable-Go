### Task 1: Workflow API Types And Update Hook

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/container.ts`
- Test: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/container.test.ts`

**Interfaces:**
- Produces: `DynamicWorkflow` with auth fields and workflow metadata.
- Produces: `normalizeDynamicWorkflow(workflow: any): DynamicWorkflow`.
- Produces: `UpdateWorkflowsPayload`.
- Produces: `useUpdateWorkflows(): { updateWorkflows(params: { id: string; payload: UpdateWorkflowsPayload }): void; isUpdating: boolean }`.
- Consumes: `buildContainerPath`, `axiosClient`, `useContainerContext`, existing React Query invalidation pattern.

- [ ] **Step 1: Add/extend workflow types**

In `src/utils/api/container.ts`, replace the current `DynamicWorkflow` interface with:

```ts
export interface DynamicWorkflow {
  id?: string;
  name: string;
  version?: number;
  trigger?: string;
  schedule?: string;
  timezone?: string;
  mode?: string;
  isActive: boolean;
  isAuthenticated?: boolean;
  isAuthorized?: boolean;
  authorizeRole?: string[];
  description?: string;
  payload?: Record<string, any>;
  conditions?: WorkflowCondition[];
  steps?: WorkflowStep[];
  stopOnError?: boolean;
  timeoutSec?: number;
  returnStep?: string;
  outputFields?: string[];
  runInTransaction?: boolean;
}

export interface WorkflowCondition {
  field?: string;
  operator: string;
  value?: any;
  conditions?: WorkflowCondition[];
}
```

- [ ] **Step 2: Add workflow update payload type**

Near `UpdatePipelinesPayload`, add:

```ts
export interface UpdateWorkflowsPayload {
  Workflows: DynamicWorkflow[];
}
```

- [ ] **Step 3: Add workflow normalization helper**

Near `normalizeDynamicApiModel`, add:

```ts
export function normalizeDynamicWorkflow(workflow: any): DynamicWorkflow {
  return {
    id: workflow.ID || workflow.id,
    name: workflow.Name || workflow.name || "",
    version: workflow.Version ?? workflow.version,
    trigger: workflow.Trigger || workflow.trigger || "manual",
    schedule: workflow.Schedule || workflow.schedule,
    timezone: workflow.Timezone || workflow.timezone,
    mode: workflow.Mode || workflow.mode || "transactional",
    isActive: workflow.IsActive ?? workflow.isActive ?? true,
    isAuthenticated:
      workflow.IsAuthenticated ?? workflow.isAuthenticated ?? false,
    isAuthorized: workflow.IsAuthorized ?? workflow.isAuthorized ?? false,
    authorizeRole: workflow.AuthorizeRole || workflow.authorizeRole || [],
    description: workflow.Description || workflow.description,
    payload: workflow.Payload || workflow.payload,
    conditions: workflow.Conditions || workflow.conditions || [],
    steps: workflow.Steps || workflow.steps || [],
    stopOnError: workflow.StopOnError ?? workflow.stopOnError ?? true,
    timeoutSec: workflow.TimeoutSec ?? workflow.timeoutSec,
    returnStep: workflow.ReturnStep || workflow.returnStep,
    outputFields: workflow.OutputFields || workflow.outputFields || [],
    runInTransaction:
      workflow.RunInTransaction ?? workflow.runInTransaction ?? false,
  };
}
```

- [ ] **Step 4: Use workflow normalization in container loaders**

In both `useContainers` and `useContainer`, replace:

```ts
workflows: container.Workflows || container.workflows || [],
```

with:

```ts
workflows: (container.Workflows || container.workflows || []).map(
  normalizeDynamicWorkflow
),
```

- [ ] **Step 5: Add update hook**

After `useUpdatePipelines`, add:

```ts
// Workflows operations
export function useUpdateWorkflows() {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const { tenantSlug, projectSlug } = useContainerContext();

  const updateMutation = useMutation({
    mutationFn: async ({
      id,
      payload,
    }: {
      id: string;
      payload: UpdateWorkflowsPayload;
    }) => {
      const path = buildContainerPath(
        tenantSlug,
        projectSlug,
        `/workflows/${id}`
      );
      const response = await axiosClient.patch(path, payload);
      return response.data;
    },
    onSuccess: (response, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["containers", tenantSlug, projectSlug],
      });
      queryClient.invalidateQueries({
        queryKey: ["container", tenantSlug, projectSlug, variables.id],
      });

      const message = response?.message || "Workflows updated successfully";
      toast.success(t(message));
    },
    onError: (error: any) => {
      console.error("Workflows update failed:", error);
      const errorMessage =
        error?.response?.data?.message ||
        error?.message ||
        "Failed to update workflows";
      toast.error(t(errorMessage));
    },
  });

  return {
    updateWorkflows: (params: {
      id: string;
      payload: UpdateWorkflowsPayload;
    }) => {
      updateMutation.mutate(params);
    },
    isUpdating: updateMutation.isPending,
  };
}
```

- [ ] **Step 6: Run type check**

Run in `/Users/osmansamilerdogan/Desktop/tenantPanel`:

```bash
yarn build
```

Expected: build completes, or failures are limited to the missing UI wiring that later tasks add.

- [ ] **Step 7: Commit**

```bash
git add src/utils/api/container.ts src/utils/api/container.test.ts
git commit -m "feat: add workflow container api hook"
```

---

