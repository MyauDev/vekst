-- name: AppliedMigrationVersion :one
-- The highest version goose has recorded as applied. /readyz compares this
-- against the binary's required version, derived from its embedded
-- migrations (design D6/Q5), and fails naming both when the schema is
-- behind.
SELECT version_id
FROM goose_db_version
ORDER BY id DESC
LIMIT 1;
