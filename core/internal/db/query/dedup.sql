-- Queries for dedup_skips, internal_transfers and internal_transfer_members
-- (change 2.6, migration 012).
--
-- No statement here carries a `WHERE org_id = ...` predicate -- the same
-- reason as every other file in this directory: each table's FORCE'd
-- row-level-security policy already restricts every statement to
-- app_current_org(). INSERT is the exception, as it is everywhere else
-- here: org_id is bound explicitly because the row does not exist yet for a
-- policy to restrict.
--
-- InsertInternalTransferPair writes the pair and both its members in one
-- statement: internal_transfer_members' own primary key,
-- (org_id, txn_id), is what refuses a transaction already spoken for, and a
-- pair whose sides are already taken must fail outright rather than
-- half-inserting one member and not the other.

-- name: InsertDedupSkip :one
-- A skip is written once and never updated (migration 012's own REVOKE) --
-- like import_validations' report, it is a record of what happened during
-- one persist.
INSERT INTO dedup_skips (
    org_id, batch_id, line_no, posting_no, level, dedup_hash,
    matched_transaction_id, matched_batch_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING id, org_id, batch_id, line_no, posting_no, level, dedup_hash,
          matched_transaction_id, matched_batch_id, created_at;

-- name: ListDedupSkipsForBatch :many
SELECT id, org_id, batch_id, line_no, posting_no, level, dedup_hash,
       matched_transaction_id, matched_batch_id, created_at
FROM dedup_skips
WHERE batch_id = $1
ORDER BY line_no, posting_no;

-- name: CountDedupSkipsForBatch :one
-- GetDedupSummary derives its counts from this table rather than from a
-- stored counter (design D3, task 3.5) -- filtered by level so the two D2
-- and D3 numbers in the summary come from one query each, not from one
-- query and a second pass in Go.
SELECT count(*) FROM dedup_skips WHERE batch_id = $1 AND level = $2;

-- name: FindTransactionByDedupHash :one
-- D3's cross-batch lookup. transactions_dedup_idx (migration 007, corrected
-- by this change's own task 0.4) is not unique, so this can return any one
-- of several matches under a true hash collision -- naming the most
-- recent, which is the one most useful for a customer checking "did I
-- already import this".
SELECT id, batch_id FROM transactions
WHERE dedup_hash = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: InsertInternalTransferPair :one
-- Both members in the same statement as the pair: a pair whose sides are
-- already taken fails here rather than half-inserting one side. See this
-- file's header.
WITH pair AS (
    INSERT INTO internal_transfers (org_id, out_txn_id, in_txn_id)
    VALUES ($1, $2, $3)
    RETURNING id, org_id, out_txn_id, in_txn_id, detected_at, dismissed_by, dismissed_at
)
INSERT INTO internal_transfer_members (org_id, txn_id, transfer_id, side)
SELECT $1, v.txn_id, pair.id, v.side
FROM pair, (VALUES ($2::uuid, 'out'), ($3::uuid, 'in')) AS v(txn_id, side)
RETURNING (SELECT id FROM pair), (SELECT out_txn_id FROM pair),
          (SELECT in_txn_id FROM pair), (SELECT detected_at FROM pair);

-- name: ListActiveInternalTransfers :many
-- Every undismissed pair for this entity, on either side -- what
-- ListInternalTransfers (task 5.1) returns, and what a later change's P&L
-- query excludes (task 4.5).
SELECT DISTINCT t.id, t.org_id, t.out_txn_id, t.in_txn_id, t.detected_at, t.dismissed_by, t.dismissed_at
FROM internal_transfers t
JOIN internal_transfer_members m ON m.transfer_id = t.id AND m.org_id = t.org_id
JOIN transactions txn ON txn.id = m.txn_id AND txn.org_id = m.org_id
WHERE t.dismissed_at IS NULL AND txn.entity_id = $1
ORDER BY t.detected_at DESC;

-- name: DismissInternalTransfer :one
-- Both columns set together (internal_transfers_dismissal_is_whole,
-- migration 012) -- who and when travel as one fact, never separately.
-- :one rather than :execrows: the caller returns the pair it just
-- dismissed, and pgx.ErrNoRows (already dismissed, or never existed) is
-- the same answer either way -- a policy denial, not a distinguishable
-- not-found.
UPDATE internal_transfers
SET dismissed_by = $2, dismissed_at = now()
WHERE id = $1 AND dismissed_at IS NULL
RETURNING id, org_id, out_txn_id, in_txn_id, detected_at, dismissed_by, dismissed_at;

-- name: PairedTransactionIDsForEntity :many
-- Task 4.5: the query a later change (the P&L) uses to exclude paired
-- transactions from every line. Unused by this change itself -- add-dedup
-- excludes nothing from a report that does not exist yet; it only detects
-- and records pairs.
SELECT m.txn_id
FROM internal_transfer_members m
JOIN internal_transfers t ON t.id = m.transfer_id AND t.org_id = m.org_id
JOIN transactions txn ON txn.id = m.txn_id AND txn.org_id = m.org_id
WHERE txn.entity_id = $1 AND t.dismissed_at IS NULL;

-- name: CountActiveTransfersForBatch :one
-- GetDedupSummary's own internal_transfers count: active pairs with at
-- least one side among this batch's transactions. The other side may
-- belong to a different batch -- a transfer's two legs are not
-- necessarily uploaded together.
SELECT count(DISTINCT t.id)
FROM internal_transfers t
JOIN internal_transfer_members m ON m.transfer_id = t.id AND m.org_id = t.org_id
JOIN transactions txn ON txn.id = m.txn_id AND txn.org_id = m.org_id
WHERE t.dismissed_at IS NULL AND txn.batch_id = $1;

-- name: TransferCandidatesForTransaction :many
-- D5's pairing rule, as a read: candidates for txn $3 (whose signed
-- base_amount_minor is $4, account is $5, and source_kind is $7) in
-- entity $1's own accounts, excluding $5 itself, within $2 days of
-- booked_on $6, opposite sign, equal absolute amount, the same
-- source_kind, not already a member of any pair. Ordered nearest by date
-- then lowest id, so the caller's greedy take-first is deterministic
-- (design D5) without re-sorting in Go.
--
-- The same source_kind: a ledger row and a bank row are never paired as an
-- internal transfer -- that is D4's job (ledger-to-bank matching), and D4
-- is Product's, out of scope here (proposal's non-goals, §6.12).
--
-- amount_minor is signed here (income positive, expense negative,
-- direction denormalised alongside it for readability) -- design D5's
-- "opposite signs, equal absolute amount" is exactly
-- "candidate's base-currency amount = -this row's base-currency amount".
--
-- COALESCE(base_amount_minor, amount_minor), not base_amount_minor alone:
-- migration 007's own all-or-nothing FX check guarantees that a row with a
-- NULL base_amount_minor is already denominated in the organisation's base
-- currency (design D2 in add-transaction-ledger), so amount_minor already
-- *is* the base-currency amount for exactly the rows that were never
-- converted. Reading base_amount_minor alone would silently exclude every
-- transaction in an organisation's own base currency from ever pairing --
-- the common case, not the rare one. $4 is the same coalesced value, read
-- for the transaction this is candidates for.
SELECT c.id, c.account_id, c.booked_on, COALESCE(c.base_amount_minor, c.amount_minor) AS base_amount
FROM transactions c
WHERE c.entity_id = $1
  AND c.id <> $3
  AND c.account_id <> $5
  AND c.source_kind = $7
  AND COALESCE(c.base_amount_minor, c.amount_minor) = -$4::bigint
  AND c.booked_on BETWEEN $6::date - ($2::int * INTERVAL '1 day')
                       AND $6::date + ($2::int * INTERVAL '1 day')
  AND NOT EXISTS (
        SELECT 1 FROM internal_transfer_members m
        WHERE m.org_id = c.org_id AND m.txn_id = c.id)
ORDER BY abs(c.booked_on - $6::date), c.id;
