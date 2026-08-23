## Why

`bootstrap-monorepo` leaves an empty Postgres that nothing connects to. Every change from
1.1 onward writes tables, and none can start until migrations, generated queries, the two
database roles and the money representation exist. This is change 0.2 of
`docs/IMPLEMENTATION_PLAN.md` §3.

Milestone: **Demo (2026-10-01)**. Capability: **`platform-foundation`**.

## What Changes

- `core` connects to Postgres 16 through a `pgx/v5` pool, with **one** transaction entry
  point every caller must use — handler and background job alike. Change 1.1 adds
  `SET LOCAL app.org_id` in exactly that one place.
- **`goose` migrations**, and a first migration creating the two roles: `vekst_migrator`
  owns the schema, `vekst_app` owns nothing and has no `BYPASSRLS`. Default privileges make
  every table 1.1 creates usable by `vekst_app` with no follow-up grant.
- **`sqlc`** generating typed queries into `/core/gen`, joining the existing codegen drift
  gate.
- **River**, resolved into this change: migrations, client, worker registry, and one no-op
  job proving enqueue-in-transaction end to end. River stores jobs in Postgres, so a job is
  inserted in the same transaction as the rows it concerns — which is what makes
  `ARCHITECTURE.md` §4a's atomic import possible without a broker.
- **`Money`** as a proto message and a Go type: `int64` minor units plus an ISO-4217 code,
  with per-currency exponents. It reaches TypeScript as a string, never a number.
- **No-float guards** in Go and Python, failing if a money field is ever floating-point —
  added before the first money column exists, the only time a guard is cheap.
- `/readyz` checks the pool; `/healthz` stays dependency-free for liveness.
- The RLS allowlist 1.1's policy test will read, seeded with River's tables.

## Capabilities

### New Capabilities

None. This extends `platform-foundation`.

### Modified Capabilities
- `platform-foundation`: adds persistence, migrations, jobs and money; changes how
  Kubernetes decides a `core` pod is ready.

## Non-goals

- **Tenant tables and RLS policies** — change 1.1. This creates the roles they depend on.
- **The RLS coverage test** — 1.1 writes it; this only seeds the allowlist it reads.
- **Any real background job.** The River job is a no-op proving the wiring. Ingest and
  classification jobs arrive with 2.2 and 3.2.
- **Any money column.** The type and its guards exist; nothing stores an amount yet.
- Auth, ingest, the classification engine, reports, the deployment overlay.

## Impact

Touches `/core`, `/proto`, `/classifier` (guard test only), `/deploy/k8s`, CI. **Apply this
after `bootstrap-monorepo` is archived** — its spec delta modifies a requirement that change
introduces. Every later change inherits this transaction entry point, this migration tooling
and this money type.
