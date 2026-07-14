# Project Templates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add tenant/global project templates and project creation from templates, including optional dynamic item cloning with fresh IDs.

**Architecture:** Store template metadata on existing `Project` records. Backend exposes template listing and tenant-template update endpoints, then extends project creation to clone scoped project collections from a visible template. Tenant panel adds template management and template selection in the create-project modal.

**Tech Stack:** Go/Fiber/MongoDB backend, React/TypeScript tenant panel, React Query API hooks.

## Global Constraints

- Global templates are DB/admin-controlled; tenant panel must not expose global publishing.
- Tenant owners/admins can manage tenant templates.
- Copied project resources must get fresh Mongo `_id` values.
- Copied objectId fields referencing copied records must be remapped to target IDs.
- Project memberships must not be copied.
- `auth` dynamic records must not be copied.
- Project creation without a template must preserve current default-schema behavior.

---

### Task 1: Backend Project Template Model and API Shape

**Files:**
- Modify: `models/tenantModel.go`
- Modify: `controllers/projectController.go`
- Modify: `routes/projectRoutes.go`
- Test: `controllers/project_templates_test.go`

**Interfaces:**
- Produces: `Project.IsTemplate`, `Project.TemplateScope`, `Project.TemplateIncludeItems`, `Project.TemplateDescription`.
- Produces: `GET /api/v1/tenant/projects/templates`.
- Produces: `PATCH /api/v1/tenant/projects/:id/template`.

- [ ] **Step 1: Write failing tests**

Add tests that seed projects with `isTemplate`, `templateScope`, and tenant IDs, then verify:

```go
func TestGetProjectTemplatesIncludesTenantAndGlobalTemplates(t *testing.T) {
    // Current tenant sees its tenant templates and all global templates.
}

func TestUpdateProjectTemplateWritesTenantScopeOnly(t *testing.T) {
    // PATCH /:id/template on own tenant project stores templateScope tenant.
}

func TestUpdateProjectTemplateRejectsOtherTenantProject(t *testing.T) {
    // PATCH /:id/template on another tenant's project returns not found/forbidden.
}
```

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./controllers -run 'Test(GetProjectTemplates|UpdateProjectTemplate)'`

Expected: fail because handlers/routes/model fields do not exist yet.

- [ ] **Step 3: Implement model fields, handlers, and routes**

Add template fields to `models.Project`. Add `GetProjectTemplates` and `UpdateProjectTemplate` to `controllers/projectController.go`. Add routes before `/:id` so `templates` is not captured as an ID.

- [ ] **Step 4: Run tests and verify pass**

Run: `go test ./controllers -run 'Test(GetProjectTemplates|UpdateProjectTemplate)'`

Expected: pass.

### Task 2: Backend Template Clone Service

**Files:**
- Modify: `controllers/projectController.go`
- Test: `controllers/project_templates_test.go`

**Interfaces:**
- Produces: `CreateProjectInput.TemplateProjectID string`.
- Produces: `CreateProjectInput.IncludeTemplateItems *bool`.
- Produces: clone helpers used by `CreateProject`.

- [ ] **Step 1: Write failing tests**

Add tests that verify:

```go
func TestCreateProjectFromTemplateClonesContainersPagesAndItems(t *testing.T) {
    // New project has copied containers/pages/items with fresh IDs.
}

func TestCreateProjectFromTemplateRemapsCopiedObjectIDReferences(t *testing.T) {
    // Copied child record points at copied parent ID, not source parent ID.
}

func TestCreateProjectFromTemplateDoesNotCopyAuthItems(t *testing.T) {
    // auth container structure exists but auth records are absent.
}

func TestCreateProjectRejectsInaccessibleTemplate(t *testing.T) {
    // Another tenant's tenant-scoped template cannot be used.
}
```

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./controllers -run 'TestCreateProjectFromTemplate'`

Expected: fail because `CreateProject` ignores template fields.

- [ ] **Step 3: Implement template visibility and cloning**

Add helpers for template lookup, collection cloning, dynamic record ID mapping, objectId remapping from container field metadata, and rollback of newly created project-scoped collections when clone fails.

- [ ] **Step 4: Run tests and verify pass**

Run: `go test ./controllers -run 'TestCreateProjectFromTemplate'`

Expected: pass.

### Task 3: Tenant Panel API Hooks and Types

**Files:**
- Modify: `../tenantPanel/src/utils/api/project.ts`
- Modify: `../tenantPanel/src/types/index.ts`

**Interfaces:**
- Produces: `Project.isTemplate`, `Project.templateScope`, `Project.templateIncludeItems`, `Project.templateDescription`.
- Produces: `CreateProjectPayload.templateProjectId`, `CreateProjectPayload.includeTemplateItems`.
- Produces: `useProjectTemplates()` and `useUpdateProjectTemplate()`.

- [ ] **Step 1: Add frontend types and hooks**

Extend project types and add hooks for `GET /tenant/projects/templates` and `PATCH /tenant/projects/:id/template`.

- [ ] **Step 2: Run TypeScript check through build**

Run: `yarn build` in `../tenantPanel`.

Expected: pass after UI task is complete; temporary type errors may remain until Task 4.

### Task 4: Tenant Panel Project UI

**Files:**
- Modify: `../tenantPanel/src/pages/ProjectsPage.tsx`

**Interfaces:**
- Consumes: Task 3 hooks and types.
- Produces: tenant template toggle and create-project template selector.

- [ ] **Step 1: Add tenant template controls**

Add a template action on project cards for tenant owners/admins. When enabling, prompt for include-items choice and call `useUpdateProjectTemplate`.

- [ ] **Step 2: Add template selection to create modal**

Add a template select populated from `useProjectTemplates`. When selected, default `includeTemplateItems` from the selected template and send template fields in `createProject`.

- [ ] **Step 3: Build tenant panel**

Run: `yarn build` in `../tenantPanel`.

Expected: pass.

### Task 5: Full Verification

**Files:**
- Backend and tenant panel touched files.

- [ ] **Step 1: Run backend tests**

Run: `go test ./...`

Expected: pass.

- [ ] **Step 2: Run tenant panel build**

Run: `yarn build` in `../tenantPanel`.

Expected: pass.

- [ ] **Step 3: Check git status**

Run: `git status --short` in backend and tenant panel.

Expected: only intended source/test/doc changes plus pre-existing unrelated dirty files.
