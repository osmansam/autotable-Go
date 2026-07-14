# Project Templates Design

## Goal

Allow tenant users to mark projects as reusable templates, choose whether the template includes existing dynamic data records, and create new projects from tenant or global templates. Global templates are DB/admin-controlled for now.

## Current Context

Projects are stored in the `projects` collection. Project-specific resources use tenant/project scoped MongoDB collections:

- `tenant_{tenantId}_project_{projectId}_containers`
- `tenant_{tenantId}_project_{projectId}_pages`
- `tenant_{tenantId}_project_{projectId}_{schemaName}` for dynamic records

Project creation currently creates a new project, adds a project-admin membership for the creator, initializes project containers, creates default `role` and `auth` schemas, creates notification indexes, and returns project-scope tokens.

## Product Behavior

Tenant owners and tenant admins can mark an existing project as a tenant template from the tenant panel. When marking a project as a template, the panel asks whether current dynamic items should be included by default.

Global templates are not managed from tenant panel initially. They are enabled by directly updating the source project document in the database with template metadata. This keeps global publishing out of normal tenant-user workflows.

When creating a project, the modal offers an optional template selector. The selector lists active template projects visible to the tenant:

- Tenant templates from the current tenant.
- Global templates from any tenant/project where `templateScope` is `global`.

If no template is selected, project creation behaves like it does today.

If a template is selected, the creator can choose whether to include template items. The default value comes from the template project's saved `templateIncludeItems` value.

## Data Model

Add template metadata to `models.Project`:

```go
IsTemplate          bool   `bson:"isTemplate,omitempty" json:"isTemplate,omitempty"`
TemplateScope       string `bson:"templateScope,omitempty" json:"templateScope,omitempty"`
TemplateIncludeItems bool  `bson:"templateIncludeItems,omitempty" json:"templateIncludeItems,omitempty"`
TemplateDescription string `bson:"templateDescription,omitempty" json:"templateDescription,omitempty"`
```

Valid `templateScope` values:

- `tenant`: visible only within the source tenant.
- `global`: visible to all tenants, but initially set only by DB/admin update.

Create project request gains:

```go
TemplateProjectID    string `json:"templateProjectId,omitempty"`
IncludeTemplateItems *bool  `json:"includeTemplateItems,omitempty"`
```

If `IncludeTemplateItems` is omitted, backend uses the selected template project's `TemplateIncludeItems`.

## Backend API

Add a template listing endpoint under tenant-scope project routes:

```http
GET /api/v1/tenant/projects/templates
```

It returns active projects where:

- `isTemplate: true`
- and either:
  - `tenantId` equals the current tenant and `templateScope` is `tenant`
  - or `templateScope` is `global`

Add a tenant template update endpoint:

```http
PATCH /api/v1/tenant/projects/:id/template
```

Tenant owners/admins can set:

```json
{
  "isTemplate": true,
  "templateIncludeItems": true,
  "templateDescription": "Optional description"
}
```

This endpoint always writes `templateScope: "tenant"` and only allows projects owned by the current tenant. It cannot create global templates.

`CreateProject` accepts optional template fields. It validates that the selected template is visible to the current tenant. It creates the new project and membership exactly as today, then either initializes default schemas or clones template resources.

## Clone Semantics

When a template is selected, the backend clones project resources into the newly created project:

1. Clone container metadata from the source project's containers collection into the target project's containers collection.
2. Clone pages from the source project's pages collection into the target project's pages collection.
3. Recreate indexes for target containers using existing `utils.EnsureIndexes`.
4. Create notification indexes for the target project.
5. If item copying is enabled, clone dynamic records for each source container schema.

Every copied document gets a fresh MongoDB `_id`.

When dynamic records are copied, the backend builds an ID map for all copied records by schema. It uses container field metadata to remap objectId fields whose `objectSchemaName` points to another copied schema. This keeps copied records linked to the newly copied records instead of the source project records. ObjectId values that do not match a copied source record are preserved as-is.

Membership documents are never copied.

Project metadata is never copied except through the explicit new project request fields. The new project always gets its own name, slug, tenant, membership, timestamps, and tokens.

The `auth` dynamic records are not copied even when item copying is enabled. Copying login users/password hashes into a new project is risky. The `auth` container structure may be copied, but the new project starts with no copied auth users. Role seed behavior should preserve at least an admin role for usability.

## Failure Handling

The clone should run after the project and creator membership are inserted. If cloning fails, backend should roll back the newly created project, creator membership, and newly created project-scoped collections where feasible.

If rollback cannot clean every collection, the API should return a failure response and log the cleanup failure. It should not return tokens for a partially cloned project.

## Frontend

`tenantPanel/src/pages/ProjectsPage.tsx` changes:

- Add template controls on project cards for tenant owners/admins.
- When enabling template mode, ask whether current items should be included.
- Add optional template selector in the create-project modal.
- Add include-items toggle that appears after a template is selected.
- Use the template's `templateIncludeItems` value as the default toggle state.

`tenantPanel/src/utils/api/project.ts` changes:

- Extend `Project` and `CreateProjectPayload` types.
- Add `useProjectTemplates`.
- Add `useUpdateProjectTemplate`.

`react-template` does not need project-template UI changes because project management lives in tenant panel, not runtime app templates.

## Testing

Backend tests should cover:

- Template listing returns tenant templates and global templates visible to the current tenant.
- Tenant template update cannot mark another tenant's project.
- Tenant template update always writes `templateScope: "tenant"`.
- Project creation without a template still initializes default schemas.
- Project creation from a template clones containers and pages with new IDs.
- Project creation from a template with items clones dynamic records with new IDs.
- Copied objectId fields are remapped to the copied target record IDs when the referenced source record was copied.
- `auth` dynamic records are not copied.
- Invalid or inaccessible template IDs are rejected.

Frontend verification should cover:

- TypeScript build passes.
- Create project modal sends template fields only when a template is selected.
- Include-items toggle defaults from the selected template.

## Non-Goals

- No tenant-panel UI for creating global templates in this iteration.
- No template versioning or frozen snapshots.
- No copying project memberships.
- No copying auth dynamic records.
