# HTTP-Only Cookie Authentication Design

## Goal

Move browser authentication for `react-template` and the related `tenantPanel` project-switch flow from JavaScript-readable tokens to server-set HTTP-only cookies while retaining Bearer-token support for integrations and non-browser API clients.

The production applications are:

- React runtime: `https://panel.autoapi.org`
- Tenant administration: `https://tenant.autoapi.org`
- API: `https://api-production.autoapi.org`

All three hosts are same-site subdomains of `autoapi.org`.

## Security invariants

1. Access and refresh tokens never appear in local storage, JavaScript-readable cookies, response bodies consumed by browser login flows, Google redirect URLs, or popup messages.
2. A project token is valid for exactly one tenant and project. Backend middleware compares its signed tenant/project claims with the requested tenant/project context on every protected request.
3. Only one project session is active in a browser at a time. An authorized project switch replaces the project cookie pair. A token issued for one project cannot access another project's URL.
4. Tenant and project sessions use separate cookie names so tenant administration and one selected project can remain authenticated simultaneously.
5. Existing integration and API-client Bearer tokens remain supported. Browser cookie authentication must not weaken integration-token permission checks.
6. Cookie-authenticated unsafe requests are accepted only from configured trusted origins.
7. Middleware records whether the selected credential came from an explicit Bearer header or a cookie. If an Authorization header is present but malformed, invalid, or expired, authentication fails without falling back to cookies.

## Cookie model

The API sets four host-only cookies. Production uses the browser-enforced `__Host-` prefix while local HTTP development uses unprefixed equivalents:

- `__Host-tenant_access_token` / `tenant_access_token`
- `__Host-tenant_refresh_token` / `tenant_refresh_token`
- `__Host-project_access_token` / `project_access_token`
- `__Host-project_refresh_token` / `project_refresh_token`

All cookies use `HttpOnly`, `SameSite=Lax`, and `Path=/`. Production cookies also use `Secure`. No `Domain` attribute is set, making them host-only to `api-production.autoapi.org`. Local HTTP development omits `Secure` through environment-aware cookie configuration.

Refresh cookies expire after seven days. Access cookies use the access-token lifetime already encoded by the token generator. Every successful refresh creates a new access token and rotates the refresh token.

Access-cookie `Max-Age` never exceeds the JWT expiration. Clearing uses the same name, path, `Secure`, and `SameSite` attributes as setting. Tests cover historical unprefixed names and paths used by the existing clients. Application, reverse-proxy, tracing, and debugging logs must not record Cookie headers, token values, OAuth codes, or authorization headers.

Central backend helpers own cookie creation and clearing so login, registration, Google OAuth, project switching, refresh, and logout cannot drift in flags or names.

## Backend authentication flow

Project authentication middleware resolves credentials in this order:

1. An explicit `Authorization` header, preserving API-client and integration behavior.
2. The `project_access_token` cookie for browser project routes.

The resolved credential includes an `AuthSource` value (`bearer` or `cookie`) stored in request locals. Origin/CSRF policy uses this selected source, not the mere presence of cookies. Valid Bearer credentials accompanied by unrelated cookies remain Bearer-authenticated. An invalid explicit Authorization header is terminal and never falls back to a valid cookie.

Tenant middleware follows the same rule using `tenant_access_token`. Optional authentication also reads the relevant cookie so public routes can apply row access and role-aware behavior for signed-in users.

After parsing a project token, middleware continues to compare signed tenant and project IDs with the request context resolved from `/t/:tenant/p/:project` or the API's equivalent tenant/project path. A mismatch returns `403`; an invalid or expired credential returns `401`.

## Login, registration, and session bootstrap

Project login and registration set the project cookie pair and return only safe session data such as the user. Tenant login and registration set the tenant cookie pair. Token strings are omitted from browser-facing JSON.

Authenticated session endpoints return the safe information the frontends previously derived by decoding JWTs:

- authentication status;
- user identity and display data;
- role or roles;
- tenant and project identifiers/slugs;
- session scope.

Session responses use `Cache-Control: no-store` and `Pragma: no-cache` and return only fields required by the UI. Roles are presentation state only; every permission remains independently enforced by the backend. Anonymous bootstrap returns `200` with `{ "authenticated": false }` so unauthenticated startup is an expected state rather than a request failure.

The project endpoint validates the requested tenant/project context before returning data. `react-template` route guards use this endpoint rather than reading or decoding a token. Audit-log visibility and other role-aware UI use returned session state.

## Refresh and request retry

Axios sends cookies using `withCredentials: true` and does not construct Bearer headers from local storage.

When a project API request receives `401`, the client performs at most one shared refresh request using the project refresh cookie. Concurrent failures share that refresh operation. After a successful refresh, each original request retries once. Login, registration, refresh, logout, and already-retried requests do not enter a refresh loop. A failed refresh clears client session state and routes the user to the correct tenant/project login page. A project-context `403` never triggers refresh; it tells a stale tab that the active project changed or that access is forbidden.

Tenant API requests use the equivalent tenant refresh endpoint and cookie. Refresh endpoints read refresh tokens only from HTTP-only cookies for browser flows.

Access and refresh JWTs have explicit token-type, scope, audience, unique token ID, and session-family claims. A refresh endpoint rejects access tokens and rejects a tenant token used for project refresh or the reverse.

Refresh tokens rotate on every successful use. The backend stores only hashed refresh-token identifiers and family/session state in a dedicated auth-session collection, supports logout/family revocation and reuse detection, and atomically advances the current token. A concurrent request presenting the immediately previous token during a short grace window receives a retryable conflict and does not revoke the family; the client retries with the newly set cookie. Reuse outside that grace window revokes the family. Expired session records are removed through a TTL index.

## Logout

Backend logout clears the relevant access and refresh cookies with attributes matching those used when setting them. Project logout does not destroy the tenant administration session. Tenant logout clears tenant cookies; project cookies may be cleared separately when the user explicitly leaves the selected project.

The frontends clear cached user/query state after successful logout. A temporary non-sensitive local-storage event may remain solely for cross-tab logout notification, but local storage contains no credentials.

## Google OAuth

The API callback sets the project cookie pair before redirecting to the frontend callback route. Redirect URLs contain only a non-sensitive success/error indicator and tenant/project routing context; they contain no access token, refresh token, or serialized user record.

OAuth state is generated cryptographically, expires after five minutes, is bound to the intended tenant/project flow, and is consumed atomically so replayed callbacks fail. The flow uses PKCE with a per-attempt verifier stored alongside the state; the callback supplies the verifier during code exchange. Missing, expired, altered, or replayed state is rejected before cookies are set.

The frontend callback or popup opener calls the project session endpoint to obtain safe user state. Popup messages contain only success/error status. The same one-project-at-a-time cookie replacement rule applies.

## Tenant project switching

`tenantPanel` sends its tenant cookie to the protected project-switch endpoint. The backend verifies membership and authorization, then sets a new project cookie pair for the selected project. The frontend opens `panel.autoapi.org/t/:tenant/p/:project/...` without reading or forwarding a token.

Switching projects replaces the previous project cookie pair. Requests against the old project's path subsequently fail the signed context comparison.

Two tabs for different projects intentionally cannot remain active. When a stale tab receives a context-mismatch `403`, it shows that the active project changed and redirects to project selection or the appropriate login route without attempting refresh.

## CORS and CSRF protection

API CORS configuration enables credentials and explicitly permits only configured origins, including `https://panel.autoapi.org` and `https://tenant.autoapi.org`. Wildcard origins are not allowed with credentials.

Because cookies are sent automatically, cookie-authenticated `POST`, `PUT`, `PATCH`, and `DELETE` requests must include an Origin that exactly matches the same normalized trusted-origin configuration. Missing Origin, `Origin: null`, look-alike suffixes, and unconfigured scheme/host/port combinations are rejected. Referer is not a substitute. Bearer-authenticated non-browser API calls without an Origin header remain supported. JSON content-type requirements and existing body/rate limits remain in force. No `GET` or `HEAD` route may perform a state-changing action.

Origin validation occurs before every cookie-based authentication action, including refresh, logout, project switching, Google session finalization, and applicable login/registration requests. The browser adds `X-AutoTable-Client: browser` to mutations as defense in depth and to require a CORS preflight; this header does not replace Origin validation.

## WebSockets

Browser WebSocket connections cannot set arbitrary Authorization headers. The WebSocket upgrade path reads the appropriate HTTP-only cookie and performs the same token, tenant, and project checks as HTTP middleware. Cookie-authenticated browser upgrades also require an exact trusted Origin. Missing or untrusted origins are rejected. Non-browser clients using explicit Bearer authentication follow the Bearer policy and do not silently fall back to cookies. No token is placed in the WebSocket URL. Integration tests exercise the actual handshake cookie and Origin behavior.

## Reverse proxy and infrastructure

Nginx preserves Origin and every Set-Cookie header, supports WebSocket upgrades, adds `Vary: Origin` where appropriate, and disables caching for login, registration, refresh, logout, OAuth callback, project switch, and session responses. The application trusts forwarded HTTPS information only from the configured reverse proxy; an untrusted direct client cannot spoof `X-Forwarded-Proto` to alter cookie policy. Production cookie `Secure` behavior is configuration-driven and cannot be accidentally disabled merely because Nginx-to-Go traffic uses HTTP.

## Frontend migration

Both frontends remove credential reads/writes through `localStorage`, `js-cookie`, URL parameters, and browser-side JWT decoding. Axios uses credentialed requests. Authentication becomes asynchronous session state with loading, authenticated, and anonymous outcomes so private routes do not redirect before the bootstrap request completes.

On startup, migration cleanup removes legacy `jwt` and `refreshToken` local-storage entries and the old JavaScript-readable `jwt` cookie. Non-sensitive cached user data may be replaced by session bootstrap data and should not be treated as proof of authentication.

## Error behavior

- Missing, invalid, or expired access cookie: `401`.
- Valid token for a different tenant/project: `403`.
- A context-mismatch `403` does not trigger refresh and causes stale-project UI handling.
- Missing or invalid refresh cookie: `401`, with both cookies cleared.
- Concurrent use of the immediately previous rotated refresh token: retryable `409` without family revocation.
- Reuse of a rotated refresh token outside the grace window: `401` with session-family revocation.
- Untrusted Origin on a cookie-authenticated unsafe request: `403`.
- Authentication failures continue to use the shared specific toast-message handling.

## Testing

Backend tests cover:

- exact cookie names, expiry, and production/development flags;
- login, registration, Google callback, refresh, project switch, and logout cookie behavior;
- omission of tokens from JSON, redirect URLs, and popup-facing data;
- tenant/project cookie isolation;
- cookie authentication and Bearer fallback;
- invalid Bearer plus valid cookie rejection, and valid Bearer plus unrelated-cookie precedence;
- explicit rejection of cross-project requests;
- replacement of the prior project session on authorized project switch;
- trusted and untrusted Origin behavior;
- rejection of missing, null, look-alike, wrong-scheme, wrong-host, and wrong-port origins;
- refresh token type/scope enforcement, rotation, reuse revocation, and multi-tab conflict behavior;
- session no-cache headers;
- OAuth state expiry, alteration, atomic replay rejection, tenant/project binding, and PKCE;
- WebSocket cookie authentication and untrusted-Origin rejection through real upgrade handshakes;
- log/redaction checks and legacy cookie cleanup behavior.

Frontend tests cover:

- credentialed Axios requests without local-storage Authorization headers;
- session bootstrap route guards and role-dependent UI;
- one shared refresh and one retry under concurrent `401` responses;
- no refresh on project-context `403` and stale-project redirection;
- refresh failure and logout redirects;
- Google success messages without tokens;
- removal of legacy credential storage;
- absence of token strings in storage and URLs.

Full Go tests, both affected frontend test suites, TypeScript checks, and production builds must pass before completion.
