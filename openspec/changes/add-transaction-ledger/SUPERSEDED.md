# Change 2.5 `add-transaction-ledger` — superseded

**Superseded 2026-09-13** by the implementation on `origin/implement-transaction-ledger`:
proposal 78079d0, migration `core/migrations/00007_transaction_ledger.sql` at c3de2e8,
with tests. That version is the one to build.

This directory previously held a competing proposal for the same change, written on
2026-09-10 before the implementation existed. It has been removed rather than kept, because
two proposals for one change at one path is a merge conflict waiting for whoever checks out
that branch — git refuses to overwrite untracked files — and because a second opinion that
nobody will read is not worth the confusion it causes.

What was in it that 007 does not do is recorded here instead.

## Three amendments 007 should take

**1. `line_no`.** 007 stores no line number on a transaction. `ARCHITECTURE.md` §4a.1 keys
the error report to the line in the **original** file, and change 4.2's drill-down has to
reach it, so a figure in a report can be opened back to the row a customer can find in
Excel. Change 2.2 already carries it that far — `ingest.Row.LineNo`, pinned by
`TestLineNumbersReferToTheOriginalFile` — and it is lost at the persist boundary. One
column now; a backfill of every transaction later.

**2. A `currencies` table.** Migration 00004's own comment assigns it to "the change that
first stores an amount (2.5)":

> Making that list one artifact instead of two — a currencies reference table with real
> foreign keys — belongs with the change that first stores an amount (2.5), because it is a
> global non-tenant table and so needs an allowlist row and both reviewers.

The ISO-4217 exponents still exist twice, as a shape `CHECK` in SQL and as a map in
`core/internal/money`. A disagreement about JPY's exponent is a report wrong by a factor of
a hundred. It is a global table, so it needs a line in `deploy/db/rls-exempt-tables.txt` and
both reviewers.

**3. `direction` as a generated column.** 007 stores `direction text CHECK (direction IN
('income','expense'))` beside a `bigint amount_minor`, with no constraint relating the two.
That is two copies of one fact, which migration 00004 refused for `base_currency` on the
grounds that two unconstrained copies can disagree with no error and whichever the query
happens to read wins. Verified against PostgreSQL 16 that the derived form needs no cast:

```sql
direction text GENERATED ALWAYS AS
    (CASE WHEN amount_minor > 0 THEN 'in' ELSE 'out' END) STORED
```

An attempt to write it is then an error rather than a silent disagreement.

## One place the superseded document was wrong

It required a same-currency row to be stored with `fx_rate = 1` and
`base_amount_minor = amount_minor`. 007 forbids that, with
`num_nonnulls(fx_rate, fx_rate_on, base_amount_minor, base_currency) IN (0,4)` and
`base_currency <> currency`, keeping "converted" and "already in base currency" as two
distinct states rather than collapsing them into one. Verified on PostgreSQL 16 that 007's
constraints reject both a half-populated conversion and a same-currency one. **007 is
right and the superseded document was wrong.**

## One question still open against 007

`CREATE UNIQUE INDEX transactions_dedup_idx ON transactions (org_id, dedup_hash)` is
**unique**. Two genuinely distinct payments on one day — same account, amount, normalised
description, empty `bank_ref` — hash identically, and the second is then refused
permanently with no record that it existed. Verified on PostgreSQL 16. The column landing
early is right; the uniqueness is the question. Change 2.6 task §0.4 owns settling it:
hash the four Priorbank fixtures, count collisions, then drop the uniqueness either here or
in migration 011.
