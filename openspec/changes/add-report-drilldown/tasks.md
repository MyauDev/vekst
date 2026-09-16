**Budget.** `docs/IMPLEMENTATION_PLAN.md` §3 allocates **1.5 person-days** to change 4.2.
These tasks total **≈ 11 hours ≈ 1.4 person-days**, and that holds only if `line_no` lands
in migration 007 before it merges. If it needs its own migration and a backfill, add half a
day and keep section 1.

**Ordering.** Apply after 4.1. Section 2 is the whole of the risk.

**Ownership.** Track B. `/proto/vekst/v1` and `/core/internal/db` need both reviewers.

## 0. Settle with Track A

- [x] 0.1 **It did not land in 007**, which merged first. A goose migration that has run somewhere is history, and editing history is how two databases claiming the same version stop having the same schema — so section 1 stands, as migration 015, and the budget note's extra half-day is what it cost
- [x] 0.2 **Confirmed lost, at one line.** `buildTransaction` copied every other field of `ingest.Row` across and not this one, so `dedup.Candidate` carried `LineNo` beside a `ledger.Transaction` that had nowhere to put it. `Transaction.LineNo` now exists, the insert names the column, and `ledger.Insert` refuses a row without one *by name* before the CHECK sees it — the boundary that lost it is the one that reports it

## 1. Migration 015 — because 0.1 did not happen

- [x] 1.1 `transactions.line_no integer NOT NULL`, beside `batch_id`: the two together are the row's provenance and either alone is half an answer. **NOT NULL with no backfill**, and the migration refuses rather than invents — no join recovers the line for a row written before it (a transaction has no pointer to its `raw_rows` line, dedup means not every line became a transaction, and one line can become several postings), so the alternatives were a sentinel or a guess and both put a number in a column a customer reads as a line in their own file
- [x] 1.1b A `NO FORCE` window around the emptiness assertion. `transactions` is FORCE'd, so `app_current_org()` raises 42704 in a migration that binds to no tenant — and binding to one organisation would answer a different question and answer it wrongly, because it is precisely the rows belonging to somebody else that this must not miss
- [x] 1.2 Down, `up → down → up`, and the two tripwires

## 2. The shared predicate — `core/internal/report`

- [x] 2.1 One builder producing the selection for `(entity_id, basis, period, line)`, used by the report sum and by the drill-down (design §D1). `report.SelectionFor`, and what it really shares is the **date clipping**: a quarterly report starting in February has a Q1 column covering February and March, and a drill-down deriving its own range from the label would read January too and return rows the figure never counted. Both paths reach dates through one `monthRange`
- [x] 2.2 `LineRef` covering both a category code and the four exclusion buckets
- [x] 2.3 A computed line resolves to its operand lines rather than to transactions (design §D2)
- [x] 2.4 Cursor paging on `(booked_on, id)` (design §D6)

## 3. Queries — `core/internal/db/query/drilldown.sql`

- [x] 3.1 `LineTransactions`: the rows behind one cell, with the classification's layer, confidence and evidence joined
- [x] 3.2 The four buckets — **inside the same query**, not beside it. `@line` discriminates in a CASE written in the order `Compute` places a row, so the rule has one home and a reader compares two adjacent things rather than five files. Verified by mutation: dropping `NOT requires_allocation` from the section arm makes task 5.1 fail, naming both the cell and the count
- [x] 3.3 `ReconciliationForPeriod` plus `OpeningBalanceBefore`, read at the N+1 boundaries between N periods so each closing *is* the next opening. Design §D5 expected the identity to hold trivially; it does not, because the three movements come from one aggregation with FILTER clauses and the two balances from a different sum over a different predicate — so a row dropped or double-counted breaks it. What it still cannot check is whether the rows match the bank, and `derived` says so
- [x] 3.4 Run `make gen`

## 4. Proto and handler

- [x] 4.1 `ListLineTransactions(organization_id, entity_id, basis, from, to, line, cursor)` in `report.proto`
- [x] 4.2 Each row carries `engine_layer`, `confidence`, `evidence`, `decided_by`, `line_no`, `batch_id` and the counterparty as the statement spelled it
- [x] 4.3 `matched_rule_priority` is **not** on the wire (design §D3)
- [x] 4.4 Money is `vekst.type.v1.Money`; `confidence` is the only `double`
- [x] 4.5 The response says whether it answered with transactions or with operand lines
- [x] 4.6 `buf lint`, `buf breaking`, `make gen`
- [x] 4.7 Any member may read, `viewer` included

## 5. Tests

- [x] 5.1 **The drill-down adds up to the figure**, for every non-zero cell of a fixture report. This is the test the change exists for
- [x] 5.2 **A computed line returns operands**, and each operand opens to transactions
- [x] 5.3 **Cross-tenant isolation:** A cannot open a cell of B's report, and an entity id belonging to B returns empty rather than an error that reveals it exists
- [x] 5.4 **Layer and evidence are what was stored**, including a `human` row carrying its decider
- [x] 5.5 **A retracted classification's row appears in the unclassified bucket**, not on its old line
- [x] 5.6 **`line_no` round-trips** from the parsed file to the drill-down, asserted against a committed fixture
- [x] 5.7 **Paging:** a cursor over a thousand rows neither skips nor repeats one
- [x] 5.8 **Reconciliation:** the identity holds on the shapes most likely to break it — a transfer pair with one leg inside the period and one outside, a dismissed pair whose legs go back to being ordinary movement, and an empty month where every term is zero. A *deliberately* broken fixture is not among them, and the reason is worth stating: the identity compares the system with itself, so no input makes a correct implementation disagree — only a bug does, which is what the awkward shapes are there to find
- [x] 5.9 **Non-base-currency:** a JPY row shows its own amount and contributes its base amount
- [x] 5.10 **No-float:** no money field in the drill-down types is floating point

## 6. Close

- [x] 6.1 `docs/IMPLEMENTATION_PLAN.md` §3 with the actual cost
- [x] 6.2 `ARCHITECTURE.md` §5.5 if `line_no` landed here rather than in 007
- [x] 6.3 Update the capability spec and run the full suite. Five requirements added from what the implementation settled: the clipped column, a refused non-cell, both amounts on a foreign row, no float, and provenance being complete or the row not being written

## 7. What this change settled that its own design did not foresee

- [x] 7.1 **`line_no` cost a migration, not a column.** The budget note allowed for it and it is still worth naming what made it expensive: not writing the migration, but deciding what a row written before it should say. The answer is nothing, and the migration refusing to run is how it says so
- [x] 7.2 **`ledger.Insert` validates provenance in Go.** The CHECK catches a zero too, but it catches it as a constraint violation at the end of a batch; this catches it at the boundary that lost it, by name, with the sentence explaining why the field is not optional
- [x] 7.3 **The purity guard grew a second half.** `selection.go` needs `time` for calendar arithmetic and must not read a clock, and an import list cannot tell those apart — so the test now walks the AST for `time.Now`, `os.Getenv` and friends, and a third test fails when a file in the package is neither guarded nor known to hold the reads
