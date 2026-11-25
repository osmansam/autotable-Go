# AutoTable Go

AutoTable is a multi-tenant API built with Go, Fiber v2, MongoDB and Redis. It serves dynamic schemas/records/workflows, management and application authentication, collaboration, uploads, localization and project WebSocket events.

## Local prerequisites

- **Go 1.26.11** (the module pins this toolchain), Python 3, Make and a C toolchain for Go race tests. Go may download the pinned toolchain automatically.
- A **dedicated local MongoDB replica set** for transactions and a local Redis instance. Never use production connection strings for development or integration tests.
- Docker is optional for unit tests and the native integration runner; it is required for production-image verification. `mongod` plus `mongo`/`mongosh` are required by the native runner.
- Network access for module downloads, pinned analysis tools and vulnerability databases.

Run commands from the repository root. The original monoliths remain in place; this hardening work does not change the service architecture.

## Configure and run

1. Create a private development file without overwriting an existing configuration:

   ```sh
   test ! -e .env && (umask 077; cp .env.example .env)
   ```

2. Generate independent JWT signing keys locally:

   ```sh
   make secrets
   ```

   Paste the two assignments into `.env` using your editor/secret manager. The command prints credentials locally but does not edit files or rotate sessions. Keep that output out of CI/chat logs. Empty, weak or reused keys prevent startup.

3. Configure your dedicated Mongo replica set. The sample expects `127.0.0.1:27017`, replica set `autotable-dev`, database `autotable_development`. Adjust all three to the local instance you actually created. Confirm its replica-set health before starting the API. Configure Redis and browser origins in `configs/development.json`; defaults use localhost Redis port 6379 and allow browser origins `http://localhost:3001` and `http://localhost:3005`.

4. Start the API:

   ```sh
   make run
   ```

   The app loads `.env` via godotenv, validates signing/diagnostics configuration, then initializes database connections/indexes and workers. It listens on `PORT_NUMBER` (4000 in the example). It writes temporary upload files to `./temp` and logs to the configured `LOG_FILE`; both directories must be writable by the running user. The JSON `app.port` does not override `PORT_NUMBER`.

Useful local endpoints:

| Endpoint | Behavior |
|---|---|
| `/health` | Public process liveness |
| `/ready` | Public 200/503 based on worker context and Mongo/Redis readiness |
| `/api/swagger`, `/api/swagger.json`, `/api/schemas` | Development documentation; disabled with `NODE_ENV=production` |
| `/metrics` | Disabled until a dedicated `OBSERVABILITY_TOKEN` is configured; then requires a bearer header |
| `/debug/pprof/` | Requires diagnostics auth plus `ENABLE_PPROF=true`; always disabled in production |
| `/api/v1/:tenantSlug/:projectSlug/ws` | Project-cookie authentication and trusted origin; subscription context is bound to the authenticated project |

## Configuration sources

Environment variables override `.env` values. `NODE_ENV` selects `configs/<NODE_ENV>.json`; normal values are `development` and `production`. JSON configuration includes Redis host/port/pools, Mongo pools, CORS whitelist, body/bulk limits, cache retention, audit and outbox settings. Missing JSON files fall back to defaults with a warning; verify the correct file is present rather than relying on that fallback.

| Environment group | Required/use |
|---|---|
| `JWT_SECRET`, `TENANT_JWT_SECRET` | Required, independent cryptographically generated keys. Use hex/base64 encoding of 32–128 random bytes. Literal environment strings remain the HMAC keys. |
| `MONGO_URI_BASE`, `COLLECTION_NAME`, `MONGO_URI_SUFFIX` | Explicit connection/database configuration. URI is their concatenation. An empty suffix gets retryWrites/w=majority defaults; set a replica-set suffix explicitly for local transactions. Do not rely on the missing-config test URI fallback. |
| `PORT_NUMBER`, `NODE_ENV` | Listener port and environment/config/cookie policy. Use production mode only for a deliberately configured environment. |
| `REDIS_PASSWORD` | Optional environment override of the selected JSON Redis password. Host/port come from JSON, not Redis URL environment variables. |
| `OBSERVABILITY_TOKEN`, `ENABLE_PPROF` | Optional diagnostics. Use an independent random token; never reuse current/previous service credentials or JWT/encryption keys. |
| `EXTERNAL_API_CREDENTIAL_KEY` | Base64-encoded 32-byte encryption key for external API credentials; validated when set, required by the production release workflow. Encryption-key rotation needs a separate plan for already encrypted data. |
| `CLOUD_*` | Cloudinary uploads; local temporary files do not replace the cloud service. |
| `GOOGLE_*`, `FRONTEND_URL` | Optional OAuth credentials/callback and frontend redirect. Register the callback with the provider. |
| `OPENAI_API_KEY`, `OPENAI_TRANSLATION_MODEL` | Required to use automatic translation; no API calls are made by the quality gates. |
| `COLLABORATION_*`, `INVITATION_*` | Feature flags plus invitation SMTP/TLS, HTTPS panel origin and encrypted delivery keyring. Keep flags false until fully configured. See [collaboration operations](docs/collaboration/operations.md). |
| `OZMISHOW_RATE_LIMIT_*` | Optional dedicated limiter credentials and protected Redis; see `docker-compose.ozmishow.yml`. Empty service secret disables this integration. |
| `LOG_LEVEL`, `LOG_FILE`, `OTEL_*` | Logging and tracing. Example disables tracing. Existing OTLP export uses cleartext gRPC: enable it only on a trusted collector network. |

The sample `.env.example` contains no working secrets and is safe to track. `.env`, generated reports/tools, uploaded files and the Prometheus bearer file are ignored. Do not execute `.env` as a shell script.

## Quality checks

```sh
make check          # lint/tidy/vet, weighted coverage + race tests, tooling tests, govulncheck
make test           # full race suite without coverage instrumentation
make coverage       # build/coverage.out and enforced file/overall floors
make lint           # pinned Staticcheck and exact legacy baseline
make test-tooling   # deployment and quality-tool regression tests; no deployment
make integration    # fresh task-owned loopback replica set; all integration-tagged tests, race enabled
make build          # Linux amd64 static binary at build/autotable (not a macOS executable)
```

Tools are versioned under `.tools/bin`; reports live under `build/`. `make clean` removes generated `build/` output only. `make integration` requires native Mongo binaries; `MONGOD_BIN`/`MONGO_SHELL_BIN` can override discovery. The runner checks process ownership and data path before replica-set initialization, creates disposable test databases, and cleans up its process. It never uses your application `.env` Mongo URI as the integration target. An optional isolated collaboration-only runner is also available at `scripts/test-collaboration-mongo-integration.sh`.

Coverage floors are in `coverage-thresholds.json`: 54% overall, 90% JWT parsing, 95% signing-key validation and 100% diagnostics/recovery/panic guard/WebSocket context. The gate weights statements, merges cross-package blocks and rejects malformed/missing records. Overall coverage is currently about 54%; this is a regression floor, not an 80% coverage claim. Add meaningful tests before raising floors.

`staticcheck-baseline.json` retains 79 legacy error-capitalization and 31 deprecated API findings to preserve customer messages and avoid unrelated API migrations. Every entry pins source, message and count; new/changed findings or stale entries fail. Bug/unused-code findings cannot be baselined. Three tests have narrow intentional nil-context annotations. No whole lint category is disabled. Use the [Staticcheck check reference](https://staticcheck.dev/docs/checks) when resolving findings; do not blindly refresh the baseline.

## Containers and release boundaries

The Dockerfile uses digest-pinned Go 1.26.11/Alpine bases, a static Linux binary and runtime UID/GID **10001:10001**. Uploaded files and logs need writable directories. Host bind mounts override image directory ownership: validate the actual staging mount permissions; do not use world-writable permissions or silently chown production data.

CI verifies quality, isolated Mongo journeys and the production image. The production workflow verifies pushes but deploys only on explicit manual dispatch. `make run`, `make check`, `make build` and integration tests do not deploy. Do not use `docker compose up` against an existing deployment merely to run checks.

JWT rotation invalidates that domain's access/refresh tokens. Coordinate replica configuration, customer reauthentication and rollback keys explicitly. This repository's checks do not rotate secrets, modify production sessions or certify customer acceptance.

HSTS belongs at trusted TLS ingress; document-specific CSP belongs at the frontend/ingress. The API does not impose a global CSP/frame/resource policy that would break Swagger/CDN scripts or customer embeds. Verify the actual HTTPS deployment and browser flows before release. See [P0 rollout requirements](docs/security/p0-rollout.md), [stage evidence](docs/security/p0-stage-report.md), and [quality follow-up evidence](docs/security/quality-followup.md). No release should treat unexecuted operator checks as passing.

## Executable release gates

```sh
make config-check   # read-only JWT/diagnostics/optional encryption-key validation; no key generation
make image-scan     # requires Docker: build, non-root probe, then OS HIGH/CRITICAL scan

STAGING_TEMP_DIR=/absolute/staging/uploads \
STAGING_LOG_DIR=/absolute/staging/logs make staging-mounts

STAGING_API_URL=https://api.staging.example.test \
STAGING_FRONTEND_URL=https://panel.staging.example.test make ingress-check
```

The default **build output tag** is `autotable:security-check`. `AUTOTABLE_CHECK_IMAGE` changes that output tag: image-check/image-scan rebuild the current checkout and replace the chosen local tag, so use an isolated test tag rather than an existing release tag. The standalone staging-mounts command checks that already-built tag without rebuilding it. `image-scan` uses digest-pinned Trivy v0.76.2 and an exported image archive without exposing the Docker socket or application credentials to the scanner. It fails on scanner errors, EOL distributions, HIGH/CRITICAL findings (including unfixed ones), and missing/invalid OS reports. The JSON report is `build/os-vulnerabilities.json`. A passing severity gate is not a claim that LOW/MEDIUM vulnerabilities do not exist. See the [official Trivy image CLI](https://trivy.dev/docs/latest/references/configuration/cli/trivy_image/) for scan semantics.

Mount checks require existing explicit absolute directories and inspect access as image UID/GID 10001. They do not create/chown directories, start the API or write customer files; real staging upload/log acceptance remains necessary. If Docker is stopped, image/scan/mount gates fail with a clear prerequisite error. They do not start unrelated machines or return a skipped success.

Ingress checks make read-only requests to the supplied HTTPS staging API `/health`, `/ready` and frontend document. System TLS certificate verification is enabled; redirects cannot leave the supplied HTTPS origin. Both origins must supply `nosniff` and HSTS with at least 180 days max-age. The frontend must send enforced CSP (report-only is insufficient) with an explicit-host `default-src` (no wildcard hosts or scheme-wide sources), `object-src 'none'` and `base-uri 'self'` or `'none'`. These are minimum checks, not a complete CSP/XSS audit; configure source lists/nonces and customer frame embedding at the frontend/ingress. The API's Swagger document does not receive a global CSP from these tools. No application HSTS/CSP behavior is changed.

`make release-check` combines quality checks, disposable Mongo integration tests, key-format validation and the image/mount/ingress gates. Supply the explicit staging variables above and the intended private environment configuration. It is a verification target: it never deploys or rotates keys. Do not run staging checks using production mount paths or secretly substitute production endpoints.

Before a manual release, operators must still verify customer login/refresh and domain isolation, WebSocket reconnect/events, uploads/logs on actual mounts, and any enabled OAuth/cloud/translation/invitation integrations. Plan any signing-key rotation and resulting reauthentication across every replica. These actions require real staging/customer context; automated minimum checks do not certify them.
