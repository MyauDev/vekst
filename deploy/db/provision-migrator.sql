-- What the environment must provision before goose runs. This is the
-- executable form of the contract described in README.md and
-- docs/ARCHITECTURE.md §7; it is not a migration and goose never sees it.
--
-- Migration 00001 refuses to run as anyone but vekst_migrator and creates
-- vekst_app itself, so this file's whole job is to produce vekst_migrator
-- and the database it owns.
--
-- vekst_migrator is deliberately NOT a superuser (openspec change
-- add-tenancy-and-rls, design D0). A superuser bypasses row-level security
-- unconditionally -- FORCE ROW LEVEL SECURITY included -- which would make
-- the isolation this schema is built on decorative, and would do so only in
-- the environments where the tests run: every managed Postgres hands out a
-- plain database owner instead. It needs exactly two attributes:
--
--   CREATEROLE   migration 00001 creates vekst_app
--   ownership    of database vekst, so 00001's REVOKE and ALTER DEFAULT
--                PRIVILEGES statements have something to act on. Postgres 15
--                and later make schema public owned by pg_database_owner, so
--                owning the database is what confers schema ownership.
--
-- Run as a throwaway superuser -- the initdb user in a container, or the
-- managed service's administrative role. That superuser is never used again.
--
-- Idempotent: roles are cluster-scoped and a second database in the same
-- cluster must not fail on an existing role, the same reasoning migration
-- 00001 applies to vekst_app.
--
-- The password here is a local-development and CI default only, matching the
-- committed dev credential in deploy/k8s/overlays/local/db-secrets.yaml. A
-- hosted environment provisions this role out of band with a real, rotated
-- password; ALTER ROLE ... PASSWORD below is what keeps re-running this file
-- from resetting one.

DO $$ BEGIN
  CREATE ROLE vekst_migrator LOGIN NOSUPERUSER NOBYPASSRLS CREATEROLE
    PASSWORD 'vekst_migrator';
EXCEPTION
  WHEN duplicate_object THEN
    ALTER ROLE vekst_migrator LOGIN NOSUPERUSER NOBYPASSRLS CREATEROLE;
END $$;

-- CREATE DATABASE cannot run inside a DO block or a transaction, so it is
-- guarded by \gexec instead: the SELECT yields the statement only when the
-- database is absent, and psql executes whatever the query returned.
SELECT 'CREATE DATABASE vekst OWNER vekst_migrator'
 WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'vekst')
\gexec

ALTER DATABASE vekst OWNER TO vekst_migrator;
