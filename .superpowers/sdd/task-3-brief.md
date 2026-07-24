### Task 3: Wire Workflows Tab Into Container Details

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/panelComponents/Modals/ContainerDetailsModal.tsx`

**Interfaces:**
- Consumes: `AddWorkflowModal`.
- Consumes: `DynamicWorkflow` and `useUpdateWorkflows`.
- Produces: `Workflows` view mode with list, add, edit, delete.

- [ ] **Step 1: Update imports**

In `ContainerDetailsModal.tsx`, add `FiPlayCircle` to the react-icons import, add `DynamicWorkflow` and `useUpdateWorkflows` to the container API import, and import the modal:

```ts
import {
  ContainerModel,
  DynamicApiModel,
  DynamicWorkflow,
  Field,
  PipelineStage,
  useCreateProjectAuthUser,
  useUpdateContainer,
  useUpdateDynamicApis,
  useUpdatePipelines,
  useUpdateWorkflows,
} from "../../../utils/api/container";
import { AddWorkflowModal } from "./AddWorkflowModal";
```

- [ ] **Step 2: Extend view state and workflow state**

Change the `viewMode` union to include `"workflows"`. Add state near pipeline state:

```ts
const [isAddWorkflowModalOpen, setIsAddWorkflowModalOpen] = useState(false);
const [editingWorkflow, setEditingWorkflow] =
  useState<DynamicWorkflow | null>(null);
const [workflowToDelete, setWorkflowToDelete] = useState<string | null>(null);
```

Add the hook near pipeline hook:

```ts
const { updateWorkflows, isUpdating: isWorkflowsUpdating } =
  useUpdateWorkflows();
```

- [ ] **Step 3: Add workflow handlers**

After pipeline handlers, add:

```ts
const handleAddWorkflow = useCallback(
  (workflow: DynamicWorkflow) => {
    if (!container?.id) return;

    const currentWorkflows = container.workflows || [];
    if (
      !editingWorkflow &&
      currentWorkflows.some((existing) => existing.name === workflow.name)
    ) {
      toast.error(t("A workflow with this name already exists"));
      return;
    }

    const updatedWorkflows = editingWorkflow
      ? currentWorkflows.map((existing) =>
          existing.name === editingWorkflow.name ? workflow : existing
        )
      : [...currentWorkflows, workflow];

    updateWorkflows({
      id: container.id,
      payload: { Workflows: updatedWorkflows },
    });
    setIsAddWorkflowModalOpen(false);
    setEditingWorkflow(null);
  },
  [container, editingWorkflow, t, updateWorkflows]
);

const handleEditWorkflow = useCallback((workflow: DynamicWorkflow) => {
  setEditingWorkflow(workflow);
  setIsAddWorkflowModalOpen(true);
}, []);

const handleDeleteWorkflow = useCallback((workflowName: string) => {
  setWorkflowToDelete(workflowName);
}, []);

const confirmDeleteWorkflow = useCallback(() => {
  if (!container?.id || !workflowToDelete) return;

  updateWorkflows({
    id: container.id,
    payload: {
      Workflows: (container.workflows || []).filter(
        (workflow) => workflow.name !== workflowToDelete
      ),
    },
  });
  setWorkflowToDelete(null);
}, [container, updateWorkflows, workflowToDelete]);
```

- [ ] **Step 4: Add Workflows tab button**

Add this button after the Pipelines button:

```tsx
<button
  onClick={() => setViewMode("workflows")}
  className={`flex items-center space-x-1 px-3 py-1 text-xs font-medium rounded ${
    viewMode === "workflows"
      ? "bg-white text-blue-600 shadow-sm"
      : "text-gray-600 hover:text-gray-900"
  }`}
>
  <FiPlayCircle size={12} />
  <span>{t("Workflows")}</span>
</button>
```

- [ ] **Step 5: Include workflows in scroll-height condition**

Update the content class condition to include:

```ts
viewMode === "workflows" ||
```

- [ ] **Step 6: Add workflows content branch**

Add a `viewMode === "workflows"` branch between pipelines and APIs. It should render the same list pattern as pipelines, with this row body:

```tsx
<div className="space-y-4 h-full overflow-y-auto">
  <div className="flex items-center justify-between sticky top-0 bg-white pb-4 border-b">
    <div>
      <h4 className="text-sm font-medium text-gray-900">
        {t("Workflows")} ({(container.workflows || []).length})
      </h4>
      <p className="text-xs text-gray-500 mt-1">
        {t("Manage workflow definitions and access controls for this container")}
      </p>
    </div>
    <GenericButton
      variant="outline"
      size="sm"
      onClick={() => setIsAddWorkflowModalOpen(true)}
      iconLeft={<FiPlus size={12} />}
      disabled={isWorkflowsUpdating}
    >
      {t("Add Workflow")}
    </GenericButton>
  </div>

  <div className="space-y-3">
    {(container.workflows || []).map((workflow, index) => (
      <div
        key={workflow.name || index}
        className="bg-gray-50 rounded-lg p-4 hover:bg-gray-100 transition-colors"
      >
        <div className="flex items-start justify-between">
          <div className="flex-1">
            <div className="mb-2 flex flex-wrap items-center gap-2">
              <span className="font-medium text-gray-900">{workflow.name}</span>
              <span className={`inline-flex px-2 py-0.5 text-xs font-medium rounded ${
                workflow.isActive ? "bg-green-100 text-green-800" : "bg-gray-100 text-gray-800"
              }`}>
                {workflow.isActive ? t("Active") : t("Inactive")}
              </span>
              <span className="inline-flex px-2 py-0.5 text-xs font-medium rounded bg-blue-100 text-blue-800">
                {workflow.trigger || "manual"}
              </span>
              <span className="inline-flex px-2 py-0.5 text-xs font-medium rounded bg-gray-100 text-gray-800">
                {workflow.mode || "transactional"}
              </span>
              {workflow.isAuthenticated && (
                <span className="inline-flex px-2 py-0.5 text-xs font-medium rounded bg-purple-100 text-purple-800">
                  {t("Auth Required")}
                </span>
              )}
              {workflow.isAuthorized && (
                <span className="inline-flex px-2 py-0.5 text-xs font-medium rounded bg-orange-100 text-orange-800">
                  {t("Role Check")}
                </span>
              )}
              {!!workflow.outputFields?.length && (
                <span className="inline-flex px-2 py-0.5 text-xs font-medium rounded bg-indigo-100 text-indigo-800">
                  {workflow.outputFields.length} {t("Output Fields")}
                </span>
              )}
            </div>

            {workflow.description && (
              <p className="mb-2 text-xs text-gray-600">{workflow.description}</p>
            )}

            <details className="text-xs">
              <summary className="cursor-pointer font-medium text-gray-600 hover:text-gray-900">
                {t("View Workflow JSON")}
              </summary>
              <pre className="mt-2 overflow-x-auto rounded bg-gray-900 p-3 text-xs text-green-400">
                {JSON.stringify(
                  {
                    payload: workflow.payload || {},
                    conditions: workflow.conditions || [],
                    steps: workflow.steps || [],
                  },
                  null,
                  2
                )}
              </pre>
            </details>

            {workflow.isAuthorized &&
              workflow.authorizeRole &&
              workflow.authorizeRole.length > 0 && (
                <div className="mt-2">
                  <span className="text-xs text-gray-500">
                    {t("Allowed Roles")}:{" "}
                  </span>
                  <span className="text-xs text-gray-700">
                    {workflow.authorizeRole.join(", ")}
                  </span>
                </div>
              )}
          </div>

          <div className="ml-4 flex items-center space-x-2">
            <GenericButton
              variant="outline"
              size="sm"
              onClick={() => handleEditWorkflow(workflow)}
              iconLeft={<FiEdit size={10} />}
              disabled={isWorkflowsUpdating}
            >
              {t("Edit")}
            </GenericButton>
            <GenericButton
              variant="outline"
              size="sm"
              onClick={() => handleDeleteWorkflow(workflow.name)}
              iconLeft={<FiTrash2 size={10} />}
              disabled={isWorkflowsUpdating}
            >
              {t("Delete")}
            </GenericButton>
          </div>
        </div>
      </div>
    ))}

    {(!container.workflows || container.workflows.length === 0) && (
      <div className="py-12 text-center text-gray-500">
        <FiPlayCircle size={48} className="mx-auto mb-4 text-gray-300" />
        <p className="mb-2">{t("No workflows defined for this container")}</p>
        <GenericButton
          variant="outline"
          size="sm"
          onClick={() => setIsAddWorkflowModalOpen(true)}
          iconLeft={<FiPlus size={12} />}
          className="mt-2"
        >
          {t("Add Your First Workflow")}
        </GenericButton>
      </div>
    )}
  </div>
</div>
```

- [ ] **Step 7: Mount modal and delete confirmation**

Near the pipeline modal, add:

```tsx
<AddWorkflowModal
  isOpen={isAddWorkflowModalOpen}
  onClose={() => {
    setIsAddWorkflowModalOpen(false);
    setEditingWorkflow(null);
  }}
  onAddWorkflow={handleAddWorkflow}
  editWorkflow={editingWorkflow}
/>
```

Near the pipeline delete confirmation, add:

```tsx
<ConfirmationDialog
  isOpen={!!workflowToDelete}
  close={() => setWorkflowToDelete(null)}
  confirm={confirmDeleteWorkflow}
  title={t("Delete Workflow")}
  text={t(
    "Are you sure you want to delete this workflow? This action cannot be undone."
  )}
/>
```

- [ ] **Step 8: Run type check**

Run:

```bash
yarn build
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add src/components/panelComponents/Modals/ContainerDetailsModal.tsx
git commit -m "feat: manage workflows in container modal"
```

---

