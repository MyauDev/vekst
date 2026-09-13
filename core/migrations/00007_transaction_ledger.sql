-- The canonical ledger. Change 2.5, capability `transaction-ledger`.
--
-- Three tables, and the one in the middle is where a wrong decision prints a
-- wrong number six months later rather than failing a test today. Most of what
-- follows is constraints that make a wrong row impossible to store, rather than
-- columns describing a right one.
--
-- `import_batches` is here in the shape `transactions` needs to point at, and
-- no larger. Change 2.1 owns the upload, the limits, the endpoint and the state
-- machine that gives `status` its meaning; a composite foreign key needs its
-- parent to exist, and a ledger row that cannot say which import produced it is
-- not a ledger row. See the change's proposal for the alternatives.

-- +goose Up

CREATE TABLE import_batches (
    org_id      uuid        NOT NULL,
    id          uuid        NOT NULL DEFAULT gen_random_uuid(),
    entity_id   uuid        NOT NULL,

    -- ledger | bank. The most load-bearing column in the schema: a report line
    -- is computed from one of these and never from both, because mixing them
    -- counts an invoice and its payment twice.
    source_kind text        NOT NULL CHECK (source_kind IN ('ledger', 'bank')),

    -- One value, deliberately. Change 2.1 adds the rest of the state machine;
    -- until then the only state this table can be in is the one rows are
    -- created in, and a CHECK admitting exactly one value makes widening it a
    -- visible edit rather than a silent drift.
    status      text        NOT NULL DEFAULT 'persisted'
                            CHECK (status IN ('persisted')),
    created_at  timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (org_id, id),
    FOREIGN KEY (org_id, entity_id) REFERENCES entities (org_id, id) ON DELETE RESTRICT
);

-- ---------------------------------------------------------------------------

CREATE TABLE transactions (
    org_id       uuid        NOT NULL,
    id           uuid        NOT NULL DEFAULT gen_random_uuid(),
    entity_id    uuid        NOT NULL,
    account_id   uuid        NOT NULL,
    batch_id     uuid        NOT NULL,

    -- Denormalised from the batch on purpose. Every report query filters on
    -- it, and reaching through batch_id to discover which half of the business
    -- a row belongs to is a join somebody eventually writes without -- a
    -- failure no test catches, because the join that was never written cannot
    -- be asserted on. A constraint trigger below holds the two copies equal.
    source_kind  text        NOT NULL CHECK (source_kind IN ('ledger', 'bank')),

    -- The grain, ARCHITECTURE.md 5.0. A bank payment is one row with a NULL
    -- document_ref and posting_no 0. A ledger document with five postings is
    -- five rows sharing one document_ref, numbered 1..5. Never one row per
    -- document with its line items in JSONB.
    document_ref text,
    posting_no   integer     NOT NULL DEFAULT 0 CHECK (posting_no >= 0),

    booked_on    date        NOT NULL,
    value_on     date,
    -- Derived, not stored beside the amount it describes. Two unconstrained
    -- copies of one fact can disagree with no error and whichever the query
    -- happens to read wins -- which is the reasoning migration 004 used to
    -- keep the reporting currency in one place, and it applies here exactly.
    --
    -- The vocabulary is load-bearing: all 71 rules seeded by migration 006
    -- match this column against 'income' and 'expense', and so do the
    -- classifier's field switch and the internal contract. Generating 'in' and
    -- 'out' instead would stop every rule firing, silently.
    --
    -- >= rather than >, so a zero-amount row -- a waived fee, a correction --
    -- is not called an expense on no evidence.
    direction    text        GENERATED ALWAYS AS
                 (CASE WHEN amount_minor >= 0 THEN 'income' ELSE 'expense' END) STORED
                 NOT NULL,

    -- Money. int64 minor units plus an ISO-4217 code, never a float, in any
    -- language. bigint is what sqlc's override in sqlc.yaml maps to int64.
    --
    -- Signed: money in is positive, money out is negative. Every sum in the
    -- system is then a plain sum and a section total needs no CASE. Three
    -- changes depend on it -- 2.6 pairs internal transfers on opposite signs,
    -- the review queue orders by the absolute value of a signed sum, and the
    -- P&L sums a section -- so it is stated in ARCHITECTURE.md 5.0 as well as
    -- enforced by the generated column below.
    amount_minor      bigint NOT NULL,
    currency          text   NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),

    -- The conversion, recorded rather than recomputed: a report that re-derives
    -- a rate at read time can disagree with the one it printed yesterday,
    -- because the rate table moved. numeric, not a float -- a rate is not money
    -- but it multiplies money.
    fx_rate           numeric(20, 10),
    fx_rate_on        date,
    base_amount_minor bigint,
    base_currency     text   CHECK (base_currency ~ '^[A-Z]{3}$'),

    counterparty_raw  text   NOT NULL DEFAULT '',
    counterparty_key  text   NOT NULL DEFAULT '',
    description_raw   text   NOT NULL DEFAULT '',
    description_norm  text   NOT NULL DEFAULT '',

    -- What produced description_norm and counterparty_key. Read from the row
    -- and sent in the classification request, never taken from configuration:
    -- the classifier refuses a version it does not implement, and that refusal
    -- is worth nothing unless the version travelling with the value is the one
    -- that made it. Changing core/internal/normalize is therefore a backfill.
    normalize_version text   NOT NULL,

    -- КНП, Typ operacji, a 1C account code. Empty when the source carried
    -- none. This is the column that makes L0.5 reachable at all.
    regulated_code    text   NOT NULL DEFAULT '',

    bank_ref     text        NOT NULL DEFAULT '',

    -- Nothing computes this yet -- deduplication is change 2.6 -- but the
    -- column and its unique index land now. Adding a unique index to a table
    -- that already holds duplicates is not a migration, it is an incident with
    -- a data-cleaning exercise attached.
    dedup_hash   text        NOT NULL CHECK (length(dedup_hash) > 0),
    created_at   timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (org_id, id),
    FOREIGN KEY (org_id, entity_id)  REFERENCES entities (org_id, id)       ON DELETE RESTRICT,
    FOREIGN KEY (org_id, account_id) REFERENCES accounts (org_id, id)       ON DELETE RESTRICT,
    FOREIGN KEY (org_id, batch_id)   REFERENCES import_batches (org_id, id) ON DELETE RESTRICT,

    CONSTRAINT txn_grain CHECK (
        (document_ref IS NULL AND posting_no = 0)
        OR (document_ref IS NOT NULL AND posting_no > 0)),

    -- Four nullable columns admit sixteen states and exactly two of them mean
    -- anything: the conversion happened, or the row is already in the
    -- organisation's base currency. A row with a rate and no converted amount
    -- is one a report will either skip or convert itself, and both are wrong
    -- without saying so.
    CONSTRAINT txn_fx_is_all_or_nothing CHECK (
        num_nonnulls(fx_rate, fx_rate_on, base_amount_minor, base_currency) IN (0, 4)),

    -- A conversion converts. Storing EUR -> EUR with a rate of 1 would make
    -- "this row was converted" and "this row is in the base currency" the same
    -- state again, which is what the constraint above exists to separate.
    CONSTRAINT txn_fx_is_a_conversion CHECK (
        base_currency IS NULL OR base_currency <> currency),

    CONSTRAINT txn_fx_rate_is_positive CHECK (fx_rate IS NULL OR fx_rate > 0)
);

-- Scoped by org_id, like every uniqueness constraint in this schema: Postgres
-- does not apply row-level security to constraint checks, so an unscoped unique
-- index is a cross-tenant oracle no policy can close.
CREATE UNIQUE INDEX transactions_dedup_idx ON transactions (org_id, dedup_hash);

-- The report read: one entity, one source kind, a date range.
CREATE INDEX transactions_report_idx
    ON transactions (org_id, entity_id, source_kind, booked_on);

-- The postings of one document, in order.
CREATE INDEX transactions_document_idx
    ON transactions (org_id, document_ref, posting_no)
    WHERE document_ref IS NOT NULL;

CREATE INDEX transactions_batch_idx ON transactions (org_id, batch_id);

-- Denormalisation is only safe while the two copies agree. A composite foreign
-- key carrying source_kind would also work, and was rejected: it makes every
-- insert repeat a column it already has, and turns a mismatch into a
-- foreign-key error that names the wrong problem.
-- +goose StatementBegin
CREATE FUNCTION txn_source_kind_matches_batch() RETURNS trigger
    LANGUAGE plpgsql
    AS $fn$
DECLARE
    batch_kind text;
BEGIN
    SELECT source_kind INTO batch_kind
      FROM import_batches
     WHERE org_id = NEW.org_id AND id = NEW.batch_id;

    IF batch_kind IS DISTINCT FROM NEW.source_kind THEN
        RAISE EXCEPTION
            'transaction source_kind % does not match batch % (%)',
            NEW.source_kind, NEW.batch_id, coalesce(batch_kind, 'no such batch')
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$fn$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER txn_source_kind_matches_batch
    AFTER INSERT OR UPDATE OF source_kind, batch_id, org_id ON transactions
    DEFERRABLE INITIALLY IMMEDIATE
    FOR EACH ROW EXECUTE FUNCTION txn_source_kind_matches_batch();

-- base_currency is the second column in this schema to name the currency a
-- report converts into, and migration 004 made "exactly one home" a tested
-- invariant precisely so that a second one could not appear quietly. This is
-- the rule that invariant asks for in exchange.
--
-- The rule is not "these two are always equal" -- it is "a row is written in
-- the organisation's base currency as it stands at the time of writing." A row
-- already stored carries the currency its base_amount_minor is actually
-- denominated in, which is the right value for that row and not a
-- disagreement. So an organisation changing its base currency is a visible
-- backfill of stored rows plus a re-conversion, rather than a silent
-- reinterpretation of every amount ever converted.
--
-- Without the column there is no honest alternative: base_amount_minor would be
-- an integer with no code beside it, which is the one thing money is never
-- allowed to be.
-- +goose StatementBegin
CREATE FUNCTION txn_base_currency_is_the_orgs() RETURNS trigger
    LANGUAGE plpgsql
    AS $fn$
DECLARE
    org_currency text;
BEGIN
    IF NEW.base_currency IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT base_currency INTO org_currency
      FROM organizations WHERE id = NEW.org_id;

    IF org_currency IS DISTINCT FROM NEW.base_currency THEN
        RAISE EXCEPTION
            'base_currency % is not the organisation''s reporting currency (%)',
            NEW.base_currency, coalesce(org_currency, 'unreadable')
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$fn$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER txn_base_currency_is_the_orgs
    AFTER INSERT OR UPDATE OF base_currency, org_id ON transactions
    DEFERRABLE INITIALLY IMMEDIATE
    FOR EACH ROW EXECUTE FUNCTION txn_base_currency_is_the_orgs();

-- ---------------------------------------------------------------------------
-- classifications. Append-only, and enforced rather than documented.
-- ---------------------------------------------------------------------------

CREATE TABLE classifications (
    org_id          uuid        NOT NULL,
    id              uuid        NOT NULL DEFAULT gen_random_uuid(),
    transaction_id  uuid        NOT NULL,

    -- No foreign key, and that is not an oversight: categories is shared and
    -- tenant at once, so this may point at a row whose org_id is NULL, which no
    -- composite key can express. The constraint trigger below says what a
    -- foreign key cannot -- and raises the same error whether the category
    -- belongs to somebody else or does not exist.
    category_id     uuid        NOT NULL,

    engine_layer    text        NOT NULL
                    CHECK (engine_layer IN ('L0', 'L0.5', 'L1', 'L2', 'human')),
    confidence      numeric(4, 3) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),

    -- Why, in a form a person can read: the counterparty key tier that
    -- matched, or the rule's scope.
    evidence        text        NOT NULL DEFAULT '',

    -- The three strings a report pins, plus the one that says how the text
    -- this matched on was produced. All four on every row, because a report is
    -- reproducible only if every input to it is recorded beside the output --
    -- and an accountant will ask.
    taxonomy_version  text      NOT NULL,
    ruleset_version   text      NOT NULL,
    engine_version    text      NOT NULL,
    normalize_version text      NOT NULL,

    decided_by      uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    decided_at      timestamptz NOT NULL DEFAULT now(),

    -- Two ways a classification stops being the current answer, and they are
    -- not the same event.
    --
    -- superseded_by names the classification that replaced it: a correction,
    -- where some other answer is now current.
    --
    -- retracted_at says the answer was withdrawn and none took its place, which
    -- is what a review decision being undone means. Supersession cannot express
    -- it -- there is no successor to point at, and pointing at itself is
    -- refused below -- and a report needs the difference: a corrected row has a
    -- category, a retracted one is back in the queue.
    superseded_by   uuid        NULL,
    retracted_at    timestamptz NULL,
    retracted_by    uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,

    PRIMARY KEY (org_id, id),
    FOREIGN KEY (org_id, transaction_id)
        REFERENCES transactions (org_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (org_id, superseded_by)
        REFERENCES classifications (org_id, id) ON DELETE RESTRICT,

    -- A machine decision has no decider and a human one must name theirs.
    CONSTRAINT cls_human_has_a_decider CHECK (
        engine_layer <> 'human' OR decided_by IS NOT NULL),

    CONSTRAINT cls_does_not_supersede_itself CHECK (superseded_by <> id),

    CONSTRAINT cls_retraction_is_whole CHECK (
        num_nonnulls(retracted_at, retracted_by) IN (0, 2)),

    -- A row is replaced or withdrawn, never both: the two describe different
    -- fates and a row with both is one no report can classify.
    CONSTRAINT cls_is_not_both CHECK (
        superseded_by IS NULL OR retracted_at IS NULL)
);

-- "The current classification" is a single row by construction. Without this
-- it is a convention every query re-implements, and the first query that gets
-- it wrong prints a transaction twice.
CREATE UNIQUE INDEX classifications_one_live_idx
    ON classifications (org_id, transaction_id)
    WHERE superseded_by IS NULL AND retracted_at IS NULL;

CREATE INDEX classifications_txn_idx ON classifications (org_id, transaction_id);

CREATE CONSTRAINT TRIGGER classifications_category_is_visible
    AFTER INSERT OR UPDATE OF category_id, org_id ON classifications
    DEFERRABLE INITIALLY IMMEDIATE
    FOR EACH ROW EXECUTE FUNCTION rules_category_is_visible();

-- Migration 006 defined that function for rules and vendors, and it is reused
-- unchanged here rather than copied with a better noun in its message. The
-- check is the same check -- visible, a leaf, not computed -- and one
-- definition of "a category something may target" is worth more than a precise
-- word in an error a test asserts by SQLSTATE.

-- ---------------------------------------------------------------------------
-- Append-only, as grants rather than as a comment.
--
-- 00001's default privileges give vekst_app SELECT, INSERT, UPDATE and DELETE
-- on every table this role creates. For this table that is three rights too
-- many: a correction inserts a new row and points the old one at it, so the
-- only column that ever changes after a write is superseded_by.
-- ---------------------------------------------------------------------------

REVOKE UPDATE, DELETE ON classifications FROM vekst_app;
GRANT UPDATE (superseded_by, retracted_at, retracted_by) ON classifications TO vekst_app;

-- ---------------------------------------------------------------------------
-- Row-level security. Three ordinary tenant tables -- none of them has rows
-- belonging to nobody, so none needs the split read/write policies migrations
-- 005 and 006 use, and none belongs on the shared-tenant allowlist.
-- ---------------------------------------------------------------------------

ALTER TABLE import_batches  ENABLE ROW LEVEL SECURITY;
ALTER TABLE import_batches  FORCE  ROW LEVEL SECURITY;
ALTER TABLE transactions    ENABLE ROW LEVEL SECURITY;
ALTER TABLE transactions    FORCE  ROW LEVEL SECURITY;
ALTER TABLE classifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE classifications FORCE  ROW LEVEL SECURITY;

CREATE POLICY import_batches_tenant ON import_batches FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

CREATE POLICY transactions_tenant ON transactions FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

CREATE POLICY classifications_tenant ON classifications FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

-- +goose Down

DROP TABLE classifications;
DROP TABLE transactions;
DROP FUNCTION txn_base_currency_is_the_orgs();
DROP FUNCTION txn_source_kind_matches_batch();
DROP TABLE import_batches;
