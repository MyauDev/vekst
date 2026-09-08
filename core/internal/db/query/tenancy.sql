-- Queries for the four tenant tables: organizations, entities, accounts and
-- memberships.
--
-- Not one statement here carries an org_id predicate, and that is the point.
-- Every one of these tables has a FORCE'd row-level-security policy restricting
-- it to app_current_org() (migration 00004), and db.InTx sets that context as
-- the transaction's first statement. The policy supplies the filter.
--
-- A hand-written `WHERE org_id = $1` would be worse than redundant: it would
-- return the right rows whether or not the policy existed, so the day someone
-- creates a table and forgets the policy, these queries keep working and
-- nothing fails. The absence of the predicate is what makes a missing policy
-- show up immediately -- as a query that raises 42704 rather than one that
-- quietly reads every tenant's rows.
--
-- Writes that are meant to touch exactly one row are :execrows, never :exec.
-- Row-level security filters UPDATE and DELETE silently -- a write whose target
-- the policy does not admit affects zero rows and raises nothing -- so the
-- affected-row count is the only signal that "the correction was saved" was a
-- lie. db.ExactlyOneRow turns a zero count into a named error (design D9).

-- name: InsertOrganization :one
-- Creating a tenant, under that tenant's own policy (design D4). The policy on
-- organizations is `id = app_current_org()`, so this needs the tenant context
-- to already name the row being inserted -- which is exactly what
-- OrgIDForNewOrg and InTx arrange. WITH CHECK passes because the identifiers
-- match, and no privileged path is needed to create an organisation.
INSERT INTO organizations (id, name, country, base_currency)
VALUES ($1, $2, $3, $4)
RETURNING id, name, country, base_currency, created_at;

-- name: GetOrganization :one
-- No WHERE clause: the policy admits exactly one row, the caller's own. A
-- predicate here could only ever narrow that to zero.
SELECT id, name, country, base_currency, created_at
FROM organizations;

-- name: UpdateOrganizationName :execrows
UPDATE organizations SET name = $1;

-- name: InsertEntity :one
INSERT INTO entities (org_id, name, legal_name, tax_id)
VALUES ($1, $2, $3, $4)
RETURNING id, org_id, name, legal_name, tax_id, created_at;

-- name: ListEntities :many
SELECT id, org_id, name, legal_name, tax_id, created_at
FROM entities
ORDER BY name;

-- name: GetEntity :one
-- id alone, with no org_id beside it. entities is keyed (org_id, id) and the
-- policy supplies the org_id half, so this cannot resolve outside the caller's
-- own organisation however the identifier was obtained.
SELECT id, org_id, name, legal_name, tax_id, created_at
FROM entities
WHERE id = $1;

-- name: UpdateEntityName :execrows
UPDATE entities SET name = $2 WHERE id = $1;

-- name: InsertAccount :one
INSERT INTO accounts (org_id, entity_id, name, currency, external_ref)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, org_id, entity_id, name, currency, external_ref, created_at;

-- name: ListAccounts :many
SELECT id, org_id, entity_id, name, currency, external_ref, created_at
FROM accounts
ORDER BY name;

-- name: ListAccountsForEntity :many
SELECT id, org_id, entity_id, name, currency, external_ref, created_at
FROM accounts
WHERE entity_id = $1
ORDER BY name;

-- name: GetAccount :one
SELECT id, org_id, entity_id, name, currency, external_ref, created_at
FROM accounts
WHERE id = $1;

-- name: UpdateAccountName :execrows
UPDATE accounts SET name = $2 WHERE id = $1;

-- name: InsertMembership :one
-- The creator's own membership, written in the same transaction that creates
-- the organisation (design D4). role is stored and nothing checks it yet --
-- enforcement is Product's, per the proposal's non-goals.
INSERT INTO memberships (org_id, user_id, role)
VALUES ($1, $2, $3)
RETURNING org_id, user_id, role, created_at;

-- name: ListMemberships :many
-- The current organisation's members. This deliberately does not join users:
-- users is global and outside row-level security, and
-- scripts/check-identity-queries.sh keeps the four identity tables reachable
-- from one query file only. A member list showing names and addresses is a
-- browser-facing read, which arrives with 2.1 and brings that decision with it.
SELECT org_id, user_id, role, created_at
FROM memberships
ORDER BY created_at;

-- name: UpdateMembershipRole :execrows
UPDATE memberships SET role = $2 WHERE user_id = $1;

-- name: DeleteMembership :execrows
DELETE FROM memberships WHERE user_id = $1;

-- orgs_for_user is deliberately NOT here.
--
-- sqlc cannot resolve the output columns of a RETURNS TABLE function: naming
-- them fails to compile ("column \"org_id\" does not exist"), and a star
-- expansion silently generates a []interface{} scanning one anonymous column,
-- which compiles and is wrong. The alternative that would satisfy sqlc is to
-- declare the function RETURNS SETOF memberships, and that trades away the
-- thing design D3 relies on: the exception is bounded by the function
-- returning two columns of one user's own rows, and widening it to the whole
-- membership row to suit a code generator is the wrong direction.
--
-- So the one call site is written by hand against pgx, in core/internal/db,
-- which is the only package permitted to hold a pool or issue a query
-- directly (scripts/check-db-entry-point.sh). See OrgIDForSession.
