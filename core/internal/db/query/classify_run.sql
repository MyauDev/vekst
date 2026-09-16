-- The record of classifying one batch. Change 3.4, capability
-- `classification-run`.
--
-- Every statement runs inside db.InTx, which sets the tenant context, and none
-- of them restates the tenant predicate: row-level security already admits this
-- organisation's rows and nothing else. Each takes `org_id` from
-- `app_current_org()` rather than as a parameter, for the same reason -- a
-- parameter is a chance to pass the wrong value, and the transaction already
-- knows the right one.

-- name: InsertClassificationRun :one
-- Opened the moment the job starts, with everything at zero.
--
-- No "does a run already exist" read first. `UNIQUE (org_id, batch_id)` is the
-- guard, and it is the only one that holds when two enqueues of the same batch
-- race: a check-then-insert has a window between the two halves, and the window
-- is exactly where a duplicate job lands. The loser gets 23505 and stops, which
-- is what idempotent means here.
INSERT INTO classification_runs (org_id, batch_id)
VALUES (app_current_org(), $1)
RETURNING *;

-- name: RecordChunkResult :one
-- One chunk's outcome, added to the run in the same transaction that wrote the
-- chunk's classifications.
--
-- Increments rather than assignments: the counters are the sum of what the
-- chunks did, and a worker that computed a running total in Go and wrote it
-- back would lose the arithmetic of any chunk whose transaction rolled back
-- after it. Read-modify-write over a whole-run total would also be a lost
-- update the moment anything else touches the row.
UPDATE classification_runs
   SET chunk_count      = chunk_count + 1,
       classified_count = classified_count + $2,
       review_count     = review_count + $3
 WHERE batch_id = $1 AND status = 'running'
RETURNING *;

-- name: FinishClassificationRun :one
-- The run is over, one way or the other.
--
-- `failure_code` is NULL on success and set on failure, which is what the
-- table's own CHECK requires: a failure names its reason and a success has none.
-- The WHERE clause makes a second finish affect no row, which db.ExactlyOneRow
-- turns into an error rather than a silent success -- a run finished twice
-- would mean the job ran twice, and that is worth hearing about.
UPDATE classification_runs
   SET status       = $2,
       failure_code = $3,
       finished_at  = now()
 WHERE batch_id = $1 AND status = 'running'
RETURNING *;

-- name: GetClassificationRun :one
-- What the import screen reads beside the batch's own status. Absent is a real
-- answer -- a batch that has not been classified yet has no run -- and is not
-- the same as a run that failed.
SELECT * FROM classification_runs WHERE batch_id = $1;
