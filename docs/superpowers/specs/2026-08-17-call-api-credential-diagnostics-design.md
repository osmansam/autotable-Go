# Call API Credential Diagnostics Design

## Goal

Make outbound workflow credential failures diagnosable from structured application logs without exposing encryption keys, encrypted credential material, decrypted secrets, or authorization headers.

## Scope

The change applies only to credential preparation in `call_api` workflow steps. HTTP request and response logging, retry behavior, credential persistence, and public error messages remain unchanged.

## Design

Before decrypting an external API credential, the workflow service will classify the credential configuration into a small set of diagnostic stages:

- `key_missing`: `EXTERNAL_API_CREDENTIAL_KEY` is empty after trimming.
- `key_invalid`: the environment value is present but is not a base64-encoded 32-byte key.
- `encrypted_secret_missing`: the scoped credential was loaded but has no encrypted secret.
- `decrypt_failed`: the encrypted secret cannot be decoded or authenticated with the configured key.

For these failures, the service will emit a structured error log containing the tenant, project, schema, workflow, step, credential ID, HTTP method, destination host, and diagnostic stage. It may include non-sensitive booleans such as whether the key and encrypted secret are present. It must not include the environment value, encrypted secret, decrypted secret, user-supplied headers, protected headers, or full URL query parameters.

The existing caller-facing errors remain stable:

- Missing or invalid keys return `call_api credential encryption is not configured`.
- Missing or undecryptable credential ciphertext returns `call_api credential could not be decrypted`.

The outbox processor already logs its event context when dispatch fails. The new credential log complements that entry with the precise safe diagnostic stage; retry and status behavior do not change.

## Testing

Unit tests will exercise each diagnostic stage through a small classification helper. They will assert the expected stage and safe metadata only. Tests will also verify that diagnostic attributes never contain the raw encryption key, encrypted secret, decrypted secret, or authorization material.

Existing workflow and full Go test suites will be run after the focused tests pass.

## Operational Outcome

For the reported failure, logs will distinguish a genuinely missing environment variable from an invalid key format or an existing credential encrypted under another key. Operators can then correct deployment configuration or recreate the credential without guessing whether the PATCH method caused the failure.
