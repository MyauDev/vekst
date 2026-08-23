**Budget note.** These tasks total **≈ 30 hours ≈ 3.75 person-days** against the 2
person-days allocated to change 0.2 in `docs/IMPLEMENTATION_PLAN.md` §3. River accounts for
most of the difference — it appears in `ARCHITECTURE.md` §3.5, §8 and §9 but in no scope line
in the plan, so its ≈6 hours were never budgeted anywhere. The `Money` type and its two
guards are another ≈6, which the plan does count but thinly.

This is the third change in a row to exceed its allocation. Together 0.1 and 0.2 are now
≈13 person-days against 6. The Demo capacity arithmetic in `IMPLEMENTATION_PLAN.md` §2 needs
re-baselining rather than each change being shaved.

**Ordering.** Apply this only after `bootstrap-monorepo` is archived — the spec delta
modifies a requirement that change introduces, and the modification will not resolve against
an empty baseline.

**Ownership.** Both developers on §1 and §2: these are the roles and the transaction seam,
and 1.1 depends on both. **B** takes River, Money and CI. **A** takes the cluster wiring.
One migration owner this week, per plan §2.1.

## 1. Migrations and roles — both

- [ ] 1.1 Add `goose` to the toolchain, a `/core/migrations` directory, and `make migrate-up`/`make migrate-down`
- [ ] 1.2 Write migration 001 creating `vekst_migrator` and `vekst_app` per design D1, with `vekst_app` explicitly `NOBYPASSRLS` and owning nothing
- [ ] 1.3 Add the `ALTER DEFAULT PRIVILEGES` statements so every table change 1.1 creates is usable by `vekst_app` with no follow-up grant
- [ ] 1.4 Make 001 idempotent with a `DO` block guard, so a second database in the same cluster does not fail on existing roles (design risk 1)
- [ ] 1.5 Write the down step for 001 and confirm `up → down → up` against a scratch database

## 2. Database access — both

- [ ] 2.1 `pgx/v5` pool construction from environment, connecting as `vekst_app`, with sane pool limits and a connect timeout
- [ ] 2.2 The single `InTx` transaction entry point per design D2, documented with the comment that change 1.1 adds `SET LOCAL app.org_id` here and nowhere else
- [ ] 2.3 Add the CI check that fails on a direct pool query or `Begin` outside the database package — the mechanism that keeps D2 true

## 3. sqlc — both

- [ ] 3.1 Configure `sqlc` reading `/core/internal/db/query/*.sql` and writing to `/core/gen/db`; add it to `make gen`
- [ ] 3.2 Write the readiness query, and extend 0.1's codegen drift job to cover the `sqlc` output as well as `buf`

## 4. River — B

- [ ] 4.1 Add River, and wrap its migrator in a goose migration so `goose status` remains the single answer to what has been applied (design D1, migration 002)
- [ ] 4.2 Grant `vekst_app` DML on River's tables; confirm the worker runs as `vekst_app` and the migrator does not
- [ ] 4.3 Construct the River client and worker registry; start and stop them with the server's lifecycle, draining on shutdown
- [ ] 4.4 Register one no-op job proving the wiring
- [ ] 4.5 Test: a job enqueued in a committed transaction runs
- [ ] 4.6 Test: a job enqueued in a rolled-back transaction never exists and never runs — the property that makes `ARCHITECTURE.md` §4a's atomic import possible
- [ ] 4.7 Create `deploy/db/rls-exempt-tables.txt` listing River's and goose's tables with the reason at the top, and add it to `CODEOWNERS` as requiring both reviewers (design D4)
- [ ] 4.8 Document in the worker's package comment that a handler takes its tenant identifier from job arguments and never from ambient state, why, and that change 1.1 adds the context that enforces it — the spec asserts this comment exists, because 0.2 has no tenant concept to enforce against

## 5. Money — B

- [ ] 5.1 Write `/proto/vekst/type/money.proto` per design D5, including `jstype = JS_STRING` on the minor-units field; run `buf breaking`
- [ ] 5.2 Go `Money` type with construction, comparison and addition that refuses two different currency codes
- [ ] 5.3 Checked-in ISO-4217 exponent table, with formatting and parsing going through it rather than a hardcoded 100
- [ ] 5.4 Non-base-currency test: format and parse JPY (exponent 0) and KWD (exponent 3) correctly, and reject adding EUR to PLN
- [ ] 5.5 Go no-float guard: a reflection test failing if any money field is a floating-point type
- [ ] 5.6 Python no-float guard in `/classifier` over the generated types — added now, before 3.2 introduces the first money field
- [ ] 5.7 TypeScript test asserting the generated minor-units field types as `string`, not `number` — asserting the outcome, so it holds regardless of how the generator delivers it

## 6. Readiness — B

- [ ] 6.1 Add `/readyz` checking the pool with a short timeout; leave `/healthz` dependency-free
- [ ] 6.1b Compile the required migration version into the binary; `/readyz` fails and names both versions when `goose_db_version` is behind it (design D6, Q5)
- [ ] 6.2 Test: with the database unreachable, `/readyz` fails and `/healthz` still succeeds
- [ ] 6.3 Test: `HealthService/Check` still returns `STATUS_SERVING` with no database — 0.1's rule, still true
- [ ] 6.4 Test: with the schema behind the binary's required version, `/readyz` fails and the message names the applied and required versions

## 7. Cluster wiring — A

- [ ] 7.1 Database credentials as a Kubernetes Secret in the `local` overlay; `core` gets `vekst_app` only
- [ ] 7.2 Confirm the `classifier` Deployment gains no database environment or Secret — 0.1's spec scenario asserts it and must stay true
- [ ] 7.3 Migration Job in **`deploy/k8s/base`** running `goose up` as `vekst_migrator` — not in an overlay, because every environment migrates and 0.1's manifest requirement forbids an overlay inventing a workload. The overlay supplies only its credentials and image tag
- [ ] 7.3b Order it ahead of `core` with a Tilt resource dependency locally, and confirm no `core` pod becomes Ready before the Job completes
- [ ] 7.4 Move `core`'s readiness probe to `/readyz` in `deploy/k8s/base`; liveness stays on `/healthz`
- [ ] 7.5 Confirm `make down` then `make dev` still reproduces the environment, now including a migrated database

## 8. Continuous integration — B

- [ ] 8.1 Migration round-trip job: `up`, `down` to zero, `up` again against a scratch Postgres, failing when a down step is missing
- [ ] 8.2 Role assertion job: `vekst_app` has `rolbypassrls` false and `rolsuper` false, and cannot create a table
- [ ] 8.3 Extend the drift job to cover `sqlc` output alongside `buf` output
- [ ] 8.4 Confirm the Go, Python and TypeScript no-float guards run in their existing language jobs

## 9. Documentation and close

- [ ] 9.1 Update `README.md`: the migration commands, the two roles and what each is for, and how to reach a psql prompt in the local cluster (A)
- [ ] 9.2 Update `CLAUDE.md`: the single transaction entry point and the rule that nothing else opens one; money as `int64` minor units with no floats; the rule that a job handler takes its tenant identifier from job arguments; and the allowlist's purpose (B)
- [ ] 9.3 Amend `docs/IMPLEMENTATION_PLAN.md` §3 so change 0.2's scope line names River, which it currently does not (both)
- [ ] 9.4 Hand change 1.1 the two items this change deliberately left it: the tenant-argument scenario for workers, and the RLS coverage test that reads `deploy/db/rls-exempt-tables.txt` (both)
- [ ] 9.5 Update the capability spec and run the full suite
