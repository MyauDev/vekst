-- `memberships.entity_id`. Change 3.1, task 0.4, finished late.
--
-- The taxonomy change's design D2(d) asked for this column and migration 004
-- shipped without it, so it is its own migration rather than one line in an
-- unwritten one. The argument is the same one 004 already accepted for
-- `entities` and `accounts`: v1 gives every organisation exactly one entity,
-- and the column exists from the first day anyway, because a holding customer
-- arriving later must be a configuration change and not a migration of every
-- row that predates them.
--
-- NULL means the whole organisation, which is every membership today. A
-- populated value will mean "this person sees one entity" -- and it is
-- deliberately read by nothing yet: the policy on every tenant table scopes by
-- `org_id`, and narrowing that to an entity is a change with its own tests,
-- its own failure modes and its own screen. Adding the column now costs
-- nothing; adding it together with the rule that reads it would mean writing
-- both under time pressure the first time a customer needs the second.

-- +goose Up

ALTER TABLE memberships ADD COLUMN entity_id uuid NULL;

-- Adding the key validates it against every existing row, and both tables are
-- FORCE'd -- so the migrator is subject to their policies like anybody else and
-- `app_current_org()` raises 42704 in a migration that binds to no tenant. A
-- window is the only way to add the constraint at all.
--
-- What is being validated is nothing: the column was created one statement ago
-- and is NULL on every row, and a NULL never violates a foreign key. The
-- alternative, `NOT VALID`, would skip the scan and leave a constraint marked
-- unproven forever for a table where the proof is trivial. Both flags are
-- reinstated below and the RLS coverage test fails if either is not.
ALTER TABLE memberships NO FORCE ROW LEVEL SECURITY;
ALTER TABLE entities    NO FORCE ROW LEVEL SECURITY;

ALTER TABLE memberships
    ADD CONSTRAINT memberships_entity_fk
    FOREIGN KEY (org_id, entity_id) REFERENCES entities (org_id, id) ON DELETE RESTRICT;

ALTER TABLE memberships FORCE ROW LEVEL SECURITY;
ALTER TABLE entities    FORCE ROW LEVEL SECURITY;

-- +goose Down

ALTER TABLE memberships DROP CONSTRAINT memberships_entity_fk;
ALTER TABLE memberships DROP COLUMN entity_id;
