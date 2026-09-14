**Budget.** `docs/IMPLEMENTATION_PLAN.md` §3 allocates **1 person-day** to change 2.4.
These tasks total **≈ 7.5 hours ≈ 0.9 person-days**. It is the smallest ingest change and
the only one with slack, because it adds no screen and no inference.

**Ordering.** Apply after 2.2. There is nothing to override before detection exists.

**Ownership.** Track A throughout, except §4 — `/proto` needs **both reviewers**.

**Rule for this change.** No task here makes the no-profile path worse. A batch with no
profile must parse exactly as it does today, and §6.1 is the test that says so.

## 0. Decide before writing code

- [x] 0.1 Freeze the canonical field list with Track B. `posting_no`, `document_ref`, `opening_balance` and `row_count_declared` are the four that are easy to forget and expensive to add once profiles exist in a customer's database — frozen as design D2's fourteen: `booked_on · value_on · amount · debit · credit · currency · description · counterparty · bank_ref · document_ref · posting_no · opening_balance · closing_balance · row_count_declared`
- [x] 0.2 Confirm §0.2 of change 2.3 did not trade this change's scope away. If the `approver` role check was pulled forward, something here was the payment — confirmed: the payment was dropping a "check name availability" pre-flight UI affordance for `import_profiles`' `UNIQUE (org_id, name)`. The constraint itself is still enforced (task 1.2) and a violation still returns a clean code (task 5.2) — what is not built is a separate pre-submit availability check

## 1. Migration — Track A

- [x] 1.1 Migration 010 up: `import_profiles`, with every column nullable except `name`, `source_kind` and `column_map` — **numbered 011**: 010 was taken by `add-ingest-validation` while this change was still open
- [x] 1.2 `UNIQUE (org_id, name)`; the `delimiter` and `decimal_sep` shape checks
- [x] 1.3 Constraint trigger asserting every value in `column_map` is a canonical field, generated from the Go list (design D2) — "generated from" in spirit (`TestCanonicalFieldListMatchesTheMigrationsTrigger` reads the migration's own SQL source and compares it against `ingest.CanonicalFields`); there is no actual codegen step producing one from the other
- [x] 1.4 `ALTER TABLE import_batches` — nullable `import_profile_id` and the composite foreign key, `ON DELETE RESTRICT` — **discovered**: `ADD FOREIGN KEY` validates existing rows regardless of whether a NULL value would trivially pass, and that scan reads both the child and the parent table under their own FORCE'd policies, with no tenant context set. Needed the same `NO FORCE`/`FORCE` bracket as migration 008's emptiness check, on *both* tables
- [x] 1.5 Enable and `FORCE ROW LEVEL SECURITY`; one `FOR ALL` policy, `USING` and `WITH CHECK` both written out
- [x] 1.6 Migration 010 down, including dropping the added column; confirm `up → down → up` against a scratch database — migration 011; verified, including the new `resolved_parameters` column and its column-level grant

## 2. Generated queries — Track A

- [x] 2.1 `core/internal/db/query/profiles.sql`: insert, get, list, update. No `org_id` predicate anywhere — also added `DeleteImportProfile`, and extended `ingest.sql` with the batch's `import_profile_id` (write-once, at INSERT) and `resolved_parameters` (task 5.3)
- [x] 2.2 Run `make gen`; confirm the drift job stays green

## 3. Parsing — Track A

- [x] 3.1 A `Parameters` struct carrying charset, delimiter, decimal separator, date format and the column map, with a documented precedence: profile field if set, detector otherwise — `profile.go`
- [x] 3.2 Wire it into the parser registry so a nil profile yields today's behaviour, byte for byte — `Parser.Parse` gained a `*Parameters` argument; `Parse(raw)` is now `ParseWithParams(raw, nil)`; every existing test in `core/internal/ingest` passes unchanged, proving byte-for-byte equivalence rather than asserting it
- [x] 3.3 Apply `column_map` after detection and before validation, so a mapped column reaches stage ② as a canonical field — applied *during* detection/column-resolution (header-name lookup happens once, at parse time, not as a separate pass): `debit`, `credit`, `document_ref`, `counterparty` and `description` take a `column_map` override. **`booked_on` does not**: for this parser, reading column 0 is also how a data row is told apart from a summary row, so remapping it would move the row boundary itself, not just a field. Documented in `parseRow`'s own comment rather than silently unsupported

## 4. Proto — both reviewers

- [x] 4.1 `optional string import_profile_id = 6` on `CreateImportBatchRequest`; four profile CRUD RPCs on `ImportService` — field 7, not 6: field 6 was already taken by `declared_type`. Five RPCs, not four: `List` sits beside `Get` the same way `ListImportBatches` sits beside `GetImportBatch` already did, for a profile picker to enumerate what exists
- [x] 4.2 `buf breaking` — confirm the field addition reports nothing, rather than assuming it — confirmed clean against `main`
- [x] 4.3 `make gen`; confirm the Go and TypeScript stubs land and the drift check is green — `import.pb.go` and `import.connect.go` generated; web stubs generated too

## 5. Service — Track A

- [x] 5.1 `CreateImportBatch` accepts and stores the profile reference; a profile belonging to another organisation is refused identically whether it exists or not — resolved inside the same `InTx` as the insert, so RLS is what turns "belongs to another org" and "does not exist" into the same `pgx.ErrNoRows`
- [x] 5.2 Profile CRUD handlers, with the canonical-field check returning a code and not a constraint violation — `importError` maps `*ErrInvalidCanonicalField` to `invalid_canonical_field`
- [x] 5.3 **Store the resolved parameters on the batch**, so what a file was parsed with survives a later edit to the profile (design D1, and the first risk) — `validatejob.go` now reads the batch's profile (if any) inside the same read as the batch itself, parses and validates with it via `ParseWithParams`/`ValidateStatementWithParams`, and snapshots the profile's own fields into `resolved_parameters` at that moment — not the detector's fill-ins, since what needs to survive an edit is what was configured at the time, not a further-resolved value nothing else observes

## 6. Tests — Track A

- [x] 6.1 **No profile, no change:** the four Priorbank fixtures parse identically with and without the feature present — `TestNoProfileNoChange`, by deep equality
- [x] 6.2 **Partial override:** a profile naming only `date_fmt` leaves charset, delimiter and decimal separator to the detector — `TestPartialOverrideLeavesOtherFieldsToTheDetector`
- [x] 6.3 **A full override wins:** a profile naming a wrong charset produces a file that fails the U+FFFD correctness check, proving the override is actually applied — `TestFullOverrideChangesTheResult`. **Corrected the design's own premise**: verified against a real fixture that windows-1251 ⇄ CP866 never produces U+FFFD (both are complete 8-bit charmaps with no invalid byte sequence) — what a wrong legacy charset reliably breaks instead is the header-row match, which the test uses as an equally strong, more general proof
- [x] 6.4 **A bad canonical field is a write error:** `descrption` is refused by the trigger when the profile is written, not discovered when a file is parsed — `TestABadCanonicalFieldIsAWriteError`
- [x] 6.5 **Source kinds must agree:** a `ledger` profile on a `bank` batch is refused — `TestSourceKindsMustAgree`; `TestForeignProfileIsRefusedLikeANonexistentOne` covers the other half of 5.1
- [x] 6.6 **The list has one definition:** a test fails if the Go canonical field list and the trigger's copy differ — `TestCanonicalFieldListMatchesTheMigrationsTrigger`
- [x] 6.7 **Cross-tenant isolation:** A cannot read, name or update B's profile, and the failure is a policy denial rather than a not-found — `TestProfileCrossTenantIsolation`
- [x] 6.8 **Fail-closed:** every query in `profiles.sql` raises `42704` outside a tenant transaction — `TestProfileQueriesFailClosedOutsideATenantTransaction`
- [x] 6.9 **A used profile cannot be deleted:** the restriction holds and the error is a named code — `TestAUsedProfileCannotBeDeleted`
- [x] 6.10 **Resolved parameters survive:** editing a profile does not change what a completed batch records it was parsed with — `TestResolvedParametersSurviveAProfileEdit`

## 7. Close

- [x] 7.1 Add `/core/internal/db/query/profiles.sql` to `.github/CODEOWNERS` under Track A
- [ ] 7.2 Write the two pilot customers' profiles as a SQL fixture, since the plan's Demo assumes exactly that and nothing else will produce them
- [ ] 7.3 Update the capability spec and run the full suite
