-- Queries for import_batches (change 2.5's migration 007, widened by 2.1's
-- migration 008).
--
-- No statement here carries a `WHERE org_id = ...` predicate, for the same
-- reason none do in tenancy.sql: the table's FORCE'd row-level-security policy
-- already restricts every statement to app_current_org(), and db.InTx sets
-- that context before any of these run. A hand-written predicate would return
-- the right rows whether or not the policy existed, which is exactly the
-- silent failure mode the policy's absence is supposed to be loud about.
-- INSERT is the one exception -- org_id is bound explicitly there, the same
-- way InsertEntity and InsertAccount bind it in tenancy.sql, because the row
-- does not exist yet for a policy to restrict and WITH CHECK is what verifies
-- the value against app_current_org().
--
-- RecordUploadMeasurement and SetImportBatchStatus never list source_kind
-- among the columns they set. source_kind is chosen at creation and immutable
-- (design D4): migration 008 revokes table-level UPDATE and grants it back
-- column by column, and source_kind is not one of the columns granted back.
-- A query that named it here would not even compile against that grant.

-- name: InsertImportBatch :one
-- Reserves a batch in awaiting_upload. status is a literal, not a parameter:
-- this is the one statement that creates a row, and the row's first state is
-- not the caller's to choose. id is supplied explicitly rather than left to
-- the column's own DEFAULT gen_random_uuid(): the object key is
-- org/<org_id>/batch/<batch_id> (blob.Key), built from identifiers only, so
-- the id has to exist before this statement runs, not come back from it.
INSERT INTO import_batches (
    org_id, id, entity_id, source_kind, status,
    uploaded_by, file_name, declared_bytes, declared_type,
    file_key, upload_expires_at, import_profile_id
) VALUES (
    $1, $2, $3, $4, 'awaiting_upload',
    $5, $6, $7, $8,
    $9, $10, $11
)
RETURNING id, org_id, entity_id, source_kind, status, uploaded_by,
          file_name, declared_bytes, declared_type, file_key,
          file_sha256, byte_length, content_type, row_count, failure_code,
          upload_expires_at, created_at, updated_at,
          import_profile_id, resolved_parameters;

-- name: GetImportBatch :one
-- id alone, with no org_id beside it -- entities is keyed (org_id, id) and the
-- policy supplies the org_id half, so this cannot resolve outside the caller's
-- own organisation however the identifier was obtained (same shape as
-- tenancy.sql's GetEntity).
SELECT id, org_id, entity_id, source_kind, status, uploaded_by,
       file_name, declared_bytes, declared_type, file_key,
       file_sha256, byte_length, content_type, row_count, failure_code,
       upload_expires_at, created_at, updated_at,
       import_profile_id, resolved_parameters
FROM import_batches
WHERE id = $1;

-- name: ListImportBatches :many
-- One entity's batches, most recent first -- what the Imports screen
-- (add-web-experience §6) lists. No pagination: Demo scale is a handful of
-- imports per organisation, and ListEntities/ListAccounts set the same
-- precedent of adding it only once a real page needs it.
SELECT id, org_id, entity_id, source_kind, status, uploaded_by,
       file_name, declared_bytes, declared_type, file_key,
       file_sha256, byte_length, content_type, row_count, failure_code,
       upload_expires_at, created_at, updated_at,
       import_profile_id, resolved_parameters
FROM import_batches
WHERE entity_id = $1
ORDER BY created_at DESC;

-- name: SetResolvedParameters :execrows
-- add-import-profiles task 5.3: what a batch was actually parsed with,
-- recorded separately from the profile it names -- a profile may be edited
-- afterward, and this is what keeps an old batch's numbers explainable
-- regardless (design D1's stated cost). Written once, by the same job that
-- parses the batch; nothing here stops a second write, because re-parsing
-- is not a thing this pipeline does outside a fresh batch.
UPDATE import_batches
SET resolved_parameters = $2, updated_at = now()
WHERE id = $1;

-- name: RecordUploadMeasurement :execrows
-- The measurement job's success write (design D2): what core actually
-- measured after reading the object back, and the only place status becomes
-- 'uploaded'. A batch this job could not measure -- a missing object, one
-- over the size limit -- goes through SetImportBatchStatus instead, with
-- these three columns left NULL, which import_batches_measured_past_upload
-- permits only for a 'failed' row.
UPDATE import_batches
SET file_sha256 = $2, byte_length = $3, content_type = $4,
    status = 'uploaded', updated_at = now()
WHERE id = $1;

-- name: SetImportBatchStatus :execrows
-- The generic transition: a status and, for a failure, the code that explains
-- it. failure_code is NULL for every non-failure transition -- a code is
-- never invented for a row that did not fail, and clearing a stale one is
-- what lets a later stage's own failure be the one a customer sees.
UPDATE import_batches
SET status = $2, failure_code = $3, updated_at = now()
WHERE id = $1;

-- name: InsertRawRows :execrows
-- Bulk insert, not one round trip per line: a real statement is hundreds of
-- rows. This is a `SELECT ... FROM unnest(...)` rather than sqlc's
-- `:copyfrom` (COPY FROM) deliberately -- verified on PostgreSQL 16 that
-- COPY FROM refuses outright against a row-level-security table, forced or
-- not: "ERROR: COPY FROM not supported with row-level security" (SQLSTATE
-- 0A000), with no tenant context to blame. An ordinary INSERT, even one
-- driven by unnest(), goes through the same WITH CHECK every other write
-- here does.
--
-- Two single-argument unnest() calls joined by WITH ORDINALITY, not the
-- two-argument unnest($1, $2) form that zips a pair of arrays directly:
-- sqlc's analyzer cannot resolve the two-argument form's parameter types
-- ("function unnest(unknown, unknown) does not exist") even though
-- PostgreSQL itself accepts it fine. This is the shape sqlc can see through.
INSERT INTO raw_rows (org_id, batch_id, line_no, payload_jsonb)
SELECT $1, $2, ln.line_no, pl.payload::jsonb
FROM unnest($3::integer[]) WITH ORDINALITY AS ln(line_no, ord)
JOIN unnest($4::text[])    WITH ORDINALITY AS pl(payload, ord) USING (ord);

-- name: AbandonExpiredBatch :one
-- Abandons only if the row is still awaiting_upload, in the same statement
-- that checks it -- so a late expiry job racing a real upload cannot win:
-- exactly one UPDATE can match the predicate and return a row. :one rather
-- than :execrows because affecting zero rows is not an error here the way it
-- is for RecordUploadMeasurement and SetImportBatchStatus -- it is the
-- expected outcome when the upload already arrived, and the caller reads
-- pgx.ErrNoRows as "nothing to do", not as db.ErrNoRowsAffected.
UPDATE import_batches
SET status = 'abandoned', updated_at = now()
WHERE id = $1 AND status = 'awaiting_upload'
RETURNING id;

-- name: FindImportedBatchAlreadyHoldingThisFile :one
-- D1 (add-dedup, change 2.6): looks first, so a genuine collision names the
-- earlier batch rather than surfacing a raw constraint violation. The
-- guarantee against two concurrent uploads winning is import_batches_file_once
-- (migration 012) -- this SELECT narrows the race window, it does not close
-- it.
SELECT id FROM import_batches WHERE file_sha256 = $1 AND status = 'imported';
