**Ordering.** Apply after `add-transaction-ledger` (2.5), `add-classification-engine`
(3.2) and `add-dedup` (2.6) — nothing reaches `imported` before that last one exists.

**Ownership.** Track B, except the migration and `import_batches`-adjacent wiring in §4,
which touches Track A's own state machine and needs both reviewers.

## 1. Migration — `classification_runs`

- [ ] 1.1 `classification_runs`: `org_id`-leading primary key, composite FK to
  `import_batches`, `UNIQUE (org_id, batch_id)`, `status` CHECK admitting
  `running`/`classified`/`failed`
- [ ] 1.2 Enable and `FORCE ROW LEVEL SECURITY`; one ordinary tenant policy
- [ ] 1.3 Migration down; confirm `up → down → up` against a scratch database
- [ ] 1.4 Bump `assertTableCount` and `RequiredVersion`

## 2. Generated queries — `core/internal/db/query/classify_run.sql`

- [ ] 2.1 `InsertClassificationRun` (`running`, zero counters) — the unique constraint is
  what refuses a second run, not an application-level check first
- [ ] 2.2 `RecordChunkResult`: increment `chunk_count`/`classified_count`/`review_count`
  in the same statement a chunk's classifications are written
- [ ] 2.3 `FinishClassificationRun`: set `classified` or `failed` plus `finished_at`
- [ ] 2.4 `GetClassificationRun` by `batch_id`
- [ ] 2.5 Run `make gen`; confirm the codegen drift job stays green

## 3. The worker

- [ ] 3.1 `ClassifyBatchArgs{db.TenantJobArgs; BatchID}`, River job, registered the same
  way `add-file-upload`'s measurement/expiry jobs are
- [ ] 3.2 `Work`: read the batch's organisation and threshold, page
  `ledger.UnclassifiedTransactions` at 5,000 per chunk, build a `classify.BatchRequest`
  via `ledger.ToClassifyBatch` plus this organisation's categories/rules/vendors
- [ ] 3.3 Per chunk, one `db.InTx`: call `Classifier`, reject the chunk if any
  `category_id` does not resolve, store each at-or-above-threshold proposal via
  `ledger.InsertClassification`, update the run's counters — commit together or not at
  all
- [ ] 3.4 Retry with backoff on an unreachable classifier (River's own retry policy);
  `FinishClassificationRun('failed', ...)` on a chunk error, never a partial run
- [ ] 3.5 `FinishClassificationRun('classified', ...)` once every chunk succeeds

## 4. Wiring — Track A + Track B, both reviewers

- [ ] 4.1 Enqueue `ClassifyBatchArgs` at the point a batch's status becomes `imported`
  (in `add-dedup`'s own persistence code, or immediately after it, per that change's own
  final task)
- [ ] 4.2 `InsertClassificationRun`'s own unique constraint is the guard against a second
  enqueue landing twice; confirm the job is idempotent against a duplicate enqueue

## 5. Surfacing the outcome

- [ ] 5.1 `optional ClassificationRunStatus classification_run` on `ImportBatch` (or a
  `GetClassificationRun` RPC) — whichever `add-web-experience`'s Imports screen (task
  6.x there) turns out to need; decide with that change's author before both reviewers
  sign off
- [ ] 5.2 `buf breaking` if §5.1 touches `/proto`

## 6. Tests

- [ ] 6.1 **Cross-tenant isolation:** A cannot read B's classification run, and the
  result is indistinguishable from no run existing
- [ ] 6.2 **Fail-closed:** every query in `classify_run.sql` raises `42704` outside a
  tenant transaction
- [ ] 6.3 **One run per batch:** a second `InsertClassificationRun` for the same batch is
  refused
- [ ] 6.4 **Atomic chunk:** a chunk that errors after computing some proposals leaves
  every transaction in it unclassified, and the run's counters unchanged
- [ ] 6.5 **Unreachable classifier retries:** a `Work` call against an unreachable
  classifier leaves the run `running`, not `failed`
- [ ] 6.6 **Unknown category is rejected wholesale:** one bad `category_id` in an
  otherwise-valid chunk response stores nothing from that chunk
- [ ] 6.7 **Below threshold is not a failure:** a low-confidence proposal leaves its
  transaction unclassified and the run still reaches `classified`
- [ ] 6.8 **A completed run's counts are exact:** classified + review counts equal the
  batch's total transaction count

## 7. Close

- [ ] 7.1 Add `/core/internal/db/query/classify_run.sql` and the new worker package to
  `CODEOWNERS`
- [ ] 7.2 Update `docs/IMPLEMENTATION_PLAN.md` §3 and `ARCHITECTURE.md` §3.5/§5.5 with
  what landed
- [ ] 7.3 Update the capability spec and run the full suite
