## Context

Two things already exist and this change sits between them. `core/internal/ledger`
(change 2.5) can read a page of transactions with no live classification
(`UnclassifiedTransactions`) and can store one (`InsertClassification`). `core/classify`
(change 3.2) has a `Classifier` interface that turns transactions into proposals. Nothing
calls the second with the first's output — this change is that call, as a River job, plus
where its outcome is recorded.

**Stack order.** This delta is written against `add-classification-engine` and
`add-transaction-ledger`, both complete but unarchived (`openspec list` shows neither
merged into `openspec/specs/`), the same stacking `add-import-profiles` documented first.
It must also apply after `add-dedup` (2.6): nothing reaches `import_batches.status =
'imported'` — this change's own trigger — until that change's persistence step exists.
This change reads none of `add-dedup`'s own tables, only the ordering matters.

## Goals / Non-Goals

**Goals:** a job that classifies one batch's transactions, chunked and retried per
ARCHITECTURE.md §3.5, atomically per chunk, with an outcome a screen can read.

**Non-Goals:** the review queue screen, any change to `Classifier`, re-classification
after an upgrade, report computation. Named in the proposal.

## The data model

```sql
CREATE TABLE classification_runs (
    org_id           uuid        NOT NULL,
    id               uuid        NOT NULL DEFAULT gen_random_uuid(),
    batch_id         uuid        NOT NULL,

    status           text        NOT NULL DEFAULT 'running'
                                  CHECK (status IN ('running', 'classified', 'failed')),
    failure_code     text        NULL,

    chunk_count      integer     NOT NULL DEFAULT 0,
    classified_count integer     NOT NULL DEFAULT 0,
    review_count     integer     NOT NULL DEFAULT 0,

    started_at       timestamptz NOT NULL DEFAULT now(),
    finished_at      timestamptz NULL,

    PRIMARY KEY (org_id, id),
    FOREIGN KEY (org_id, batch_id) REFERENCES import_batches (org_id, id) ON DELETE RESTRICT,
    UNIQUE (org_id, batch_id)
);

ALTER TABLE classification_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE classification_runs FORCE  ROW LEVEL SECURITY;
CREATE POLICY classification_runs_tenant ON classification_runs FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());
```

`UNIQUE (org_id, batch_id)`: one run per batch. A re-run (after an upgrade, or a retry of a
`failed` run) is out of scope (see proposal) and this constraint is what makes it a
decision to design later rather than an accident today.

## Decisions

### D1 — A dedicated run row, not a wider `import_batches.status`

**Rejected: widen `import_batches`'s status CHECK again**, the way 2.1 → 2.3 → 2.4 each
did. `imported` is that state machine's own terminal state (design D3, `add-file-upload`);
treating classification as one more transition on the same column means every future
Track A change that touches `import_batches` has to reason about Track B's states too, and
`import_batches_measured_past_upload`-style constraints would need to grow a matching case.

**Chosen:** a new table, one row per batch, that only this job writes. `status` here is
`running | classified | failed` — three values, not eleven, because nothing outside this
job's own retry loop needs to distinguish "chunk 2 of 5" from "chunk 3 of 5".

### D2 — Chunking, retry and rejection are ARCHITECTURE.md §3.5, unchanged

5,000 transactions per chunk, 60s timeout, retry-with-backoff on an unreachable classifier,
atomic failure on an error, and reject-the-whole-response on an unknown `category_id`. This
change wires the table those rules already describe; it does not renegotiate them.

**What "atomic per chunk" means precisely:** one `db.InTx` per chunk writes every
classification and updates `classification_runs.classified_count`/`review_count`
together. A chunk that errors after some proposals were computed writes none of them —
the next attempt (River's own retry) re-reads `UnclassifiedTransactions` and gets the same
page back, because nothing in it was classified yet.

### D3 — Below threshold is not a failure

A proposal below the organisation's threshold is not an error and is not retried: the
transaction is left with no live classification, which is exactly what
`UnclassifiedTransactions` already means by "needs attention" — the review queue (a later
change) reads the same query this job does. `review_count` on the run row is a count for
the batch's own screen, not a second queue.

### D4 — One job per batch, not one job per chunk

River enqueues one `ClassifyBatchArgs{BatchID}` job when a batch reaches `imported`; that
job's own `Work` loops over chunks internally rather than enqueueing a job per chunk.
Chunk count for a Demo-sized import (thousands, not millions, of rows) is small enough that
per-chunk job overhead would cost more in bookkeeping (one `classification_runs` row update
per chunk's own job completion, ordering between chunks) than it saves. A batch large
enough to need chunk-level retry independent of its siblings is a scale problem for a later
change, not this one.

## Rejected alternatives

| Rejected | Why |
| --- | --- |
| Widen `import_batches.status` instead of a new table (D1) | Couples every future Track A change touching that column to Track B's states too |
| One River job per chunk instead of one per batch (D4) | Chunk counts at Demo scale do not justify the extra bookkeeping between siblings |
| Treat below-threshold as a failure (D3) | It is the review queue's normal input, not an error condition |
| A `current` boolean or a second "needs review" table | `UnclassifiedTransactions` already answers "needs attention" by construction (no live classification); a second flag is a second place that answer can disagree with itself |

## Risks / Trade-offs

| Risk | Mitigation |
| --- | --- |
| A batch never reaches `imported` because `add-dedup` is not yet implemented | This change cannot ship before 2.6 does; the ordering is stated in the proposal, not discovered at apply time |
| `classification_runs` and `import_batches.status` can drift — a batch shows `imported` while its run shows `failed` forever | Deliberate: the run row is additional information, not a duplicate state machine. The import screen reads both |
| Re-running a `failed` run has no defined behaviour yet | `UNIQUE (org_id, batch_id)` refuses a second row rather than silently allowing a duplicate that would produce a second, disagreeing set of classifications |
