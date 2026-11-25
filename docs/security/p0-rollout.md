# P0 hardening rollout notes

This branch is not deployed. Merge/release only after staging verification and an operator-controlled release window. Do not deploy intermediate commits whose Docker builder predates the Go upgrade.

## Signing keys

Use `go run ./cmd/autotable-secrets` in a trusted local environment to generate two independent 32-byte keys using `crypto/rand`. The command prints dotenv assignments and **does not change files, repository secrets, signing keys, sessions, or production data**. Keep its output out of chat/CI logs and store keys in the existing secret manager. Do not check generated keys into Git.

Startup requires JWT_SECRET and TENANT_JWT_SECRET to encode 32–128 random bytes as hex or standard/URL base64 (padded or raw), with no surrounding whitespace. Repetitive material is rejected. If legacy CONTAINER_JWT_SECRET is configured it must meet the same rules and be distinct. Unset it if unused; it is not an active signing domain in this source. All configured JWT keys must decode to distinct material, even across different encodings.

The application continues to sign with the **literal environment string**, without decoding it into a new HMAC key. Thus existing strong keys of an accepted encoding retain token compatibility. Size/encoding/repetition validation cannot prove cryptographic provenance of externally supplied strings: generate them cryptographically, never handcraft them. There is no weak-key override.

If existing keys fail validation, replace them before the release. Changing a signing secret invalidates outstanding access and refresh tokens in that domain: plan customer reauthentication and update every replica atomically. This work does not revoke or modify stored production sessions. A rollback must retain a coordinated key configuration; restoring old compromised keys is not a safe rollback strategy.

## Browser integrations

Regression coverage includes real Swagger HTML, trusted-origin credentialed CORS/preflight, and a WebSocket 101 handshake/message. No global CSP/frame denial/cross-origin resource isolation is imposed because customer embeds and Swagger use inline/CDN content. Configure document-specific CSP and HSTS at the trusted TLS/frontend ingress after checking customer integrations. API serves plain HTTP behind a proxy; forwarding headers are not used to authorize diagnostics.

## Recovery

HTTP panic responses are generic 500s; panic values are omitted from logs. Durable outbox task panics become retryable errors, not successful acknowledgements. Localization panic leaves its claim for existing expiry/recovery. Essential worker panic cancels work, shuts down HTTP with a deadline and exits nonzero for the process supervisor to restart. Optional cache fills and websocket clients recover locally. Fatal runtime errors/OOM are not recoverable. External side effects still require their existing idempotency protections.

## Metrics and profiling

Metrics are disabled (404) until OBSERVABILITY_TOKEN contains an independent cryptographically generated key meeting the same format/strength floor. When enabled, `/metrics` requires `Authorization: Bearer <token>`; missing/wrong credentials return 401. Do not reuse JWT, encryption or service credentials. Requests/forwarded loopback headers, cookies and query strings do not bypass authorization. Keep the diagnostics credential on trusted networks/TLS only; unauthenticated public health/readiness routes remain available.

Create a separate diagnostics key in your secret manager (for example with `openssl rand -hex 32`), configure OBSERVABILITY_TOKEN on the app, and mount a readable `observability/secrets/autotable-observability-token` containing only that literal token for the example Prometheus stack. This directory is ignored by Git. Prometheus now reads `/etc/prometheus/secrets/autotable-observability-token` for bearer authentication. No real credential file is supplied. Missing token/mount configuration will intentionally stop scrapes until configured. Production workflow forwards the optional OBSERVABILITY_TOKEN repository secret without printing it.

Pprof additionally requires `ENABLE_PPROF=true` and a non-production NODE_ENV. It is disabled by default in development and always disabled in production. Do not run long profiling sessions over public ingress. Auth covers every pprof subpath.
