## Why

Two things happen to every customer in the first month. They upload the same export twice,
because the first upload was before the month closed. And they move their own money between
their own accounts, which is not revenue and not a cost.

`ARCHITECTURE.md` §5 names the second as the more dangerous: without internal-transfer
detection, money a customer moves from one of their accounts to another is counted as
income. The columns both mechanisms need — `file_sha256`, `dedup_hash`, `normalize_version`
— are stored by changes 2.1 and 2.5 and compared by nothing.

Milestone: **Demo**. Capability: **`dedup-and-matching`** (new).

## Scope note — read before reviewing

**This change carries three capability deltas, against the rule in `CLAUDE.md`, and the
rule is right to make me say so.** The seam is clean and the split is real:

- **§1–§3, D1/D2/D3** — one hash family, one skip record, one number the customer reads
  ("412 rows imported · 88 duplicates skipped"). Splitting these three means assembling one
  sentence from two changes.
- **§4, internal transfers** — a different table, a different rule, a different consumer,
  and no dependency on the above.

`docs/IMPLEMENTATION_PLAN.md` §3 budgets them as one change of 1.5 days. **Recommend
splitting anyway**: `add-dedup` at 1 day and `add-internal-transfers` at 0.5. §4 is the
seam, and if the Demo runs short, internal transfers is the half that cannot be cut — so it
is better as a change that can be scheduled than as a section that can be quietly dropped.

## What Changes

- **Migration 011.**
- **D1, the same file twice.** A partial unique index on `(org_id, file_sha256)` over
  imported batches. It is a guarantee rather than a check, so two concurrent uploads of one
  file cannot both succeed.
- **D2 and D3, the same row twice** — within a batch, and against everything the
  organisation has already imported. Matching rows are skipped, not rejected: a duplicate
  is not a validation failure.
- **`dedup_skips` — every skipped row is recorded.** `ARCHITECTURE.md` says "skip, count,
  show which batch holds the original". A count alone is unreviewable, and the hash has
  false positives (§D3), so each skip keeps its line number and the transaction it matched.
- **Internal transfers.** Opposite signs, equal absolute amount, within 3 days, different
  accounts, same organisation. Detected pairs are **excluded from the P&L on detection**
  and a person may dismiss a pair.
- `normalize()` is **already** one implementation: PR #6 landed `core/internal/normalize`
  with a conformance test against `eval/norm.py`, and migration 007 stamps
  `normalize_version` on every transaction. This change consumes that rather than building it.

## Capabilities

### New Capabilities
- `dedup-and-matching`: D1, D2, D3, the skip record, and internal-transfer pairs.

### Modified Capabilities
- `file-ingestion`: a batch can fail as `already_imported`.
- `transaction-ledger`: persistence skips rows rather than writing all of them.

## Non-goals

- **No D4.** Ledger-to-bank matching is Product (`add-d4-ledger-bank-matching`). The Demo
  gives each customer one source kind and does not mix them.
- **No unique constraint on `dedup_hash`.** Two identical payments on one day are real data.
  See design D3. **Migration 007 already creates one** — `CREATE UNIQUE INDEX
  transactions_dedup_idx ON transactions (org_id, dedup_hash)`. Either it comes out before
  007 merges, or this change drops it. §0.4 settles which.
- **No dedup screen.** The counts and the skip list are returned; `add-web-experience` §6
  renders them.
- **No re-running dedup over history**, and no change to `normalize()`'s behaviour — only
  to where it lives.

## Impact

Touches `/core/migrations`, `/core/internal/db/query`, `/core/internal/dedup` (first code
in it), `/core/internal/ingest`, `/core/internal/normalize` (read only) and
`/proto/vekst/v1/import.proto` (**both reviewers**).

**Apply after 2.5.** Every comparison here reads a column that change writes.
