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

Three migrations, applied by `goose` as `vekst_migrator`.

**001 — roles and default privileges.** No tables. This runs once per database and is the
foundation of the isolation story in `ARCHITECTURE.md` §7.

```sql
-- Roles are cluster-scoped; created only if absent so the migration is re-runnable.
CREATE ROLE vekst_migrator LOGIN;
CREATE ROLE vekst_app      LOGIN NOBYPASSRLS;

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
river_job
river_leader
river_queue
river_client
river_client_queue
```

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

### D6 — Liveness and readiness split

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

### D7 — sqlc, and what generates what

`sqlc` generates typed query code from `/core/internal/db/query/*.sql` into `/core/gen/db`,
committed and covered by 0.1's drift check. This change adds one query — the readiness
probe's — because a generator with no input proves nothing.

`sqlc` and `buf` are separate generators writing to separate directories under one
`make gen`. Both are checked by the same drift job.

## Risks / Trade-offs

| Risk | Mitigation |
| --- | --- |
| **Roles are cluster-scoped, migrations are database-scoped.** `CREATE ROLE` in a migration is unusual and fails on a second database in the same cluster | Guard with `IF NOT EXISTS` semantics via a `DO` block, and treat 001 as idempotent. Document that a managed Postgres may require roles to be created out-of-band, which is a Q1/Q6 question for the hosted environment |
| A developer opens a transaction outside `InTx`, and 1.1's tenant context silently does not apply | A CI grep for `pool.Begin`/`pool.Query` outside `/core/internal/db`. Cheap, and it fails loudly at the moment the second door is added rather than the day a customer sees another customer's data |
| The RLS allowlist becomes a place to silence the 1.1 test | It is a checked-in file under CODEOWNERS requiring both reviewers, with the reason written at the top. The alternative — a regex inside the test — is edited without anyone noticing |
| `jstype = JS_STRING` may not be honoured identically by every generator | Assert the outcome, not the mechanism: a TypeScript test that fails if the generated `minorUnits` type is `number`. If the option does not deliver it, the fix is a custom scalar, and the test says so immediately |
| Migrations and River's migrator running as different tools produce two histories | River's migration is wrapped in a goose migration, so `goose status` is the single answer to "what has been applied" |
| **2 person-days is optimistic.** These tasks estimate ≈30 hours ≈ 3.75 days, largely because River was not in the original scope line | Stated, not absorbed. See the budget note in `tasks.md`. This is the third change in a row to exceed its allocation; the Demo plan needs re-baselining |

## Migration Plan

- Forward: `goose up` as `vekst_migrator`, run as a Kubernetes Job in the `local` overlay
  before `core` starts, gated by an init container or a Tilt resource dependency.
- Backward: every migration has a tested `-- +goose Down`. CI applies up, down, and up again
  against a scratch database on each PR — the cheapest way to discover that a down migration
  was never written.
- There is no data. Rollback is `goose down` and a revert.
- 0.1's manifests change: `core`'s readiness probe moves from `/healthz` to `/readyz`, and
  the pod gains database credentials from a Secret. The `classifier` Deployment must **not**
  gain them — 0.1's spec scenario asserts this and still applies.

## Open Questions

| # | Question | Blocks | Position taken here |
| --- | --- | --- | --- |
| Q1 | Can the hosted Postgres create roles from a migration, or must `vekst_migrator` and `vekst_app` be provisioned out-of-band? Managed providers usually restrict this | The deployment overlay | Migration 001 is written to be idempotent and skippable. The hosted answer depends on 0.1's Q1/Q6, still open |
| Q2 | Where do database credentials come from in the hosted environment? | The deployment overlay | Kubernetes Secrets in `local`. Anything stronger is a decision for the provisioning change |
| Q3 | Does `vekst_app` need `TRUNCATE`? | Test fixtures | No. Tests truncate as `vekst_migrator`. Granting it to `vekst_app` widens the blast radius of a bug for a convenience only tests want |
| Q4 | Which currencies ship in the exponent table? | 2.5 and the report layer | ISO-4217 in full, checked in as data. It is small, and a missing currency is a runtime failure on a customer's file |

## Rejected alternatives

| Rejected | Why |
| --- | --- |
| **A Redis-backed job queue** (Asynq, or a hand-rolled list) | No shared commit with the data. Enqueue-before-commit runs jobs against rows that never landed; enqueue-after-commit loses jobs on a crash. The fix is a Postgres outbox plus a relay, which is a Postgres queue with more moving parts and a second stateful system to back up |
| A transactional outbox table with a custom relay | That is what River already is, minus the retries, the backoff, the uniqueness handling and the maintenance |
| One database role for both migrating and serving | The application would own its schema and could disable RLS on its own tables. Two roles is the mechanism that makes `FORCE ROW LEVEL SECURITY` meaningful rather than advisory |
| `numeric` in Postgres for money instead of `int64` minor units | Correct arithmetic, but it arrives in Go as a string or a `decimal` type and invites a `float64` conversion at the first careless call site. Minor units make the wrong thing hard to write |
| Deferring the no-float guard until the first money column | The guard is free to satisfy when nothing is money and expensive when twelve columns are. Guards are worth adding before they can fail |
| Putting the database check in `/healthz` | A database blip would restart every pod, converting a recoverable outage into a crash loop. Liveness answers "is this process broken", not "is the system healthy" |
| An ORM instead of `sqlc` | `ARCHITECTURE.md` fixes `sqlc`. It also keeps the SQL visible, which matters when RLS makes the difference between a correct and a catastrophic query invisible at the call site |
