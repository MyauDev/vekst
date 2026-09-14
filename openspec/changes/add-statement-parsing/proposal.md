> **RETROACTIVE, 2026-09-13.** The code described here already exists and is merged:
> `core/internal/ingest` landed inside PR #4, which was the classification-taxonomy pull
> request, with no change proposed for it. This document is written from the code rather
> than the code from this document. Tasks already satisfied are ticked and say which file
> and which test satisfies them; the open ones are what is genuinely missing.
>
> It is written because three later changes declare "apply after 2.2" against a change that
> did not exist, and because the code settles two questions the planning documents get
> wrong. See **Corrections to the planning documents** below — those matter more than the
> bookkeeping.

## Why

Nothing can be validated, persisted, classified or reported until something can read a
bank's file. `core/internal/ingest` does: it detects a CIS charset, turns a bank's number
rendering into `int64` minor units without passing through a float, locates columns by
header name, and produces a `Statement` whose every row knows which line of the original
file it came from.

It reads one bank — Priorbank, Belarus — behind a registry, so the second bank is a file
and not an `if`.

Milestone: **Demo**. Capability: **`file-ingestion`** (new — this change introduces it;
2.1 and 2.4 extend it).

## What Changes

- **A `Parser` interface and a registry.** One parser per bank, not per file extension,
  because two banks writing CSV agree on almost nothing else. Registration happens in each
  parser's `init`, so adding a bank adds a file.
- **Ambiguity is an error.** If two parsers claim a file, `ParserFor` refuses and names
  both. A first-match win would hand a customer numbers read by the wrong reader.
- **Charset detection for CIS exports** — UTF-8, windows-1251, CP866 — decided by which
  code page yields more Cyrillic and fewer box-drawing characters.
- **Number-locale handling that never touches a float.** `"2 009,07"`, `"2.009,07"` and
  `"2,009.07"` all reach `money.Parse` as a plain decimal string and become `int64` minor
  units scaled by the currency's own exponent.
- **The Priorbank parser**, against four redacted real exports in `core/testdata`, covering
  both of that bank's column layouts.
- **Every row carries `LineNo`**, the 1-based line in the original file.
- **`Statement.BalanceCheck()`** measures opening + credits − debits against the declared
  closing balance, at zero tolerance, and returns the difference. It only measures; 2.3
  decides what a difference means.

## Corrections to the planning documents

**1. "Debit and credit are mutually exclusive" is wrong as written, and change 2.3 inherited
the error.** `IMPLEMENTATION_PLAN.md` §1.1 and `ARCHITECTURE.md` §4a.1 both state the
correctness check as failing "when both columns [are] populated". In a real Priorbank
export both columns are populated on every row, one of them `0,00`. Taken literally that
check rejects every row of every file. The rule is **both non-zero**. `priorbank.go` says
so in a comment and `TestDebitAndCreditAreBothPresentButNeverBothNonZero` pins it.

**2. The balance check is measured here and decided in 2.3.** `ARCHITECTURE.md` §4a.1 places
it in stage ②. The arithmetic needs the parsed rows and the bank's declared figures, which
only the parser has, so the measurement lives here and returns a difference. 2.3 owns the
outcome, the override and the report. Recording the seam so it does not get implemented
twice.

## Capabilities

### New Capabilities
- `file-ingestion`: reading a bank's own export format into a statement whose rows are
  traceable to the file they came from.

### Modified Capabilities
- None.

## Non-goals

- **No validation.** This package answers "what does the file say". 2.3 answers "is what it
  says complete and correct", including the date parsing `Row.BookedOn` defers by staying a
  string.
- **No persistence.** `Statement` is in memory. `raw_rows` from `ARCHITECTURE.md` §5.5 is
  not written by this change — see task 4.1, which is open.
- **No upload and no batch.** 2.1.
- **No import profiles.** 2.4. Columns are located by header name, which is why a profile
  pinning column positions would be wrong half the time.
- **No second bank and no XLSX.** The registry exists so that both are additive.

## Impact

Touches `/core/internal/ingest` and `/core/testdata` — both already landed. What remains is
`raw_rows` persistence, which needs 2.1's batch to point at.
