## Why

`add-postgres-and-migrations` created `vekst_app` — a role that owns nothing and has no
`BYPASSRLS`. With no policy on any table, "no `BYPASSRLS`" restricts nothing. This change is
what turns that role into isolation.

Every customer's financial data shares one database and one schema. The only thing keeping
customer B's transactions out of customer A's Management P&L is a predicate on `org_id`. If
the application owns that predicate, every query, join, report and job must carry it forever,
and the first one that forgets fails *silently*: the report renders, the figures look
plausible, and they are another company's. RLS moves the predicate into the database, which
applies it whether the query remembered it or not.

It must land before the first tenant table: retro-fitting isolation is a migration of every
table, a rewrite of every query, and no proof that nothing was exposed meanwhile.

Milestone: **Demo**. Capability: **`tenancy`** (new), extending **`platform-foundation`**.

## What Changes

- Migration 003 creates `organizations` → `entities` → `accounts` and `memberships`, each
  with `org_id`, an RLS policy and `FORCE ROW LEVEL SECURITY`. `entity_id` is populated from
  day one, although the Demo gives each organisation one entity.
- Foreign keys between tenant tables are **composite and carry `org_id`**. Postgres does not
  apply RLS to referential-integrity checks, so a plain FK is a cross-tenant existence oracle.
- `db.InTx` gains a required organisation argument and issues `set_config('app.org_id', …,
  true)` there and nowhere else. A second, deliberately awkward entry point serves the few
  untenanted reads.
- Tenant context is **fail-closed**: a query outside a tenant transaction raises, rather than
  returning an empty result that reads as "this customer has no data".
- The RLS coverage test 0.2 deferred: every `public` table absent from
  `deploy/db/rls-exempt-tables.txt` must have RLS enabled, forced, and a policy.
- A River worker takes its tenant context from its job arguments, never from ambient state.

## Capabilities

### New Capabilities
- `tenancy`: the organisation model and the mechanism that enforces it.

### Modified Capabilities
- `platform-foundation`: `InTx` sets tenant context; the coverage test joins CI.

## Non-goals

- **Authentication and sessions** — change 1.2 `add-identity`, which **must land first**:
  `memberships` references `users`, the one global table.
- **Role enforcement.** `memberships.role` is stored, never checked. That is Product.
- **Entity-level isolation.** `entity_id` is stored and constrained; no policy filters on it.
- **Any browser-facing RPC reading tenant data** — the first arrives with 2.1.
- **Data tables** and **soft delete.**

## Impact

Touches `/core/migrations`, `/core/internal/db`, `/core/internal/jobs`, `/deploy/db`, CI.
Every later change inherits the `InTx` signature — cheap to move now, expensive later.
**Apply after `add-identity`.**
