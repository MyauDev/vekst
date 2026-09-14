-- Change 2.4, add-import-profiles: import_profiles, and the batch's
-- reference to one.
--
-- A profile overrides field by field (design D1): every column here except
-- name, source_kind and column_map is nullable, and NULL means "let the
-- detector decide this one." column_map is the exception -- there is no
-- useful partial column map, the same reason a detector cannot guess which
-- column is the amount.

-- +goose Up

CREATE TABLE import_profiles (
    id     uuid NOT NULL DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,

    name        text NOT NULL CHECK (length(btrim(name)) > 0),
    source_kind text NOT NULL CHECK (source_kind IN ('ledger', 'bank')),

    -- Source column name -> canonical field. Keys are the source's own
    -- words and cannot be constrained; values come from the fixed list
    -- below, enforced by the trigger, not by this column's own type.
    column_map jsonb NOT NULL,

    charset     text NULL,
    delimiter   text NULL CHECK (delimiter IS NULL OR length(delimiter) = 1),
    decimal_sep text NULL CHECK (decimal_sep IN (',', '.')),
    date_fmt    text NULL,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (org_id, id),
    -- Scoped by org_id like every uniqueness constraint in this schema
    -- (referential-integrity checks bypass row-level security, migration
    -- 00004 design D1).
    UNIQUE (org_id, name)
);

ALTER TABLE import_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE import_profiles FORCE  ROW LEVEL SECURITY;

CREATE POLICY import_profiles_tenant ON import_profiles FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

-- design D2: column_map's values name a canonical field or the write fails.
-- The list is the fourteen names frozen in core/internal/ingest.CanonicalFields
-- (task 0.1) -- TestCanonicalFieldListMatchesTheTrigger fails the day this
-- array and that Go slice disagree, since nothing mechanically generates one
-- from the other.
--
-- A typo here -- "descrption" -- is otherwise a column that maps to nothing,
-- a description that fails the "present after trimming" correctness check
-- on every row, and a rejected file whose error report blames the
-- customer's data for a mistake made when the profile was written.
-- +goose StatementBegin
CREATE FUNCTION import_profiles_column_map_is_canonical() RETURNS trigger
    LANGUAGE plpgsql
    AS $fn$
DECLARE
    canonical_fields text[] := ARRAY[
        'booked_on', 'value_on', 'amount', 'debit', 'credit', 'currency',
        'description', 'counterparty', 'bank_ref', 'document_ref', 'posting_no',
        'opening_balance', 'closing_balance', 'row_count_declared'
    ];
    bad_field text;
BEGIN
    SELECT value INTO bad_field
      FROM jsonb_each_text(NEW.column_map)
     WHERE value != ALL (canonical_fields)
     LIMIT 1;

    IF bad_field IS NOT NULL THEN
        RAISE EXCEPTION 'column_map names % which is not a canonical field', bad_field
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$fn$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER import_profiles_column_map_is_canonical
    AFTER INSERT OR UPDATE OF column_map ON import_profiles
    DEFERRABLE INITIALLY IMMEDIATE
    FOR EACH ROW EXECUTE FUNCTION import_profiles_column_map_is_canonical();

-- The batch's reference to the profile it was created with. Composite, like
-- every foreign key between tenant tables (migration 00004 design D1) --
-- an unscoped one is a cross-tenant existence oracle. ON DELETE RESTRICT:
-- deleting a profile would delete the record of how an existing batch's
-- numbers were produced (task 6.9).
--
-- ADD CONSTRAINT validates existing rows regardless of whether a NULL
-- foreign key value would trivially pass -- and that validation scan reads
-- import_batches under its own FORCE'd policy, which applies to
-- vekst_migrator too and raises with no tenant context set (the same
-- gotcha migration 008's emptiness check exists for). NO FORCE / FORCE
-- brackets it the same way.
ALTER TABLE import_batches ADD COLUMN import_profile_id uuid NULL;
-- +goose StatementBegin
DO $$ BEGIN
    -- Both tables: the child being altered and the parent the new foreign
    -- key validates against. import_profiles is empty (just created above),
    -- but the validation scan still evaluates its policy to find that out.
    ALTER TABLE import_batches  NO FORCE ROW LEVEL SECURITY;
    ALTER TABLE import_profiles NO FORCE ROW LEVEL SECURITY;
    ALTER TABLE import_batches ADD FOREIGN KEY (org_id, import_profile_id)
        REFERENCES import_profiles (org_id, id) ON DELETE RESTRICT;
    ALTER TABLE import_batches  FORCE ROW LEVEL SECURITY;
    ALTER TABLE import_profiles FORCE ROW LEVEL SECURITY;
END $$;
-- +goose StatementEnd

-- design D1's cost: a profile is not a complete description of a file, so
-- what a batch was actually parsed with has to be recorded on the batch
-- itself, separately from the profile, which may be edited afterward (task
-- 5.3). NULL until the measurement/parse job resolves it.
ALTER TABLE import_batches
    ADD COLUMN resolved_parameters jsonb NULL;

-- Migration 008 revoked table-level UPDATE on import_batches and granted
-- back only the pipeline's own bookkeeping columns. A new column added by
-- ALTER TABLE inherits none of that -- verified on PostgreSQL 16, the same
-- gotcha 008's own comment records -- so it has to be granted explicitly for
-- the job that resolves it to be able to write it at all. import_profile_id
-- needs no such grant: it is set once, at INSERT, which the table-level
-- INSERT grant already covers, and staying out of the UPDATE list is what
-- keeps it immutable afterward.
GRANT UPDATE (resolved_parameters) ON import_batches TO vekst_app;

-- +goose Down

REVOKE UPDATE (resolved_parameters) ON import_batches FROM vekst_app;
ALTER TABLE import_batches DROP COLUMN IF EXISTS resolved_parameters;
ALTER TABLE import_batches DROP COLUMN IF EXISTS import_profile_id;
DROP TABLE import_profiles;
DROP FUNCTION import_profiles_column_map_is_canonical();
