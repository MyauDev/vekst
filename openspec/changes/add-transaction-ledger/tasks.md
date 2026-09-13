**Budget.** `docs/IMPLEMENTATION_PLAN.md` §3 allocates **2 person-days** to change 2.5.
These tasks total **≈ 15 hours ≈ 2 person-days**, which fits — but only because ingest,
dedup, FX fetching and the classification worker are all explicitly out of scope. If any of
them creeps in, the estimate is wrong rather than tight.

**Ordering.** Apply after 3.2. Task 1 is the whole of the risk; everything after it is
queries and proof.

**Ownership.** The plan puts 2.5 on Track A. This change takes the table and leaves the
upload — see the proposal's assumption. `/core/internal/db` needs both reviewers.

## 1. Migration 007 — the tables

- [ ] 1.1 `import_batches`, minimal: `org_id`-leading primary key, composite FK to `entities`, `source_kind` and a `status` CHECK admitting one value
- [ ] 1.2 `transactions` with the grain CHECK (`document_ref` null ⟺ `posting_no` 0), composite FKs to `entities`, `accounts` and `import_batches`
- [ ] 1.3 Money: `amount_minor`/`currency` plus the four FX columns, with the all-or-nothing CHECK and the "a conversion converts" CHECK (design §D2)
- [ ] 1.4 `normalize_version`, `description_norm`, `counterparty_key`, `regulated_code` — the columns change 3.2 specified (design §D3)
- [ ] 1.5 `dedup_hash` NOT NULL with its unique index, though nothing computes it yet (design §D5)
- [ ] 1.6 Constraint trigger: a row's `source_kind` equals its batch's (design §D1)
- [ ] 1.7 `classifications`, append-only, with all four version columns and the partial unique index on the live row (design §D4)
- [ ] 1.8 Constraint trigger on `classifications.category_id`: visible, a leaf, and not computed — the same function migration 006 uses, extended or reused
- [ ] 1.9 Grants: `vekst_app` gets INSERT and SELECT on `classifications`, UPDATE on `superseded_by` alone, and no DELETE
- [ ] 1.10 RLS: enable and `FORCE` on all three; ordinary tenant policies, no shared rows
- [ ] 1.11 Migration 007 down, and `up → down → up` against a scratch database
- [ ] 1.12 Bump the tripwires: `assertTableCount` and `RequiredVersion`

## 2. Generated queries — `core/internal/db/query/ledger.sql`

- [ ] 2.1 `InsertTransaction`, taking every column the grain and the money rules require
- [ ] 2.2 `UnclassifiedTransactions`: a page of rows with no live classification, ordered by `booked_on`, for the worker a later change writes
- [ ] 2.3 `InsertClassification` and `SupersedeClassification` — two statements, because a correction is an insert plus a pointer and never an update
- [ ] 2.4 `CurrentClassification` for one transaction, and `TransactionsForReport` filtered by `source_kind`, entity and date range
- [ ] 2.5 Run `make gen`; confirm the codegen drift job stays green

## 3. `core/internal/ledger` — the typed seam

- [ ] 3.1 `Transaction` and `Classification` domain types, money as `money.Money` in both directions
- [ ] 3.2 `DedupHash(txn)` — the content hash of design §D5, with the field set written down in one place
- [ ] 3.3 `Insert(ctx, tx, []Transaction)` inside `db.InTx`, never taking a pool
- [ ] 3.4 A `ToClassifyBatch` helper turning a page of transactions into `classify.BatchRequest`, so the worker that a later change writes has nothing to invent

## 4. Tests

- [ ] 4.1 **Cross-tenant isolation, `transactions`:** A cannot read, update or delete B's rows, and the outcome is indistinguishable from the row not existing
- [ ] 4.2 **Cross-tenant isolation, `classifications`:** the same, including that A cannot classify B's transaction
- [ ] 4.3 **The grain:** a bank row with a `document_ref` and `posting_no` 0 is rejected; a ledger document's five postings share one `document_ref` and are accepted
- [ ] 4.4 **Non-base-currency:** a JPY row (exponent 0) and a KWD row (exponent 3) round-trip through Go with no rounding, and their base amounts are stored rather than derived
- [ ] 4.5 **No-float:** a reflection test failing if any money column reaches Go as a float
- [ ] 4.6 **FX all-or-nothing:** each of the fifteen partial states is rejected, and both complete states accepted
- [ ] 4.7 **Append-only:** `vekst_app` cannot UPDATE a classification's category or DELETE a row; superseding one works and leaves exactly one live row
- [ ] 4.8 **One live classification:** two live rows for one transaction violate the partial unique index
- [ ] 4.9 **`source_kind` cannot drift** from its batch's, and the trigger says so
- [ ] 4.10 **A classification cannot target a section or a computed line**, with the same error as migration 006 raises
- [ ] 4.11 **`dedup_hash` is stable**: the same content hashes the same, and two rows differing in one field do not collide
- [ ] 4.12 **RLS coverage** still passes with three new tables and no new allowlist entries

## 5. Close

- [ ] 5.1 Add `/core/internal/ledger/` to `CODEOWNERS`, and migration 007 alongside 004–006
- [ ] 5.2 Update `docs/IMPLEMENTATION_PLAN.md` §3 with what landed and what 2.1 still owns
- [ ] 5.3 Amend `ARCHITECTURE.md` §5.5 where the sketch and the migration differ, the way 3.1 did for `categories`
- [ ] 5.4 Propose the classification-run change this one unblocks — the worker, its chunking, and the retry behaviour §3.5 already specifies
- [ ] 5.5 Run the full suite
