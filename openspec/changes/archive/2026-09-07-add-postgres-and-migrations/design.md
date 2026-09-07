## Context

`bootstrap-monorepo` (0.1) leaves three services running and an empty Postgres 16 that
nothing connects to. This change makes the database real: a connection, a migration tool,
generated queries, the two roles, background jobs, and the money representation.

It is the last change before tenancy. From 1.1 onward every table carries `org_id`, an RLS
policy and `FORCE ROW LEVEL SECURITY`, and the value of this change is measured by how few
decisions 1.1 has to revisit. Two things in particular are built here so that 1.1 has one
place to change rather than many: **a single transaction entry point**, and **the two
database roles**.

River is resolved into this change (0.1 design Q3). It is the reason `ARCHITECTURE.md` §9
needs no message broker: because River stores jobs in Postgres, a job can be enqueued in the
same transaction as the rows it is about.

Constraints inherited from 0.1: generated code is committed and drift-checked; the classifier
holds no database credentials; three services, one Ingress; `/healthz` is dependency-free.

## Goals / Non-Goals

**Goals:**

- A migration applies, is idempotent, and can be rolled back; CI proves it on every PR.
- Exactly one code path opens a transaction, so 1.1 can add `SET LOCAL app.org_id` once.
- `vekst_app` cannot bypass RLS, and cannot be granted the ability by accident later.
- A job enqueued in a transaction that rolls back does not run. Proved by a test, not
  asserted in prose.
- Money is representable and its floating-point misuse is caught mechanically, in both
  languages, before any money column exists.

**Non-Goals:**

- Tenant tables, RLS policies, and the RLS coverage test — change 1.1.
- Any real background job. Ingest is 2.2, classification is 3.2.
- Any table with a money column. The type exists; nothing stores an amount.
- FX rates, rate sources, base-currency conversion — change 2.5 (D-5 in the plan).

## Decisions

### D1 — Data model

Three migrations, applied by `goose` as `vekst_migrator`. Why `goose` and not another
migration tool is argued in D8; it was inherited from `ARCHITECTURE.md` §8 rather than
chosen here, and three of this change's decisions turn out to depend on it.

**001 — roles and default privileges.** No tables. This runs once per database and is the
foundation of the isolation story in `ARCHITECTURE.md` §7.

`vekst_migrator` is **not** created here. `goose` authenticates as it before any migration
runs, so it exists first in every environment by definition — see Q1. It is provisioned by
the environment, which also makes it the owner of database `vekst` and of schema `public`;
the `REVOKE` below requires that ownership.

```sql
-- vekst_migrator already exists: it is the role running this migration (Q1).
-- Fail loudly rather than three statements later if that is somehow untrue.
DO $$ BEGIN
  IF current_user <> 'vekst_migrator' THEN
    RAISE EXCEPTION 'migrations must be applied as vekst_migrator, not %', current_user;
  END IF;
END $$;

-- Roles are cluster-scoped and migrations are database-scoped, so guard the
-- creation: a second database in the same cluster must not fail on an existing
-- role. Needs CREATEROLE on the migrator, never superuser.
CREATE ROLE vekst_app LOGIN NOBYPASSRLS;   -- inside a DO/IF NOT EXISTS guard

GRANT CONNECT ON DATABASE vekst TO vekst_app;
GRANT USAGE   ON SCHEMA public  TO vekst_app;

-- vekst_app owns nothing and may not create anything.
REVOKE CREATE ON SCHEMA public FROM vekst_app;
REVOKE ALL    ON SCHEMA public FROM PUBLIC;

-- Every table change 1.1 and later create becomes usable by vekst_app with no
-- follow-up grant. Forgetting this once produces a permission error in production
-- and a hotfix migration; setting it now costs three lines.
ALTER DEFAULT PRIVILEGES FOR ROLE vekst_migrator IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES    TO vekst_app;
ALTER DEFAULT PRIVILEGES FOR ROLE vekst_migrator IN SCHEMA public
  GRANT USAGE, SELECT                  ON SEQUENCES TO vekst_app;
```

`vekst_app` is explicitly `NOBYPASSRLS`. It is not enough that it lacks the attribute today;
a test asserts it, so a later `ALTER ROLE` cannot quietly grant it.

The `DO` block above catches the wrong-identity case. The spec's role requirement also
commits to a second failure mode — a migrator that *is* `vekst_migrator` but lacks
`CREATEROLE` — which needs its own guard around the `CREATE ROLE` statement itself, not just
around `current_user`:

```sql
-- Second guard, around CREATE ROLE itself: a migrator missing CREATEROLE gets a
-- bare "permission denied for database" from Postgres. Catch it and re-raise
-- naming the role to provision (Q1), rather than three cryptic statements in.
DO $$ BEGIN
  CREATE ROLE vekst_app LOGIN NOBYPASSRLS;
EXCEPTION WHEN insufficient_privilege THEN
  RAISE EXCEPTION 'vekst_migrator needs CREATEROLE to provision vekst_app; see Q1';
WHEN duplicate_object THEN
  NULL; -- idempotent per risk row 1
END $$;
```

Confirm the exact exception class on task 1.2/1.4 — this is illustrative, not tested SQL.

**002 — River's schema.** Applied by River's own migrator, wrapped in a goose migration so
there is one ordered history rather than two. Creates `river_job`, `river_leader`,
`river_queue` and friends.

`river_job` carries no `org_id` and gets **no RLS policy**. It is infrastructure, not tenant
data. See D4 for what follows from that.

**003 — nothing.** There is no third migration in this change, and that is the point: no
business table exists until 1.1 creates `organizations`. Recorded so a reader does not go
looking for one.

**New tables introduced by this change, with their `org_id` column and RLS policy:** none.
The only tables created are River's, which are infrastructure and are allowlisted in D4.

### D2 — One transaction entry point

Every database access — HTTP handler, Connect RPC, River worker — goes through one function:

```go
// InTx is the only place in the codebase that begins a transaction.
// Change 1.1 adds SET LOCAL app.org_id here, and nowhere else.
func (d *DB) InTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error
```

The pool is configured with `vekst_app` credentials. `vekst_migrator` credentials are used
only by the migration job and are not present in the `core` container's environment.

Why this matters more than it looks: `ARCHITECTURE.md` §7 requires that "a repository call
outside that transaction must fail". That is enforceable only if there is one door. A lint
rule or review convention forbidding `pool.Query` outside `/core/internal/db` keeps it that
way; a CI grep is enough for now.

### D3 — River, and why no broker

River is a Go job queue that stores jobs in Postgres. `client.InsertTx` enqueues inside the
caller's transaction:

```
BEGIN;
  INSERT INTO transactions ...        -- 2.5's work
  INSERT INTO import_validations ...  -- 2.3's work
  river.InsertTx(ctx, tx, ClassifyBatchArgs{...})
COMMIT;                               -- the job exists iff the rows do
```

With a Redis-backed queue this is two systems and no shared commit: enqueue before the commit
and a worker can read rows that never landed; enqueue after it and a crash in between loses
the job silently, leaving a batch stuck in `classifying` forever. The usual fix is a
transactional outbox table in Postgres plus a relay — which is a Postgres queue with extra
moving parts. `ARCHITECTURE.md` §9 rejects a broker for the same reason.

This change registers one worker for a no-op job, purely to prove the property above with a
test that rolls back and asserts the job never ran.

### D4 — River's tables are infrastructure, and the seam that follows

`ARCHITECTURE.md` §7 requires a CI test that "lists tenant tables without an RLS policy and
fails the build". That test belongs to change 1.1. River's tables would fail it.

This change therefore creates the allowlist the test will read — a checked-in file, not a
regex in a script, so that adding to it is a reviewable act:

```
# deploy/db/rls-exempt-tables.txt
# Tables that are infrastructure, not tenant data. Adding a line here is a
# security decision and requires both reviewers (CODEOWNERS).
goose_db_version
river_migration
river_job
river_leader
river_queue
river_notification
```

`river_migration` is River's own version table. Wrapping River's migrator in a goose
migration gives one *ordered* history, not one table — River still records what it applied.

Confirmed against `github.com/riverqueue/river v0.47.0`, the version task 4.1 pins: its
`main` migration line (`riverdriver/riverdatabasesql/migration/main`) creates exactly
`river_migration`, `river_job` (plus the `river_job_state` enum, which the allowlist does not
need to name — it is a type, not a table), `river_leader`, `river_queue` and
`river_notification`. `river_client` and `river_client_queue` do not exist in this version —
an earlier draft of this list named them from memory rather than from the migrations
themselves; if a future River upgrade adds or renames a table, task 4.7b's test catches it
before 1.1's coverage test would.

**The rule that must travel with it:** because River's tables have no RLS, a job handler
must set its own tenant context. A worker calls `InTx` and 1.1's `SET LOCAL app.org_id` is
applied from the job's arguments, exactly as an HTTP request applies it from the session. A
job payload is untrusted input for tenancy purposes, never ambient context. A background job
that runs with no tenant filter is precisely the failure §7 exists to prevent, and it is
easier to introduce in a worker than in a handler because there is no request to notice its
absence.

### D5 — Money

A proto message, a Go type, and no floating point anywhere near either.

```protobuf
// vekst/type/money.proto — shared by the browser API and the classifier contract.
message Money {
  // ISO-4217, uppercase. "EUR", "PLN", "KZT".
  string currency_code = 1;

  // Signed amount in the currency's minor units. 1234 with "EUR" is 12.34 EUR.
  // jstype = JS_STRING so this reaches TypeScript as a string and never as a
  // JS number, which cannot hold int64 exactly. ARCHITECTURE.md §6.
  int64 minor_units = 2 [jstype = JS_STRING];
}
```

Go carries `Money{CurrencyCode string; MinorUnits int64}` with arithmetic that refuses to
operate on two different currencies rather than silently picking one.

**Currency exponents.** Minor units are only meaningful with the currency's exponent: most
currencies use 2, JPY uses 0, KWD and BHD use 3. A small checked-in table maps code to
exponent; formatting and parsing go through it. This is data that changes rarely and is
wrong silently when guessed, so it is a table rather than a constant `100`.

**The guard, in both languages.** A reflection test walks the generated and hand-written
types and fails if any field whose name or type marks it as money is a float. In Python the
same guard runs over the classifier's generated types. `ARCHITECTURE.md` §6 asks for this
explicitly because pandas makes float the default path — and the guard is added now, before
the first money column, because that is the only moment it costs nothing to satisfy.

### D6 — Liveness and readiness split, and the schema-version gate

0.1 put both Kubernetes probes on `/healthz`, which was correct when `core` had no
dependencies. Now it has one.

| Probe | Endpoint | Checks | Failure means |
| --- | --- | --- | --- |
| Liveness | `/healthz` | the process responds | restart the pod |
| Readiness | `/readyz` | the process responds **and** the pool answers | stop sending traffic |

Liveness must not check the database. If it did, a database blip would restart every `core`
pod, turning a recoverable outage into a crash loop that makes recovery slower.

`HealthService/Check` is unchanged and stays database-free — it remains the unauthenticated
RPC, and 0.1's rule that it reads no tenant data still holds.

**Readiness also gates on the schema version.** The binary derives the migration version it
requires from the migrations it embeds (Q5 — derived, never declared) and compares it against
`goose_db_version` at startup; if the database is behind, `/readyz` fails and names both
versions. Without this, a deploy that races its migration
serves queries against a schema that does not have the columns the code expects — which in a
reporting product means wrong numbers rather than an error.

### D7 — sqlc, and what generates what

`sqlc` generates typed query code from `/core/internal/db/query/*.sql` into `/core/gen/db`,
committed and covered by 0.1's drift check. This change adds one query — the readiness
probe's — because a generator with no input proves nothing.

`sqlc` and `buf` are separate generators writing to separate directories under one
`make gen`. Both are checked by the same drift job.

### D8 — Why `goose`, and not the alternatives

`goose` was fixed by `ARCHITECTURE.md` §8 and `openspec/config.yaml` before this change
existed, and was never argued anywhere. It is argued here once, because three decisions in
this change depend on properties not every migration tool has — and because a tool chosen by
inheritance is a tool nobody has checked.

| Depends on | Property | Why it is not generic |
| --- | --- | --- |
| D1 / migration 002 | **Go functions as migrations** | River's migrator is a Go API. `goose` registers a Go function in the same ordered history as the SQL migrations, so `goose status` stays the single answer to what has been applied. A SQL-file-only tool forces River's generated SQL to be vendored and re-vendored on every River upgrade — drift in the one table that cannot be rebuilt from source |
| D6 / the readiness gate | **A plain integer version table** | `/readyz` compares the binary's required version against `goose_db_version` with one query. Tools that add failure state to that table — `golang-migrate`'s `dirty` flag — make the gate ambiguous: "behind" and "wedged" are different conditions needing different responses |
| D7 / `sqlc` | **A schema source `sqlc` already parses** | `sqlc` reads `-- +goose Up` / `-- +goose Down` annotations directly, so the migrations *are* the schema definition. Any other format means a second schema file to keep in sync, and the drift gate cannot see a disagreement between two hand-written files |

**To verify on task 4.1.** `goose`'s Go migrations run against `database/sql`, not `pgx`.
River ships drivers for both, so wrapping its migrator means using its `database/sql` driver
inside the migration while the application keeps `pgx/v5` — or taking the SQL route via
River's CLI and checking the output in. Confirm which before writing 002; it is the one place
this decision could still cost an afternoon.

## Risks / Trade-offs

| Risk | Mitigation |
| --- | --- |
| **Roles are cluster-scoped, migrations are database-scoped.** `CREATE ROLE` in a migration is unusual and fails on a second database in the same cluster | Guard `vekst_app` with `IF NOT EXISTS` semantics via a `DO` block, and treat 001 as idempotent. `vekst_migrator` is not created by a migration at all — it is provisioned by the environment, because it is the role the migration runs as (Q1). **Confirmed live**: `CREATE DATABASE vekst2` in the same cluster, then `migrate up` against it — 001 ran clean against the already-existing `vekst_app` role, and `vekst2`'s own tables got correct default privileges independently |
| **The migrator may lack `CREATEROLE` on a managed provider**, so 001 cannot create `vekst_app` | 001's `DO` block raises an exception naming the role to provision, instead of failing on a permission error. The escape hatch is the same path Q1 already uses for `vekst_migrator` |
| A developer opens a transaction outside `InTx`, and 1.1's tenant context silently does not apply | A CI grep for `pool.Begin`/`pool.Query` outside `/core/internal/db`. Cheap, and it fails loudly at the moment the second door is added rather than the day a customer sees another customer's data |
| The RLS allowlist becomes a place to silence the 1.1 test | It is a checked-in file under CODEOWNERS requiring both reviewers, with the reason written at the top. The alternative — a regex inside the test — is edited without anyone noticing |
| `jstype = JS_STRING` may not be honoured identically by every generator | Assert the outcome, not the mechanism: a TypeScript test that fails if the generated `minorUnits` type is `number`. If the option does not deliver it, the fix is a custom scalar, and the test says so immediately |
| Migrations and River's migrator running as different tools produce two histories | River's migration is wrapped in a goose migration, so `goose status` is the single answer to "what has been applied" |
| **2 person-days is optimistic.** These tasks estimate ≈30 hours ≈ 3.75 days, largely because River was not in the original scope line | Stated, not absorbed. See the budget note in `tasks.md`. This is the third change in a row to exceed its allocation; the Demo plan needs re-baselining |

## Migration Plan

- Forward: `goose up` as `vekst_migrator`, run as a Kubernetes Job **in `k8s/base`**, not in
  an overlay. Every environment migrates, so a Job that lived only in `local` would force the
  deployment overlay to reinvent it — exactly the divergence 0.1's manifest requirement
  forbids. The overlay supplies the Job's credentials and image tag and nothing else.
  Ordering is a Tilt resource dependency locally and Job completion elsewhere.
- Backward: every migration has a tested `-- +goose Down`. CI applies up, down, and up again
  against a scratch database on each PR — the cheapest way to discover that a down migration
  was never written.
- There is no data. Rollback is `goose down` and a revert.
- 0.1's manifests change: `core`'s readiness probe moves from `/healthz` to `/readyz`, and
  the pod gains database credentials from a Secret. The `classifier` Deployment must **not**
  gain them — 0.1's spec scenario asserts this and still applies.

## Open Questions — resolved

All five are answered. What genuinely remains is named at the end, and it belongs to the
provisioning change, not to this one.

### Q1 — Who creates the roles?

**Resolved: `vekst_migrator` is provisioned by the environment; migration 001 creates
`vekst_app` only.**

The question carried a false premise, and finding it is worth more than the answer. 001 as
originally written created the role that applies it. `goose` authenticates as
`vekst_migrator` before any migration runs, so that role necessarily exists first — in every
environment, managed or not. That is a tautology, not a hosting decision, and the managed-
provider question Q1 was really asking had already been forced by the tool.

A second reason was hiding in D1's own SQL. `REVOKE ALL ON SCHEMA public FROM PUBLIC`
requires ownership of schema `public`. Making `vekst_migrator` the initdb user makes it the
owner of the database and hence of the schema, so the existing statement already assumed
this arrangement without saying so.

| | Provisioned by | Needs |
| --- | --- | --- |
| `vekst_migrator` | the environment. Locally `POSTGRES_USER: vekst_migrator` in the `local` overlay — one line of configuration, which the manifest rule permits. Hosted, the provisioning change | nothing from this change |
| `vekst_app` | migration 001, inside the `DO` guard risk row 1 already calls for | `CREATEROLE` on the migrator, never superuser — which every managed provider grants its admin role |

Where `CREATEROLE` is unavailable, 001's guard raises an exception naming the role to
provision, rather than failing on a permission error three statements later. That is the
same escape hatch `vekst_migrator` already uses, so a fully out-of-band environment is
supported without a second code path.

**Task consequences:** 1.2 creates `vekst_app` only and asserts `current_user`; 7.1 sets the
local overlay's `POSTGRES_USER` and drops the `vekst` superuser it ships today. 1.4, 8.1 and
8.2 are unchanged — the round trip and the role assertions still hold.

### Q2 — Where do credentials come from?

**Resolved in shape; the source stays with the provisioning change.**

Fix the interface now, defer the supply. `core` reads one `DATABASE_URL` from a Secret named
`vekst-db-app`, key `url`. The migration Job reads `vekst-db-migrator`, key `url`. Both names
are fixed in `k8s/base`; only the contents differ per environment, so an overlay supplies
credentials without restating a workload — which is what 0.1's manifest requirement demands.

**Two Secrets, not two keys in one.** The spec scenario "migrator credentials are absent from
the serving container" then holds by construction: `core` mounts a Secret that does not
contain them. One Secret with two keys would make that scenario a matter of review
discipline, and reviewers do not read Deployment YAML twice.

Still with the provisioning change: where the contents come from — sealed secrets, External
Secrets, SOPS — and rotation. Neither touches the base.

### Q5 — How does `core` learn the schema version it requires?

**Resolved: derive it from the migrations directory. Never declare it.**

The earlier position was a constant compiled into the binary. Its failure mode is that
someone adds a migration and does not bump the constant, so the gate passes exactly when it
was needed. A declared number is a second fact that can disagree with the first, and a gate
that can silently agree with a stale value is not a gate.

Instead: `//go:embed migrations/*.sql` in the package that owns migrations, with the required
version computed at init as the highest filename prefix in the embedded filesystem. Adding a
migration file *is* bumping the version. There is nothing to forget.

This also settles what the migration Job runs, which task 7.3 left implicit. The CI image
matrix builds `core`, `classifier` and `web` and nothing else, so the Job runs the `core`
image with a `goose up` argument rather than an image of its own. The binary that migrates
and the binary that serves are then the same binary reading the same embedded files, and
"new code, old schema" reduces to "the Job has not finished yet" — precisely what D6's gate
reports and what task 7.3b orders.

### Q3 — Does `vekst_app` need `TRUNCATE`?

**Confirmed: no. Tests truncate as `vekst_migrator`.**

Unchanged, with one wrinkle worth recording before 1.1 writes its first fixture. Under
`FORCE ROW LEVEL SECURITY`, a `DELETE FROM t` issued as `vekst_app` removes only the rows
visible in the current tenant context, so a cleanup helper running as `vekst_app` leaves
other tenants' rows behind and looks broken. That is the policy working, and someone will
lose an hour to it.

`TRUNCATE` is the opposite hazard: it ignores row-level security entirely, even for a
non-superuser. That is a second and better reason not to grant it to the role that serves
requests — it is not merely a wider blast radius, it is the one statement that makes the
isolation mechanism irrelevant.

### Q4 — Which currencies ship in the exponent table?

**Confirmed: ISO-4217 in full, as checked-in data, with the list's publication date recorded
in the file.**

Three details the earlier position did not carry:

- **The list is versioned by ISO and changes** — currencies are added and withdrawn. The
  file records which publication it came from, so "why is this currency missing" has an
  answer that is not "nobody knows".
- **An unknown code is an error, never a default exponent of 2.** A silent default is how a
  KWD amount becomes wrong by a factor of ten in a report nobody re-checks. This is the same
  failure class as guessing a number locale, which stage ② blocks for the same reason.
- **The table is data, not a generated artefact.** It does not come from `/proto` and does
  not pass through `make gen`, so the codegen drift gate does not cover it. A test that
  parses the file and asserts a few known entries — EUR 2, JPY 0, KWD 3 — is what covers it
  instead.

This is also why D5 puts `currency_code` on the wire as a `string` rather than a proto
`enum`: an enum makes every ISO addition a contract change, and `buf breaking` at `FILE`
level would be right to complain about it.

### What remains open

| # | Question | Owner |
| --- | --- | --- |
| — | Where hosted Secret contents come from, and rotation | the provisioning change |
| — | Whether `goose`'s Go migrations can drive River's migrator on `database/sql` while the application stays on `pgx/v5`, or whether 002 takes the checked-in SQL route | verify on task 4.1 — see D8 |

## Rejected alternatives

| Rejected | Why |
| --- | --- |
| **A Redis-backed job queue** (Asynq, or a hand-rolled list) | No shared commit with the data. Enqueue-before-commit runs jobs against rows that never landed; enqueue-after-commit loses jobs on a crash. The fix is a Postgres outbox plus a relay, which is a Postgres queue with more moving parts and a second stateful system to back up |
| A transactional outbox table with a custom relay | That is what River already is, minus the retries, the backoff, the uniqueness handling and the maintenance |
| One database role for both migrating and serving | The application would own its schema and could disable RLS on its own tables. Two roles is the mechanism that makes `FORCE ROW LEVEL SECURITY` meaningful rather than advisory |
| `numeric` in Postgres for money instead of `int64` minor units | Correct arithmetic, but it arrives in Go as a string or a `decimal` type and invites a `float64` conversion at the first careless call site. Minor units make the wrong thing hard to write |
| Deferring the no-float guard until the first money column | The guard is free to satisfy when nothing is money and expensive when twelve columns are. Guards are worth adding before they can fail |
| Putting the database check in `/healthz` | A database blip would restart every pod, converting a recoverable outage into a crash loop. Liveness answers "is this process broken", not "is the system healthy" |
| **`golang-migrate` instead of `goose`** | SQL files only, so River's Go migrator cannot be wrapped in the migration history — you vendor its generated SQL and re-vendor it on every upgrade. Its `schema_migrations` also carries a `dirty` flag whose recovery is a manual `force`; with the migration Job in `k8s/base` running on every deploy, one failed migration wedges every deploy after it until a human intervenes |
| **Atlas** | Genuinely good, and the declarative model would suit a schema this size. Rejected on cost of ownership: a second toolchain to pin, learn and drift-check in a change already at ≈30 hours, with the linting that justifies it behind a commercial tier. Revisit when hand-written migrations start colliding — plan §2.1 already expects that |
| **`tern`** | `pgx`-native, which is a real fit, but no Go migrations, a small ecosystem, and `sqlc` has no awareness of its file format — so the schema would be defined twice |
| **No migration tool; a `schema.sql` applied by hand** | No ordered history, no tested down step, and nothing for D6's readiness gate to read. The gate needs a version number that exists in the database |
| An ORM instead of `sqlc` | `ARCHITECTURE.md` fixes `sqlc`. It also keeps the SQL visible, which matters when RLS makes the difference between a correct and a catastrophic query invisible at the call site |
