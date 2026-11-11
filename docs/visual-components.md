# Project visual component library

Tenant Panel saves reusable designs independently of pages at:

- `GET /api/v1/:tenantSlug/:projectSlug/admin/visual-components`
- `POST /api/v1/:tenantSlug/:projectSlug/admin/visual-components`
- `PUT /api/v1/:tenantSlug/:projectSlug/admin/visual-components/:id`

These strict management routes use project config-read permission for listing
and project page-write permission for creating/updating. Runtime credentials do
not authorize management. The existing project context resolver and management
authorizer validate scope before handlers run.

Create body: `{ "name": "Card", "value": { "document": {...}, "theme": {...} } }`.
Update adds the revision obtained from the previous response. Responses contain
`id`, `name`, `value`, `revision`, `createdAt` and `updatedAt`; internal tenant and
project identifiers are not returned. Lists return arrays, including `[]` when
empty. Create returns201, update200, malformed metadata/envelopes or missing revisions400, stale updates or
missing update targets409. The frontend validates the full engine schema;
the backend checks the basic envelope, required name and 2MiB value size limit.

`project_visual_components` is shared metadata storage, indexed by tenant,
project and update time. Every query/update filters tenantId + projectId; updates
also filter id + revision and increment revisions atomically. Authored value is
stored as raw JSON bytes in BSON and embedded as JSON in responses, preserving
case-sensitive keys and hyphenated theme/effect tokens. Database round-trip tests
cover this boundary.

Project template cloning copies definitions into the target scope with new IDs
and revision1; cleanup removes only the deleted project's library rows. Existing
page placements embed independent values and render without library lookups.
No existing page data is automatically migrated. A placed-copy editor can save
an explicit independent copy into the library.

No library-delete endpoint or production migration is introduced. Startup ensures
the new scope index and registers routes. A backend without hot reload must be
restarted to expose them. The running local backend was checked without credentials
and returned401 for the new protected route, versus404 for a nonexistent route;
verification did not create or edit real project records.
