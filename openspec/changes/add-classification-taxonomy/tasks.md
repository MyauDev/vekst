**Budget.** `docs/IMPLEMENTATION_PLAN.md` §3 allocates **1.5 person-days** to change 3.1.
These tasks total **≈ 11 hours ≈ 1.4 person-days**, which fits — but only because D-1 was
answered first. The taxonomy itself, its 101 nodes and the decisions behind them, was the
expensive part and is already done in `eval/`.

**Ordering.** Apply after `add-tenancy-and-rls` is merged. Three tasks below depend on
`app_current_org()` and on the non-superuser migrator, and neither exists before it. §D2 of
the design lists four points to agree with 1.1 *before* starting; two of them change 1.1's
own code, so raise them there first.

**Ownership.** Track B throughout. No file here is owned by Track A.

## 0. Points that belonged to change 1.1

Written when 1.1 was in flight. It merged as PR #5 on 2026-09-08 before these
were raised, so three of them are now changes to merged code and are made here.

- [x] 0.1 §D2(a): the RLS coverage test learns a `shared+tenant` state. Implemented as `deploy/db/rls-shared-tenant-tables.txt` plus `checkSharedTenantPolicies`, which demands a **stricter** pair — one read policy admitting shared-or-mine, one write policy admitting only mine — rather than relaxing the general rule
- [x] 0.2 §D2(b): `parent_id` takes a constraint trigger, not a composite foreign key
- [x] 0.3 §D2(c): seed precedes `ENABLE`/`FORCE` inside migration 005
- [ ] 0.4 §D2(d): `memberships.entity_id`. Migration 004 is written and merged without it, so this is now its own migration rather than one column in an unwritten one. Still cheap — nullable, no policy reads it — and still the same argument 1.1 accepts for `entities`

**A design correction found while implementing.** The proposal split shared from
per-organisation by depth: levels 1-2 shared, 3+ not. That is wrong, and the
data says so. A shared rule cannot point at a per-organisation row — every
organisation holds its own id for its own "Bank commission" — so anything a
template rule targets must itself be shared. Fourteen of the nineteen targeted
categories sit at levels 3 to 5, and all of them (Bank commission, VAT,
Currency exchange, Office rent) are universal. The split is now derived rather
than judged: 41 shared, 60 in the industry template. `eval/build.py`
`assign_scopes` states it.

The sixty are not seeded. They belong to whoever adopts them, and a shared row
belongs to nobody; they are written to `eval/out/industry_template.csv` for the
change that creates an organisation.

## 1. Migration — Track B

- [x] 1.1 Migration 005 up: `categories` with every column and check constraint from the design
- [x] 1.2 Add the `UNIQUE NULLS NOT DISTINCT (taxonomy_version, org_id, code)` constraint and both indexes
- [x] 1.3 Constraint trigger asserting a parent is shared or same-organisation (§D2(b))
- [x] 1.4 Seed the 46 shared rows from `eval/out/seed_categories.sql`, inside the same migration and **before** RLS is enabled
- [x] 1.5 Enable and `FORCE ROW LEVEL SECURITY`; create `categories_read` and `categories_write` as two separate policies
- [x] 1.6 Migration 005 down, and confirm `up → down → up` against a scratch database
- [x] 1.7 Grant `vekst_app` DML on the table; confirmed: 00001's default privileges already grant SELECT, INSERT, UPDATE and DELETE, and no follow-up grant is needed

## 2. Generated queries — Track B

- [x] 2.1 `core/internal/db/query/taxonomy.sql`: `EffectiveTaxonomy` and `ClassifiableCategories`
- [x] 2.2 Run `make gen`; confirm the codegen drift job stays green

## 3. Tests — Track B

- [x] 3.1 **Cross-tenant isolation:** organisation A cannot read organisation B's leaves, and sees every shared row
- [x] 3.2 **Cross-tenant write:** A cannot insert, update or delete a row owned by B, and the failure is a policy denial rather than a not-found
- [x] 3.3 **Shared rows are read-only to the application:** `vekst_app` cannot insert, update or delete a row with `org_id IS NULL`, under either policy
- [x] 3.4 **Parent trigger:** a leaf may hang under a shared parent; a leaf may not hang under another organisation's parent, and the error is identical whether that parent exists or not
- [x] 3.5 **Seed ordering:** asserted directly rather than by building a deliberately broken migration — once the policies exist, even `vekst_migrator` is refused a shared row, which is what makes the ordering load-bearing
- [x] 3.6 **Computed lines:** `ClassifiableCategories` returns no row where `is_computed`, so no rule can ever target GM, NM, CM, IBT or NI
- [x] 3.7 **Seed integrity:** every seeded row's `parent_id` resolves, every leaf is childless, and the 66 leaves and 20 shared nodes match what `eval/emit.py` reports
- [x] 3.8 **`requires_allocation`:** both payroll buckets carry it and nothing else does

## 4. Drift — Track B

- [x] 4.1 CI **cannot** re-run `eval/emit.py`: its input is `../docCl`, deliberately outside the repository. `scripts/check-taxonomy-seed.sh` checks what is checkable without it, and catches the failure that actually matters — migration 005 embeds a copy of the seed, and this proves the copy has not drifted
- [x] 4.2 Same script: every `category_code` in `seed_rules.sql` is seeded, is a leaf, and is not a computed line. Verified to fail on both — a broken copy and a dangling code — rather than only to pass

## 5. Close

- [x] 5.1 `docs/IMPLEMENTATION_PLAN.md`: 3.1 marked delivered, D-1 closed, D-2 marked partly closed — four redacted Priorbank fixtures landed with the parser
- [x] 5.2 CODEOWNERS. The original task is obsolete: ownership moved to `@MyauDev/admin` and `/core/internal/db/` already covers the query file. What did need entries: `deploy/db/rls-shared-tenant-tables.txt`, because deciding a table has unowned rows is a security decision, and migration 005 itself
- [x] 5.3 Capability spec updated — it carried the depth-based split in a heading and now states the rule that replaced it. Full suite run against a live Postgres
