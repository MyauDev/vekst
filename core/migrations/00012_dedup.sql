-- Deduplication and internal-transfer matching. Change 2.6, capability
-- `dedup-and-matching`.
--
-- Numbered 012, not 011 as the proposal's own text says: 011 was taken by
-- add-import-profiles while this change was still open, the same
-- renumbering that change's own migration records for itself.
--
-- D1 (the same file twice) and the loosening of transactions_dedup_idx are
-- in migration 00007, corrected in place (task 0.4) rather than undone
-- here: that index is dedup_hash's own home, and reverting a fix in the
-- migration that shipped it only to redo it here would read as two
-- decisions where there was one.

-- +goose Up

-- D1. A guarantee, not a check-then-act: two concurrent uploads of the same
-- file cannot both reach imported. Partial on status = 'imported': a batch
-- that was rejected or abandoned must be re-uploadable once the customer
-- fixes it, and an upload that never completed must not block a retry.
-- Scoped by org_id: the same public bank template uploaded by two customers
-- is two files, not one.
CREATE UNIQUE INDEX import_batches_file_once
    ON import_batches (org_id, file_sha256)
    WHERE status = 'imported';

-- ---------------------------------------------------------------------------
-- dedup_skips. A skipped row is a row the customer's file contained and
-- their reports do not -- that has to be inspectable, the same reasoning
-- import_validations' own report_jsonb follows. Every skip keeps the line
-- it came from, so it can be found in the original file the way a
-- correctness error already can.
-- ---------------------------------------------------------------------------

CREATE TABLE dedup_skips (
    id       uuid NOT NULL DEFAULT gen_random_uuid(),
    org_id   uuid NOT NULL,
    batch_id uuid NOT NULL,

    line_no    integer  NOT NULL CHECK (line_no > 0),
    posting_no smallint NOT NULL,

    level      text NOT NULL CHECK (level IN ('D2', 'D3')),
    -- text, not bytea: transactions.dedup_hash (migration 007) is text, and
    -- a skip whose hash cannot be compared to the column it came from is
    -- not a record of anything.
    dedup_hash text NOT NULL CHECK (length(dedup_hash) > 0),

    -- D3 only: the row this one matched, and the batch that holds it. NULL
    -- for D2, where the original is another line of this same file.
    matched_transaction_id uuid NULL,
    matched_batch_id       uuid NULL,

    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (org_id, id),
    FOREIGN KEY (org_id, batch_id) REFERENCES import_batches (org_id, id) ON DELETE RESTRICT,
    UNIQUE (org_id, batch_id, line_no, posting_no),
    CONSTRAINT dedup_skips_d3_names_its_original CHECK (
        (level = 'D2' AND matched_transaction_id IS NULL AND matched_batch_id IS NULL) OR
        (level = 'D3' AND matched_transaction_id IS NOT NULL AND matched_batch_id IS NOT NULL))
);

CREATE INDEX dedup_skips_batch_idx ON dedup_skips (org_id, batch_id);

-- A skip is a record of what happened during one persist, like a
-- validation report (migration 010's own reasoning for import_validations).
-- It is written once.
REVOKE UPDATE ON dedup_skips FROM vekst_app;

-- ---------------------------------------------------------------------------
-- Internal transfers. Deliberately not transaction_links (D4, a future
-- change): that table relates two SOURCES describing one event; this
-- relates two EVENTS that cancel. Excluded from the P&L on detection,
-- reversible by dismissal (design D4, confirmed with the founder) -- a
-- missed pair silently inflates revenue, which ARCHITECTURE.md names as the
-- most dangerous failure this mechanism guards against, and a false
-- exclusion is visible in the drill-down rather than silent.
-- ---------------------------------------------------------------------------

CREATE TABLE internal_transfers (
    id     uuid NOT NULL DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,

    out_txn_id uuid NOT NULL,
    in_txn_id  uuid NOT NULL,

    detected_at  timestamptz NOT NULL DEFAULT now(),
    dismissed_by uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    dismissed_at timestamptz NULL,

    PRIMARY KEY (org_id, id),
    FOREIGN KEY (org_id, out_txn_id) REFERENCES transactions (org_id, id) ON DELETE CASCADE,
    FOREIGN KEY (org_id, in_txn_id)  REFERENCES transactions (org_id, id) ON DELETE CASCADE,

    CONSTRAINT internal_transfers_two_sides CHECK (out_txn_id <> in_txn_id),
    CONSTRAINT internal_transfers_dismissal_is_whole CHECK (
        (dismissed_by IS NULL) = (dismissed_at IS NULL))
);

-- The P&L read: every pair not yet dismissed.
CREATE INDEX internal_transfers_active_idx ON internal_transfers (org_id)
    WHERE dismissed_at IS NULL;

-- One row per transaction that is in any pair, on either side. THIS is what
-- enforces "a transaction belongs to at most one pair" -- two separate
-- UNIQUE (org_id, out_txn_id) / UNIQUE (org_id, in_txn_id) constraints on
-- internal_transfers itself do not: verified on PostgreSQL 16 that both
-- happily accept a transaction as the `in` side of one pair and the `out`
-- side of another, since two constraints on two different columns cannot
-- see each other. A single primary key on (org_id, txn_id) here can.
--
-- Both member rows are written in the same statement as the pair itself
-- (application code, not a trigger here), so a pair whose sides are
-- already spoken for fails outright rather than half-inserting.
CREATE TABLE internal_transfer_members (
    org_id      uuid NOT NULL,
    txn_id      uuid NOT NULL,
    transfer_id uuid NOT NULL,
    side        text NOT NULL CHECK (side IN ('out', 'in')),

    PRIMARY KEY (org_id, txn_id),
    FOREIGN KEY (org_id, txn_id)      REFERENCES transactions (org_id, id)       ON DELETE CASCADE,
    FOREIGN KEY (org_id, transfer_id) REFERENCES internal_transfers (org_id, id) ON DELETE CASCADE,

    UNIQUE (org_id, transfer_id, side)
);

-- ---------------------------------------------------------------------------
-- Row-level security. Both new tables are ordinary tenant tables -- no
-- shared rows, no split read/write policy.
-- ---------------------------------------------------------------------------

ALTER TABLE dedup_skips              ENABLE ROW LEVEL SECURITY;
ALTER TABLE dedup_skips              FORCE  ROW LEVEL SECURITY;
ALTER TABLE internal_transfers       ENABLE ROW LEVEL SECURITY;
ALTER TABLE internal_transfers       FORCE  ROW LEVEL SECURITY;
ALTER TABLE internal_transfer_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE internal_transfer_members FORCE  ROW LEVEL SECURITY;

CREATE POLICY dedup_skips_tenant ON dedup_skips FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

CREATE POLICY internal_transfers_tenant ON internal_transfers FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

CREATE POLICY internal_transfer_members_tenant ON internal_transfer_members FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

-- +goose Down

DROP TABLE internal_transfer_members;
DROP TABLE internal_transfers;
DROP TABLE dedup_skips;
DROP INDEX import_batches_file_once;
