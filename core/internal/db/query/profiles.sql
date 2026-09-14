-- Queries for import_profiles (change 2.4, migration 011).
--
-- No statement here carries a `WHERE org_id = ...` predicate -- the table's
-- FORCE'd row-level-security policy already restricts every statement to
-- app_current_org() (see tenancy.sql's header for the full reasoning).
-- INSERT is the exception, the same as everywhere else in this directory:
-- org_id is bound explicitly because the row does not exist yet for a
-- policy to restrict.

-- name: InsertImportProfile :one
INSERT INTO import_profiles (
    org_id, name, source_kind, column_map, charset, delimiter, decimal_sep, date_fmt
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING id, org_id, name, source_kind, column_map, charset, delimiter, decimal_sep, date_fmt,
          created_at, updated_at;

-- name: GetImportProfile :one
-- id alone, with no org_id beside it -- entities is keyed (org_id, id) and
-- the policy supplies the org_id half, the same shape as tenancy.sql's
-- GetEntity.
SELECT id, org_id, name, source_kind, column_map, charset, delimiter, decimal_sep, date_fmt,
       created_at, updated_at
FROM import_profiles
WHERE id = $1;

-- name: ListImportProfiles :many
SELECT id, org_id, name, source_kind, column_map, charset, delimiter, decimal_sep, date_fmt,
       created_at, updated_at
FROM import_profiles
ORDER BY name;

-- name: UpdateImportProfile :execrows
-- source_kind is deliberately absent from the SET list: a profile's kind is
-- chosen at creation, the same reasoning as import_batches' own
-- source_kind (design D4 in add-file-upload) -- a ledger profile silently
-- becoming a bank one would re-baseline every batch that already used it.
UPDATE import_profiles
SET name = $2, column_map = $3, charset = $4, delimiter = $5, decimal_sep = $6, date_fmt = $7,
    updated_at = now()
WHERE id = $1;

-- name: DeleteImportProfile :execrows
-- ON DELETE RESTRICT on import_batches.import_profile_id is what actually
-- enforces "a used profile cannot be deleted" (task 6.9); this statement is
-- refused by that foreign key, not by an application-level check first.
DELETE FROM import_profiles WHERE id = $1;
