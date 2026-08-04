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

## Cookie model

The API sets four host-only cookies:

- `tenant_access_token`
- `tenant_refresh_token`
- `project_access_token`
- `project_refresh_token`

All cookies use `HttpOnly`, `SameSite=Lax`, and `Path=/`. Production cookies also use `Secure`. No `Domain` attribute is set, making them host-only to `api-production.autoapi.org`. Local HTTP development omits `Secure` through environment-aware cookie configuration.

Refresh cookies expire after seven days. Access cookies use the access-token lifetime already encoded by the token generator. Refreshing creates and sets a new access cookie; where supported by the current token model, it also rotates the refresh token.

Central backend helpers own cookie creation and clearing so login, registration, Google OAuth, project switching, refresh, and logout cannot drift in flags or names.

## Backend authentication flow

Project authentication middleware resolves credentials in this order:

1. An explicit `Authorization` header, preserving API-client and integration behavior.
2. The `project_access_token` cookie for browser project routes.

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

The project endpoint validates the requested tenant/project context before returning data. `react-template` route guards use this endpoint rather than reading or decoding a token. Audit-log visibility and other role-aware UI use returned session state.

## Refresh and request retry

Axios sends cookies using `withCredentials: true` and does not construct Bearer headers from local storage.

When a project API request receives `401`, the client performs at most one shared refresh request using the project refresh cookie. Concurrent failures share that refresh operation. After a successful refresh, each original request retries once. Login, registration, refresh, logout, and already-retried requests do not enter a refresh loop. A failed refresh clears client session state and routes the user to the correct tenant/project login page.

Tenant API requests use the equivalent tenant refresh endpoint and cookie. Refresh endpoints read refresh tokens only from HTTP-only cookies for browser flows.

## Logout

Backend logout clears the relevant access and refresh cookies with attributes matching those used when setting them. Project logout does not destroy the tenant administration session. Tenant logout clears tenant cookies; project cookies may be cleared separately when the user explicitly leaves the selected project.

The frontends clear cached user/query state after successful logout. A temporary non-sensitive local-storage event may remain solely for cross-tab logout notification, but local storage contains no credentials.

## Google OAuth

The API callback sets the project cookie pair before redirecting to the frontend callback route. Redirect URLs contain only a non-sensitive success/error indicator and tenant/project routing context; they contain no access token, refresh token, or serialized user record.

The frontend callback or popup opener calls the project session endpoint to obtain safe user state. Popup messages contain only success/error status. The same one-project-at-a-time cookie replacement rule applies.

## Tenant project switching

`tenantPanel` sends its tenant cookie to the protected project-switch endpoint. The backend verifies membership and authorization, then sets a new project cookie pair for the selected project. The frontend opens `panel.autoapi.org/t/:tenant/p/:project/...` without reading or forwarding a token.

Switching projects replaces the previous project cookie pair. Requests against the old project's path subsequently fail the signed context comparison.

## CORS and CSRF protection

API CORS configuration enables credentials and explicitly permits only configured origins, including `https://panel.autoapi.org` and `https://tenant.autoapi.org`. Wildcard origins are not allowed with credentials.

Because cookies are sent automatically, cookie-authenticated `POST`, `PUT`, `PATCH`, and `DELETE` requests must pass a server-side Origin check against the same trusted-origin configuration. Bearer-authenticated non-browser API calls without an Origin header remain supported. JSON content-type requirements and existing body/rate limits remain in force.

## WebSockets

Browser WebSocket connections cannot set arbitrary Authorization headers. The WebSocket upgrade path reads the appropriate HTTP-only cookie and performs the same token, tenant, and project checks as HTTP middleware. No token is placed in the WebSocket URL.

## Frontend migration

Both frontends remove credential reads/writes through `localStorage`, `js-cookie`, URL parameters, and browser-side JWT decoding. Axios uses credentialed requests. Authentication becomes asynchronous session state with loading, authenticated, and anonymous outcomes so private routes do not redirect before the bootstrap request completes.

On startup, migration cleanup removes legacy `jwt` and `refreshToken` local-storage entries and the old JavaScript-readable `jwt` cookie. Non-sensitive cached user data may be replaced by session bootstrap data and should not be treated as proof of authentication.

## Error behavior

- Missing, invalid, or expired access cookie: `401`.
- Valid token for a different tenant/project: `403`.
- Missing or invalid refresh cookie: `401`, with both cookies cleared.
- Untrusted Origin on a cookie-authenticated unsafe request: `403`.
- Authentication failures continue to use the shared specific toast-message handling.

## Testing

Backend tests cover:

- exact cookie names, expiry, and production/development flags;
- login, registration, Google callback, refresh, project switch, and logout cookie behavior;
- omission of tokens from JSON, redirect URLs, and popup-facing data;
- tenant/project cookie isolation;
- cookie authentication and Bearer fallback;
- explicit rejection of cross-project requests;
- replacement of the prior project session on authorized project switch;
- trusted and untrusted Origin behavior;
- WebSocket cookie authentication.

Frontend tests cover:

- credentialed Axios requests without local-storage Authorization headers;
- session bootstrap route guards and role-dependent UI;
- one shared refresh and one retry under concurrent `401` responses;
- refresh failure and logout redirects;
- Google success messages without tokens;
- removal of legacy credential storage;
- absence of token strings in storage and URLs.

Full Go tests, both affected frontend test suites, TypeScript checks, and production builds must pass before completion.
