-- Queries for the four identity tables: users, user_identities, sessions and
-- auth_flows.
--
-- These four tables are outside row-level security (deploy/db/rls-exempt-
-- tables.txt), so nothing at the database level filters what they return.
-- Every statement in this file therefore reads by primary key or unique key,
-- and joins only to another of the four. scripts/check-identity-queries.sh
-- enforces both properties: no other query file may name these tables, and
-- nothing here may reach outside them. See openspec add-identity design 1.1.
--
-- There is deliberately no update-user statement and no update-identity
-- statement. The account record is provisioned once and never written by a
-- later token claim (design D3a); the identity binding is insert-once, and
-- vekst_app holds no UPDATE privilege on it at all (design D3b).

-- name: FindIdentity :one
-- The whole of sign-in resolution. An identity is (provider, subject) and
-- never an email: an issuer may reuse an email across different end users
-- over time, so matching a login by address is the standard account-takeover
-- path.
SELECT provider, subject, user_id, created_at
FROM user_identities
WHERE provider = $1 AND subject = $2;

-- name: InsertUser :one
-- Provisioning, and the only statement that ever writes a user row. A
-- duplicate address raises 23505 on users_email_key, which the caller
-- translates into the email_taken code -- the constraint is the check, so two
-- concurrent first sign-ins on one address cannot both succeed.
INSERT INTO users (email, name, locale)
VALUES ($1, $2, $3)
RETURNING id, email, name, locale, created_at;

-- name: InsertIdentity :exec
-- Insert-once, with no ON CONFLICT clause. Repointing a subject at another
-- user is account takeover; vekst_app has no UPDATE privilege here, so this
-- cannot become an upsert without a migration that CODEOWNERS would catch.
INSERT INTO user_identities (provider, subject, user_id)
VALUES ($1, $2, $3);

-- name: FindUserByID :one
SELECT id, email, name, locale, created_at
FROM users
WHERE id = $1;

-- name: InsertSession :one
-- token_sha256 is the SHA-256 of 32 random bytes; the raw value lives only in
-- the cookie and is never stored, so a read of this table yields nothing a
-- browser could present.
INSERT INTO sessions (token_sha256, user_id, expires_at, user_agent)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, created_at, expires_at;

-- name: FindLiveSessionWithUser :one
-- The middleware's one read per request: a unique-index probe on the token
-- hash, joined to the session's own user. Both tables are inside the exempt
-- four, so this join stays within the boundary the narrowness check draws.
-- Expiry and revocation are predicates here rather than checks in Go, so a
-- revoked or expired session simply matches nothing.
SELECT
    s.id           AS session_id,
    s.expires_at   AS session_expires_at,
    u.id           AS user_id,
    u.email        AS user_email,
    u.name         AS user_name,
    u.locale       AS user_locale
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_sha256 = $1
  AND s.revoked_at IS NULL
  AND s.expires_at > now();

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = now() WHERE id = $1;

-- name: RevokeSession :exec
-- Sign-out marks the row revoked rather than deleting it, so the session is
-- auditable afterwards and the expiry job is what finally removes it.
UPDATE sessions SET revoked_at = now()
WHERE token_sha256 = $1 AND revoked_at IS NULL;

-- name: InsertAuthFlow :one
INSERT INTO auth_flows (state, nonce, code_verifier, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: ConsumeAuthFlow :one
-- Consuming deletes, in one statement, so a replayed callback cannot race a
-- first one: exactly one DELETE can return a row. A callback whose state
-- matches nothing and one whose flow was already consumed are therefore
-- indistinguishable, which is deliberate -- the caller learns nothing from
-- the difference.
DELETE FROM auth_flows
WHERE state = $1 AND expires_at > now()
RETURNING id, nonce, code_verifier;

-- name: DeleteExpiredAuthFlows :execrows
DELETE FROM auth_flows WHERE expires_at < now();

-- name: DeleteExpiredSessions :execrows
-- Sessions outlive their expiry by a retention window so a recently expired
-- session is still visible to an operator asking what happened.
DELETE FROM sessions WHERE expires_at < $1;
