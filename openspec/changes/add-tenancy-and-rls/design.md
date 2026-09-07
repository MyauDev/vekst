# Design — add-tenancy-and-rls

**This change touches tenant isolation.** It is the only mechanism that keeps one customer's
finances out of another customer's report. Every decision below is made in favour of failing
loudly over degrading quietly.

## 1. Data model

Migration `00004_tenancy.sql`, applied as `vekst_migrator` (change 1.2 `add-identity` takes `00003`). Column shapes follow
`ARCHITECTURE.md` §5.5.

```sql
-- +goose Up

CREATE TABLE organizations (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text        NOT NULL CHECK (length(btrim(name)) > 0),
    country       text        NOT NULL CHECK (country ~ '^[A-Z]{2}$'),      -- ISO-3166-1 alpha-2
    base_currency text        NOT NULL CHECK (base_currency ~ '^[A-Z]{3}$'), -- ISO-4217, see §1.4
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE entities (
    id            uuid        NOT NULL DEFAULT gen_random_uuid(),
    org_id        uuid        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    name          text        NOT NULL CHECK (length(btrim(name)) > 0),
    legal_name    text,
    tax_id        text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (org_id, id)                          -- the target of every composite FK below
);

CREATE TABLE accounts (
    id           uuid        NOT NULL DEFAULT gen_random_uuid(),
    org_id       uuid        NOT NULL,
    entity_id    uuid        NOT NULL,
    name         text        NOT NULL CHECK (length(btrim(name)) > 0),
    currency     text        NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
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
change therefore **must be applied after 1.2**, or this migration's foreign key has no
target. 1.2 also allowlists `user_identities`, `sessions` and `auth_flows` for the same
reason: none of them belongs to an organisation.

Three column-shape decisions above differ from `ARCHITECTURE.md` §5.5's sketch, and each is
deliberate.

**`text` with a `CHECK`, not `char(n)`.** `char(n)` is blank-padded: a two-character value in a
`char(3)` column compares equal to its unpadded form but does not `length()` equal to it, and
the padding travels into Go as part of the string. The shape check is what §5.5 actually meant.

**No `deleted_at` yet.** The proposal's non-goals defer soft delete, so the column would land
with nothing setting it, nothing reading it and — importantly — no policy excluding
soft-deleted rows. A column with no semantics is worse than a missing one, because the next
change assumes it is honoured. It arrives with the change that defines what it means.

**`base_currency` on `organizations` only, not on both.** §5.5 puts it on `entities` too, with
nothing relating the two. Two unconstrained copies of the same fact is precisely how a report
prints a wrong number: when 2.5 converts to base currency, whichever copy the query happens to
read wins, and the two can disagree with no error. The organisation is the reporting boundary
for the Demo — one entity per organisation — so the organisation holds it. When a holding
customer needs per-entity reporting currency, that change adds the column *and* the rule for
which one applies, together.

**Open, deferred to 2.5, not to be re-decided silently:** the checked-in ISO-4217 list in
`core/internal/money/exponents.go` is the authority on which codes exist, and the `CHECK`
above only constrains shape, so `'XXX'` is storable and fails later in Go. A `currencies`
reference table generated from `exponents.go`, with real foreign keys, would make the list one
artifact instead of two — but it is a global non-tenant table, so it needs an allowlist row
and both reviewers. That belongs with the change that first stores an amount.

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
| `memberships` | `org_id` | enabled + forced | `org_id = app_current_org()`, **plus** one `FOR SELECT` policy scoped `TO vekst_membership_reader` (D3) |
| `users`, `user_identities`, `sessions`, `auth_flows` | none — global | exempt, allowlisted by 1.2 | — |

Each table takes one `FOR ALL` permissive policy with identical `USING` and `WITH CHECK`
expressions, written out in full.

Both halves are written explicitly for legibility and for the coverage test (D6), **not**
because omitting `WITH CHECK` would open a write hole. It would not: on a `FOR ALL` policy
Postgres derives `WITH CHECK` from `USING` when it is absent — "the policy above implicitly
provides a `WITH CHECK` clause identical to its `USING` clause". Verified against Postgres 16:
a deliberately `USING`-only `FOR ALL` policy still rejects an insert stamped with another
organisation's `org_id` (`new row violates row-level security policy`).

The reason to write both anyway is that the equivalence stops holding the moment anyone splits
the policy per command. `FOR SELECT` has no `WITH CHECK` at all, and `FOR INSERT` has no
`USING`; a later refactor into per-command policies inherits nothing. Spelling both halves out
now means the split starts from two visible expressions rather than one implied one, and it
gives D6 a `polwithcheck` to assert on. A coverage test must therefore treat a NULL
`polwithcheck` on a `FOR ALL` policy as "defaults to `USING`", not as a failure.

Every table also gets `ALTER TABLE … FORCE ROW LEVEL SECURITY`. Without `FORCE`, the table
owner is exempt; `vekst_app` owns nothing today, but a future migration that changes an owner
would silently disable isolation, and nothing would report it.

## 2. Decisions

### D0 — `vekst_migrator` is **not** a superuser, and CI provisions it that way

Every mechanism below depends on one unwritten fact: what privileges the schema owner holds.
Today the answer is an accident of packaging. `deploy/k8s/overlays/local/postgres.yaml` and
`.github/workflows/ci.yaml` both set `POSTGRES_USER: vekst_migrator`, and the Postgres image's
`POSTGRES_USER` is the **initdb user** — a superuser. Every hosted Postgres (RDS, Cloud SQL,
Neon) will instead hand us a plain database owner, because managed services do not give out
superuser at all.

That divergence is not cosmetic. `FORCE ROW LEVEL SECURITY` subjects the *table owner* to
policies, but "superusers and roles with the `BYPASSRLS` attribute always bypass the row
security system" regardless of `FORCE`. So a superuser-owned schema silently disables the two
mechanisms this change is built on, in exactly the environment where the tests run. The
failure mode is the worst available: **green in CI, broken on the first login in production.**

**Decision: `vekst_migrator` is a non-superuser role with `CREATEROLE` and ownership of
database `vekst` and schema `public`, in every environment, and CI is changed to provision it
that way.** Migration 00001 already needs only `CREATEROLE` plus that ownership; nothing in it
requires superuser. The published guidance is the same — "create at least one role that has
CREATE USER and CREATE DATABASE permissions but is not a superuser", and "test as the
application role, never as a superuser".

| | `vekst_migrator` as superuser | `vekst_migrator` as plain owner *(chosen)* |
| --- | --- | --- |
| RLS under `FORCE` | Bypassed silently. `FORCE` is decorative. | Enforced. `FORCE` means what it says. |
| `SECURITY DEFINER` functions it owns | Inherit the bypass — an unbounded implicit privilege | Hold only what is granted to them |
| Fidelity to production | None. CI proves nothing about a managed Postgres | CI is the same shape as production |
| Migrations touching tenant rows | Just work, and no one notices they were bypassing RLS | Fail loudly until D8's idiom is used |
| Blast radius of a leaked credential | Total: read/write everything, disable logging, drop anything | Schema and data of one database, still under policy |
| `CREATE EXTENSION`, replication, `COPY` from file | Available | Not available; a hosted environment would not have given them either |
| Cost | Nothing to write today | One CI/overlay provisioning change, plus D3 and D8 below |

The one real cost is the last row: two mechanisms need explicit privileges rather than
inherited ones. That cost is the point — an explicit grant is reviewable, and an inherited
superuser bypass is not.

**This is asserted, not assumed.** A test reads `pg_roles` and fails if `vekst_migrator` has
`rolsuper` or `rolbypassrls`, and if `vekst_app` has either. Without that assertion the
divergence returns the first time someone simplifies a compose file.

### D1 — Every referential-integrity check between tenant tables carries `org_id`

Postgres does **not** apply row-level security to referential-integrity checks. The rule is
broader than foreign keys, and the documentation states both halves in one sentence:
"Referential integrity checks, such as **unique or primary key constraints** and foreign key
references, always bypass row security to ensure that data integrity is maintained."

**Foreign keys.** A plain `accounts.entity_id REFERENCES entities(id)` is an existence oracle:
tenant A inserts a row pointing at a UUID it guessed, and the success or failure of the insert
reveals whether tenant B owns that entity. Worse, the row is then accepted while pointing
across a tenant boundary. Composite keys close both holes. `entities` carries
`UNIQUE (org_id, id)` so that `accounts (org_id, entity_id) REFERENCES entities (org_id, id)`
is expressible; the FK can only resolve inside the row's own organisation.

**Unique constraints.** The same bypass makes a *global* unique constraint on a tenant table a
louder oracle than the FK, because it discloses a value rather than an identifier: a
duplicate-key error on `UNIQUE (external_ref)` tells tenant A that some invisible tenant
already holds that reference. Every uniqueness constraint on a tenant table is therefore
scoped by `org_id` as its leading column. The tables here already comply —
`entities UNIQUE (org_id, id)`, `accounts UNIQUE (org_id, id)` and
`UNIQUE (org_id, entity_id, external_ref)`, `memberships PRIMARY KEY (org_id, user_id)` — and
the surrogate `PRIMARY KEY (id)` columns are `gen_random_uuid()` values, which disclose
nothing because a collision is not reachable by guessing.

This is the rule 2.1 onward will actually be tempted to break: `import_profiles` wants
`UNIQUE (name)` and must have `UNIQUE (org_id, name)`. Both halves — composite FKs and
org-leading unique constraints — are recorded in `CLAUDE.md` and in `ARCHITECTURE.md` §7 as
one rule about referential integrity, not two rules about two constraint kinds.

### D2 — `InTx` takes the organisation as an argument

```go
// core/internal/db
type OrgID struct{ v uuid.UUID }   // unexported field -- see D7

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
puts the failure in production rather than in the build. Making it un*forgettable* is only
half the seam, though; D7 covers the other half, which is making it un*forgeable*.

`InSystemTx` sets no tenant context and exists for readiness checks, migration status and
the identity tables. **1.2 already created it**, renaming the original entry point while it
still had no call sites, so this change only *adds* the tenant-aware `InTx` beside it and
renames nothing. It is intentionally unpleasant to reach for, and
`scripts/check-db-entry-point.sh` grows a second assertion: its call sites are counted, and
the count is committed, so adding one is a visible diff rather than a quiet habit. The
committed count starts from the sites 1.2 wrote.

### D3 — Resolving a user's organisations at login, without a privileged role

Login must answer "which organisations does this user belong to?" *before* any organisation
is known. That read cannot go through `InTx` (there is no org yet) and cannot go through
`InSystemTx` either, because `memberships` has RLS and `app_current_org()` would raise.

**`SECURITY DEFINER` alone does not solve this.** A definer function does not bypass RLS; it
runs *as its owner*, and only skips RLS if that owner could. Under D0's non-superuser
`vekst_migrator`, `FORCE ROW LEVEL SECURITY` subjects the owner to the policy too, so the
function re-enters `app_current_org()` and raises. Verified against Postgres 16 — the identical
function, called by the application role with no tenant context:

```
owner = plain (non-superuser) schema owner   →  ERROR: unrecognized configuration parameter "app.org_id"
owner = initdb superuser (today's CI)        →  2 rows returned
```

That is the CI-green/production-broken shape D0 exists to remove, and it lands on the only
path into the product.

**The mechanism is a role-scoped policy, not a privileged role.** RLS policies can be granted
`TO` a specific role. So the exception becomes a row in `pg_policies` — a catalog object the
coverage test can see, name and count — rather than an attribute of whoever happens to own a
function:

```sql
-- A role with no attributes at all: no LOGIN, no BYPASSRLS, no SUPERUSER.
CREATE ROLE vekst_membership_reader NOLOGIN NOSUPERUSER NOBYPASSRLS;
GRANT USAGE ON SCHEMA public TO vekst_membership_reader;
GRANT SELECT ON memberships  TO vekst_membership_reader;

-- The single deliberate exception to "RLS decides what is visible", scoped to
-- that role and to SELECT. FORCE ROW LEVEL SECURITY stays on memberships.
CREATE POLICY membership_reader ON memberships
    FOR SELECT TO vekst_membership_reader USING (true);

CREATE FUNCTION orgs_for_user(p_user_id uuid)
    RETURNS TABLE (org_id uuid, role text)
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public
    AS $fn$ SELECT m.org_id, m.role FROM memberships m WHERE m.user_id = p_user_id $fn$;
ALTER FUNCTION orgs_for_user(uuid) OWNER TO vekst_membership_reader;

REVOKE EXECUTE ON FUNCTION orgs_for_user(uuid) FROM PUBLIC;
GRANT  EXECUTE ON FUNCTION orgs_for_user(uuid) TO vekst_app;
```

Two provisioning details the migration must handle, both discovered by running it as a
non-superuser owner: reassigning ownership requires `SET` on the target role
(`GRANT vekst_membership_reader TO CURRENT_USER WITH SET TRUE` — Postgres 16 split `SET` from
`ADMIN`), and the incoming owner needs `CREATE` on the schema *at the moment of reassignment*,
so the migration grants it, reassigns, and revokes it again in the same transaction.

Verified end to end against Postgres 16, schema built by a non-superuser owner, `FORCE` intact:

| Probe | Result |
| --- | --- |
| `orgs_for_user(u)` with no tenant context | returns that user's memberships |
| `orgs_for_user(other_user)` | returns only that other user's rows |
| `SELECT … FROM memberships` with no context, as `vekst_app` | still raises `undefined_object` — fail-closed preserved |
| `SELECT … FROM memberships` with context = org A, as `vekst_app` | A's rows only |
| `SET ROLE vekst_membership_reader` as `vekst_app` | `ERROR: permission denied to set role` |

The last row is what bounds the exception. `vekst_membership_reader` cannot log in and
`vekst_app` cannot become it, so the role's entire reachable surface is the body of one
function — one `WHERE m.user_id = $1`, on one table, returning two columns. `CODEOWNERS`
covers the function, and D6's coverage test asserts that this is the only role-scoped
`USING (true)` policy and that `orgs_for_user` is the only `SECURITY DEFINER` function
`vekst_app` may execute.

Why not the obvious alternatives: giving `vekst_migrator` `BYPASSRLS` turns one narrow
exception into a blanket one and deletes the guarantee `FORCE` exists to provide; dropping
`FORCE` on `memberships` restores owner bypass invisibly, in `pg_class` rather than in
`pg_policies`, where nobody reads it. Both are in §4.

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

### D6 — The coverage test asserts isolation, not the existence of a policy

A Go test, run in CI against the migrated scratch database, reads
`deploy/db/rls-exempt-tables.txt` and checks every other table in `public`. A new tenant table
with no policy fails here on the pull request that adds it — which is the point, and why 0.2
seeded the allowlist before the test existed to read it.

"Has at least one row in `pg_policies`" is too weak to be that gate. All of these pass it and
none of them isolates anything:

```sql
CREATE POLICY p ON t FOR ALL USING (true);                        -- no isolation at all
CREATE POLICY p ON t FOR ALL USING (created_by = app_current_org());  -- wrong column
CREATE POLICY p ON t FOR SELECT USING (org_id = app_current_org());   -- writes silently denied
```

The test therefore asserts, per non-allowlisted table:

1. `relrowsecurity` **and** `relforcerowsecurity` in `pg_class`. Enabled-but-not-forced is a
   distinct failure with its own case, because it restores owner bypass silently.
2. At least one policy that applies to **all** roles (`polroles = '{0}'`) whose `USING`
   expression restricts a column of that table to `app_current_org()`. A `USING (true)`
   policy no longer counts, and neither does one naming a column the table does not have.
3. That column is `org_id` — **except on `organizations`, where it is `id`**, because the
   organisation *is* the tenant and carries no `org_id` of its own (§1.2). A test that simply
   required an `org_id` column would fail on the first table this change creates.
4. `polcmd` covers writes — a `FOR ALL` policy, or an insert/update policy alongside the read
   one — so a table cannot be left readable-but-unwritable by accident.
5. Where `polwithcheck` is present it equals `polqual`; where it is absent on a `FOR ALL`
   policy it is treated as equal, because Postgres derives it from `USING` (§1.2).

Every check above reads `polroles = '{0}'` — it judges the policies that apply to everyone.
D3's `membership_reader` is deliberately `USING (true)` and would fail check 2 if it were
weighed as a general policy; it is not, and the next paragraph is what bounds it instead.

Two inventory assertions run in the same test, because both are stated as guarantees elsewhere
and neither is otherwise proved:

- **Role-scoped exceptions are enumerated.** Any policy with a non-empty `polroles` is an
  exception to "RLS decides what is visible", and is exempt from the per-table checks above
  precisely because it is counted here instead. Exactly one is expected: `membership_reader`
  on `memberships` (D3). A second one fails the build, as does one granted to `vekst_app`.
- **`SECURITY DEFINER` functions are enumerated.** Exactly one `prosecdef` function in
  `public` may be `EXECUTE`-able by `vekst_app`: `orgs_for_user`. This is the assertion behind
  the spec's "it is the only such exception", which nothing else checks.

And two role assertions, which are what keep D0 true over time: `vekst_app` and
`vekst_migrator` must both have `rolsuper = false` and `rolbypassrls = false` in `pg_roles`.
These extend the assertion 0.2 already made about `vekst_app` alone, rather than adding a
second one beside it.

`deploy/db/rls-exempt-tables.txt` stays what 0.2 said it is — the record of which *tables* are
infrastructure rather than tenant data. The two inventories above are deliberately **not**
file-driven: an allowlist is the right shape for a list that grows, and the correct length of
both of these is one.

**The test must be able to fail.** Each of the shapes above gets a case that creates the bad
object in a scratch database, runs the checker, asserts it reports that object, and drops it.
A green test that cannot go red proves nothing — and the likeliest real failure is a policy
that exists and does not isolate, not a table with no policy at all.

### D7 — `OrgID` is unforgeable, not merely unforgettable

D2 makes the organisation impossible to *omit*. Nothing yet makes it impossible to *invent*.
With `type OrgID uuid.UUID`, this compiles, from any package:

```go
org := db.OrgID(uuid.MustParse(req.Msg.GetOrgId()))   // straight off the wire
return d.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error { … })
```

RLS then does its job perfectly and isolates the transaction to the organisation *the caller
named*. The database has no way to know the caller was never a member of it.

This is the distinction the change has to be precise about: **RLS is containment, not
authorization.** It guarantees "a transaction bound to org X touches only X's rows". It cannot
guarantee "this request was entitled to bind to X". The proposal is right that no
tenant-reading RPC exists yet — and that is exactly why the seam is cheap to close now, since
*"every later change inherits the `InTx` signature."*

```
  session cookie ──▶ user_id ──▶ orgs_for_user ──▶ { orgs this user may act as }
                                                              │
                                                              │  D7 makes this the ONLY
                                                              │  arrow that produces an OrgID
                                                              ▼
  req.Msg.OrgId ─────────────────────────────────────▶ InTx(ctx, org, fn) ──▶ RLS
```

#### The options considered

| Approach | How it stops a forged org | Cost |
| --- | --- | --- |
| Convention: "handlers must check membership first" | Nothing enforces it. Identical to the application-level `WHERE org_id` this change exists to reject | none, and worth nothing |
| Middleware resolves the org and puts it in `context.Context` | Real, but back to a value that can be absent at run time — D2 already rejected this shape | reintroduces the failure D2 removed |
| Check membership *inside* `InTx` | Airtight, but `InTx` would need a user for every call, and workers and org-creation have none. It also puts an authorization decision inside the transaction primitive | conflates two jobs |
| Restrictive RLS policy joining `memberships` on a second `app.user_id` GUC | Enforced by the database, which is the strongest place. But it makes every tenant query carry a subquery against `memberships`, and workers have no user at all | performance and a worker story |
| **Unexported field + named constructors** *(chosen)* | A value of the type cannot be built outside the package. Every way to obtain one is a named function, so the ways can be enumerated, reviewed and counted | one type change, three constructors |

#### The shape

```go
// core/internal/db

// OrgID is a tenant identifier that has been established, not asserted. The
// field is unexported, so no package outside db can construct one from a raw
// UUID -- every value came through one of the constructors below, and those
// are the complete list of ways a request can come to act for a tenant.
type OrgID struct{ v uuid.UUID }

// The three doors, each named for its provenance:

// OrgIDForSession resolves the organisation an authenticated caller asked to
// act for, through orgs_for_user (D3). It returns the same error whether the
// organisation does not exist or the user is simply not a member -- the
// distinction is the oracle D1 spends a section closing.
//
// It is a method on *DB because it has to reach the database before any org is
// known: it opens InSystemTx and calls orgs_for_user. That makes it the eighth
// InSystemTx call site -- see the note below.
func (d *DB) OrgIDForSession(ctx context.Context, userID, requested uuid.UUID) (OrgID, Role, error)

// OrgIDFromJobArgs is the worker door (D5). River's tables carry no tenant
// column, so a job's arguments are the only place its tenant can come from.
//
// TenantJobArgs is declared here, in db, not in jobs: jobs already depends on
// db and the dependency runs one way. Workers embed it in their own argument
// structs rather than db learning anything about River.
type TenantJobArgs struct{ OrgID uuid.UUID }

func OrgIDFromJobArgs(args TenantJobArgs) (OrgID, error)

// OrgIDForNewOrg mints the identifier for an organisation that does not exist
// yet, for the one insert in D4 that creates it.
func OrgIDForNewOrg() OrgID
```

Three properties follow from the unexported field, and none needs a reviewer to remember
anything:

- **The zero value is inert.** `var org db.OrgID` holds the nil UUID, which matches no row and
  satisfies no policy, so a forgotten assignment fails closed rather than reading tenant zero.
  `InTx` rejects it explicitly anyway, with an error naming the constructor to use.
- **The doors are countable.** `scripts/check-db-entry-point.sh` already commits a call-site
  count for `InSystemTx`; it gains the same treatment for `OrgIDFromJobArgs` and
  `OrgIDForNewOrg`. `OrgIDForSession` is left uncounted — it is the door that is *supposed* to
  be used, and counting it would only punish normal work.

  **This change moves the `InSystemTx` count from 7 to 8**, and `OrgIDForSession` is the eighth
  — the one untenanted read this change legitimately adds, for the lookup that by definition
  runs before an organisation is known. Recording it as a deliberate increment is exactly what
  the committed count is for; leaving the number at 7 and discovering the mismatch during
  implementation is not.
- **Tests cannot quietly become the fourth door.** The test helper takes a `testing.TB`:
  `func OrgIDForTest(tb testing.TB, id uuid.UUID) OrgID`. Non-test code cannot call it without
  importing `testing`, which is both obvious in review and greppable in CI.

`OrgIDForSession` returns the `Role` alongside, because the membership row is already being
read and a second lookup at enforcement time is how a stale-permissions bug is built. The role
is **stored and returned, still never checked** — that remains Product's, per the proposal's
non-goals. Nothing caches the membership: it is one index scan on `memberships (user_id)`, and
caching it is how a revoked user keeps their access until a session expires.

### D8 — How a migration writes to a `FORCE`d tenant table

D0 has a consequence that will land on change 2.x, not on this one, and it is better written
down now than discovered then. Under a non-superuser owner, `FORCE ROW LEVEL SECURITY` applies
to migrations too. Verified against Postgres 16:

```
UPDATE organizations SET …            as the plain owner, no tenant context
  →  ERROR: unrecognized configuration parameter "app.org_id"
```

So the first migration that backfills a tenant column fails, and — because CI today runs as a
superuser — it fails *after* merge. D0 fixes the "after merge" half. This decision fixes the
other half by naming the idiom:

```sql
-- +goose StatementBegin
ALTER TABLE t NO FORCE ROW LEVEL SECURITY;
UPDATE t SET … ;                          -- the backfill, unfiltered, as the owner
ALTER TABLE t FORCE ROW LEVEL SECURITY;   -- restored in the same transaction
-- +goose StatementEnd
```

Confirmed to work as a plain owner, with no superuser and no `BYPASSRLS`. Three properties
make it acceptable: the window is one transaction, the bypass is spelled out in the migration
diff where a reviewer sees it, and forgetting the restore is not silent — it lands in
`pg_class.relforcerowsecurity`, which is exactly what D6 asserts, so CI fails on the same pull
request. The rejected alternatives are a `BYPASSRLS` migrator (§4) and `SET row_security = off`,
which is not a bypass at all — Postgres raises rather than dropping the policy.

### D9 — A write that matches no row is an error, not a no-op

RLS filters rows; it does not report having filtered them. An `UPDATE` or `DELETE` whose target
is invisible under the current policy affects **zero rows and raises nothing**. `INSERT` is the
loud case (`WITH CHECK` violations raise); update and delete are the quiet ones.

For this product the quiet failure is the dangerous one. "The correction was saved" when it was
not is the same class of bug as a report showing another company's figures: the system reports
success and the number is wrong. It also interacts directly with the append-only rule in
`CLAUDE.md` — superseding a classification is an `UPDATE` of the superseded row, and a silent
no-op there leaves two live classifications for one transaction.

Convention, enforced by review and by the repository layer rather than by a new mechanism:
every generated write whose intent is "exactly one row" checks the affected-row count and
returns an error when it is zero. `sqlc`'s `:execrows` returns it; the wrapper treats `0` as a
distinct, named error rather than success. Recorded in `CLAUDE.md` alongside the other
invariants.

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
| **Plain single-column foreign keys, or an unscoped `UNIQUE`** | Cheaper to write, and both are cross-tenant oracles because referential-integrity checks bypass RLS. See D1. |
| **A `SECURITY DEFINER` function for every tenant read** | Solves D3 generally, and puts the isolation decision back in hand-written function bodies — the exact thing RLS exists to remove. One narrow exception is reviewable; a pattern is not. |
| **`app.user_id` as a second GUC, with a self-membership policy** | Would let `memberships` be read as "your own rows, or your current org's". But permissive policies are `OR`-combined into one qual with no evaluation-order guarantee, so `app_current_org()` is still reached and still raises in the login transaction. Making it fail-soft to avoid that trades a loud error for silent empty results everywhere else. D3's role-scoped policy gets the same result without touching `app_current_org()`. |
| **`vekst_migrator` as a superuser** | The status quo, and the reason D3 appeared to work. It makes `FORCE ROW LEVEL SECURITY` decorative, hands every function it owns an unbounded implicit bypass, and makes CI prove nothing about a managed Postgres. Full comparison in D0. |
| **`BYPASSRLS` on `vekst_migrator`, or a dedicated `BYPASSRLS` role** | A common published pattern, and it would solve D3 and D8 in one line. Rejected because only a superuser can create a `BYPASSRLS` role, which reintroduces D0's provisioning dependency in every environment; and because the exception then lives in a role attribute rather than in `pg_policies`, where D6 can count it. |
| **Dropping `FORCE` on `memberships` so the owner can read it** | Also solves D3, with no new role. Rejected because it restores owner bypass for *every* access path, not just the one function, and it records that in `pg_class` — where nothing reads it — instead of in `pg_policies`, where the coverage test does. |
| **`SET row_security = off` for migration backfills** | Not a bypass. Postgres raises `query would be affected by row-level security policy` rather than dropping the policy, unless the role could bypass anyway. See D8. |
| **Membership checked inside `InTx`** | Airtight, but every call would need a user, which workers and organisation-creation do not have, and it puts an authorization decision inside the transaction primitive. D7 keeps the two jobs separate. |
| **`type OrgID uuid.UUID` (a plain newtype)** | Convertible from any UUID in any package, so the tenant a request acts for can be taken straight off the wire. Unforgettable but forgeable. See D7. |

## 5. Risks

| Risk | Handling |
| --- | --- |
| A pooled connection carries `app.org_id` into the next request | `set_config(…, true)` is transaction-local. Tested directly: two sequential transactions on the same connection, the second with no context, must raise rather than return the first tenant's rows. |
| A future migration hands a tenant table to a new owner | `FORCE ROW LEVEL SECURITY` on every table; the coverage test asserts `relforcerowsecurity`, not only `relrowsecurity`. |
| `orgs_for_user` grows a second use | It is in `CODEOWNERS`; a change to it needs both reviewers. |
| Someone adds a tenant table with no policy | The coverage test fails the pull request. |
| `InSystemTx` becomes the easy way out | Its call sites are counted and the count is committed. |
| **CI stops resembling production again** — someone simplifies a compose file back to an initdb superuser, and `FORCE` quietly stops meaning anything | D0's role assertion: a test reads `pg_roles` and fails if `vekst_app` or `vekst_migrator` has `rolsuper` or `rolbypassrls`. This is the single highest-value test in the change, because it is the one that keeps every other test honest. |
| `vekst_membership_reader` acquires a second policy, a second function, or `LOGIN` | D6 enumerates role-scoped policies and `SECURITY DEFINER` functions and asserts there is exactly one of each; the role is in `CODEOWNERS`. |
| A handler binds a transaction to an organisation the caller does not belong to | D7: `OrgID` cannot be constructed outside `core/internal/db`, and `OrgIDForSession` is the only door that starts from a session. |
| A migration's `NO FORCE` window (D8) is left open | `relforcerowsecurity` is asserted by D6 on the same pull request. |
| `vekst_app` re-sets `app.org_id` mid-transaction and escapes its own tenant | A custom GUC in a user-defined namespace cannot be locked down with `GRANT … ON PARAMETER`, so there is no database-side guard. The application-side one is that every statement is generated by `sqlc` from checked-in queries — there is no dynamic SQL for a value to reach — and `InTx` is the only place that calls `set_config`. Worth restating in review whenever raw SQL is proposed. |
| An `UPDATE` filtered out by policy reports success | D9: writes intended to affect one row check the affected count and error on zero. |

## 6. Call-outs

- **Tenant isolation:** the whole change. Cross-tenant scenarios are mandatory on every task
  that adds a table.
- **Money, currency, `source_kind`, the classifier contract:** no money column appears here.
  `accounts.currency` and `organizations.base_currency` are ISO-4217 codes, not amounts, and
  they now carry a shape `CHECK`. §1 records why `base_currency` sits on the organisation
  alone and why binding it to the checked-in ISO-4217 list is deferred to 2.5 rather than
  left unstated.
- **Privilege model:** D0 changes how `vekst_migrator` is provisioned in CI and in the local
  overlay. That is a `deploy/` and CI change inside a change that is otherwise schema and Go,
  and it is load-bearing for D3, D6 and D8 — review it as part of the isolation story, not as
  housekeeping.
