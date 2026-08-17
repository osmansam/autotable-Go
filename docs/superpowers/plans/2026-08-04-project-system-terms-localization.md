# Project System Terms Localization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a predefined, project-specific catalog of system UI terms that AI can translate and tenant users can manually override without AI overwriting their edits.

**Architecture:** The backend owns a stable predefined system-term catalog and adds it to localization discovery as `SourceString` records with `resourceType: "system"`. Existing generation, manual-update, persistence, and runtime APIs handle these records unchanged. TenantPanel adds a system-content filter to its existing editor, while react-template continues resolving the terms through existing `t(sourceText)` calls and the runtime i18next catalog.

**Tech Stack:** Go, Fiber, MongoDB, React, TypeScript, TanStack Query, i18next, Vitest.

## Global Constraints

- System keys are predefined in backend code; tenants cannot add, rename, or delete keys.
- System translations are stored in the existing `project_translations` collection.
- Stable translation keys use `system:<key>`.
- Current manual translations are never overwritten by AI generation.
- Missing runtime translations fall back to the source term.
- Runtime translations retain the array-based `sourceText`/`translatedText` response contract.

---

### Task 1: Backend Predefined System-Term Catalog

**Files:**
- Create: `services/localization_system_terms.go`
- Create: `services/localization_system_terms_test.go`

**Interfaces:**
- Consumes: `models.SourceString`, `LocalizationSourceHash(locale, text string) string`
- Produces: `DiscoverSystemStrings(sourceLocale string) []models.SourceString`

- [ ] **Step 1: Write the failing catalog test**

```go
func TestDiscoverSystemStringsUsesStableKeys(t *testing.T) {
	items := DiscoverSystemStrings("en")
	byKey := map[string]models.SourceString{}
	for _, item := range items {
		byKey[item.TranslationKey] = item
	}
	for key, source := range map[string]string{
		"system:actions": "Actions",
		"system:add": "Add",
		"system:edit": "Edit",
		"system:delete": "Delete",
		"system:save": "Save",
		"system:cancel": "Cancel",
		"system:search": "Search",
		"system:select_all": "Select All",
	} {
		item, ok := byKey[key]
		if !ok || item.SourceText != source || item.ResourceType != "system" {
			t.Fatalf("%s = %#v", key, item)
		}
	}
}
```

- [ ] **Step 2: Run the focused test and confirm it fails**

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./services -run TestDiscoverSystemStringsUsesStableKeys -count=1`

Expected: build failure because `DiscoverSystemStrings` does not exist.

- [ ] **Step 3: Implement the predefined catalog**

```go
type systemTerm struct {
	Key        string
	SourceText string
	Context    string
}

var predefinedSystemTerms = []systemTerm{
	{Key: "actions", SourceText: "Actions", Context: "Table actions column"},
	{Key: "add", SourceText: "Add", Context: "Add action"},
	{Key: "create", SourceText: "Create", Context: "Create action"},
	{Key: "edit", SourceText: "Edit", Context: "Edit action"},
	{Key: "delete", SourceText: "Delete", Context: "Delete action"},
	{Key: "save", SourceText: "Save", Context: "Save action"},
	{Key: "cancel", SourceText: "Cancel", Context: "Cancel action"},
	{Key: "close", SourceText: "Close", Context: "Close action"},
	{Key: "search", SourceText: "Search", Context: "Search input"},
	{Key: "filter", SourceText: "Filter", Context: "Filter action"},
	{Key: "select", SourceText: "Select", Context: "Selection prompt"},
	{Key: "select_all", SourceText: "Select All", Context: "Select all rows"},
	{Key: "clear", SourceText: "Clear", Context: "Clear action"},
	{Key: "reset", SourceText: "Reset", Context: "Reset action"},
	{Key: "yes", SourceText: "Yes", Context: "Affirmative choice"},
	{Key: "no", SourceText: "No", Context: "Negative choice"},
	{Key: "confirm", SourceText: "Confirm", Context: "Confirmation action"},
	{Key: "continue", SourceText: "Continue", Context: "Continue action"},
	{Key: "back", SourceText: "Back", Context: "Back navigation"},
	{Key: "next", SourceText: "Next", Context: "Next navigation"},
	{Key: "loading", SourceText: "Loading", Context: "Loading state"},
	{Key: "no_data", SourceText: "No data", Context: "Empty data state"},
	{Key: "success", SourceText: "Success", Context: "Success status"},
	{Key: "error", SourceText: "Error", Context: "Error status"},
}

func DiscoverSystemStrings(sourceLocale string) []models.SourceString {
	items := make([]models.SourceString, 0, len(predefinedSystemTerms))
	for _, term := range predefinedSystemTerms {
		items = append(items, models.SourceString{
			TranslationKey: "system:" + term.Key,
			ResourceType: "system",
			ResourceID: term.Key,
			PropertyPath: "system." + term.Key,
			SourceText: term.SourceText,
			SourceHash: LocalizationSourceHash(sourceLocale, term.SourceText),
			Context: term.Context,
		})
	}
	return items
}
```

- [ ] **Step 4: Run the service test**

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./services -run TestDiscoverSystemStringsUsesStableKeys -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the catalog**

```bash
git add services/localization_system_terms.go services/localization_system_terms_test.go
git commit -m "feat: define localizable system terms"
```

---

### Task 2: Include System Terms in AI Discovery and Runtime

**Files:**
- Modify: `services/localization_job.go`
- Modify: `services/localization_discovery_test.go`
- Modify: `controllers/runtime_localization_test.go`

**Interfaces:**
- Consumes: `DiscoverSystemStrings(sourceLocale string) []models.SourceString`
- Produces: system records in existing translation jobs and existing runtime resources

- [ ] **Step 1: Write a failing merged-discovery test**

Extract the existing map merge into:

```go
func mergeLocalizationSources(groups ...[]models.SourceString) []models.SourceString
```

Test it with a page string, container string, and `DiscoverSystemStrings("en")`; assert that `system:actions` is present once and has source text `Actions`.

- [ ] **Step 2: Run the test and confirm it fails**

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./services -run TestMergeLocalizationSourcesIncludesSystemTerms -count=1`

Expected: build failure because `mergeLocalizationSources` does not exist.

- [ ] **Step 3: Implement merged discovery**

Update `discoverAllProjectStrings` to collect page and container strings, then append:

```go
groups = append(groups, DiscoverSystemStrings(sourceLocale))
return mergeLocalizationSources(groups...), nil
```

`mergeLocalizationSources` must deduplicate by `TranslationKey` and return the merged slice.

- [ ] **Step 4: Verify runtime filtering keeps system records**

Add a `models.TranslationEntry` with `ResourceType: "system"`, `SourceText: "Actions"`, `TranslatedText: "İşlemler"`, current status, and active state to `TestRuntimeTranslationCatalogUsesCurrentActiveTranslations`. Assert the returned runtime resource includes the exact pair.

- [ ] **Step 5: Run service and controller tests**

Run: `GOCACHE=/private/tmp/autotable-go-build-cache go test ./services ./controllers -run 'TestMergeLocalizationSourcesIncludesSystemTerms|TestRuntimeTranslationCatalogUsesCurrentActiveTranslations' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit discovery integration**

```bash
git add services/localization_job.go services/localization_discovery_test.go controllers/runtime_localization_test.go
git commit -m "feat: generate and publish system term translations"
```

---

### Task 3: TenantPanel System-Term Editor Filter

**Files:**
- Create: `src/components/localization/translationFilters.ts`
- Create: `src/components/localization/translationFilters.test.ts`
- Modify: `src/components/localization/ProjectLocalizationSection.tsx`

**Interfaces:**
- Consumes: existing `TranslationRow.resourceType`
- Produces: `filterTranslations(rows, filter)` where `filter` is `"all" | "content" | "system"`

- [ ] **Step 1: Write the failing filter test**

```ts
expect(filterTranslations(rows, "system").map((row) => row.translationKey))
  .toEqual(["system:actions"]);
expect(filterTranslations(rows, "content").map((row) => row.translationKey))
  .toEqual(["container:abc.field:email.displayName"]);
```

- [ ] **Step 2: Run the test and confirm it fails**

Run from `/Users/osmansamilerdogan/Desktop/tenantPanel`:

`yarn test src/components/localization/translationFilters.test.ts`

Expected: failure because `filterTranslations` does not exist.

- [ ] **Step 3: Implement the pure filter**

```ts
export type TranslationFilter = "all" | "content" | "system";

export function filterTranslations(rows: TranslationRow[], filter: TranslationFilter) {
  if (filter === "all") return rows;
  return rows.filter((row) =>
    filter === "system" ? row.resourceType === "system" : row.resourceType !== "system",
  );
}
```

- [ ] **Step 4: Add the filter control to the editor**

Add local state initialized to `"all"`, render an accessible select with options `All terms`, `Tenant content`, and `System terms`, and render `filterTranslations(translations.data || [], translationFilter)`.

Keep the existing manual edit mutation. System rows use the same PATCH endpoint, so saved values become `origin: "manual"` and remain protected.

- [ ] **Step 5: Run TenantPanel tests and build**

Run:

```bash
yarn test src/components/localization/translationFilters.test.ts src/components/localization/localeSettings.test.ts
yarn build
```

Expected: tests and TypeScript production build pass.

- [ ] **Step 6: Commit TenantPanel filtering**

```bash
git add src/components/localization/translationFilters.ts src/components/localization/translationFilters.test.ts src/components/localization/ProjectLocalizationSection.tsx
git commit -m "feat: edit project system term translations"
```

---

### Task 4: React-Template System-Term Resolution

**Files:**
- Modify: `src/components/header/runtimeTranslations.test.ts`
- Test existing: `src/utils/tableColumns.test.ts`

**Interfaces:**
- Consumes: existing runtime `RuntimeTranslationPayload`
- Produces: i18next resolution of `t("Actions")` to the project value

- [ ] **Step 1: Add the system-term runtime test**

```ts
it("resolves a project-specific system term", async () => {
  const instance = i18next.createInstance();
  await instance.init({ lng: "tr", fallbackLng: "en", resources: {} });
  installRuntimeTranslations(instance, "tr", catalogFromRuntimeTranslations([
    { sourceText: "Actions", translatedText: "İşlemler" },
  ]));
  expect(instance.t("Actions")).toBe("İşlemler");
});
```

- [ ] **Step 2: Run the focused test**

Run from `/Users/osmansamilerdogan/Desktop/react-template`:

`yarn test src/components/header/runtimeTranslations.test.ts`

Expected: PASS because the existing generic runtime catalog supports system records. If it fails, fix only the generic catalog installation rather than adding a system-specific rendering branch.

- [ ] **Step 3: Run react-template tests and build**

```bash
yarn test src/components/header/runtimeTranslations.test.ts src/utils/tableColumns.test.ts
yarn build
```

Expected: tests, type checking, and production build pass.

- [ ] **Step 4: Commit runtime coverage**

```bash
git add src/components/header/runtimeTranslations.test.ts
git commit -m "test: cover project system term localization"
```

---

### Task 5: End-to-End Verification

**Files:**
- Verify only; no production files

**Interfaces:**
- Consumes: completed backend, TenantPanel, and react-template changes
- Produces: verified `Actions → İşlemler` project flow

- [ ] **Step 1: Run the complete backend suite**

`GOCACHE=/private/tmp/autotable-go-build-cache go test ./...`

Expected: all packages pass.

- [ ] **Step 2: Run TenantPanel localization tests and build**

```bash
yarn test src/components/localization/translationFilters.test.ts src/components/localization/localeSettings.test.ts src/utils/api/localization.test.ts
yarn build
```

Expected: all tests and build pass.

- [ ] **Step 3: Run react-template localization tests and build**

```bash
yarn test src/components/header/runtimeTranslations.test.ts src/components/header/localeResolution.test.ts src/utils/tableColumns.test.ts
yarn build
```

Expected: all tests and build pass.

- [ ] **Step 4: Verify formatting in all repositories**

Run `git diff --check` in autotable-Go, tenantPanel, and react-template.

- [ ] **Step 5: Verify the project flow**

For project `acme/deneme`, regenerate Turkish translations, edit `Actions` to `İşlemler` in TenantPanel’s System terms filter, refresh react-template, and confirm every `t("Actions")` consumer displays `İşlemler`.
