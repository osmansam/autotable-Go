# Tenant Panel Workflow Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add workflow list, add, edit, delete, authentication, authorization, and role controls to the tenantPanel container details modal.

**Architecture:** This is a frontend-first change in `tenantPanel` using existing backend workflow storage and runtime authorization. Add a workflow update hook in `src/utils/api/container.ts`, normalize workflow fields consistently, add an `AddWorkflowModal`, and wire a `Workflows` tab into `ContainerDetailsModal`.

**Tech Stack:** React, TypeScript, Vite, React Query, Tailwind CSS, existing tenantPanel component library, autotable-Go Fiber backend.

## Global Constraints

- Backend workflow behavior must stay unchanged unless frontend verification exposes a real backend contract bug.
- Workflow update requests must call `PATCH /:tenantSlug/:projectSlug/container/workflows/:containerId`.
- Workflow update payload must use the PascalCase key `{ "Workflows": [...] }` because `controllers.WorkflowsUpdate` is tagged as `json:"Workflows"`.
- Workflow items must use camelCase JSON fields matching `models.DynamicWorkflow` JSON tags.
- The editor is JSON-based for `payload`, `conditions`, and `steps`; it is not a visual workflow builder.
- Workflow auth fields must be `isAuthenticated`, `isAuthorized`, and `authorizeRole`.
- Name is required and immutable while editing.
- Validate JSON before save: `payload` must be an object when provided; `conditions` and `steps` must be arrays.

---

## File Structure

- Modify `tenantPanel/src/utils/api/container.ts`
  - Extend `DynamicWorkflow`.
  - Add workflow normalization.
  - Add `UpdateWorkflowsPayload`.
  - Add `useUpdateWorkflows`.
- Create `tenantPanel/src/components/panelComponents/Modals/AddWorkflowModal.tsx`
  - Owns workflow form state, JSON validation, role selector, and submit object construction.
- Modify `tenantPanel/src/components/panelComponents/Modals/ContainerDetailsModal.tsx`
  - Add Workflows tab, list rendering, add/edit/delete handlers, and delete confirmation.
- Optional test additions in `tenantPanel/src/utils/api/container.test.ts`
  - Cover workflow normalization only if the existing test harness can import the helper without large refactoring.

---

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

### Task 2: Add Workflow Modal

**Files:**
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/panelComponents/Modals/AddWorkflowModal.tsx`

**Interfaces:**
- Consumes: `DynamicWorkflow` from `src/utils/api/container`.
- Consumes: `useRoleItems` and `SelectInput` role option pattern from `AddPipelineModal`.
- Produces: `AddWorkflowModal` component with props `{ isOpen: boolean; onClose: () => void; onAddWorkflow: (workflow: DynamicWorkflow) => void; editWorkflow?: DynamicWorkflow | null }`.

- [ ] **Step 1: Create modal component**

Create `src/components/panelComponents/Modals/AddWorkflowModal.tsx` with this structure:

```tsx
import React, { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { FiX } from "react-icons/fi";
import { toast } from "react-toastify";
import { CheckSwitch } from "../../../common/CheckSwitch";
import { OptionType } from "../../../types";
import { DynamicWorkflow } from "../../../utils/api/container";
import { useRoleItems } from "../../../utils/api/roleInfo";
import { GenericButton } from "../FormElements/GenericButton";
import SelectInput from "../FormElements/SelectInput";
import TextInput from "../FormElements/TextInput";

interface AddWorkflowModalProps {
  isOpen: boolean;
  onClose: () => void;
  onAddWorkflow: (workflow: DynamicWorkflow) => void;
  editWorkflow?: DynamicWorkflow | null;
}

const defaultWorkflow: Partial<DynamicWorkflow> = {
  name: "",
  trigger: "manual",
  mode: "transactional",
  isActive: true,
  isAuthenticated: false,
  isAuthorized: false,
  authorizeRole: [],
  description: "",
  returnStep: "",
  outputFields: [],
  timeoutSec: 30,
  stopOnError: true,
  runInTransaction: false,
  payload: {},
  conditions: [],
  steps: [],
};

const triggerOptions: OptionType[] = [
  { value: "manual", label: "Manual" },
  { value: "before_create", label: "Before Create" },
  { value: "after_create", label: "After Create" },
  { value: "before_update", label: "Before Update" },
  { value: "after_update", label: "After Update" },
  { value: "before_delete", label: "Before Delete" },
  { value: "after_delete", label: "After Delete" },
  { value: "cron", label: "Cron" },
];

const modeOptions: OptionType[] = [
  { value: "transactional", label: "Transactional" },
  { value: "outbox", label: "Outbox" },
  { value: "hybrid", label: "Hybrid" },
];

const formatJson = (value: unknown) => JSON.stringify(value ?? {}, null, 2);
const formatJsonArray = (value: unknown) =>
  JSON.stringify(Array.isArray(value) ? value : [], null, 2);

export const AddWorkflowModal: React.FC<AddWorkflowModalProps> = ({
  isOpen,
  onClose,
  onAddWorkflow,
  editWorkflow = null,
}) => {
  const { t } = useTranslation();
  const { data: roleItems = [] } = useRoleItems();
  const roleOptions: OptionType[] = useMemo(
    () => roleItems.map((role) => ({ value: role._id, label: role.name })),
    [roleItems]
  );

  const [workflowData, setWorkflowData] =
    useState<Partial<DynamicWorkflow>>(defaultWorkflow);
  const [payloadJson, setPayloadJson] = useState("{}");
  const [conditionsJson, setConditionsJson] = useState("[]");
  const [stepsJson, setStepsJson] = useState("[]");
  const [jsonErrors, setJsonErrors] = useState<Record<string, string>>({});
  const [formKey, setFormKey] = useState(0);

  useEffect(() => {
    if (!isOpen) return;

    const nextWorkflow = editWorkflow || defaultWorkflow;
    setWorkflowData({
      ...defaultWorkflow,
      ...nextWorkflow,
      authorizeRole: nextWorkflow.authorizeRole || [],
      outputFields: nextWorkflow.outputFields || [],
    });
    setPayloadJson(formatJson(nextWorkflow.payload || {}));
    setConditionsJson(formatJsonArray(nextWorkflow.conditions || []));
    setStepsJson(formatJsonArray(nextWorkflow.steps || []));
    setJsonErrors({});
    setFormKey((prev) => prev + 1);
  }, [editWorkflow, isOpen]);

  const parseJson = (label: string, value: string, expected: "object" | "array") => {
    try {
      const parsed = value.trim() ? JSON.parse(value) : expected === "array" ? [] : {};
      if (expected === "array" && !Array.isArray(parsed)) {
        return { error: t(`${label} must be a JSON array`) };
      }
      if (
        expected === "object" &&
        (Array.isArray(parsed) || parsed === null || typeof parsed !== "object")
      ) {
        return { error: t(`${label} must be a JSON object`) };
      }
      return { value: parsed };
    } catch {
      return { error: t(`${label} has invalid JSON`) };
    }
  };

  const handleSubmit = () => {
    if (!workflowData.name?.trim()) {
      toast.error(t("Workflow name is required"));
      return;
    }

    const payloadResult = parseJson("Payload", payloadJson, "object");
    const conditionsResult = parseJson("Conditions", conditionsJson, "array");
    const stepsResult = parseJson("Steps", stepsJson, "array");
    const errors = {
      ...(payloadResult.error ? { payload: payloadResult.error } : {}),
      ...(conditionsResult.error ? { conditions: conditionsResult.error } : {}),
      ...(stepsResult.error ? { steps: stepsResult.error } : {}),
    };
    setJsonErrors(errors);

    if (Object.keys(errors).length > 0) {
      toast.error(t("Please fix JSON errors before saving"));
      return;
    }

    const workflow: DynamicWorkflow = {
      id: workflowData.id,
      name: workflowData.name.trim(),
      version: workflowData.version,
      trigger: workflowData.trigger || "manual",
      schedule: workflowData.schedule,
      timezone: workflowData.timezone,
      mode: workflowData.mode || "transactional",
      isActive: workflowData.isActive ?? true,
      isAuthenticated: workflowData.isAuthenticated || false,
      isAuthorized: workflowData.isAuthorized || false,
      authorizeRole: workflowData.isAuthorized ? workflowData.authorizeRole || [] : [],
      description: workflowData.description?.trim() || undefined,
      payload: payloadResult.value,
      conditions: conditionsResult.value,
      steps: stepsResult.value,
      stopOnError: workflowData.stopOnError ?? true,
      timeoutSec: workflowData.timeoutSec || undefined,
      returnStep: workflowData.returnStep?.trim() || undefined,
      outputFields: (workflowData.outputFields || []).filter(Boolean),
      runInTransaction: workflowData.runInTransaction || false,
    };

    onAddWorkflow(workflow);
    handleClose();
  };

  const handleClose = () => {
    setWorkflowData(defaultWorkflow);
    setPayloadJson("{}");
    setConditionsJson("[]");
    setStepsJson("[]");
    setJsonErrors({});
    onClose();
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-[60] overflow-y-auto">
      <div className="flex min-h-full items-end justify-center p-4 text-center sm:items-center sm:p-0">
        <div className="fixed inset-0 bg-gray-500 bg-opacity-75 transition-opacity" onClick={handleClose} />
        <div className="relative transform overflow-hidden rounded-lg bg-white px-4 pb-4 pt-5 text-left shadow-xl transition-all sm:my-8 sm:w-full sm:max-w-4xl sm:p-6">
          <div className="mb-6 flex items-start justify-between">
            <h3 className="text-lg font-semibold text-gray-900">
              {editWorkflow ? t("Edit Workflow") : t("Add New Workflow")}
            </h3>
            <button onClick={handleClose} className="text-gray-400 transition-colors hover:text-gray-600">
              <FiX size={20} />
            </button>
          </div>

          <div className="max-h-[70vh] space-y-6 overflow-y-auto">
            <TextInput
              key={`workflow-name-${formKey}`}
              label={t("Workflow Name")}
              type="text"
              value={workflowData.name || ""}
              onChange={(value: string) => setWorkflowData({ ...workflowData, name: value })}
              placeholder={t("e.g., create-order")}
              requiredField={true}
              disabled={!!editWorkflow}
            />

            <TextInput
              label={t("Description")}
              type="text"
              value={workflowData.description || ""}
              onChange={(value: string) => setWorkflowData({ ...workflowData, description: value })}
              placeholder={t("Optional workflow description")}
            />

            <div className="grid gap-4 sm:grid-cols-2">
              <SelectInput
                label={t("Trigger")}
                options={triggerOptions}
                value={triggerOptions.find((option) => option.value === workflowData.trigger) || triggerOptions[0]}
                onChange={(selected) =>
                  setWorkflowData({ ...workflowData, trigger: String((selected as OptionType).value) })
                }
                isMultiple={false}
              />
              <SelectInput
                label={t("Mode")}
                options={modeOptions}
                value={modeOptions.find((option) => option.value === workflowData.mode) || modeOptions[0]}
                onChange={(selected) =>
                  setWorkflowData({ ...workflowData, mode: String((selected as OptionType).value) })
                }
                isMultiple={false}
              />
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <TextInput
                label={t("Return Step")}
                type="text"
                value={workflowData.returnStep || ""}
                onChange={(value: string) => setWorkflowData({ ...workflowData, returnStep: value })}
                placeholder={t("Optional step id or name")}
              />
              <TextInput
                label={t("Output Fields")}
                type="text"
                value={(workflowData.outputFields || []).join(", ")}
                onChange={(value: string) =>
                  setWorkflowData({
                    ...workflowData,
                    outputFields: value.split(",").map((field) => field.trim()).filter(Boolean),
                  })
                }
                placeholder={t("fieldA, fieldB")}
              />
            </div>

            <TextInput
              label={t("Timeout (seconds)")}
              type="number"
              value={workflowData.timeoutSec?.toString() || ""}
              onChange={(value: string) =>
                setWorkflowData({ ...workflowData, timeoutSec: parseInt(value) || undefined })
              }
              placeholder="30"
            />

            {[
              ["Active", "Enable or disable this workflow", "isActive"],
              ["Require Authentication", "Users must be logged in to execute this workflow", "isAuthenticated"],
              ["Require Authorization", "Users must have specific roles to execute this workflow", "isAuthorized"],
              ["Stop On Error", "Stop workflow execution when a step fails", "stopOnError"],
              ["Run In Transaction", "Run write workflow steps in a transaction", "runInTransaction"],
            ].map(([label, description, key]) => (
              <div key={key} className="flex items-center justify-between rounded-lg bg-gray-50 p-3">
                <div>
                  <label className="text-sm font-medium text-gray-700">{t(label)}</label>
                  <p className="text-xs text-gray-500">{t(description)}</p>
                </div>
                <CheckSwitch
                  checked={Boolean((workflowData as any)[key])}
                  onChange={() =>
                    setWorkflowData({
                      ...workflowData,
                      [key]: !Boolean((workflowData as any)[key]),
                      ...(key === "isAuthorized" && workflowData.isAuthorized
                        ? { authorizeRole: [] }
                        : {}),
                    })
                  }
                />
              </div>
            ))}

            {workflowData.isAuthorized && (
              <SelectInput
                label={t("Authorized Roles")}
                options={roleOptions}
                value={roleOptions.filter((option) =>
                  workflowData.authorizeRole?.includes(String(option.value))
                )}
                onChange={(selected) =>
                  setWorkflowData({
                    ...workflowData,
                    authorizeRole: selected
                      ? (selected as OptionType[]).map((option) => String(option.value))
                      : [],
                  })
                }
                placeholder={t("Select roles...")}
                isMultiple={true}
              />
            )}

            {[
              ["Payload", payloadJson, setPayloadJson, "payload"],
              ["Conditions", conditionsJson, setConditionsJson, "conditions"],
              ["Steps", stepsJson, setStepsJson, "steps"],
            ].map(([label, value, setter, key]) => (
              <div key={String(key)}>
                <label className="mb-2 block text-sm font-medium text-gray-700">{t(String(label))}</label>
                <textarea
                  value={String(value)}
                  onChange={(event) => (setter as React.Dispatch<React.SetStateAction<string>>)(event.target.value)}
                  className={`h-40 w-full rounded-lg border px-3 py-2 font-mono text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 ${
                    jsonErrors[String(key)] ? "border-red-300 bg-red-50" : "border-gray-300 bg-gray-50"
                  }`}
                />
                {jsonErrors[String(key)] && (
                  <p className="mt-1 text-xs text-red-600">{jsonErrors[String(key)]}</p>
                )}
              </div>
            ))}
          </div>

          <div className="mt-6 flex justify-end space-x-3">
            <GenericButton variant="outline" onClick={handleClose}>
              {t("Cancel")}
            </GenericButton>
            <GenericButton onClick={handleSubmit}>
              {editWorkflow ? t("Update Workflow") : t("Add Workflow")}
            </GenericButton>
          </div>
        </div>
      </div>
    </div>
  );
};
```

- [ ] **Step 2: Run type check**

Run:

```bash
yarn build
```

Expected: build either passes or reports only integration errors from `ContainerDetailsModal` not importing this component yet.

- [ ] **Step 3: Commit**

```bash
git add src/components/panelComponents/Modals/AddWorkflowModal.tsx
git commit -m "feat: add workflow editor modal"
```

---

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

### Task 4: End-To-End Verification

**Files:**
- Read only unless a bug is found.

**Interfaces:**
- Consumes all previous task outputs.
- Produces verified working tenantPanel build and unchanged backend tests.

- [ ] **Step 1: Check tenantPanel status**

Run:

```bash
git status --short
```

Expected: clean or only intended uncommitted changes.

- [ ] **Step 2: Build tenantPanel**

Run:

```bash
yarn build
```

Expected: PASS.

- [ ] **Step 3: Run relevant frontend tests**

Run:

```bash
yarn test --run src/utils/api/container.test.ts
```

Expected: PASS if the file exists and is supported by Vitest. If no workflow normalization test was added, skip this command and record that `yarn build` is the frontend verification.

- [ ] **Step 4: Run backend tests**

Run in `/Users/osmansamilerdogan/Desktop/autotable-Go`:

```bash
GOCACHE=/private/tmp/autotable-go-build-cache go test ./...
```

Expected: PASS.

- [ ] **Step 5: Final status**

Run in both repos:

```bash
git status --short
```

Expected: clean after commits, or only unrelated user changes.

