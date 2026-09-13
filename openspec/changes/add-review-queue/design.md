## Context

The queue is a read and a write, and almost all of the difficulty is in the write. The read
is a grouped aggregate over `transactions` left-joined to its live `classification`; the
write has to do four things atomically and be undoable afterwards, because a person working
a keyboard-first queue at the pace this product promises will press the wrong digit.

What exists and constrains this:

| | |
| --- | --- |
| `transactions.counterparty_key` | migration 007, produced by `core/internal/normalize` |
| `classifications`, append-only, one live row per transaction | migration 007 |
| `vendors (org_id, key, key_version, category_id, decided_by)` | migration 006, no writer yet |
| `memberships.role` ∈ owner/admin/approver/viewer | migration 004, read by nothing |
| Proposals below the threshold are simply absent from a response | change 3.2 |

That last line is the one that shapes the read: **a transaction needing review is a
transaction with no live classification.** The engine does not emit a low-confidence
proposal and mark it; it emits nothing. So "needs review" is already a fact about the data
rather than a state somebody has to set.

## D1 — The queue is derived; only the decision is stored

`ARCHITECTURE.md` §5.5 sketches `review_items(id, org_id, transaction_id, state,
resolved_by, resolved_at)` — a row per transaction, with a state machine. This change does
not create it, and the reason is worth stating because the sketch is the more obvious design.

A `review_items` row would duplicate a fact `classifications` already holds. "Resolved"
means a live classification exists; "open" means one does not. Two representations of one
fact drift, and the drift is silent: a row marked resolved with no classification is
invisible on the P&L and absent from the queue, which is the worst of both.

What is *not* derivable is the human act: who decided, when, on what evidence, and — the
one that matters — **which transactions one decision covered**. A decision covers a
counterparty, and the rows it covered are the rows that existed at that moment. Tomorrow's
import adds more rows for the same counterparty; those are caught by L0 from the vendor
memory the decision wrote, not by the decision itself. So:

```sql
CREATE TABLE review_decisions (
    org_id        uuid        NOT NULL,
    id            uuid        NOT NULL DEFAULT gen_random_uuid(),

    -- What the decision was about. The key, not a transaction: one decision
    -- covers every row from that counterparty, which is the whole reason
    -- fifteen minutes is achievable.
    counterparty_key text     NOT NULL,
    key_version      text     NOT NULL,

    -- What was decided. Exactly one of these is meaningful, which the CHECK
    -- below enforces rather than trusts.
    outcome       text        NOT NULL
                  CHECK (outcome IN ('categorised', 'internal_transfer', 'non_pnl', 'skipped')),
    category_id   uuid        NULL,

    decided_by    uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    decided_at    timestamptz NOT NULL DEFAULT now(),

    -- How many rows it covered when it was made, and what they were worth.
    -- Stored because it is what the user was shown, and a decision has to be
    -- explicable in the terms it was made in.
    covered_count integer     NOT NULL CHECK (covered_count >= 0),
    covered_minor bigint      NOT NULL,
    covered_currency text     NOT NULL CHECK (covered_currency ~ '^[A-Z]{3}$'),

    undone_at     timestamptz NULL,
    undone_by     uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,

    PRIMARY KEY (org_id, id),

    CONSTRAINT decision_category_matches_outcome CHECK (
        (outcome = 'categorised' AND category_id IS NOT NULL)
        OR (outcome <> 'categorised' AND category_id IS NULL)),

    CONSTRAINT decision_undo_is_whole CHECK (
        num_nonnulls(undone_at, undone_by) IN (0, 2))
);

-- One live decision per counterparty. A second one is a correction, and a
-- correction undoes the first rather than sitting beside it.
CREATE UNIQUE INDEX review_decisions_one_live_idx
    ON review_decisions (org_id, key_version, counterparty_key)
    WHERE undone_at IS NULL;
```

**Alternative rejected:** derive the decision too, from the `classifications` rows with
layer `human`. It nearly works, and it loses the grouping: five hundred classification rows
written by one keystroke look identical to five hundred separate decisions, so the undo is
"which of these did I mean" and the audit trail is unreadable.

## D2 — Resolving is one transaction that does four things

```
ResolveGroup(counterparty_key, outcome, category_id)
  ├─ authorise: the caller's membership role is owner | admin | approver
  ├─ read: every transaction with this counterparty_key and no live classification
  ├─ write: a vendors row      (outcome = categorised only)
  ├─ write: a classifications row per transaction, engine_layer 'human'
  ├─ write: the review_decisions row, with the count and sum it covered
  └─ all of it inside one db.InTx, or none of it
```

The order is not arbitrary. The read establishes the set, and the set is what
`covered_count` records — so a row imported between the read and the commit is not silently
included in a decision the user never saw. Serialising is unnecessary: the classification
insert takes the live-row unique index, so a concurrent classification for the same
transaction makes this transaction fail rather than double-write.

`vendors` is written only for `categorised`. An internal transfer is not a category and a
non-P&L row is not a vendor fact — both are properties of the movement, and writing memory
for them would make L0 answer next month with a category that does not exist.

**Internal transfer and non-P&L need somewhere to land**, and the taxonomy has exactly one
of the two. Codes `08` CAPEX and `09` OUT OF P&L are the seeded leaves carrying
`is_pnl false`; codes 91–95 are GM, NM, CM, IBT and NI, which are computed lines a
classification may never target. So:

- **`N`, non-P&L**, resolves to `09` — an existing leaf, excluded from every P&L line by
  `is_pnl`, needing no new concept in change 4.1.
- **`T`, internal transfer**, has no home. It is not a category: a transfer between two of
  the organisation's own accounts is one movement seen twice, and both legs must be excluded
  *and* recognisable as a pair — which is what `ARCHITECTURE.md` §5.2 describes and change
  2.6 implements by finding the pairs automatically.

Two ways to resolve that, and **task 0.1 has to settle it before section 1 is written**:

1. Seed a non-P&L leaf for internal transfers, so `T` is an ordinary categorisation and 2.6
   later upgrades a guess into a confirmed pair. Cheap, and it stores "somebody said this is
   a transfer" without storing which two rows are the pair.
2. Leave `T` out of this change and let 2.6 own it entirely. Honest, and it means the
   keyboard legend `DESIGN.md` §8 promises is wrong until 2.6 ships.

The design's working assumption is (1), because a promised key that does nothing is worse
than a decision recorded imprecisely — but it adds a category to a seeded taxonomy, which is
migration 005's territory and a change to `eval/build.py`, so it is not free.

The alternative for both — a boolean pair on `transactions` — is rejected: it puts the same
fact in two places and needs every report query to remember both.

## D3 — Grouping and ordering

```sql
SELECT counterparty_key,
       max(counterparty_raw)      AS display_name,
       count(*)                   AS row_count,
       sum(coalesce(base_amount_minor, amount_minor)) AS total_minor
  FROM transactions t
 WHERE NOT EXISTS (SELECT 1 FROM classifications c
                    WHERE c.org_id = t.org_id AND c.transaction_id = t.id
                      AND c.superseded_by IS NULL)
   AND t.entity_id = $1
 GROUP BY counterparty_key
 ORDER BY abs(sum(coalesce(base_amount_minor, amount_minor))) DESC,
          count(*) DESC,
          counterparty_key
```

Three things in that query are decisions.

**`coalesce(base_amount_minor, amount_minor)`** — sorting by amount across currencies is
only meaningful in one currency, and migration 007 stores the converted amount rather than
deriving it. A row with no conversion is already in the base currency, so the coalesce is
exact rather than approximate. Summing unconverted foreign amounts would put a JPY row at
the top of every queue.

**`abs(...)`** — a €40,000 refund matters as much as a €40,000 payment, and sorting signed
puts every expense below every income.

**`counterparty_key` last** — a deterministic tiebreak, so the queue does not reshuffle
between two reads and a user working by keyboard does not have the ground move.

An empty `counterparty_key` — a row whose counterparty could not be identified at all —
groups as one bucket and is sorted like any other. It is not hidden: `WORKFLOW.md`'s
"eighty unknown counterparties" is exactly this case, and a queue that quietly drops the
hardest rows reports a completion it did not achieve.

## D4 — Role enforcement, finally

`memberships.role` has been stored since migration 004 and checked by nothing, on the stated
grounds that role enforcement is Product's and a column read by nothing is still the right
place to put the fact. This is the change where it is read, because it is the first write a
`viewer` must not be able to make.

The check goes in `core/internal/identity`, beside the session resolution that already
reads memberships, and **not** in the RLS policy. RLS is containment, not authorization: it
guarantees a transaction bound to org X touches only X's rows and has no opinion about what
the caller may do within X. A role check in a policy would be the second mechanism deciding
tenancy, which is the thing the architecture spends its effort avoiding.

The failure is a Connect `permission_denied` with a code, not a sentence.

## D5 — Undo reverses, it does not delete

`UndoDecision` supersedes every classification the decision wrote, deletes the vendor-memory
row it created, and stamps `undone_at`/`undone_by`. It does not delete the decision: what a
person did and then reversed is part of the audit trail, and `classifications` is append-only
by grant so the classifications could not be deleted even if that were wanted.

The vendor row *is* deleted rather than superseded, because `vendors` has no supersession
model — it is a lookup, and a stale row would keep answering L0 with a category the user
just took back. `vendors` is the one table in this flow that is not append-only, and that is
a deliberate asymmetry: memory is current state, classifications are history.

## Risks

| Risk | Mitigation |
| --- | --- |
| A decision covers rows the user never saw, because an import landed mid-decision | The covered set is read inside the same transaction and recorded as a count and a sum; the UI shows both before and after |
| Two approvers resolve the same counterparty at once | The live-row unique index on `classifications` makes the second fail rather than double-write |
| The queue is "everything" until the classification worker exists | Stated in the proposal as a blocker; the decision path is testable against hand-inserted classifications and the worker changes none of it |
| `T` and `N` need report semantics that 4.1 has not defined | They resolve to non-P&L categories that already exist in the taxonomy, so 4.1 excludes them by `is_pnl` and needs no new concept |
| Grouping by exact key misses "OOO Ромашка" vs "Ромашка ООО" | `counterparty_key` already normalises legal forms and addresses; fuzzy matching is L3 and Commercial |
