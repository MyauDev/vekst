-- The management P&L's reads. Change 4.1, capability `report-mgmt-pnl`.
--
-- Every statement runs inside db.InTx, which sets the tenant context, and none
-- of them restates the tenant predicate: row-level security already admits this
-- organisation's rows and nothing else, and writing it again would be a second
-- place to get isolation right.
--
-- Three properties hold across the whole file, and each is a way this product
-- would otherwise print a wrong number.
--
--   **One source kind.** Every read filters on `source_kind`, because a line
--   computed from a mix of ledger and bank rows counts an invoice and its
--   payment twice (ARCHITECTURE.md 5.1). `report_source_kind_test.go` asserts
--   that no query here omits the filter -- a join somebody adds later without
--   it is the failure no other test can see.
--
--   **The base currency, always.** `coalesce(base_amount_minor, amount_minor)`:
--   migration 007 stores the conversion rather than deriving it, and a row with
--   no conversion is already in the base currency. So this is exact, not an
--   approximation, and summing face values across currencies -- arithmetic on
--   incompatible units -- cannot happen.
--
--   **Aggregates, not rows.** The report sums; it does not need identifiers.
--   Returning one row per transaction would put a year of a real business --
--   tens of thousands of rows -- on the wire to produce a table of twelve
--   columns. These return one row per (period, category), which is bounded by
--   the taxonomy no matter how much a customer trades. The drill-down that does
--   want the transactions is change 4.2, and it asks for one figure at a time.
--
-- Periods are months here and nothing else. Quarters and years are months
-- folded in `core/internal/report`, where the arithmetic is a pure function
-- with tests and not a `to_char` format string repeated in four queries.

-- name: ReportLines :many
-- What the report is computed from: transactions with a live classification,
-- of the requested basis, in the requested range.
--
-- The category's own `is_pnl` and `requires_allocation` travel with the sum
-- rather than being decided here. Which bucket a row falls in is D6's rule and
-- it lives in one place; a CASE here would be a second copy of it, in the
-- language least able to test it.
SELECT to_char(t.booked_on, 'YYYY-MM')::text AS period,
       cat.code::text                        AS category_code,
       cat.pnl_section::text                 AS section,
       cat.is_pnl,
       cat.requires_allocation,
       sum(coalesce(t.base_amount_minor, t.amount_minor))::bigint AS amount_minor
  FROM transactions t
  JOIN classifications c
    ON c.org_id = t.org_id
   AND c.transaction_id = t.id
   AND c.superseded_by IS NULL
   AND c.retracted_at IS NULL
  JOIN categories cat ON cat.id = c.category_id
 WHERE t.entity_id = @entity_id
   AND t.source_kind = @source_kind
   AND t.booked_on BETWEEN @from_date AND @to_date
 GROUP BY 1, 2, 3, 4, 5
 ORDER BY 1, 2;

-- name: ReportUnclassifiedTotals :many
-- The bucket that decides whether the table above can be trusted: rows of the
-- report's own basis that no live classification answers. A P&L summing only
-- what was classified describes a smaller business than the one that exists,
-- and looks finished while doing it.
--
-- Same NOT EXISTS as the review queue, and deliberately so: "needs review" and
-- "missing from the report" are one fact, and two definitions of it would
-- drift into a row that is in neither.
SELECT to_char(t.booked_on, 'YYYY-MM')::text AS period,
       sum(coalesce(t.base_amount_minor, t.amount_minor))::bigint AS amount_minor
  FROM transactions t
 WHERE t.entity_id = @entity_id
   AND t.source_kind = @source_kind
   AND t.booked_on BETWEEN @from_date AND @to_date
   AND NOT EXISTS (
       SELECT 1 FROM classifications c
        WHERE c.org_id = t.org_id
          AND c.transaction_id = t.id
          AND c.superseded_by IS NULL
          AND c.retracted_at IS NULL)
 GROUP BY 1
 ORDER BY 1;

-- name: ReportOtherBasisTotals :many
-- What the report is not computed from, counted rather than dropped (design
-- D4). Where an organisation holds both ledger and bank rows for a period, the
-- other side is reconciliation evidence: a figure a person can compare against,
-- and the thing that makes "this is the bank basis" a statement with a
-- consequence rather than a label.
--
-- `<>` and not a second parameter naming the other kind: there are exactly two,
-- and a caller that could name the third could name the same one twice.
SELECT to_char(t.booked_on, 'YYYY-MM')::text AS period,
       sum(coalesce(t.base_amount_minor, t.amount_minor))::bigint AS amount_minor
  FROM transactions t
 WHERE t.entity_id = @entity_id
   AND t.source_kind <> @source_kind
   AND t.booked_on BETWEEN @from_date AND @to_date
 GROUP BY 1
 ORDER BY 1;

-- name: ReportVersions :many
-- The versions the summed classifications were actually made under.
--
-- Read from the rows, never from the binary. A constant compiled into the
-- process says what this build would classify with today; a report says what
-- its figures were classified with, and those are the same string only until
-- the first redeploy. Those three versions plus the normalisation that produced
-- the text they matched on are what make a March report reproduce in June, and
-- an accountant will ask.
--
-- DISTINCT, and returned as a set: a report summing rows classified under two
-- engine versions was produced under two engine versions, and naming one of
-- them is a claim about reproducibility that is not true.
SELECT DISTINCT c.taxonomy_version, c.ruleset_version,
       c.engine_version, c.normalize_version
  FROM transactions t
  JOIN classifications c
    ON c.org_id = t.org_id
   AND c.transaction_id = t.id
   AND c.superseded_by IS NULL
   AND c.retracted_at IS NULL
 WHERE t.entity_id = @entity_id
   AND t.source_kind = @source_kind
   AND t.booked_on BETWEEN @from_date AND @to_date
 ORDER BY 1, 2, 3, 4;
