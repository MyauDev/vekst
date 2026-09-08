**Ordering.** Apply this after `add-identity` is merged and archived. This migration's
`memberships.user_id` foreign key has no target until `users` exists, and 1.2 is what
allowlists `users`, `user_identities`, `sessions` and `auth_flows`. Confirm both before
starting §1. Migration numbering: 1.2 takes `00003`, this change takes `00004`.

**Spec ordering.** Both requirements this change lists as MODIFIED — "Two database roles with
distinct powers" and "One transaction entry point" — are ADDED by `add-postgres-and-migrations`
(0.2) and do not exist in `openspec/specs/` yet. 0.2 must be archived before this change's
delta can be synced, and three changes are currently stacked on `platform-foundation`: 0.2
adds both, `add-identity` modifies the health requirement, and this one modifies these two.
Archive in that order or the deltas will not resolve.

**Start with §0.** Every mechanism in this change depends on `vekst_migrator` not being a
superuser (design D0), and today CI and the local overlay make it one. Doing §0 first means
§1 and §3 are written against a database shaped like production. Doing it last means writing
D3 and D8 twice.

**Ownership.** Both developers on §0, §1 and §3 — the privilege model, the schema and the
transaction seam are the places a mistake is unrecoverable. One migration owner this week,
per plan §2.1.

**Rule for this change.** Every task that creates a table has a paired cross-tenant test in
the same task. A table lands with its policy and its proof, or it does not land.

## 0. Privilege model and CI fidelity — both

- [x] 0.1 Change `.github/workflows/ci.yaml` so the Postgres service starts under a throwaway
      superuser and `vekst_migrator` is created as `NOSUPERUSER NOBYPASSRLS CREATEROLE`, owning
      database `vekst` and schema `public`; goose still authenticates as `vekst_migrator`
      (design D0). Migration 00001 needs only `CREATEROLE` plus that ownership. There are three
      `POSTGRES_USER: vekst_migrator` service definitions to change, not one
- [x] 0.2 Make the same change in `deploy/k8s/overlays/local/postgres.yaml`, so `make dev`
      reproduces production's privilege shape rather than the initdb superuser's
- [x] 0.3 Confirm migrations 00001–00003 still apply end to end under the new role, including
      the `up → down → up` round trip. Any statement that silently required superuser surfaces
      here — that is the point of doing this before §1. **It found one:** `00001`'s down step
      failed with `permission denied to drop objects (42501)` on `DROP OWNED BY vekst_app`,
      which needs *membership* in the role, not merely admin over it — a superuser had that
      implicitly, and Postgres 16 split `ADMIN` from `SET`/`INHERIT` so `CREATEROLE`'s implicit
      admin does not carry it either. Fixed with a transient `GRANT vekst_app TO CURRENT_USER`
      before the drop; the grant dies with the role on the next line. Full round trip then
      green against a non-superuser owner
- [x] 0.4 Extend 0.2's existing `vekst_app` role assertion to cover `vekst_migrator` too:
      read `pg_roles` and fail if either has `rolsuper` or `rolbypassrls`. Extend the existing
      test rather than adding a second beside it — the spec delta modifies 0.2's requirement,
      it does not add a new one. This is the assertion that keeps every other test in this
      change honest; without it the divergence returns the next time someone simplifies a
      compose file
- [x] 0.5 Record the provisioning contract in `README.md` and in `docs/ARCHITECTURE.md` §7:
      what the environment must create before goose runs, and that superuser is not part of it

## 1. Migration and policies — both

- [x] 1.1 Add `00004_tenancy.sql` creating `organizations`, `entities`, `accounts` and
      `memberships` per design §1, with composite `UNIQUE (org_id, id)` on `entities` and
      `accounts` and composite foreign keys carrying `org_id` (design D1). Applies clean
      against Postgres 16 provisioned by `deploy/db/provision-migrator.sql` alone — a
      non-superuser owner with `createdb=false`
- [x] 1.2 Write the down step and confirm `up → down → up` against a scratch database; the
      existing CI round-trip job must stay green. The down step drops the role D3 creates only
      if no other database in the cluster still uses it — roles are cluster-scoped: it grants
      itself membership (the same Postgres 16 `ADMIN`/`SET` split 00001 hit), runs
      `DROP OWNED BY`, and leaves the role alone on `dependent_objects_still_exist`.
      **The round trip was not green, and not because of this change.** §0 replaced the
      initdb superuser with a role holding only `CREATEROLE`, and `testMigratorURL` creates a
      disposable database per test — so `core/internal/migrate` failed with `permission denied
      to create database (42501)` on a clean tree. Task 0.3 checked that migrations applied,
      not that the suite still ran. Fixed without widening the provisioning contract: the
      harness takes `DATABASE_URL_ADMIN`, a throwaway superuser, for the `CREATE`/`DROP
      DATABASE` only — creating a database is something no production environment does — and
      creates it `OWNER vekst_migrator` so 00001's `REVOKE` still has schema ownership to act
      on. `provision-migrator.sql`, `README.md` and `ARCHITECTURE.md` §7 are unchanged;
      `vekst_migrator` still has `rolcreatedb = false`. All four migrate tests green
- [x] 1.3 Add `app_current_org()` with the single-argument `current_setting` call, and a test
      asserting it raises rather than returning NULL when `app.org_id` is unset (design §1.1).
      **There are two ways to have no context, not one.** A connection that never carried one
      raises `42704` from `current_setting` itself; one whose tenant transaction has *ended*
      finds the parameter still registered holding `''`, and a bare `''::uuid` raises `22P02`
      instead. The second is the common case, because core runs on a pool. The body is
      `plpgsql` so both normalise to `42704` with a message that says what is wrong — one
      condition, one code. Confirmed the planner still folds the STABLE call during selectivity
      estimation, so the empty-table raise (below) survives the change of language
- [x] 1.4 Enable and force RLS on all four tables, with one `FOR ALL` policy per table whose
      `USING` and `WITH CHECK` expressions are identical and written out in full (design §1.2 —
      both halves are for legibility and for D6, not because `USING` alone opens a write hole)
- [x] 1.5 Cross-tenant test — read: with `app.org_id` set to org A, selecting each of the four
      tables returns A's rows and none of B's, where B's rows exist.
      `TestCrossTenantRead`, run as `vekst_app` against a non-superuser-owned schema
- [x] 1.6 Cross-tenant test — write: with `app.org_id` set to org A, inserting a row stamped
      `org_id = B` is rejected by `WITH CHECK`; updating an A row to `org_id = B` is rejected.
      Both `42501`, and B's row is re-read afterwards to prove it was untouched
- [x] 1.7 Cross-tenant test — foreign key: with context on org A, inserting an `accounts` row
      whose `entity_id` belongs to B fails on the composite FK, and the failure does not
      distinguish "B owns it" from "it does not exist" (design D1). Asserted on both the
      SQLSTATE (`23503`) and the full message text, since the message is the likelier leak
- [x] 1.8 Cross-tenant test — uniqueness: two organisations may hold the same
      `accounts.external_ref` for the same-named entity without collision, so a duplicate-key
      error can never disclose another tenant's row (design D1). Confirm no constraint on a
      tenant table lacks `org_id` as its leading column.
      **The confirmation failed against design §1 as written.** `entities_pkey (id)` and
      `accounts_pkey (id)` are uniqueness constraints on tenant tables that do not carry
      `org_id`, and unique constraints bypass RLS exactly as foreign keys do — the same oracle
      D1 closes for FKs, left open on the primary key. Practically it costs 122 bits of
      guessing, but the argument stops holding for any tenant table whose key is not random.
      Both are now `PRIMARY KEY (org_id, id)`; the separate `UNIQUE (org_id, id)` is dropped as
      redundant and the composite FK targets the primary key directly. `id` alone is no longer
      globally unique, which nothing depends on. `TestEveryUniquenessConstraintOnATenantTable
      LeadsWithOrgID` enumerates the catalog rather than spot-checking, so the constraint a
      later migration adds without thinking is caught
- [x] 1.9 Negative test: with no `app.org_id` set at all, a select on each tenant table raises
      `undefined_object` rather than returning an empty result. Three paths asserted, all
      `42704` after 1.3's normalisation: a connection that never carried a context, one whose
      tenant transaction has ended (the pooled case, and what task 3.9 will exercise from Go),
      and a table that is genuinely **empty** — that last one because a per-row qual would have
      returned zero rows silently, and the first tenant to run a report before importing
      anything is exactly who would have found it
- [x] 1.10 Apply the column shapes from design §1: `text` with a shape `CHECK` rather than
      `char(n)` for `country` and the currency codes; no `deleted_at` until soft delete is
      designed; `base_currency` on `organizations` only. Note in the migration why each differs
      from `ARCHITECTURE.md` §5.5's sketch, and update §5.5 to match — §5.5 currently shows
      `deleted_at` on `organizations` and `base_currency` on `entities`, and neither lands here
      Done, and `§5.5` now shows the four tables as `00004` creates them, with the three
      differences and their reasons recorded beneath it rather than left as a silent divergence
- [x] 1.11 Create `vekst_membership_reader` (`NOLOGIN NOSUPERUSER NOBYPASSRLS`), the
      `FOR SELECT … TO vekst_membership_reader USING (true)` policy on `memberships`, and
      `orgs_for_user` owned by that role with a pinned `search_path`; `REVOKE EXECUTE … FROM
      PUBLIC` and grant it to `vekst_app` (design D3). The migration must grant
      `vekst_membership_reader TO CURRENT_USER WITH SET TRUE` before reassigning ownership, and
      grant then revoke `CREATE ON SCHEMA public` around the reassignment — both are required
      when the owner is not a superuser, and neither is needed when it is, which is why §0
      comes first. Use the same `DO`-block idempotency shape as migration 00001, since roles
      are cluster-scoped
      Verified against the §0 database: the migration needed both provisioning steps, and
      neither is needed under a superuser owner — which is the whole argument for §0 going first
- [x] 1.12 Test D3 against the §0 database: `orgs_for_user` returns one user's memberships with
      no tenant context set; returns nothing belonging to another user; `memberships` still
      raises for `vekst_app` with no context; `memberships` still returns only org A's rows with
      context on A; and `vekst_app` cannot `SET ROLE vekst_membership_reader`
      `TestOrgsForUserIsABoundedException`, all five of design D3's probes plus the two
      catalog assertions that bound it: the role has no login, no superuser and no bypass, and
      `memberships` is still enabled *and* forced
- [x] 1.13 Add `/core/internal/db/`, `/core/internal/money/`, `/core/internal/jobs/`,
      `orgs_for_user` and `vekst_membership_reader` to `.github/CODEOWNERS`; the file's own
      comment defers the paths until they exist, and they now do

## 2. Queries — both
      All three package paths exist, so the entries bind rather than being silently ignored;
      `00004_tenancy.sql` is listed too, since it carries the role and the function
- [x] 2.1 Write the `sqlc` queries for organisation, entity, account and membership reads and
      writes into `/core/internal/db/query`, with no `org_id` predicate written by hand — the
      policy supplies it, and a hand-written predicate hides a missing policy
      16 queries in `core/internal/db/query/tenancy.sql`, not one carrying an `org_id`
      predicate. `orgs_for_user` is deliberately **not** among them: sqlc cannot resolve a
      `RETURNS TABLE` function's columns — naming them fails to compile, and `SELECT *`
      silently generates a `[]interface{}` over one anonymous column, which compiles and is
      wrong. The shape that would satisfy sqlc is `RETURNS SETOF memberships`, which widens
      exactly the two-column signature D3's bounded-exception argument rests on. The one call
      site is hand-written against pgx inside `core/internal/db`, the only package allowed to
      issue a query directly
- [x] 2.2 Use `:execrows` for every write intended to affect exactly one row, and have the
      wrapper return a named error when the count is zero (design D9). An update filtered out
      by policy is silent otherwise
      `db.ExactlyOneRow` composes with sqlc's `(int64, error)` signature. It also rejects a
      count above one: the policy can only ever reduce the count, so >1 means the statement's
      own predicate matched more than the caller believed existed
- [x] 2.3 Test: an update whose target row the tenant policy does not admit returns the
      zero-affected-rows error rather than reporting success (design D9). This is the only
      spec scenario in §2 and it was otherwise implemented but unproved
      `TestUpdateFilteredByPolicyReportsNoRowsAffected`, which also asserts the row was not
      modified and that the owning tenant's identical update *succeeds* — otherwise the test
      would pass against a wrapper that always errored
- [x] 2.4 Run `make gen`; confirm the codegen drift job stays green

## 3. Transaction entry point — both
      `make gen` runs clean; sqlc parses 00004 including the policies, the plpgsql body and
      the `DO` blocks
- [x] 3.1 Change `db.InTx` to take a required `OrgID` and issue
      `SELECT set_config('app.org_id', $1, true)` as the transaction's first statement
      (design D2). Never interpolate the value into a `SET` statement, and add the check that
      proves it: no `SET`/`set_config` call outside `InTx`, and no tenant identifier
      concatenated into SQL text anywhere — the spec asserts this and nothing tested it
      `set_config` with a bound parameter, as the transaction's first statement. The check
      script now also fails on any `set_config`/`SET LOCAL`/`SET app.` outside `tx.go`, and on
      any `app.org_id` reaching SQL through `Sprintf` or concatenation. Both verified red
- [x] 3.2 Confirm `db.InSystemTx` already exists from 1.2 and add the tenant-aware `InTx`
      beside it. This change renames nothing — 1.2 did the rename while the entry point had no
      call sites — and documents what may use `InSystemTx` and why the list is short
      `InSystemTx`'s doc now lists its three legitimate kinds of caller and states the rule
      that keeps the list short: everything with a tenant goes through `InTx`
- [x] 3.3 Extend `scripts/check-db-entry-point.sh` to count `InSystemTx` call sites against a
      committed expected count, so a new one is a visible diff. The starting number is the
      count 1.2 recorded in its task 2.4: **7**, all inside `core/internal/identity` — verified
      against the working tree as `session.go` (3), `signin.go` (3) and `expiry.go` (1). Count
      non-test call sites only; the harness and `tx_test.go` add nine more. **Task 3.4 raises
      the committed count to 8**, because `OrgIDForSession` is itself an untenanted read; land
      that increment with 3.4, not as a surprise in 3.3
      The count came out at exactly **8**, as predicted — 7 in `core/internal/identity` plus
      `OrgIDForSession`. Verified red by adding a ninth
- [x] 3.4 Make `OrgID` a struct with an unexported field, and add the three constructors —
      `OrgIDForSession`, `OrgIDFromJobArgs`, `OrgIDForNewOrg` — plus the `testing.TB`-gated
      `OrgIDForTest` (design D7). `OrgIDForSession` is a method on `*DB` that opens
      `InSystemTx` to call `orgs_for_user`, so raise the committed count in 3.3 from 7 to 8 in
      the same commit. It returns the same error for "not a member" as for "no such
      organisation". Declare `TenantJobArgs` in `db`, not in `jobs` — `jobs` depends on `db`
      and the dependency runs one way
      `OrgID` is a struct with an unexported field; `OrgIDForTest` lives in a non-test file so
      other packages' tests can reach it, and takes a `testing.TB` it never uses
- [x] 3.5 Test: a package outside `core/internal/db` cannot construct an `OrgID` from a raw
      UUID, and non-test code cannot call `OrgIDForTest` without importing `testing`. Assert
      both as compile failures, not comments — `go build` on testdata files that must not
      compile, or an equivalent `go/types` check
      Three files under `core/internal/db/testdata/forgery/`, compiled by explicit path
      (`testdata` is invisible to `./...`). Each asserts the *expected compiler message* too,
      so a file that stops compiling for an unrelated reason fails rather than looking like proof
- [x] 3.6 Test: `OrgIDForSession` refuses an organisation the user does not belong to, and the
      refusal is indistinguishable from one for an organisation that does not exist
      Both refusals are `ErrNotAMember` and compare equal as strings
- [x] 3.7 Test: `InTx` rejects a zero-valued `OrgID` before opening a transaction, naming the
      constructor to use
      The error names all three constructors, asserted
- [x] 3.8 Extend `scripts/check-db-entry-point.sh` to count `OrgIDFromJobArgs` and
      `OrgIDForNewOrg` call sites the same way. `OrgIDForSession` is deliberately uncounted —
      it is the door that is supposed to be used
      `OrgIDForNewOrg` committed at 1, `OrgIDFromJobArgs` at 0. Both verified red
- [x] 3.9 Test: two sequential transactions on the same pooled connection — the first sets
      org A, the second sets nothing — and the second raises rather than seeing A's rows.
      This is the leak the transaction-local flag exists to prevent
      Forced onto one backend with `MaxConns: 1`, so "the same pooled connection" is not a
      hope. Raises `42704` — the code 1.3's normalisation guarantees on this path
- [x] 3.10 Test: a rolled-back tenant transaction leaves no context behind on its connection
      Same shape, after a rollback
- [x] 3.11 Implement organisation creation: mint the identifier with `OrgIDForNewOrg`, open
      `InTx` with it, insert the organisation, its single entity and the creator's `owner`
      membership in one transaction (design D4), and test that it needs no privileged path

## 4. Background jobs — B
      `db.CreateOrganization`: organisation, entity and owner membership in one `InTx`
- [x] 4.1 Give the job-argument struct an organisation field, and have the worker obtain its
      `OrgID` through `OrgIDFromJobArgs` and open `InTx` with it
      `TenantProbeArgs` embeds `db.TenantJobArgs`; `TenantProbeWorker` goes through
      `OrgIDFromJobArgs` then `InTx`. `jobs.New` now takes the `*db.DB` rather than a bare
      pool — River's driver still gets the raw pool from it
- [x] 4.2 Test: a job whose arguments name org A reads A's rows and none of B's
      Both organisations hold exactly one entity, so a worker that leaked would count two.
      The count is the assertion, not merely that the job succeeded
- [x] 4.3 Test: a job whose arguments carry no organisation fails rather than running with no
      tenant context
      Asserted at the door rather than by waiting out River's retries, which would prove the
      same thing more slowly and without saying which of several reasons caused it
- [x] 4.4 Extend the worker package comment: job arguments are not tenant-isolated, so they
      carry identifiers and never customer financial data (design D5)

## 5. Coverage test and CI — B
      In both the package doc and `TenantProbeArgs`: River's tables have no `org_id` and no
      policy, so arguments carry identifiers and never customer financial data
- [x] 5.1 Write the RLS coverage test per design D6: read `deploy/db/rls-exempt-tables.txt`,
      list every table in `public`, and for each non-allowlisted table assert
      `relrowsecurity`, `relforcerowsecurity`, and a policy that applies to all roles
      (`polroles = '{0}'`), covers writes, and restricts a column of that table to
      `app_current_org()`. That column is `org_id` **except on `organizations`, where it is
      `id`** — a test that simply demanded an `org_id` column would fail on the first table
      this change creates. Judge only all-roles policies, so D3's role-scoped
      `membership_reader` is not weighed as a general policy. Treat a NULL `polwithcheck` on a
      `FOR ALL` policy as equal to `polqual`, because Postgres derives it
      `checkRLSCoverage` in `core/internal/migrate/rls_coverage_test.go`.
      **It enumerates `relkind IN ('r','p','m','f')`, not just ordinary tables**, which the
      design did not specify and which turned out to matter — see the note under 5.3
- [x] 5.2 Extend the same test with the two inventories: exactly one role-scoped policy
      (`membership_reader` on `memberships`) and exactly one `SECURITY DEFINER` function
      `vekst_app` may execute (`orgs_for_user`). This is what proves the spec's "it is the only
      such exception", which nothing else checks
      Both inventories, plus a check that no role-scoped policy names `vekst_app`: a
      role-scoped policy granted to the application role is a general policy in disguise
- [x] 5.3 Test the test, once per condition it asserts: a table with no policy, a policy that
      is unconditionally true, a policy covering reads only, a table enabled but not forced, a
      second role-scoped policy, and a second `SECURITY DEFINER` function. Create each in a
      scratch database, confirm the checker reports it, drop it. A green test that cannot go
      red proves nothing, and the likeliest real failure is a policy that exists and does not
      isolate
      Ten conditions, each created, checked and dropped, with a clean-baseline assertion
      before and a clean-schema assertion after so no case can pass on another's leftovers.
      **Two conditions the design did not name, found by probing Postgres 16 directly:**
      a *materialized view* over a tenant table cannot have row-level security at all, is
      granted to `vekst_app` automatically by 00001's `ALTER DEFAULT PRIVILEGES`, and is
      invisible to `information_schema.tables`, to `pg_tables` and to `pg_class` filtered on
      `relkind='r'` — every plausible reading of "every table in `public`". A *partition*
      inherits neither its parent's RLS flags nor its policies, so `SELECT` on the partition
      directly returned another tenant's row while the same query through the parent returned
      none. Both are now hard failures. The `BYPASSRLS` case needed the throwaway superuser to
      set up, because `vekst_migrator` cannot grant it — which is itself part of D0's defence
- [x] 5.4 Wire the coverage test and the §0 role assertion into the existing `database` CI job,
      after migrations
      The migrate package already runs in the `database` job; it now also receives
      `DATABASE_URL_ADMIN`. `checkRoleAttributes` asserts both roles' `rolsuper` and
      `rolbypassrls` in Go, alongside the shell assertion 0.4 added
- [x] 5.5 Confirm the allowlist still matches the tables the migrations create — 004 adds four
      tables and none of them is exempt

## 6. Documentation and close
      And the reverse direction too: every allowlisted name must still exist, because a stale
      line is standing permission to recreate that table with no policy
- [x] 6.1 Update `CLAUDE.md`: `InTx` now takes an organisation and `OrgID` cannot be built
      outside `core/internal/db`; referential-integrity checks between tenant tables are scoped
      by `org_id` — composite foreign keys *and* org-leading unique constraints; tenant context
      is fail-closed; a single-row write that affects no rows is an error
      All five points, plus the two the implementation added: the `relkind` rule (a
      materialized view and a partition are both failures, for different reasons) and the
      superuser half of the role rule
- [x] 6.2 Update `README.md`: how to open a psql prompt with a tenant context set, since
      without one every select on a tenant table now raises; and what the environment must
      provision before goose runs (§0.5)
      Both recipes verified against a live Postgres 16 before being written down, including
      that `ROLLBACK` restores `FORCE`. The documented error text was wrong on the first pass —
      a fresh connection gets `current_setting`'s own wording, not the normalised one — so both
      are shown, with why there are two. Also records `DATABASE_URL_ADMIN` and why it is not
      `CREATEDB` on the migrator
- [x] 6.3 Record in `docs/ARCHITECTURE.md` §7 that referential-integrity checks bypass RLS —
      foreign keys and unique constraints alike — and that `org_id`-leading keys are the
      answer; that `vekst_migrator` is not a superuser and why; and the `NO FORCE`/restore
      idiom for a migration that backfills a tenant table (design D8). None of these is
      obvious, and all three will be re-litigated otherwise
      §7 gains the primary-key half of the referential-integrity rule, the `relkind` blind
      spot, "RLS is containment, not authorization", the fail-closed guarantee including the
      empty-table case, and the two enumerated exceptions
- [x] 6.4 Update the capability spec and run the full suite
      Spec updated with the two `relkind` scenarios and a widened "the coverage test can fail"
      scenario listing all ten conditions. `openspec validate` passes. Full suite: `buf lint`,
      `go vet ./...`, the three db/identity/codeowners checks, ruff, mypy, tsc, pytest, vitest
      and both Go suites all pass. **`make lint` as a whole does not**, on
      `scripts/check-web-tokens.sh` — a check another session added while this change was in
      progress, failing on 18 literal Tailwind classes in `web/src/*.tsx`. Nothing in that path
      belongs to this change

## 7. Verification pass

`/opsx:verify` found four gaps after the tasks were complete; all four are fixed and the suite
re-run. None changed shipped behaviour — two were untested-but-correct schema properties, two
were design statements the implementation's own mid-flight decisions had outdated.

- [x] 7.1 Test the spec scenario "The reporting currency is not duplicated", which nothing
      enforced: `base_currency` must live on exactly one relation. Verified red against the
      `ARCHITECTURE.md` §5.5 sketch's own version of the schema — `base_currency` on `entities`
      as well — which is precisely the migration someone would plausibly write
- [x] 7.2 Test the spec scenario "An entity-scoped row cannot omit its entity". The column was
      already `NOT NULL` and rejected the insert; nothing asserted it, so making it nullable to
      unblock an import would have gone unnoticed. Verified red by doing exactly that
- [x] 7.3 Correct design §5's risk row on re-setting `app.org_id`. Its mitigation read "every
      statement is generated by `sqlc`", which stopped being true when `OrgIDForSession` had to
      be hand-written (2.1). The security property was never affected — both hand-written
      statements are string literals with bound parameters — but the stated reason was wrong,
      and it is the row a future reviewer would lean on. Now names both statements and cites
      the check that enforces the rule, which did not exist when the row was written
- [x] 7.4 Bring design D6 up to the implementation and the spec: it still said "every other
      table in `public`", the wording whose wrong unit let materialized views and partitions
      through in the first place. Now records the `relkind` table and what each kind evades