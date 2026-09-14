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

- [x] 0.1 **Founder decision:** are detected internal transfers excluded from the P&L on detection, or only once confirmed? Design D4 recommends on detection and gives the reason. It is product-visible, and product is his — confirmed: on detection
- [x] 0.2 Confirm the split in the proposal's scope note with Track B, and record which of the two changes is scheduled first if the Demo runs short — confirmed: kept as one change (`add-dedup`), same engineering scope either way; not worth the bookkeeping split for a two-person team mid-Demo
- [x] 0.4 **Settle `transactions_dedup_idx`.** Migration 007 creates it UNIQUE. Verified on PostgreSQL 16 that this permanently refuses a second genuinely distinct but identically-hashing payment, with no record that it existed. Run the hash over the four Priorbank fixtures and count collisions; then either drop the uniqueness in 007 before it merges, or drop it here in migration 011 — ran the *occurrence-aware* hash (design D5 in `add-transaction-ledger`, already shipped) over all four fixtures: 0 collisions, even though two of the four files contain rows whose content genuinely repeats (305/307 and 358/362 distinct content-keys) — occurrence already disambiguates those. Dropped the `UNIQUE` in migration **007** directly (not 011/012 — see the renumbering note below): a hard, permanent, unscoped-by-recoverability constraint is exactly what design D3 rejects, for the coincidence case occurrence does *not* cover (two unrelated real transactions in two unrelated batches landing on the same occurrence)
- [x] 0.3 Confirm with 2.3 that the balance check runs over **every parsed row**, before dedup. If it runs after, a skipped duplicate breaks reconciliation and the two changes disagree by construction — confirmed by construction: `ValidateStatement`/`checkPeriodContinuity` etc. run over `st.Rows` (every parsed row) in `validatejob.go`, entirely before persistence exists in the pipeline; dedup has nothing to skip until this change adds it

**Renumbering.** This proposal's own text says "migration 011" throughout — written before `add-import-profiles` claimed 011 while this change was still open. Numbered **012** here, the same renumbering that change recorded for itself.

## 1. Migration — Track A

- [x] 1.1 Migration 011 up: the partial unique index `import_batches_file_once` on `(org_id, file_sha256) WHERE status = 'imported'` — migration 012; verified against a live database with existing `import_batches` rows present at migration time, not only an empty scratch one
- [x] 1.2 `dedup_skips`, with `UNIQUE (org_id, batch_id, line_no, posting_no)` and `dedup_skips_d3_names_its_original`
- [x] 1.3 `internal_transfers` with its two check constraints, and `internal_transfer_members` with `PRIMARY KEY (org_id, txn_id)` — the members table is what enforces one pair per transaction, not the pair table (design D5)
- [x] 1.4 Enable and `FORCE ROW LEVEL SECURITY` on both tables; one `FOR ALL` policy each, `USING` and `WITH CHECK` written out
- [x] 1.5 `REVOKE UPDATE ON dedup_skips FROM vekst_app`
- [x] 1.6 The partial index on active (undismissed) transfers
- [x] 1.7 Migration 011 down; confirm `up → down → up` against a scratch database — migration 012; verified, including the corrected migration 007's own `up → down → up`

## 2. `normalize()` — Track A, reviewed by Track B

- [x] 2.1 **`core/internal/normalize` already exists** — PR #6 landed it with a `conformance_test.go`. Read it before writing anything: this section is now a review of what is there, not a move — confirmed: `Description`, `CounterpartyKey`, `Version`, all exercised by `TestDescriptionMatchesTheReference`/`TestCounterpartyKeyMatchesTheReference` against `eval/out/normalize_conformance.json`
- [x] 2.2 A checked-in case table — input, normalised output, counterparty key — that both the Go and the Python suites read — already exists (`eval/out/normalize_conformance.json`, produced by `eval/conformance.py`); no new table needed
- [x] 2.3 Point `eval/norm.py` at the same table without changing its behaviour — `eval/norm.py` is the table's own source (`eval/conformance.py` calls it to produce the JSON), so there is no second implementation to point at anything; regenerating and diffing is the checked ceremony a behaviour change already carries

## 3. D1, D2, D3 — Track A

- [x] 3.1 D1 in the measurement job: look for an imported batch with this hash, fail as `already_imported` naming it, and let the index be the guarantee (design D1) — `measure.go`; the backstop (a race the early check narrows but does not close) is caught in `persistjob.go`'s own `isFileOnceViolation`
- [x] 3.2 D2: partition a batch's rows by `dedup_hash` before insert; the first occurrence is inserted and the rest are skips with `level = 'D2'` — **corrected during implementation, confirmed with the founder**: `dedup.Partition` assigns the occurrence-aware hash (design D5 in `add-transaction-ledger`) to every row; two genuinely distinct rows sharing every other field get different occurrences and are *both* kept, never skipped. D2 now only catches the same (content, occurrence) pair appearing twice in one call — a pipeline defect, not a customer's real duplicate — which real data never triggers (verified: zero collisions across all four Priorbank fixtures)
- [x] 3.3 D3: look up each remaining hash against the organisation's transactions; a match is a skip with `level = 'D3'`, naming the matched transaction and its batch — `dedup.FindByHash`, called from `persistjob.go`
- [x] 3.4 Write insertions and skips in the **same** `db.InTx` as the persist, so a batch is never half-deduplicated — one `db.InTx` in `persistWorker.Work`, from account resolution through the final `SetImportBatchStatus`
- [x] 3.5 `GetDedupSummary` derives its counts from `dedup_skips`, never from a stored counter — `ingest.Service.GetDedupSummary`, via `dedup.CountSkips`

## 4. Internal transfers — Track A

- [x] 4.1 Detection: same organisation, equal absolute `base_amount_minor`, opposite signs, different account, within 3 days (design D5) — `dedup.DetectAndPair` / `TransferCandidatesForTransaction`. **Extended during implementation**: also matched on `source_kind` (§6.12's own requirement, missing from the design's literal rule list until this task's own test caught it), and reads `COALESCE(base_amount_minor, amount_minor)` rather than `base_amount_minor` alone — the all-or-nothing FX check guarantees the latter is NULL for every transaction already in the organisation's base currency, which is the common case, not the rare one
- [x] 4.2 Deterministic pairing — nearest by date, then lowest `id`, greedy, each side used once — the query's own `ORDER BY`; `internal_transfer_members`' primary key is the "each side used once" half
- [x] 4.3 Run detection after persist, over the new rows and the window of existing rows they could pair with — `persistjob.go`, once per newly-inserted transaction, in the same `db.InTx`
- [x] 4.4 `DismissInternalTransfer`: records who and when, together, and returns the pair to the P&L — `dedup.Dismiss`
- [x] 4.5 A query that change 4.1 uses to exclude paired transactions from every P&L line. It is unused here — say so in the comment — `PairedTransactionIDsForEntity` / `dedup.PairedTransactionIDs`

## 5. Proto — both reviewers

- [x] 5.1 The four RPCs and `DedupSummary` on the existing `ImportService` — plus `imported_rows` on `DedupSummary`, matching the design's own message shape
- [x] 5.2 `buf breaking` — confirm adding RPCs reports nothing — confirmed clean against `main`
- [x] 5.3 Confirm no new field carries a bare amount; a skipped row's money is read through `Money` — confirmed: `SkippedRow` and `InternalTransfer` carry no amount field at all

## 6. Tests — Track A

- [x] 6.1 **D1 is a guarantee:** two concurrent confirms of one file — one succeeds, one fails as `already_imported`, and the index is what refuses it — `TestD1IsAGuarantee`. Tests the index directly (a second batch forced to `imported` with the first's hash, bypassing the Go-level check entirely) rather than a literal race between two goroutines, which cannot be made deterministic; the assertion is exactly "the index is what refuses it"
- [x] 6.2 **D1 is scoped:** the same bytes uploaded by two organisations are two imports — `TestD1IsScopedToOneOrganisation`
- [x] 6.3 **D1 is partial:** a file whose batch was rejected can be uploaded again after the customer fixes it — `TestD1IsPartial`
- [x] 6.4 **D2:** a file containing the same row twice imports one and records one skip at the second row's original line — **superseded by the §3.2 correction**: `TestTwoIndistinguishableRowsInOneFileBothImport` proves the corrected behaviour (both rows import, zero D2 skips) instead
- [x] 6.5 **D3:** re-uploading last month's export imports only the new rows, and each skip names the transaction and batch it matched — `TestD3SkipsACrossBatchDuplicateAndNamesIt`
- [x] 6.6 **A skip does not break reconciliation:** a file whose rows are 90% already imported still passes the balance check, because the check ran over every parsed row (§0.3) — same test, `TestD3SkipsACrossBatchDuplicateAndNamesIt`
- [x] 6.7 **Two identical payments in one day are not a duplicate of each other when they are not:** a fixture with two genuinely distinct coffees — differing only in `bank_ref` — imports both — `TestTwoRowsDifferingOnlyByBankRefBothImport`
- [x] 6.8 **And when they are indistinguishable, the loss is recorded:** a fixture with no `bank_ref` skips the second and `dedup_skips` names its line, so the customer can find it — **superseded by the §3.2 correction**, the same way 6.4 is: nothing is lost, by design, once dedup_hash carries occurrence. `TestTwoIndistinguishableRowsInOneFileBothImport` is this task's corrected form too
- [x] 6.9 **Skips are never deletes:** no code path in this change removes a transaction — proven inside `TestD3SkipsACrossBatchDuplicateAndNamesIt`: the matched transaction is re-read, unchanged, after the skip that named it
- [x] 6.10 **Transfer detection:** an out and an in, equal, two days apart, different accounts, one organisation, produce one pair — `TestTransferDetection`
- [x] 6.11 **Across currencies:** a PLN-to-EUR transfer between the customer's own accounts pairs on the base amount, and the pair is unchanged by a later import carrying a different rate — `TestTransferAcrossCurrencies`
- [x] 6.12 **Within one source kind:** a ledger row and a bank row are never paired as an internal transfer — that is D4's job, and D4 is Product — `TestTransferNeverCrossesSourceKind`. **This test is what caught §4.1's missing filter** — the query had no `source_kind` predicate until this test failed against it
- [x] 6.13 **Deterministic:** three equal transfers in one week produce the same pairing whatever order the rows arrive in — `TestTransferPairingIsDeterministic`
- [x] 6.13b **One pair per transaction, on either side:** a transaction that is the `in` side of a pair cannot become the `out` side of another. This is the case two separate unique constraints let through, so assert it directly against the database — `TestOnlyOnePairPerTransaction`
- [x] 6.14 **One `normalize()`:** the Go and Python suites both pass the case table, and a deliberate change to one fails the other — already true by construction (§2.1-2.3): the Go suite fails immediately on a divergence from the checked-in fixture, and a change to `norm.py` requires regenerating and diffing that same fixture; no new test needed
- [x] 6.15 **Cross-tenant isolation:** A cannot read or dismiss B's skips or transfers, and the failure is a policy denial rather than a not-found — `TestSkipsCrossTenantIsolation`, `TestTransfersCrossTenantIsolation`
- [x] 6.16 **Fail-closed:** every query added here raises `42704` outside a tenant transaction — `TestDedupQueriesFailClosedOutsideATenantTransaction`

## 7. Front end — Track A

- [x] 7.1 A typed client for the four RPCs in `web/src/data` — `web/src/data/dedup.ts`
- [x] 7.2 i18n keys for `already_imported`, the two skip levels and the transfer states, in `en` and `ru` — plus `error.transfer_not_found` (`DismissInternalTransfer`'s own not-found code, not named in the task but needed for the RPC this section's client calls)

## 8. Close

- [x] 8.1 Add `/core/internal/dedup/` and the new query files to `.github/CODEOWNERS` under Track A — the directory was already listed; added `00012_dedup.sql` and `dedup.sql`
- [x] 8.2 Record the §0.1 decision in `docs/ARCHITECTURE.md` §5.2, which currently supports both readings — recorded, dated, with the reasoning
- [x] 8.3 Record in `docs/ARCHITECTURE.md` §5 that `dedup_hash` has ordinary false positives and that skips are therefore recorded, not counted — rewrote the table and the hash formula (now including `occurrence`), and added the D2/D3 note
- [x] 8.4 Update the capability spec and run the full suite — `specs/dedup-and-matching/spec.md` corrected to match the §3.2 finding (`openspec validate add-dedup` passes); full suite below
