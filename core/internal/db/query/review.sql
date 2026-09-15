-- The review queue: the read that builds it, and the writes that empty it.
--
-- The queue is not a table. A transaction needing review is one with no live
-- classification, which `classifications` already says -- so these are queries
-- over `transactions`, not over a state somebody has to keep current.
--
-- Every statement runs inside db.InTx, which sets the tenant context. None of
-- them restates the tenant predicate: row-level security already admits this
-- organisation's rows and nothing else, and writing it again here would be a
-- second place to get isolation right.
--
-- Every insert takes org_id from app_current_org() rather than as a parameter,
-- for the same reason. db.OrgID cannot be built outside core/internal/db and
-- its wire form is unexported, so a caller could not supply one anyway -- but
-- the deeper point is that a parameter is a chance to pass the wrong value,
-- and the transaction already knows the right one. The WITH CHECK on each
-- policy would reject a mismatch; not being able to express one is better.

-- name: OrganizationBaseCurrency :one
-- The currency every total in this file is denominated in. Read inside the
-- same transaction that sums, rather than passed in by a caller who read it
-- earlier: a total and the code beside it have to come from one moment.
SELECT base_currency FROM organizations WHERE id = app_current_org();

-- name: ReviewGroups :many
-- The queue, grouped by counterparty and ordered so that the largest amount is
-- settled first.
--
-- Three things here are decisions rather than SQL.
--
--   coalesce(base_amount_minor, amount_minor) -- comparing amounts across
--   currencies is only meaningful in one of them, and migration 007 stores the
--   converted amount rather than deriving it. A row with no conversion is
--   already in the base currency, so this is exact and not an approximation.
--   Summing face values would put a JPY row at the top of every queue.
--
--   abs(...) -- a 40,000 refund matters as much as a 40,000 payment, and
--   sorting signed puts every expense below every income.
--
--   counterparty_key last -- a deterministic tiebreak, so two reads agree and
--   the ground does not move under somebody working by keyboard.
--
-- The empty key is a group like any other: a row whose counterparty could not
-- be identified is the hardest row in the queue, and a queue that hides those
-- reports a completion it did not reach.
SELECT t.counterparty_key,
       max(t.counterparty_raw)::text AS display_name,
       count(*)                      AS row_count,
       sum(coalesce(t.base_amount_minor, t.amount_minor))::bigint AS total_minor,
       min(t.booked_on)::date        AS first_seen,
       max(t.booked_on)::date        AS last_seen
FROM transactions t
WHERE t.entity_id = $1
  AND NOT EXISTS (
      SELECT 1 FROM classifications c
       WHERE c.org_id = t.org_id
         AND c.transaction_id = t.id
         AND c.superseded_by IS NULL
         AND c.retracted_at IS NULL)
GROUP BY t.counterparty_key
ORDER BY abs(sum(coalesce(t.base_amount_minor, t.amount_minor))) DESC,
         count(*) DESC,
         t.counterparty_key
LIMIT $2 OFFSET $3;

-- name: ReviewGroupRows :many
-- The transactions behind one group, for the drill-down the screen opens and
-- for the resolve that follows it. Ordered by date so a person reading them
-- sees a story rather than a set.
SELECT t.id, t.entity_id, t.account_id, t.booked_on, t.direction,
       t.amount_minor, t.currency, t.base_amount_minor, t.base_currency,
       t.description_raw, t.description_norm, t.counterparty_raw,
       t.counterparty_key, t.regulated_code, t.source_kind, t.normalize_version
FROM transactions t
WHERE t.entity_id = $1
  AND t.counterparty_key = $2
  AND NOT EXISTS (
      SELECT 1 FROM classifications c
       WHERE c.org_id = t.org_id
         AND c.transaction_id = t.id
         AND c.superseded_by IS NULL
         AND c.retracted_at IS NULL)
ORDER BY t.booked_on, t.id;

-- name: UnclassifiedTotals :one
-- "412 rows across 88 counterparties left." What the screen puts above the
-- queue, and what tells a person whether fifteen minutes is plausible.
SELECT count(*)                                        AS row_count,
       count(DISTINCT t.counterparty_key)              AS counterparty_count,
       coalesce(sum(abs(coalesce(t.base_amount_minor, t.amount_minor))), 0)::bigint AS total_minor
FROM transactions t
WHERE t.entity_id = $1
  AND NOT EXISTS (
      SELECT 1 FROM classifications c
       WHERE c.org_id = t.org_id
         AND c.transaction_id = t.id
         AND c.superseded_by IS NULL
         AND c.retracted_at IS NULL);

-- name: InsertReviewDecision :one
-- One row per human decision about a counterparty. The covered count and total
-- are what the user was shown, recorded so the decision stays explicable in the
-- terms it was taken in.
INSERT INTO review_decisions (
    org_id, counterparty_key, key_version, outcome, category_id, decided_by,
    covered_count, covered_minor, covered_currency)
VALUES (app_current_org(), $1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UndoReviewDecision :one
-- Stamped, never deleted: what a person did and then reversed is part of the
-- audit trail. The WHERE clause makes a second undo affect no row, which
-- db.ExactlyOneRow turns into an error rather than a silent success.
UPDATE review_decisions
   SET undone_at = now(), undone_by = $2
 WHERE id = $1 AND undone_at IS NULL
RETURNING *;

-- name: LiveDecisionForCounterparty :one
SELECT * FROM review_decisions
 WHERE key_version = $1 AND counterparty_key = $2 AND undone_at IS NULL;

-- name: ReviewDecisionByID :one
SELECT * FROM review_decisions WHERE id = $1;

-- ---------------------------------------------------------------------------
-- The writes a decision fans out into.
-- ---------------------------------------------------------------------------

-- Inserting a classification lives in core/internal/ledger, which change 2.5
-- gave a typed home and its own tests. A second copy here would be a second
-- place to get the append-only rule wrong.

-- name: RetractClassificationsOfTransactions :execrows
-- The undo path. A retraction is not a supersession: supersession names the
-- classification that replaced this one, and an undo has no replacement --
-- the rows go back into the queue with no answer at all. Migration 007 carries
-- both columns for exactly this reason, and a row may carry one or the other
-- and never both.
UPDATE classifications
   SET retracted_at = now(), retracted_by = $1
 WHERE transaction_id = ANY(@transaction_ids::uuid[])
   AND superseded_by IS NULL
   AND retracted_at IS NULL;

-- name: LiveClassificationsForCounterparty :many
-- What a decision wrote, found again by the counterparty it was about. Used by
-- the undo, which has a decision and needs the rows it touched.
SELECT c.* FROM classifications c
  JOIN transactions t ON t.org_id = c.org_id AND t.id = c.transaction_id
 WHERE t.counterparty_key = $1
   AND c.superseded_by IS NULL
   AND c.retracted_at IS NULL
   AND c.engine_layer = 'human';

-- ---------------------------------------------------------------------------
-- Vendor memory: the reason month two takes three minutes.
-- ---------------------------------------------------------------------------

-- name: UpsertVendor :one
-- Written only when a category was chosen. An internal transfer is not a
-- vendor fact and a non-P&L marking is a property of the movement -- memory
-- for either would make L0 answer next month with something that is not a
-- category.
INSERT INTO vendors (org_id, key, key_version, display_name, category_id, decided_by)
VALUES (app_current_org(), $1, $2, $3, $4, $5)
ON CONFLICT (org_id, key_version, key)
DO UPDATE SET display_name = excluded.display_name,
              category_id  = excluded.category_id,
              decided_by   = excluded.decided_by,
              decided_at   = now()
RETURNING *;

-- name: DeleteVendor :execrows
-- The one non-append-only write in this flow, and the asymmetry is deliberate:
-- memory is current state, classifications are history. A superseded vendor row
-- would keep answering L0 with a category the user has just taken back.
DELETE FROM vendors WHERE key_version = $1 AND key = $2;
