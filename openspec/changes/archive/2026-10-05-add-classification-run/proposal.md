## Why

`add-transaction-ledger` (2.5) gives `core` a way to read a page of unclassified
transactions and a way to store a classification, and `add-classification-engine` (3.2)
gives it something that turns transactions into proposals. Nothing yet calls one with the
other: every row landing in `transactions` stays unclassified forever, with no path from
"persisted" to "in a report."

## What Changes

- A River job, enqueued once per import batch after it reaches `imported`, that pages
  through that batch's transactions via `ledger.UnclassifiedTransactions`, chunks them at
  5,000 (ARCHITECTURE.md §3.5), and calls the `Classifier` interface once per chunk.
- Each chunk's response is written in one `db.InTx`: a proposal at or above the
  organisation's threshold becomes a classification via `ledger.InsertClassification`;
  below threshold, the transaction is left for the review queue a later change reads.
  **A chunk that errors writes nothing** — atomic per chunk, never partial.
- Retry, timeout and unknown-`category_id` rejection exactly as ARCHITECTURE.md §3.5
  already specifies — this change wires the table, not the policy.
- A recorded run outcome on the batch, so the import screen can show "classifying" versus
  "classified" versus "failed to classify" instead of sitting at `imported` silently.

## Non-goals

- The review queue screen and its approval-writes-vendor-memory behaviour.
- L2/L3/L4 layers, fuzzy matching, or any change to `Classifier` itself.
- Re-running classification after a taxonomy/ruleset/engine upgrade — a backfill with its
  own scope.
- Report computation (`add-management-pnl`'s).

## Capabilities

### New Capabilities
- `classification-run`: the job turning a persisted batch's transactions into
  classifications (or leaving them for review), and the outcome recorded on the batch.

### Modified Capabilities
(none)

## Impact

- New: a job/worker package under `core/internal`, registered with `core/internal/jobs`.
- Reads `core/internal/ledger` only (`UnclassifiedTransactions`, `InsertClassification`) —
  no schema change there.
- Stacks on `add-classification-engine` (the `Classifier` interface) and
  `add-transaction-ledger` (the ledger this reads) — both complete in `openspec list` but
  unarchived, the same stacking `add-import-profiles` already documented. **Apply after
  `add-dedup` (2.6) too**: nothing reaches `imported` — this change's own trigger — until
  that change's persistence step runs, even though this change reads none of its tables.
