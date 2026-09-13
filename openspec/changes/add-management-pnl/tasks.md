**Budget.** `docs/IMPLEMENTATION_PLAN.md` §3 allocates **2 person-days** to change 4.1.
These tasks total **≈ 17 hours ≈ 2.2 person-days**. The overrun is section 0: the sign
convention and the generated `direction` column are not 4.1's work by rights, but the report
cannot be written while `sum(amount_minor)` over a cost section has no defined sign.

**Ordering.** Apply after 2.5. Sections 2 and 3 need no database and can be written first —
that is the point of design §D5.

**Ownership.** Track B. `/proto/vekst/v1` and `/core/internal/db` need both reviewers.
Section 0 touches migration 007, which is Track A's to merge.

## 0. Settle with Track A before writing SQL

- [x] 0.1 **The sign convention** (design §D3): `amount_minor` is signed, money in positive. 2.6 already assumes it and the review queue already sums that way; write it into `ARCHITECTURE.md` §5.0 so the third change does not have to rediscover it
- [x] 0.2 **`direction` as a generated column**, with the vocabulary `income`/`expense` and not `in`/`out`. Track A's `SUPERSEDED.md` amendment 3 proposes the column and the wrong values; all 71 seeded rules, the classifier's field switch and the proto comment use the other pair
- [ ] 0.3 **Migration numbering.** `add-file-upload` claims 008, which `00008_review_decisions.sql` holds. Agree the order for 008–012 before either branch merges
- [x] 0.4 Tell Track A that `transactions_dedup_idx` is already answered: 2.5's design §D5 puts the occurrence index of the content inside the hash, so two identical payments differ and a re-imported file still collides. Their 2.6 task 0.4 and `SUPERSEDED.md` both still read it as open
- [ ] 0.5 Close **D-3**, the output table structure. It is this change's shape and it is overdue

## 1. Migration 009 — `pnl_section`

- [x] 1.1 Fill `pnl_section` on all 46 seeded rows from the level-1 ancestor (design §D2)
- [x] 1.2 A constraint trigger: a new row's section equals its parent's; a level-1 row is its own
- [x] 1.3 `NOT NULL` once filled, so a row that names no section cannot be written
- [x] 1.4 Down, `up → down → up`, and the two tripwires

## 2. The calculation — `core/internal/report`, no database

- [x] 2.1 `Row`, `Spec` and `Report` types; money as `money.Money` throughout
- [x] 2.2 The computed-line table of design §D1, over codes, with the stored `formula` carried as a label
- [x] 2.3 `Compute(rows, spec)`: section totals, the five-line chain, Total and % of revenue
- [x] 2.4 Costs print positive; the one inversion lives here and nowhere else (design §D3)
- [x] 2.5 The four exclusion buckets of design §D6, each in money
- [x] 2.6 Periods from the range and granularity, with empty periods present as zero columns (design §D7)
- [x] 2.7 `% of revenue` is a ratio, not money: a float here is correct, and a test asserts it is the only one

## 3. Tests for the calculation

- [x] 3.1 **The chain:** a fixture with one row in each section, asserting GM, NM, CM, IBT and NI against hand-computed values
- [x] 3.2 **The formula table matches the taxonomy:** every name in every stored `formula` resolves to a category, and names the operands the table uses
- [x] 3.3 **Costs are positive on the page and negative in the store**, proved in both directions
- [x] 3.4 **Non-base-currency:** a JPY row (exponent 0) and a KWD row (exponent 3) contribute their base amounts and nothing rounds
- [x] 3.5 **No-float:** every money field in the report types is `money.Money`; `% of revenue` is the sole exception and is named in the test
- [x] 3.6 **Division by zero:** % of revenue with no revenue is absent, not infinity and not zero
- [x] 3.7 **An empty period is a zero column**, distinguishable from a period that was not requested
- [x] 3.8 **The exclusion buckets sum with the report to the whole:** every row given to `Compute` lands in exactly one of a line or a bucket, asserted as an identity

## 4. Queries — `core/internal/db/query/report.sql`

- [ ] 4.1 `ClassifiedRowsForPeriod`: live classifications joined to transactions, filtered by entity, source kind and date range, returning the category code and the base amount
- [ ] 4.2 `UnclassifiedTotalForPeriod` and the three other bucket queries of design §D6
- [ ] 4.3 `OtherBasisTotal`: what the report is not computed from, for §D4's reconciliation note
- [ ] 4.4 Every read filters on one `source_kind`; a test asserts no query in this file omits it
- [ ] 4.5 Run `make gen`

## 5. Proto and handler

- [ ] 5.1 `proto/vekst/v1/report.proto`: `GetManagementPNL(organization_id, entity_id, from, to, granularity, basis)`
- [ ] 5.2 The response carries the pinned `taxonomy_version`, `ruleset_version` and `engine_version` — the three strings that make a March report reproduce in June
- [ ] 5.3 Money is `vekst.type.v1.Money`; the only `double` is `percent_of_revenue`
- [ ] 5.4 `buf lint`, `buf breaking`, `make gen`
- [ ] 5.5 The handler binds the organisation through the review service's pattern — the tenancy binding lives in the service, because `core/internal/server` may not import `core/internal/db`
- [ ] 5.6 Any member may read a report, `viewer` included

## 6. Tests against the database

- [ ] 6.1 **Cross-tenant isolation:** organisation A's report contains no row of B's, and an entity id belonging to B returns an empty report rather than an error that says it exists
- [ ] 6.2 **Only live classifications count:** a superseded one and a retracted one are both absent, and the retracted row appears in the unclassified bucket instead
- [ ] 6.3 **One basis:** a ledger row and a bank row for the same invoice do not both reach a line
- [ ] 6.4 **The versions on the response are the versions on the rows**, not constants read from the binary
- [ ] 6.5 **End to end:** classify through the review queue, then print the report, and assert the decided amount is on the line the decision named

## 7. Close

- [ ] 7.1 `CODEOWNERS`: `/core/internal/report/` and migration 009
- [ ] 7.2 `ARCHITECTURE.md` §5.0 gains the sign convention; §5.5 notes `pnl_section` is populated
- [ ] 7.3 `docs/IMPLEMENTATION_PLAN.md` §3 with the actual cost, and D-3 and D-4 closed
- [ ] 7.4 Update the capability spec and run the full suite
