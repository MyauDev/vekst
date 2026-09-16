-- One column: where in the file a transaction came from. Change 4.2,
-- capability `report-mgmt-pnl`.
--
-- A customer checking a figure opens their own file. A drill-down that says
-- "412,000 across these nine payments" without saying which line of march.csv
-- each came from is asking them to match on amount and date and hope -- which
-- is the thing they were paying us not to do.
--
-- The number already exists. `ingest.Row.LineNo` is the line in the *original*
-- file rather than the index of a parsed row, and
-- `TestLineNumbersReferToTheOriginalFile` pins that distinction; migration 009
-- stores it on `raw_rows` and 012 stores it on `dedup_skips`. It was dropped at
-- exactly one boundary -- the transaction insert -- and this migration plus the
-- persist path is where that stops.
--
-- Beside `batch_id`, because the pair is the row's provenance and either alone
-- is half an answer: a batch says which file, a line says where in it.
--
-- ---------------------------------------------------------------------------
-- Why this is a separate migration and not an edit to 007
--
-- 007 is merged. A goose migration that has run somewhere is history, and
-- editing history is how two databases claiming the same version stop having
-- the same schema.
--
-- Why it is NOT NULL with no backfill: there is nothing to backfill from. A
-- transaction carries no pointer to the `raw_rows` line it came from,
-- deduplication means not every raw line became a transaction, and one raw line
-- can become several postings -- so no join recovers the number for a row
-- written before this migration. The alternatives are a sentinel or a guess,
-- and both put a number in a column a customer will read as the line in their
-- file. A missing provenance that says so is better than a confident wrong one.
--
-- So the column is NOT NULL, and the migration refuses rather than invents. No
-- environment holds rows this costs anything: there is no deployment, and the
-- rows a developer has are their own test data. That is a one-time cost paid by
-- two people before launch, which is the cheapest this ever gets.

-- +goose Up

ALTER TABLE transactions ADD COLUMN line_no integer;

-- The assertion below counts rows across every organisation, and `transactions`
-- is FORCE'd -- so the migrator is subject to the policy like anybody else, and
-- `app_current_org()` raises 42704 in a migration that binds to no tenant. A
-- window is the only way to ask "is this table empty" at all; binding to one
-- organisation instead would answer a different question and answer it wrongly,
-- because it is precisely the rows belonging to somebody else that this must
-- not miss. Reinstated below, and the RLS coverage test fails if it is not.
ALTER TABLE transactions NO FORCE ROW LEVEL SECURITY;

-- +goose StatementBegin
DO $$
DECLARE
    stranded integer;
BEGIN
    SELECT count(*) INTO stranded FROM transactions WHERE line_no IS NULL;
    IF stranded <> 0 THEN
        RAISE EXCEPTION
            '% transactions predate line_no and nothing can supply it; recreate the database (there is no deployment this applies to) rather than letting a sentinel stand in for a line in a customer''s file',
            stranded
            USING ERRCODE = '23502';
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE transactions ALTER COLUMN line_no SET NOT NULL;

ALTER TABLE transactions FORCE ROW LEVEL SECURITY;

-- 1-based, like every line number a person reads. 0 would be the sentinel this
-- migration's header refuses.
ALTER TABLE transactions ADD CONSTRAINT txn_line_no_is_a_line CHECK (line_no > 0);

-- The drill-down's own read is by (batch, line): "show me this figure's rows,
-- in the order they appear in the file I uploaded".
CREATE INDEX transactions_provenance_idx
    ON transactions (org_id, batch_id, line_no, posting_no);

-- +goose Down

DROP INDEX transactions_provenance_idx;
ALTER TABLE transactions DROP CONSTRAINT txn_line_no_is_a_line;
ALTER TABLE transactions DROP COLUMN line_no;
