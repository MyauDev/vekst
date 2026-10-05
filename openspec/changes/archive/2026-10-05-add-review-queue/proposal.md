## Why

The engine answers 79% of rows and says nothing about the rest. This is the screen where a
person settles the rest, and the reason the *second* month takes three minutes instead of
fifteen: every decision made here writes a vendor-memory row, and L0 catches that
counterparty for free ever after. Without it the 21% is permanently unclassified and the
P&L has a hole in it that nothing closes.

It is also where the product's headline number lives. `WORKFLOW.md` screen 6a sets the
target: twelve months of first-time data resolved in **under fifteen minutes**. That is not
a UI aspiration, it is a constraint on the data model — it is only reachable if one decision
covers every row from a counterparty at once, which means the queue is grouped by
counterparty rather than by row, and sorted by amount so the €40,000 line is settled before
the €4 one.

Milestone: **Demo**. Capability: **`review-queue`** (new), with `classification-taxonomy`
extended where `vendors` gains its first writer.

## What Changes

- **Migration 008 creates `review_decisions`** — one row per human decision about a
  counterparty, not one per transaction. See the design's D1; this is the change's only real
  schema question.
- **`core/internal/review`** — the queue read (group, sort, count, sum) and the decision
  write, which is atomic and does four things at once: a `vendors` row, a `classifications`
  row for every affected transaction, the decision record, and nothing at all if any of them
  fails.
- **`proto/vekst/v1/review.proto`** — `ListReviewGroups`, `ResolveGroup`, `UndoDecision`.
  Browser-facing, so Connect rather than gRPC, and **both reviewers**.
- **Role enforcement arrives.** `memberships.role` has been stored and checked by nothing
  since migration 004, deliberately. This is the change where that stops: only `owner`,
  `admin` and `approver` may resolve, and a `viewer` gets the same screen read-only.
- **Internal transfer and non-P&L** become decisions a person can make, because the keyboard
  legend promises `T` and `N` and there is nowhere to record either today.

## Non-goals

- **No chat.** Screen 6b is the residue the queue cannot settle — "what *is* this account"
  rather than "which category" — and it is a separate change with an escalation rule and a
  human on the other end.
- **No UI.** The Review screen is change 5.2. This change ships the RPCs it calls and the
  behaviour it relies on.
- **No fuzzy matching.** Grouping is by exact `counterparty_key`, which is what
  `core/internal/normalize` already produces. L3 `pg_trgm` is Commercial.
- **No D4 matching.** A bank payment and the invoice it settles are linked by change 2.6,
  not resolved by a person here.
- **No bulk undo across batches.** `UndoDecision` reverses one decision; unwinding an
  import is the import's own concern.

## What this change is blocked by

Both are in flight and neither is optional:

| | |
| --- | --- |
| **2.5 `add-transaction-ledger`** | `transactions` and `classifications`. Migration 007 exists on `implement-transaction-ledger`; sections 2–5 of that change are not written yet |
| **The classification run** | The River worker that calls `ClassifyBatch` and writes the answers. Still unproposed — it is the gap 2.5's proposal names |

Until the second exists, every transaction is unclassified and the queue is "all of them",
which is a testable state but not a useful one. The plan below is written so that the
decision path can be built and tested against hand-inserted classifications, and the worker
changes nothing about it when it lands.

## Impact

Touches `/proto/vekst/v1` (**both reviewers**), `/core/internal/review` (new),
`/core/internal/identity` (the role check), `/core/migrations`, and `/core/internal/db`
(**both reviewers**) for the queries. Apply after 2.5.
