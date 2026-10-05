## Why

Change 3.1 supplies the categories. This supplies the thing that chooses one, and the
contract it is chosen over. Without it the review queue has nothing to review and the P&L
has nothing to sum.

The engine already exists and is measured: `eval/engine.py` classifies 79.1% of 4,508 real
transactions across three countries, disagreeing with the accountant's own labelling on 20
of them. This change moves that code into the service, puts its 71 rules in a table, and
freezes the wire format it answers over.

Milestone: **Demo**. Capabilities: **`classification-engine`** (new) and
**`classification-taxonomy`** (extended with rules and vendor memory).

## What Changes

- **`ClassifyBatch` enters `vekst.internal.v1`.** Change 0.1 left the contract holding only
  `Version`, with a test that fails if anything else appears — deliberately, so it could not
  be widened by accident. This change removes that stop and replaces it with the real shapes.
- Migration 006 creates `classification_rules` (seeded with the 71 template rules from
  `eval/out/seed_rules.sql`) and `vendors`, the per-organisation memory the review queue
  fills in change 3.3.
- **The engine moves to `classifier/`**, ported from `eval/engine.py`, which stays as the
  reference the harness measures. Layers L0, L0.5 and L1 ship; L2 does not — see Non-goals.
- **Normalisation moves to `core`, in Go.** It belongs to ingest, not to classification:
  `transactions.counterparty_key` is a stored column and `dedup_hash` already depends on it.
  The classifier receives values already normalised and matches them exactly. §D2 records
  what Track A must add to change 2.5 for that to hold, and a conformance test asserts the Go
  and Python implementations agree on the committed fixtures.
- `regulated_code` generalises КНП, `Typ operacji` and a 1C account number into one field, so
  L0.5 stops being Kazakhstan-shaped.

## Non-goals

- **No persistence of results.** `classifications` needs `transactions`, which change 2.5
  has not created. The RPC answers; nothing stores the answer yet.
- **No L2 and no L4.** L2's input is the wizard's industry profile, which ships at
  Commercial; the slot has no source before then. L4 is Intelligence.
- **No review queue.** 3.3 fills `vendors`; this change only reads it.
- **No Poland-specific parsing.** The template exists; the parser is Track A's.

## Impact

Touches `/proto/vekst/internal/v1` (**both reviewers**, and `buf breaking` runs),
`/classifier`, `/core/classify`, `/core/internal/db`, `/core/migrations`. Apply after 3.1.
