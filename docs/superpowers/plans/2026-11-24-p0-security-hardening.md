# P0 security hardening implementation plan

**Goal:** Implement the user's P0 scope in independently verifiable commits without deployment, production-data access, license changes, or unrelated refactoring.

**Architecture:** Retain Fiber v2, existing authentication domains, token wire claims, API routes, and durable worker semantics. Upgrade only verified security dependencies. Fail closed for secrets and diagnostics. Keep browser compatibility through conservative headers.

**Execution:** Inline in `/tmp/autotable-p0-hardening`, branch `security/p0-hardening`, starting at `9d29722`. The original checkout has unfinished cherry-pick metadata and is left untouched. The detailed user request authorizes implementation; no additional design approval is needed.

**Verification:** Before and after every dependency group run `go test ./... -count=1`. Add security regression tests before behavior changes and observe failure. Finish with vet, race tests, Linux build, deployment-script tests, vulnerability scan, and isolated integration tests if Docker is available. Never use the production environment or data.

## Stages

- [ ] Baseline and advisory inventory: official Go vulnerability DB, module proxy go.mod requirements, maintainer advisories; record exact targets and reachable vs module-only findings.
- [ ] Toolchain and transport dependencies: choose supported Go toolchain required by patched modules; verify tests before/after each group.
- [ ] JWT v5: migrate imports/registered claims, preserve string audience output and claim names, require expiry, enforce HS256, purpose/audience/scope boundaries; test authentication and cross-token rejection.
- [ ] Recovery: sanitize HTTP panic errors; recover task errors inside child goroutines so lease/result channels complete; supervise essential workers by canceling service and shutting down on panic rather than silently losing workers; protect websocket callbacks and writer cleanup.
- [ ] Headers: nosniff, referrer policy, safe permissions policy; preserve Swagger scripts, cross-origin APIs and websocket upgrades. Avoid global restrictive CSP/frame/resource policies that break customer embeds.
- [ ] Secrets: startup validation of distinct strong JWT secrets; crypto/rand generation utility and documented rotation. Cannot prove randomness of externally supplied strings; enforce encoded key size and reject weak material honestly.
- [ ] Diagnostics: default-deny metrics; opt-in non-production pprof behind dedicated bearer token; no forwarded-header/IP trust shortcut. Update Prometheus example and deployment notes.
- [ ] Container and CI: digest-pinned builder/runtime, non-root UID with writable uploads/logs, govulncheck pinned in CI; verify manifests/build where available.
- [ ] Artifact cleanup: untrack generated binaries and profiles, ignore future outputs, no history rewriting.
- [ ] Final review, verification and release-risk report.

## Review focus

1. Legacy string audiences, refresh rotation, and management/project separation survive JWT migration.
2. Child panic sends an error without deadlocking leases or marking failed jobs successful.
3. CORS preflight, Swagger content, websocket upgrades and frontend embedding remain usable.
4. Weak/equal keys and missing diagnostics credentials fail closed without exposing secret values.
5. Non-root bind mounts require host ownership; diagnostics scraping and secret rotation require explicit operator configuration.

## Baseline

`go1.24.10`; `go test ./... -count=1` passes in original and isolated worktree with local networking enabled. Sandbox-only runs fail because test listeners cannot bind; no application defect inferred. Govulncheck latest v1.10.0 requires Go 1.26; evaluating compatible scanner v1.1.4 for old toolchain inventory.
