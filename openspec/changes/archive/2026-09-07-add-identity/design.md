# Design — add-identity

**This change touches tenant isolation indirectly and account security directly.** It
creates the first four tables that are deliberately *not* tenant-scoped, and it decides how
a person's session survives between requests. `ARCHITECTURE.md` records nothing about
authentication, so every decision here is made in this document rather than inherited.

## 1. Data model

Migration `00003_identity.sql`, applied as `vekst_migrator`. Change 1.1's tenancy migration
becomes `00004`.

```sql
-- +goose Up

CREATE TABLE users (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email      text,                          -- nullable; provisioned once, see D3a
    name       text,
    locale     text        NOT NULL DEFAULT 'en' CHECK (locale IN ('en','ru')),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_email_lowercase CHECK (email IS NULL OR email = lower(email)),
    UNIQUE (email)                            -- Postgres allows many NULLs; one owner per address
);

CREATE TABLE user_identities (
    provider   text        NOT NULL CHECK (provider IN ('google')),
    subject    text        NOT NULL,          -- the OIDC `sub` claim
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, subject)
);
CREATE INDEX user_identities_user_idx ON user_identities (user_id);

-- The identity row is a pure binding and has no mutable column: no email, no
-- name, no last-login. 00001's ALTER DEFAULT PRIVILEGES grants vekst_app
-- UPDATE on every new table, so it has to be taken back here explicitly.
-- Without this, one `ON CONFLICT DO UPDATE SET user_id` repoints a provider
-- subject at a different person -- full account takeover, by a write that
-- looks entirely legitimate and that no RLS policy would have caught. See D3b.
REVOKE UPDATE ON user_identities FROM vekst_app;

CREATE TABLE sessions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token_sha256 bytea       NOT NULL UNIQUE,  -- the cookie value is never stored; see D2
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz,
    user_agent   text
);
CREATE INDEX sessions_user_idx ON sessions (user_id) WHERE revoked_at IS NULL;
CREATE INDEX sessions_expiry_idx ON sessions (expires_at);

CREATE TABLE auth_flows (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    state         text        NOT NULL UNIQUE, -- echoed by Google; the callback's lookup key
    nonce         text        NOT NULL,       -- replay guard, checked in the ID token
    code_verifier text        NOT NULL,       -- PKCE
    created_at    timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL
);
CREATE INDEX auth_flows_expiry_idx ON auth_flows (expires_at);
```

### 1.1 None of these is a tenant table

All four join `deploy/db/rls-exempt-tables.txt`, which is a both-reviewers change per
`CODEOWNERS`. The reasoning, recorded there:

| Table | Why it holds no tenant data |
| --- | --- |
| `users` | Global by design — `ARCHITECTURE.md` §5.5. A person may belong to several organisations; access is mediated by `memberships`, which 1.1 adds. |
| `user_identities` | A person's provider accounts. Same scope as `users`. |
| `sessions` | A session belongs to a person, not to an organisation. Tenant context is chosen per request, after authentication. |
| `auth_flows` | Exists before anyone is authenticated. It cannot have an owner. |

**The honest limit:** RLS is not protecting these tables, so nothing at the database level
stops a wrong query from returning another person's session or email. Note what the exposure
actually is — it is not cross-*tenant*, because there is no `org_id` here to forget. It is
cross-*person*: an unfiltered `SELECT … FROM users` returns every user of every customer,
which in a product whose customers are named companies discloses who those companies are and
who their finance staff are, without a single figure moving.

"State it in the package comment and hold to it" is not enough on its own — `CLAUDE.md` asks
for a failing test before a rule in a document, and every other invariant in this system has
a mechanism behind it. Three layers, weakest last:

1. **Privilege.** `REVOKE UPDATE ON user_identities` (§1) makes the account-takeover write
   impossible rather than merely reviewed. This is the same move as `FORCE ROW LEVEL
   SECURITY`, applied where RLS cannot reach.
2. **A narrowness check**, in the spirit of `scripts/check-db-entry-point.sh`: no `.sql` file
   outside the identity query file may name these four tables, and no statement inside it may
   join to a table outside the four. The file is roughly ten statements, so this is cheap, and
   like the coverage test it gets a test-the-test proving it can go red.
3. **The package comment**, which now records the reasoning rather than carrying the whole
   burden of it.

**Reading `users` in a tenant context.** The rule above forbids the join, so the pattern is
two steps: read `memberships` under RLS to get user ids, then fetch those users by key. Worth
stating explicitly, because the natural instinct is to write
`JOIN memberships m ON m.user_id = u.id` — which is in fact *safe*, since `memberships` is
RLS-protected and re-imposes tenancy on the join. Either discipline works. What does not work
is a prohibition with no stated alternative, so this change picks the two-step and says so.
The dangerous query is the one with no membership predicate at all, and neither RLS nor this
rule would catch it — only the narrowness check does.

0.2's existing CI step already asserts that the tables present after migration exactly match
the allowlist, so a fifth table added here without a line fails on the pull request that adds
it. The stronger coverage test — RLS enabled, forced, and a policy on everything *outside*
the allowlist — arrives with 1.1, and there is nothing outside the allowlist until then.

## 2. Decisions

### D1 — The login flow is HTTP, not Connect

OIDC is a browser redirect flow. A Connect RPC cannot answer with a 302, and the callback
arrives as a plain browser navigation carrying query parameters. Three `chi` routes,
alongside `/healthz` and `/readyz`:

| Route | Does |
| --- | --- |
| `GET /auth/google/start` | Creates an `auth_flows` row, sets a short-lived flow cookie holding its `id`, redirects to Google's authorisation endpoint with `state`, `nonce` and a PKCE `code_challenge` |
| `GET /auth/google/callback` | Finds the flow by `state`, requires the flow cookie to name that same row, deletes it, exchanges the code with PKCE, validates the ID token, resolves or provisions the identity, creates a session, sets the session cookie, redirects to `/` |
| `POST /auth/logout` | Marks the session revoked and clears the cookie |

This is the single exception to `ARCHITECTURE.md`'s "browser ↔ core is ConnectRPC", and it
is an exception about transport, not about contract: no application data crosses these
routes. Everything the application reads goes through Connect, as before.

**The flow cookie holds the `auth_flows` row's `id`; `state` travels in the URL.** The
callback checks both halves: `state` from the query string must match a live, unconsumed row,
and that row's `id` must equal the cookie's value. `state` alone proves only that *someone*
started a flow here; the cookie is what proves the callback reached the same browser that
started this one. Splitting the two across the two channels means a leaked callback URL —
referrer, history, a shoulder-surfed address bar — carries only half of what is needed.

The alternative, putting `state` in the cookie as a double-submit, is weaker for exactly that
reason: the same value would appear in both places, so one leak yields both halves.

`auth_flows.state` is `UNIQUE` because it is a lookup key on a security check. Two rows
sharing a value would make "the flow row" ambiguous at the worst possible moment, and the
lookup would otherwise be a sequential scan on every sign-in.

Consuming a flow **deletes** the row, so a replayed callback and a callback with an invented
`state` are indistinguishable — deliberately, since the caller learns nothing from the
difference. The flow cookie is cleared in the same response.

The ID token is validated properly, not decoded: signature against Google's JWKS,
`iss`, `aud` equal to the client id, `exp`, and `nonce` equal to the flow's. A library does
this; hand-rolling it is how the check silently becomes `base64 decode`.

### D2 — Sessions are server-side, and the token is stored hashed

The cookie holds 32 bytes of `crypto/rand`, base64url. The database stores only its
SHA-256. A read of the `sessions` table — a backup, a log line, a query gone wrong — then
yields nothing a person can log in with. Lookup is by hash, which is a unique-index probe,
so nothing is lost in speed.

Cookie attributes: `HttpOnly`, `SameSite=Lax`, `Secure`, `Path=/`, no `Domain`. `Secure`
stays on locally: current browsers treat `http://localhost` as a secure context. [Likely —
verify once in the browser you develop in; it is a one-line configuration if not.]

Both cookies are `SameSite=Lax` rather than `Strict`, but for different reasons, and it is
worth keeping them apart:

- The **flow** cookie must survive the callback, which is a top-level GET navigation from
  Google. `Strict` would withhold it on arrival and every sign-in would fail. This is the
  cookie D1's redirect chain depends on — the session cookie does not exist yet at that point.
- The **session** cookie is never sent on the callback, so that argument does not apply to it.
  It wants `Lax` so that a person following an external link into the app is not presented as
  signed out, which `Strict` would do on that first navigation.

Revocation is the whole reason for the table: logout, a lost laptop, an offboarded external
accountant holding the `viewer` role. A signed stateless token cannot offer it.

### D3 — An identity is `(provider, subject)`; an email is never an identifier

Google's `sub` claim is stable for the life of the account. The email is not — it changes,
and it can be reassigned within a Workspace domain. Matching a login to an account by email
is the standard account-takeover path.

This is not a local opinion. OpenID Connect states that an issuer may reuse an email value
across different end users over time, so `email` and similar claims must not be used as
unique identifiers; the `(iss, sub)` pair is the only combination guaranteed to identify an
end user. Keycloak links accounts by email *by default*, and carries a known defect where
`FEDERATED_IDENTITY` lacks a uniqueness constraint and one federated id linked to two users
breaks sign-in outright. Both are reasons the constraint here is a real database constraint
rather than an application check.

`users.email` is therefore descriptive: stored lowercased, unique so two people cannot both
claim one address, and never used to resolve a login. An `email_verified` claim that is false
is refused outright.

**`provider` names an issuer instance, not a protocol.** For Google the two collapse, because
`iss` is always `https://accounts.google.com`. They do not collapse for a multi-tenant Entra
or for SAML, where a NameID is unique only within the issuing IdP — `provider = 'saml'` would
collide across two customers' IdPs. When such a provider arrives, either the values become
issuer-scoped (`saml:acme-corp`) or an `issuer` column joins the natural key. Recorded here so
that migration is a decision rather than a surprise.

**What magic-link actually costs.** The identities table means `add-magic-link-auth` needs no
change to `users`, no data migration, and no change to how existing accounts resolve — and
linking a magic-link identity to an existing user is one `INSERT`, a flow rather than a
schema. It does still need two schema changes: a `'magic_link'` value admitted by the
`provider` CHECK, and somewhere to keep link tokens, since `auth_flows` is OIDC-shaped
(`nonce`, `code_verifier`) and a magic link needs a token hash and an address instead. Its
`subject` is the email address, because for that provider mailbox control *is* the proof —
the one place where "an email is never an identifier" is legitimately conditional.

### D3a — `users` is provisioned once and never overwritten by a claim

`users` holds app-owned facts. It is written once, at provisioning, from the claims of the
identity that creates it, and **no later sign-in updates it**. It changes afterwards only by a
deliberate act of the person or an operator. `email`, `name` and `locale` all follow this rule
uniformly.

This is the decision that lets one person hold several login methods without them fighting.
The contention was never about which table the email lived in — it was last-write-wins on
sign-in. Two providers reporting different addresses only contend if a sign-in is allowed to
write. Forbid the write and a single column is sufficient, which is why this change keeps
`users.email` rather than moving the address onto the identity or adding a third table for it.

The sign-in path is therefore:

```
identity found by (provider, subject)  →  use that user_id, write nothing to users
identity not found                     →  provision: INSERT users, then INSERT user_identities
```

Three consequences, each deliberate:

- **A provider email that changes goes stale.** The person stays signed in and nothing
  collides, because nothing is written. A stale address is cosmetic until notifications exist;
  correcting it is a deliberate change flow, and that flow is Product's, not this change's. At
  Demo scale, with pilot users created by hand, an operator edit is the interim answer.
- **The `UNIQUE (email)` violation can only happen on the provisioning path**, where the
  refusal costs an account that never existed. It can no longer strike a returning user, which
  would have locked a valid person out of a working account over another account's data, with
  no self-service recovery.
- **The refusal is a feature, not a dead end.** When a Google user later tries magic-link on
  the same address, provisioning refuses with `email_taken` — which is precisely the signal a
  linking flow needs. The alternative, dropping the constraint, silently creates a second
  account with no memberships and nothing to point a link at.

`locale` arrives as a BCP-47 tag (`en-GB`, `ru-RU`, `nb-NO`) and is normalised to the
supported set before insert. Passing the claim through unnormalised fails the CHECK on an
ordinary sign-in.

### D3b — The identity binding is insert-once, and the database enforces it

`user_identities` has no mutable column by construction — no email, no name, no last-login.
The write on the provisioning path is a plain `INSERT`; the returning path does not write at
all. "Upsert" is the wrong word for it, and the wrong word is dangerous here: one
`ON CONFLICT (provider, subject) DO UPDATE SET user_id = excluded.user_id` repoints a provider
subject at a different person, which after 1.1 hands the attacker every organisation that
person belongs to. No RLS policy would catch it — it is a legitimate-looking write by
`vekst_app` — so `REVOKE UPDATE` in §1 makes it impossible instead.

The `UNIQUE (email)` check on provisioning is likewise the constraint itself, not a `SELECT`
before the `INSERT`: two concurrent first sign-ins on one address would both pass a read. The
`INSERT` runs, and `23505` on `users_email_key` is translated to the `email_taken` code.

### D4 — Session lookup runs untenanted, through `InSystemTx`

The middleware resolves a cookie before any organisation is known, so it cannot use a
tenant-aware entry point. **This change performs the rename**: today's `db.InTx` becomes
`db.InSystemTx`, and 1.1 adds the tenant-aware `InTx(ctx, org, fn)` beside it rather than
renaming anything.

The rename belongs here because `InTx` has **zero non-test call sites today** — only doc
comments mention it. Doing it now costs nothing; deferring it to 1.1 costs every call site
this change is about to write, and leaves identity's code carrying a name that will change
underneath it. Identity's queries are written against the name they keep permanently, and the
change boundary becomes purely additive.

The identity package is the first and, until 1.1, the only `InSystemTx` caller. Its call-site
count is what 1.1's committed expected count (its task 3.3) starts from — the coupling is
recorded here because whoever implements this change has no reason to read 1.1's design.

**Forward note.** This change reads no tenant table, but 1.1 makes identity the holder of the
one deliberate `SECURITY DEFINER` exception in the system. Login must answer "which
organisations does this user belong to?" before any organisation is known, which can use
neither `InTx` (no org yet) nor `InSystemTx` (`memberships` has RLS, and `app_current_org()`
is fail-closed, so it raises). 1.1's `orgs_for_user` exists for this package's benefit.

### D5 — Expiry is a River job, not a cron

0.2 built River and proved it with a no-op. A periodic job deletes `auth_flows` rows past
`expires_at` and `sessions` rows past `expires_at` plus a retention window. It is the first
real job in the system, and it needs no tenant argument — these tables have no tenant.

### D6a — Google credentials do not follow the database-credential pattern

`deploy/k8s/overlays/local/db-secrets.yaml` is committed, because those credentials are
throwaway values for a Postgres that exists only on a laptop. The Google client secret is
not throwaway: it is a real bearer credential for a real Google client, and `.gitignore`
already refuses `/deploy/k8s/**/*.secret.yaml` and `.env*` for exactly this reason.

But CI's `manifests` job runs `kubectl kustomize` on every overlay, so an overlay that
references a git-ignored file fails to render. The resolution keeps both properties:

- `deploy/k8s/overlays/local/google-oidc.yaml` is **committed with empty values** and is what
  the overlay references. `kubectl kustomize` renders, CI stays green, and `gitleaks` finds
  nothing because there is nothing to find.

  Empty rather than a `REPLACE_ME` sentinel, which is what this document first said. That
  Secret really is deployed to the local cluster, so its committed contents are what `core`
  reads when nobody has configured sign-in — and a sentinel there would trip the refuse-to-start
  rule below and hand a developer with no OAuth client a crash loop, contradicting the next
  bullet. Empty is a supported state: `core` serves, and only the auth routes fail. The
  sentinel keeps its meaning where it actually indicates a mistake — inside a
  `google-oidc.secret.yaml` somebody created and left unfilled. *(Found while implementing:
  both branches of the Tiltfile were exercised and the absent-credentials branch deployed the
  sentinel.)*
- The real values live in a git-ignored `deploy/k8s/overlays/local/google-oidc.secret.yaml`,
  applied by Tilt when it is present and skipped when it is not. A developer without
  credentials still gets a working stack; only the sign-in route fails, and it fails with a
  clear configuration error rather than a nil dereference.
- `core` refuses to start if the client id or secret is the `REPLACE_ME` sentinel. Such a
  value can only come from a secret file somebody wrote and did not fill in, and it is worse
  than a missing one: sign-in would fail at Google with an opaque error, far from its cause.
  Absent credentials are different, and are allowed.

### D6 — The local host becomes `localhost`

Google rejects a redirect URI whose host is a subdomain of `localhost`: the console requires
a public-suffix top-level domain, and exempts only `localhost` itself and localhost IPs. The
local overlay serves `vekst.localhost`, so no local redirect URI can be registered.

The overlay's host becomes `localhost`, which keeps the whole property the base ingress was
built for: the document and `/rpc` and `/auth` share one origin, so the session cookie is
first-party and no CORS header is needed. The Tiltfile's printed URL and the README change
with it. This is a configuration change in the overlay, not a change in the set of
workloads.

## 3. API

One new browser-facing Connect service, in `proto/vekst/v1/identity.proto`:

```proto
syntax = "proto3";
package vekst.v1;

service IdentityService {
  // Returns the signed-in user, or the unauthenticated code.
  rpc GetCurrentUser(GetCurrentUserRequest) returns (GetCurrentUserResponse);
}

message User {
  string id     = 1;
  string email  = 2;
  string name   = 3;
  string locale = 4;
}

message GetCurrentUserRequest {}
message GetCurrentUserResponse { User user = 1; }
```

`User` carries **no** organisation and no role. There is no organisation model until 1.1,
and a field that exists before it means anything is a field the front end will start
trusting. 1.1 extends this message; `buf breaking` protects the addition.

No internal `core` → `classifier` gRPC changes. The classifier learns nothing about people.

Sign-out is `POST /auth/logout` rather than an RPC, because it must clear a cookie in the
same response that ends the session, and it belongs with the other two auth routes.

## 4. Rejected alternatives

| Rejected | Why |
| --- | --- |
| **Stateless signed JWT in a cookie** | No table and no read per request, but no revocation either: a leaked token stays valid until it expires, and "sign out everywhere" cannot be built. Unacceptable for an external accountant holding a `viewer` role. |
| **Short JWT plus refresh-token rows** | Fewer database reads, revocation within one token lifetime. Rejected for now on moving parts: rotation and reuse detection are easy to get subtly wrong, and the read this avoids is a unique-index probe. Reconsider if it ever measures. |
| **Storing the session token itself** | One `SELECT` — or one backup — then yields working credentials. Hashing costs nothing. |
| **`users.google_sub` instead of `user_identities`** | Forces a migration when Product adds magic-link sign-in, and invites resolving logins by email when a second provider appears. |
| **Matching a login to an account by email** | Google emails change and can be reassigned. This is the standard account-takeover path. |
| **Updating `users.email` from the claim on every sign-in** | Last-write-wins is what makes two providers contend over one column, and it puts a `UNIQUE` violation on the login path, where it locks a valid person out of a working account over another account's data. See D3a. |
| **Moving `email` onto `user_identities`** | Removes the contention, but also removes the only home the "one address, one account" rule has, leaving the invariant with no mechanism. The contention is caused by the write policy, not the column's location — fix the policy and one column suffices. |
| **A third `user_emails` table (address namespace, `primary` flag, unique-where-verified)** | Where django-allauth, Discourse and GitLab all end up, and where this will end up too. It buys multiple addresses per person, a primary among them, and a home for the uniqueness rule. Vekst needs none of the first two yet, and `users.email UNIQUE` already provides the third. Keeping that constraint now is also what makes the eventual backfill conflict-free: one row per user, no collisions possible. |
| **Full RLS on the identity tables via an `app.user_id` GUC** | Structurally impossible, not merely costly: the middleware resolves a session *before* it knows the user, and `auth_flows` exists before anyone is authenticated at all, so neither table can be user-scoped. 1.1 rejects a second GUC on separate grounds. |
| **A separate `identity` schema, outside `search_path`** | Genuinely stronger against accidental joins — a cross-schema reference needs an explicit prefix and is visible in review. Rejected because it removes these tables from 1.1's "every table in `public`" coverage scan, trading the simplicity of the test that proves the most for a narrower guarantee. |
| **`state`, `nonce` and the PKCE verifier in a signed cookie** | Avoids the `auth_flows` table, but introduces a signing key the system does not otherwise need, and nothing expires on the server. The table reuses River, which already exists. |
| **Login as a Connect RPC** | Cannot issue a 302, and the callback is a browser navigation, not an RPC call. |
| **Keeping `vekst.localhost` and registering it with Google** | Google refuses it. See D6. |
| **Self-service signup in this change** | The Demo has two pilot customers whose users you create. Signup without organisations to join is a screen that leads nowhere. |

## 5. Risks

| Risk | Handling |
| --- | --- |
| The ID token is decoded rather than verified | Use a maintained OIDC library; a test asserts a token with a bad signature, a wrong `aud` and a wrong `nonce` is each refused. |
| Session fixation | The session is created only after a successful token exchange, with a fresh random token. No session exists before login. |
| An open redirect through the post-login destination | The callback redirects to a fixed path. No caller-supplied return URL in this change. |
| The **flow** cookie is not sent on the callback, so every sign-in fails | `SameSite=Lax` permits it on a top-level GET navigation, where `Strict` would withhold it; a test drives the full redirect chain rather than asserting the attribute string. The session cookie is not involved here — it does not exist until the callback succeeds (D2). |
| `sessions` and `users` are outside RLS | Three layers per §1.1: `REVOKE UPDATE` on the binding, a narrowness check in CI, and the package comment. Revisit if a query ever needs a join. |
| An identity is repointed at another user, granting every organisation that person belongs to | `user_identities` has no mutable column and `vekst_app` holds no `UPDATE` on it. A test asserts the privilege is absent, so a later migration cannot quietly restore it. |
| An unfiltered `users` read discloses the customer directory | The narrowness check; the two-step read pattern in §1.1. |
| A returning user is locked out by a `UNIQUE (email)` violation | Cannot happen — the returning path writes nothing to `users` (D3a). The constraint is only reachable while provisioning an account that does not yet exist. |
| Invitation by email resolves ambiguously | Not reachable in this change; `UNIQUE (email)` keeps one owner per address. The pre-hijacking literature (USENIX Security '22, the Classic-Federated Merge Attack) targets exactly the invite-then-federate flow, so the invitations change verifies ownership before granting and prunes unverified accounts. |
| Google credentials in the repository | They arrive as a Kubernetes Secret, like the database credentials; `gitleaks` already runs in CI. |

## 6. Call-outs

- **Tenant isolation:** this change creates four tables outside RLS and states why each is
  outside it. Nothing here is tenant data, and 1.1's coverage test will confirm it. Because
  RLS cannot reach them, §1.1 replaces "hold to it" with three mechanisms: a revoked
  privilege, a narrowness check in CI, and the package comment.
- **The transaction seam:** this change renames `db.InTx` to `db.InSystemTx` while it still
  has no call sites, so 1.1 only adds beside it (D4).
- **Money, currency, `source_kind`, the classifier contract:** untouched.
