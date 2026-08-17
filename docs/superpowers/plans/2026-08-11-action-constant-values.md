# Action Constant Values Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make action-level `constantValues` act as editable defaults for rendered fields and enforced values for fields omitted from the form, consistently across the backend and both frontend runtimes.

**Architecture:** Each frontend gets the same small pure helper that partitions action constants using the final rendered form keys. Form initialization merges visible constants as defaults, while submission merges only hidden constants last. Tenant Panel additionally owns structured constant editing and validation; the Go backend validates persisted keys without trying to infer frontend visibility.

**Tech Stack:** Go 1.x with standard `testing`; React 18 and TypeScript; Vitest; Yarn 4; existing Generic Add/Edit table components.

## Global Constraints

- Preserve all pre-existing uncommitted changes in all three repositories.
- Keep `action.constantValues` as the only persisted representation; do not add hidden-field or submit-level alternatives.
- Values must preserve every JSON-compatible value, including `false`, `0`, `""`, arrays, objects, and `null`.
- Determine visibility from the final effective rendered form keys, not raw `formFields` alone.
- Visible precedence is existing form-field default, configured constant default, existing edit value, then user input.
- Hidden constants merge last and therefore override an existing or submitted record value.
- Delete and Link behavior remains unchanged.
- Paginated and unpaginated implementations must remain behaviorally identical.

---

### Task 1: Backend action-constant validation

**Files:**
- Modify: `models/frontendValidation.go`
- Modify: `models/models_test.go`

**Interfaces:**
- Consumes: `ActionConfig.ConstantValues map[string]interface{}` from `models/containerModel.go`.
- Produces: `ValidateActionConfig(action ActionConfig) error` rejects a constant key whose trimmed form is empty; all JSON-compatible values remain valid.

- [ ] **Step 1: Write the failing validation and round-trip tests**

Add a table-driven test beside the existing action validation tests:

```go
func TestValidateActionConfigConstantValues(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]interface{}
		wantErr string
	}{
		{name: "string value", values: map[string]interface{}{"status": "ACTIVE"}},
		{name: "falsy and null values", values: map[string]interface{}{"enabled": false, "count": 0, "note": "", "parent": nil}},
		{name: "blank key", values: map[string]interface{}{"   ": "ACTIVE"}, wantErr: "constantValues requires non-empty keys"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActionConfig(ActionConfig{Kind: "update", ConstantValues: tt.values})
			if tt.wantErr == "" && err != nil { t.Fatalf("unexpected error: %v", err) }
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
```

Extend the existing table-action BSON/JSON round-trip fixture with `ConstantValues` containing a string, `false`, `0`, and `nil`, then assert those keys survive decoding. The production mutation caught is either skipping key validation or dropping valid falsy values during serialization.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `go test ./models -run 'TestValidateActionConfigConstantValues|TestPageTableAction' -count=1`

Expected: the blank-key case fails because `ValidateActionConfig` currently accepts it.

- [ ] **Step 3: Implement minimal key validation**

In `ValidateActionConfig`, iterate over `action.ConstantValues` and return `fmt.Errorf("constantValues requires non-empty keys")` when `strings.TrimSpace(key) == ""`. Do not reject values based on truthiness or type.

- [ ] **Step 4: Run focused and package tests**

Run: `gofmt -w models/frontendValidation.go models/models_test.go`

Run: `go test ./models -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the backend change**

```bash
git add models/frontendValidation.go models/models_test.go
git commit -m "feat: validate action constant keys"
```

Before committing, inspect `git diff --cached` and ensure the user's unrelated array-source edits are not staged. Use an interactive or narrowly constructed patch if the same files overlap.

---

### Task 2: Pure action-constant precedence helpers in both frontends

**Files:**
- Create: `../tenantPanel/src/utils/actionConstantValues.ts`
- Create: `../tenantPanel/src/utils/actionConstantValues.test.ts`
- Create: `../react-template/src/utils/actionConstantValues.ts`
- Create: `../react-template/src/utils/actionConstantValues.test.ts`

**Interfaces:**
- Produces: `partitionActionConstantValues(constants, effectiveFormKeys): { visibleDefaults; hiddenValues }`.
- Produces: `buildActionInitialValues(fieldDefaults, visibleDefaults, existingValues?): Record<string, unknown>`.
- Produces: `applyHiddenActionValues(payload, hiddenValues): Record<string, unknown>`.
- Effective keys accept `Iterable<string>` so callers can pass form-key arrays or sets.

- [ ] **Step 1: Write identical failing helper tests in both projects**

Use literal expectations:

```ts
it("partitions constants by effective rendered keys without dropping falsy values", () => {
  expect(partitionActionConstantValues(
    { status: "ACTIVE", enabled: false, count: 0, note: "", parent: null },
    ["status", "enabled"],
  )).toEqual({
    visibleDefaults: { status: "ACTIVE", enabled: false },
    hiddenValues: { count: 0, note: "", parent: null },
  });
});

it("lets existing and user values win for visible fields", () => {
  expect(buildActionInitialValues(
    { status: "FORM_DEFAULT" },
    { status: "ACTIVE", enabled: false },
    { status: "PAUSED" },
  )).toEqual({ status: "PAUSED", enabled: false });
});

it("applies hidden values after the submitted payload", () => {
  expect(applyHiddenActionValues(
    { status: "USER_VALUE", name: "Ada" },
    { status: "ACTIVE" },
  )).toEqual({ status: "ACTIVE", name: "Ada" });
});
```

The production mutations caught are classifying from raw config, merging all constants last, or filtering values by truthiness.

- [ ] **Step 2: Run both tests and verify RED**

Run in `tenantPanel`: `yarn test src/utils/actionConstantValues.test.ts`

Run in `react-template`: `yarn test src/utils/actionConstantValues.test.ts`

Expected: FAIL because the module does not exist.

- [ ] **Step 3: Implement the minimal pure helper identically in both projects**

```ts
export type PartitionedActionConstantValues = {
  visibleDefaults: Record<string, unknown>;
  hiddenValues: Record<string, unknown>;
};

export const partitionActionConstantValues = (
  constants: Record<string, unknown> | undefined,
  effectiveFormKeys: Iterable<string>,
): PartitionedActionConstantValues => {
  const visibleKeys = new Set(effectiveFormKeys);
  const visibleDefaults: Record<string, unknown> = {};
  const hiddenValues: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(constants || {})) {
    (visibleKeys.has(key) ? visibleDefaults : hiddenValues)[key] = value;
  }
  return { visibleDefaults, hiddenValues };
};

export const buildActionInitialValues = (
  fieldDefaults: Record<string, unknown>,
  visibleDefaults: Record<string, unknown>,
  existingValues: Record<string, unknown> = {},
) => ({ ...fieldDefaults, ...visibleDefaults, ...existingValues });

export const applyHiddenActionValues = (
  payload: Record<string, unknown>,
  hiddenValues: Record<string, unknown>,
) => ({ ...payload, ...hiddenValues });
```

- [ ] **Step 4: Run both focused tests and builds**

Run in each frontend:

```bash
yarn test src/utils/actionConstantValues.test.ts
yarn build
```

Expected: PASS.

- [ ] **Step 5: Commit separately in each frontend**

Commit message in each repository: `feat: add action constant precedence helpers`.

---

### Task 3: Tenant Panel runtime integration

**Files:**
- Modify: `../tenantPanel/src/utils/tableActions.tsx`
- Create: `../tenantPanel/src/utils/tableActions.test.ts`
- Modify: `../tenantPanel/src/components/panelComponents/FormElements/GenericPaginatedPage.tsx`
- Modify: `../tenantPanel/src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx`

**Interfaces:**
- Produces: `resolveActionConstantValues(action, effectiveFormKeys)` combining `getActionConstantValues` with the Task 2 partition helper.
- Consumes: final `GenericInputType[].formKey` or final `FormKeyType[].key`; bulk Edit uses only the second-step selected value keys, excluding the `bulkSelectedKeys` control.

- [ ] **Step 1: Write failing action-resolution tests**

In `tableActions.test.ts`, test a configured action whose effective inputs are `status` and `name`. Assert that `status` is returned in `visibleDefaults`, a non-rendered `tenantId` is returned in `hiddenValues`, and `false` survives. Also test generated-field actions by passing the keys derived from final action inputs rather than `action.formFields`.

- [ ] **Step 2: Run the focused test and verify RED**

Run: `yarn test src/utils/tableActions.test.ts`

Expected: FAIL because the resolver is missing or currently treats every constant as enforced.

- [ ] **Step 3: Integrate create and configured row actions**

For Add, derive effective keys from `createActionFormKeys`, partition `getActionConstantValues(configuredCreateAction)`, and initialize the panel with:

```ts
buildActionInitialValues(
  createActionDefaults,
  createConstants.visibleDefaults,
)
```

At submit, build the normal user payload and call `applyHiddenActionValues(payload, createConstants.hiddenValues)` before applying existing table/caller constant-filter behavior.

For Edit and custom form actions, initialize with field defaults, visible defaults, then the normalized existing row. On submit, merge the user's updates normally and apply only `hiddenValues` last. Use the same final record for direct mutations and workflow submissions. Non-form Update actions have no effective form keys, so all constants are hidden/enforced.

- [ ] **Step 4: Integrate bulk Edit**

Partition constants against only the actual second-step editable keys. Prepopulate visible constants in `bulkForm` when the second step opens without overwriting later user selections. Merge hidden values into each outgoing bulk update last. Apply the same records to workflow-backed bulk Edit and direct `updateMultipleDynamicItem` paths.

- [ ] **Step 5: Mirror behavior in the unpaginated component**

Use the same helper calls and merge order in `GenericUnpaginatedPage.tsx`; do not copy a new precedence implementation into the component.

- [ ] **Step 6: Run targeted tests, existing table tests, and build**

Run:

```bash
yarn test src/utils/actionConstantValues.test.ts src/utils/tableActions.test.ts src/utils/tableConstantValues.test.ts src/utils/tableConfig.test.ts
yarn build
```

Expected: PASS.

- [ ] **Step 7: Commit Tenant Panel runtime integration**

Commit message: `feat: apply action constants by field visibility`.

---

### Task 4: React Template runtime integration

**Files:**
- Modify: `../react-template/src/utils/tableActions.ts`
- Modify: `../react-template/src/utils/tableActions.test.ts`
- Modify: `../react-template/src/components/panelComponents/FormElements/GenericPaginatedPage.tsx`
- Modify: `../react-template/src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx`

**Interfaces:**
- Consumes the Task 2 helper with the same effective-key and precedence contract as Tenant Panel.
- Produces one shared parser/resolver for action constants so paginated and unpaginated components no longer maintain separate `parseActionConstantValues` implementations.

- [ ] **Step 1: Write failing shared resolver tests**

Move parsing behind a shared utility and test persisted `constantValues`, legacy/designer `constantValuesJson`, invalid JSON returning `{}`, and visible/hidden partitioning using literal expected objects. The production mutations caught are divergence between table modes and treating all constants as enforced.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `yarn test src/utils/tableActions.test.ts src/utils/actionConstantValues.test.ts`

Expected: FAIL because the shared resolver and partition-aware contract do not yet exist.

- [ ] **Step 3: Implement paginated Add, Edit, custom action, workflow, and bulk Edit behavior**

Replace whole-map last merges with the Task 2 partition. Derive effective keys from each final `actionInputs`/`actionFormKeys` pair. Initialize visible values with `buildActionInitialValues`; submit direct mutations and workflow records with `applyHiddenActionValues`. Treat non-form Update constants as fully hidden. Partition bulk values against selected second-step edit keys.

- [ ] **Step 4: Implement identical unpaginated behavior**

Import the same shared resolver and helper. Remove the local parser after both components use the shared utility. Ensure action, workflow, and bulk records match the paginated path.

- [ ] **Step 5: Run targeted tests and build**

Run:

```bash
yarn test src/utils/actionConstantValues.test.ts src/utils/tableActions.test.ts src/utils/tableConstantValues.test.ts src/utils/tableConfig.test.ts
yarn build
```

Expected: PASS.

- [ ] **Step 6: Commit React Template runtime integration**

Commit message: `feat: apply action constants by field visibility`.

---

### Task 5: Structured constant editor and save validation in Tenant Panel

**Files:**
- Create: `../tenantPanel/src/utils/actionConstantEditor.ts`
- Create: `../tenantPanel/src/utils/actionConstantEditor.test.ts`
- Modify: `../tenantPanel/src/components/PageDesigner/PageDesigner.tsx`

**Interfaces:**
- Produces: editor rows `{ id: string; key: string; valueText: string }[]` derived from and serialized to `Record<string, unknown>`.
- Produces: `parseActionConstantRows(rows)` returning `{ ok: true; values: Record<string, unknown> } | { ok: false; errors: Record<string, string> }`.
- Produces: `cleanActionConstantValues(rows)` returning the valid persisted object and refusing invalid rows.
- Every table action editor uses the same component/row renderer: Add, Edit, custom action, and bulk Edit configuration.

- [ ] **Step 1: Write failing editor helper tests**

Test conversion of `{ status: "ACTIVE", enabled: false, count: 0, parent: null, tags: ["a"] }` to editable rows and back. Assert JSON literals parse to their original types, plain unquoted text parses as a string, duplicate/blank keys report row errors, and malformed array/object JSON reports an error instead of becoming `{}`.

- [ ] **Step 2: Run the helper tests and verify RED**

Run: `yarn test src/utils/actionConstantEditor.test.ts`

Expected: FAIL because the helper is missing.

- [ ] **Step 3: Implement the pure editor parser/serializer**

Keep stable row IDs out of the persisted object. Parse recognizable JSON literals with `JSON.parse`; preserve ordinary unquoted text as strings. Reject blank keys, duplicates, and malformed text beginning with `{` or `[`. Serialize all valid rows into `constantValues` without truthiness filtering.

- [ ] **Step 4: Write a failing designer-cleaning test**

In `actionConstantEditor.test.ts`, build representative rows for `addButton`, a configured row action, and `bulkActions.edit`, pass each through `cleanActionConstantValues`, and assert the exact persisted object. Add invalid blank-key input and assert the helper returns the validation failure instead of silently returning no constants.

- [ ] **Step 5: Run the cleaning test and verify RED**

Run: `yarn test src/utils/actionConstantEditor.test.ts`

Expected: FAIL because `cleanActionConstantValues` does not yet exist.

- [ ] **Step 6: Add the reusable structured editor to every applicable action section**

Render key, typed value text, inline error, add-row, and remove-row controls. Label the section “Default / hidden values” and explain: “Rendered fields use these as defaults; fields not in the form are submitted hidden.” Replace the custom-action-only raw JSON textarea. Wire Add, Edit, custom actions, and bulk Edit through the same update functions and persist only `constantValues`.

Disable the Page Designer save action while any constant row is invalid, consistent with existing validation presentation. Do not silently discard invalid rows.

- [ ] **Step 7: Run focused tests and build**

Run:

```bash
yarn test src/utils/actionConstantEditor.test.ts src/utils/tableConfig.test.ts
yarn build
```

Expected: PASS.

- [ ] **Step 8: Commit the designer integration**

Commit message: `feat: configure constants for every record action`.

---

### Task 6: Cross-project verification and parity audit

**Files:**
- Modify only tests or implementation required to fix a demonstrated parity failure.

**Interfaces:**
- Verifies the full contract from `docs/superpowers/specs/2026-08-11-action-constant-values-design.md`.

- [ ] **Step 1: Run complete backend verification**

Run in `autotable-Go`:

```bash
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 2: Run complete Tenant Panel verification**

Run in `tenantPanel`:

```bash
yarn test
yarn build
```

Expected: PASS.

- [ ] **Step 3: Run complete React Template verification**

Run in `react-template`:

```bash
yarn test
yarn build
```

Expected: PASS.

- [ ] **Step 4: Audit the five required record paths**

Using controlled action configurations, verify Add, Edit, custom direct mutation, workflow submission, and bulk Edit in both table data modes. For each path, check these literal scenarios:

1. visible `status` starts at `ACTIVE` but submits the user's `PAUSED` choice;
2. visible edit `status` starts from the row's `INACTIVE` value;
3. hidden `status` submits `ACTIVE` even when the row contained `INACTIVE`;
4. hidden `enabled: false`, `count: 0`, `note: ""`, and `parent: null` survive;
5. actions without constants retain their prior payloads.

- [ ] **Step 5: Review diffs and commit any verification-only fixes**

Check `git diff --check` and `git status --short` in all repositories. Do not stage `react-template/dist/index.html` or any unrelated pre-existing modifications. If verification required fixes, commit them in their owning repository with `fix: preserve action constant precedence`.
