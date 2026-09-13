-- The review queue's only table. Change 3.3, capability `review-queue`.
--
-- The queue itself is not stored. A transaction needing review is one with no
-- live classification, which `classifications` already says; a second
-- representation of that fact would drift, and the drift is silent -- a row
-- marked resolved with no classification is absent from the report and from
-- the queue at once.
--
-- What is not derivable is the human act: who decided, covering how many rows,
-- worth how much. One decision covers a counterparty rather than a
-- transaction, and that grouping is the whole reason twelve months of
-- first-time data is settled in fifteen minutes rather than in five hundred
-- keystrokes.

-- +goose Up

CREATE TABLE review_decisions (
    org_id           uuid        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    id               uuid        NOT NULL DEFAULT gen_random_uuid(),

    -- What the decision was about. Produced by core/internal/normalize, and
    -- stored with the version that produced it for the same reason the
    -- transaction stores it: changing that function changes what counts as the
    -- same counterparty, which is a backfill and never a reinterpretation.
    counterparty_key text        NOT NULL,
    key_version      text        NOT NULL,

    -- categorised        -- a category was chosen
    -- internal_transfer  -- one leg of a movement between the org's own accounts
    -- non_pnl            -- outside the P&L, and not a transfer
    -- skipped            -- looked at, deferred, still in the queue
    --
    -- The last three all resolve to the seeded leaf 09 OUT OF P&L, because
    -- `is_pnl` is what every report line excludes on. The distinction is kept
    -- here rather than in the taxonomy so that change 2.6 can find the pairs
    -- behind an internal_transfer and upgrade a claim into a confirmed link.
    outcome          text        NOT NULL
                     CHECK (outcome IN ('categorised', 'internal_transfer', 'non_pnl', 'skipped')),

    -- No foreign key, for the reason `classifications` gives: categories is
    -- shared and tenant at once, so this may name a row whose org_id is NULL,
    -- which no composite key can express. The trigger below says what the key
    -- cannot.
    category_id      uuid        NULL,

    decided_by       uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    decided_at       timestamptz NOT NULL DEFAULT now(),

    -- What the decision covered when it was made. Recorded because it is what
    -- the user was shown, and a decision has to stay explicable in the terms it
    -- was taken in -- tomorrow's import adds rows for the same counterparty,
    -- and those are answered by vendor memory rather than by this decision.
    covered_count    integer     NOT NULL CHECK (covered_count >= 0),
    covered_minor    bigint      NOT NULL,
    covered_currency text        NOT NULL CHECK (covered_currency ~ '^[A-Z]{3}$'),

    undone_at        timestamptz NULL,
    undone_by        uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,

    PRIMARY KEY (org_id, id),

    CONSTRAINT decision_category_matches_outcome CHECK (
        (outcome = 'categorised') = (category_id IS NOT NULL)),

    CONSTRAINT decision_undo_is_whole CHECK (
        num_nonnulls(undone_at, undone_by) IN (0, 2)),

    CONSTRAINT decision_key_is_not_blank CHECK (length(key_version) > 0)
);

-- One live decision per counterparty. A correction undoes the first rather
-- than sitting beside it, so "what did we decide about this vendor" has one
-- answer rather than a list to interpret.
CREATE UNIQUE INDEX review_decisions_one_live_idx
    ON review_decisions (org_id, key_version, counterparty_key)
    WHERE undone_at IS NULL;

CREATE INDEX review_decisions_recent_idx
    ON review_decisions (org_id, decided_at DESC);

-- A decision may name a category only on the same terms a classification may:
-- visible to this organisation, a leaf, and not computed. Migration 006
-- defined that check for rules and vendors and migration 007 reused it for
-- classifications; this is its fourth caller and the reason it is a function.
--
-- It runs only when there is a category to check. The three non-categorised
-- outcomes carry NULL, which the constraint above already pairs with the
-- outcome, so a NULL here is a decision that deliberately named no category.
-- +goose StatementBegin
CREATE FUNCTION review_decision_category_is_visible() RETURNS trigger
    LANGUAGE plpgsql
    AS $fn$
BEGIN
    IF NEW.category_id IS NULL THEN
        RETURN NEW;
    END IF;
    RETURN rules_category_is_visible();
END
$fn$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER review_decision_category_is_visible
    AFTER INSERT OR UPDATE OF category_id, org_id ON review_decisions
    DEFERRABLE INITIALLY IMMEDIATE
    FOR EACH ROW EXECUTE FUNCTION review_decision_category_is_visible();

-- ---------------------------------------------------------------------------
-- An undo stamps; it never removes. What a person did and then reversed is
-- part of the audit trail, and the classifications the decision wrote cannot be
-- deleted either -- migration 007 withheld that grant.
-- ---------------------------------------------------------------------------

REVOKE DELETE ON review_decisions FROM vekst_app;

ALTER TABLE review_decisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE review_decisions FORCE  ROW LEVEL SECURITY;

CREATE POLICY review_decisions_tenant ON review_decisions FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

-- +goose Down

DROP TABLE review_decisions;
DROP FUNCTION review_decision_category_is_visible();
