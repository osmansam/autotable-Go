# Tenant panel login recovery

The reported browser sequence was:

```text
POST /api/v1/tenant/auth/login           401
POST /api/v1//tenant/auth/refresh        404
```

The tenant panel treated the unsuccessful login as an expired session and
concatenated a base URL ending in `/` with a refresh path starting in `/`.
That hid the original login response behind a second failed request.

## Authentication boundaries

| Identity | Entry | Refresh | Cookie/claim boundary |
| --- | --- | --- | --- |
| Tenant management | `/tenant/auth/login` | `/tenant/auth/refresh` | Tenant cookie namespace, tenant role scope, no project ID. |
| Project management | Explicit `/tenant/auth/switch-project` after tenant login | `/:tenant/:project/tenant-auth/refresh` | Separate management-project cookie namespace and exact project binding. |
| Application user | `/:tenant/:project/auth/login` | `/:tenant/:project/auth/refresh` | Separate application-user cookie namespace, JWT audience/signing configuration and account collection. |

Paths in this table are relative to `/api/v1`. The fix preserves membership
authorization, token validation, cookie security attributes and the distinction
between management users and application users.

## Changes

- Login, registration, OAuth and logout failures return their own error instead
  of triggering management refresh. Protected tenant session and management
  requests still recover expired access sessions.
- Axios joins refresh URLs using the configured client base URL. Refresh
  requests are deduplicated by full URL, keeping tenant and project identities
  separate. A failed project refresh leaves the tenant login state intact.
- The tenant session endpoint returns 401 when access is invalid but a tenant
  refresh cookie remains. No cookies still produces an anonymous 200 response.
- Only complete tenant-scoped management access claims authenticate that session
  endpoint. A project-management token placed in a tenant cookie is rejected.
- Successful tenant login, registration and OAuth callback clear previous query
  data/project selection and explicitly store tenant role scope.

## Verification and rollout

Regressions reproduced the incorrect login refresh, doubled URL, cross-scope
refresh sharing, project failure logging out the tenant, missed tenant recovery,
incorrect session claim acceptance and stale project state after sign-in.

The backend roundtrip test uses real signed tokens, HTTP requests, cookie
serialization and membership-policy checks with mocked membership/session
storage. It verifies that root tenant refresh restores the tenant session and
does not modify either project cookie namespace. The existing application
session/refresh regressions also run unchanged.

Validation: `go test ./...` passed with local test-server sockets allowed;
the panel suite passed all 603 tests in 111 files; `npm run build` passed.
An independent authentication-boundary review found no blocking issues.

Release the tenant panel fix before or together with the backend session fix.
These are local source changes; no production account, password, secret,
membership or deployment was changed during this work.

The initial login 401 is separate from the refresh 404. TenantLogin returns 401
when the global tenant user lookup fails or bcrypt verification fails; it does
not authenticate application-user credentials. The original login response
and server-side lookup diagnostics are needed to diagnose a continuing 401.
This patch preserves credential checks and does not convert a rejected login
into a successful session.
