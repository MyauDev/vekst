-- Tenancy: organizations -> entities -> accounts, and the mechanism that keeps
-- one customer's finances out of another customer's report.
--
-- Change 0.2 created vekst_app -- a role that owns nothing and has no
-- BYPASSRLS. With no policy on any table, "no BYPASSRLS" restricts nothing.
-- This migration is what turns that role into isolation. See openspec
-- add-tenancy-and-rls design §1.
--
-- Applied as vekst_migrator, which is NOT a superuser in any environment
-- (design D0, task §0). That matters here more than anywhere else: a superuser
-- bypasses row-level security unconditionally, FORCE included, so a schema
-- built by one would pass CI with isolation that does not work where it is
-- deployed. Two mechanisms below -- the FORCE on every table, and the
-- SECURITY DEFINER function in §4 -- behave differently under a superuser
-- owner, which is why §0 came first.
--
-- Ordering: this migration's memberships.user_id references users, created by
-- 00003 (change 1.2 add-identity). users is deliberately global -- see
-- ARCHITECTURE.md §5.5 and deploy/db/rls-exempt-tables.txt.

-- +goose Up

-- ---------------------------------------------------------------------------
-- §1  The tenant context accessor
-- ---------------------------------------------------------------------------

-- current_setting is called with ONE argument on purpose. The two-argument
-- form returns NULL when app.org_id is unset, every policy below would then
-- evaluate to false, and a query issued outside a tenant transaction would
-- return zero rows with no error -- which a report renders as "this customer
-- has no revenue". ARCHITECTURE.md §7 requires the opposite: a repository call
-- outside a tenant transaction must fail.
--
-- There are TWO ways to have no tenant context, not one, and the plpgsql body
-- exists to give them a single error code:
--
--   The connection has never carried one. current_setting raises
--   undefined_object (42704) itself -- the parameter is not registered.
--
--   The connection carried one and the transaction ended. set_config(..., true)
--   is reverted at commit, but reverting does not *unregister* the parameter:
--   it is left holding the empty string. current_setting then returns '', and
--   a bare ''::uuid would raise invalid_text_representation (22P02) with the
--   message `invalid input syntax for type uuid: ""`.
--
-- The second is the common case in production, because core runs on a
-- connection pool and a pooled connection is reused. Two codes for one
-- condition means every caller that wants to recognise "no tenant context"
-- has to know both and know why there are two, so this normalises them: the
-- empty string is raised as 42704 with a message that says what is actually
-- wrong. Verified against Postgres 16 on both paths.
--
-- STABLE, and the raise is not per-row: the planner folds a no-argument STABLE
-- function to a constant while estimating selectivity, so a select on an
-- *empty* tenant table raises before a single tuple is read -- confirmed for
-- this plpgsql body as well as for a plain SQL one. Fail-closed therefore
-- holds on an empty table as well as a full one, which is the case a per-row
-- qual would have missed.
-- +goose StatementBegin
CREATE FUNCTION app_current_org() RETURNS uuid
    LANGUAGE plpgsql STABLE
    AS $fn$
DECLARE
    v text;
BEGIN
    v := current_setting('app.org_id');
    IF v = '' THEN
        RAISE EXCEPTION 'app.org_id is not set'
            USING ERRCODE = '42704',
                  HINT = 'open the transaction through db.InTx, which sets the tenant context';
    END IF;
    RETURN v::uuid;
END
$fn$;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- §2  The tables
-- ---------------------------------------------------------------------------

-- The organisation is the tenant. It carries no org_id because its own id is
-- the tenant key -- the one table whose policy names `id` rather than
-- `org_id`, and the reason the coverage test (design D6) has to know the
-- difference rather than simply demanding an org_id column.
--
-- Three column shapes differ from ARCHITECTURE.md §5.5's sketch, deliberately:
--
--   text + CHECK, not char(n).  char(n) is blank-padded: a two-character value
--   in a char(3) column compares equal to its unpadded form but does not
--   length() equal to it, and the padding travels into Go as part of the
--   string. A shape CHECK is what §5.5 actually meant.
--
--   No deleted_at.  Soft delete is a non-goal of this change, so the column
--   would land with nothing setting it, nothing reading it and -- the part
--   that bites -- no policy excluding soft-deleted rows. A column with no
--   semantics is worse than a missing one, because the next change assumes it
--   is honoured. It arrives with the change that defines what it means.
--
--   base_currency here and NOT on entities.  §5.5 puts it on both with nothing
--   relating the two, and two unconstrained copies of the same fact is how a
--   report prints a wrong number: whichever copy the query happens to read
--   wins, and the two can disagree with no error. The organisation is the
--   reporting boundary for the Demo. When a holding customer needs per-entity
--   reporting currency, that change adds the column *and* the rule for which
--   one applies, together.
--
-- The CHECK constrains shape only, so 'XXX' is storable and fails later in Go
-- against core/internal/money's checked-in ISO-4217 list. Making that list one
-- artifact instead of two -- a currencies reference table with real foreign
-- keys -- belongs with the change that first stores an amount (2.5), because
-- it is a global non-tenant table and so needs an allowlist row and both
-- reviewers.
CREATE TABLE organizations (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text        NOT NULL CHECK (length(btrim(name)) > 0),
    country       text        NOT NULL CHECK (country ~ '^[A-Z]{2}$'),       -- ISO-3166-1 alpha-2
    base_currency text        NOT NULL CHECK (base_currency ~ '^[A-Z]{3}$'), -- ISO-4217
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- entity_id exists and is populated from this first migration even though the
-- Demo gives each organisation exactly one entity. A holding customer then
-- needs no migration of existing data -- only new rows.
--
-- The primary key leads with org_id, and that is the whole of the uniqueness
-- story for this table: it is also the target every composite foreign key
-- below points at, so no separate UNIQUE (org_id, id) is needed.
--
-- A single-column PRIMARY KEY (id) would have been the ordinary choice, and it
-- is a uniqueness constraint on a tenant table that does not carry org_id.
-- Referential-integrity checks are not subject to row-level security -- unique
-- constraints as much as foreign keys -- so under context A an insert naming an
-- id that belongs to organisation B raises unique_violation where an unused id
-- succeeds, and the difference answers "does some other tenant hold this?".
-- The identifier is a random uuid, so the probe costs 122 bits of guessing and
-- the oracle is not practically reachable; it is closed anyway, because it is
-- the same shape D1 closes for foreign keys and because the argument stops
-- holding the moment any tenant table takes a primary key that is not random --
-- an imported source identifier, say.
--
-- The cost is that id alone is no longer unique across organisations. Nothing
-- depends on that: every query runs under a tenant context, so the policy
-- supplies org_id and this index serves a lookup by id alone, and every foreign
-- key between tenant tables is composite by rule.
CREATE TABLE entities (
    id         uuid        NOT NULL DEFAULT gen_random_uuid(),
    org_id     uuid        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    name       text        NOT NULL CHECK (length(btrim(name)) > 0),
    legal_name text,
    tax_id     text,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, id)
);

-- The composite foreign key is the point of design D1, and it now references
-- entities' primary key directly. Postgres does not apply row-level security to
-- referential-integrity checks -- neither to foreign keys nor to unique
-- constraints -- so a plain `REFERENCES entities (id)`
-- would be a cross-tenant existence oracle: inserting an account naming
-- another organisation's entity would succeed where a random UUID failed, and
-- the difference in the error is the disclosure. Carrying org_id into the
-- check makes both failures identical.
--
-- UNIQUE (org_id, entity_id, external_ref) is the same argument one step
-- louder. An unscoped UNIQUE (external_ref) would disclose a *value* rather
-- than an identifier: a duplicate-key error on 'ACME-4471' tells the caller
-- some other tenant holds that reference. external_ref is nullable and
-- Postgres treats NULLs as distinct, so accounts without one never collide.
CREATE TABLE accounts (
    id           uuid        NOT NULL DEFAULT gen_random_uuid(),
    org_id       uuid        NOT NULL,
    entity_id    uuid        NOT NULL,
    name         text        NOT NULL CHECK (length(btrim(name)) > 0),
    currency     text        NOT NULL CHECK (currency ~ '^[A-Z]{3}$'), -- ISO-4217
    external_ref text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, id),
    FOREIGN KEY (org_id, entity_id) REFERENCES entities (org_id, id) ON DELETE RESTRICT,
    UNIQUE (org_id, entity_id, external_ref)
);

-- memberships is what mediates access to an organisation: users is global
-- (ARCHITECTURE.md §5.5), and this table is the only thing relating a person
-- to a tenant. role is stored and returned but nothing checks it -- role
-- enforcement is Product's, and a column read by nothing is still the right
-- place to put the fact now, because backfilling it later means asking every
-- customer who owns what.
CREATE TABLE memberships (
    org_id     uuid        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    role       text        NOT NULL CHECK (role IN ('owner','admin','approver','viewer')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, user_id)
);

-- The primary key leads with org_id, which is right for the policy and wrong
-- for the one query that runs before any organisation is known: orgs_for_user
-- (§4) looks up by user_id alone. Without this index that lookup is a
-- sequential scan on every sign-in.
CREATE INDEX memberships_user_idx ON memberships (user_id);

-- ---------------------------------------------------------------------------
-- §3  Row-level security
-- ---------------------------------------------------------------------------

-- FORCE, not merely ENABLE. Without FORCE the table owner is exempt from its
-- own policies. vekst_app owns nothing today, so ENABLE alone would look
-- identical -- right up until a migration changes an owner, at which point
-- isolation is silently off and nothing reports it. FORCE also applies to
-- vekst_migrator, which is what makes a backfill in a later migration fail
-- loudly rather than quietly write across tenants (design D8 records the
-- NO FORCE / restore idiom for when one legitimately needs to).
--
-- USING and WITH CHECK are both written out on every policy. This is NOT
-- because omitting WITH CHECK would open a write hole -- on a FOR ALL policy
-- Postgres derives WITH CHECK from USING when it is absent, and a USING-only
-- policy does reject an insert stamped with another organisation's org_id.
-- The reason to spell both out is that the equivalence stops holding the
-- moment anyone splits a policy per command: FOR SELECT has no WITH CHECK at
-- all and FOR INSERT has no USING, so a later refactor into per-command
-- policies inherits nothing. Two visible expressions survive that refactor;
-- one implied expression does not. It also gives the coverage test a
-- polwithcheck to assert on.

ALTER TABLE organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE organizations FORCE  ROW LEVEL SECURITY;
CREATE POLICY organizations_tenant ON organizations FOR ALL
    USING      (id = app_current_org())
    WITH CHECK (id = app_current_org());

ALTER TABLE entities ENABLE ROW LEVEL SECURITY;
ALTER TABLE entities FORCE  ROW LEVEL SECURITY;
CREATE POLICY entities_tenant ON entities FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

ALTER TABLE accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE accounts FORCE  ROW LEVEL SECURITY;
CREATE POLICY accounts_tenant ON accounts FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE memberships FORCE  ROW LEVEL SECURITY;
CREATE POLICY memberships_tenant ON memberships FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

-- ---------------------------------------------------------------------------
-- §4  Resolving a user's organisations, without a privileged role  (design D3)
-- ---------------------------------------------------------------------------

-- Login must answer "which organisations does this user belong to?" *before*
-- any organisation is known. That read cannot go through a tenant transaction
-- (there is no org yet) and cannot go through the untenanted one either,
-- because memberships has a policy and app_current_org() would raise.
--
-- SECURITY DEFINER alone does not solve this. A definer function does not
-- bypass row-level security; it runs as its owner, and only skips RLS if that
-- owner could. Under a non-superuser vekst_migrator, FORCE subjects the owner
-- to the policy too, so the function re-enters app_current_org() and raises.
-- Verified: with an initdb-superuser owner the identical function returns two
-- rows; with a plain owner it raises. That CI-green/production-broken
-- divergence is what §0 removed, and it would have landed on the only path
-- into the product.
--
-- The mechanism is therefore a role-scoped policy, not a privileged role. An
-- exception written as a row in pg_policies is a catalog object the coverage
-- test can see, name and count; an exception written as an attribute of
-- whoever happens to own a function is not.
--
-- Roles are cluster-scoped and migrations are database-scoped, so a second
-- database in the same cluster must not fail on an already-existing role --
-- same DO-block shape as 00001's vekst_app.
-- +goose StatementBegin
DO $$ BEGIN
  CREATE ROLE vekst_membership_reader NOLOGIN NOSUPERUSER NOBYPASSRLS;
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;
-- +goose StatementEnd

GRANT USAGE  ON SCHEMA public TO vekst_membership_reader;
GRANT SELECT ON memberships   TO vekst_membership_reader;

-- The single deliberate exception to "row-level security decides what is
-- visible", scoped to that role and to SELECT. FORCE stays on memberships.
--
-- This policy is permissive and so is OR'd with memberships_tenant above,
-- which means a query by this role evaluates `true OR (org_id =
-- app_current_org())`. That does not raise with no tenant context set, because
-- the planner simplifies the OR before the raising branch is ever evaluated --
-- the same constant-folding that makes §1's fail-closed hold on empty tables.
-- The two behaviours are one mechanism, and neither is safe to assume without
-- the other.
CREATE POLICY membership_reader ON memberships
    FOR SELECT TO vekst_membership_reader
    USING (true);

CREATE FUNCTION orgs_for_user(p_user_id uuid)
    RETURNS TABLE (org_id uuid, role text)
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public
    AS $fn$ SELECT m.org_id, m.role FROM memberships m WHERE m.user_id = p_user_id $fn$;

-- Two provisioning details that are needed only because the owner is not a
-- superuser, and so would have gone undiscovered until deployment had §0 not
-- come first:
--
--   Reassigning ownership requires SET on the target role. Postgres 16 split
--   SET from ADMIN, so CREATEROLE's implicit admin over a role it just created
--   does not carry the right to become it. The grant is transient -- it dies
--   with the role in the down step, and nothing can use it meanwhile because
--   the role cannot log in.
--
--   The incoming owner needs CREATE on the schema *at the moment of
--   reassignment*. Granted, used, and revoked in the same transaction, so the
--   role ends the migration exactly as narrow as it started: no LOGIN, no
--   CREATE, no BYPASSRLS, and reachable only through the body of one function.
GRANT vekst_membership_reader TO CURRENT_USER WITH SET TRUE;
GRANT CREATE ON SCHEMA public TO vekst_membership_reader;

ALTER FUNCTION orgs_for_user(uuid) OWNER TO vekst_membership_reader;

REVOKE CREATE ON SCHEMA public FROM vekst_membership_reader;

-- A SECURITY DEFINER function is EXECUTE-able by PUBLIC unless told otherwise,
-- and PUBLIC includes every future role.
REVOKE EXECUTE ON FUNCTION orgs_for_user(uuid) FROM PUBLIC;
GRANT  EXECUTE ON FUNCTION orgs_for_user(uuid) TO vekst_app;

-- +goose Down

DROP FUNCTION IF EXISTS orgs_for_user(uuid);

-- Policies and indexes go with their tables. Order is reverse-dependency:
-- memberships and accounts reference entities and organizations.
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS entities;
DROP TABLE IF EXISTS organizations;

DROP FUNCTION IF EXISTS app_current_org();

-- Roles are cluster-scoped but this migration is database-scoped, so the role
-- may still own objects or hold grants in another database -- the test suite
-- alone creates several. DROP OWNED BY only reaches the current database; if
-- anything remains elsewhere, DROP ROLE raises dependent_objects_still_exist
-- and the correct behaviour is to leave the role alone. It is inert either
-- way: NOLOGIN, no BYPASSRLS, and with its policy and function now gone it
-- grants nothing at all.
--
-- DROP OWNED BY requires membership in the role, not merely admin over it --
-- the same Postgres 16 ADMIN/SET split that 00001's down step ran into once
-- vekst_migrator stopped being a superuser.
-- +goose StatementBegin
DO $$ BEGIN
  GRANT vekst_membership_reader TO CURRENT_USER WITH SET TRUE;
  DROP OWNED BY vekst_membership_reader;
  DROP ROLE vekst_membership_reader;
EXCEPTION
  WHEN undefined_object THEN NULL;
  WHEN dependent_objects_still_exist THEN
    RAISE NOTICE 'vekst_membership_reader still owns objects in another database; leaving it';
END $$;
-- +goose StatementEnd
