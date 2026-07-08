# Bulk Operation Limit 3000 Design

## Goal

Allow bulk create, update, and delete requests containing up to 3000 items.

## Design

Change the fallback constants for `MaxBulkWriteLimit`, `MaxBulkUpdateLimit`, and `MaxBulkDeleteLimit` from 1000 to 3000. Set the corresponding values in both `configs/development.json` and `configs/production.json` to 3000 so configured and fallback behavior agree.

The three limits remain separate and independently configurable. Existing request parsing and HTTP 400 behavior for requests above the configured limits remain unchanged.

## Verification

Add explicit getter tests asserting that zero-valued configuration falls back to 3000 for all three operations. Run the configuration tests and then the full Go test suite.
