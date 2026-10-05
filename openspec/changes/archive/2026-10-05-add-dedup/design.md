## Context

`ARCHITECTURE.md` §5 gives four levels, their keys and their actions, and the hash:

```
dedup_hash = sha256(org_id, entity_id, account_id, booked_on, amount_minor, currency,
                    normalize(description), bank_ref, document_ref, posting_no)
```

Change 2.5 computes and stores it and compares nothing. Change 2.1 stores the file hash and
compares nothing. This change is the comparison, and almost every decision in it comes from
one property of that hash that the document does not state:

**The hash has false positives, and they are ordinary.** Two coffees on the same card on
the same day, at the same price, with the same merchant description and no bank reference,
produce one hash. That is not a corrupted file; that is Tuesday. Every design decision
below follows from refusing to treat a hash collision as proof.

## Goals / Non-Goals

**Goals:** D1 as a database guarantee, D2 and D3 as reviewable skips, internal-transfer
detection with a deterministic pairing rule, and one home for `normalize()`.

**Non-goals:** D4, a screen, re-running over history. Named in the proposal.

## The data model

```sql
-- D1. A guarantee, not a check-then-act: two concurrent uploads of one file
-- cannot both reach imported. Partial, because a rejected or abandoned batch's
-- file must be re-uploadable after the customer fixes it.
CREATE UNIQUE INDEX import_batches_file_once
    ON import_batches (org_id, file_sha256)
    WHERE status = 'imported';

-- D2 and D3. A skipped row is a row the customer's file contained and their
-- reports do not. That has to be inspectable.
CREATE TABLE dedup_skips (
    id       uuid NOT NULL DEFAULT gen_random_uuid(),
    org_id   uuid NOT NULL,
    batch_id uuid NOT NULL,

    -- The line in the ORIGINAL file, so a skip can be found in Excel like an
    -- error can.
    line_no    integer  NOT NULL CHECK (line_no > 0),
    posting_no smallint NOT NULL,

    level      text  NOT NULL CHECK (level IN ('D2','D3')),
    -- text, not bytea: migration 007 declares transactions.dedup_hash as text,
    -- and a skip record whose hash cannot be compared to the column it came
    -- from is not a record of anything.
    dedup_hash text NOT NULL CHECK (length(dedup_hash) > 0),

    -- D3 only: the row this one matched, and the batch that holds it. NULL for
    -- D2, where the original is another line of this same file.
    matched_transaction_id uuid NULL,
    matched_batch_id       uuid NULL,

    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (org_id, id),
    FOREIGN KEY (org_id, batch_id) REFERENCES import_batches (org_id, id) ON DELETE RESTRICT,
    UNIQUE (org_id, batch_id, line_no, posting_no),
    CONSTRAINT dedup_skips_d3_names_its_original CHECK (
        (level = 'D2' AND matched_transaction_id IS NULL AND matched_batch_id IS NULL) OR
        (level = 'D3' AND matched_transaction_id IS NOT NULL AND matched_batch_id IS NOT NULL))
);

-- Internal transfers. Deliberately NOT transaction_links: that table is D4's,
-- and it relates two SOURCES describing one event. This relates two EVENTS that
-- cancel. One table for both would make every consumer filter by which kind,
-- and the P&L rule differs between them.
CREATE TABLE internal_transfers (
    id     uuid NOT NULL DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,

    out_txn_id uuid NOT NULL,
    in_txn_id  uuid NOT NULL,

    detected_at  timestamptz NOT NULL DEFAULT now(),
    dismissed_by uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    dismissed_at timestamptz NULL,

    PRIMARY KEY (org_id, id),
    FOREIGN KEY (org_id, out_txn_id) REFERENCES transactions (org_id, id) ON DELETE CASCADE,
    FOREIGN KEY (org_id, in_txn_id)  REFERENCES transactions (org_id, id) ON DELETE CASCADE,

    CONSTRAINT internal_transfers_two_sides CHECK (out_txn_id <> in_txn_id),
    CONSTRAINT internal_transfers_dismissal_is_whole CHECK (
        (dismissed_by IS NULL) = (dismissed_at IS NULL))
);

CREATE INDEX internal_transfers_active_idx ON internal_transfers (org_id)
    WHERE dismissed_at IS NULL;

-- One row per transaction that is in any pair, on either side. THIS is what
-- enforces "a transaction belongs to at most one pair".
--
-- An earlier draft put UNIQUE (org_id, out_txn_id) and UNIQUE (org_id, in_txn_id)
-- on the table above and claimed they did it. They do not, and the difference is
-- not subtle: verified on PostgreSQL 16 that both constraints happily accept a
-- transaction as the `in` side of one pair and the `out` side of another. Two
-- constraints on two columns cannot see each other. A three-way set of equal
-- transfers would then pair ambiguously, and which lines the P&L excludes would
-- depend on insertion order.
--
-- Both member rows are written in the same statement as the pair, so a pair
-- whose sides are already spoken for fails rather than half-inserting.
CREATE TABLE internal_transfer_members (
    org_id      uuid NOT NULL,
    txn_id      uuid NOT NULL,
    transfer_id uuid NOT NULL,
    side        text NOT NULL CHECK (side IN ('out','in')),

    PRIMARY KEY (org_id, txn_id),
    FOREIGN KEY (org_id, txn_id)      REFERENCES transactions (org_id, id)       ON DELETE CASCADE,
    FOREIGN KEY (org_id, transfer_id) REFERENCES internal_transfers (org_id, id) ON DELETE CASCADE,

    -- Exactly one out and one in per pair.
    UNIQUE (org_id, transfer_id, side)
);
```

### Row-level security

All three tables take the ordinary shape — `ENABLE`, `FORCE`, one `FOR ALL` policy with `USING`
and `WITH CHECK` written out on `org_id = app_current_org()`. Neither belongs in
`rls-exempt-tables.txt` or `rls-shared-tenant-tables.txt`.

`REVOKE UPDATE ON dedup_skips FROM vekst_app`: a skip is a record of what happened during
one persist, like a validation report. It is written once.

## D1 — The file hash is an index, not an `if`

The obvious implementation reads "does a batch with this hash exist?" and then writes. Two
uploads of the same file, seconds apart, both read nothing and both write.

The partial unique index removes the race and removes the question. The measurement job
still looks first, so the customer gets `already_imported` naming the earlier batch rather
than a constraint violation, but the guarantee does not depend on the job being careful.

Scoped to `org_id`: the same public bank template uploaded by two customers is two files.
Partial on `status = 'imported'`: a file that was rejected for a broken row must be
re-uploadable once the customer fixes it, and an abandoned upload must not block a retry.

## D2 — A skip is not a rejection, and the two must not be conflated

Change 2.3 makes validation blocking and atomic: a bad file persists nothing. This change
makes a duplicate row *skipped*: the rest of the file persists.

These look contradictory and are not. A validation failure means the file's numbers cannot
be trusted. A duplicate means the file's numbers are fine and the system already has some
of them. Conflating them in either direction is bad: rejecting a file because one row was
already imported makes a monthly re-export unusable, and skipping a row that failed a
correctness check silently drops a transaction.

So the order inside persist is: validation has already passed → compute hashes → partition
into `insert` and `skip` → write both in one transaction.

## D3 — The hash has false positives, so a skip is recorded rather than counted

**Rejected: `UNIQUE (org_id, dedup_hash)` on `transactions`.** It is the shortest
implementation of D3 and it is wrong. Two identical payments on one day are ordinary data,
and a unique index would refuse the second one forever, with no way to record that it
existed.

**Rejected: skip and keep only a count.** This is what `ARCHITECTURE.md` §5.3's sentence
suggests — "88 duplicates skipped" — and it is a count of rows that were in the customer's
file and are not in their reports. When the balance check then fails by the amount of two
coffees, nothing can explain it.

**Chosen: skip, and record each skip with its original line number and, for D3, the
transaction it matched.** The count is derived from the table rather than stored. A customer
who disagrees can see exactly which lines were dropped and against what, which is also the
only way the false-positive rate will ever be measured.

The consequence for change 2.3's balance check is worth stating: **the balance check runs
before dedup, over every parsed row.** A skipped duplicate must not break reconciliation,
because the file did contain it. §6.6 is that test.

## D4 — Internal transfers are excluded on detection, not on confirmation

`ARCHITECTURE.md` §5.2 says "propose them", and §5.2's last sentence says "they are excluded
from every P&L line". Those can be read two ways and the readings differ by which error you
prefer:

- **Exclude only when confirmed:** a missed pair counts a transfer between the customer's
  own accounts as revenue. §5.1 names this as the single most likely way this product prints
  a wrong number.
- **Exclude on detection:** a false positive removes a real line from the P&L, and the
  drill-down shows it as excluded.

**Chosen: exclude on detection, reversible by dismissal.** The failure the architecture
names by name is inflated revenue, and an excluded line is visible in a way that a silently
counted transfer is not. D4's "propose, never delete" rule is not the same situation: D4
merges two *records*, which is destructive; this one classifies a pair and changes nothing
about either row.

Flagged for the founder in §0.1, because it is a product-visible choice and the decision
split gives product to him.

## D5 — Pairing must be deterministic

"Opposite signs, equal absolute amount, within ±3 days, different accounts" does not pick a
unique pair when a company makes three equal transfers in a week. Left unspecified, the P&L
would depend on row order.

The rule, in order: same organisation; equal absolute `base_amount_minor`; opposite signs;
different `account_id`; `|booked_on difference| <= 3 days`. Among the candidates, pair the
nearest by date, then the lowest `id`. Take each pair greedily, and skip any candidate either of
whose sides is already paired — which `internal_transfer_members`' primary key enforces
independently of the algorithm.

Matching on `base_amount_minor` rather than `amount_minor`: a transfer between a PLN account
and a EUR account is still a transfer, and the amounts are equal only after conversion.
This makes the rate the pairing depends on, which is why §6.11 pairs across currencies and
asserts the result is stable when the rate changes on a later import.

## `normalize()` has one home

`eval/norm.py` defines `normalize_description` and `counterparty_key` and calls them "pure,
versioned". Change 2.5 stamps `normalize_version` on every row. This change makes the Go
implementation and the Python one provably the same function: one table of cases, checked
into the repository, run by both test suites. A divergence is then a red build rather than
a duplicate count nobody can explain.

## Proto

Three RPCs on the existing `ImportService`, plus one on the report side for transfers. Both
reviewers, per CODEOWNERS. Adding RPCs to an existing service is not breaking; §4.2 confirms
it rather than assuming.

```protobuf
rpc GetDedupSummary(GetDedupSummaryRequest) returns (GetDedupSummaryResponse);
rpc ListSkippedRows(ListSkippedRowsRequest) returns (ListSkippedRowsResponse);
rpc ListInternalTransfers(ListInternalTransfersRequest) returns (ListInternalTransfersResponse);
rpc DismissInternalTransfer(DismissInternalTransferRequest) returns (DismissInternalTransferResponse);

message DedupSummary {
  int32 imported_rows      = 1;
  int32 skipped_in_batch   = 2;  // D2
  int32 skipped_cross_batch = 3; // D3
  int32 internal_transfers = 4;
}
```

No money field: a skipped row's amount is read from the transaction it matched, through the
`Money` message change 2.5's consumers already use.

## What this touches from the invariants list

- **`source_kind`** — D4 is excluded, so nothing here mixes sources. Internal-transfer
  pairing is within one source kind; §6.12 asserts it.
- **Money and currency** — pairing is on `base_amount_minor`. D5.
- **Tenant isolation** — two new tenant tables, ordinary shape.
- **Validation is blocking and atomic** — unchanged, and deliberately distinguished. D2.
- **The classifier contract** — none.

## Rejected alternatives

| Rejected | Why |
| --- | --- |
| Check for a duplicate file in application code | Two concurrent uploads both read nothing and both write. See D1 |
| `UNIQUE (org_id, dedup_hash)` on `transactions` | Two identical payments on one day are real. It would refuse the second forever. See D3 |
| Skip duplicates and store only a count | A count of rows that were in the customer's file and are not in their report, with nothing to inspect. See D3 |
| Reject a file because one row was already imported | Makes a monthly re-export unusable, which is the normal way a customer works. See D2 |
| Exclude internal transfers only after confirmation | The failure is inflated revenue, which §5.1 names as the most likely way this product prints a wrong number. See D4 |
| Reuse `transaction_links` for internal transfers | That table relates two sources describing one event; this relates two events that cancel. One table makes every consumer filter by kind |
| Pair on `amount_minor` | A PLN-to-EUR transfer between the customer's own accounts is still a transfer. See D5 |
| Two implementations of `normalize()`, one per language | A divergence shows up as a duplicate count nobody can explain, months later |

## Risks

| Risk | Mitigation |
| --- | --- |
| A false-positive D3 skip removes a real payment and nobody notices | Each skip keeps its line number and matched row, the balance check runs before dedup, and §6.6 proves a skipped row does not break reconciliation |
| Excluding transfers on detection removes a real revenue line | It is visible as excluded in the drill-down and reversible by dismissal, which is not true of the opposite error. §0.1 puts the choice to the founder |
| Greedy pairing picks differently after a later import adds a nearer candidate | Pairs are written once and a written pair is never re-paired; `internal_transfer_members` holds independently of the algorithm. A new candidate finds both sides taken |
| The Go and Python `normalize()` drift | One checked-in case table, run by both suites. §6.14 |
| `dedup_skips` grows without bound on a customer who re-uploads monthly | It grows with skipped rows, not with imports, and a skipped row is by construction one the system already holds once |
