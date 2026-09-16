-- What happened when a batch was classified. Change 3.4, capability
-- `classification-run`.
--
-- One row per import batch, written only by the job that classifies it. It is
-- not a second state machine for the batch: `import_batches.status` says how
-- far the file got through ingest and reaches `imported` as its terminal state,
-- and classification is a different question asked of a batch that already
-- arrived.
--
-- Widening that CHECK again -- the way 2.1, 2.3 and 2.4 each did -- would make
-- every future Track A change that touches `import_batches` reason about Track
-- B's states as well, and the constraints already on that table
-- (`import_batches_measured_past_upload` and its siblings) would each have to
-- grow a matching case. A separate table costs one join on one screen.
--
-- Three statuses, not eleven. Nothing outside this job's own retry loop needs
-- to distinguish "chunk 2 of 5" from "chunk 3 of 5", and a column that can say
-- so is a column somebody will eventually read as progress.

-- +goose Up

CREATE TABLE classification_runs (
    org_id   uuid NOT NULL,
    id       uuid NOT NULL DEFAULT gen_random_uuid(),
    batch_id uuid NOT NULL,

    status       text NOT NULL DEFAULT 'running'
                 CHECK (status IN ('running', 'classified', 'failed')),

    -- A code, never a sentence: the backend returns codes and translation is
    -- the client's (CLAUDE.md, Conventions).
    failure_code text NULL,

    -- What the run did, counted as it goes rather than derived afterwards. A
    -- count derived at read time would be a second answer to a question the
    -- rows already answer, and the two would disagree the first time a
    -- classification was superseded.
    chunk_count      integer NOT NULL DEFAULT 0 CHECK (chunk_count >= 0),
    classified_count integer NOT NULL DEFAULT 0 CHECK (classified_count >= 0),

    -- Below the organisation's threshold: the engine answered, and not
    -- confidently enough to write down. Not a failure -- it is the review
    -- queue's ordinary input -- and counted here only so the import screen can
    -- say "820 of 1,294 classified" without running the queue's own query.
    review_count     integer NOT NULL DEFAULT 0 CHECK (review_count >= 0),

    started_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz NULL,

    PRIMARY KEY (org_id, id),
    FOREIGN KEY (org_id, batch_id) REFERENCES import_batches (org_id, id) ON DELETE RESTRICT,

    -- One run per batch. Re-running after an engine upgrade is a real thing a
    -- customer will want and it is not designed yet: a second run would write a
    -- second, disagreeing set of classifications over the first, and which one
    -- a report should pin is exactly the question nobody has answered. This
    -- constraint makes that a decision somebody has to take deliberately rather
    -- than an accident a duplicate enqueue produces.
    --
    -- It is also the idempotence guard for the enqueue itself: a job that runs
    -- twice loses the race here instead of classifying everything twice.
    UNIQUE (org_id, batch_id),

    -- A run that is over says when, and a run that is not says nothing. Three
    -- of the four combinations mean something and this refuses the fourth.
    CONSTRAINT classification_run_finished_is_whole CHECK (
        (status = 'running') = (finished_at IS NULL)),

    -- A failure names its reason, and a success does not have one.
    CONSTRAINT classification_run_failure_is_whole CHECK (
        (failure_code IS NOT NULL) = (status = 'failed'))
);

-- The import screen's read: one batch, one run.
CREATE INDEX classification_runs_batch_idx ON classification_runs (org_id, batch_id);

-- An ordinary tenant table: no shared rows, no split read/write policy. A run
-- belongs to the organisation whose file it classified and to nobody else.
ALTER TABLE classification_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE classification_runs FORCE  ROW LEVEL SECURITY;

CREATE POLICY classification_runs_tenant ON classification_runs FOR ALL
    USING      (org_id = app_current_org())
    WITH CHECK (org_id = app_current_org());

-- 00001's default privileges already grant vekst_app SELECT, INSERT, UPDATE and
-- DELETE here. DELETE is one right too many: a run is a record of what the
-- system did to a customer's data, and the only reason to remove one is to
-- re-run, which the unique constraint above deliberately does not allow yet.
REVOKE DELETE ON classification_runs FROM vekst_app;

-- +goose Down

DROP POLICY classification_runs_tenant ON classification_runs;
DROP TABLE classification_runs;
