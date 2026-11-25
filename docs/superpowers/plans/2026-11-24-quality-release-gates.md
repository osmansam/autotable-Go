# Quality and release gate follow-up

**Goal:** Finish the user's accepted path from the P0 hardening toward an A grade, preserving API behavior and small verifiable commits.

**Architecture:** Keep the existing application and monoliths. Add reproducible local/CI tooling, exact lint baselines only for legacy externally visible error text and deprecations, weighted coverage floors from actual measurements, and executable release preflight checks. No deployment, production mutation, license addition or history rewrite.

**Starting point:** security/p0-hardening at 7c54e11 in the existing isolated worktree. Original checkout remains untouched.

## Tasks

- [x] Cleanup commit: remove backup source, relocate generated arithmetic examples to test-only fixtures (no production routes reference them), remove four unused WebSocket helpers, run tidy, test and verify no version upgrade.
- [x] Quality gate commit: resolve actionable Staticcheck warnings without architectural refactoring; preserve exact legacy messages/deprecations in a counted baseline that rejects new findings. Add tested coverage-profile validation with overall and critical-file thresholds, pinned compatible Staticcheck, tidy checks and CI artifacts. Use complete package coverage for cross-package request tests.
- [x] Onboarding commit: root README, safe .env.example, Makefile for setup/check/build/coverage/integration/image gates. Document required versus optional configuration, disposable databases, routes, JSON CORS/Redis selection, secrets and operator limits.
- [x] Release gate commit: reusable Docker build/non-root smoke and OS scan scripts wired to CI; explicit staging checks for image mounts, JWT configuration and trusted HTTPS ingress. Do not start unrelated OrbStack machines, access production, rotate keys or perform customer acceptance on behalf of operators.
- [x] Final verification/review: all Go tests/race, isolated Mongo integration, quality gate regression tests, workflow/shell validation, module/vulnerability checks, Linux build; report real results and remaining Docker/operator limits after each commit.

## Review focus

Fail closed on malformed/missing coverage and scanner/tool failures. New lint findings must not silently join the legacy baseline. CI and Make targets must use the same commands. .env.example must remain secret-free and trackable despite .env ignore rules. Release checks must not rewrite host mount permissions, generate real credentials into logs, or imply that static inspection proves runtime/customer readiness.

## Baseline

Complete race test suite PASS. Package-local statement coverage 53.0%; cross-package profile measurement pending. Staticcheck v0.8.2 requires Go 1.26 and is verified via the official module proxy. Trivy latest v0.76.2 requires Go 1.27, so use a verified release binary/container rather than upgrading application Go for a scanner. Docker engine remains stopped. No application dependency upgrades planned.

## Final status

Implementation and locally executable verification complete; evidence is in `docs/security/quality-followup.md`. Independent review found five important release-tooling issues and two small ordering/documentation issues; corrections were reproduced and regression-verified in separate commits. Actual Docker/image/OS execution remains blocked by the stopped engine. Real staging configuration, mount paths, HTTPS origins, key rotation and customer acceptance require operator inputs and are not claimed as passed. No merge, push, deployment, production-data operation, license addition or history rewrite.
