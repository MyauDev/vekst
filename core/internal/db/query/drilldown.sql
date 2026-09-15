-- Opening a figure. Change 4.2, capability `report-mgmt-pnl`.
--
-- Every statement runs inside db.InTx, which sets the tenant context, and none
-- of them restates the tenant predicate: row-level security already admits this
-- organisation's rows and nothing else.
--
-- The whole difficulty of this change is in the first query, and it is not the
-- SQL. A drill-down that returns rows adding up to something other than the
-- figure they were opened from is worse than no drill-down: it makes a correct
-- report look wrong, or -- the case that matters -- a wrong report look
-- checked. So there is **one** predicate here and not one per line. `@line`
-- discriminates inside it, in the same order `report.Compute` places a row, and
-- the two are kept honest by a test that sums every non-zero cell's drill-down
-- back to the cell.
--
-- Everything else follows the report's own file: one `source_kind` per read,
-- `coalesce(base_amount_minor, amount_minor)` so amounts are summed in one
-- currency, and dates as a closed interval on `booked_on`.

-- name: LineTransactions :many
-- The rows behind one cell, oldest first, from a cursor.
--
-- The line predicate below is `report.Compute`'s switch, written out. Read them
-- side by side, in this order, because the order is load-bearing: a row of the
-- other basis never reaches a bucket about classification; an unclassified row
-- is unclassified whatever its category would have said; a category marked
-- non-P&L is excluded before anybody asks whether it needs allocating. Change
-- one and the figure and its drill-down stop agreeing, which is the one failure
-- this file exists to prevent.
--
-- The classification is a LEFT JOIN and not an inner one, because three of the
-- four buckets contain rows that have none, and an inner join would return them
-- as an empty page -- which reads as "these rows went missing" rather than as
-- "these rows were never answered".
SELECT t.id, t.booked_on, t.line_no, t.batch_id, t.posting_no, t.document_ref,
       t.amount_minor, t.currency, t.base_amount_minor, t.base_currency,
       t.counterparty_raw, t.description_raw, t.regulated_code, t.source_kind,
       coalesce(cat.code, '')::text          AS category_code,
       coalesce(cat.name, '')::text          AS category_name,
       coalesce(c.engine_layer, '')::text    AS engine_layer,
       c.confidence,
       coalesce(c.evidence, '')::text        AS evidence,
       c.decided_by
  FROM transactions t
  LEFT JOIN classifications c
    ON c.org_id = t.org_id
   AND c.transaction_id = t.id
   AND c.superseded_by IS NULL
   AND c.retracted_at IS NULL
  LEFT JOIN categories cat ON cat.id = c.category_id
 WHERE t.entity_id = @entity_id
   AND t.booked_on BETWEEN @from_date AND @to_date

   -- One source kind, always. The other-basis bucket is the one cell whose
   -- rows are of the kind this report is *not* computed from, which is what
   -- makes it evidence rather than an omission.
   AND CASE WHEN @line::text = 'other_basis'
            THEN t.source_kind <> @source_kind
            ELSE t.source_kind =  @source_kind
       END

   AND CASE @line::text
       WHEN 'other_basis'  THEN true
       WHEN 'unclassified' THEN c.id IS NULL
       WHEN 'non_pnl'      THEN c.id IS NOT NULL
                            AND (NOT cat.is_pnl
                                 OR (NOT cat.requires_allocation
                                     AND cat.pnl_section IN ('08', '09')))
       WHEN 'unallocated'  THEN c.id IS NOT NULL
                            AND cat.is_pnl AND cat.requires_allocation
       ELSE                     c.id IS NOT NULL
                            AND cat.is_pnl
                            AND NOT cat.requires_allocation
                            AND cat.pnl_section = @line
       END

   -- The cursor. A drill-down is not being emptied while somebody reads it --
   -- unlike the review queue, which is why that one pages by offset and this
   -- one does not -- so (booked_on, id) is stable, unique and already indexed,
   -- and a cursor over it neither skips a row nor repeats one. A NULL cursor is
   -- the first page: a client asking for it has nothing to send.
   AND (sqlc.narg(cursor_booked_on)::date IS NULL
        OR (t.booked_on, t.id) > (sqlc.narg(cursor_booked_on)::date, sqlc.narg(cursor_id)::uuid))
 ORDER BY t.booked_on, t.id
 LIMIT sqlc.arg(row_limit);

-- name: LineTotal :one
-- The same cell, summed. What the screen puts above the list -- "9 rows,
-- 412,000" -- so a reader knows whether the page they are looking at is the
-- whole answer.
--
-- The predicate is copied from the query above, deliberately and visibly: sqlc
-- generates static SQL, so a shared fragment is not available, and the two must
-- be read together. What keeps them equal is not proximity but the test, which
-- compares this total, the sum of the paged rows, and the report's own figure
-- -- three numbers from three code paths, and any two disagreeing names which
-- one drifted.
SELECT count(*)                                                     AS row_count,
       coalesce(sum(coalesce(t.base_amount_minor, t.amount_minor)), 0)::bigint AS amount_minor
  FROM transactions t
  LEFT JOIN classifications c
    ON c.org_id = t.org_id
   AND c.transaction_id = t.id
   AND c.superseded_by IS NULL
   AND c.retracted_at IS NULL
  LEFT JOIN categories cat ON cat.id = c.category_id
 WHERE t.entity_id = @entity_id
   AND t.booked_on BETWEEN @from_date AND @to_date
   AND CASE WHEN @line::text = 'other_basis'
            THEN t.source_kind <> @source_kind
            ELSE t.source_kind =  @source_kind
       END
   AND CASE @line::text
       WHEN 'other_basis'  THEN true
       WHEN 'unclassified' THEN c.id IS NULL
       WHEN 'non_pnl'      THEN c.id IS NOT NULL
                            AND (NOT cat.is_pnl
                                 OR (NOT cat.requires_allocation
                                     AND cat.pnl_section IN ('08', '09')))
       WHEN 'unallocated'  THEN c.id IS NOT NULL
                            AND cat.is_pnl AND cat.requires_allocation
       ELSE                     c.id IS NOT NULL
                            AND cat.is_pnl
                            AND NOT cat.requires_allocation
                            AND cat.pnl_section = @line
       END;

-- name: ReconciliationForPeriod :one
-- The strip at the foot of the table: in, out and transfers, of one basis over
-- one range.
--
-- Money in and money out are reported as positive magnitudes, because that is
-- how they read on a page, and the store's signs are what separates them here.
--
-- A transfer leg is excluded from both and counted on its own. It is the
-- organisation moving its own money, and calling it revenue in one account and
-- an expense in another is how a business appears to trade with itself. The
-- join to `internal_transfers` carries the dismissal: a pair a person has
-- dismissed is not a transfer any more, and its legs go back to being ordinary
-- movement -- which is why the membership row alone is not the test.
--
-- The transfers term is signed as an outflow, so the strip's identity reads the
-- way DESIGN.md writes it: opening + in - out - transfers = closing. Where both
-- legs of every pair are inside this entity it comes to zero, and a zero with a
-- reason beside it is worth more than a blank.
SELECT coalesce(sum(coalesce(t.base_amount_minor, t.amount_minor))
                FILTER (WHERE it.id IS NULL
                          AND coalesce(t.base_amount_minor, t.amount_minor) > 0), 0)::bigint
           AS in_minor,
       coalesce(-sum(coalesce(t.base_amount_minor, t.amount_minor))
                FILTER (WHERE it.id IS NULL
                          AND coalesce(t.base_amount_minor, t.amount_minor) < 0), 0)::bigint
           AS out_minor,
       coalesce(-sum(coalesce(t.base_amount_minor, t.amount_minor))
                FILTER (WHERE it.id IS NOT NULL), 0)::bigint
           AS transfers_minor,
       count(*) FILTER (WHERE it.id IS NOT NULL) AS transfer_row_count
  FROM transactions t
  LEFT JOIN internal_transfer_members m
    ON m.org_id = t.org_id AND m.txn_id = t.id
  LEFT JOIN internal_transfers it
    ON it.org_id = m.org_id AND it.id = m.transfer_id AND it.dismissed_at IS NULL
 WHERE t.entity_id = @entity_id
   AND t.source_kind = @source_kind
   AND t.booked_on BETWEEN @from_date AND @to_date;

-- name: OpeningBalanceBefore :one
-- Everything this entity moved before the range, in the base currency.
--
-- Derived, and the response says so. A real opening balance is the one the
-- statement itself declared, which nothing stores yet -- change 2.3's balance
-- check is where those arrive. Until then the identity at the foot of the table
-- holds by construction, and labelling that as derived is the difference
-- between an informative strip and a check somebody trusts for something it
-- cannot do.
SELECT coalesce(sum(coalesce(t.base_amount_minor, t.amount_minor)), 0)::bigint AS amount_minor
  FROM transactions t
 WHERE t.entity_id = @entity_id
   AND t.source_kind = @source_kind
   AND t.booked_on < @from_date;
