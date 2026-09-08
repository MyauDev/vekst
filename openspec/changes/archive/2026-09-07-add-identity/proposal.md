## Why

Every change from 1.1 onward asks which organisation a request is for, and that answer
starts with which person is asking. `core` today serves every request as an anonymous
public caller.

It also unblocks tenancy directly: `ARCHITECTURE.md` §5.5 makes `users` the one global,
non-tenant table, and `memberships(org_id, user_id, role)` references it. Change 1.1 cannot
create `memberships` until `users` exists. Identity is a dependency, not a preference.

`ARCHITECTURE.md` has no section on authentication, so the decisions are recorded in this
change's design rather than inherited.

Milestone: **Demo**. Capability: **`identity-access`** (new), extending
**`platform-foundation`**.

## What Changes

- Migration 003 creates four global tables: `users`, `user_identities`, `sessions` and
  `auth_flows`. None is tenant data; all four join `deploy/db/rls-exempt-tables.txt`.
- **Google as an OIDC provider**, authorisation-code flow with PKCE, on plain `chi` HTTP
  endpoints — `/auth/google/start`, `/auth/google/callback`, `/auth/logout`. A redirect
  flow cannot be a Connect RPC: the one deliberate exception to "the browser talks to core
  over Connect".
- **Server-side sessions.** The cookie carries an opaque random token; the database stores
  only its SHA-256. A session can be revoked, which a signed token cannot.
- **An identity is `(provider, subject)`**, never an email. Google's `sub` is stable, an
  email is not, and matching on one is how accounts get taken over. The binding is
  insert-once and `vekst_app` holds no `UPDATE` on it, so it cannot be repointed at another
  person.
- **The account record is provisioned once** and never written by a later token claim. That
  is what lets one person hold several sign-in methods without them overwriting each other,
  and it keeps a `UNIQUE (email)` violation off the login path entirely.
- **`db.InTx` is renamed to `db.InSystemTx`** while it still has no call sites, so 1.1 adds
  the tenant-aware entry point beside it rather than renaming underneath this change.
- **Auth middleware** resolving the cookie to a user; `/healthz` and `/readyz` stay open.
- One Connect RPC, `IdentityService/GetCurrentUser`, returning the signed-in user and
  deliberately **no** organisation — none exists until 1.1.
- A River job expiring old sessions and abandoned login flows.
- The local overlay's host changes from `vekst.localhost` to `localhost`, because Google
  rejects redirect URIs on a subdomain of localhost.

## Capabilities

### New Capabilities
- `identity-access`: who a caller is, and how that survives between requests.

### Modified Capabilities
- `platform-foundation`: adds authenticated routes; health and readiness stay open.

## Non-goals

- **Organisations, memberships and roles** — change 1.1. After this change a signed-in
  user belongs to nothing and can reach no data. That is the correct end state here.
- **Role enforcement.** `owner`/`admin`/`approver`/`viewer` arrive with 1.1 and Product.
- **Magic-link sign-in** — Product. The `user_identities` shape means it needs no change to
  `users`, no data migration, and no change to how existing accounts resolve; linking it to an
  existing account is one `INSERT`. It does still need a `provider` CHECK edit and its own
  token storage (design D3).
- **Signup, invitations, account linking, admin user management.** You create the pilot
  users.
- **A deployed environment.** The redirect-URI list grows when a host exists.

## Impact

Touches `/core/migrations`, `/core/internal/db`, `/core/internal/server`,
`/core/internal/jobs`, `/proto/vekst/v1`, `/web/src`, `/deploy/k8s/overlays/local`,
`/scripts`, CI and the RLS allowlist.
**A Google Cloud OAuth client is a prerequisite:** without a client id, a secret and a
registered redirect URI, none of this can be run or tested.
