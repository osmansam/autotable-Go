# Tenant and Project Branding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add inherited tenant/project branding that users manage in tenantPanel and react-template renders consistently across project identity surfaces.

**Architecture:** Embed optional typed branding overrides in tenant and project records, resolve effective values server-side with platform defaults, and expose separate authenticated management and public runtime contracts. Store validated raster assets in Cloudinary, then consume the effective runtime contract through a shared React provider so components never reproduce inheritance logic.

**Tech Stack:** Go 1.x, Fiber, MongoDB Go driver, Cloudinary Go SDK, React 18, TypeScript, TanStack Query 5, Axios, Tailwind CSS, Vitest.

**Spec:** `docs/superpowers/specs/2026-08-25-project-branding-design.md`

## Global Constraints

- Precedence is field-by-field: project override, then tenant default, then AutoTable platform default.
- Any authenticated user with tenant access may manage tenant branding; any authenticated user with project access may manage project branding. Do not add role checks in this release.
- The first release accepts PNG, JPEG, and WebP only; SVG, external URLs, arbitrary CSS/HTML, custom domains, and email branding are excluded.
- The public response contains renderable effective fields and version only; it never exposes Cloudinary provider metadata, asset IDs, sizes, internal IDs, or raw overrides.
- Missing branding on existing records must require no migration and must resolve to safe AutoTable defaults.
- Upload replacement order is upload, persist, then best-effort deletion of the superseded asset.
- Run each repository's complete test and build commands before final completion.

## File Structure

### `autotable-Go`

- Create `models/branding.go`: storage types, public runtime type, defaults, validation helpers, and pure effective resolver.
- Modify `models/tenantModel.go`: embed optional branding in `Tenant` and `Project`.
- Create `models/branding_test.go`: resolver and validation unit tests.
- Replace/extend `utils/imageUpload.go`: provider interface plus Cloudinary upload/delete implementation returning asset metadata.
- Create `controllers/brandingController.go`: management/runtime handlers and injectable storage seams.
- Create `controllers/branding_helpers_test.go`: update document, response filtering, validation, and lifecycle tests.
- Create `routes/brandingRoutes.go`: authenticated management routes and public runtime route.
- Modify `main.go`: register branding routes.
- Modify `routes/routes_test.go`: middleware and route exposure checks.
- Modify `controllers/projectController.go` and its tests: preserve branding during project/template cloning.

### `tenantPanel`

- Create `src/types/branding.ts`: shared management and effective branding types.
- Modify `src/types/index.ts` and `src/utils/api/project.ts`: add branding to tenant/project types.
- Create `src/utils/api/branding.ts`: tenant/project management query and mutation hooks.
- Create `src/components/branding/BrandingEditor.tsx`: reusable inherited/override editor.
- Create `src/components/branding/BrandingAssetField.tsx`: upload/preview/reset behavior for one asset slot.
- Create `src/components/branding/BrandingPreview.tsx`: header/sidebar/login/favicon previews.
- Create corresponding Vitest files beside the new utilities/components.
- Modify `src/pages/ProjectManagementPage.tsx`: mount project branding editor.
- Create `src/pages/TenantBrandingPage.tsx` and register the existing `/settings` route as the tenant-branding settings screen.
- Modify `src/pages/index.ts`, `src/navigation/routes.tsx`, and `src/navigation/constants.tsx`: export the page, activate the existing Settings route, and keep it visible to every tenant member.

### `react-template`

- Create `src/types/branding.ts`: public runtime branding contract.
- Create `src/utils/api/branding.ts`: public runtime fetcher.
- Create `src/context/Branding.context.tsx`: TanStack query, document effects, fallbacks, and hook.
- Create `src/components/BrandedImage.tsx`: consistent broken-image fallback.
- Modify `src/App.tsx`: mount provider inside QueryClient and router context.
- Modify `src/components/header/Header.tsx`, `src/common/Sidebar.tsx`, and `src/pages/Login.tsx`: consume effective branding.
- Modify `src/navigation/RouteLoadingFallback.tsx` and runtime loading/error surfaces: show project identity where already appropriate.
- Add focused Vitest files for resolution consumption, document effects, and component fallbacks.

---

### Task 1: Backend Branding Types and Pure Resolution

**Files:**
- Create: `models/branding.go`
- Modify: `models/tenantModel.go`
- Test: `models/branding_test.go`

**Interfaces:**
- Produces: `Branding`, `BrandingAsset`, `RuntimeBranding`, `DefaultRuntimeBranding(projectName string) RuntimeBranding`, `ResolveBranding(projectName string, tenant, project *Branding) RuntimeBranding`, and `ValidateBrandingPatch(BrandingPatch) error`.
- Consumes: only standard-library strings/regexp and existing model conventions.

- [ ] **Step 1: Write failing resolver tests**

```go
func TestResolveBrandingUsesFieldLevelPrecedence(t *testing.T) {
	tenant := &Branding{DisplayName: ptr("Tenant"), PrimaryColor: ptr("#112233"), Logo: &BrandingAsset{URL: "tenant.png"}}
	project := &Branding{DisplayName: ptr("Project"), CompactLogo: &BrandingAsset{URL: "compact.png"}}
	got := ResolveBranding("Stored Project", tenant, project)
	if got.DisplayName != "Project" || got.PrimaryColor != "#112233" || got.LogoURL != "tenant.png" || got.CompactLogoURL != "compact.png" {
		t.Fatalf("unexpected effective branding: %#v", got)
	}
}

func TestResolveBrandingFallsBackForLegacyProject(t *testing.T) {
	got := ResolveBranding("Inventory", nil, nil)
	if got.DisplayName != "Inventory" || got.LogoURL == "" || got.FaviconURL == "" || got.PrimaryColor == "" {
		t.Fatalf("missing platform fallback: %#v", got)
	}
}
```

- [ ] **Step 2: Run the tests and confirm the missing types fail**

Run: `go test ./models -run 'TestResolveBranding'`

Expected: FAIL because `Branding` and `ResolveBranding` do not exist.

- [ ] **Step 3: Implement storage/runtime types and resolver**

```go
type BrandingAsset struct {
	URL string `bson:"url" json:"url"`
	Provider string `bson:"provider" json:"provider"`
	AssetID string `bson:"assetId" json:"assetId"`
	Width int `bson:"width" json:"width"`
	Height int `bson:"height" json:"height"`
	Format string `bson:"format" json:"format"`
	Bytes int64 `bson:"bytes" json:"bytes"`
}

type Branding struct {
	DisplayName *string `bson:"displayName,omitempty" json:"displayName,omitempty"`
	Logo *BrandingAsset `bson:"logo,omitempty" json:"logo,omitempty"`
	CompactLogo *BrandingAsset `bson:"compactLogo,omitempty" json:"compactLogo,omitempty"`
	Favicon *BrandingAsset `bson:"favicon,omitempty" json:"favicon,omitempty"`
	LogoAlt *string `bson:"logoAlt,omitempty" json:"logoAlt,omitempty"`
	PrimaryColor *string `bson:"primaryColor,omitempty" json:"primaryColor,omitempty"`
	LoginBrandingEnabled *bool `bson:"loginBrandingEnabled,omitempty" json:"loginBrandingEnabled,omitempty"`
	Version int64 `bson:"version,omitempty" json:"version,omitempty"`
}

type RuntimeBranding struct {
	DisplayName string `json:"displayName"`
	LogoURL string `json:"logoUrl"`
	CompactLogoURL string `json:"compactLogoUrl"`
	FaviconURL string `json:"faviconUrl"`
	LogoAlt string `json:"logoAlt"`
	PrimaryColor string `json:"primaryColor"`
	LoginBrandingEnabled bool `json:"loginBrandingEnabled"`
	Version int64 `json:"version"`
}
```

Implement small overlay helpers so every pointer/asset field is considered independently. Derive compact logo from effective logo, favicon from compatible compact logo, and alt text from display name.

- [ ] **Step 4: Add failing validation tests and implement canonical validation**

Test whitespace trimming, 100-character display name, 160-character alt text, `^#[0-9A-Fa-f]{6}$` colors, and rejection of empty-string overrides. Implement `BrandingPatch` with pointer-to-pointer or explicit reset fields so omitted and unset are distinguishable.

Run: `go test ./models -run 'Test(Resolve|Validate)Branding'`

Expected: PASS.

- [ ] **Step 5: Embed branding in tenant and project models and run model tests**

```go
Branding *Branding `bson:"branding,omitempty" json:"branding,omitempty"`
```

Run: `go test ./models`

Expected: PASS.

- [ ] **Step 6: Commit backend model slice**

```bash
git add models/branding.go models/branding_test.go models/tenantModel.go
git commit -m "feat: add inherited branding model"
```

### Task 2: Cloudinary Asset Service With Safe Replacement Metadata

**Files:**
- Modify: `utils/imageUpload.go`
- Modify: `files/upload_service.go`
- Test: `files/upload_service_test.go`
- Create: `utils/imageUpload_test.go`

**Interfaces:**
- Consumes: `models.BrandingAsset` from Task 1.
- Produces: `type BrandingAssetStore interface { Upload(ctx context.Context, file io.Reader, options BrandingUploadOptions) (models.BrandingAsset, error); Delete(ctx context.Context, assetID string) error }` and `ValidateBrandingImage(io.Reader, maxBytes int64) (ValidatedImage, error)`.

- [ ] **Step 1: Write failing signature/format tests**

Create table tests using in-memory 1x1 PNG/JPEG/WebP fixtures. Assert a renamed text file is rejected, allowed images report decoded width/height/format, files over 2 MiB are rejected, and decoded dimensions over the configured limit are rejected.

Run: `go test ./utils -run TestValidateBrandingImage`

Expected: FAIL because the validator is missing.

- [ ] **Step 2: Implement bounded decode validation**

Use `io.LimitReader(maxBytes+1)`, `http.DetectContentType`, `image.DecodeConfig`, and explicit MIME allowlisting. Return a replayable buffer plus normalized `png`, `jpeg`, or `webp` metadata. Import the WebP decoder already available transitively only if `go list -m` confirms it; otherwise add `golang.org/x/image/webp` with `go get` and commit `go.mod/go.sum` in this task.

- [ ] **Step 3: Write failing fake-store lifecycle tests**

Test that upload returns URL, asset ID, dimensions, format, and byte count; delete receives exactly the stored asset ID; and provider errors propagate without a partial asset.

Run: `go test ./utils ./files -run 'Test(Branding|SaveAndUpload)'`

Expected: FAIL against the URL-only Cloudinary helper.

- [ ] **Step 4: Implement provider interface and Cloudinary adapter**

Pass server-generated `PublicID`, `Folder`, and `ResourceType: "image"` upload parameters. Map `SecureURL`, `PublicID`, dimensions, format, and bytes into `BrandingAsset`. Delete by `PublicID` through Cloudinary's destroy API. Keep legacy `UploadToCloudinary` behavior working for existing callers by adapting it to the new service rather than removing it.

- [ ] **Step 5: Run focused and full utility/file tests**

Run: `go test ./utils ./files`

Expected: PASS.

- [ ] **Step 6: Commit asset service**

```bash
git add utils/imageUpload.go utils/imageUpload_test.go files/upload_service.go files/upload_service_test.go go.mod go.sum
git commit -m "feat: validate and manage branding assets"
```

### Task 3: Backend Management Persistence and Upload Lifecycle

**Files:**
- Create: `controllers/brandingController.go`
- Create: `controllers/branding_helpers_test.go`

**Interfaces:**
- Consumes: Task 1 resolver/types and Task 2 `BrandingAssetStore`.
- Produces: `GetTenantBranding`, `PatchTenantBranding`, `UploadTenantBrandingAsset`, `DeleteTenantBrandingAsset`, `GetProjectBranding`, `PatchProjectBranding`, `UploadProjectBrandingAsset`, `DeleteProjectBrandingAsset` Fiber handlers.

- [ ] **Step 1: Write failing update-document tests**

```go
func TestBrandingPatchUpdateUsesSetAndUnset(t *testing.T) {
	patch := models.BrandingPatch{DisplayName: ptr("Acme"), Reset: []string{"logoAlt"}}
	set, unset, err := brandingUpdateDocuments(patch, 7, time.Unix(1, 0))
	if err != nil { t.Fatal(err) }
	if set["branding.displayName"] != "Acme" || set["branding.version"] != int64(8) { t.Fatalf("bad set: %#v", set) }
	if unset["branding.logoAlt"] != "" { t.Fatalf("bad unset: %#v", unset) }
}
```

Also test that asset slot names outside `logo`, `compactLogo`, and `favicon` are rejected and reset cannot target `version` or asset metadata subfields.

- [ ] **Step 2: Run controller helper tests to confirm failure**

Run: `go test ./controllers -run TestBranding`

Expected: FAIL because the controller helpers do not exist.

- [ ] **Step 3: Implement scoped lookup and PATCH handlers**

Read `tenantID`/`projectID` from existing authenticated locals, always filter projects by both `_id` and `tenantId`, validate the patch, issue one `$set`/`$unset` update with version increment, and return both stored overrides and `ResolveBranding(...)`. Do not apply `TenantAuthorize` role middleware.

After a successful scalar, upload, or reset mutation, emit a structured branding-change audit event through the existing audit helper when that helper accepts the tenant/project context. Audit-write failure must be logged but must not turn a successfully persisted branding change into an API failure.

- [ ] **Step 4: Write failing upload-order tests with injected fakes**

Assert these sequences:

```text
success: upload(new) -> mongo(update) -> delete(old)
upload failure: upload(new), no mongo, no delete(old)
mongo failure: upload(new) -> mongo(fail) -> delete(new), retain old
old delete failure: upload(new) -> mongo(update) -> delete(old fail), return success
```

Expose small function variables or a `brandingDependencies` struct for collection and asset-store injection; do not require a live MongoDB or Cloudinary service in unit tests.

- [ ] **Step 5: Implement upload/delete handlers and lifecycle**

Parse a single multipart `file`, validate before Cloudinary upload, generate folder `branding/tenants/<tenantID>` or `branding/tenants/<tenantID>/projects/<projectID>`, persist the returned asset under the selected slot, increment version, then best-effort delete the old asset. A delete/reset operation unsets only the requested slot and deletes its own asset after persistence.

- [ ] **Step 6: Run controller tests**

Run: `go test ./controllers -run TestBranding`

Expected: PASS.

- [ ] **Step 7: Commit management handlers**

```bash
git add controllers/brandingController.go controllers/branding_helpers_test.go
git commit -m "feat: manage tenant and project branding"
```

### Task 4: Public Runtime Contract, Routes, and Clone Semantics

**Files:**
- Modify: `controllers/brandingController.go`
- Create: `controllers/branding_runtime_test.go`
- Create: `routes/brandingRoutes.go`
- Modify: `routes/routes_test.go`
- Modify: `main.go`
- Modify: `controllers/projectController.go`
- Modify: `controllers/project_templates_test.go`

**Interfaces:**
- Consumes: management handlers and resolver from Tasks 1–3.
- Produces: `GetRuntimeBranding` and registered `/api/v1/:tenantSlug/:projectSlug/branding` runtime endpoint plus authenticated `/api/v1/tenant/branding` and `/api/v1/tenant/projects/:id/branding` groups.

- [ ] **Step 1: Write failing public response filtering tests**

Build a tenant/project pair containing `Provider: "cloudinary"`, `AssetID: "secret-id"`, and byte metadata. Marshal the runtime response and assert it contains `logoUrl` and `version` but does not contain `assetId`, `provider`, `bytes`, `tenantId`, or `projectId`.

Run: `go test ./controllers -run TestRuntimeBranding`

Expected: FAIL before the handler/view builder exists.

- [ ] **Step 2: Implement public lookup and conditional caching**

Resolve active tenant by slug and active project by slug plus tenant ID. Return a generic 404 for either miss. Set `ETag` to a stable value derived from tenant/project branding versions and return 304 when `If-None-Match` matches. Use `SearchRateLimit` and do not require authentication.

- [ ] **Step 3: Write route middleware tests**

Register stub handlers through a `registerBrandingRoutes` helper. Assert runtime GET reaches its handler without auth; management GET/PATCH/upload/delete return unauthorized without cookies; and project management paths use both tenant auth and project scope checks.

- [ ] **Step 4: Register explicit routes**

```text
GET    /api/v1/:tenantSlug/:projectSlug/branding
GET    /api/v1/tenant/branding
PATCH  /api/v1/tenant/branding
POST   /api/v1/tenant/branding/assets/:slot
DELETE /api/v1/tenant/branding/assets/:slot
GET    /api/v1/tenant/projects/:id/branding
PATCH  /api/v1/tenant/projects/:id/branding
POST   /api/v1/tenant/projects/:id/branding/assets/:slot
DELETE /api/v1/tenant/projects/:id/branding/assets/:slot
```

Apply existing body-size/write limits to scalar mutations and a branding-specific 2 MiB multipart limit to upload routes. Register `BrandingRoutes(app)` from `main.go`.

- [ ] **Step 5: Add failing clone test and preserve project branding**

Extend the template clone test fixture with branding and assert the created project contains equal branding metadata while retaining its own new project ID/name/slug. Update the clone document/model mapping minimally to copy branding.

- [ ] **Step 6: Run backend package and full tests**

Run: `go test ./controllers ./routes ./models ./utils ./files`

Expected: PASS.

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 7: Commit runtime/routes slice**

```bash
git add controllers/brandingController.go controllers/branding_runtime_test.go routes/brandingRoutes.go routes/routes_test.go main.go controllers/projectController.go controllers/project_templates_test.go
git commit -m "feat: expose effective project branding"
```

### Task 5: tenantPanel Branding API and State Contracts

**Files:**
- Create: `../tenantPanel/src/types/branding.ts`
- Modify: `../tenantPanel/src/types/index.ts`
- Modify: `../tenantPanel/src/utils/api/project.ts`
- Create: `../tenantPanel/src/utils/api/branding.ts`
- Create: `../tenantPanel/src/utils/api/branding.test.ts`

**Interfaces:**
- Consumes: backend management routes from Task 4.
- Produces: `BrandingOverrides`, `EffectiveBranding`, `BrandingManagementResponse`, `useTenantBranding`, `useProjectBranding`, scalar patch mutations, asset upload mutations, and asset reset mutations.

- [ ] **Step 1: Define types and write failing response-normalization tests**

```ts
export type BrandingAssetSlot = "logo" | "compactLogo" | "favicon";
export interface EffectiveBranding {
  displayName: string;
  logoUrl: string;
  compactLogoUrl: string;
  faviconUrl: string;
  logoAlt: string;
  primaryColor: string;
  loginBrandingEnabled: boolean;
  version: number;
}
```

Test both `{ data: { overrides, effective } }` and Axios-unwrapped variants, plus query keys that include scope and project ID.

- [ ] **Step 2: Run test and verify missing module failure**

Run from `../tenantPanel`: `yarn test src/utils/api/branding.test.ts`

Expected: FAIL because `branding.ts` is missing.

- [ ] **Step 3: Implement API hooks with precise invalidation**

Use query keys `['branding','tenant',tenantId]` and `['branding','project',projectId]`. Use `FormData` with field name `file`. After mutations, invalidate the changed scope and all effective project branding queries affected by a tenant update. Return server error messages for the UI rather than showing toasts inside pure fetch functions.

- [ ] **Step 4: Add branding fields to tenant/project types and run tests/build**

Run from `../tenantPanel`: `yarn test src/utils/api/branding.test.ts && yarn build`

Expected: PASS.

- [ ] **Step 5: Commit tenantPanel API slice**

```bash
cd ../tenantPanel
git add src/types/branding.ts src/types/index.ts src/utils/api/project.ts src/utils/api/branding.ts src/utils/api/branding.test.ts
git commit -m "feat: add branding management API"
```

### Task 6: tenantPanel Reusable Branding Editor

**Files:**
- Create: `../tenantPanel/src/components/branding/BrandingAssetField.tsx`
- Create: `../tenantPanel/src/components/branding/BrandingPreview.tsx`
- Create: `../tenantPanel/src/components/branding/BrandingEditor.tsx`
- Create: `../tenantPanel/src/components/branding/brandingState.ts`
- Create: `../tenantPanel/src/components/branding/brandingState.test.ts`

**Interfaces:**
- Consumes: API/types from Task 5.
- Produces: `<BrandingEditor scope="tenant" tenantId={...} />` and `<BrandingEditor scope="project" tenantId={...} projectId={...} />`.

- [ ] **Step 1: Write failing inherited-state unit tests**

```ts
it("resets one project field without copying the tenant value", () => {
  const result = buildBrandingPatch(
    { displayName: "Tenant" },
    { displayName: "Project" },
    { displayName: { mode: "inherit", value: "Tenant" } },
  );
  expect(result).toEqual({ reset: ["displayName"] });
});
```

Also cover changed override, unchanged override, normalized uppercase hex color, invalid hex, and dirty-state detection.

- [ ] **Step 2: Run unit test and verify failure**

Run from `../tenantPanel`: `yarn test src/components/branding/brandingState.test.ts`

Expected: FAIL because state helpers do not exist.

- [ ] **Step 3: Implement pure editor state helpers**

Keep inheritance/patch construction outside React. Return `{ set, reset, errors, dirty }` with no API calls. Never represent inheritance by copying effective values into overrides.

- [ ] **Step 4: Implement asset field and preview components**

`BrandingAssetField` must accept `slot`, stored asset, effective URL, inherited flag, upload callback, and reset callback. Validate browser-side extension/type/2 MiB size for fast feedback while treating backend validation as authoritative. Preserve the old preview during upload and on error.

`BrandingPreview` renders four labeled, non-interactive previews: desktop header, expanded/collapsed sidebar, login card, and favicon tile. Use `object-contain`, never crop logos.

- [ ] **Step 5: Implement the editor shell**

Render display name, alt text, uppercase six-digit color, login-branding checkbox, three asset fields, inheritance badges, `Use tenant default` actions for project scope, Save, and Reset unsaved changes. Add `beforeunload` protection while scalar changes are dirty. Completed asset changes persist immediately and refresh effective data.

- [ ] **Step 6: Add component behavior tests**

Test accessible labels and buttons, project inheritance labels, upload failure preserving the current image, successful reset exposing effective tenant URL, invalid color blocking save, and successful save clearing dirty state. Use the repository's existing Vitest environment; if DOM rendering utilities are unavailable, test extracted view-model/event functions without adding a new framework.

- [ ] **Step 7: Run focused tests and build**

Run from `../tenantPanel`: `yarn test src/components/branding && yarn build`

Expected: PASS.

- [ ] **Step 8: Commit editor**

```bash
cd ../tenantPanel
git add src/components/branding
git commit -m "feat: add inherited branding editor"
```

### Task 7: Mount tenantPanel Project and Tenant Branding Screens

**Files:**
- Modify: `../tenantPanel/src/pages/ProjectManagementPage.tsx`
- Create: `../tenantPanel/src/pages/TenantBrandingPage.tsx`
- Modify: `../tenantPanel/src/pages/index.ts`
- Modify: `../tenantPanel/src/navigation/routes.tsx`
- Modify: `../tenantPanel/src/navigation/constants.tsx`
- Test: `../tenantPanel/src/pages/brandingMounts.test.ts`

**Interfaces:**
- Consumes: `BrandingEditor` from Task 6 and existing current tenant/project hooks.
- Produces: discoverable tenant defaults and project override UI.

- [ ] **Step 1: Write failing mount tests**

Assert `ProjectManagementPage` includes one project-scoped editor with the current project ID. Assert `/settings` renders `TenantBrandingPage`, and that page includes one tenant-scoped editor with the current tenant ID.

- [ ] **Step 2: Mount the project editor**

Add a `Project Branding` card after the project identity header and before containers/pages. Pass the current tenant/project IDs and do not use the user's roles to hide it.

- [ ] **Step 3: Create and mount the tenant settings page**

Create a small authenticated `TenantBrandingPage`, export it from `pages/index.ts`, render it at the existing `Routes.Settings` (`/settings`) path, and read `currentTenant` from the existing `useTenant` hook. Keep the existing Settings sidebar entry and do not attach `requiredRoles`.

- [ ] **Step 4: Run tenantPanel tests, lint, and build**

Run from `../tenantPanel`: `yarn test && yarn lint && yarn build`

Expected: PASS.

- [ ] **Step 5: Commit screens**

```bash
cd ../tenantPanel
git add src/pages src/navigation/routes.tsx src/navigation/constants.tsx
git commit -m "feat: expose tenant and project branding settings"
```

### Task 8: react-template Runtime Branding Provider and Document State

**Files:**
- Create: `../react-template/src/types/branding.ts`
- Create: `../react-template/src/utils/api/branding.ts`
- Create: `../react-template/src/context/Branding.context.tsx`
- Create: `../react-template/src/context/brandingDocument.ts`
- Create: `../react-template/src/context/brandingDocument.test.ts`
- Modify: `../react-template/src/App.tsx`

**Interfaces:**
- Consumes: public backend endpoint from Task 4 and `useTenantProject`.
- Produces: `useBranding(): EffectiveBranding`, `BrandingProvider`, and `applyBrandingToDocument(branding, document)` cleanup function.

- [ ] **Step 1: Write failing document-effect tests**

```ts
it("updates and restores title, favicon, and primary color", () => {
  const cleanup = applyBrandingToDocument(branding, document);
  expect(document.title).toBe("Acme Inventory");
  expect(document.documentElement.style.getPropertyValue("--brand-primary")).toBe("#2563EB");
  expect(document.querySelector('link[data-project-favicon]')?.getAttribute("href")).toBe("favicon.png");
  cleanup();
  expect(document.title).toBe(originalTitle);
});
```

Also test switching from project A to B replaces, rather than accumulates, favicon links.

- [ ] **Step 2: Run the document tests and confirm failure**

Run from `../react-template`: `yarn test src/context/brandingDocument.test.ts`

Expected: FAIL because the module is missing.

- [ ] **Step 3: Implement public fetcher and provider**

Fetch `/branding` through the existing tenant/project-prefixed Axios client. Query with `['runtime-branding', tenant, project]`, enabled only when both slugs exist. Return bundled defaults while loading or on error. Do not retain A's effective data when the URL changes to B.

- [ ] **Step 4: Implement reversible document effects**

Set `document.title`, one `<link rel="icon" data-project-favicon>`, and `--brand-primary`. Capture prior values and restore them during cleanup. Treat empty/broken favicon URLs as platform default.

- [ ] **Step 5: Mount provider**

Mount `BrandingProvider` inside `QueryClientProvider` and router context so it can read URL params, and outside identity consumers so header/sidebar/login all share one query.

- [ ] **Step 6: Run focused tests and build**

Run from `../react-template`: `yarn test src/context/brandingDocument.test.ts && yarn build`

Expected: PASS.

- [ ] **Step 7: Commit provider slice**

```bash
cd ../react-template
git add src/types/branding.ts src/utils/api/branding.ts src/context/Branding.context.tsx src/context/brandingDocument.ts src/context/brandingDocument.test.ts src/App.tsx
git commit -m "feat: load effective project branding"
```

### Task 9: react-template Identity Surfaces and Safe Image Fallbacks

**Files:**
- Create: `../react-template/src/components/BrandedImage.tsx`
- Create: `../react-template/src/components/BrandedImage.test.ts`
- Modify: `../react-template/src/components/header/Header.tsx`
- Modify: `../react-template/src/common/Sidebar.tsx`
- Modify: `../react-template/src/pages/Login.tsx`
- Modify: `../react-template/src/navigation/RouteLoadingFallback.tsx`
- Create: `../react-template/src/components/brandingSelection.ts`
- Create: `../react-template/src/components/brandingSelection.test.ts`

**Interfaces:**
- Consumes: `useBranding` from Task 8.
- Produces: `BrandedImage` with URL/fallback handling and consistently branded header/sidebar/login/loading surfaces.

- [ ] **Step 1: Write failing selection and fallback tests**

Test expanded navigation selects `logoUrl`, collapsed navigation selects `compactLogoUrl`, missing compact selects logo, broken custom image invokes the bundled `<Logo>`, and disabled login branding uses AutoTable identity.

Run from `../react-template`: `yarn test src/components/brandingSelection.test.ts src/components/BrandedImage.test.ts`

Expected: FAIL because the helpers/components are missing.

- [ ] **Step 2: Implement `BrandedImage`**

Track failure per URL and clear the failed state when URL changes. Render an `<img>` with `object-contain`, explicit dimensions, alt text, and `onError`; render the existing `Logo` component as fallback. Do not retry a broken URL on every render.

- [ ] **Step 3: Brand the header**

Preserve explicit `Header` props as the highest local override for reusable call sites; otherwise use provider values. Replace the rounded/cropped image styling with contained sizing and display effective name.

- [ ] **Step 4: Brand the sidebar**

Add an identity row above search/navigation. Expanded mode renders primary logo plus display name; collapsed mode renders compact logo. Keep the existing toggle accessible and avoid increasing the fixed 3.5rem top-bar height.

- [ ] **Step 5: Brand login and loading/error identity surfaces**

When `loginBrandingEnabled` is true, show effective logo/name above the login form. Otherwise retain AutoTable. Update `RouteLoadingFallback` and the existing full-page loading/error blocks only where they currently show product identity; do not add decorative logos to business content or 404 text.

- [ ] **Step 6: Run frontend tests, lint, and build**

Run from `../react-template`: `yarn test && yarn lint && yarn build`

Expected: PASS.

- [ ] **Step 7: Commit runtime UI slice**

```bash
cd ../react-template
git add src/components src/common/Sidebar.tsx src/pages/Login.tsx src/navigation/RouteLoadingFallback.tsx
git commit -m "feat: render branding across project identity surfaces"
```

### Task 10: Cross-Repository Verification and Operational Documentation

**Files:**
- Modify: `DEPLOYMENT_GUIDE.md`

**Interfaces:**
- Consumes: all prior tasks.
- Produces: deployable, documented feature with evidence from all test/build suites.

- [ ] **Step 1: Document operational behavior**

Document accepted formats (PNG/JPEG/WebP), 2 MiB limit, required existing Cloudinary variables (`CLOUD_NAME`, `CLOUD_API_KEY`, `CLOUD_API_SECRET`), public delivery/private management distinction, deployment order (backend, tenantPanel, react-template), and orphan-cleanup log behavior. Do not document SVG or roles as implemented.

- [ ] **Step 2: Run backend formatting and complete verification**

Run from `autotable-Go`:

```bash
gofmt -w models/branding.go models/branding_test.go models/tenantModel.go utils/imageUpload.go utils/imageUpload_test.go files/upload_service.go files/upload_service_test.go controllers/brandingController.go controllers/branding_helpers_test.go controllers/branding_runtime_test.go routes/brandingRoutes.go routes/routes_test.go controllers/projectController.go controllers/project_templates_test.go main.go
go test ./...
```

Expected: formatting makes no semantic changes and all packages PASS.

- [ ] **Step 3: Run tenantPanel complete verification**

Run from `../tenantPanel`: `yarn test && yarn lint && yarn build`

Expected: all commands exit 0.

- [ ] **Step 4: Run react-template complete verification**

Run from `../react-template`: `yarn test && yarn lint && yarn build`

Expected: all commands exit 0.

- [ ] **Step 5: Perform manual smoke test**

Use two projects in one tenant. Configure tenant defaults, override only name/logo in project A, leave project B inherited, and verify:

```text
project A: overridden name/logo + inherited color/favicon
project B: all tenant values
logged-out login: correct project identity
expanded/collapsed sidebar: main/compact asset
browser navigation A -> B: title/favicon/color replace without stale A values
broken/deleted delivery asset: AutoTable fallback, application remains usable
reset project A logo: tenant logo becomes effective
```

- [ ] **Step 6: Review repository status and commit documentation**

Run in each repository: `git status --short` and `git diff --check`.

Confirm only intended files are changed. Then, in `autotable-Go`:

```bash
git add DEPLOYMENT_GUIDE.md
git commit -m "docs: document project branding operations"
```

- [ ] **Step 7: Record final evidence**

In the handoff, report the three final commit hashes, exact verification commands and exit results, any intentionally deferred items from Global Constraints, and whether the manual two-project smoke test was completed or remains for an environment owner.
