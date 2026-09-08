-- Roles and default privileges. No tables. Runs once per database and is the
-- foundation of the isolation story in ARCHITECTURE.md §7 -- see openspec
-- design D1 for the full argument.
--
-- vekst_migrator is NOT created here. goose authenticates as it before any
-- migration runs, so it exists first in every environment by definition
-- (design Q1). It is provisioned by the environment, which also makes it the
-- owner of database vekst and of schema public -- the REVOKE below requires
-- that ownership.

-- +goose Up
-- +goose StatementBegin
DO $$ BEGIN
  IF current_user <> 'vekst_migrator' THEN
    RAISE EXCEPTION 'migrations must be applied as vekst_migrator, not %', current_user;
  END IF;
END $$;
-- +goose StatementEnd

-- Roles are cluster-scoped and migrations are database-scoped, so a second
-- database in the same cluster must not fail on an already-existing role
-- (design risk row 1). A missing CREATEROLE privilege is a separate failure
-- mode (design Q1): catch it and name the role that needs provisioning,
-- rather than surfacing a bare permission-denied error three statements in.
-- The password below is a local-development-only default -- matching the
-- committed dev credential already in deploy/k8s/overlays/local/postgres.yaml
-- -- never a value fit for a hosted environment. Where the real, rotated
-- password comes from in a hosted environment is Q2's open item, left to the
-- provisioning change; ALTER ROLE ... PASSWORD is idempotent, so re-running
-- this migration never resets a password an out-of-band process later set.
-- +goose StatementBegin
DO $$ BEGIN
  CREATE ROLE vekst_app LOGIN NOBYPASSRLS PASSWORD 'vekst_app';
EXCEPTION
  WHEN insufficient_privilege THEN
    RAISE EXCEPTION 'vekst_migrator needs CREATEROLE to provision vekst_app; see design Q1';
  WHEN duplicate_object THEN
    ALTER ROLE vekst_app PASSWORD 'vekst_app';
END $$;
-- +goose StatementEnd

GRANT CONNECT ON DATABASE vekst TO vekst_app;
GRANT USAGE   ON SCHEMA public  TO vekst_app;

-- vekst_app owns nothing and may not create anything.
REVOKE CREATE ON SCHEMA public FROM vekst_app;
REVOKE ALL    ON SCHEMA public FROM PUBLIC;

-- Every table change 1.1 and later creates becomes usable by vekst_app with
-- no follow-up grant. Forgetting this once produces a permission error in
-- production and a hotfix migration; setting it now costs three lines.
ALTER DEFAULT PRIVILEGES FOR ROLE vekst_migrator IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES    TO vekst_app;
ALTER DEFAULT PRIVILEGES FOR ROLE vekst_migrator IN SCHEMA public
  GRANT USAGE, SELECT                  ON SEQUENCES TO vekst_app;

-- goose_db_version is the one table this default-privileges rule does not
-- reach: goose creates it itself, before applying this migration's content,
-- so it predates the rule above rather than being created by it. Confirmed
-- against a real database while writing task 6.1b's readiness check, which
-- reads this table as vekst_app and failed with a bare permission error
-- until this line was added.
GRANT SELECT ON goose_db_version TO vekst_app;

-- +goose Down
ALTER DEFAULT PRIVILEGES FOR ROLE vekst_migrator IN SCHEMA public
  REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLES    FROM vekst_app;
ALTER DEFAULT PRIVILEGES FOR ROLE vekst_migrator IN SCHEMA public
  REVOKE USAGE, SELECT                  ON SEQUENCES FROM vekst_app;

-- Restore the schema to what a fresh Postgres 16 database grants PUBLIC,
-- so `goose down` followed by `goose up` starts from the same baseline.
GRANT USAGE ON SCHEMA public TO PUBLIC;

REVOKE ALL ON SCHEMA public   FROM vekst_app;
REVOKE ALL ON DATABASE vekst  FROM vekst_app;

-- DROP OWNED BY requires membership in the role, not merely admin over it.
-- A superuser has that implicitly, which is why this line worked while
-- vekst_migrator was the initdb superuser; under the plain, non-superuser
-- owner change 1.1 provisions (design D0) it fails with "permission denied
-- to drop objects" (42501). Postgres 16 split ADMIN from SET/INHERIT, so
-- CREATEROLE's implicit admin over a role it created does not carry
-- membership either. The grant is transient: it disappears with the role on
-- the next line.
GRANT vekst_app TO CURRENT_USER;

DROP OWNED BY vekst_app;
DROP ROLE IF EXISTS vekst_app;
