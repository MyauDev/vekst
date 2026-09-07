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

- **`vekst_migrator` stops being a superuser in CI and in the local overlay.** Both set
  `POSTGRES_USER: vekst_migrator`, which makes it the initdb superuser — and a superuser
  bypasses row-level security unconditionally, `FORCE` included. Every hosted Postgres gives a
  plain owner instead, so today's CI would pass a schema whose isolation does not work where
  it matters. Two of this change's own mechanisms depend on the answer, so the change fixes
  the provisioning first and asserts it in a test.
- Migration 004 creates `organizations` → `entities` → `accounts` and `memberships`, each
  with `org_id`, an RLS policy and `FORCE ROW LEVEL SECURITY`. `entity_id` is populated from
  day one, although the Demo gives each organisation one entity.
- **Every referential-integrity check between tenant tables carries `org_id`.** Postgres does
  not apply RLS to these checks, so a plain foreign key is a cross-tenant existence oracle —
  and an unscoped unique constraint is a louder one, disclosing a *value* rather than an
  identifier. Foreign keys become composite; every uniqueness constraint takes `org_id` as its
  leading column.
- `db.InTx` gains a required organisation argument and issues `set_config('app.org_id', …,
  true)` there and nowhere else. A second, deliberately awkward entry point serves the few
  untenanted reads.
- **`OrgID` becomes unforgeable, not just unforgettable.** A required argument cannot be
  forgotten, but a plain newtype over a UUID can be built from a request field — and RLS would
  then faithfully isolate the transaction to the organisation an attacker named. RLS is
  containment, not authorization. The type gains an unexported field and three named
  constructors: an authenticated session whose membership was resolved, a job's own arguments,
  and the creation of a new organisation.
- Login resolves a user's organisations through **a policy scoped to a dedicated `NOLOGIN`
  role**, not through a privileged one. A `SECURITY DEFINER` function does not bypass RLS; it
  runs as its owner, and under `FORCE` the owner is subject to the policy too.
- Tenant context is **fail-closed**: a query outside a tenant transaction raises, rather than
  returning an empty result that reads as "this customer has no data".
- The RLS coverage test 0.2 deferred, asserting isolation rather than the mere existence of a
  policy: every `public` table absent from `deploy/db/rls-exempt-tables.txt` must have RLS
  enabled and forced, an `org_id` column, and a policy that actually consults the tenant
  context and covers writes. It also enumerates the deliberate exceptions — one role-scoped
  policy, one `SECURITY DEFINER` function — so a second of either fails the build.
- A River worker takes its tenant context from its job arguments, never from ambient state.

## Capabilities

### New Capabilities
- `tenancy`: the organisation model and the mechanism that enforces it.

### Modified Capabilities
- `platform-foundation`: two requirements change. **One transaction entry point** — `InTx`
  takes an organisation of a type no other package can construct, and sets the tenant context.
  **Two database roles with distinct powers** — `vekst_migrator` is asserted non-superuser
  alongside `vekst_app`, and a third, non-authenticating role is admitted: it owns the one
  `SECURITY DEFINER` function, cannot log in, and `vekst_app` cannot assume it.

## Non-goals

- **Authentication and sessions** — change 1.2 `add-identity`, which **must land first**:
  `memberships` references `users`, the one global table.
- **Role enforcement.** `memberships.role` is stored, and `OrgIDForSession` returns it because
  it is reading the membership row anyway — but nothing checks it. That is Product.
- **Entity-level isolation.** `entity_id` is stored and constrained; no policy filters on it.
- **Any browser-facing RPC reading tenant data** — the first arrives with 2.1.
- **Data tables.**
- **Soft delete** — and so `organizations.deleted_at` does *not* land here. A column nothing
  sets, nothing reads and no policy excludes is worse than a missing one, because the next
  change assumes it is honoured.

## Impact

Touches `/core/migrations`, `/core/internal/db`, `/core/internal/jobs`, `/deploy/db`,
`/deploy/k8s/overlays/local`, `/scripts`, `.github/CODEOWNERS` and CI.
Every later change inherits the `InTx` signature — cheap to move now, expensive later.
**Apply after `add-identity`.**
