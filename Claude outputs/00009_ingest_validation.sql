-- Change 2.3, add-ingest-validation: the record of whether a file could be
-- believed.
--
-- Two properties are in tension here and both are load-bearing.
--
--   A rejected import persists nothing. The import is atomic, which is what
--   makes a half-imported file impossible.
--
--   A rejected import has to be explainable, days later, to the person who
--   uploaded it. A customer told "3 errors" who cannot see which lines will
--   not use this product twice.
--
-- The resolution is that "nothing" means no TRANSACTION. This row is written on
-- every outcome, rejection included, because it is a record of what the system
-- did rather than a record of the customer's money. Getting that backwards in
-- either direction is expensive: write no row and the failure is
-- unexplainable; write transactions and the atomicity claim is false.

-- +goose Up

CREATE TABLE import_validations (
    id       uuid NOT NULL DEFAULT gen_random_uuid(),
    org_id   uuid NOT NULL,
    batch_id uuid NOT NULL,

    outcome text NOT NULL CHECK (outcome IN ('valid', 'valid_with_warnings', 'rejected')),

    row_count     integer NOT NULL CHECK (row_count     >= 0),
    error_count   integer NOT NULL CHECK (error_count   >= 0),
    warning_count integer NOT NULL CHECK (warning_count >= 0),

    -- Tri-state on purpose. TRUE reconciled, FALSE did not, NULL the file
    -- declared no balances -- 1C exports often do not, and change 2.2's
    -- Statement carries the bank's declared figures precisely so that the
    -- difference between "did not reconcile" and "could not be checked" is
    -- knowable. A boolean NOT NULL would force the second into the first and
    -- warn a customer about something they cannot act on.
    balance_check_passed boolean NULL,

    -- Codes, line numbers, field names and the offending source text. Never a
    -- rendered sentence: CLAUDE.md puts the backend on codes and leaves
    -- translation to the client, and storing English also means a Russian
    -- customer reads a database's English. A structure test asserts it.
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
    -- reason a correction is a new import: a batch with two histories is one a
    -- report can be printed against either way.
    UNIQUE (org_id, batch_id),

    -- A correctness error can never be overridden. ARCHITECTURE.md 4a.1 says so
    -- and the ordinary way to honour it is an IF in the override handler --
    -- which is a financial control living in one branch of one function, and it
    -- survives exactly as long as nobody refactors. As a constraint, a rejected
    -- batch carrying an override is not a state the database can hold. The
    -- handler still checks, so the customer gets a clean error code rather than
    -- a constraint violation, but the check is no longer what makes it true.
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

-- The report query change 4.1 calls: which batches feeding a period carried an
-- accepted warning. Partial, because an override is rare and the index exists
-- to make "did anything in this report rest on one" cheap.
CREATE INDEX import_validations_overridden_idx ON import_validations (org_id, overridden_at)
    WHERE overridden_at IS NOT NULL;

ALTER TABLE import_validations ENABLE ROW LEVEL SECURITY;
ALTER TABLE import_validations FORCE  ROW LEVEL SECURITY;

CREATE POLICY import_validations_tenant ON import_validations FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

-- An override is written once. Re-deciding it silently is the one edit that
-- would change an already-printed report with no trace, and the outcome
-- columns are a record of what happened rather than state to be maintained.
-- Same shape as 00008's narrowing: revoke the table grant that 00001's default
-- privileges gave, then hand back only the columns that legitimately move.
--
-- A column-level REVOKE alone does nothing here -- table-level and column-level
-- privileges are separate, and the table grant covers every column. That was
-- found by running 00008 rather than by reading it.
REVOKE UPDATE ON import_validations FROM vekst_app;
GRANT  UPDATE (overridden_by, override_reason, overridden_at)
    ON import_validations TO vekst_app;

-- +goose Down

REVOKE UPDATE (overridden_by, override_reason, overridden_at) ON import_validations FROM vekst_app;
GRANT  UPDATE ON import_validations TO vekst_app;

DROP TABLE IF EXISTS import_validations;
