**Budget.** `docs/IMPLEMENTATION_PLAN.md` §3 allocates **1 person-day** to change 2.4.
These tasks total **≈ 7.5 hours ≈ 0.9 person-days**. It is the smallest ingest change and
the only one with slack, because it adds no screen and no inference.

**Ordering.** Apply after 2.2. There is nothing to override before detection exists.

**Ownership.** Track A throughout, except §4 — `/proto` needs **both reviewers**.

**Rule for this change.** No task here makes the no-profile path worse. A batch with no
profile must parse exactly as it does today, and §6.1 is the test that says so.

## 0. Decide before writing code

- [ ] 0.1 Freeze the canonical field list with Track B. `posting_no`, `document_ref`, `opening_balance` and `row_count_declared` are the four that are easy to forget and expensive to add once profiles exist in a customer's database
- [ ] 0.2 Confirm §0.2 of change 2.3 did not trade this change's scope away. If the `approver` role check was pulled forward, something here was the payment

## 1. Migration — Track A

- [ ] 1.1 Migration 010 up: `import_profiles`, with every column nullable except `name`, `source_kind` and `column_map`
- [ ] 1.2 `UNIQUE (org_id, name)`; the `delimiter` and `decimal_sep` shape checks
- [ ] 1.3 Constraint trigger asserting every value in `column_map` is a canonical field, generated from the Go list (design D2)
- [ ] 1.4 `ALTER TABLE import_batches` — nullable `import_profile_id` and the composite foreign key, `ON DELETE RESTRICT`
- [ ] 1.5 Enable and `FORCE ROW LEVEL SECURITY`; one `FOR ALL` policy, `USING` and `WITH CHECK` both written out
- [ ] 1.6 Migration 010 down, including dropping the added column; confirm `up → down → up` against a scratch database

## 2. Generated queries — Track A

- [ ] 2.1 `core/internal/db/query/profiles.sql`: insert, get, list, update. No `org_id` predicate anywhere
- [ ] 2.2 Run `make gen`; confirm the drift job stays green

## 3. Parsing — Track A

- [ ] 3.1 A `Parameters` struct carrying charset, delimiter, decimal separator, date format and the column map, with a documented precedence: profile field if set, detector otherwise
- [ ] 3.2 Wire it into the parser registry so a nil profile yields today's behaviour, byte for byte
- [ ] 3.3 Apply `column_map` after detection and before validation, so a mapped column reaches stage ② as a canonical field

## 4. Proto — both reviewers

- [ ] 4.1 `optional string import_profile_id = 6` on `CreateImportBatchRequest`; four profile CRUD RPCs on `ImportService`
- [ ] 4.2 `buf breaking` — confirm the field addition reports nothing, rather than assuming it
- [ ] 4.3 `make gen`; confirm the Go and TypeScript stubs land and the drift check is green

## 5. Service — Track A

- [ ] 5.1 `CreateImportBatch` accepts and stores the profile reference; a profile belonging to another organisation is refused identically whether it exists or not
- [ ] 5.2 Profile CRUD handlers, with the canonical-field check returning a code and not a constraint violation
- [ ] 5.3 **Store the resolved parameters on the batch**, so what a file was parsed with survives a later edit to the profile (design D1, and the first risk)

## 6. Tests — Track A

- [ ] 6.1 **No profile, no change:** the four Priorbank fixtures parse identically with and without the feature present
- [ ] 6.2 **Partial override:** a profile naming only `date_fmt` leaves charset, delimiter and decimal separator to the detector
- [ ] 6.3 **A full override wins:** a profile naming a wrong charset produces a file that fails the U+FFFD correctness check, proving the override is actually applied
- [ ] 6.4 **A bad canonical field is a write error:** `descrption` is refused by the trigger when the profile is written, not discovered when a file is parsed
- [ ] 6.5 **Source kinds must agree:** a `ledger` profile on a `bank` batch is refused
- [ ] 6.6 **The list has one definition:** a test fails if the Go canonical field list and the trigger's copy differ
- [ ] 6.7 **Cross-tenant isolation:** A cannot read, name or update B's profile, and the failure is a policy denial rather than a not-found
- [ ] 6.8 **Fail-closed:** every query in `profiles.sql` raises `42704` outside a tenant transaction
- [ ] 6.9 **A used profile cannot be deleted:** the restriction holds and the error is a named code
- [ ] 6.10 **Resolved parameters survive:** editing a profile does not change what a completed batch records it was parsed with

## 7. Close

- [ ] 7.1 Add `/core/internal/db/query/profiles.sql` to `.github/CODEOWNERS` under Track A
- [ ] 7.2 Write the two pilot customers' profiles as a SQL fixture, since the plan's Demo assumes exactly that and nothing else will produce them
- [ ] 7.3 Update the capability spec and run the full suite
