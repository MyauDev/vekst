# Design — add-tenancy-and-rls

**This change touches tenant isolation.** It is the only mechanism that keeps one customer's
finances out of another customer's report. Every decision below is made in favour of failing
loudly over degrading quietly.

## 1. Data model

Migration `00003_tenancy.sql`, applied as `vekst_migrator`. Column shapes follow
`ARCHITECTURE.md` §5.5.

```sql
-- +goose Up

CREATE TABLE organizations (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text        NOT NULL CHECK (length(btrim(name)) > 0),
    country       char(2)     NOT NULL,          -- ISO-3166-1 alpha-2
    base_currency char(3)     NOT NULL,          -- ISO-4217
    created_at    timestamptz NOT NULL DEFAULT now(),
    deleted_at    timestamptz
);

CREATE TABLE entities (
    id            uuid        NOT NULL DEFAULT gen_random_uuid(),
    org_id        uuid        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    name          text        NOT NULL CHECK (length(btrim(name)) > 0),
    legal_name    text,
    tax_id        text,
    base_currency char(3)     NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (org_id, id)                          -- the target of every composite FK below
);

CREATE TABLE accounts (
    id           uuid        NOT NULL DEFAULT gen_random_uuid(),
    org_id       uuid        NOT NULL,
    entity_id    uuid        NOT NULL,
    name         text        NOT NULL CHECK (length(btrim(name)) > 0),
    currency     char(3)     NOT NULL,
    external_ref text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (org_id, id),
    FOREIGN KEY (org_id, entity_id) REFERENCES entities (org_id, id) ON DELETE RESTRICT,
    UNIQUE (org_id, entity_id, external_ref)
);

CREATE TABLE memberships (
    org_id     uuid        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    role       text        NOT NULL CHECK (role IN ('owner','admin','approver','viewer')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, user_id)
);

CREATE INDEX memberships_user_idx ON memberships (user_id);
```

`users` is created by change 1.2 `add-identity` and is deliberately **not** a tenant table —
`ARCHITECTURE.md` §5.5 makes it the single global table, reached through `memberships`. This
change therefore **must be applied after 1.2**, or migration 003's foreign key has no target.

### 1.1 Tenant context and its accessor

```sql
CREATE FUNCTION app_current_org() RETURNS uuid
    LANGUAGE sql STABLE
    AS $fn$ SELECT current_setting('app.org_id')::uuid $fn$;
```

`current_setting` is called with **one** argument on purpose. If `app.org_id` was never set,
it raises `undefined_object` (42704). The two-argument form would return NULL, every policy
would evaluate to false, and a query outside a tenant transaction would return **zero rows
with no error** — which a report renders as "this customer has no revenue". `ARCHITECTURE.md`
§7 already requires the opposite: "A repository call outside that transaction must fail."

### 1.2 Every table, its column and its policy

| Table | Tenant column | RLS | Policy (`USING` and `WITH CHECK`) |
| --- | --- | --- | --- |
| `organizations` | `id` *is* the tenant key | enabled + forced | `id = app_current_org()` |
| `entities` | `org_id` | enabled + forced | `org_id = app_current_org()` |
| `accounts` | `org_id` | enabled + forced | `org_id = app_current_org()` |
| `memberships` | `org_id` | enabled + forced | `org_id = app_current_org()` |
| `users` | none — global | exempt, allowlisted by 1.2 | — |

Each table takes one `FOR ALL` permissive policy with identical `USING` and `WITH CHECK`
expressions. Identical halves matter: `USING` alone would let a tenant *write* a row stamped
with another organisation's `org_id` and then lose sight of it.

Every table also gets `ALTER TABLE … FORCE ROW LEVEL SECURITY`. Without `FORCE`, the table
owner is exempt; `vekst_app` owns nothing today, but a future migration that changes an owner
would silently disable isolation, and nothing would report it.

## 2. Decisions

### D1 — Foreign keys between tenant tables carry `org_id`

Postgres does **not** apply row-level security to referential-integrity checks; the check
runs internally with row security off. A plain `accounts.entity_id REFERENCES entities(id)`
is therefore an existence oracle: tenant A inserts a row pointing at a UUID it guessed, and
the success or failure of the insert reveals whether tenant B owns that entity. Worse, the
row is then accepted while pointing across a tenant boundary.

Composite keys close both holes. `entities` carries `UNIQUE (org_id, id)` so that
`accounts (org_id, entity_id) REFERENCES entities (org_id, id)` is expressible; the FK can
only resolve inside the row's own organisation. Every tenant table added from 2.1 onward
follows this shape, and the pattern is recorded in `CLAUDE.md`.

### D2 — `InTx` takes the organisation as an argument

```go
// core/internal/db
type OrgID uuid.UUID

func InTx(ctx context.Context, org OrgID, fn func(context.Context, pgx.Tx) error) error
func InSystemTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error
```

`InTx` issues `SELECT set_config('app.org_id', $1, true)` as the first statement of the
transaction — `true` means transaction-local, so the value dies at commit or rollback and
cannot survive on a pooled connection. `SET LOCAL` itself takes no parameters, so
`set_config` is used instead of interpolating the value into SQL; interpolating tenant
identifiers into a `SET` statement is an injection seam that would defeat the whole
mechanism.

The organisation is a **required parameter**, not a value pulled from `context`. A parameter
cannot be forgotten — the compiler refuses. A context value can be absent at run time, which
puts the failure in production rather than in the build.

`InSystemTx` sets no tenant context and exists for readiness checks, migration status and
the `users` table. It is intentionally unpleasant to reach for, and
`scripts/check-db-entry-point.sh` grows a second assertion: its call sites are counted, and
the count is committed, so adding one is a visible diff rather than a quiet habit.

### D3 — Resolving a user's organisations at login

Login must answer "which organisations does this user belong to?" *before* any organisation
is known. That read cannot go through `InTx` (there is no org yet) and cannot go through
`InSystemTx` either, because `memberships` has RLS and `app_current_org()` would raise.

The answer is one narrow `SECURITY DEFINER` function, owned by `vekst_migrator` and executable
by `vekst_app`:

```sql
CREATE FUNCTION orgs_for_user(p_user_id uuid)
    RETURNS TABLE (org_id uuid, role text)
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public
    AS $fn$ SELECT m.org_id, m.role FROM memberships m WHERE m.user_id = p_user_id $fn$;
```

It takes one user id, returns only that user's rows, and touches nothing else. It is the
single deliberate exception to "RLS decides what is visible", so it is listed in `CODEOWNERS`
and it carries its own test: a user's id returns that user's memberships and no other's.

### D4 — Creating an organisation under its own policy

`organizations` has `WITH CHECK (id = app_current_org())`, so an insert needs the tenant
context to already name the row being inserted. This is not a problem — it is the design.
`core` generates the UUID, opens `InTx` with it, and inserts. The check passes because the
identifiers match, and no privileged path is needed to create a tenant.

### D5 — Background jobs

River's tables carry no `org_id` and stay allowlisted. A worker reads `OrgID` from its job
arguments and calls the same `InTx`. It never reads ambient state, and there is no code path
by which a worker can acquire a tenant context other than the one written in its own
arguments. The corollary belongs in the worker's package comment: **job arguments are not
tenant-isolated**, so they carry identifiers, never customer financial data.

### D6 — The coverage test

A Go test, run in CI against the migrated scratch database, reads
`deploy/db/rls-exempt-tables.txt` and asserts that every other table in `public` has
`relrowsecurity`, `relforcerowsecurity` and at least one row in `pg_policies`. A new tenant
table with no policy fails here on the pull request that adds it — which is the point, and
why 0.2 seeded the allowlist before the test existed to read it.

## 3. API

**No new RPC.** Neither Connect nor internal gRPC changes in this change. The first
browser-facing call that reads tenant data arrives with 2.1 `add-file-upload`, and it will
inherit the `InTx` signature rather than negotiate its own. `proto/` is untouched, so
`buf breaking` has nothing to report.

## 4. Rejected alternatives

| Rejected | Why |
| --- | --- |
| **Application-level `WHERE org_id = …`, no RLS** | It works until one query forgets, and that query fails silently with plausible numbers. There is no test that proves the absence of a missing predicate; there is a test that proves a policy exists. |
| **`current_setting('app.org_id', true)` (fail-soft)** | Returns NULL when unset, every policy goes false, and a context-less query returns zero rows instead of raising. An empty P&L looks like a quiet month, not a bug. |
| **Tenant context from `context.Context` instead of an argument** | Compiles when it is missing. The parameter form makes the mistake impossible rather than detectable. |
| **A schema or a database per tenant** | Isolation by construction, but migrations, connection pooling and cross-tenant operational work all multiply by the customer count, and `ARCHITECTURE.md` §7 already fixed one shared schema. |
| **Plain single-column foreign keys** | Cheaper to write and a cross-tenant existence oracle, because RLS does not apply to FK checks. See D1. |
| **A `SECURITY DEFINER` function for every tenant read** | Solves D3 generally, and puts the isolation decision back in hand-written function bodies — the exact thing RLS exists to remove. One narrow exception is reviewable; a pattern is not. |
| **`app.user_id` as a second GUC, with a self-membership policy** | Avoids `SECURITY DEFINER`, but forces `app_current_org()` to be fail-soft so the login transaction does not raise — trading a loud error for silent empty results everywhere else. |

## 5. Risks

| Risk | Handling |
| --- | --- |
| A pooled connection carries `app.org_id` into the next request | `set_config(…, true)` is transaction-local. Tested directly: two sequential transactions on the same connection, the second with no context, must raise rather than return the first tenant's rows. |
| A future migration hands a tenant table to a new owner | `FORCE ROW LEVEL SECURITY` on every table; the coverage test asserts `relforcerowsecurity`, not only `relrowsecurity`. |
| `orgs_for_user` grows a second use | It is in `CODEOWNERS`; a change to it needs both reviewers. |
| Someone adds a tenant table with no policy | The coverage test fails the pull request. |
| `InSystemTx` becomes the easy way out | Its call sites are counted and the count is committed. |

## 6. Call-outs

- **Tenant isolation:** the whole change. Cross-tenant scenarios are mandatory on every task
  that adds a table.
- **Money, currency, `source_kind`, the classifier contract:** untouched. `accounts.currency`
  and `*.base_currency` are ISO-4217 codes, not amounts; no money column appears here.
