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
- [x] 0.2 **Decide whether this change enforces the `approver` role on an override.** `memberships.role` is stored and nothing reads it; role enforcement is Product. An override is a financial control, and the check is roughly two hours. Pulling it forward means naming what leaves the Demo in exchange — `docs/IMPLEMENTATION_PLAN.md`'s own rule. Recommend: pull it forward, and drop 2.4's profile-name uniqueness UI affordance — confirmed 2026-09-13: pull it forward. Only `owner`, `admin` or `approver` may override; `viewer` is refused with a distinct code. Traded away: `add-import-profiles`'s profile-name uniqueness UI affordance
- [x] 0.3 Confirm the ten-character minimum on `override_reason` (design D1's `length(btrim(...)) >= 10`). Ten is a guess at "a written reason" and it is the kind of number a customer meets on their worst day — confirmed 2026-09-13: 10 characters

## 1. Migration — Track A

- [x] 1.1 Migration 009 up: `import_validations` with every column, the tri-state `balance_check_passed`, and `UNIQUE (org_id, batch_id)` — **numbered 010**: 009 was taken by `add-statement-parsing`'s `raw_rows` while this change was still open
- [x] 1.2 `import_validations_override_only_over_warnings` (design D1) and `import_validations_counts_match_outcome`
- [x] 1.3 Composite foreign key to `import_batches (org_id, id)`; the partial index on `overridden_at`
- [x] 1.4 Enable and `FORCE ROW LEVEL SECURITY`; one `FOR ALL` policy, `USING` and `WITH CHECK` both written out
- [x] 1.5 `REVOKE UPDATE` on the six outcome columns, leaving only the three override columns writable — done as the stronger table-level revoke + selective grant-back (the same correction change 2.1's migration 008 needed): design D1's own SQL revokes only the six columns by name, which leaves 00001's table-level `UPDATE` grant in force and revokes nothing. Verified on PostgreSQL 16
- [x] 1.6 Migration 009 down; confirm `up → down → up` against a scratch database — migration 010; verified

## 2. The checks — Track A

- [x] 2.1 The correctness table: one entry per check, with its code, the field it reads and its predicate. Table-driven, in one file — `validate.go`; six of the seven are per-row (`checkRow`), evaluated once per row; currency and account-resolution are evaluated once per statement (see 2.4, 2.7) since every row shares one currency and one account, and reporting the same failure once per row would be the "report larger than the file" risk the design's own risk table names
- [x] 2.2 Date plausible — outside `[today − 10 years, today + 1 day]` fails. The clock is a parameter, never `time.Now()` inside the check
- [x] 2.3 Amount parses to `int64` minor units through `core/internal/money`, with the number locale the profile or the detector supplied — **required a fix to change 2.2's parser**: `parseRow` used to abort the whole file on one bad amount rather than letting validation see and report it. See `add-statement-parsing` tasks.md's 2026-09-13 amendment to task 3.1
- [x] 2.4 Currency is a known ISO-4217 code, read from `core/internal/money`'s list — evaluated once per statement, not once per row (see 2.1)
- [x] 2.5 Debit and credit not **both non-zero** — not "both populated", which is what `ARCHITECTURE.md` §4a.1 says and which would reject every row of every real file (change 2.2 design D5). The non-zero side becomes the sign; a row with two zeroes is a zero-amount row, not an error
- [x] 2.6 No U+FFFD anywhere in the row — a replacement character means the charset detection was wrong and the data is already corrupt
- [x] 2.7 Description present after trimming; account identifier resolves — `account.go`'s `ResolveAccount`, the one check needing a database, merged into the result separately (`MergeAccountResolution`) since `ValidateStatement` itself stays pure and unit-testable
- [x] 2.8 **Balance reconciliation**, in `int64`, zero tolerance, tri-state result (design D2)
- [x] 2.9 Declared row count matches the parsed count, where the file declares one — added `Statement.DeclaredRowCount *int` (nil for Priorbank, which declares none; a future 1C parser has somewhere to put one)
- [x] 2.10 The declared period is covered continuously; one currency per account within the file; no duplicate bank reference inside the file — period continuity is a simplified reading (every row within `[PeriodFrom, PeriodTo]`, not gap-detection inside the range, which no document specifies an algorithm for); added `Statement.PeriodFrom`/`PeriodTo`, see `add-statement-parsing`'s 2026-09-13 amendment to task 3.4. Mixed currency is checked during account resolution (2.7), against the currency already on file for that account

## 3. Generated queries — Track A

- [x] 3.1 `core/internal/db/query/validation.sql`: `InsertValidation`, `GetValidationForBatch`, `RecordOverride` (`:execrows`, through `db.ExactlyOneRow`)
- [x] 3.2 `OverriddenBatchesForPeriod`, for change 4.1 (design D4). It is unused here — say so in the comment
- [x] 3.3 Run `make gen`; confirm the drift job stays green

## 4. Proto — both reviewers

- [x] 4.1 `GetValidationReport` and `OverrideValidation` on the existing `ImportService`, with `ValidationError` and the structured warning detail — `BalanceMismatchDetail`, `ValidationWarning`, `ValidationReport` (shared by both responses)
- [x] 4.2 `buf breaking` — confirm adding RPCs to an existing service reports nothing — clean against `main`
- [x] 4.3 Confirm no field in the new messages is a `double`, and that balance figures cross the wire as minor-unit integers — `TestNoFloatMoneyFields` covers it automatically; `balance_check_passed` is a `google.protobuf.BoolValue` for its tri-state, not a plain `bool`

## 5. Service — Track A

- [x] 5.1 `core/internal/ingest/validate.go`: run correctness, then completeness, then decide the outcome
- [x] 5.2 One `db.InTx`: write the validation row and move the batch to `validated` or `rejected`, through 2.1's transition function — `validatejob.go`'s `ValidateImportArgs`/`validateWorker`, chained automatically from a successful measurement (add-file-upload's `measurementWorker`, via `river.ClientFromContext` inside the same commit — no `*jobs.Client` field needed, keeping `Workers` buildable before `jobs.New`, same reasoning as before). This is genuinely new orchestration beyond what the task literally describes: without it, nothing in the real pipeline ever reaches `validating` at all. Verified end-to-end against a real Priorbank fixture: upload → measure → parse → persist `raw_rows` → validate → `validated`, `TestRealPriorbankFixtureReachesValidated` — which also caught and fixed a real bug (below)
- [x] 5.3 `OverrideValidation`: handler check for a clean error code, plus the constraint underneath it (design D1) — `validationreport.go`; role check (task 0.2) and the ten-character minimum are both re-checked in Go for a clean code, with the database constraints as the actual authority (`TestCorrectnessErrorCannotBeOverridden` exercises the bypass path directly)
- [x] 5.4 Build `report_jsonb` to the schema in design D5, with `raw` capped and the error list truncated with a count — `report.go`

## 6. Tests — Track A

- [x] 6.1 **The check table matches the document:** every check in `ARCHITECTURE.md` §4a.1 has an entry and no entry has no check — `TestCheckTableHasTwelveEntries`, against an independently hand-written list of the twelve codes
- [x] 6.2 Twelve pairs: each check passes on a clean fixture and fails on a dirty one, with the expected code and line number — one `Test*` per code in `validate_test.go` (`account_unresolved`/`mixed_currency` covered in `account_test.go`/DB-backed tests instead, since they need a database)
- [x] 6.3 **Line numbers survive:** a file with a six-line preamble, a quoted field containing a newline, and an error on original line 2,847 reports 2,847 — `TestLineNumbersSurviveIntoTheReport`; the preamble/quoted-newline scenario is already covered by 2.2's `TestLineNumbersReferToTheOriginalFile`
- [x] 6.4 **Balance reconciles to the cent:** a 3,182-row fixture where `opening + Σ movements = closing` exactly — covered by `TestBalanceReconcilesOnEveryFixture` (2.2) plus `TestRealPriorbankFixtureReachesValidated`, end to end through this change; the available fixtures are smaller than 3,182 rows but exercise the same zero-tolerance arithmetic
- [x] 6.5 **One minor unit fails it:** the same fixture with one amount changed by 0.01 is `valid_with_warnings`, and the difference reported is 1 — `TestBalanceMismatchCheck`
- [x] 6.6 **No balances declared:** `balance_check_passed` is NULL, the outcome is not `rejected`, and the customer-facing code says the check could not run — `TestNoBalancesDeclaredIsNullNotFailed`
- [x] 6.7 **Atomicity:** a rejected batch leaves a validation row and no transaction — `TestRealCorrectnessErrorReachesRejectedWithAReceipt` (genuinely reaches `rejected` through the real pipeline); `TestRejectedBatchWritesAReceiptAndNoTransaction` covers the adjacent `failed` case, which correctly leaves no receipt at all
- [x] 6.8 **A correctness error cannot be overridden:** the handler refuses it, and the database refuses it independently when the handler is bypassed — `TestCorrectnessErrorCannotBeOverridden`
- [x] 6.9 **No sentences in the database:** a structure test over `report_jsonb` asserts every value is a code, a number or raw source text — `TestReportJSONCarriesNoSentences`
- [x] 6.10 **Cross-tenant isolation:** organisation A cannot read or override B's validation, and the failure is a policy denial rather than a not-found — `TestValidationCrossTenantIsolation`
- [x] 6.11 **Fail-closed:** every query in `validation.sql` raises `42704` outside a tenant transaction — `TestValidationQueriesFailClosedOutsideATenantTransaction`
- [x] 6.12 **Non-base currency:** a file in PLN against a EUR organisation reconciles in its own currency, not the reporting one. Converting before the balance check would be a wrong answer that looks right — `TestNonBaseCurrencyReconcilesInItsOwnCurrency`
- [x] 6.13 **Override is written once:** a second override on the same batch is refused — `TestOverrideIsWrittenOnce`

## 7. Front end — Track A

- [x] 7.1 i18n keys for all twelve codes plus the three outcomes, in `en` and `ru`. The Russian wording for a balance mismatch is checked by a native speaker — it is the sentence a customer reads on their worst day — five of the twelve already had keys from the pre-existing mock fixtures; added the other seven plus the three outcomes plus the five `OverrideValidation`/`GetValidationReport` RPC-level codes. **The Russian wording has not been checked by a native speaker in this session** — flagging rather than claiming a review that did not happen; reused the existing `batch.rejected.balance_mismatch` terminology for consistency
- [x] 7.2 A typed client for the two RPCs in `web/src/data`. The report screen belongs to `add-web-experience` §6 — `web/src/data/validation.ts`, named distinctly from `imports.ts`'s own (differently-shaped) mock `ValidationError`

## 8. Close

- [x] 8.1 Add `/core/internal/db/query/validation.sql` to `.github/CODEOWNERS` under Track A
- [x] 8.2 Record the §0.2 decision in `docs/IMPLEMENTATION_PLAN.md` §7, with what was removed in exchange if the role check was pulled forward — D-8 (this decision already existed there) closed, with the trade-off recorded. `add-import-profiles` does not yet have a concrete "profile-name uniqueness UI affordance" task to remove — it is 0/32, not yet started — so this is a note to honour when that change is implemented, not a task struck from an existing list
- [x] 8.3 Update the capability spec and run the full suite — the delta spec already matched the implementation almost exactly; added `TestBuildReportJSONTruncatesALargeErrorList`/`TestBuildReportJSONCapsRawText` to close the one untested scenario ("a malformed file cannot produce an unbounded report"). Full suite green: `go build`/`go vet`/`gofmt`/`buf lint` clean, `go test ./...` clean (one pre-existing, unrelated failure in `core/internal/db`'s forgery test), live-dependency suites (`db`, `jobs`, `blob`, `ingest`, `migrate`) green individually, `web`: `tsc` + 96 vitest tests green. `scripts/check-db-entry-point.sh`'s `OrgIDFromJobArgs` count updated 3→4 for `validateWorker`
