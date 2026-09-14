-- Change 2.3, add-ingest-validation: import_validations.
--
-- Numbered 010, not 009 as the change's own tasks.md says: migration 009 was
-- taken by add-statement-parsing's raw_rows while this change was still
-- open. The table below is unchanged from design.md's own SQL.
--
-- Two properties are in tension here, and both are load-bearing. A rejected
-- import persists no transaction -- the import is atomic. But a rejected
-- import has to be explainable, days later, to the person who uploaded it --
-- "3 errors" with no way to see which lines helps nobody. The resolution:
-- "nothing" means no transaction, not no record. This row is written on
-- every outcome, rejection included, because it is a receipt of what the
-- system did, not a record of the customer's money.

-- +goose Up

CREATE TABLE import_validations (
    id       uuid NOT NULL DEFAULT gen_random_uuid(),
    org_id   uuid NOT NULL,
    batch_id uuid NOT NULL,

    outcome text NOT NULL CHECK (outcome IN ('valid', 'valid_with_warnings', 'rejected')),

    row_count     integer NOT NULL CHECK (row_count >= 0),
    error_count   integer NOT NULL CHECK (error_count >= 0),
    warning_count integer NOT NULL CHECK (warning_count >= 0),

    -- Tri-state on purpose. TRUE reconciled, FALSE did not, NULL the file
    -- declared no balances -- 1C exports often do not. A boolean NOT NULL
    -- would force "no balances declared" to be recorded as "did not
    -- reconcile", and a report would then carry a warning the customer
    -- cannot act on.
    balance_check_passed boolean NULL,

    -- Codes, line numbers and field names. Never a rendered sentence: the
    -- backend returns error codes and translation is the client's
    -- (CLAUDE.md). A structure test asserts no value in here is a message.
    report_jsonb jsonb NOT NULL,

    -- The override. Populated together or not at all, and only over
    -- warnings -- see the constraint below, which is what actually enforces
    -- that, not this column's own nullability.
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

    -- D1: a correctness error can never be overridden, as a constraint
    -- rather than as a code path in a handler that survives only as long as
    -- nobody refactors it.
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

-- The read change 4.1 needs: which batches in a period were overridden, so a
-- report can show it (design D4). A report line drawn from an overridden
-- batch with no visible mark on it is the omission this index exists to make
-- cheap to avoid, not to cause.
CREATE INDEX import_validations_overridden_idx ON import_validations (org_id, overridden_at)
    WHERE overridden_at IS NOT NULL;

ALTER TABLE import_validations ENABLE ROW LEVEL SECURITY;
ALTER TABLE import_validations FORCE  ROW LEVEL SECURITY;

CREATE POLICY import_validations_tenant ON import_validations FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

-- An override is written once. Re-deciding it silently is the one edit that
-- would change a printed report with no trace, so the writable column set is
-- narrowed rather than trusted to a code path -- the same shape migration
-- 008 uses for import_batches.
REVOKE UPDATE ON import_validations FROM vekst_app;
GRANT UPDATE (overridden_by, override_reason, overridden_at) ON import_validations TO vekst_app;

-- +goose Down

REVOKE UPDATE (overridden_by, override_reason, overridden_at) ON import_validations FROM vekst_app;
GRANT UPDATE ON import_validations TO vekst_app;

DROP TABLE import_validations;
