# HTTP-Only Cookie Authentication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace browser-readable tenant/project JWTs with secure API-host HTTP-only cookie sessions while preserving explicit Bearer authentication for integrations and enforcing one active project context.

**Architecture:** Central Go helpers issue scoped typed JWTs, set environment-appropriate cookies, resolve Bearer-versus-cookie authentication, validate trusted Origins, and rotate persisted refresh-token families. Both React applications bootstrap safe server session state and send credentialed requests without reading tokens; project switching replaces the project cookie pair.

**Tech Stack:** Go 1.24, Fiber v2, MongoDB Go driver, Redis, `golang.org/x/oauth2`, React 18, TypeScript, Axios, TanStack Query, Vitest, Vite.

## Global Constraints

- Production uses host-only `__Host-` cookie names with `Secure`, `HttpOnly`, `SameSite=Lax`, and `Path=/`; local HTTP development uses unprefixed names.
- Refresh lifetime is seven days; access-cookie lifetime never exceeds JWT `exp`.
- An explicit invalid Authorization header never falls back to cookies.
- A signed tenant/project mismatch returns `403` and never triggers frontend refresh.
- Browser token values never enter local storage, JavaScript-readable cookies, URLs, response JSON, popup messages, or logs.
- Bearer integration/API-client behavior remains compatible.
- Every production change follows a witnessed red-green test cycle.

---

### Task 1: Typed token and cookie primitives

**Files:**
- Create: `models/authSession.go`
- Create: `utils/authCookies.go`
- Modify: `utils/jwt.go`
- Modify: `utils/tenantJwt.go`
- Test: `utils/security_test.go`
- Test: `utils/auth_cookies_test.go`

**Interfaces:**
- Produces `TokenTypeAccess`, `TokenTypeRefresh`, `TokenScopeTenant`, `TokenScopeProject`.
- Produces token claims containing `token_type`, `scope`, `aud`, `jti`, and `family_id`.
- Produces `SetAuthCookies(c, scope, tokens)`, `ClearAuthCookies(c, scope)`, and `AuthCookieName(scope, kind)`.

- [ ] Write failing table tests proving access/refresh type separation, tenant/project scope rejection, production `__Host-` names/flags, development names, seven-day refresh expiry, access Max-Age bounds, and matching clear attributes.
- [ ] Run `GOCACHE=/private/tmp/autotable-go-build-cache go test ./utils -run 'Test(AuthCookie|TypedToken)' -count=1` and confirm failures identify missing typed claims/cookie helpers.
- [ ] Add explicit typed claims to both token generators and implement environment-driven cookie helpers:

```go
type TokenMetadata struct { TokenType, Scope, Audience, TokenID, FamilyID string }
func SetAuthCookies(c *fiber.Ctx, scope string, tokens AuthTokenPair)
func ClearAuthCookies(c *fiber.Ctx, scope string)
```

- [ ] Run the targeted tests and existing `utils` tests until green.

### Task 2: Persisted refresh families and atomic rotation

**Files:**
- Create: `repositories/auth_session_repository.go`
- Create: `services/auth_session_service.go`
- Create: `services/auth_session_service_test.go`
- Modify: `configs/setup.go`

**Interfaces:**
- Consumes typed refresh `jti`, `family_id`, scope, subject, tenant, and project claims.
- Produces `CreateSession`, `RotateRefreshToken`, `RevokeFamily`, and `RevokeScopeForUser`.
- Returns `ErrRefreshConflict` for previous-token grace use and `ErrRefreshReuse` for reuse requiring family revocation.

- [ ] Write failing service tests with a deterministic in-memory repository proving hash-only storage, atomic current-JTI rotation, retryable previous-token conflict, reuse revocation, logout revocation, and expiry.
- [ ] Run `GOCACHE=/private/tmp/autotable-go-build-cache go test ./services -run TestAuthSession -count=1` and confirm expected failures.
- [ ] Implement SHA-256 JTI hashing, Mongo atomic compare-and-update, a short previous-token grace window, family revocation, and TTL index creation:

```go
type AuthSession struct { FamilyID, CurrentJTIHash, PreviousJTIHash, Scope, UserID string; PreviousValidUntil, ExpiresAt time.Time; RevokedAt *time.Time }
func (s *AuthSessionService) Rotate(ctx context.Context, claims RefreshClaims) (*TokenPair, error)
```

- [ ] Run service/repository tests until green.

### Task 3: Credential-source resolution and exact Origin policy

**Files:**
- Create: `middlewares/auth_source.go`
- Create: `middlewares/origin.go`
- Modify: `middlewares/authenticate.go`
- Modify: `middlewares/tenantAuthenticate.go`
- Modify: `main.go`
- Test: `middlewares/middlewares_test.go`
- Test: `main_test.go`

**Interfaces:**
- Produces `AuthSourceBearer`, `AuthSourceCookie`, `ResolveProjectCredential`, `ResolveTenantCredential`, and exact normalized trusted-origin checks.
- Stores selected source in `c.Locals("authSource")`.

- [ ] Write failing tests for Bearer precedence, invalid Bearer terminal failure, cookie fallback only when Authorization is absent, cookie unsafe request missing/null/look-alike/wrong-port Origin rejection, trusted exact Origin acceptance, and Bearer calls without Origin.
- [ ] Run targeted middleware/main tests and observe failures.
- [ ] Implement shared resolution and Origin middleware; enable credentialed CORS, `Vary: Origin`, `X-AutoTable-Client`, and explicit whitelist matching:

```go
type AuthSource string
const (AuthSourceBearer AuthSource = "bearer"; AuthSourceCookie AuthSource = "cookie")
func RequireTrustedCookieOrigin(allowed map[string]struct{}) fiber.Handler
```

- [ ] Replace direct Authorization reads in required and optional project/tenant middleware and run tests until green.

### Task 4: Cookie login, refresh, logout, session, and project switch endpoints

**Files:**
- Modify: `controllers/authController.go`
- Modify: `controllers/tenantAuthController.go`
- Modify: `controllers/projectController.go`
- Modify: `routes/authRoutes.go`
- Modify: `routes/tenantAuthRoutes.go`
- Test: `controllers/error_paths_test.go`
- Create: `controllers/cookie_auth_test.go`

**Interfaces:**
- Produces project `/auth/session`, `/auth/refresh`, `/auth/logout` and tenant `/tenant/auth/session`, `/tenant/auth/refresh`, `/tenant/auth/logout` behavior.
- Login/register/project switch set cookie pairs and omit tokens from JSON.
- Session shape includes `authenticated`, `scope`, safe user, roles, tenant, and optional project.

- [ ] Write failing handler tests for cookies and flags, token omission, anonymous session `200`, no-store headers, typed refresh rejection, `409` rotation conflict, reuse revocation, cookie clearing, and project-switch replacement.
- [ ] Run targeted controller tests and observe contract failures.
- [ ] Wire auth-session issuance/rotation/revocation and return safe session data:

```go
c.Set("Cache-Control", "no-store")
c.Set("Pragma", "no-cache")
return c.JSON(fiber.Map{"authenticated": true, "scope": "project", "user": safeUser, "roles": roles, "tenant": tenant, "project": project})
```

- [ ] Protect cookie actions with Origin policy, retain explicit Bearer request support, and run controller/route tests until green.

### Task 5: Google OAuth atomic state, PKCE, and token-free redirect

**Files:**
- Modify: `controllers/authController.go`
- Modify: `utils/googleOAuth.go`
- Test: `controllers/auth_google_test.go`

**Interfaces:**
- Redis state stores tenant/project context and PKCE verifier for five minutes.
- Callback atomically consumes state, supplies `oauth2.VerifierOption`, sets project cookies, and redirects with status only.

- [ ] Write failing tests for missing/expired/altered/replayed state, atomic single-use under concurrency, tenant/project binding, PKCE challenge/verifier, and absence of tokens/user JSON/OAuth code from redirects and logs.
- [ ] Run targeted Google auth tests and confirm failures.
- [ ] Generate `S256` PKCE, store verifier with state, atomically consume with Redis `GETDEL` semantics, exchange with verifier, issue session cookies, and redirect to `/auth/google/callback?status=success`.
- [ ] Run Google auth tests until green.

### Task 6: Authenticated WebSocket handshake

**Files:**
- Modify: `main.go`
- Modify: `ws/hub.go`
- Modify: `ws/hub_test.go`
- Modify: `ws/redis_integration_test.go`

**Interfaces:**
- Upgrade middleware resolves Bearer/cookie source, exact Origin, signed tenant/project claims, and requested slug context before upgrade.
- `HandleWS` consumes verified tenant/project locals rather than trusting query IDs alone.

- [ ] Write failing real-upgrade tests for trusted cookie handshake, missing/untrusted Origin rejection, invalid Bearer no-cookie-fallback, and signed cross-project rejection.
- [ ] Run targeted `ws` and main tests and observe unauthenticated upgrade failures.
- [ ] Implement upgrade authentication and pass verified context through Fiber locals into the websocket connection.
- [ ] Run WebSocket tests until green.

### Task 7: React-template credentialed transport and session state

**Files:**
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/api/session.ts`
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/context/Session.context.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/api/axiosClient.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/api/auth.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/navigation/PrivateRoutes.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/navigation/routes.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/hooks/useAuth.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/App.tsx`
- Test: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/api/index.test.ts`
- Create: `/Users/osmansamilerdogan/Desktop/react-template/src/utils/api/session.test.ts`

**Interfaces:**
- Axios always uses `withCredentials` and `X-AutoTable-Client` on unsafe requests.
- Session provider exposes `status: "loading" | "authenticated" | "anonymous"`, safe session data, and `refreshSession`.
- One shared refresh promise retries `401` once; `403` never refreshes.

- [ ] Write failing Vitest cases for credentialed requests without Authorization/local-storage access, one single-flight refresh, one retry, excluded auth endpoints, failed refresh, and no refresh on `403`.
- [ ] Run `yarn test src/utils/api/index.test.ts src/utils/api/session.test.ts` and confirm expected failures.
- [ ] Implement session transport/provider, async route guarding, legacy credential cleanup, and stale-project `403` handling.
- [ ] Run targeted tests and TypeScript checks until green.

### Task 8: React-template login, Google, roles, and WebSocket migration

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/pages/Login.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/pages/GoogleCallback.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/pages/googleCallbackAuth.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/pages/AuditLogs.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/hooks/useFilteredRoutes.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/hooks/useWebSocket.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/components/header/PageSelector.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/react-template/src/hooks/useSidebarNavigation.ts`
- Test: corresponding existing `*.test.ts` files and new auth-flow tests.

**Interfaces:**
- Google popup messages contain only status.
- Roles come from session context, and WebSocket URLs contain routing context but no credential.

- [ ] Write failing tests proving no token storage/URL/popup data, session-derived roles, cookie WebSocket URL behavior, and logout through the backend.
- [ ] Run the focused frontend tests and confirm failures.
- [ ] Remove browser token parsing/storage and migrate all consumers to session state.
- [ ] Run focused tests until green.

### Task 9: TenantPanel cookie transport and project-switch migration

**Files:**
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/axiosClient.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/auth.ts`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/pages/ProjectsPage.tsx`
- Modify: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/hooks/useWebSocket.ts`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/session.ts`
- Create: `/Users/osmansamilerdogan/Desktop/tenantPanel/src/utils/api/session.test.ts`

**Interfaces:**
- TenantPanel uses tenant cookies for admin APIs and receives project cookies through project switch without reading token JSON.
- Opening `panel.autoapi.org/t/:tenant/p/:project` transfers no credential in URL or JavaScript.

- [ ] Write failing tests for credentialed tenant requests, tenant refresh single-flight, token-free project switch, separate tenant/project logout behavior, and cookie WebSocket use.
- [ ] Run focused tenantPanel tests and confirm failures.
- [ ] Implement tenant session bootstrap/transport and remove credential storage/forwarding.
- [ ] Run focused tests and TypeScript checks until green.

### Task 10: Proxy hardening and complete verification

**Files:**
- Modify: `nginx.conf`
- Modify: `nginx-init.conf`
- Modify: `main_test.go`
- Test: all backend/frontend suites.

**Interfaces:**
- Nginx preserves Origin/Set-Cookie, upgrades WebSockets, disables auth/session caching, and does not log Cookie/Authorization values.

- [ ] Write/extend configuration behavior tests for forwarded headers, no-cache routes, Set-Cookie preservation, and sensitive-value redaction.
- [ ] Update Nginx configuration with explicit proxy headers and no-cache rules without trusting client-supplied forwarding data inside the app.
- [ ] Run `GOCACHE=/private/tmp/autotable-go-build-cache go test ./...` with localhost test permission.
- [ ] Run `yarn test` and `yarn build` in both `react-template` and `tenantPanel`.
- [ ] Search production source and generated redirect construction for `localStorage.*jwt`, `localStorage.*refreshToken`, JavaScript `Cookies.set("jwt")`, token query parameters, and WebSocket token parameters; require zero browser credential occurrences.
- [ ] Run `git diff --check` in all three repositories and review only intended source/test/config changes.
