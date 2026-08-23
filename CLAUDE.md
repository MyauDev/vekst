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
make dev     # k3d cluster + Tilt. Docker must be running
make down    # delete the cluster
make gen     # regenerate all stubs from /proto
make lint    # buf lint, go vet, ruff, mypy, tsc
make test    # go test, pytest, vitest
make ci      # lint + test, what CI runs
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
| `deploy/k8s/base/` | both | what the application is, in every environment |

`/proto` and `/deploy/k8s/base` need both reviewers. See `.github/CODEOWNERS`.

## Invariants

These are constraints on future work, not descriptions of what exists. Most name
code that has not been written yet — that is the point. Violating one is not a
style disagreement; each corresponds to a way this product prints a wrong number
or leaks one customer's finances to another.

**Money is `int64` minor units plus an ISO-4217 code.** Never a float, in any
language. Never a JavaScript `number` on the wire — it cannot hold `int64`
exactly. Multi-currency stores the original amount, the FX rate, the rate date
and the base-currency amount. Change 0.2 adds the type and the guards.

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

**A background job sets its own tenant context.** River's tables carry no
`org_id` and no RLS, so a worker takes its tenant identifier from its job
arguments — never from ambient state. Arrives with change 0.2.

## Conventions

- The engine is written as if it already ran in another process, because it
  does: inputs in, outputs out, no database handle, no clock, no globals.
- Liveness (`/healthz`) never checks a dependency. Readiness (`/readyz`, change
  0.2) does. A liveness probe that checks the database turns a recoverable
  outage into a crash loop.
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
