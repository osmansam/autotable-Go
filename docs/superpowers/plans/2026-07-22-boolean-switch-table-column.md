# Boolean Switch Table Column Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `booleanSwitch` as an editable boolean table column type.

**Architecture:** Extend shared table column types in tenantPanel and react-template, add the Page Designer option, and update table renderers to use the existing row update mutation for switch toggles. Keep existing `boolean` badge behavior unchanged.

**Tech Stack:** React, TypeScript, Vite, Vitest, existing `CheckSwitch` component and dynamic row update hooks.

## Global Constraints

Do not change backend validation for table column type strings.
Do not change existing `boolean` display-only badge behavior.
Use the existing `CheckSwitch` component and existing update row function.
Do not touch generated `dist` files.

---

### Task 1: TenantPanel Type Option

**Files:**
- Modify: `../tenantPanel/src/types/page.ts`
- Modify: `../tenantPanel/src/types/layout.ts`
- Modify: `../tenantPanel/src/utils/api/page.ts`
- Modify: `../tenantPanel/src/utils/pageDesignerTableConfig.ts`
- Test: `../tenantPanel/src/utils/pageDesignerTableConfig.test.ts`

**Interfaces:**
- Produces: `booleanSwitch` as `TableColumnType` and `TABLE_COLUMN_TYPE_OPTIONS` entry `{ value: "booleanSwitch", label: "Boolean Switch" }`.

### Task 2: TenantPanel Table Renderer

**Files:**
- Modify: `../tenantPanel/src/components/panelComponents/FormElements/GenericPaginatedPage.tsx`
- Modify: `../tenantPanel/src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx`

**Interfaces:**
- Consumes: `columnConfig.type === "booleanSwitch"`.
- Produces: editable `CheckSwitch` cells that call `updateDynamicItem(row._id, { [fieldName]: nextBoolean })`.

### Task 3: React Template Runtime

**Files:**
- Modify: `../react-template/src/types/page.ts`
- Modify: `../react-template/src/types/layout.ts`
- Modify: `../react-template/src/components/panelComponents/FormElements/GenericPaginatedPage.tsx`
- Modify: `../react-template/src/components/panelComponents/FormElements/GenericUnpaginatedPage.tsx`

**Interfaces:**
- Consumes: persisted table config with `type: "booleanSwitch"`.
- Produces: same inline switch update behavior in end-user runtime tables.

### Task 4: Verification

Run:

```bash
yarn test src/utils/pageDesignerTableConfig.test.ts
yarn build
```

from `../tenantPanel`.

Run:

```bash
yarn build
```

from `../react-template`.
