-- Change 2.1, add-file-upload: the rest of an import batch.
--
-- This is an ALTER, not a CREATE, and that inverts the order
-- docs/IMPLEMENTATION_PLAN.md §3 implies. Migration 00007 (change 2.5) creates
-- import_batches in the minimal shape transactions needs to point at, already
-- enables and FORCEs row-level security on it, and already carries the
-- import_batches_tenant policy. Its status column holds a CHECK admitting
-- exactly one value, and its own comment says why:
--
--   "Change 2.1 adds the rest of the state machine; until then the only state
--    this table can be in is the one rows are created in, and a CHECK admitting
--    exactly one value makes widening it a visible edit rather than a silent
--    drift."
--
-- This migration is that widening. It adds NO policy: 00007's is correct, and a
-- second definition of one rule is how two definitions drift.

-- +goose Up

-- ---------------------------------------------------------------------------
-- §0  The table must be empty
-- ---------------------------------------------------------------------------

-- Every column below arrives NOT NULL with no default, which is only safe on an
-- empty table. 00007 has not been deployed anywhere that persists a batch, so
-- this asserts that rather than assuming it: a non-empty table here means
-- somebody imported against a schema this migration is about to move, and the
-- right answer is to stop rather than to invent values for their rows.
--
-- FORCE ROW LEVEL SECURITY applies to vekst_migrator too (00004 §3), so this
-- count would normally raise 42704 for want of a tenant context. The migrator
-- owns the table, so NO FORCE / restore is the idiom 00004 design D8 records
-- for exactly this case: a migration that legitimately has to see every row.
-- +goose StatementBegin
DO $$
DECLARE
    n bigint;
BEGIN
    ALTER TABLE import_batches NO FORCE ROW LEVEL SECURITY;
    SELECT count(*) INTO n FROM import_batches;
    ALTER TABLE import_batches FORCE ROW LEVEL SECURITY;

    IF n > 0 THEN
        RAISE EXCEPTION 'import_batches holds % row(s); change 2.1 assumes it has never been written to', n
            USING HINT = 'migration 00007 created this table for change 2.5 and nothing was supposed to write it yet';
    END IF;
END $$;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- §1  The columns an upload needs
-- ---------------------------------------------------------------------------

ALTER TABLE import_batches
    -- users is global and outside row-level security (ARCHITECTURE.md §5.5). A
    -- plain reference is right here for the same reason memberships carries
    -- one: the target is not tenant data, so the check discloses nothing about
    -- another tenant.
    ADD COLUMN uploaded_by uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,

    -- What the browser asked for. These three are signing conditions, never
    -- facts: core never sees the bytes, so nothing downstream may read them.
    -- §2 records what core measured instead.
    ADD COLUMN file_name      text   NOT NULL CHECK (length(btrim(file_name)) > 0),
    ADD COLUMN declared_bytes bigint NOT NULL CHECK (declared_bytes > 0),
    ADD COLUMN declared_type  text   NOT NULL,

    -- The object key, built by core from org_id and id. Never from file_name: a
    -- user-supplied name is a path traversal and a collision at the same time.
    -- file_name is kept only to show the customer what they uploaded.
    ADD COLUMN file_key text NOT NULL,

    -- What core measured after reading the object back out of the store. NULL
    -- until the measurement job has run; §3's constraint is what stops a later
    -- stage treating that NULL as "no duplicate found".
    ADD COLUMN file_sha256  bytea  NULL CHECK (file_sha256 IS NULL OR length(file_sha256) = 32),
    ADD COLUMN byte_length  bigint NULL CHECK (byte_length IS NULL OR byte_length > 0),
    ADD COLUMN content_type text   NULL,

    -- Filled by later changes. 2.2 sets row_count; any stage may set
    -- failure_code, which is a code and never a sentence (CLAUDE.md: the
    -- backend returns error codes, translation is the client's).
    ADD COLUMN row_count    integer NULL CHECK (row_count IS NULL OR row_count >= 0),
    ADD COLUMN failure_code text    NULL,

    ADD COLUMN upload_expires_at timestamptz NOT NULL,
    ADD COLUMN updated_at        timestamptz NOT NULL DEFAULT now();

-- ---------------------------------------------------------------------------
-- §2  The state machine
-- ---------------------------------------------------------------------------

-- 'persisted' is 00007's single placeholder and it is dropped rather than
-- carried forward: it names the same state as 'imported', and two names for one
-- state is precisely the drift that narrow CHECK existed to make visible. §0
-- proved the table is empty, so nothing is migrated -- only redefined.
--
-- The default goes with it. A batch's first state is awaiting_upload and it is
-- set deliberately by the RPC that reserves the batch; a column default would
-- let a row arrive in a state nobody chose.
--
-- Eleven states, declared in one place. ARCHITECTURE.md §4a draws this pipeline
-- and calls it fixed, so this is writing down a decision already taken rather
-- than speculating about unwritten changes. 2.2, 2.3 and 2.5 drive the states
-- they own and none of them adds a value.
--
--   awaiting_upload ─┬─▶ uploaded ─▶ parsing ─▶ parsed ─▶ validating ─┬─▶ validated ─▶ persisting ─▶ imported
--                    │                                                └─▶ rejected
--                    └─▶ abandoned                    any stage ──────────▶ failed
--
-- rejected and failed are deliberately different states. rejected is a
-- validation outcome and is shown to the customer as their file's problem;
-- failed is a defect in us and carries a failure_code. One name for both would
-- put our bugs in front of the customer as their mistake.
ALTER TABLE import_batches DROP CONSTRAINT import_batches_status_check;
ALTER TABLE import_batches ALTER COLUMN status DROP DEFAULT;
ALTER TABLE import_batches ADD CONSTRAINT import_batches_status_check CHECK (status IN (
    'awaiting_upload', 'abandoned', 'uploaded',
    'parsing', 'parsed', 'validating', 'validated', 'rejected',
    'persisting', 'imported', 'failed'));

-- ---------------------------------------------------------------------------
-- §3  A batch past the upload has been measured -- with one exception
-- ---------------------------------------------------------------------------

-- The point of this constraint is that the measurement job cannot be skipped:
-- with core never touching the bytes, these three columns are the only true
-- facts about the file, and a stage that reads a NULL sha256 as "no duplicate
-- found" is how the same file imports twice.
--
-- 'failed' has to be exempt, and that exception is load-bearing rather than
-- convenient. A batch whose object is missing from the store fails BEFORE it
-- can be measured -- upload_missing, the first failure the design names. An
-- earlier draft omitted it, and against PostgreSQL 16 that refused exactly
-- that row:
--
--   ERROR:  new row violates check constraint "import_batches_measured_past_upload"
--   DETAIL: Failing row contains (failed, null, null, null).
--
-- A batch that fails AFTER measurement keeps its measurements. failure_code is
-- what distinguishes the two cases, not the nullity of these three columns.
ALTER TABLE import_batches ADD CONSTRAINT import_batches_measured_past_upload CHECK (
    status IN ('awaiting_upload', 'abandoned', 'failed')
    OR (file_sha256 IS NOT NULL AND byte_length IS NOT NULL AND content_type IS NOT NULL));

-- ---------------------------------------------------------------------------
-- §4  Uniqueness and indexes
-- ---------------------------------------------------------------------------

-- Scoped by org_id like every uniqueness constraint in this schema, because
-- referential-integrity checks are not subject to row-level security (00004
-- design D1) and an unscoped one is a cross-tenant oracle. The key embeds a
-- random uuid, so two organisations cannot collide in the bucket whatever this
-- constraint scopes; scoping it is about the oracle, not about collisions.
--
-- Deliberately NOT added: UNIQUE (org_id, id, source_kind). An earlier draft of
-- change 2.1 added it so that 2.5 could hang a composite foreign key off it and
-- make transactions.source_kind provably equal to its batch's. 00007 solved the
-- same problem with the txn_source_kind_matches_batch constraint trigger, which
-- needs nothing from here, so the constraint would now be a redundant index on
-- a tenant table.
ALTER TABLE import_batches ADD CONSTRAINT import_batches_file_key_unique UNIQUE (org_id, file_key);

CREATE INDEX import_batches_status_idx ON import_batches (org_id, status, created_at DESC);
CREATE INDEX import_batches_entity_idx ON import_batches (org_id, entity_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- §5  source_kind is immutable
-- ---------------------------------------------------------------------------

-- A report line is computed from one source kind and the basis label -- cash or
-- accrual -- is derived from it rather than chosen (ARCHITECTURE.md §5.1). If a
-- batch's source_kind could be edited after its rows were persisted, every
-- report already printed from it would change basis with no event recorded.
--
-- The customer's remedy is to abandon the batch and upload again, which is
-- correct: it is a different import.
--
-- A column-level REVOKE alone does NOT do this, and the difference is invisible
-- until it is tested. Postgres holds table-level and column-level privileges
-- separately: 00001's default privileges grant vekst_app UPDATE on the whole
-- table, and `REVOKE UPDATE (source_kind)` only removes a column-level grant
-- that was never issued. Verified against PostgreSQL 16 -- after that statement,
-- has_column_privilege('vekst_app','import_batches','source_kind','UPDATE')
-- still returned true.
--
-- The table-level grant has to go first, and then the columns that legitimately
-- change after creation are granted back one by one. That is a stronger rule
-- than the one this section set out to write: source_kind is immutable, and so
-- is every other column describing what the customer uploaded. Only the
-- pipeline's own bookkeeping moves.
REVOKE UPDATE ON import_batches FROM vekst_app;
GRANT UPDATE (status, file_sha256, byte_length, content_type,
              row_count, failure_code, updated_at) ON import_batches TO vekst_app;

-- +goose Down

-- Restore the table-level UPDATE that 00001's default privileges gave, and drop
-- the column-level grants -- leaving them would let a later migration think the
-- narrowing is still in force when it is not.
REVOKE UPDATE (status, file_sha256, byte_length, content_type,
               row_count, failure_code, updated_at) ON import_batches FROM vekst_app;
GRANT UPDATE ON import_batches TO vekst_app;

DROP INDEX IF EXISTS import_batches_entity_idx;
DROP INDEX IF EXISTS import_batches_status_idx;

ALTER TABLE import_batches DROP CONSTRAINT IF EXISTS import_batches_file_key_unique;
ALTER TABLE import_batches DROP CONSTRAINT IF EXISTS import_batches_measured_past_upload;

ALTER TABLE import_batches DROP COLUMN IF EXISTS updated_at;
ALTER TABLE import_batches DROP COLUMN IF EXISTS upload_expires_at;
ALTER TABLE import_batches DROP COLUMN IF EXISTS failure_code;
ALTER TABLE import_batches DROP COLUMN IF EXISTS row_count;
ALTER TABLE import_batches DROP COLUMN IF EXISTS content_type;
ALTER TABLE import_batches DROP COLUMN IF EXISTS byte_length;
ALTER TABLE import_batches DROP COLUMN IF EXISTS file_sha256;
ALTER TABLE import_batches DROP COLUMN IF EXISTS file_key;
ALTER TABLE import_batches DROP COLUMN IF EXISTS declared_type;
ALTER TABLE import_batches DROP COLUMN IF EXISTS declared_bytes;
ALTER TABLE import_batches DROP COLUMN IF EXISTS file_name;
ALTER TABLE import_batches DROP COLUMN IF EXISTS uploaded_by;

-- Restore 00007's shape exactly: one value, and the default that goes with it.
-- A down migration that leaves the eleven states in place would let 2.5's own
-- tests pass against a schema 2.5 never wrote.
ALTER TABLE import_batches DROP CONSTRAINT import_batches_status_check;
ALTER TABLE import_batches ALTER COLUMN status SET DEFAULT 'persisted';
ALTER TABLE import_batches ADD CONSTRAINT import_batches_status_check CHECK (status IN ('persisted'));
