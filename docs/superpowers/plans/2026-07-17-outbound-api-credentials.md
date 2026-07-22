# Outbound API Credentials Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add encrypted tenant/project-scoped outbound API credentials for workflow `call_api` steps without breaking existing configurations.

**Architecture:** Store outbound API secrets in a separate collection from inbound integration credentials. Workflow steps reference a credential id; runtime loads the scoped credential, validates lifecycle and allowed host, decrypts the secret, and injects protected auth headers into the outbound HTTP request. HTTP execution rejects redirect targets outside the originally validated host policy and prevents user headers from overwriting injected authentication headers.

**Tech Stack:** Go, MongoDB, Fiber, AES-256-GCM, existing workflow service and repository patterns.

## Global Constraints

- Existing `call_api` steps without credentials remain backward compatible.
- Secrets and `Authorization` headers must not appear in logs, traces, errors, or workflow execution results.
- Domain checks must use parsed hostnames.
- Redirects must not bypass allowed-domain restrictions.
- User-provided headers cannot overwrite injected auth headers.
- Credential access must be tenant/project scoped.

---

### Task 1: Crypto And Model

**Files:**
- Create: `models/externalAPICredential.go`
- Create: `utils/secret_crypto.go`
- Test: `utils/secret_crypto_test.go`

**Interfaces:**
- Produces: `models.ExternalAPICredential`, `utils.EncryptExternalSecret`, `utils.DecryptExternalSecret`, `utils.ValidateExternalAPIEncryptionKey`.

- [x] Write failing tests for AES-256-GCM round trip, invalid key rejection, and tamper rejection.
- [x] Implement model and crypto helpers.
- [x] Run focused tests.

### Task 2: HTTP Execution Security

**Files:**
- Modify: `utils/executeApiRequest.go`
- Test: `utils/additional_helpers_test.go`

**Interfaces:**
- Produces: `utils.ExecuteApiRequestWithStatusAndHeaders(ctx, method, url, body, userHeaders, protectedHeaders, hostAllowed)`.

- [x] Write failing tests for protected authorization override and redirect domain bypass.
- [x] Implement header merging and redirect host validation.
- [x] Run focused tests.

### Task 3: Credential Repository And Controllers

**Files:**
- Modify: `repositories/dynamic_repository.go`
- Create: `controllers/externalAPICredentialController.go`
- Modify: `routes/integrationRoutes.go`
- Test: `controllers/external_api_credential_test.go`

**Interfaces:**
- Produces CRUD-style create/list/revoke endpoints under authenticated integration routes.

- [x] Write failing tests for create/list redaction and tenant/project isolation.
- [x] Implement repository methods and controllers.
- [x] Run focused tests.

### Task 4: Workflow Integration

**Files:**
- Modify: `services/dynamic_workflow.go`
- Modify: `services/dynamic_workflow_validation.go`
- Test: `services/helpers_test.go`

**Interfaces:**
- Consumes: repository credential lookup and HTTP utility with protected headers.

- [x] Write failing tests for bearer token injection, wrong-domain rejection, revoked/expired rejection, and attempted authorization override.
- [x] Implement `credentialId` handling in `workflowCallAPI`.
- [x] Run focused tests and full Go test suite.
