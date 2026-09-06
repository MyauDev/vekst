**Ordering.** Apply this after `add-identity` is merged and archived. Migration 003's
`memberships.user_id` foreign key has no target until `users` exists, and 1.2 is what adds
`users` to `deploy/db/rls-exempt-tables.txt`. Confirm both before starting §1.

**Ownership.** Both developers on §1 and §3 — the schema and the transaction seam are the
places a mistake is unrecoverable. One migration owner this week, per plan §2.1.

**Rule for this change.** Every task that creates a table has a paired cross-tenant test in
the same task. A table lands with its policy and its proof, or it does not land.

## 1. Migration and policies — both

- [ ] 1.1 Add `00003_tenancy.sql` creating `organizations`, `entities`, `accounts` and
      `memberships` per design §1, with composite `UNIQUE (org_id, id)` on `entities` and
      `accounts` and composite foreign keys carrying `org_id` (design D1)
- [ ] 1.2 Write the down step and confirm `up → down → up` against a scratch database; the
      existing CI round-trip job must stay green
- [ ] 1.3 Add `app_current_org()` with the single-argument `current_setting` call, and a test
      asserting it raises rather than returning NULL when `app.org_id` is unset (design §1.1)
- [ ] 1.4 Enable and force RLS on all four tables, with one `FOR ALL` policy per table whose
      `USING` and `WITH CHECK` expressions are identical
- [ ] 1.5 Cross-tenant test — read: with `app.org_id` set to org A, selecting each of the four
      tables returns A's rows and none of B's, where B's rows exist
- [ ] 1.6 Cross-tenant test — write: with `app.org_id` set to org A, inserting a row stamped
      `org_id = B` is rejected by `WITH CHECK`; updating an A row to `org_id = B` is rejected
- [ ] 1.7 Cross-tenant test — foreign key: with context on org A, inserting an `accounts` row
      whose `entity_id` belongs to B fails on the composite FK, and the failure does not
      distinguish "B owns it" from "it does not exist" (design D1)
- [ ] 1.8 Negative test: with no `app.org_id` set at all, a select on each tenant table raises
      `undefined_object` rather than returning an empty result
- [ ] 1.9 Add `orgs_for_user` as `SECURITY DEFINER` with a pinned `search_path`, grant
      `EXECUTE` to `vekst_app`, and test that it returns exactly one user's memberships
      (design D3)
- [ ] 1.10 Add `/core/internal/db/`, `/core/internal/money/`, `/core/internal/jobs/` and the
      new function to `.github/CODEOWNERS`; the file's own comment defers them until the
      paths exist, and they now do

## 2. Queries — both

- [ ] 2.1 Write the `sqlc` queries for organisation, entity, account and membership reads and
      writes into `/core/internal/db/query`, with no `org_id` predicate written by hand — the
      policy supplies it, and a hand-written predicate hides a missing policy
- [ ] 2.2 Run `make gen`; confirm the codegen drift job stays green

## 3. Transaction entry point — both

- [ ] 3.1 Change `db.InTx` to take a required `OrgID` and issue
      `SELECT set_config('app.org_id', $1, true)` as the transaction's first statement
      (design D2). Never interpolate the value into a `SET` statement
- [ ] 3.2 Add `db.InSystemTx` for untenanted reads, documented with what may use it and why
      the list is short
- [ ] 3.3 Extend `scripts/check-db-entry-point.sh` to count `InSystemTx` call sites against a
      committed expected count, so a new one is a visible diff
- [ ] 3.4 Test: two sequential transactions on the same pooled connection — the first sets
      org A, the second sets nothing — and the second raises rather than seeing A's rows.
      This is the leak the transaction-local flag exists to prevent
- [ ] 3.5 Test: a rolled-back tenant transaction leaves no context behind on its connection
- [ ] 3.6 Implement organisation creation: generate the UUID in `core`, open `InTx` with it,
      insert (design D4), and test that it needs no privileged path

## 4. Background jobs — B

- [ ] 4.1 Give the job-argument struct an organisation field, and have the worker open `InTx`
      with it
- [ ] 4.2 Test: a job whose arguments name org A reads A's rows and none of B's
- [ ] 4.3 Test: a job whose arguments carry no organisation fails rather than running with no
      tenant context
- [ ] 4.4 Extend the worker package comment: job arguments are not tenant-isolated, so they
      carry identifiers and never customer financial data (design D5)

## 5. Coverage test and CI — B

- [ ] 5.1 Write the RLS coverage test: read `deploy/db/rls-exempt-tables.txt`, list every
      table in `public`, and fail for any table outside the allowlist lacking
      `relrowsecurity`, `relforcerowsecurity` or a `pg_policies` row (design D6)
- [ ] 5.2 Test the test: create a table with no policy in a scratch database and confirm the
      coverage test fails on it, then remove it. A green test that cannot go red proves nothing
- [ ] 5.3 Wire the coverage test into the existing `database` CI job, after migrations
- [ ] 5.4 Confirm the allowlist still matches the tables the migrations create — 003 adds four
      tables and none of them is exempt

## 6. Documentation and close

- [ ] 6.1 Update `CLAUDE.md`: `InTx` now takes an organisation; foreign keys between tenant
      tables are composite and carry `org_id`; tenant context is fail-closed
- [ ] 6.2 Update `README.md`: how to open a psql prompt with a tenant context set, since
      without one every select on a tenant table now raises
- [ ] 6.3 Record in `docs/ARCHITECTURE.md` §7 that FK checks bypass RLS and that composite
      keys are the answer — the reason is not obvious and will be re-litigated otherwise
- [ ] 6.4 Update the capability spec and run the full suite
