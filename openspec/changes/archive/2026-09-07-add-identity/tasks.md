**Prerequisite, blocking.** A Google Cloud project with an OAuth client, and the redirect
URIs registered. Without the client id, the secret and a registered URI, nothing from §5
onward can be run or tested. Confirm this before starting §1 — treat it as a third open input
alongside D-1 and D-2.

**Ordering.** This change runs **before** 1.1 `add-tenancy-and-rls`. Its migration takes
number `00003`; tenancy's takes `00004`. This change also performs the `InTx` →
`InSystemTx` rename while that entry point still has no call sites (design D4), so 1.1 only
adds the tenant-aware `InTx` beside it.

**Ownership.** B, per plan §2.1. Both reviewers on the four allowlist lines in
`deploy/db/rls-exempt-tables.txt` and on `/proto`.

## 1. Migration — B

- [x] 1.1 Add `00003_identity.sql` creating `users`, `user_identities`, `sessions` and
      `auth_flows` per design §1, with the lowercase-email check constraint and the partial
      and expiry indexes. `users.email` is **nullable** — not every provider asserts one — and
      `UNIQUE`, which Postgres satisfies with any number of NULLs (design D3a). `auth_flows.state`
      is `UNIQUE`: it is the callback's lookup key on a security check, and without it the
      lookup is a sequential scan and "the flow row" is ambiguous (design D1)
- [x] 1.1b Add `REVOKE UPDATE ON user_identities FROM vekst_app` to the same migration.
      00001's `ALTER DEFAULT PRIVILEGES` grants it otherwise, and the identity binding has no
      mutable column (design D3b). Test that the privilege is absent, so a later migration
      cannot quietly restore it
- [x] 1.2 Write the down step and confirm `up → down → up` against a scratch database
- [x] 1.3 Add all four tables to `deploy/db/rls-exempt-tables.txt`, each with the reason from
      design §1.1 — a both-reviewers change
- [x] 1.4 Confirm 0.2's allowlist CI step still passes, so that a fifth table added later
      without a line fails on its own pull request
- [x] 1.5 Add the narrowness check (design §1.1 layer 2), in the spirit of
      `scripts/check-db-entry-point.sh`: no `.sql` file outside the identity query file may
      name these four tables, and no statement in that file may join to a table outside them.
      Wire it into `make lint`
- [x] 1.6 Test the test: introduce a violating query, confirm the check goes red, remove it.
      A green check that cannot go red proves nothing

## 2. Queries and the transaction entry point — B

- [x] 2.1 Write the `sqlc` queries: find identity by `(provider, subject)`, insert user,
      insert identity, find user by id, insert session, find live session by token hash,
      touch `last_seen_at`, revoke session, insert and consume an auth flow. There is **no**
      update-user query and **no** update-identity query — `users` is provisioned once (D3a)
      and the binding is insert-once (D3b)
- [x] 2.2 Run `make gen`; confirm the codegen drift job stays green
- [x] 2.3 Rename `db.InTx` to `db.InSystemTx` (design D4). It has no non-test call sites
      today, so this is free now and costs every site written below if deferred to 1.1. Fix its
      doc comment in the same edit — it currently says 1.1 adds `SET LOCAL app.org_id` "here",
      which after the rename names the one entry point that will never set tenant context
- [x] 2.4 Record the resulting `InSystemTx` call-site count in this change's notes — 1.1's
      task 3.3 commits an expected count and starts from this number.
      **Result: 7 non-test call sites, all in `core/internal/identity`** —
      `signin.go` (3: create flow, consume flow, resolve-or-provision),
      `session.go` (3: create, resolve, revoke), `expiry.go` (1: the sweep).
      Nothing outside that package calls it, which is the property 1.1's
      committed count is there to keep true

## 3. Proto — B

- [x] 3.1 Add `proto/vekst/v1/identity.proto` with `IdentityService/GetCurrentUser` and the
      `User` message, carrying no organisation and no role (design §3)
- [x] 3.2 Run `buf lint` and `buf breaking --against origin/main`; run `make gen` and commit
      the Go and TypeScript stubs

## 4. Configuration and secrets — B

- [x] 4.1 Extend `core/internal/config` with the client id, client secret, redirect URL,
      session lifetime and cookie-secure flag, read once at startup like everything else
- [x] 4.2 Add `deploy/k8s/overlays/local/google-oidc.yaml` with **empty** values, committed, so
      `kubectl kustomize` renders in CI. Empty and not a sentinel: this Secret is really
      deployed, so a sentinel would trip 4.2c and crash-loop a developer who has no OAuth
      client. Do **not** copy the `db-secrets.yaml` pattern — those credentials are throwaway
      and these are not (design D6a)
- [x] 4.2b Load the real values from a git-ignored `google-oidc.secret.yaml`, applied by Tilt
      when present and skipped when absent, so a developer with no credentials still gets a
      working stack
- [x] 4.2c Make `core` refuse to start when the client id or secret is still the placeholder,
      and confirm `gitleaks` passes on the working tree and on history
- [x] 4.3 Change the local overlay host from `vekst.localhost` to `localhost`, and update the
      Tiltfile's printed URL and the README (design D6)

## 5. The login flow — B

- [x] 5.1 Discover Google's endpoints and JWKS from the well-known document at startup, with
      a bounded timeout and a clear failure if it is unreachable
- [x] 5.2 `GET /auth/google/start`: create the `auth_flows` row with `state`, `nonce` and a
      PKCE verifier, set the flow cookie to the row's `id`, redirect to Google. The cookie is
      `HttpOnly`, `SameSite=Lax`, `Secure`, short-lived, and holds the id rather than `state`,
      so a leaked callback URL carries only half of what a callback needs (design D1)
- [x] 5.3 `GET /auth/google/callback`: look the flow up by `state`, require the flow cookie to
      be present and to equal that row's `id`, then consume the row by deleting it and clear the
      flow cookie. A missing cookie, a mismatched one, and an unknown `state` are refused
      identically (design D1)
- [x] 5.4 Validate the ID token properly — signature against JWKS, `iss`, `aud`, `exp`, and
      `nonce` against the flow. Do not hand-roll it
- [x] 5.5 Resolve by `(provider, subject)`. **Found:** use that `user_id` and write nothing to
      `users`. **Not found:** provision — `INSERT` the user, then `INSERT` the binding. Never
      `ON CONFLICT DO UPDATE SET user_id`, which repoints an identity at another person
      (design D3b). Refuse an `email_verified` claim that is false
- [x] 5.5b Translate `23505` on `users_email_key` into the `email_taken` code rather than
      pre-checking with a `SELECT`, which two concurrent first sign-ins would both pass. The
      constraint is the check (design D3b)
- [x] 5.5c Normalise the BCP-47 `locale` claim (`en-GB`, `ru-RU`, `nb-NO`) to the supported set
      before insert, falling back to the default when it maps to nothing supported; the raw
      claim fails the CHECK on an ordinary sign-in (design D3a)
- [x] 5.6 Create the session: 32 random bytes, store only the SHA-256, set the cookie
      `HttpOnly`, `SameSite=Lax`, `Secure`, `Path=/`, no `Domain`
- [x] 5.7 `POST /auth/logout`: mark the session revoked and clear the cookie
- [x] 5.8 Return error codes, never sentences, from all three routes

## 6. Middleware and RPC — B

- [x] 6.1 Auth middleware: read the cookie, hash it, look up a live session, load the user,
      put it in the request context, touch `last_seen_at`
- [x] 6.2 Reject an unauthenticated Connect call with the unauthenticated code. Leave open:
      `/healthz`, `/readyz`, the three auth routes, **and `HealthService/Check`** — that one
      is a Connect RPC behind `/rpc`, not an HTTP probe path, and 0.2's spec requires it to
      answer with no database and no credential. Forgetting it breaks an existing test
- [x] 6.3 Implement `IdentityService/GetCurrentUser` from the request context
- [x] 6.4 Document in the identity package comment that these four tables are outside RLS and
      are read only by key through this package; that reading `users` in a tenant context is the
      two-step — `memberships` under RLS for the ids, then users by key — rather than a join;
      and that the query the rule cannot catch is an unfiltered read with no membership
      predicate at all, which is what the narrowness check is for (design §1.1)

## 7. Expiry job — B

- [x] 7.1 A periodic River job deleting `auth_flows` past `expires_at` and `sessions` past
      `expires_at` plus the retention window
- [x] 7.2 Test: expired rows go, live rows stay

## 8. Tests — B

- [x] 8.1 An ID token with a bad signature is refused
- [x] 8.2 An ID token with the wrong `aud`, an expired `exp`, or a `nonce` that does not match
      the flow is refused — one case each
- [x] 8.3 A callback with a `state` that matches no flow row, or a consumed one, is refused
- [x] 8.3b A callback carrying a valid `state` but no flow cookie, or a flow cookie naming a
      different flow, is refused — the two halves are checked independently (design D1)
- [x] 8.4 A second sign-in with the same `(provider, subject)` reuses the user rather than
      creating a second one
- [x] 8.5 A sign-in whose email already belongs to another user is refused while provisioning,
      and no account is merged
- [x] 8.5b A returning sign-in whose provider email has changed still succeeds, writes nothing
      to `users`, and leaves the stored address unchanged — including when that new address
      belongs to a different user, which must not lock the returning person out (design D3a)
- [x] 8.5c The identity binding cannot be repointed: `vekst_app` holds no `UPDATE` on
      `user_identities`, and an attempt to move a `(provider, subject)` row to another
      `user_id` fails at the database (design D3b)
- [x] 8.6 Negative: a request with no cookie, an unknown cookie, an expired session or a
      revoked session reaches no authenticated route
- [x] 8.7 The raw session token appears nowhere in the database — assert the stored value is
      a hash of the cookie value, not the value
- [x] 8.8 `/healthz` and `/readyz` still answer with no cookie, and carry no user data
- [x] 8.9 Drive the full redirect chain against a stub provider, so the cookie is proved to
      arrive on the callback rather than asserted from its attribute string

## 9. Web — A

**Deliberately unstyled.** `add-web-app-shell` (5.1) establishes the Tailwind token layer.
Anything designed here is restyled there, so build the smallest thing that works and expect
to throw the markup away.

- [x] 9.1 A sign-in screen with one button, sending the browser to `/auth/google/start`
- [x] 9.2 Call `GetCurrentUser` on load; unauthenticated renders sign-in, authenticated
      renders the shell with a sign-out control
- [x] 9.3 Sign-out posts to `/auth/logout` and returns to the sign-in screen
- [x] 9.4 State plainly that a signed-in user has no organisation yet, so 1.1's absence looks
      deliberate rather than broken
- [x] 9.5 i18n keys for every string added, `en` and `ru`

## 10. Documentation and close

- [x] 10.1 Add an authentication section to `docs/ARCHITECTURE.md` — it has none, and this
      change is the first to decide anything about it
- [x] 10.2 Update `README.md`: the local host is now `localhost`, the OAuth client is a
      prerequisite, and how to obtain a session for manual testing
- [x] 10.3 Update `CLAUDE.md`: the auth routes are the one non-Connect browser surface; an
      identity is `(provider, subject)`, never an email; `users` is provisioned once and never
      written by a token claim; and the transaction entry point is now `db.InSystemTx`
- [x] 10.4 Update the capability spec and run the full suite
