# Split Page Routes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split end-user runtime page loading from admin page management so `react-template` uses authenticated `/page` and `tenantPanel` uses authenticated `/admin/page`.

**Architecture:** Backend registers runtime read routes under `/page` and admin CRUD routes under `/admin/page`. `react-template` requests runtime pages from `/page`; `tenantPanel` requests full admin page config from `/admin/page`.

**Tech Stack:** Go Fiber backend, React TypeScript frontends, TanStack Query.

## Global Constraints

- Remove `/page/public` from react-template and backend route registration.
- Keep runtime page loading authenticated and filtered by page authentication/authorization settings.
- Keep tenantPanel page editing on full admin page payloads.
- Preserve existing project/tenant URL prefix behavior.

---

### Task 1: Backend Page Route Split

**Files:**
- Modify: `routes/pageRoutes.go`
- Modify: `routes/routes_test.go`
- Test: `go test ./routes ./controllers ./middlewares`

**Interfaces:**
- Produces: `GET /page` runtime route handled by `controllers.GetAllPagesPublic`.
- Produces: `/admin/page` admin CRUD/list routes handled by existing page CRUD controllers.

- [ ] **Step 1: Write failing route tests**

Add assertions that `/pages/public` is not registered and `/pages` still requires auth for runtime pages.

- [ ] **Step 2: Implement route split**

Move admin CRUD to `adminGroup := app.Group("/admin" + baseUrl)` and bind runtime `GET baseUrl` to `GetAllPagesPublic`.

- [ ] **Step 3: Run backend tests**

Run: `go test ./routes ./controllers ./middlewares`
Expected: PASS.

### Task 2: Frontend Page API Split

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/api/page.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/page.ts`
- Test: targeted frontend tests/builds.

**Interfaces:**
- Consumes: backend `GET /page` runtime route.
- Consumes: backend `/admin/page` admin route group.

- [ ] **Step 1: Update react-template**

Change `useGetAllPages()` to request `/page`.

- [ ] **Step 2: Update tenantPanel**

Change page API path builder to use `/admin/page`.

- [ ] **Step 3: Run frontend checks**

Run targeted tests, touched-file lint, and builds.
Expected: PASS except existing repo-wide lint issues if not touched.
