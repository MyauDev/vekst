-- Identity: who a caller is, and how that survives between requests.
--
-- All four tables here are deliberately global rather than tenant-scoped, and
-- all four are listed in deploy/db/rls-exempt-tables.txt with their reasons.
-- ARCHITECTURE.md 5.5 makes users the one global table; memberships (change
-- 1.1) is what mediates access to an organisation's data. A session belongs to
-- a person and not to an organisation, and an auth_flow exists before anyone is
-- authenticated at all, so it cannot have an owner.
--
-- Because row-level security is not protecting these tables, the mitigations
-- are elsewhere: the REVOKE below, the narrowness check in
-- scripts/check-identity-queries.sh, and the package comment in
-- core/internal/identity. See openspec add-identity design 1.1.

-- +goose Up

-- email is nullable: not every OIDC provider asserts one (Apple's private
-- relay, SAML attribute mappings), and Postgres treats NULLs as distinct in a
-- unique index, so any number of address-less accounts coexist. It is UNIQUE
-- because one verified address must resolve to one account -- that constraint
-- is the sole mechanism behind "one address, one account", and it is what keeps
-- a future user_emails backfill conflict-free.
--
-- The row is written once, at provisioning, from the claims of the identity
-- that creates it, and no later sign-in updates it (design D3a). That rule --
-- not the column's location -- is what stops two providers overwriting each
-- other's view of the same person.
CREATE TABLE users (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email      text,
    name       text,
    locale     text        NOT NULL DEFAULT 'en' CHECK (locale IN ('en','ru')),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_email_lowercase CHECK (email IS NULL OR email = lower(email)),
    UNIQUE (email)
);

-- An identity is (provider, subject) -- never an email. OpenID Connect states
-- that an issuer may reuse an email across different end users over time, so
-- only the issuer/subject pair identifies a person stably. Matching a login to
-- an account by email is the standard account-takeover path.
--
-- provider names an *issuer instance*, not a protocol. For Google the two
-- collapse, because iss is always https://accounts.google.com. They do not
-- collapse for multi-tenant Entra or for SAML, where a NameID is unique only
-- within its issuing IdP: such a provider arrives either as an issuer-scoped
-- value ('saml:acme-corp') or by adding an issuer column to the key.
CREATE TABLE user_identities (
    provider   text        NOT NULL CHECK (provider IN ('google')),
    subject    text        NOT NULL,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, subject)
);
CREATE INDEX user_identities_user_idx ON user_identities (user_id);

-- The binding is insert-once and has no mutable column by construction: no
-- email, no name, no last-login. 00001's ALTER DEFAULT PRIVILEGES grants
-- vekst_app UPDATE on every table created afterwards, so it has to be taken
-- back explicitly here.
--
-- Without this, one `ON CONFLICT (provider, subject) DO UPDATE SET user_id`
-- repoints a provider subject at a different person -- which, once change 1.1
-- adds memberships, hands the holder of that Google account every organisation
-- the original person belongs to. No RLS policy would have caught it: it is a
-- legitimate-looking write by vekst_app. The privilege makes it impossible
-- instead of merely reviewed. See design D3b.
REVOKE UPDATE ON user_identities FROM vekst_app;

-- The cookie carries 32 random bytes; only their SHA-256 lands here, so a read
-- of this table -- a backup, a log line, a query gone wrong -- yields nothing a
-- browser could present. Lookup is by that hash, a unique-index probe, so
-- nothing is lost in speed. Revocation is the reason the table exists at all:
-- a signed stateless token cannot offer it.
CREATE TABLE sessions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token_sha256 bytea       NOT NULL UNIQUE,
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz,
    user_agent   text
);
CREATE INDEX sessions_user_idx ON sessions (user_id) WHERE revoked_at IS NULL;
CREATE INDEX sessions_expiry_idx ON sessions (expires_at);

-- The memory of an in-flight login. state, nonce and the PKCE verifier are all
-- generated at /auth/google/start and needed again at /auth/google/callback --
-- two separate HTTP requests, with a redirect to Google in between.
--
-- state is UNIQUE because it is the callback's lookup key on a security check:
-- two rows sharing a value would make "the flow row" ambiguous at exactly the
-- moment it matters, and without the index the lookup is a sequential scan on
-- every sign-in. Consuming a flow deletes the row, so a replayed callback and
-- an invented state are refused identically.
CREATE TABLE auth_flows (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    state         text        NOT NULL UNIQUE,
    nonce         text        NOT NULL,
    code_verifier text        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL
);
CREATE INDEX auth_flows_expiry_idx ON auth_flows (expires_at);

-- +goose Down

DROP TABLE auth_flows;
DROP TABLE sessions;
DROP TABLE user_identities;
DROP TABLE users;
