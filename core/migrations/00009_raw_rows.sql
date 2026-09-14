-- Change 2.2, add-statement-parsing task 4.1: raw_rows, ARCHITECTURE.md §5.5.
--
-- The object store already keeps the original file (add-file-upload), so
-- this table is not about durability -- it is about being able to see what
-- the parser decided for one line without re-parsing the file through
-- whatever version of the parser happens to be deployed today. A report is
-- reproducible only if what it was built from is what the parser actually
-- read at the time, not what a later bug-fixed parser would read now.

-- +goose Up

CREATE TABLE raw_rows (
    org_id   uuid    NOT NULL,
    id       uuid    NOT NULL DEFAULT gen_random_uuid(),
    batch_id uuid    NOT NULL,

    -- The 1-based line in the original file. core/internal/ingest.Row.LineNo,
    -- unchanged -- this is the same number an error report keys by
    -- (CLAUDE.md: never by parsed row index).
    line_no integer NOT NULL CHECK (line_no > 0),

    -- The parsed cells (core/internal/ingest.Row, JSON-marshaled), not the
    -- raw decoded text. Design task 4.3: the whole decoded text adds nothing
    -- a re-decode of the object store's original bytes cannot reproduce
    -- exactly, since decoding is a pure function of those bytes -- but the
    -- parser's own extraction logic can change between now and a later
    -- question about this row, and payload_jsonb is the snapshot of what it
    -- decided when this batch was actually parsed.
    payload_jsonb jsonb NOT NULL,

    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (org_id, id),
    FOREIGN KEY (org_id, batch_id) REFERENCES import_batches (org_id, id) ON DELETE RESTRICT,

    -- One row per line per batch. A parse that runs twice -- a retried job,
    -- an idempotent re-run -- must not double the table; the caller upserts
    -- or the second run is a no-op, either way this is what makes a bug in
    -- that decision loud rather than a silent duplicate.
    UNIQUE (org_id, batch_id, line_no)
);

-- The read every future consumer does: one batch's rows, in order.
CREATE INDEX raw_rows_batch_idx ON raw_rows (org_id, batch_id, line_no);

ALTER TABLE raw_rows ENABLE ROW LEVEL SECURITY;
ALTER TABLE raw_rows FORCE ROW LEVEL SECURITY;

CREATE POLICY raw_rows_tenant ON raw_rows FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

-- +goose Down

DROP TABLE raw_rows;
