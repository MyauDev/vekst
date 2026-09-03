# CLAUDE.md

Guidance for working in this repository.

## What this is

Vekst: management reporting for SME owners. Financial data is imported,
classified, and rendered as reports. Three services — `core` (Go, owns all
state), `classifier` (Python, stateless), `web` (React).

Read `docs/ARCHITECTURE.md` before making a structural decision. It records what
was decided and why, including the alternatives that were rejected.

## Commands

```sh
make dev          # k3d cluster + Tilt. Docker must be running
make down         # delete the cluster
make gen          # regenerate all stubs from /proto and /core/internal/db/query
make migrate-up   # apply pending migrations to DATABASE_URL_MIGRATOR
make migrate-down # roll back one migration on DATABASE_URL_MIGRATOR
make lint         # buf lint, go vet, the DB-entry-point check, ruff, mypy, tsc
make test         # go test, pytest, vitest
make ci           # lint + test, what CI runs
```

Run `make ci` before proposing a change is finished.

## Layout and ownership

| Path | Owner | Contains |
| --- | --- | --- |
| `proto/vekst/v1/` | both | browser-facing contract, served over Connect |
| `proto/vekst/internal/v1/` | Track B | `core` → `classifier`, native gRPC |
| `core/internal/ingest/`, `core/internal/dedup/` | Track A | ingest and deduplication |
| `core/classify/`, `core/internal/report/` | Track B | classification boundary, reports |
| `classifier/` | Track B | the Python service |
| `core/internal/db/` | both | the pool and `InTx` — the one transaction entry point |
| `core/internal/jobs/` | both | River: client, worker registry |
| `deploy/k8s/base/` | both | what the application is, in every environment |
| `deploy/db/rls-exempt-tables.txt` | both | infrastructure tables exempt from the RLS coverage test |

`/proto`, `/deploy/k8s/base` and the RLS-exempt allowlist need both reviewers. See
`.github/CODEOWNERS`.

## Invariants

These are constraints on future work, not descriptions of what exists. Most name
code that has not been written yet — that is the point. Violating one is not a
style disagreement; each corresponds to a way this product prints a wrong number
or leaks one customer's finances to another.

**Money is `int64` minor units plus an ISO-4217 code.** Never a float, in any
language. Never a JavaScript `number` on the wire — it cannot hold `int64`
exactly. The type and its no-float guards (Go, Python, and a TypeScript test
of the generated field type) exist in `core/internal/money` and
`proto/vekst/type/v1/money.proto`; no table stores an amount yet. Multi-currency
storage — the original amount, the FX rate, the rate date and the
base-currency amount — is change 2.5.

**Every tenant table carries `org_id`, an RLS policy, and `FORCE ROW LEVEL
SECURITY`.** Tenancy is `organizations` → `entities` → `accounts`; `entity_id`
exists from the first migration even though v1 creates one entity per
organisation. Two database roles: `vekst_migrator` owns the schema, `vekst_app`
owns nothing and has no `BYPASSRLS`. Tenant context is `SET LOCAL app.org_id`
inside the request transaction.

**The classifier never receives database credentials.** Not a connection string,
not a driver, not indirectly. `core` reads the tenant's context under RLS and
passes it in the request. This keeps tenant isolation single-mechanism: one
process holds the connection, so a bug in the classifier cannot cross a tenant
boundary because it never had the ability to. Enforced by a test.

**Classifications are append-only.** A correction inserts a new row superseding
the old one. Every row carries `engine_layer`, `ruleset_version` and
`engine_version`; a report pins `taxonomy_version` + `ruleset_version` +
`engine_version`. Those three strings are what make a March report reproduce in
June, and an accountant will ask.

**One row per payment or posting.** A bank payment is one row with
`document_ref` null. A ledger document with five postings is five rows sharing
one `document_ref`. Never one row per document with line items in JSONB.

**A report line is computed from one `source_kind`.** Mixing ledger and bank
data without a confirmed D4 match counts an invoice and its payment twice. Such
a line is blocked, not guessed.

**Validation is blocking and atomic.** A file that fails persists nothing.
Correctness errors can never be overridden. The error report is keyed by line
number in the original file, never by parsed row index.

**Generated code is never hand-edited.** `core/gen`, `web/src/gen` and
`classifier/src/vekst` come from `make gen` and are drift-checked in CI.

**`core/internal/db.InTx` is the only place a transaction begins.** Every
caller — Connect RPC handler, River worker — goes through it; nothing else may
hold the pool or query it directly. `scripts/check-db-entry-point.sh` enforces
this by import (only `core/internal/db` and `core/internal/jobs` may import
`pgxpool` — the latter because River's own driver needs the raw pool for its
background polling, which touches only River's own infrastructure tables, not
application data). Change 1.1 adds `SET LOCAL app.org_id` inside `InTx`, and
nowhere else.

**A background job sets its own tenant context.** River's tables carry no
`org_id` and no RLS, so a worker takes its tenant identifier from its job
arguments — never from ambient state, the same way a handler trusts only the
authenticated session and never a global. `deploy/db/rls-exempt-tables.txt` is
the checked-in, both-reviewers-required record of which tables are River's or
goose's infrastructure rather than tenant data; change 1.1's RLS coverage test
reads it instead of embedding its own exceptions.

## Conventions

- The engine is written as if it already ran in another process, because it
  does: inputs in, outputs out, no database handle, no clock, no globals.
- Liveness (`/healthz`) never checks a dependency. Readiness (`/readyz`) checks
  the database and the schema version. A liveness probe that checks the
  database turns a recoverable outage into a crash loop.
- The backend returns error codes, never sentences. Translation is the client's.
- An overlay differs from the Kustomize base in configuration only — never in
  the set of workloads.
- Prefer adding a test that fails before adding a rule to a document.

## Working with OpenSpec

Changes live in `openspec/changes/<name>/` as proposal, design, specs and tasks.
`openspec validate <name>` checks them. Implement with `/opsx:apply`.

One change is one capability delta. Every change touching a tenant table adds a
cross-tenant isolation scenario; every change touching money adds a
non-base-currency scenario and a no-float test.
