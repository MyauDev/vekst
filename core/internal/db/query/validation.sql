-- Queries for import_validations (change 2.3, migration 010).
--
-- No statement here carries a `WHERE org_id = ...` predicate -- the same
-- reason as every other file in this directory: the table's FORCE'd
-- row-level-security policy already restricts every statement to
-- app_current_org(), and db.InTx sets that context before any of these run.
-- INSERT is the exception, the same way it is in ingest.sql: org_id is bound
-- explicitly because the row does not exist yet for a policy to restrict.
--
-- RecordOverride never lists outcome, row_count, error_count, warning_count,
-- balance_check_passed or report_jsonb among the columns it sets. Migration
-- 010 revokes table-level UPDATE and grants back only the three override
-- columns, so a query naming any of the six would not even compile against
-- that grant.

-- name: InsertValidation :one
INSERT INTO import_validations (
    org_id, batch_id, outcome, row_count, error_count, warning_count,
    balance_check_passed, report_jsonb
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING id, org_id, batch_id, outcome, row_count, error_count, warning_count,
          balance_check_passed, report_jsonb,
          overridden_by, override_reason, overridden_at, created_at;

-- name: GetValidationForBatch :one
-- batch_id alone, with no org_id beside it -- the row is keyed (org_id, id)
-- and the policy supplies the org_id half, the same shape as ingest.sql's
-- GetImportBatch.
SELECT id, org_id, batch_id, outcome, row_count, error_count, warning_count,
       balance_check_passed, report_jsonb,
       overridden_by, override_reason, overridden_at, created_at
FROM import_validations
WHERE batch_id = $1;

-- name: RecordOverride :execrows
-- Written once (design D4): nothing here reads the current row first to
-- decide whether to write, because there is no legitimate second write --
-- overridden_at IS NULL is not checked here because the CHECK constraint
-- import_validations_override_only_over_warnings and the handler's own
-- lookup (task 5.3) are what refuse a second override, not this statement's
-- WHERE clause.
UPDATE import_validations
SET overridden_by = $2, override_reason = $3, overridden_at = now()
WHERE batch_id = $1;

-- name: OverriddenBatchesForPeriod :many
-- Change 4.1 calls this and puts what it returns on the report (design D4).
-- A report that does not call it is a report that hides an override, so
-- 4.1's own task list carries a test that fails when the call is missing.
-- Unused here -- this change has no report to put it on yet.
SELECT b.id, v.override_reason, v.overridden_at, v.balance_check_passed
FROM import_validations v
JOIN import_batches b ON b.id = v.batch_id
JOIN transactions t   ON t.batch_id = b.id
WHERE v.overridden_at IS NOT NULL
  AND t.entity_id = $1 AND t.booked_on >= $2 AND t.booked_on < $3
GROUP BY b.id, v.override_reason, v.overridden_at, v.balance_check_passed;
