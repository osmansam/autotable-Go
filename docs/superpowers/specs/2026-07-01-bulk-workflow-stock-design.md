# Bulk Workflow Stock Adjustment Design

## Goal

Bulk create, update, and delete operations on orders must update product stock through the same `after_create`, `after_update`, and `after_delete` workflows used by single-record operations.

## Current State

The bulk service paths already contain per-record workflow calls:

- Bulk create runs `after_create` after `InsertMany` and after assigning inserted IDs.
- Bulk update loads the existing record, creates a before snapshot, merges the update, persists it, and runs `after_update` with both versions.
- Bulk delete loads the record before deletion and runs `after_delete` with the deleted record.

These calls are sequential and use the transaction session. The missing engineering work is to make this contract explicit, cover it with regression tests, preserve operation metadata, and support a correct stock workflow when an order's product changes.

## Trigger Semantics

Bulk operations reuse existing lifecycle trigger names. No `after_bulk_create`, `after_bulk_update`, or `after_bulk_delete` triggers will be added.

For every successful record:

- Bulk create invokes `after_create` with `record` set to the inserted order and `oldRecord` unset.
- Bulk update invokes `after_update` with `record` set to the complete updated order and `oldRecord` set to the complete pre-update order.
- Bulk delete invokes `after_delete` with both `record` and `oldRecord` set to the deleted order snapshot.

Failed bulk update or delete items do not run after-workflows. Workflows execute sequentially to avoid unrestricted concurrent updates to the same product.

## Operation Context

The workflow execution payload will carry the originating mutation operation:

- `create`
- `update`
- `delete`
- `bulk_create`
- `bulk_update`
- `bulk_delete`

The value will be available as `{{operation}}` and as the condition field `operation`. It will also be copied into workflow-step outbox payloads so transactional, hybrid, and outbox execution observe the same context.

This metadata does not change trigger matching. It lets a workflow distinguish bulk and single-record origins without duplicating the workflow.

## Transaction and Error Behavior

The bulk database mutation, transactional workflow steps, stock updates, audit records, and outbox inserts remain in the same MongoDB transaction.

If a transactional workflow returns an error and the workflow is configured to stop on error, the service returns an error and MongoDB rolls back the entire transaction. Existing per-item validation and not-found failures in bulk update/delete remain item-level failures; no workflow runs for those failed items.

Outbox steps are only enqueued in the transaction. Their later execution preserves the operation context but cannot roll back the already committed mutation, matching existing outbox semantics.

## Stock Rules

The existing workflow definitions remain valid for these cases:

- Create: decrement the new product by `record.quantity`.
- Quantity-only update: increment stock by `oldRecord.quantity - record.quantity`.
- Delete: increment the deleted order's product by `record.quantity`.

An update that changes the product requires different behavior:

1. Increment the old product by `oldRecord.quantity`.
2. Decrement the new product by `record.quantity`.
3. Do not also apply the quantity-difference adjustment.

To express the third rule cleanly, the workflow condition engine will add `not_changed`, the inverse of `changed`. The quantity-difference workflow will require both `quantity changed` and `product not_changed`. Two additional conditional `update_record` steps will handle returning stock to the old product and removing stock from the new product.

## Testing

Tests will cover:

- Bulk create payloads use `after_create`, the inserted record, and `bulk_create`.
- Bulk update payloads use `after_update`, preserve distinct old/new records, and expose `bulk_update`.
- Bulk delete payloads use `after_delete`, preserve the deleted record, and expose `bulk_delete`.
- `changed` and `not_changed` conditions select the correct quantity-only and product-change paths.
- `{{operation}}` resolves in transactional execution and after an outbox payload round trip.
- A workflow error aborts sequential bulk workflow processing and is returned to the transaction callback.
- Existing single-record operation context remains `create`, `update`, or `delete`.

The full Go test suite and race-enabled test suite will be run after implementation.

## Non-Goals

- Aggregating stock deltas by product.
- Running per-record workflows concurrently.
- Adding array-based bulk workflow payloads.
- Changing current partial-success rules for validation and not-found errors.
- Enforcing non-negative stock; that is a separate business rule.
