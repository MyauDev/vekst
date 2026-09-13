## Context

Two things already exist and this change sits between them. `core/internal/ingest` detects
the parsing parameters of a file. `ARCHITECTURE.md` §5.5 sketches `import_profiles` with
nine columns and says nothing about how a profile and a detector relate when both have an
opinion.

That relationship is the only interesting decision here, and it is worth more than the
table.

## Spec ordering — this change stacks on an unarchived one

The `MODIFIED Requirements` block in `specs/file-ingestion/spec.md` restates a requirement
that **is not in `openspec/specs/`**: that directory holds `identity-access`,
`platform-foundation` and `tenancy` and nothing else. `file-ingestion` is introduced by
change 2.2 `add-statement-parsing` and the specific requirement modified here belongs to
change 2.1 `add-file-upload`. Neither is archived, so the stack is three deep: 2.2, then
2.1, then this.

So this delta does not resolve against a synced baseline, and `openspec validate` cannot
check it. Stated here rather than discovered at archive time: **2.1 must be implemented and
archived before 2.4 is archived**, and if 2.1's requirement changes in review, the quoted
baseline here changes with it. Change `add-web-experience` wrote the same paragraph for the
same reason; it is a property of stacked changes, not a mistake in either.

## Goals / Non-Goals

**Goals:** the table, the canonical field list, the constraint that protects it, the
override semantics, and the batch's reference to a profile.

**Non-goals:** a screen, inference, versioning, per-account profiles. Named in the proposal.

## The data model

```sql
CREATE TABLE import_profiles (
    id     uuid NOT NULL DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,

    name        text NOT NULL CHECK (length(btrim(name)) > 0),
    source_kind text NOT NULL CHECK (source_kind IN ('ledger','bank')),

    -- Source column name -> canonical field. Keys are the source's words and
    -- cannot be constrained; values come from a fixed list and are, by the
    -- trigger below.
    column_map jsonb NOT NULL,

    -- Every one of these is NULL-able and every NULL means "let the detector
    -- decide". See D1.
    charset     text NULL,
    delimiter   text NULL CHECK (delimiter IS NULL OR length(delimiter) = 1),
    decimal_sep text NULL CHECK (decimal_sep IN (',', '.')),
    date_fmt    text NULL,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (org_id, id),
    UNIQUE (org_id, name)
);

ALTER TABLE import_batches
    ADD COLUMN import_profile_id uuid NULL,
    ADD FOREIGN KEY (org_id, import_profile_id)
        REFERENCES import_profiles (org_id, id) ON DELETE RESTRICT;
```

The foreign key is composite, like every referential check between tenant tables, because
an unscoped one is a cross-tenant existence oracle (migration 00004, design D1).

`ON DELETE RESTRICT`, so a profile cannot be removed while a batch records that it was
parsed with it. What a file was parsed with is part of how its numbers were produced.

### Row-level security

```sql
ALTER TABLE import_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE import_profiles FORCE  ROW LEVEL SECURITY;
CREATE POLICY import_profiles_tenant ON import_profiles FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());
```

Ordinary shape: one owner per row. No shared rows — an industry template that a customer
adopts is a copy waiting to be made, which is the distinction
`deploy/db/rls-shared-tenant-tables.txt` already writes down.

## D1 — A profile overrides field by field

**Rejected: a profile replaces detection entirely.** It is simpler to reason about — either
the file is described or it is guessed — and it means the first hand-written profile has to
get charset, delimiter, decimal separator and date format all correct, for a file whose
charset the detector already identifies correctly. A profile that gets three of four right
is then *worse* than no profile, and the failure is a silently mis-parsed number rather
than an error.

**Chosen: each NULL field means "detect this one".** A profile can say only what the
detector is wrong about. The cost is that a profile is no longer a complete description of
a file, so `import_validations` and the batch record what was *used*, not what was
configured — §5.3 stores the resolved parameters on the batch for exactly this reason.

`column_map` is the exception and is `NOT NULL`: there is no useful partial column map, and
a detector does not guess which column is the amount.

## D2 — The canonical field list is fixed, and the database enforces it

`column_map` values name canonical fields. The list is closed, because every one of them
has to land in a column of `transactions`:

```
booked_on · value_on · amount · debit · credit · currency
description · counterparty · bank_ref · document_ref · posting_no
opening_balance · closing_balance · row_count_declared
```

A typo — `descrption` — in a free-form JSONB is a column that maps to nothing, a
description that fails the "present after trimming" correctness check on every row, and a
rejected file whose error report blames the customer's data.

A constraint trigger asserting every value is in the list turns that into a write error at
the moment the profile is written. This follows change 3.1's precedent, where a trigger
states a rule a foreign key cannot.

**Rejected: a `profile_fields` reference table with a real foreign key.** It is the more
ordinary shape and it needs `column_map` normalised into rows, which makes a profile a
join rather than a document and buys nothing at this size.

## D3 — The profile is named on the batch, never inferred

`CreateImportBatchRequest` gains `optional string import_profile_id`. The browser names it;
nothing guesses.

Inference by header fingerprint is the obvious next step and it is Product's, because the
failure mode is silent: two of a customer's banks share a header shape, the wrong profile
is selected, the amount column maps to the balance column, and the file validates. Every
check in stage ② passes on well-formed wrong numbers.

Naming it explicitly means a wrong profile is a wrong choice a person made and can see.

## Queries

```sql
-- name: InsertImportProfile :one
-- name: ListImportProfiles :many
-- name: GetImportProfile :one
-- name: UpdateImportProfile :execrows
-- No org_id predicate anywhere: the policy is the filter (see tenancy.sql).
```

## Proto

One optional field added to the existing `CreateImportBatchRequest`, plus four RPCs on
`ImportService` for profile CRUD so that a profile can be created without SQL once a screen
exists. Both reviewers, per CODEOWNERS.

```protobuf
message CreateImportBatchRequest {
  // ... fields 1-5 from change 2.1 ...
  optional string import_profile_id = 6;
}
```

Field addition. `buf breaking` reports nothing, and §4.2 confirms it rather than assuming.

## What this touches from the invariants list

- **Tenant isolation** — a new tenant table, ordinary shape, plus a composite foreign key
  from `import_batches`.
- **Money and currency** — indirectly: `decimal_sep` and the amount mapping decide whether
  an amount parses at all. No amount is stored here.
- **`source_kind`** — a profile carries one, and §6.5 refuses a profile whose `source_kind`
  differs from the batch's.
- **The classifier contract** — none.

## Rejected alternatives

| Rejected | Why |
| --- | --- |
| A profile replaces detection entirely | The first hand-written profile must get four parameters right for a file whose charset is already detected correctly. Three of four right is worse than none. See D1 |
| Free-form `column_map` values | A typo becomes a column mapping to nothing and an error report that blames the customer's file. See D2 |
| A `profile_fields` reference table | Normalises a document into rows and makes every read a join, for a list of fourteen names |
| Infer the profile from a header fingerprint | The failure is silent and well-formed: every stage ② check passes on wrong numbers. Product's, and with a UI to show the choice. See D3 |
| Typed columns instead of `column_map` JSONB | The canonical fields are fixed but the source names are not, so the table would need a column per canonical field and still hold a string |
| `ON DELETE CASCADE` from profile to batch | Deleting a profile would delete the record of how existing numbers were produced |

## Risks

| Risk | Mitigation |
| --- | --- |
| A profile is edited after a batch used it, and the batch's numbers can no longer be explained | §5.3 stores the **resolved** parameters on the batch, so what was used is recorded even when the profile changes |
| The canonical field list grows and the trigger's copy drifts from the parser's | The list is defined once in Go and the migration's trigger is generated from it; §6.6 fails if they differ |
| Two profiles are near-identical and a person picks the wrong one | Out of scope, and the reason inference is not in this change: a wrong choice is at least visible on the batch |
