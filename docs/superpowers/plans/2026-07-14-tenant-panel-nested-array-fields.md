# Tenant Panel Nested Array Fields Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add tenantPanel schema-editor support for child fields on container `object` and `array` fields.

**Architecture:** Reuse the existing `Field.children` model and the current `AddFieldModal` state. Add a focused inline child-field builder to `AddFieldModal`, then render saved child fields in `ContainerDetailsModal` so users can review nested schema structure after saving.

**Tech Stack:** React, TypeScript, Vite, existing tenantPanel components (`TextInput`, `SelectInput`, `CheckSwitch`, `GenericButton`), existing container API types.

## Global Constraints

- Scope is tenantPanel schema editing only.
- Do not change runtime create/edit forms in tenant applications.
- Reuse the existing `Field` shape and `children?: Field[]`.
- Support one visible nested level in the UI for this pass.
- Prevent missing child names, missing child types, and duplicate child names under the same parent.
- Build tenantPanel successfully before completion.

---

### Task 1: Add Child Field Editing Helpers To AddFieldModal

**Files:**
- Modify: `../tenantPanel/src/components/panelComponents/Modals/AddFieldModal.tsx`

**Interfaces:**
- Consumes: existing `Field` type from `../../../utils/api/container`.
- Produces:
  - `canHaveChildFields: boolean`
  - `childFieldDraft: Partial<Field>`
  - `editingChildFieldIndex: number | null`
  - helper functions:
    - `resetChildFieldDraft(): void`
    - `handleChildFieldDraftChange(field: string, value: any): void`
    - `handleSaveChildField(): void`
    - `handleEditChildField(index: number): void`
    - `handleRemoveChildField(index: number): void`
    - `parseEnumText(value: string): string[]`

- [ ] **Step 1: Add child draft state and helper constants**

In `AddFieldModal.tsx`, keep the existing imports and add these state values near the existing `childFields` state:

```tsx
const [childFieldDraft, setChildFieldDraft] = useState<Partial<Field>>({
  name: "",
  type: "string",
  tag: "",
  unique: false,
  isSearchable: true,
  enumList: [],
});
const [childEnumValues, setChildEnumValues] = useState<string>("");
const [editingChildFieldIndex, setEditingChildFieldIndex] = useState<number | null>(null);

const canHaveChildFields =
  fieldData.type === "object" || fieldData.type === "array";
```

- [ ] **Step 2: Add draft reset and update helpers**

Add these functions after `handleFieldChange`:

```tsx
const resetChildFieldDraft = () => {
  setChildFieldDraft({
    name: "",
    type: "string",
    tag: "",
    unique: false,
    isSearchable: true,
    enumList: [],
  });
  setChildEnumValues("");
  setEditingChildFieldIndex(null);
};

const handleChildFieldDraftChange = (field: string, value: any) => {
  setChildFieldDraft((prev) => ({ ...prev, [field]: value }));
};

const parseEnumText = (value: string): string[] =>
  value
    .split("|")
    .map((item) => item.trim())
    .filter(Boolean);
```

- [ ] **Step 3: Add save, edit, and remove helpers**

Add these functions after `parseEnumText`:

```tsx
const handleSaveChildField = () => {
  const name = childFieldDraft.name?.trim();
  const type = childFieldDraft.type?.trim();

  if (!name) {
    toast.error(t("Child field name is required"));
    return;
  }

  if (!type) {
    toast.error(t("Child field type is required"));
    return;
  }

  const duplicate = childFields.some(
    (field, index) =>
      field.name === name && index !== editingChildFieldIndex
  );

  if (duplicate) {
    toast.error(t("Child field name already exists"));
    return;
  }

  const nextField: Field = {
    name,
    type,
    tag: childFieldDraft.tag || "",
    unique: childFieldDraft.unique || false,
    isSearchable: childFieldDraft.isSearchable || false,
    isLoginCredential: false,
    isHashed: false,
    isForceDelete: false,
    enumList:
      type === "enum" && childEnumValues
        ? parseEnumText(childEnumValues)
        : undefined,
  };

  setChildFields((prev) => {
    if (editingChildFieldIndex === null) {
      return [...prev, nextField];
    }
    return prev.map((field, index) =>
      index === editingChildFieldIndex ? nextField : field
    );
  });

  resetChildFieldDraft();
};

const handleEditChildField = (index: number) => {
  const field = childFields[index];
  if (!field) return;

  setChildFieldDraft({
    name: field.name,
    type: field.type,
    tag: field.tag || "",
    unique: field.unique || false,
    isSearchable: field.isSearchable || false,
    enumList: field.enumList,
  });
  setChildEnumValues(field.enumList?.join("|") || "");
  setEditingChildFieldIndex(index);
};

const handleRemoveChildField = (index: number) => {
  setChildFields((prev) => prev.filter((_field, fieldIndex) => fieldIndex !== index));
  if (editingChildFieldIndex === index) {
    resetChildFieldDraft();
  }
};
```

- [ ] **Step 4: Clear child fields when parent type no longer supports them**

Inside the `Field Type` `onChange`, after updating `type`, clear child state for non-`object` and non-`array` values:

```tsx
const nextType = selectedValue?.value || "";
handleFieldChange("type", nextType);
if (nextType !== "object" && nextType !== "array") {
  setChildFields([]);
  resetChildFieldDraft();
}
```

- [ ] **Step 5: Ensure parent submit omits children for unsupported types**

Update the `children` assignment in `newField`:

```tsx
children:
  canHaveChildFields && childFields.length > 0 ? childFields : undefined,
```

### Task 2: Render Child Field Builder In AddFieldModal

**Files:**
- Modify: `../tenantPanel/src/components/panelComponents/Modals/AddFieldModal.tsx`

**Interfaces:**
- Consumes helpers from Task 1.
- Produces visible UI for adding, editing, and deleting child fields on parent `object` and `array` fields.

- [ ] **Step 1: Add child field section after enum/equation controls**

In the form body after the existing enum/equation blocks and before "Field Properties", add:

```tsx
{canHaveChildFields && (
  <div className="border-t pt-4">
    <div className="flex items-center justify-between mb-3">
      <div>
        <h4 className="text-sm font-medium text-gray-900">
          {t("Child Fields")}
        </h4>
        <p className="text-xs text-gray-500 mt-1">
          {fieldData.type === "array"
            ? t("Each child field describes one object inside this array")
            : t("Child fields describe the nested object structure")}
        </p>
      </div>
      {editingChildFieldIndex !== null && (
        <GenericButton variant="outline" size="sm" onClick={resetChildFieldDraft}>
          {t("Cancel Edit")}
        </GenericButton>
      )}
    </div>

    <div className="grid grid-cols-2 gap-3 rounded-lg bg-gray-50 p-3">
      <TextInput
        label={t("Child Field Name")}
        type="text"
        value={childFieldDraft.name || ""}
        onChange={(value: string) => handleChildFieldDraftChange("name", value)}
        placeholder="e.g., quantity"
      />
      <SelectInput
        label={t("Child Field Type")}
        value={FIELD_TYPES.find((option) => option.value === childFieldDraft.type) || null}
        onChange={(value) => {
          const selectedValue = value as { value: string; label: string } | null;
          handleChildFieldDraftChange("type", selectedValue?.value || "");
          if (selectedValue?.value !== "enum") {
            setChildEnumValues("");
          }
        }}
        options={FIELD_TYPES.filter(
          (option) => option.value !== "object" && option.value !== "array"
        )}
        customControlBackgroundColor="white"
      />
      <TextInput
        label={t("Validation Tag")}
        type="text"
        value={childFieldDraft.tag || ""}
        onChange={(value: string) => handleChildFieldDraftChange("tag", value)}
        placeholder='e.g., required,positive'
      />
      {childFieldDraft.type === "enum" && (
        <TextInput
          label={t("Enum Values")}
          type="text"
          value={childEnumValues}
          onChange={setChildEnumValues}
          placeholder="small|medium|large"
        />
      )}
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-2">
          {t("Unique")}
        </label>
        <CheckSwitch
          checked={childFieldDraft.unique || false}
          onChange={() =>
            handleChildFieldDraftChange("unique", !childFieldDraft.unique)
          }
        />
      </div>
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-2">
          {t("Searchable")}
        </label>
        <CheckSwitch
          checked={childFieldDraft.isSearchable || false}
          onChange={() =>
            handleChildFieldDraftChange(
              "isSearchable",
              !childFieldDraft.isSearchable
            )
          }
        />
      </div>
      <div className="col-span-2 flex justify-end">
        <GenericButton
          variant="outline"
          size="sm"
          onClick={handleSaveChildField}
          iconLeft={<FiPlus size={12} />}
        >
          {editingChildFieldIndex === null
            ? t("Add Child Field")
            : t("Update Child Field")}
        </GenericButton>
      </div>
    </div>

    <div className="mt-3 space-y-2">
      {childFields.map((childField, index) => (
        <div
          key={`${childField.name}-${index}`}
          className="flex items-center justify-between rounded-lg border border-gray-200 bg-white p-3"
        >
          <div>
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium text-gray-900">
                {childField.name}
              </span>
              <span className="rounded bg-gray-100 px-2 py-1 text-xs font-medium text-gray-700">
                {childField.type}
              </span>
            </div>
            {childField.tag && (
              <p className="mt-1 text-xs text-gray-500">
                {t("Tag")}: {childField.tag}
              </p>
            )}
          </div>
          <div className="flex items-center gap-2">
            <GenericButton
              variant="outline"
              size="sm"
              onClick={() => handleEditChildField(index)}
            >
              {t("Edit")}
            </GenericButton>
            <button
              type="button"
              onClick={() => handleRemoveChildField(index)}
              className="p-1.5 text-red-500 hover:text-red-700"
              title={t("Remove Child Field")}
            >
              <FiTrash2 size={16} />
            </button>
          </div>
        </div>
      ))}

      {childFields.length === 0 && (
        <p className="text-sm text-gray-500 italic">
          {t("No child fields added")}
        </p>
      )}
    </div>
  </div>
)}
```

- [ ] **Step 2: Verify modal behavior manually in browser or by component inspection**

Expected behavior:

- Parent type `string` does not show the child section.
- Parent type `object` shows the child section.
- Parent type `array` shows the child section.
- Adding a child field adds it to the list without closing the parent modal.
- Editing a child field preloads it into the child draft.
- Saving the parent includes `children`.

### Task 3: Display Nested Child Fields In ContainerDetailsModal

**Files:**
- Modify: `../tenantPanel/src/components/panelComponents/Modals/ContainerDetailsModal.tsx`

**Interfaces:**
- Consumes: `Field.children` saved by `AddFieldModal`.
- Produces: visible nested child field list under parent fields in container details.

- [ ] **Step 1: Add a small render helper near `getFieldTypeColor`**

Add this helper inside the component, after `getFieldTypeColor`:

```tsx
const renderChildFields = (children: Field[] = []) => {
  if (!children.length) return null;

  return (
    <div className="mt-3 space-y-2 border-l-2 border-gray-200 pl-3">
      {children.map((child, childIndex) => (
        <div
          key={`${child.name}-${childIndex}`}
          className="rounded bg-white px-3 py-2"
        >
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-medium text-gray-800">
              {child.name}
            </span>
            <span
              className={`inline-flex rounded px-2 py-1 text-xs font-medium ${getFieldTypeColor(
                child.type
              )}`}
            >
              {child.type}
            </span>
            {child.unique && (
              <span className="inline-flex rounded bg-indigo-100 px-2 py-1 text-xs font-medium text-indigo-800">
                {t("Unique")}
              </span>
            )}
            {child.isSearchable && (
              <span className="inline-flex rounded bg-teal-100 px-2 py-1 text-xs font-medium text-teal-800">
                {t("Searchable")}
              </span>
            )}
          </div>
          {child.tag && (
            <p className="mt-1 text-xs text-gray-500">
              {t("Tag")}: {child.tag}
            </p>
          )}
        </div>
      ))}
    </div>
  );
};
```

- [ ] **Step 2: Render children under each parent field**

Inside the existing field card, after the `Object Schema` paragraph, add:

```tsx
{renderChildFields(field.children || [])}
```

- [ ] **Step 3: Verify display manually**

Expected behavior:

- Fields without children look unchanged.
- `array` and `object` fields with children show nested rows.
- The nested list does not interfere with edit/delete/move buttons.

### Task 4: Verification

**Files:**
- Verify: `../tenantPanel`

**Interfaces:**
- Consumes all prior tasks.
- Produces a build-verified tenantPanel implementation.

- [ ] **Step 1: Run tenantPanel build**

Run:

```bash
yarn build
```

Working directory:

```bash
../tenantPanel
```

Expected: build completes successfully.

- [ ] **Step 2: Review changed files**

Run:

```bash
git diff -- ../tenantPanel/src/components/panelComponents/Modals/AddFieldModal.tsx ../tenantPanel/src/components/panelComponents/Modals/ContainerDetailsModal.tsx
```

Expected:

- `AddFieldModal` has inline child-field editing.
- `ContainerDetailsModal` displays child fields.
- No runtime form files changed.

- [ ] **Step 3: Commit tenantPanel changes if requested**

Only commit if the user asks for a commit. Otherwise leave changes in the worktree.
