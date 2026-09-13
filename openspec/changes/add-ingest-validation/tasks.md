**Budget.** `docs/IMPLEMENTATION_PLAN.md` §3 allocates **2.5 person-days** to change 2.3.
These tasks total **≈ 19 hours ≈ 2.4 person-days**. It is the largest of the ingest changes
and the one with the least room, because twelve checks each need a passing and a failing
fixture and neither can be shared.

**Ordering.** Apply after 2.2. §0.1 is not a formality: this change is unbuildable if a
parsed row cannot say which line of the original file it came from.

**Ownership.** Track A throughout, except §4 — `/proto` needs **both reviewers**.

**Rule for this change.** Every check gets a fixture that passes and a fixture that fails.
A check with only a passing fixture is a check that has never been observed to do anything.

## 0. Decide before writing code

- [x] 0.1 ~~Confirm with 2.2 that every parsed row carries its line number in the original file.~~ **Satisfied.** `ingest.Row.LineNo` is the 1-based line in the original file and `TestLineNumbersReferToTheOriginalFile` pins it against a fixture with a multi-line preamble
- [ ] 0.2 **Decide whether this change enforces the `approver` role on an override.** `memberships.role` is stored and nothing reads it; role enforcement is Product. An override is a financial control, and the check is roughly two hours. Pulling it forward means naming what leaves the Demo in exchange — `docs/IMPLEMENTATION_PLAN.md`'s own rule. Recommend: pull it forward, and drop 2.4's profile-name uniqueness UI affordance
- [ ] 0.3 Confirm the ten-character minimum on `override_reason` (design D1's `length(btrim(...)) >= 10`). Ten is a guess at "a written reason" and it is the kind of number a customer meets on their worst day

## 1. Migration — Track A

- [ ] 1.1 Migration 009 up: `import_validations` with every column, the tri-state `balance_check_passed`, and `UNIQUE (org_id, batch_id)`
- [ ] 1.2 `import_validations_override_only_over_warnings` (design D1) and `import_validations_counts_match_outcome`
- [ ] 1.3 Composite foreign key to `import_batches (org_id, id)`; the partial index on `overridden_at`
- [ ] 1.4 Enable and `FORCE ROW LEVEL SECURITY`; one `FOR ALL` policy, `USING` and `WITH CHECK` both written out
- [ ] 1.5 `REVOKE UPDATE` on the six outcome columns, leaving only the three override columns writable
- [ ] 1.6 Migration 009 down; confirm `up → down → up` against a scratch database

## 2. The checks — Track A

- [ ] 2.1 The correctness table: one entry per check, with its code, the field it reads and its predicate. Table-driven, in one file
- [ ] 2.2 Date plausible — outside `[today − 10 years, today + 1 day]` fails. The clock is a parameter, never `time.Now()` inside the check
- [ ] 2.3 Amount parses to `int64` minor units through `core/internal/money`, with the number locale the profile or the detector supplied
- [ ] 2.4 Currency is a known ISO-4217 code, read from `core/internal/money`'s list
- [ ] 2.5 Debit and credit not **both non-zero** — not "both populated", which is what `ARCHITECTURE.md` §4a.1 says and which would reject every row of every real file (change 2.2 design D5). The non-zero side becomes the sign; a row with two zeroes is a zero-amount row, not an error
- [ ] 2.6 No U+FFFD anywhere in the row — a replacement character means the charset detection was wrong and the data is already corrupt
- [ ] 2.7 Description present after trimming; account identifier resolves
- [ ] 2.8 **Balance reconciliation**, in `int64`, zero tolerance, tri-state result (design D2)
- [ ] 2.9 Declared row count matches the parsed count, where the file declares one
- [ ] 2.10 The declared period is covered continuously; one currency per account within the file; no duplicate bank reference inside the file

## 3. Generated queries — Track A

- [ ] 3.1 `core/internal/db/query/validation.sql`: `InsertValidation`, `GetValidationForBatch`, `RecordOverride` (`:execrows`, through `db.ExactlyOneRow`)
- [ ] 3.2 `OverriddenBatchesForPeriod`, for change 4.1 (design D4). It is unused here — say so in the comment
- [ ] 3.3 Run `make gen`; confirm the drift job stays green

## 4. Proto — both reviewers

- [ ] 4.1 `GetValidationReport` and `OverrideValidation` on the existing `ImportService`, with `ValidationError` and the structured warning detail
- [ ] 4.2 `buf breaking` — confirm adding RPCs to an existing service reports nothing
- [ ] 4.3 Confirm no field in the new messages is a `double`, and that balance figures cross the wire as minor-unit integers

## 5. Service — Track A

- [ ] 5.1 `core/internal/ingest/validate.go`: run correctness, then completeness, then decide the outcome
- [ ] 5.2 One `db.InTx`: write the validation row and move the batch to `validated` or `rejected`, through 2.1's transition function
- [ ] 5.3 `OverrideValidation`: handler check for a clean error code, plus the constraint underneath it (design D1)
- [ ] 5.4 Build `report_jsonb` to the schema in design D5, with `raw` capped and the error list truncated with a count

## 6. Tests — Track A

- [ ] 6.1 **The check table matches the document:** every check in `ARCHITECTURE.md` §4a.1 has an entry and no entry has no check
- [ ] 6.2 Twelve pairs: each check passes on a clean fixture and fails on a dirty one, with the expected code and line number
- [ ] 6.3 **Line numbers survive:** a file with a six-line preamble, a quoted field containing a newline, and an error on original line 2,847 reports 2,847
- [ ] 6.4 **Balance reconciles to the cent:** a 3,182-row fixture where `opening + Σ movements = closing` exactly
- [ ] 6.5 **One minor unit fails it:** the same fixture with one amount changed by 0.01 is `valid_with_warnings`, and the difference reported is 1
- [ ] 6.6 **No balances declared:** `balance_check_passed` is NULL, the outcome is not `rejected`, and the customer-facing code says the check could not run
- [ ] 6.7 **Atomicity:** a rejected batch leaves a validation row and no transaction
- [ ] 6.8 **A correctness error cannot be overridden:** the handler refuses it, and the database refuses it independently when the handler is bypassed
- [ ] 6.9 **No sentences in the database:** a structure test over `report_jsonb` asserts every value is a code, a number or raw source text
- [ ] 6.10 **Cross-tenant isolation:** organisation A cannot read or override B's validation, and the failure is a policy denial rather than a not-found
- [ ] 6.11 **Fail-closed:** every query in `validation.sql` raises `42704` outside a tenant transaction
- [ ] 6.12 **Non-base currency:** a file in PLN against a EUR organisation reconciles in its own currency, not the reporting one. Converting before the balance check would be a wrong answer that looks right
- [ ] 6.13 **Override is written once:** a second override on the same batch is refused

## 7. Front end — Track A

- [ ] 7.1 i18n keys for all twelve codes plus the three outcomes, in `en` and `ru`. The Russian wording for a balance mismatch is checked by a native speaker — it is the sentence a customer reads on their worst day
- [ ] 7.2 A typed client for the two RPCs in `web/src/data`. The report screen belongs to `add-web-experience` §6

## 8. Close

- [ ] 8.1 Add `/core/internal/db/query/validation.sql` to `.github/CODEOWNERS` under Track A
- [ ] 8.2 Record the §0.2 decision in `docs/IMPLEMENTATION_PLAN.md` §7, with what was removed in exchange if the role check was pulled forward
- [ ] 8.3 Update the capability spec and run the full suite
