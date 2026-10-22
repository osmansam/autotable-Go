# Auth and role schema reads follow configured route permissions

The reported request was GET `/api/v1/acme/test/dynamic/table-source?schemaName=auth&sourceType=schema`, returning 401. The same restriction affected `/dynamic/selection`.

## Cause

`ConditionalAuthentication` classified every auth container and the `role` schema as account administration before reading the selected data-route settings. Application sessions and anonymous visitors therefore hit management authentication even when the read route was active and public. Runtime schema metadata separately excluded auth and role schemas unconditionally.

## Behavior after this change

Application data reads now follow their configured `RouteSpec`:

| Request | Permission settings |
| --- | --- |
| `/dynamic/table-source?sourceType=schema` (or omitted source type) | `GetAllDynamicModelItemsWithPagination` |
| `/dynamic/selection` | `GetItemsForSelection` |
| Standard list, item, search, filter, pagination, and export reads | Corresponding route settings |

An active route without authentication, role authorization, or record-access requirements permits anonymous reads. Authentication, allowed roles, tenant/project identity, and record access remain enforced when configured. The table and select permissions are independent; enabling pagination does not enable selection.

Management sessions retain their account administration path. Plain schema table reads require management configuration-read permission; executable table sources still require schema-write permission. Account mutations and executable account sources remain management-only.

Auth/role runtime metadata becomes visible only when a supported data-read route permits the current visitor. It retains the existing safe field projection. Read-only public data requests can identify an application session from its cookie, allowing configured field visibility to use the current role. Public mutations and executable sources do not gain optional ambient cookie identity.

Strict document projection now removes hashed fields recursively. Regression tests caught nested hashed fields surviving table and parent-object selection responses; these are now omitted alongside unauthorized fields.

## Validation and rollout

New tests cover public/disabled/authenticated/authorized reads for both schemas, cookie and bearer sessions, foreign-project credentials, record-access requirements, management-cookie precedence, account write/execution restrictions, optional cookie scope, metadata visibility, and actual table/selection service response filtering with mock MongoDB.

The complete `go test ./...` suite passed. These changes are local and require backend deployment; no production data or route configuration was changed, and the reported deployment's authenticated browser flow has not been verified.
