-- Queries for import_batches (this change's minimal shape), transactions
-- and classifications (change 2.5, migration 007).
--
-- No statement here carries a `WHERE org_id = ...` predicate -- the same
-- reason as every other file in this directory: each table's FORCE'd
-- row-level-security policy already restricts every statement to
-- app_current_org(), and db.InTx sets that context before any of these run.
-- INSERT is the exception, as it is everywhere else here: org_id is bound
-- explicitly because the row does not exist yet for a policy to restrict.
--
-- InsertClassification never sets superseded_by -- a classification is
-- always born live -- and SupersedeClassification never sets anything but
-- it: migration 007 revokes table-level UPDATE and grants back only that one
-- column (design D4), so a query naming another would not even compile
-- against that grant. Its own comment explains why an ordinary insert then
-- an ordinary update is correct here: the deferred constraint trigger is
-- what makes the momentary two-live-rows state between them safe, not a
-- single combined statement.

-- name: InsertTransaction :one
-- direction is not among the columns: migration 007 generates it from the sign
-- of amount_minor, and Postgres refuses an insert that names a generated
-- column at all. That is the point of generating it -- the amount and its
-- direction cannot be written into disagreement -- and it is why Transaction's
-- own Direction field is read-only.
INSERT INTO transactions (
    org_id, entity_id, account_id, batch_id, line_no, source_kind,
    document_ref, posting_no, booked_on, value_on,
    amount_minor, currency, fx_rate, fx_rate_on, base_amount_minor, base_currency,
    counterparty_raw, counterparty_key, description_raw, description_norm,
    normalize_version, regulated_code, bank_ref, dedup_hash
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10,
    $11, $12, $13, $14, $15, $16,
    $17, $18, $19, $20,
    $21, $22, $23, $24
)
RETURNING *;

-- name: UnclassifiedTransactions :many
-- A page of rows with no live classification, oldest booked_on first, for
-- the worker a later change writes. Self-advancing: once the worker inserts
-- a classification for a row in one page, that row's classification is live
-- and the next call no longer returns it -- there is no offset to track or
-- to get out of step with a concurrent insert.
SELECT t.*
FROM transactions t
LEFT JOIN classifications c
    ON c.transaction_id = t.id AND c.superseded_by IS NULL
WHERE c.id IS NULL
ORDER BY t.booked_on, t.id
LIMIT $1;

-- name: InsertClassification :one
-- A correction is an insert plus a pointer, never an update: this is the
-- insert half.
INSERT INTO classifications (
    org_id, transaction_id, category_id, engine_layer, confidence, evidence,
    taxonomy_version, ruleset_version, engine_version, normalize_version, decided_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
RETURNING *;

-- name: SupersedeClassification :execrows
-- The pointer half of design D4's "an insert plus a pointer". Column-scoped
-- to superseded_by alone -- see this file's header.
--
-- Called after InsertClassification, in the same db.InTx, as two ordinary
-- statements. Between them there is a moment with two live rows for this
-- transaction_id, which classifications_one_live_per_transaction (migration
-- 007) is a DEFERRABLE INITIALLY DEFERRED constraint trigger specifically so
-- that moment is safe: it is checked once, at commit, by which point this
-- statement has already run and settled the count back to one. A plain
-- unique index -- what migration 007 first shipped with -- checks
-- immediately and cannot express this; verified directly, not assumed,
-- against the live schema before it was corrected.
UPDATE classifications SET superseded_by = $2 WHERE id = $1;

-- name: CurrentClassification :one
SELECT * FROM classifications
WHERE transaction_id = $1 AND superseded_by IS NULL;

-- name: TransactionsForReport :many
-- One entity, one source kind (design D1: a report line is computed from one
-- kind and never both), a date range.
SELECT * FROM transactions
WHERE entity_id = $1 AND source_kind = $2 AND booked_on >= $3 AND booked_on <= $4
ORDER BY booked_on, id;

-- name: CountTransactionsForBatch :one
-- add-dedup's GetDedupSummary (change 2.6): how many of a batch's parsed
-- rows actually became transactions, alongside dedup_skips' own counts.
SELECT count(*) FROM transactions WHERE batch_id = $1;
