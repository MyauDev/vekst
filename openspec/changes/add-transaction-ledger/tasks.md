**Budget.** `docs/IMPLEMENTATION_PLAN.md` §3 allocates **2 person-days** to change 2.5.
These tasks total **≈ 15 hours ≈ 2 person-days**, which fits — but only because ingest,
dedup, FX fetching and the classification worker are all explicitly out of scope. If any of
them creeps in, the estimate is wrong rather than tight.

**Ordering.** Apply after 3.2. Task 1 is the whole of the risk; everything after it is
queries and proof.

**Ownership.** The plan puts 2.5 on Track A. This change takes the table and leaves the
upload — see the proposal's assumption. `/core/internal/db` needs both reviewers.

## 1. Migration 007 — the tables

- [x] 1.1 `import_batches`, minimal: `org_id`-leading primary key, composite FK to `entities`, `source_kind` and a `status` CHECK admitting one value
- [x] 1.2 `transactions` with the grain CHECK (`document_ref` null ⟺ `posting_no` 0), composite FKs to `entities`, `accounts` and `import_batches`
- [x] 1.3 Money: `amount_minor`/`currency` plus the four FX columns, with the all-or-nothing CHECK and the "a conversion converts" CHECK (design §D2)
- [x] 1.4 `normalize_version`, `description_norm`, `counterparty_key`, `regulated_code` — the columns change 3.2 specified (design §D3)
- [x] 1.5 `dedup_hash` NOT NULL with its unique index, though nothing computes it yet (design §D5)
- [x] 1.6 Constraint trigger: a row's `source_kind` equals its batch's (design §D1)
- [x] 1.7 `classifications`, append-only, with all four version columns and the partial unique index on the live row (design §D4) — **corrected during 3's implementation**: a plain unique index is checked immediately, which makes the insert-then-point write pattern impossible without a race (verified directly: neither statement order nor a single combined multi-CTE statement is safe — Postgres documents that a statement's own data-modifying CTEs may run in an unspecified relative order, and this was confirmed to actually fail that way, not merely suspected to). Replaced with a plain (non-unique) index for the query plus a `DEFERRABLE INITIALLY DEFERRED` constraint trigger, `classifications_one_live_per_transaction`, checked once at commit — the same shape this migration already uses three times for `txn_source_kind_matches_batch`, `txn_base_currency_is_the_orgs` and `classifications_category_is_visible`
- [x] 1.8 Constraint trigger on `classifications.category_id`: visible, a leaf, and not computed — the same function migration 006 uses, extended or reused
- [x] 1.9 Grants: `vekst_app` gets INSERT and SELECT on `classifications`, UPDATE on `superseded_by` alone, and no DELETE
- [x] 1.10 RLS: enable and `FORCE` on all three; ordinary tenant policies, no shared rows
- [x] 1.11 Migration 007 down, and `up → down → up` against a scratch database
- [x] 1.12 Bump the tripwires: `assertTableCount` and `RequiredVersion`

## 2. Generated queries — `core/internal/db/query/ledger.sql`

- [x] 2.1 `InsertTransaction`, taking every column the grain and the money rules require
- [x] 2.2 `UnclassifiedTransactions`: a page of rows with no live classification, ordered by `booked_on`, for the worker a later change writes
- [x] 2.3 `InsertClassification` and `SupersedeClassification` — two statements, because a correction is an insert plus a pointer and never an update. Made safe by 1.7's correction: `SupersedeClassification` is a plain, column-scoped `UPDATE`, and the deferred constraint trigger — not statement combination — is what makes the moment between the two statements safe
- [x] 2.4 `CurrentClassification` for one transaction, and `TransactionsForReport` filtered by `source_kind`, entity and date range
- [x] 2.5 Run `make gen`; confirm the codegen drift job stays green — ran `go tool sqlc generate` directly (the sqlc step of `make gen`) plus `buf lint`/`buf breaking`, since this section touches no `.proto`

## 3. `core/internal/ledger` — the typed seam

- [x] 3.1 `Transaction` and `Classification` domain types, money as `money.Money` in both directions — `Transaction.Amount` plus `Transaction.FX.Base` when converted (`ledger.go`)
- [x] 3.2 `DedupHash(txn, occurrence)` — the content hash of design §D5 including the occurrence term, with the field set written down in one place — `dedup.go`; occurrence-counting within a batch is the caller's job, kept out of `Insert` so the field set stays defined in exactly one function
- [x] 3.3 `Insert(ctx, tx, []Transaction)` inside `db.InTx`, never taking a pool — `insert.go`; takes `pgx.Tx` directly, and writes whatever `DedupHash` each `Transaction` already carries rather than computing one
- [x] 3.4 A `ToClassifyBatch` helper turning a page of transactions into `classify.BatchRequest`, so the worker that a later change writes has nothing to invent — `classify.go`; returns `[]classify.Txn`, the part of `BatchRequest` a `Transaction` alone can answer (categories, rules, vendors and the three pinned versions come from elsewhere)

## 4. Tests

- [x] 4.1 **Cross-tenant isolation, `transactions`:** A cannot read, update or delete B's rows, and the outcome is indistinguishable from the row not existing — `TestTransactionCrossTenantIsolation`
- [x] 4.2 **Cross-tenant isolation, `classifications`:** the same, including that A cannot classify B's transaction — `TestClassificationCrossTenantIsolation`
- [x] 4.3 **The grain:** a bank row with a `document_ref` and `posting_no` 0 is rejected; a ledger document's five postings share one `document_ref` and are accepted — `TestGrainConstraintHolds`
- [x] 4.4 **Non-base-currency:** a JPY row (exponent 0) and a KWD row (exponent 3) round-trip through Go with no rounding, and their base amounts are stored rather than derived — `TestNonBaseCurrencyRoundTripsWithNoRounding`
- [x] 4.5 **No-float:** a reflection test failing if any money column reaches Go as a float — already satisfied by `core/internal/money`'s existing `TestNoFloatMoneyFields`, which walks all of `/core` and therefore this package too; no new test needed, and `Classification.Confidence float64` does not match its money-field regex because it names a probability, not an amount
- [x] 4.6 **FX all-or-nothing:** each of the fifteen partial states is rejected, and both complete states accepted — `TestFXIsAllOrNothing`. **Corrected the count**: four nullable columns admit **sixteen** states (2⁴), of which two are valid (all null, or all four set), leaving **fourteen** invalid partial ones, never fifteen — the design doc's original arithmetic, already corrected in migration 007's own inline comment; this task's wording is corrected here too
- [x] 4.7 **Append-only:** `vekst_app` cannot UPDATE a classification's category or DELETE a row; superseding one works and leaves exactly one live row — `TestClassificationsAreAppendOnly`. This is the test that first surfaced 1.7's bug: an earlier, two-statement `SupersedeClassification` against the original plain unique index failed here every time, deterministically, which is what prompted the migration correction rather than a workaround in this test
- [x] 4.8 **One live classification:** two live rows for one transaction violate the partial unique index — `TestOnlyOneLiveClassificationPerTransaction`. Now a deferred constraint trigger rather than a plain index (1.7); the test asserts the same outward SQLSTATE (`23505`) either way
- [x] 4.9 **`source_kind` cannot drift** from its batch's, and the trigger says so — `TestSourceKindCannotDriftFromItsBatch`
- [x] 4.10 **A classification cannot target a section or a computed line**, with the same error as migration 006 raises — `TestAClassificationCannotTargetASectionOrComputedLine`
- [x] 4.11 **`dedup_hash`**: the same content at the same occurrence hashes the same; two rows differing in one field differ; and two genuinely identical payments in one batch are both stored rather than one being rejected (design §D5) — `TestDedupHash`, `TestTwoIdenticalPaymentsInOneBatchAreBothStored`
- [x] 4.12 **RLS coverage** still passes with three new tables and no new allowlist entries — confirmed against the live schema (`TestRLSCoverageOfTheRealSchema`); no code change needed, since the coverage test scans every table not on the allowlist rather than naming these three

## 5. Close

- [x] 5.1 Add `/core/internal/ledger/` to `CODEOWNERS`, and migration 007 alongside 004–006
- [x] 5.2 Update `docs/IMPLEMENTATION_PLAN.md` §3 with what landed and what 2.1 still owns
- [x] 5.3 Amend `ARCHITECTURE.md` §5.5 where the sketch and the migration differ, the way 3.1 did for `categories` — found no trace that 3.1 actually did this for `categories` either (its sketch line is still simplified), so this corrects `transactions`/`classifications` to migration 007's real columns and adds a callout paragraph, the same shape the existing `add-tenancy-and-rls` callout already uses
- [x] 5.4 Propose the classification-run change this one unblocks — the worker, its chunking, and the retry behaviour §3.5 already specifies — `openspec/changes/add-classification-run/` (proposal, design, specs, tasks; validated). Stacked on this change, `add-classification-engine`, and — for its own trigger, `import_batches.status = 'imported'` — on `add-dedup`, not yet implemented
- [x] 5.5 Run the full suite
