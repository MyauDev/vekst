## Context

`ARCHITECTURE.md` §4a.1 lists the checks and `openspec/config.yaml` repeats them. Neither
says what a validation *is* once it has run, and that is the whole of this design.

Two properties are in tension, and both are load-bearing:

- **A rejected import persists nothing.** The import is atomic. This is what makes a
  partially imported file impossible.
- **A rejected import has to be explainable, days later, to the person who uploaded it.**
  A customer who is told "3 errors" and cannot see which lines will not use the product
  twice.

The resolution is that "nothing" means no *transaction*. The validation row is written on
every outcome, rejection included, because it is a record of what the system did rather
than a record of the customer's money. Getting this backwards in either direction is
expensive: write no row and the failure is unexplainable; write transactions and the
atomicity claim is false.

## Goals / Non-Goals

**Goals:** the `import_validations` table, the twelve checks, the three outcomes, the
override and its constraint, and the query change 4.1 needs to surface an override.

**Non-goals:** parsing, persistence, dedup, screens. Named in the proposal.

## The data model

```sql
CREATE TABLE import_validations (
    id        uuid NOT NULL DEFAULT gen_random_uuid(),
    org_id    uuid NOT NULL,
    batch_id  uuid NOT NULL,

    outcome   text NOT NULL CHECK (outcome IN ('valid','valid_with_warnings','rejected')),

    row_count     integer NOT NULL CHECK (row_count     >= 0),
    error_count   integer NOT NULL CHECK (error_count   >= 0),
    warning_count integer NOT NULL CHECK (warning_count >= 0),

    -- Tri-state on purpose. TRUE reconciled, FALSE did not, NULL the file
    -- declared no balances -- 1C exports often do not. A boolean NOT NULL would
    -- force "no balances declared" to be recorded as "did not reconcile", and a
    -- report would then carry a warning the customer cannot act on.
    balance_check_passed boolean NULL,

    -- Codes, line numbers and field names. Never a rendered sentence: the
    -- backend returns error codes and translation is the client's (CLAUDE.md).
    -- A structure test asserts no value in here is a message.
    report_jsonb jsonb NOT NULL,

    -- The override. Populated together or not at all, and only over warnings.
    overridden_by   uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    override_reason text        NULL CHECK (override_reason IS NULL
                                            OR length(btrim(override_reason)) >= 10),
    overridden_at   timestamptz NULL,

    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (org_id, id),
    FOREIGN KEY (org_id, batch_id) REFERENCES import_batches (org_id, id) ON DELETE RESTRICT,

    -- One validation per batch. A re-validation is a new batch, for the same
    -- reason a correction is a new import.
    UNIQUE (org_id, batch_id),

    -- D1: a correctness error can never be overridden, as a constraint rather
    -- than as a code path.
    CONSTRAINT import_validations_override_only_over_warnings CHECK (
        (overridden_by IS NULL AND override_reason IS NULL AND overridden_at IS NULL)
        OR (outcome = 'valid_with_warnings'
            AND overridden_by IS NOT NULL AND override_reason IS NOT NULL
            AND overridden_at IS NOT NULL)),

    -- The counts and the outcome cannot disagree.
    CONSTRAINT import_validations_counts_match_outcome CHECK (
        (outcome = 'rejected'            AND error_count > 0) OR
        (outcome = 'valid_with_warnings' AND error_count = 0 AND warning_count > 0) OR
        (outcome = 'valid'               AND error_count = 0 AND warning_count = 0))
);

CREATE INDEX import_validations_overridden_idx ON import_validations (org_id, overridden_at)
    WHERE overridden_at IS NOT NULL;
```

### Row-level security

```sql
ALTER TABLE import_validations ENABLE ROW LEVEL SECURITY;
ALTER TABLE import_validations FORCE  ROW LEVEL SECURITY;
CREATE POLICY import_validations_tenant ON import_validations FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

-- An override is written once. Re-deciding it silently is the one edit that
-- would change a printed report with no trace, so the column set is narrowed
-- rather than trusted to a code path.
REVOKE UPDATE (outcome, row_count, error_count, warning_count,
               balance_check_passed, report_jsonb) ON import_validations FROM vekst_app;
```

## D1 — A correctness error is unoverridable because it is unrepresentable

`ARCHITECTURE.md` §4a.1 says a correctness failure may not be overridden. The ordinary way
to honour that is an `if` in the override handler.

**Rejected.** That rule is a financial control, and a control that lives in one branch of
one function survives exactly as long as nobody refactors it. It also cannot be audited:
"was this ever overridden wrongly?" becomes a code-history question rather than a query.

**Chosen: `import_validations_override_only_over_warnings`.** A rejected batch with an
override is not a state the database can hold. The handler still checks, so the customer
gets a clean error code rather than a constraint violation, but the check is no longer the
thing that makes it true.

## D2 — Zero tolerance is a property of `int64`, and worth saying out loud

The balance check is `opening + Σ movements = closing`, with zero tolerance. That is only a
coherent sentence because every amount is an `int64` count of minor units. In floating
point the comparison would need an epsilon, an epsilon is a tolerance, and a tolerance
means the check no longer proves that no row was lost — it proves that not much was.

This is the clearest payoff of the money invariant in the whole product, and the test suite
should show it: §6.4 reconciles a 3,182-row file to the cent, and §6.5 fails it by a single
minor unit.

The tri-state `balance_check_passed` handles the third case honestly. A file that declares
no balances gets `NULL`, not `FALSE`, and the customer is told that the strongest check
could not run rather than that it failed.

## D3 — Rejection writes a row, and that is not a contradiction

The atomicity rule is about the customer's money: a rejected file contributes no
transaction, no vendor memory, no classification. It is not about the system's own record
of what it did.

So the order inside one `db.InTx` is: run the checks, write `import_validations`, and set
the batch to `rejected` — or to `validated`, at which point 2.5 persists in its own
transaction. Nothing about a rejection is conditional or best-effort, which is what makes
the error report reliably there when the customer asks.

## D4 — An override has to reach the report, and the report is not written yet

`ARCHITECTURE.md` §4a.1: an override is "recorded and shown on any report computed from
that batch". Change 4.1 is not written, so this change cannot enforce that. It can make the
omission loud instead of silent:

```sql
-- name: OverriddenBatchesForPeriod :many
-- Change 4.1 calls this and puts what it returns on the report. A report that
-- does not call it is a report that hides an override, so 4.1's tasks carry a
-- test that fails when the call is missing.
SELECT b.id, v.override_reason, v.overridden_at, v.balance_check_passed
FROM import_validations v
JOIN import_batches b ON b.id = v.batch_id
JOIN transactions t   ON t.batch_id = b.id
WHERE v.overridden_at IS NOT NULL
  AND t.entity_id = $1 AND t.booked_on >= $2 AND t.booked_on < $3
GROUP BY b.id, v.override_reason, v.overridden_at, v.balance_check_passed;
```

The override is written once and never recomputed — the same rule the Palm specification
sets for `scheduled_deletion_at`, and for the same reason: a value that can be
recalculated later can change a document that was already printed.

## D5 — The error report is codes, and its shape is fixed here

```jsonc
{
  "schema": 1,
  "errors": [
    { "line": 2847, "field": "amount", "code": "amount_not_parsable", "raw": "1 234,56-" }
  ],
  "warnings": [
    { "code": "balance_mismatch",
      "detail": { "opening": 41230000, "movements": -1844215, "closing": 39386000,
                  "difference": 215, "currency": "EUR" } }
  ]
}
```

`line` is the line in the original file. `raw` is the offending source text, kept because
the customer has to find it in Excel, and capped so a malformed file cannot produce a
report larger than the file. No key in this document holds a sentence; §6.9 asserts it by
structure, so a hurried fix cannot put English in the database.

Amounts inside `detail` are minor units, like every other amount in the system.

## Proto

`ImportService` gains two calls in the existing `proto/vekst/v1/import.proto`. Both
reviewers, per CODEOWNERS. Adding RPCs and messages to an existing service is not a
breaking change, and `buf breaking` confirms it.

```protobuf
rpc GetValidationReport(GetValidationReportRequest) returns (GetValidationReportResponse);
rpc OverrideValidation(OverrideValidationRequest) returns (OverrideValidationResponse);

message ValidationError {
  int32  line  = 1;   // in the original file
  string field = 2;
  string code  = 3;   // a code. The client owns the words
  string raw   = 4;
}

message OverrideValidationRequest {
  string batch_id = 1;
  string reason   = 2;   // stored, and shown on every report from this batch
}
```

No money field. The balance figures travel inside the report as minor-unit integers within
a structured detail message, never as a `double`.

## What this touches from the invariants list

- **Money and currency** — indirectly and decisively. D2.
- **Validation is blocking and atomic** — this change is that invariant. D3.
- **Tenant isolation** — a new tenant table, ordinary shape.
- **`source_kind` and the classifier contract** — neither.

## Rejected alternatives

| Rejected | Why |
| --- | --- |
| Enforce "no override over errors" in the handler only | A financial control in one branch of one function. See D1 |
| `balance_check_passed boolean NOT NULL` | Forces "the file declared no balances" to be recorded as "the balance did not reconcile", and warns the customer about something they cannot act on |
| Write no row when a batch is rejected | The atomicity rule is about the customer's money, not the system's record. The customer then cannot be told which lines failed. See D3 |
| Allow re-validation of the same batch in place | A batch would then have two histories and a report could be printed against either. A re-validation is a new batch |
| Store rendered messages in `report_jsonb` | `CLAUDE.md`: the backend returns codes and translation is the client's. Storing English also means a Russian customer reads a database's English |
| A tolerance of one minor unit on the balance check | One minor unit is a tolerance. Zero tolerance is the entire value of the check, and `int64` is what makes it available |
| Recompute the override state when a report runs | A printed report could change afterwards. Written once, like `scheduled_deletion_at` |

## Risks

| Risk | Mitigation |
| --- | --- |
| 2.2 does not carry the original line number through parsing, and this change discovers it late | §0.1 confirms the contract before any code here is written. It is the single hard dependency |
| An override is recorded and no report ever shows it, because 4.1 forgets to ask | `OverriddenBatchesForPeriod` exists now and 4.1's task list carries a test that fails without the call. D4 |
| The seven correctness checks drift from `ARCHITECTURE.md` §4a.1 as codes are added | The check list is table-driven in one file and §6.1 asserts the table against the document, so adding a check to code without the document fails |
| A hostile or broken file produces a report larger than the file itself | `raw` is capped and the error list is truncated with a count, which is also what a customer can actually read |
