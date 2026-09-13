## Why

`docs/IMPLEMENTATION_PLAN.md` §0 calls this "the most valuable thing added since the first
draft", and `docs/ARCHITECTURE.md` §4a.1 calls the balance check "the strongest check that
exists". Neither statement is true of anything in the repository, because stage ② is not
written.

Without it the product computes numbers that look right. With it the product computes
numbers it can prove are complete. That difference is what an accountant is buying, and it
is the only claim the Demo makes that a spreadsheet cannot.

Milestone: **Demo**. Capability: **`ingest-validation`** (new).

## What Changes

- Migration 009 creates `import_validations` — one row per validated batch, holding the
  outcome, the counts, whether the balance reconciled, the error report, and the override
  if there was one.
- **Seven correctness checks, per row, never overridable.** Date plausible, amount parses
  to minor units, ISO-4217 currency, **debit and credit not both non-zero**, no U+FFFD, a
  description after trimming, the account resolves.
- **Five completeness checks, per file.** The critical one is
  `opening + Σ movements = closing`, **at zero tolerance** — which is a check that only
  exists because amounts are `int64` minor units. At any tolerance above zero it stops
  proving anything, and in floating point "zero tolerance" is not a sentence that means
  anything.
- **Three outcomes:** `valid`, `valid_with_warnings`, `rejected`. A rejected batch persists
  no transaction. The validation row is still written, because it is the receipt, not the
  data.
- **A completeness warning may be overridden; a correctness error may not.** This is a
  check constraint, not a code path: the override columns can only be populated when the
  outcome is `valid_with_warnings`.
- **An override reaches every report computed from that batch.** `OverriddenBatchesForPeriod`
  exists so that change 4.1 cannot silently omit it.
- The error report is keyed by **line number in the original file**, and carries codes, not
  sentences.

## Capabilities

### New Capabilities
- `ingest-validation`: the two check families, the three outcomes, the override and its
  audit trail.

### Modified Capabilities
- `file-ingestion`: the batch moves through `validating` to `validated` or `rejected`.

## A rule this change inherited wrong

`ARCHITECTURE.md` §4a.1 and `IMPLEMENTATION_PLAN.md` §1.1 both state the check as failing
when **both columns are populated**. Change 2.2's Priorbank parser shows that is wrong: in
every real export both columns are populated on every row, one of them `0,00`, so the rule
as written rejects every row of every file. The check is **both non-zero**, and
`TestDebitAndCreditAreBothPresentButNeverBothNonZero` already pins it in
`core/internal/ingest`. Change 2.2 task §5.1 corrects both documents; this change
implements the corrected rule and nothing else.

## Non-goals

- **No parsing.** 2.2 produces the rows and the line numbers this change reads, and it
  already does: `ingest.Row.LineNo` carries the original file's line and a test pins it.
  2.2 also already *measures* the balance check via `Statement.BalanceCheck()`; this change
  decides what a non-zero difference means, and calls that function rather than repeating
  the arithmetic.
- **No persistence.** 2.5 writes the transactions that only a `valid` or
  `valid_with_warnings` batch reaches.
- **No deduplication.** A duplicate is not a validation failure. 2.6.
- **No error-report screen and no override UI.** `add-web-experience` §6 renders them; this
  change produces the codes and the i18n keys it renders.
- **No role enforcement**, unless §0.2 is answered the other way. See there.

## Impact

Touches `/core/migrations`, `/core/internal/db/query`, `/core/internal/ingest`,
`/proto/vekst/v1/import.proto` (**both reviewers**) and `/web` for the message catalogue.

**Apply after 2.2 and 2.5.** This change reads parsed rows and their original line numbers.

**`line_no` does not exist.** Change 2.5's migration 007 stores no line number on a
transaction, so the error report can be keyed to the original file but a figure in a report
cannot be opened back to it. Either 007 gains the column before it merges, or this change
adds it — §0.1 settles which, and it is not optional either way.
