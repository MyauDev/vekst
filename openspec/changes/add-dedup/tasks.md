**Budget.** `docs/IMPLEMENTATION_PLAN.md` §3 allocates **1.5 person-days** to change 2.6.
These tasks total **≈ 13 hours ≈ 1.6 person-days**, which is over. The proposal's
recommended split — §1–§3 as `add-dedup` at 1 day, §4 as `add-internal-transfers` at 0.5 —
is also how this fits the budget honestly rather than by rounding.

**Ordering.** Apply after 2.5. Every comparison here reads a column that change writes.

**Ownership.** Track A throughout, except §5 — `/proto` needs **both reviewers**. `/eval`
is Track B's; §2.2 changes it and needs Track B's review.

**Rule for this change.** No task deletes a row the customer's file contained. Every skip
is recorded with the line it came from. If a task seems to need a delete, it is the wrong
task.

## 0. Decide before writing code

- [ ] 0.1 **Founder decision:** are detected internal transfers excluded from the P&L on detection, or only once confirmed? Design D4 recommends on detection and gives the reason. It is product-visible, and product is his
- [ ] 0.2 Confirm the split in the proposal's scope note with Track B, and record which of the two changes is scheduled first if the Demo runs short
- [ ] 0.4 **Settle `transactions_dedup_idx`.** Migration 007 creates it UNIQUE. Verified on PostgreSQL 16 that this permanently refuses a second genuinely distinct but identically-hashing payment, with no record that it existed. Run the hash over the four Priorbank fixtures and count collisions; then either drop the uniqueness in 007 before it merges, or drop it here in migration 011
- [ ] 0.3 Confirm with 2.3 that the balance check runs over **every parsed row**, before dedup. If it runs after, a skipped duplicate breaks reconciliation and the two changes disagree by construction

## 1. Migration — Track A

- [ ] 1.1 Migration 011 up: the partial unique index `import_batches_file_once` on `(org_id, file_sha256) WHERE status = 'imported'`
- [ ] 1.2 `dedup_skips`, with `UNIQUE (org_id, batch_id, line_no, posting_no)` and `dedup_skips_d3_names_its_original`
- [ ] 1.3 `internal_transfers` with its two check constraints, and `internal_transfer_members` with `PRIMARY KEY (org_id, txn_id)` — the members table is what enforces one pair per transaction, not the pair table (design D5)
- [ ] 1.4 Enable and `FORCE ROW LEVEL SECURITY` on both tables; one `FOR ALL` policy each, `USING` and `WITH CHECK` written out
- [ ] 1.5 `REVOKE UPDATE ON dedup_skips FROM vekst_app`
- [ ] 1.6 The partial index on active (undismissed) transfers
- [ ] 1.7 Migration 011 down; confirm `up → down → up` against a scratch database

## 2. `normalize()` — Track A, reviewed by Track B

- [ ] 2.1 **`core/internal/normalize` already exists** — PR #6 landed it with a `conformance_test.go`. Read it before writing anything: this section is now a review of what is there, not a move
- [ ] 2.2 A checked-in case table — input, normalised output, counterparty key — that both the Go and the Python suites read
- [ ] 2.3 Point `eval/norm.py` at the same table without changing its behaviour; a behaviour change here would change what `eval/run_eval.py` measured and invalidate the D-1 numbers

## 3. D1, D2, D3 — Track A

- [ ] 3.1 D1 in the measurement job: look for an imported batch with this hash, fail as `already_imported` naming it, and let the index be the guarantee (design D1)
- [ ] 3.2 D2: partition a batch's rows by `dedup_hash` before insert; the first occurrence is inserted and the rest are skips with `level = 'D2'`
- [ ] 3.3 D3: look up each remaining hash against the organisation's transactions; a match is a skip with `level = 'D3'`, naming the matched transaction and its batch
- [ ] 3.4 Write insertions and skips in the **same** `db.InTx` as the persist, so a batch is never half-deduplicated
- [ ] 3.5 `GetDedupSummary` derives its counts from `dedup_skips`, never from a stored counter

## 4. Internal transfers — Track A

- [ ] 4.1 Detection: same organisation, equal absolute `base_amount_minor`, opposite signs, different account, within 3 days (design D5)
- [ ] 4.2 Deterministic pairing — nearest by date, then lowest `id`, greedy, each side used once
- [ ] 4.3 Run detection after persist, over the new rows and the window of existing rows they could pair with
- [ ] 4.4 `DismissInternalTransfer`: records who and when, together, and returns the pair to the P&L
- [ ] 4.5 A query that change 4.1 uses to exclude paired transactions from every P&L line. It is unused here — say so in the comment

## 5. Proto — both reviewers

- [ ] 5.1 The four RPCs and `DedupSummary` on the existing `ImportService`
- [ ] 5.2 `buf breaking` — confirm adding RPCs reports nothing
- [ ] 5.3 Confirm no new field carries a bare amount; a skipped row's money is read through `Money`

## 6. Tests — Track A

- [ ] 6.1 **D1 is a guarantee:** two concurrent confirms of one file — one succeeds, one fails as `already_imported`, and the index is what refuses it
- [ ] 6.2 **D1 is scoped:** the same bytes uploaded by two organisations are two imports
- [ ] 6.3 **D1 is partial:** a file whose batch was rejected can be uploaded again after the customer fixes it
- [ ] 6.4 **D2:** a file containing the same row twice imports one and records one skip at the second row's original line
- [ ] 6.5 **D3:** re-uploading last month's export imports only the new rows, and each skip names the transaction and batch it matched
- [ ] 6.6 **A skip does not break reconciliation:** a file whose rows are 90% already imported still passes the balance check, because the check ran over every parsed row (§0.3)
- [ ] 6.7 **Two identical payments in one day are not a duplicate of each other when they are not:** a fixture with two genuinely distinct coffees — differing only in `bank_ref` — imports both
- [ ] 6.8 **And when they are indistinguishable, the loss is recorded:** a fixture with no `bank_ref` skips the second and `dedup_skips` names its line, so the customer can find it
- [ ] 6.9 **Skips are never deletes:** no code path in this change removes a transaction
- [ ] 6.10 **Transfer detection:** an out and an in, equal, two days apart, different accounts, one organisation, produce one pair
- [ ] 6.11 **Across currencies:** a PLN-to-EUR transfer between the customer's own accounts pairs on the base amount, and the pair is unchanged by a later import carrying a different rate
- [ ] 6.12 **Within one source kind:** a ledger row and a bank row are never paired as an internal transfer — that is D4's job, and D4 is Product
- [ ] 6.13 **Deterministic:** three equal transfers in one week produce the same pairing whatever order the rows arrive in
- [ ] 6.13b **One pair per transaction, on either side:** a transaction that is the `in` side of a pair cannot become the `out` side of another. This is the case two separate unique constraints let through, so assert it directly against the database
- [ ] 6.14 **One `normalize()`:** the Go and Python suites both pass the case table, and a deliberate change to one fails the other
- [ ] 6.15 **Cross-tenant isolation:** A cannot read or dismiss B's skips or transfers, and the failure is a policy denial rather than a not-found
- [ ] 6.16 **Fail-closed:** every query added here raises `42704` outside a tenant transaction

## 7. Front end — Track A

- [ ] 7.1 A typed client for the four RPCs in `web/src/data`
- [ ] 7.2 i18n keys for `already_imported`, the two skip levels and the transfer states, in `en` and `ru`

## 8. Close

- [ ] 8.1 Add `/core/internal/dedup/` and the new query files to `.github/CODEOWNERS` under Track A
- [ ] 8.2 Record the §0.1 decision in `docs/ARCHITECTURE.md` §5.2, which currently supports both readings
- [ ] 8.3 Record in `docs/ARCHITECTURE.md` §5 that `dedup_hash` has ordinary false positives and that skips are therefore recorded, not counted
- [ ] 8.4 Update the capability spec and run the full suite
