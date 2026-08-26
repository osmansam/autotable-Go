# Configurable Page Navigator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an enterprise-style, configurable page-header breadcrumb derived from project hierarchy with safe tenant-authored overrides and links.

**Architecture:** Extend the persisted page model with a validated `pageNavigator` configuration. Tenant Panel owns authoring and preview through a focused editor and pure resolver; `react-template` owns runtime resolution and accessible rendering through matching contracts. Internal destinations store page IDs and resolve canonical routes at runtime.

**Tech Stack:** Go, Fiber, MongoDB BSON, React 18, TypeScript, React Router, Tailwind CSS, Vitest.

**Spec:** `docs/superpowers/specs/2026-08-25-page-navigator-design.md`

## Global Constraints

- Render in a dedicated page-header region, never a grid cell.
- Existing pages without `pageNavigator` render no navigator.
- Current page is always last, visible, and non-clickable.
- Internal destinations store page IDs, not arbitrary paths.
- External destinations accept only absolute `http:` or `https:` URLs.
- Labels: maximum 100 characters. Overrides: maximum 20. Additional items: maximum 20.
- Existing tenant access is sufficient; do not introduce role checks.
- Do not accept arbitrary JavaScript, callbacks, HTML, or CSS.
- Follow red-green-refactor for every behavior change.
- Preserve the pre-existing unstaged `react-template/dist/index.html` change.

---

### Task 1: Persist and Validate Page Navigator Configuration

**Files:**
- Modify: `models/pageModel.go`
- Create: `models/page_navigator_validation.go`
- Create: `models/page_navigator_validation_test.go`
- Modify: `controllers/pageController.go`

**Interfaces:**
- Produces: `PageNavigatorConfig`, `PageNavigatorOverride`, `PageNavigatorAdditionalItem`, and `PageNavigatorDestination`.
- Produces: `func ValidatePageNavigatorConfig(page *PageModel) error`.
- Consumed by existing `CreatePage` and `UpdatePage`.

- [ ] **Step 1: Write failing validation tests**

Create table-driven tests for: legacy nil configuration; valid automatic/custom modes; unknown mode; labels over 100 characters; more than 20 overrides/additions; duplicate item IDs; empty manual labels; missing page ID; missing external URL; mixed destination fields; and unsafe protocols.

```go
func TestValidatePageNavigatorConfigRejectsUnsafeExternalURL(t *testing.T) {
	page := PageModel{PageNavigator: &PageNavigatorConfig{
		Enabled: true, Mode: PageNavigatorModeAutomatic,
		AdditionalItems: []PageNavigatorAdditionalItem{{
			ID: "docs", Label: "Docs",
			Destination: PageNavigatorDestination{
				Type: PageNavigatorDestinationExternal,
				URL: "javascript:alert(1)",
			},
		}},
	}}
	err := ValidatePageNavigatorConfig(&page)
	if err == nil || !strings.Contains(err.Error(), "absolute http(s)") {
		t.Fatalf("error = %v, want safe URL validation", err)
	}
}
```

- [ ] **Step 2: Verify RED**

```bash
GOCACHE=/private/tmp/autotable-page-navigator-red go test ./models -run PageNavigator -count=1
```

Expected: FAIL because the types and validator do not exist.

- [ ] **Step 3: Add typed BSON/JSON model fields**

Define exact `automatic|custom` and `page|external` constants. Add lower-camel BSON/JSON structs from the spec and `PageNavigator *PageNavigatorConfig \`bson:"pageNavigator,omitempty" json:"pageNavigator,omitempty"\`` to `PageModel`.

- [ ] **Step 4: Implement strict validation**

Return nil for nil config. Enforce bounds, trimmed unique IDs, exact destination union, and recursive subpage validation. Parse external URLs with `net/url`, require `IsAbs()`, and allow only `http` and `https`.

- [ ] **Step 5: Wire create/update validation**

Call `ValidatePageNavigatorConfig` in both page write paths before persistence and return the existing structured HTTP 400 validation response.

- [ ] **Step 6: Verify GREEN**

```bash
GOCACHE=/private/tmp/autotable-page-navigator-green go test ./models ./controllers -run 'PageNavigator|CreatePageReturnsRuntimeValidationDetail' -count=1
```

- [ ] **Step 7: Commit**

```bash
git add models/pageModel.go models/page_navigator_validation.go models/page_navigator_validation_test.go controllers/pageController.go
git commit -m "feat: validate page navigator configuration"
```

---

### Task 2: Add Tenant Panel Contracts and Pure Preview Resolver

**Files:**
- Modify: `../tenantPanel/src/types/page.ts`
- Create: `../tenantPanel/src/components/PageDesigner/pageNavigatorResolver.ts`
- Create: `../tenantPanel/src/components/PageDesigner/pageNavigatorResolver.test.ts`

**Interfaces:**
- Produces: `defaultPageNavigatorConfig(): PageNavigatorConfig`.
- Produces: `resolvePageNavigatorPreview(args): ResolvedPageNavigatorItem[]`.
- Items expose `id`, `label`, `pageId?`, `href?`, `current`, `external`, and `openInNewTab`.

- [ ] **Step 1: Write failing resolver tests**

Use a literal Home → Catalog → Products hierarchy. Assert automatic order, renamed/hidden ancestors, home de-duplication, custom mode, manual item order, deleted reference omission, unsafe external omission, cycle handling, and current-last enforcement.

```ts
expect(resolvePageNavigatorPreview(args)).toEqual([
  { id: "page:home", label: "Home", pageId: "home", current: false, external: false, openInNewTab: false },
  { id: "page:catalog", label: "Shop", pageId: "catalog", current: false, external: false, openInNewTab: false },
  { id: "page:products", label: "Products", pageId: "products", current: true, external: false, openInNewTab: false },
]);
```

- [ ] **Step 2: Verify RED**

```bash
cd ../tenantPanel
yarn test src/components/PageDesigner/pageNavigatorResolver.test.ts --run
```

- [ ] **Step 3: Add matching TypeScript contracts**

Add the spec's discriminated unions and `pageNavigator?: PageNavigatorConfig` to `PageModel`. Re-export through existing page-type exports.

- [ ] **Step 4: Implement the pure resolver**

Map pages by stable ID; walk parents cycle-safely; reverse ancestors; apply overrides; de-duplicate home/page references; append valid additions; force current last. Return stable references, not runtime paths.

- [ ] **Step 5: Verify GREEN and commit**

```bash
yarn test src/components/PageDesigner/pageNavigatorResolver.test.ts --run
git add src/types/page.ts src/components/PageDesigner/pageNavigatorResolver.ts src/components/PageDesigner/pageNavigatorResolver.test.ts
git commit -m "feat: resolve page navigator previews"
```

---

### Task 3: Build the Tenant Panel Editor and Preview

**Files:**
- Create: `../tenantPanel/src/components/PageDesigner/PageNavigatorEditor.tsx`
- Create: `../tenantPanel/src/components/PageDesigner/PageNavigatorPreview.tsx`
- Create: `../tenantPanel/src/components/PageDesigner/pageNavigatorEditorState.ts`
- Create: `../tenantPanel/src/components/PageDesigner/pageNavigatorEditorState.test.ts`
- Modify: `../tenantPanel/src/components/PageDesigner/PageDesigner.tsx`

**Interfaces:**
- Produces: `PageNavigatorEditor({ value, pages, currentPageId, onChange })`.
- Produces: immutable add/remove/move/change-destination helpers.
- Produces: `validatePageNavigatorDraft(config, pages): Record<string,string>`.

- [ ] **Step 1: Write failing state tests**

Assert stable unique item creation, reorder, removal, destination-type changes clearing stale fields, missing-page warnings, unsafe-URL errors, and disabled drafts retaining configuration.

- [ ] **Step 2: Verify RED**

```bash
yarn test src/components/PageDesigner/pageNavigatorEditorState.test.ts --run
```

- [ ] **Step 3: Implement state helpers and verify GREEN**

Use immutable operations and the repository's safe ID utility. Validate exact limits and page references against loaded project pages.

```bash
yarn test src/components/PageDesigner/pageNavigatorEditorState.test.ts --run
```

- [ ] **Step 4: Build the preview**

Render semantic desktop and narrow previews with Home, chevrons, current state, external indicator, and the compact `Home / … / Parent / Current` treatment.

- [ ] **Step 5: Build the editor**

Add enable, mode, home, generated overrides, page/external destination controls, new-tab control, remove, and reorder actions. Support pointer drag-and-drop with native drag events and provide accessible up/down buttons for keyboard users; do not add a drag-and-drop dependency.

- [ ] **Step 6: Integrate Page Designer**

Place under **Page Header → Breadcrumb Navigation**. Initialize from `page.pageNavigator`; leave undefined until enabled. Include configuration in the existing page save payload.

- [ ] **Step 7: Verify and commit**

```bash
yarn test src/components/PageDesigner/pageNavigatorResolver.test.ts src/components/PageDesigner/pageNavigatorEditorState.test.ts --run
yarn build
git add src/components/PageDesigner/PageNavigatorEditor.tsx src/components/PageDesigner/PageNavigatorPreview.tsx src/components/PageDesigner/pageNavigatorEditorState.ts src/components/PageDesigner/pageNavigatorEditorState.test.ts src/components/PageDesigner/PageDesigner.tsx
git commit -m "feat: configure page header navigation"
```

---

### Task 4: Add Runtime Types and Canonical Resolution

**Files:**
- Modify: `../react-template/src/types/page.ts`
- Create: `../react-template/src/navigation/pageNavigatorResolver.ts`
- Create: `../react-template/src/navigation/pageNavigatorResolver.test.ts`

**Interfaces:**
- Produces: `resolvePageNavigator(args: ResolveRuntimePageNavigatorArgs): ResolvedPageNavigatorItem[]`.
- Consumes current project page list, current page, tenant/project slugs, and route parameters.

- [ ] **Step 1: Write failing runtime resolver tests**

Mirror Task 2 contract fixtures. Add canonical URL tests: a target `/orders/:orderId` with params `{orderId:"42", tab:"audit"}` preserves `orderId` and omits unused `tab`; missing required params omit the target. Test safe external/new-tab behavior.

- [ ] **Step 2: Verify RED**

```bash
cd ../react-template
yarn test src/navigation/pageNavigatorResolver.test.ts --run
```

- [ ] **Step 3: Add matching page types**

Mirror the persisted discriminated unions exactly; do not use `Record<string, any>`.

- [ ] **Step 4: Implement resolution**

Perform cycle-safe hierarchy walking, overrides, home de-duplication, custom mode, additions, deleted-reference omission, and current-last enforcement. Use existing tenant/project route helpers and replace only declared `:param` tokens with encoded values.

- [ ] **Step 5: Verify and commit**

```bash
yarn test src/navigation/pageNavigatorResolver.test.ts --run
git add src/types/page.ts src/navigation/pageNavigatorResolver.ts src/navigation/pageNavigatorResolver.test.ts
git commit -m "feat: resolve runtime page navigation"
```

---

### Task 5: Render the Accessible Responsive Breadcrumb

**Files:**
- Create: `../react-template/src/navigation/PageNavigator.tsx`
- Create: `../react-template/src/navigation/pageNavigatorPresentation.ts`
- Create: `../react-template/src/navigation/pageNavigatorPresentation.test.ts`

**Interfaces:**
- Produces: `PageNavigator({ items })`.
- Produces: `collapsePageNavigatorItems(items, compact)`.

- [ ] **Step 1: Write failing presentation tests**

Assert five compact items become Home, Ellipsis, Parent, Current; three remain unchanged; current never enters the ellipsis group; and omitted items retain order.

- [ ] **Step 2: Verify RED**

```bash
yarn test src/navigation/pageNavigatorPresentation.test.ts --run
```

- [ ] **Step 3: Implement compact presentation and verify GREEN**

Create a pure discriminated union for link and ellipsis items.

- [ ] **Step 4: Build the component**

Render `<nav aria-label="Breadcrumb"><ol>`. Use React Router `Link` internally and anchors externally. Apply `aria-current="page"`, decorative hidden chevrons, safe new-tab attributes, full-label titles, visible focus rings, and `var(--brand-primary)`.

Implement an accessible ellipsis menu with `aria-expanded`, keyboard navigation, outside-click dismissal, Escape dismissal, and trigger focus restoration.

- [ ] **Step 5: Add DOM behavior tests only with existing infrastructure**

If Testing Library/jsdom already exists, test semantics and menu behavior. Otherwise keep logic in the tested pure module and do not add a new dependency solely for these tests.

- [ ] **Step 6: Verify and commit**

```bash
yarn test src/navigation/pageNavigatorPresentation.test.ts --run
git add src/navigation/PageNavigator.tsx src/navigation/pageNavigatorPresentation.ts src/navigation/pageNavigatorPresentation.test.ts
git commit -m "feat: render responsive page navigator"
```

---

### Task 6: Integrate the Dedicated Runtime Page Header

**Files:**
- Modify: `../react-template/src/components/DynamicPageRenderer.tsx`
- Create: `../react-template/src/navigation/pageNavigatorIntegration.ts`
- Create: `../react-template/src/navigation/pageNavigatorIntegration.test.ts`

**Interfaces:**
- Produces: `buildPageNavigatorViewModel(args)`.
- Renders navigator before filters and page sections only when present and enabled.

- [ ] **Step 1: Write failing integration adapter tests**

Assert absent and disabled config return empty items, enabled config resolves, and unresolved current page suppresses the navigator without affecting page content.

- [ ] **Step 2: Verify RED**

```bash
yarn test src/navigation/pageNavigatorIntegration.test.ts --run
```

- [ ] **Step 3: Integrate into `DynamicPageRenderer`**

Memoize resolution using current page, loaded project pages, slugs, and params. Render `PageNavigator` before page filters and `PageSectionView`. Do not add a new `ComponentType`.

- [ ] **Step 4: Verify and commit**

```bash
yarn test src/navigation/pageNavigatorResolver.test.ts src/navigation/pageNavigatorPresentation.test.ts src/navigation/pageNavigatorIntegration.test.ts --run
yarn build
git add src/components/DynamicPageRenderer.tsx src/navigation/pageNavigatorIntegration.ts src/navigation/pageNavigatorIntegration.test.ts
git commit -m "feat: add navigator to page headers"
```

---

### Task 7: Cross-Project Verification

**Files:**
- Modify spec only if implemented behavior intentionally differs.
- Preserve: `../react-template/dist/index.html`.

- [ ] **Step 1: Verify backend**

```bash
cd /Users/osmansamilerdogan/Desktop/autotable-Go
GOCACHE=/private/tmp/autotable-page-navigator-final go test ./...
```

Expected: all packages PASS. Rerun with approved local-network permissions if sandbox-only bind errors occur.

- [ ] **Step 2: Verify Tenant Panel**

```bash
cd /Users/osmansamilerdogan/Desktop/tenantPanel
yarn test && yarn lint && yarn build
```

Expected: tests/build PASS and lint has zero errors; report pre-existing warnings.

- [ ] **Step 3: Verify runtime frontend**

```bash
cd /Users/osmansamilerdogan/Desktop/react-template
yarn test && yarn build
yarn eslint src/navigation src/components/DynamicPageRenderer.tsx src/types/page.ts
```

Expected: tests/build/changed-file lint PASS. Keep unrelated repo-wide lint baseline separate.

- [ ] **Step 4: Browser smoke test**

Verify automatic Home → Parent → Current, rename/hide, custom trail, shared route parameters, external new tab, keyboard focus, Escape dismissal, compact viewport, branding color, deleted references, and a legacy page without config.

- [ ] **Step 5: Audit status**

```bash
git -C /Users/osmansamilerdogan/Desktop/autotable-Go status --short
git -C /Users/osmansamilerdogan/Desktop/tenantPanel status --short
git -C /Users/osmansamilerdogan/Desktop/react-template status --short
```

Expected: feature files committed; `react-template/dist/index.html` remains unstaged.
