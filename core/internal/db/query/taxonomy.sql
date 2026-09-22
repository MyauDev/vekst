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

-- name: IndustryTemplate :many
-- Every category_templates row for one taxonomy version, in adoption order:
-- level ascending, then code. AdoptIndustryTemplate relies on the order -- a
-- level-5 leaf's parent is a level-4 row this same loop already inserted, so
-- the parent must exist before the child is reached. category_templates
-- carries no org_id and no policy (deploy/db/rls-exempt-tables.txt); it
-- belongs to nobody, so unlike every other query in this file there is no
-- tenant context to rely on and none to set.
SELECT taxonomy_version, code, parent_code, level, name, is_leaf, is_pnl
FROM category_templates
WHERE taxonomy_version = $1
ORDER BY level, code;

-- name: InsertCategory :one
-- An organisation's own category row -- adopted from the industry template,
-- or any other org-scoped category a later change writes. parent_id is
-- resolved by the caller (tenancy.AdoptIndustryTemplate): it may point at a
-- shared row or at this same organisation's own, and this query does not
-- know which -- categories_parent_is_visible is the trigger that refuses a
-- wrong answer, at the database's own insistence rather than this query's.
INSERT INTO categories (taxonomy_version, org_id, scope, code, parent_id, level,
                        name, is_leaf, is_pnl)
VALUES ($1, $2, 'org', $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: CategoryByCode :one
-- One category by its natural key. No org_id predicate, same as
-- EffectiveTaxonomy above and for the same reason: RLS already admits a
-- shared row or this organisation's own, and a code is unique within
-- whichever of those it belongs to.
SELECT id, taxonomy_version, org_id, scope, code, parent_id, level, name,
       pnl_section, is_pnl, is_leaf, is_computed, formula, requires_allocation
FROM categories
WHERE taxonomy_version = $1 AND code = $2;
