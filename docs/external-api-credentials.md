# Sharing external API credentials between projects

External API credentials authenticate outbound workflow `call_api` steps. A tenant-owned credential stores one encrypted provider key and can be connected to selected projects in that tenant. Other tenants and unconnected projects cannot use it.

The same provider account/key is used for every connected project. Provider quotas, credits and billing remain with that account; this feature does not introduce per-project provider budgets.

## Manage keys in Tenant Panel

Tenant owners and tenant admins can open **External APIs** in tenant context, without selecting a project:

1. Create a credential with its name, authentication type, provider token, allowed destination domains and optional expiry.
2. Connect the projects that should be allowed to use it.
3. Use the credential ID in those projects' workflow `call_api` steps.

The token is encrypted on the server and is not returned in metadata. Keep the existing `EXTERNAL_API_CREDENTIAL_KEY` configured; losing or changing that encryption key without migrating ciphertext prevents existing keys from being decrypted.

Existing project-only credentials remain valid. Use **Promote** from tenant management to make one reusable without changing its ID or re-entering its token. Promotion preserves access for its original project. Grant additional projects explicitly afterward.

Project Integrations lists both its own credentials and connected tenant credentials. Project-only users cannot grant themselves access or revoke a tenant key globally.

## Create a project from a template

Choose a template in **Create Project**. Its External APIs section lists each credential connection required by the copied workflows. Select an existing tenant credential for each connection, or explicitly save a new credential and select it.

You may also select additional credentials to connect to the project. The project-creation request carries only credential IDs and mappings; it never includes provider tokens. If project creation fails, an explicitly saved tenant credential remains available for retry.

Templates carry workflow references, not secrets. All credential references, including inactive and nested steps, must be mapped. A global template from another tenant uses your tenant's replacement credentials; it does not grant access to the template author's keys. Invalid, revoked, expired or foreign credentials are rejected. Static destination hosts are checked while binding; dynamic destinations are checked when the workflow executes.

Example project-create request:

```json
{
  "name": "Customer portal",
  "slug": "customer-portal",
  "templateProjectId": "111111111111111111111111",
  "includeTemplateItems": true,
  "externalAPICredentialIds": ["333333333333333333333333"],
  "externalAPICredentialBindings": {
    "222222222222222222222222": "333333333333333333333333"
  }
}
```

Here `222…` is the credential reference in the template, and `333…` is the selected tenant-owned credential. The server validates ownership, connects the new project and rewrites the copied `call_api` references. It does not modify the source template.

## Disconnect, revoke and delete

- **Disconnect a project:** future executions in that project lose access; other connected projects keep access.
- **Revoke a shared credential:** future executions in every connected project are blocked.
- **Delete a project:** its grants and project-owned keys are removed. Tenant-owned keys and other projects' connections are preserved.
- **Failed project creation:** new-project grants are cleaned up; existing shared keys are not deleted.

Checks apply on subsequent credential lookups. An outbound request already authorized/sent cannot be recalled.

## Management API

These routes use tenant management authentication and live owner/admin authorization, independently of project management roles. Relative to `/api/v1`:

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/tenant/integrations/external-api-credentials` | List credential metadata |
| POST | `/tenant/integrations/external-api-credentials` | Create a tenant-owned credential |
| POST | `/tenant/integrations/external-api-credentials/:id/promote` | Promote a legacy project credential |
| POST | `/tenant/integrations/external-api-credentials/:id/revoke` | Revoke a shared credential |
| PUT | `/tenant/integrations/external-api-credentials/:id/projects/:projectId` | Connect a project idempotently |
| DELETE | `/tenant/integrations/external-api-credentials/:id/projects/:projectId` | Disconnect a project |
| GET | `/tenant/projects/templates/:id/external-api-requirements` | Read safe template connection requirements |

Existing project credential routes remain supported. Shared metadata includes `scope: "tenant"` and `grantedProjectIds`; absent scope in stored legacy records means project ownership. No metadata response exposes plaintext or encrypted secret values.

## Rollout

Deploy the backend and Tenant Panel changes together. Existing keys require no automatic migration; promoting them is explicit. Templates containing credential references intentionally require bindings when copied, so API clients creating projects from those templates must send the new mapping fields.

Shared grants and promotion use MongoDB transactions to coordinate with project deletion. MongoDB must support transactions (a replica set or supported sharded deployment); these operations do not support standalone MongoDB.

If project creation fails and cleanup also fails, the project is kept inactive so its owner can retry deletion. Shared credentials are preserved, and grant cleanup is attempted even if dropping a project collection fails.

Verification uses disposable local projects, keys and provider fixtures. No production credential, tenant, provider account or hosted configuration needs to be changed to test this feature.
