**Ordering.** Apply after `add-transaction-ledger` (2.5), `add-classification-engine`
(3.2) and `add-dedup` (2.6) — nothing reaches `imported` before that last one exists.

**Ownership.** Track B, except the migration and `import_batches`-adjacent wiring in §4,
which touches Track A's own state machine and needs both reviewers.

## 1. Migration 016 — `classification_runs`

- [x] 1.1 `classification_runs`: `org_id`-leading primary key, composite FK to
  `import_batches`, `UNIQUE (org_id, batch_id)`, `status` CHECK admitting
  `running`/`classified`/`failed`
- [x] 1.2 Enable and `FORCE ROW LEVEL SECURITY`; one ordinary tenant policy
- [x] 1.3 Migration down; confirm `up → down → up` against a scratch database
- [x] 1.4 Bump `assertTableCount` and `RequiredVersion`, plus two CHECKs the design did not name: a run that is over says when, and a failure names its reason

## 2. Generated queries — `core/internal/db/query/classify_run.sql`

- [x] 2.1 `InsertClassificationRun` (`running`, zero counters) — the unique constraint is
  what refuses a second run, not an application-level check first
- [x] 2.2 `RecordChunkResult`: increment `chunk_count`/`classified_count`/`review_count`
  in the same statement a chunk's classifications are written
- [x] 2.3 `FinishClassificationRun`: set `classified` or `failed` plus `finished_at`
- [x] 2.4 `GetClassificationRun` by `batch_id`
- [x] 2.5 Run `make gen`; confirm the codegen drift job stays green

## 3. The worker

- [x] 3.1 `ClassifyBatchArgs{db.TenantJobArgs; BatchID}`, River job, registered the same
  way `add-file-upload`'s measurement/expiry jobs are
- [x] 3.2 `Work`: read the batch's organisation and threshold, page
  `ledger.UnclassifiedTransactions` at 5,000 per chunk, build a `classify.BatchRequest`
  via `ledger.ToClassifyBatch` plus this organisation's categories/rules/vendors
- [x] 3.3 Per chunk: call `Classifier`, reject the chunk if any `category_id` does not
  resolve, store each proposal via `ledger.InsertClassification`, update the run's
  counters — commit together or not at all. **The call is outside the transaction and the
  write inside it**, a deliberate departure from this task's literal wording: one
  `db.InTx` containing the call would hold a database transaction open across a network
  round trip of up to sixty seconds per chunk, which is how a pool runs out of connections
  and how autovacuum stops being able to clean up behind an import. The property being
  protected is atomicity of the *write*, and splitting keeps it. What splitting adds is a
  window where somebody else classifies a row already read — and that is safe: the insert
  violates `classifications_one_live_per_transaction`, the chunk rolls back whole, and the
  retry re-reads a page the row has left
- [x] 3.3b The engine already applies the threshold, so core stores every proposal it is
  sent. D-10's number stays on the side that computed the confidence it is compared
  against, and `Threshold` travels unset until an organisation can choose its own
- [x] 3.4 Retry with backoff on an unreachable classifier (River's own retry policy);
  `FinishClassificationRun('failed', ...)` on a chunk error, never a partial run
- [x] 3.5 `FinishClassificationRun('classified', ...)` once every chunk succeeds

## 4. Wiring — Track A + Track B, both reviewers

- [x] 4.1 Enqueue `ClassifyBatchArgs` at the point a batch's status becomes `imported`
  — **in the same transaction**, not after it. A job inserted after the commit is one a
  crash in between loses, and a batch that is imported and never classified looks finished
  while showing a business with no revenue. River's insert is a row in the same database,
  so making the two facts one costs nothing
- [x] 4.2 `InsertClassificationRun`'s own unique constraint is the guard against a second
  enqueue landing twice; confirm the job is idempotent against a duplicate enqueue

## 5. Surfacing the outcome

- [x] 5.1 `ClassificationRun` on `ImportBatch`, populated by `GetImportBatch` and
  deliberately **not** by the list. The list answers "how did my uploads go", which is
  `import_batches.status`; one join per row to answer a question the list does not ask is
  a cost the screen that opens a batch can pay instead. If the Imports list turns out to
  need it, that is one LEFT JOIN in Track A's own query and a conversation with the change
  that owns the screen — which is what this task asked for, and it is reversible either
  way. Absent is a real answer: a batch that has just landed has no run, which a screen
  renders differently from a run that failed
- [x] 5.2 `buf breaking` if §5.1 touches `/proto`

## 6. Tests

- [x] 6.1 **Cross-tenant isolation:** A cannot read B's classification run, and the
  result is indistinguishable from no run existing
- [x] 6.2 **Fail-closed:** every query in `classify_run.sql` raises `42704` outside a
  tenant transaction
- [x] 6.3 **One run per batch:** a second `InsertClassificationRun` for the same batch is
  refused
- [x] 6.4 **Atomic chunk:** a response carrying one good proposal and one poisoned one
  stores neither, and the run's counters stay at zero. The failure is induced before the
  write rather than during it, because no input makes a correct implementation fail
  halfway through one `InTx` — what is left to Postgres is Postgres's, and what this
  asserts is that the whole response is refused rather than the part of it that parsed
- [x] 6.5 **Unreachable classifier retries:** a `Work` call against an unreachable
  classifier leaves the run `running`, not `failed`
- [x] 6.6 **Unknown category is rejected wholesale:** one bad `category_id` in an
  otherwise-valid chunk response stores nothing from that chunk
- [x] 6.7 **Below threshold is not a failure:** a low-confidence proposal leaves its
  transaction unclassified and the run still reaches `classified`
- [x] 6.8 **A completed run's counts are exact:** classified + review counts equal the
  batch's total transaction count

## 7. Close

- [x] 7.1 Add `/core/internal/db/query/classify_run.sql` and the new worker package to
  `CODEOWNERS`
- [x] 7.2 Update `docs/IMPLEMENTATION_PLAN.md` §3 and `ARCHITECTURE.md` §3.5/§5.5 with
  what landed, including the two rows of §3.5's table that needed saying more precisely:
  how a refusal is told apart from an outage, and what "chunked" needs from the query
  underneath it
- [x] 7.3 Update the capability spec and run the full suite

## 8. What this change found in code it only had to call

- [x] 8.1 **`UnclassifiedTransactions` could not terminate.** It paged by relying on rows
  leaving the result as they were classified — true only if every row gets classified, and
  a threshold exists precisely so that some do not. One below-threshold row and the worker
  reads the same page until something kills it. It pages by `(booked_on, id)` now
- [x] 8.2 **It was not scoped to a batch.** Two imports running at once would classify
  each other's rows and each count them as its own
- [x] 8.3 **It did not know about `retracted_at`.** Migration 007 grew a second way for a
  classification to stop being live and this join still knew only about supersession, so a
  row whose only answer had been withdrawn was invisible to the worker while the review
  queue and every report counted it as unanswered — stuck, permanently, in the one state
  nothing would act on
- [x] 8.4 **`classify.IsRejected`.** Retryable and not-retryable were a distinction the
  engine's own service layer already drew, in a comment that names River, with nothing on
  this side reading it
