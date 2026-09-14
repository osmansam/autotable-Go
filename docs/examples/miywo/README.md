# Miywo AutoTable containers

37 application containers plus the optional `spatial_ref_sys` reference table. Each file in `containers/` is a complete AutoTable CreateContainer request body.

## Create manually

Create a container using each file's `schemaName` and JSON definition. If your editor accepts only fields, paste that file's `fields` array and configure its routes/indexes separately. For API creation, send each complete file separately to `POST /api/v1/{tenantSlug}/{projectSlug}/container/` with your authenticated management session. `all-containers.json` can also be sent directly to the public development endpoint `POST /api/v1/{tenantSlug}/{projectSlug}/container/create-multiple`; see [bulk import instructions](bulk-import-instructions.md). Do not send this array to the single-container endpoint. `ALL-CONTAINERS.md` contains every definition for copy/paste.

Any creation order works because UUID references remain strings. `spatial_ref_sys` is optional for the application; creating it does not install PostGIS or its reference data. AutoTable's existing auth/role containers are separate; do not create another auth container from profiles. The source references a `users` table whose definition was not supplied; its identity mapping remains application work.

## Conversion details

- All 38 tables and all 476 source columns are retained. MongoDB also generates `_id`; use `_id` for AutoTable item URLs and keep source `id`/`user_id` for application relationships. UUIDs are strings, not MongoDB ObjectIds; foreign-key indexes improve lookup but do not enforce relationships or enable automatic population.
- Required fields remain required, including fields that had SQL defaults. Supply UUIDs, timestamps, booleans, counters, invite codes and expiry dates when creating records. SQL default expressions are preserved in `source-schema.json`; container fields do not execute them. AutoTable's own timestamp fields do not replace the source snake_case fields.
- Timestamp and time columns use strings to preserve time and timezone information; use consistent UTC RFC3339 timestamps and HH:MM:SS time values. Date-only columns use `date` with YYYY-MM-DD values.
- PostgreSQL enums use strings because the input omits their allowed values. Explicit text CHECK lists are retained as enumList, and supplied numeric bounds become validation tags. UUID syntax and timestamp string syntax require application validation.
- SQL ARRAY columns allergens/additives use stringArray, assuming their elements are text; the source omits element types.
- JSONB fields use JSON-encoded strings to preserve arbitrary object, array or scalar content without guessing its shape. Serialize on write and parse on read. If you want native nested editing, change a field to object or array after confirming its actual shape.
- PostGIS location fields use strings (for example existing WKT/EWKT values); no geographic queries or spatial indexes are configured.
- Numeric columns use float; this is not exact PostgreSQL decimal arithmetic. Integer/bigint columns use int; avoid values beyond JavaScript's safe integer range in browser/API JSON clients.
- Primary keys and explicit unique columns have unique indexes. Foreign keys, cascades, SQL triggers and database permissions are not reproduced.
- Enabled routes require authentication and the admin role. These are management definitions; owner/caregiver access and per-pet sharing must be implemented before exposing application routes. Fields named visibility/can_edit do not enforce permissions by themselves.

## Files

- [profiles](containers/profiles.container.json)
- [pets](containers/pets.container.json)
- [pet_members](containers/pet_members.container.json)
- [vet_clinics](containers/vet_clinics.container.json)
- [pet_vet_links](containers/pet_vet_links.container.json)
- [pet_documents](containers/pet_documents.container.json)
- [medical_conditions](containers/medical_conditions.container.json)
- [allergies](containers/allergies.container.json)
- [vaccination_events](containers/vaccination_events.container.json)
- [parasite_treatments](containers/parasite_treatments.container.json)
- [medications](containers/medications.container.json)
- [medication_schedules](containers/medication_schedules.container.json)
- [medication_logs](containers/medication_logs.container.json)
- [weight_logs](containers/weight_logs.container.json)
- [hydration_logs](containers/hydration_logs.container.json)
- [vet_appointments](containers/vet_appointments.container.json)
- [food_products](containers/food_products.container.json)
- [food_scans](containers/food_scans.container.json)
- [feeding_plans](containers/feeding_plans.container.json)
- [food_inventory](containers/food_inventory.container.json)
- [walk_sessions](containers/walk_sessions.container.json)
- [walk_events](containers/walk_events.container.json)
- [litter_box_logs](containers/litter_box_logs.container.json)
- [timeline_entries](containers/timeline_entries.container.json)
- [lost_pet_alerts](containers/lost_pet_alerts.container.json)
- [lost_pet_sightings](containers/lost_pet_sightings.container.json)
- [expenses](containers/expenses.container.json)
- [insurance_policies](containers/insurance_policies.container.json)
- [user_notification_settings](containers/user_notification_settings.container.json)
- [ai_pet_reports](containers/ai_pet_reports.container.json)
- [spatial_ref_sys](containers/spatial_ref_sys.container.json)
- [subscriptions](containers/subscriptions.container.json)
- [user_devices](containers/user_devices.container.json)
- [scheduled_notifications](containers/scheduled_notifications.container.json)
- [pet_invites](containers/pet_invites.container.json)
- [care_events](containers/care_events.container.json)
- [feeding_logs](containers/feeding_logs.container.json)
- [ai_usage](containers/ai_usage.container.json)
