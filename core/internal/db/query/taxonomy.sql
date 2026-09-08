-- Queries over the category taxonomy.
--
-- `categories` is the first table that is shared and tenant at once: a row with
-- a NULL org_id belongs to everyone, a row with one belongs to a single
-- organisation. Row-level security already enforces that split -- the read
-- policy admits a shared row or mine and nothing else -- so nothing here
-- restates it. A predicate on org_id in this file would be a second place to
-- get isolation right, and the point of doing it in the database is that there
-- is only one.
--
-- Every statement runs inside db.InTx, which sets the tenant context. Without
-- it app_current_org() raises 42704 and the query fails, rather than returning
-- an empty tree that a report would render as a customer with no categories.

-- name: EffectiveTaxonomy :many
-- The tree this organisation actually sees: the shared skeleton plus its own
-- leaves, in code order, which is also report order because a code carries its
-- own position (two characters per level).
SELECT id, taxonomy_version, org_id, scope, code, parent_id, level, name,
       pnl_section, is_pnl, is_leaf, is_computed, formula, requires_allocation
FROM categories
WHERE taxonomy_version = $1
ORDER BY code;

-- name: ClassifiableCategories :many
-- What a classification may target, and the only list change 3.2 sends to the
-- classifier.
--
-- Leaves only, and never a computed line. GM, NM, CM, IBT and NI are
-- arithmetic over other lines; a transaction landing in one would be counted
-- twice, once where it belongs and once in the formula that already includes
-- it. A section is excluded for the same reason: its figure is the sum of its
-- children.
SELECT id, taxonomy_version, org_id, scope, code, parent_id, level, name,
       pnl_section, is_pnl, is_leaf, is_computed, formula, requires_allocation
FROM categories
WHERE taxonomy_version = $1
  AND is_leaf
  AND NOT is_computed
ORDER BY code;

-- name: CategoryByCode :one
-- One category by its natural key. `org_id IS NOT DISTINCT FROM $2` rather
-- than `=`: the shared rows carry NULL, and NULL = NULL is unknown, so the
-- ordinary comparison would never match exactly the rows every organisation
-- needs to reach.
SELECT id, taxonomy_version, org_id, scope, code, parent_id, level, name,
       pnl_section, is_pnl, is_leaf, is_computed, formula, requires_allocation
FROM categories
WHERE taxonomy_version = $1 AND org_id IS NOT DISTINCT FROM $2 AND code = $3;
