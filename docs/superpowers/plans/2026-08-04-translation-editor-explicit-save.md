# Translation Editor Explicit Save Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace TenantPanel’s invisible save-on-blur translation editing with an explicit Save/Saving/Saved workflow that persists manual translations and reports failures.

**Architecture:** A focused `TranslationEditorRow` owns the controlled draft and per-row request state. The existing localization mutation performs the PATCH and query invalidation. The backend continues decoding URL-encoded translation keys before MongoDB lookup.

**Tech Stack:** React, TypeScript, TanStack Query, React Toastify, Vitest, Go, Fiber.

## Global Constraints

- Empty translations cannot be saved.
- Failed saves retain the user’s draft.
- Successful AI-origin edits become manual.
- Manual translations remain protected from AI overwrite.
- Save-on-blur is removed.

---

### Task 1: Translation Row State and Explicit Save UI

**Files:**
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/localization/translationRowState.ts`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/localization/translationRowState.test.ts`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/localization/TranslationEditorRow.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/components/localization/ProjectLocalizationSection.tsx`

**Interfaces:**
- Consumes: `TranslationRow` and `save(key, translatedText): Promise<void>`
- Produces: `translationDraftState(persisted, draft, pending)` and `TranslationEditorRow`

- [ ] **Step 1: Write the failing state test**

```ts
expect(translationDraftState("E-posta", "E-posta", false)).toEqual({
  isDirty: false, canSave: false,
});
expect(translationDraftState("Email", "E-posta", false)).toEqual({
  isDirty: true, canSave: true,
});
expect(translationDraftState("Email", "", false).canSave).toBe(false);
expect(translationDraftState("Email", "E-posta", true).canSave).toBe(false);
```

- [ ] **Step 2: Run the test and confirm failure**

Run: `yarn test src/components/localization/translationRowState.test.ts`

Expected: failure because `translationDraftState` does not exist.

- [ ] **Step 3: Implement the state helper**

```ts
export function translationDraftState(
  persisted: string,
  draft: string,
  pending: boolean,
) {
  const isDirty = draft !== persisted;
  return {
    isDirty,
    canSave: isDirty && draft.trim().length > 0 && !pending,
  };
}
```

- [ ] **Step 4: Implement `TranslationEditorRow`**

Use controlled state for `draft`, `persisted`, `origin`, `pending`, and `saved`. On row prop changes, reset these values from the new row. On Save:

```ts
const submitted = draft;
setPending(true);
setSaved(false);
try {
  await save(row.translationKey, submitted);
  setPersisted(submitted);
  setOrigin("manual");
  setSaved(true);
  window.setTimeout(() => setSaved(false), 2000);
} catch (error: any) {
  toast.error(error?.response?.data?.message || "Could not save translation");
} finally {
  setPending(false);
}
```

Render the source/status metadata, controlled input, origin badge, and button text selected from `pending ? "Saving…" : saved ? "Saved" : "Save"`. Do not attach `onBlur`.

- [ ] **Step 5: Replace inline rows**

In `ProjectLocalizationSection`, render:

```tsx
<TranslationEditorRow
  key={row.translationKey}
  row={row}
  save={(key, translatedText) =>
    editTranslation.mutateAsync({ key, translatedText }).then(() => undefined)
  }
/>
```

- [ ] **Step 6: Run focused tests**

Run: `yarn test src/components/localization/translationRowState.test.ts src/components/localization/localeSettings.test.ts`

Expected: PASS.

- [ ] **Step 7: Commit TenantPanel UI**

```bash
git add src/components/localization/translationRowState.ts src/components/localization/translationRowState.test.ts src/components/localization/TranslationEditorRow.tsx src/components/localization/ProjectLocalizationSection.tsx
git commit -m "fix: add explicit translation saving"
```

---

### Task 2: Encoded Translation-Key Persistence

**Files:**
- Modify: `controllers/localizationController.go`
- Modify: `controllers/runtime_localization_test.go`

**Interfaces:**
- Consumes: encoded Fiber route parameter `key`
- Produces: `decodeTranslationKeyParam(key string) (string, error)`

- [ ] **Step 1: Keep the failing regression test**

```go
func TestDecodeTranslationKeyParam(t *testing.T) {
	encoded := "container%3A69542a0d.field%3Aemail.displayName"
	want := "container:69542a0d.field:email.displayName"
	got, err := decodeTranslationKeyParam(encoded)
	if err != nil || got != want {
		t.Fatalf("got %q, %v; want %q", got, err, want)
	}
}
```

- [ ] **Step 2: Implement decoding before lookup**

```go
key, err := decodeTranslationKeyParam(c.Params("key"))
if err != nil {
	return c.Status(400).JSON(responses.GeneralResponse{
		Status: 400, Message: "Invalid translation key",
	})
}
```

Implement `decodeTranslationKeyParam` with `url.PathUnescape`.

- [ ] **Step 3: Run focused controller tests**

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./controllers -run TestDecodeTranslationKeyParam -count=1`

Expected: PASS.

- [ ] **Step 4: Commit backend persistence fix**

```bash
git add controllers/localizationController.go controllers/runtime_localization_test.go
git commit -m "fix: decode translation keys before saving"
```

---

### Task 3: Verification

**Files:**
- Verify only

**Interfaces:**
- Consumes: completed TenantPanel and backend changes
- Produces: verified explicit persistence workflow

- [ ] **Step 1: Run complete backend tests**

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./...`

Expected: all packages pass.

- [ ] **Step 2: Run TenantPanel tests and build**

```bash
yarn test src/components/localization/translationRowState.test.ts src/components/localization/localeSettings.test.ts src/utils/api/localization.test.ts
yarn build
```

Expected: tests, type checking, and production build pass.

- [ ] **Step 3: Run formatting checks**

Run `git diff --check` in autotable-Go and tenantPanel.

- [ ] **Step 4: Verify manually**

Edit `Email` to `E-posta`, confirm Save → Saving… → Saved, refresh TenantPanel, and confirm `E-posta` remains with a Manual badge.
