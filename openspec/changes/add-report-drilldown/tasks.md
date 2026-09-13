**Budget.** `docs/IMPLEMENTATION_PLAN.md` §3 allocates **1.5 person-days** to change 4.2.
These tasks total **≈ 11 hours ≈ 1.4 person-days**, and that holds only if `line_no` lands
in migration 007 before it merges. If it needs its own migration and a backfill, add half a
day and keep section 1.

**Ordering.** Apply after 4.1. Section 2 is the whole of the risk.

**Ownership.** Track B. `/proto/vekst/v1` and `/core/internal/db` need both reviewers.

## 0. Settle with Track A

- [ ] 0.1 **`line_no` in migration 007**, before it merges — Track A's own `SUPERSEDED.md` amendment 1 asks for it. One column on an empty table beats a backfill of every transaction. If it lands there, delete section 1
- [ ] 0.2 Confirm 2.5's persist path carries `ingest.Row.LineNo` through rather than dropping it, which is where it is lost today

## 1. Migration 010 — only if 0.1 does not happen

- [ ] 1.1 `transactions.line_no integer NOT NULL`, beside `batch_id`: the two together are the row's provenance and either alone is half an answer
- [ ] 1.2 Down, `up → down → up`, and the two tripwires

## 2. The shared predicate — `core/internal/report`

- [ ] 2.1 One builder producing the selection for `(entity_id, basis, period, line)`, used by the report sum and by the drill-down (design §D1)
- [ ] 2.2 `LineRef` covering both a category code and the four exclusion buckets
- [ ] 2.3 A computed line resolves to its operand lines rather than to transactions (design §D2)
- [ ] 2.4 Cursor paging on `(booked_on, id)` (design §D6)

## 3. Queries — `core/internal/db/query/drilldown.sql`

- [ ] 3.1 `LineTransactions`: the rows behind one cell, with the classification's layer, confidence and evidence joined
- [ ] 3.2 `BucketTransactions` for unclassified, non-P&L, unallocated and other-basis
- [ ] 3.3 `ReconciliationForPeriod`: in, out and transfers, with opening and closing derived (design §D5)
- [ ] 3.4 Run `make gen`

## 4. Proto and handler

- [ ] 4.1 `ListLineTransactions(organization_id, entity_id, basis, from, to, line, cursor)` in `report.proto`
- [ ] 4.2 Each row carries `engine_layer`, `confidence`, `evidence`, `decided_by`, `line_no`, `batch_id` and the counterparty as the statement spelled it
- [ ] 4.3 `matched_rule_priority` is **not** on the wire (design §D3)
- [ ] 4.4 Money is `vekst.type.v1.Money`; `confidence` is the only `double`
- [ ] 4.5 The response says whether it answered with transactions or with operand lines
- [ ] 4.6 `buf lint`, `buf breaking`, `make gen`
- [ ] 4.7 Any member may read, `viewer` included

## 5. Tests

- [ ] 5.1 **The drill-down adds up to the figure**, for every non-zero cell of a fixture report. This is the test the change exists for
- [ ] 5.2 **A computed line returns operands**, and each operand opens to transactions
- [ ] 5.3 **Cross-tenant isolation:** A cannot open a cell of B's report, and an entity id belonging to B returns empty rather than an error that reveals it exists
- [ ] 5.4 **Layer and evidence are what was stored**, including a `human` row carrying its decider
- [ ] 5.5 **A retracted classification's row appears in the unclassified bucket**, not on its old line
- [ ] 5.6 **`line_no` round-trips** from the parsed file to the drill-down, asserted against a committed fixture
- [ ] 5.7 **Paging:** a cursor over a thousand rows neither skips nor repeats one
- [ ] 5.8 **Reconciliation:** the identity holds on a fixture, and a deliberately broken one is reported rather than printed
- [ ] 5.9 **Non-base-currency:** a JPY row shows its own amount and contributes its base amount
- [ ] 5.10 **No-float:** no money field in the drill-down types is floating point

## 6. Close

- [ ] 6.1 `docs/IMPLEMENTATION_PLAN.md` §3 with the actual cost
- [ ] 6.2 `ARCHITECTURE.md` §5.5 if `line_no` landed here rather than in 007
- [ ] 6.3 Update the capability spec and run the full suite
