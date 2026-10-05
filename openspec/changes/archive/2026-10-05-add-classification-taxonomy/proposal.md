## Why

Nothing can be classified until there is a list of categories to classify into, and
nothing can be reported until those categories carry a section and a sign. Changes 3.2
(`add-classification-engine`), 3.3 (`add-review-queue`) and 4.1 (`add-management-pnl`) all
wait on this table.

Decision **D-1** supplied the content on 2026-09-08: 101 categories, merged in
`eval/build.py` and measured against 4,508 real transactions. This change is the schema that
receives it.

Milestone: **Demo**. Capability: **`classification-taxonomy`** (new).

## What Changes

- Migration 005 creates `categories`, seeded from `eval/out/seed_categories.sql`.
- **The table is shared and tenant at once, which is new.** Levels 1–2 are ours, versioned,
  and identical for every customer; `GM = NET SALES − CS` must mean one thing everywhere.
  Leaves such as `IT Park - membership` belong to one organisation and to no other. So
  `org_id` is nullable, and the read policy admits a row that is shared *or* mine while the
  write policy admits only mine. `categories` is the first table in the schema with that
  shape, and §D2 of the design asks change 1.1 to sanction it.
- Three columns the current schema does not have: `is_computed` with `formula` (GM, NM, CM,
  IBT and NI are arithmetic over other lines and can never receive a transaction),
  `requires_allocation` (payroll is known, the department is not), and `scope`.
- A `taxonomy_version` on every row. A report pins it, so a customer adding a leaf in June
  does not silently redraw March.
- sqlc queries to read a tenant's effective tree — shared rows plus its own, ordered.

## Capabilities

### New Capabilities
- `classification-taxonomy`: the category tree, its versioning, and its shared/tenant split.

### Modified Capabilities
- None. `tenancy` is depended on, not modified.

## Non-goals

- **No classification.** No rules, no engine, no `ClassifyBatch`. That is change 3.2, which
  seeds `classification_rules` from `eval/out/seed_rules.sql`.
- **No editing UI.** Change 3.3 lets a customer add a leaf from the review queue; this only
  makes such rows possible.
- **No report.** `pnl_section`, ordering and the formulas are stored, not evaluated. 4.1
  evaluates them.
- **No customer dimension.** Revenue by customer comes from the counterparty, never from the
  tree. 84 customer names were deliberately lifted out of it.

## Impact

Touches `/core/migrations`, `/core/internal/db/query`, `/core/gen/db`, and the RLS coverage
test that change 1.1 introduces. **Apply after 1.1**: the policies below need
`app_current_org()` and the non-superuser migrator, and neither exists before it.
