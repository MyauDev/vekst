-- Finish a column migration 005 started. Change 4.1, capability
-- `report-mgmt-pnl`.
--
-- `categories.pnl_section` has existed since 005 and is NULL on all 46 seeded
-- rows. A column created, read by nothing and populated nowhere is not a
-- decision somebody made; it is one nobody finished.
--
-- The section of any category is its level-1 ancestor: NET SALES, CS, OCS,
-- OPEX, OIE, FR, CIT, CAPEX, OUT OF P&L, and the five computed lines which are
-- their own. That is derivable from the code -- two characters per level, so
-- the first two are the root -- but only for rows whose codes follow the
-- convention. An organisation's own leaf hangs under a shared parent and its
-- code begins with that parent's, and nothing in the schema says so. A report
-- reading the prefix would be reading a habit.
--
-- So it is stored, and a trigger keeps it true.

-- +goose Up

-- The shared rows are seeded by 005 and belong to nobody, so the write policy
-- refuses them and FORCE subjects the migrator to it. NO FORCE for the length
-- of this backfill is the only way to amend a shared taxonomy after the fact;
-- it is reinstated below, and the coverage test would fail if it were not.
ALTER TABLE categories NO FORCE ROW LEVEL SECURITY;

UPDATE categories c
   SET pnl_section = root.name
  FROM categories root
 WHERE root.taxonomy_version = c.taxonomy_version
   AND root.org_id IS NOT DISTINCT FROM c.org_id
   AND root.level = 1
   AND root.code = left(c.code, 2)
   AND c.pnl_section IS NULL;

-- Counted inside the same window. The read is subject to the policy the
-- moment FORCE comes back, and app_current_org() raises in a migration that
-- sets no tenant context -- so an assertion placed after it would fail for a
-- reason that has nothing to do with what it asserts.
-- +goose StatementBegin
DO $$
DECLARE
    unfilled integer;
BEGIN
    SELECT count(*) INTO unfilled FROM categories WHERE pnl_section IS NULL;
    IF unfilled <> 0 THEN
        RAISE EXCEPTION
            '% categories have no section; the code prefix did not resolve to a level-1 row',
            unfilled;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE categories ALTER COLUMN pnl_section SET NOT NULL;

ALTER TABLE categories FORCE ROW LEVEL SECURITY;

-- A new row inherits its parent's section, and a root is its own.
--
-- BEFORE rather than a constraint trigger, because it fills as well as checks.
-- A caller that omits the section gets the right one; a caller that states a
-- section disagreeing with its parent is refused. Filling alone would let a
-- stated-but-wrong value through, which is the disagreement this column exists
-- to prevent; checking alone would make every insert repeat a fact the parent
-- already carries.
-- +goose StatementBegin
CREATE FUNCTION category_section_follows_parent() RETURNS trigger
    LANGUAGE plpgsql
    AS $fn$
DECLARE
    inherited text;
BEGIN
    IF NEW.parent_id IS NULL THEN
        inherited := NEW.name;
    ELSE
        SELECT pnl_section INTO inherited FROM categories WHERE id = NEW.parent_id;
        IF inherited IS NULL THEN
            -- The parent is unreadable or absent. categories_parent_is_visible
            -- reports that, with the error that does not say which, so this
            -- says nothing further.
            RETURN NEW;
        END IF;
    END IF;

    IF NEW.pnl_section IS NULL THEN
        NEW.pnl_section := inherited;
    ELSIF NEW.pnl_section IS DISTINCT FROM inherited THEN
        RAISE EXCEPTION 'category % declares section % but belongs to %',
            NEW.code, NEW.pnl_section, inherited USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$fn$;
-- +goose StatementEnd

CREATE TRIGGER category_section_follows_parent
    BEFORE INSERT OR UPDATE OF pnl_section, parent_id, name ON categories
    FOR EACH ROW EXECUTE FUNCTION category_section_follows_parent();

-- Every report read groups by it.
CREATE INDEX categories_section_idx ON categories (taxonomy_version, pnl_section);

-- +goose Down

DROP INDEX categories_section_idx;
DROP TRIGGER category_section_follows_parent ON categories;
DROP FUNCTION category_section_follows_parent();
ALTER TABLE categories ALTER COLUMN pnl_section DROP NOT NULL;

ALTER TABLE categories NO FORCE ROW LEVEL SECURITY;
UPDATE categories SET pnl_section = NULL;
ALTER TABLE categories FORCE ROW LEVEL SECURITY;
