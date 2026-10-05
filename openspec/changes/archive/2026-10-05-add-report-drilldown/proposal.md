## Why

A figure nobody can open is a figure nobody can trust. The P&L says OPEX was 412,000 last
month; the only useful next question is *which payments*, and the second is *why are they
OPEX*. Without an answer to both, the report is a machine's assertion and an accountant has
no way to check it — which, for a product whose whole claim is that the numbers are right,
is the difference between a tool and a demo.

It is also where the engine becomes accountable. Every classification already carries the
layer that produced it, the evidence it rested on and the four versions it was made under.
Change 4.1 sums them and shows none of it. This shows it.

Milestone: **Demo**. Capability: **`report-mgmt-pnl`** (extended).

## What Changes

- **`ListLineTransactions`** — any cell in the report opens to the rows behind it, with
  category, engine layer, confidence and evidence on each.
- **Migration 010 adds `transactions.line_no`**, the line in the original file. Track A's
  own note on migration 007 asks for it and this is the change that needs it: a customer
  checking a figure opens their own spreadsheet, and a row they cannot find there is a row
  they cannot verify.
- **The reconciliation strip** — opening, in, out, transfers, closing — which `DESIGN.md` §8
  puts at the foot of the table. It is the arithmetic that proves the report did not lose
  anything.
- **The four exclusion buckets become openable too.** 4.1 counts unclassified money; this
  change lets somebody look at it, which is the only way that number turns into work.

## Non-goals

- **No editing.** Reclassifying from the drill-down is the review queue's job and it already
  has one; a second write path for the same act is a second place to get the append-only
  rule wrong.
- **No export.** XLSX carries a raw transactions sheet and is its own change.
- **No D4 evidence.** Showing the ledger row that matches a bank payment needs 2.6's links.
- **No search.** Filtering a drill-down by counterparty or amount is a screen concern, and
  the screen is 5.2.

## Blocked by

| | |
| --- | --- |
| **4.1** | The line identity this opens — a section code, a period, a basis — is 4.1's shape |
| **2.5** | `transactions`, and the column migration 010 adds to it |
| **2.2** | `line_no` has to arrive from the parser. `ingest.Row.LineNo` already carries it, pinned by `TestLineNumbersReferToTheOriginalFile`; it is lost at the persist boundary |

## One thing to settle with Track A first

`line_no` could equally land in migration 007 before it merges, which is where Track A's
`SUPERSEDED.md` puts it. That is better: one column added before the table has rows beats a
backfill of every transaction afterwards, and this change then needs no migration at all.

The task list below assumes it does not happen and carries migration 010. If 007 takes the
column, section 1 disappears and this change is two days rather than one and a half.

## Impact

Touches `/proto/vekst/v1` (**both reviewers**), `/core/internal/report`,
`/core/internal/db` (**both reviewers**), and `/core/migrations` only if `line_no` is not
already there. Apply after 4.1.
