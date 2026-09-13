## Why

This is the report the product exists to print. Everything before it — ingest, the engine,
the queue — is machinery for producing one table a business owner reads in the first minute
of a month.

It is also the change where every earlier decision is either vindicated or found out. A line
computed from a mix of ledger and bank rows double-counts an invoice and its payment. A
figure that quietly omits unclassified transactions reports a smaller business than the one
that exists. A total summed across currencies without conversion is arithmetic on
incompatible units. All three are silent, and all three are this change's to prevent.

Milestone: **Demo**. Capability: **`report-mgmt-pnl`** (new).

## What Changes

- **`core/internal/report`** — the P&L: sections, computed lines, one column per period,
  Total and % of revenue. It is a pure function of the rows it is given, so it can be tested
  without a database and reproduced from stored versions.
- **Migration 009 fills `pnl_section`**, which migration 005 created and left NULL on all 46
  rows. See design §D2.
- **The computed lines stop being prose.** `formula` holds `'NET SALES - CS'` — category
  *names*, resolved by nothing and checked by nothing. Design §D1 replaces the arithmetic
  with structure and keeps the string as a label.
- **`proto/vekst/v1/report.proto`** — `GetManagementPNL`, browser-facing, **both
  reviewers**.
- **A report states what it could not include.** Unclassified rows, rows excluded as
  non-P&L, and rows a decision deferred are each counted and shown, in money. A report that
  omits them silently is the failure this change is most likely to ship.

## Non-goals

- **No drill-down.** Opening a figure to its transactions is change 4.2, and it needs one
  column change 2.5 does not yet store — see the blockers.
- **No other reports.** Sales, OPEX and Cash Flow are Commercial.
- **No export.** XLSX is its own change; this produces the numbers, not the file.
- **No readiness check.** "Ready / partial / blocked" is per-report and spans more than this
  one; the counts this change produces are what it will read.
- **No charts.** §5.3's tiles and charts ship with the web shell and consume this.
- **No VAT split.** D-4's default stands: report gross and label it.

## What this change is blocked by, and by whom

| | |
| --- | --- |
| **2.5** `transactions` and `classifications` | Migration 007 exists on `implement-transaction-ledger`; Track A is finishing the change |
| **The classification run** | Still unproposed. Without it every row is unclassified, which this report will state correctly and uselessly |
| **D-3, the output table structure** | Due 15 September, still open, and it is this change's shape |

The first two do not block writing the calculation: it is a pure function over rows, and
§D5 is about testing it that way.

## Three collisions with `origin/changes-2` that this change has to survive

Track A's branch proposes five ingest changes and three amendments to migration 007. Two of
them reach into this one.

1. **`direction` as a generated column.** The proposal is
   `CASE WHEN amount_minor > 0 THEN 'in' ELSE 'out' END`. The vocabulary is wrong for what
   already ships: all 71 seeded rules in migration 006 match `direction` against `income`
   and `expense`, and so do the classifier's field switch and the proto's own comment. A
   generated column is the right instinct — two unconstrained copies of one fact is exactly
   what migration 004 refused for `base_currency` — but it must generate `income` and
   `expense`, or it silently stops every rule from firing.

2. **The sign convention is undefined, and three changes now depend on it.** 2.6 pairs
   internal transfers on "opposite signs". The review queue sums signed amounts and orders
   by absolute value. This report sums a section. Nothing anywhere says whether an expense
   is stored negative. Until it does, `sum(amount_minor)` over OPEX has no defined sign and
   the P&L cannot be written. §D3 settles it.

3. **Migration numbering.** Track A's `add-file-upload` claims 008, which
   `00008_review_decisions.sql` already holds on `implement-review-queue`, and runs 009–011
   for the rest of the ingest chain. This change takes 009 only if that renumbering happens
   first; renumbering is free before a merge and an incident after.

## Impact

Touches `/proto/vekst/v1` (**both reviewers**), `/core/internal/report` (Track B),
`/core/internal/db` (**both reviewers**) and `/core/migrations`. Apply after 2.5.
