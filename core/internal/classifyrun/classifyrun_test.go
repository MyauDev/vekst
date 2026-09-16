package classifyrun

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/MyauDev/vekst/core/classify"
	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/normalize"
)

// These run against a live, migrated Postgres reached as vekst_app. Row-level
// security means nothing against a superuser, so there is no in-memory version
// of the isolation these assert.

const (
	taxonomyV = "v1"
	rulesetV  = "v1"

	revenueLeaf = "0101"       // NET SALES
	opexLeaf    = "0401010204" // OPEX, Bank commission
	computedGM  = "91"         // a line, not a leaf: nothing may be classified into it
)

// stubClassifier answers with whatever a test planted, or refuses.
//
// A stub and not the real engine, because what these tests are about is what
// core does with an answer -- stores it, refuses it, retries it -- and the
// engine's own behaviour is tested in the engine's own language, against the
// same fixtures it was ported from.
type stubClassifier struct {
	proposals []classify.Proposal
	engine    string
	ruleset   string

	err error

	// calls counts requests, so a test can say "the batch took three chunks"
	// rather than inferring it.
	calls   int
	lastReq classify.BatchRequest
	perCall func(n int) (classify.BatchResponse, error)
}

func (s *stubClassifier) Version(context.Context) (classify.VersionInfo, error) {
	return classify.VersionInfo{EngineVersion: s.engine}, nil
}

func (s *stubClassifier) Classify(_ context.Context, req classify.BatchRequest) (classify.BatchResponse, error) {
	s.calls++
	s.lastReq = req
	if s.perCall != nil {
		return s.perCall(s.calls)
	}
	if s.err != nil {
		return classify.BatchResponse{}, s.err
	}
	// Answer for every transaction in the chunk that a test planted a proposal
	// for; the rest come back unanswered, which is what below-threshold means
	// on the wire.
	wanted := map[string]classify.Proposal{}
	for _, p := range s.proposals {
		wanted[p.TransactionID] = p
	}
	var out []classify.Proposal
	for _, t := range req.Txns {
		if p, ok := wanted[t.TransactionID]; ok {
			out = append(out, p)
		}
	}
	return classify.BatchResponse{
		EngineVersion:  s.engine,
		RulesetVersion: s.ruleset,
		Proposals:      out,
	}, nil
}

type fixture struct {
	db     *db.DB
	stub   *stubClassifier
	worker *classifyWorker

	org    db.OrgID
	orgID  uuid.UUID
	owner  uuid.UUID
	entity uuid.UUID
	batch  uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping a test that needs a live, migrated Postgres")
	}
	d, err := db.New(context.Background(), db.Config{URL: url})
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(d.Close)

	owner := seedUser(t, d)
	org, err := d.CreateOrganization(context.Background(), db.NewOrganization{
		Name:         "Classify " + uuid.NewString(),
		Country:      "NO",
		BaseCurrency: "NOK",
		EntityName:   "Classify AS",
		CreatorID:    owner,
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	stub := &stubClassifier{engine: "engine-test", ruleset: rulesetV}
	f := &fixture{
		db:    d,
		stub:  stub,
		org:   org,
		orgID: org.UUID(),
		owner: owner,
		worker: &classifyWorker{
			database:   d,
			classifier: stub,
			versions:   Versions{Taxonomy: taxonomyV, Ruleset: rulesetV},
		},
	}
	f.seed(t)
	return f
}

func seedUser(t *testing.T, d *db.DB) uuid.UUID {
	t.Helper()
	var id pgtype.UUID
	err := d.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`,
			uuid.NewString()+"@classify.test").Scan(&id)
	})
	if err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	return uuid.UUID(id.Bytes)
}

func (f *fixture) seed(t *testing.T) {
	t.Helper()
	var e, b pgtype.UUID
	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT id FROM entities LIMIT 1`).Scan(&e); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO accounts (org_id, entity_id, name, currency)
			VALUES (app_current_org(), $1, 'Main', 'NOK')`, e); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			INSERT INTO import_batches (
				org_id, entity_id, source_kind, status, uploaded_by,
				file_name, declared_bytes, declared_type, file_key, upload_expires_at,
				file_sha256, byte_length, content_type)
			VALUES (app_current_org(), $1, 'bank', 'imported', $2,
				'march.csv', 1024, 'text/csv', 'uploads/' || gen_random_uuid(),
				now() + interval '1 hour',
				sha256(gen_random_uuid()::text::bytea), 1024, 'text/csv')
			RETURNING id`, e, pgtype.UUID{Bytes: f.owner, Valid: true}).Scan(&b)
	})
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}
	f.entity, f.batch = uuid.UUID(e.Bytes), uuid.UUID(b.Bytes)
}

// transactions plants n rows in the batch and returns their ids in the order
// the worker will page them.
func (f *fixture) transactions(t *testing.T, n int) []uuid.UUID {
	t.Helper()
	ids := make([]uuid.UUID, 0, n)
	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		for i := range n {
			var id pgtype.UUID
			if err := tx.QueryRow(ctx, `
				INSERT INTO transactions (
					org_id, entity_id, account_id, batch_id, line_no, source_kind,
					booked_on, amount_minor, currency,
					counterparty_raw, counterparty_key, description_raw,
					description_norm, normalize_version, dedup_hash)
				SELECT app_current_org(), $1, a.id, $2, $3, 'bank',
				       date '2026-03-01' + $3::int, $4, 'NOK',
				       'Acme', 'tax:acme', 'Payment', 'payment', $5,
				       gen_random_uuid()::text
				  FROM accounts a WHERE a.entity_id = $1 LIMIT 1
				RETURNING id`,
				pgtype.UUID{Bytes: f.entity, Valid: true},
				pgtype.UUID{Bytes: f.batch, Valid: true},
				int32(i+1), int64((i+1)*100), normalize.Version).Scan(&id); err != nil {
				return fmt.Errorf("row %d: %w", i, err)
			}
			ids = append(ids, uuid.UUID(id.Bytes))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seeding transactions: %v", err)
	}
	return ids
}

func (f *fixture) work(t *testing.T) error {
	t.Helper()
	return f.worker.Work(context.Background(), &river.Job[ClassifyBatchArgs]{
		Args: ClassifyBatchArgs{
			TenantJobArgs: db.TenantJobArgs{OrgID: f.orgID},
			BatchID:       f.batch,
		},
	})
}

func (f *fixture) run(t *testing.T) (Run, bool) {
	t.Helper()
	var out Run
	var ok bool
	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, ok, err = ForBatch(ctx, tx, f.batch)
		return err
	})
	if err != nil {
		t.Fatalf("reading the run: %v", err)
	}
	return out, ok
}

func (f *fixture) liveClassifications(t *testing.T) int {
	t.Helper()
	var n int
	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM classifications
			 WHERE superseded_by IS NULL AND retracted_at IS NULL`).Scan(&n)
	})
	if err != nil {
		t.Fatalf("counting classifications: %v", err)
	}
	return n
}

// proposal is a convenience: the engine answers in codes, never in ids.
func proposal(txnID uuid.UUID, code string) classify.Proposal {
	return classify.Proposal{
		TransactionID: txnID.String(),
		CategoryCode:  code,
		EngineLayer:   "L1",
		Confidence:    0.95,
		Evidence:      "rule:test",
	}
}

// ---------------------------------------------------------------------------
// The happy path, which is the one nothing could do before this change
// ---------------------------------------------------------------------------

func TestABatchIsClassifiedEndToEnd(t *testing.T) {
	f := newFixture(t)
	ids := f.transactions(t, 3)
	f.stub.proposals = []classify.Proposal{
		proposal(ids[0], revenueLeaf),
		proposal(ids[1], opexLeaf),
		proposal(ids[2], opexLeaf),
	}

	if err := f.work(t); err != nil {
		t.Fatalf("Work: %v", err)
	}

	run, ok := f.run(t)
	if !ok {
		t.Fatal("no run was recorded")
	}
	if run.Status != "classified" {
		t.Errorf("status = %q (%s), want classified", run.Status, run.FailureCode)
	}
	if run.ClassifiedCount != 3 || run.ReviewCount != 0 {
		t.Errorf("counts = %d classified / %d review, want 3/0",
			run.ClassifiedCount, run.ReviewCount)
	}
	if run.ChunkCount != 1 {
		t.Errorf("chunk_count = %d, want 1", run.ChunkCount)
	}
	if run.FinishedAt.IsZero() {
		t.Error("a finished run has no finished_at")
	}
	if got := f.liveClassifications(t); got != 3 {
		t.Errorf("%d live classifications, want 3", got)
	}

	// The versions on the rows are the engine's own, not this binary's
	// constants: what answered is the engine's fact to state.
	var engine, ruleset, taxonomy string
	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT DISTINCT engine_version, ruleset_version, taxonomy_version
			  FROM classifications`).Scan(&engine, &ruleset, &taxonomy)
	})
	if err != nil {
		t.Fatalf("reading versions: %v", err)
	}
	if engine != "engine-test" {
		t.Errorf("engine_version = %q, want the one the response carried", engine)
	}
	if taxonomy != taxonomyV {
		t.Errorf("taxonomy_version = %q", taxonomy)
	}

	// And the request carried everything the engine reasons with, because it
	// holds no database of its own.
	req := f.stub.lastReq
	if len(req.Categories) == 0 || len(req.Rules) == 0 {
		t.Errorf("the request carried %d categories and %d rules; the classifier has no "+
			"database and cannot look either up", len(req.Categories), len(req.Rules))
	}
	if req.NormalizeVersion != normalize.Version {
		t.Errorf("normalize_version = %q, want %q", req.NormalizeVersion, normalize.Version)
	}
	// Left unset, so the engine applies its own documented default (D-10).
	// The number lives on the side that computed the confidence it is compared
	// against.
	if req.Threshold != 0 {
		t.Errorf("threshold = %v; core does not hold a second copy of D-10", req.Threshold)
	}
}

// ---------------------------------------------------------------------------
// Task 6.7 and 6.8 -- below threshold, and the counts
// ---------------------------------------------------------------------------

// A transaction the engine did not answer for is not a failure. It is the
// review queue's ordinary input, and the run still finishes.
//
// This is also the test that would hang forever against the query this change
// found: paging used to rely on rows leaving the result as they were
// classified, which is true only if every row gets classified. One unanswered
// row and the worker reads the same page until something kills it.
func TestBelowThresholdIsNotAFailure(t *testing.T) {
	f := newFixture(t)
	ids := f.transactions(t, 4)
	// Two answered, two not.
	f.stub.proposals = []classify.Proposal{
		proposal(ids[0], revenueLeaf),
		proposal(ids[2], opexLeaf),
	}

	if err := f.work(t); err != nil {
		t.Fatalf("Work: %v", err)
	}

	run, _ := f.run(t)
	if run.Status != "classified" {
		t.Errorf("status = %q, want classified: an unanswered row is the queue's input, "+
			"not an error", run.Status)
	}
	if run.ClassifiedCount != 2 || run.ReviewCount != 2 {
		t.Errorf("counts = %d/%d, want 2 classified and 2 for review",
			run.ClassifiedCount, run.ReviewCount)
	}
	// Task 6.8: nothing falls between the two.
	if int(run.ClassifiedCount+run.ReviewCount) != len(ids) {
		t.Errorf("%d + %d does not account for the batch's %d rows",
			run.ClassifiedCount, run.ReviewCount, len(ids))
	}
	if got := f.liveClassifications(t); got != 2 {
		t.Errorf("%d live classifications, want 2", got)
	}
}

// The chunking itself, driven by a batch bigger than one chunk.
//
// ChunkSize is 5,000 in production and a fixture of that size would be absurd,
// so the worker pages three at a time here. What is asserted is that the cursor
// advances: every row is offered exactly once, including the ones no proposal
// came back for -- which is the case the old self-advancing read could not
// survive, because a row nothing answers never leaves the result.
func TestEveryRowIsOfferedExactlyOnceAcrossChunks(t *testing.T) {
	f := newFixture(t)
	f.worker.chunkSize = 3
	ids := f.transactions(t, 7)
	// Nothing is answered, so nothing leaves the result set. A self-advancing
	// read would return the first page forever.
	f.stub.proposals = nil

	seen := map[string]int{}
	f.stub.perCall = func(int) (classify.BatchResponse, error) {
		for _, txn := range f.stub.lastReq.Txns {
			seen[txn.TransactionID]++
		}
		return classify.BatchResponse{
			EngineVersion: "engine-test", RulesetVersion: rulesetV,
		}, nil
	}

	if err := f.work(t); err != nil {
		t.Fatalf("Work: %v", err)
	}

	if len(seen) != len(ids) {
		t.Fatalf("%d distinct rows were offered, want %d", len(seen), len(ids))
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("transaction %s was offered %d times", id, n)
		}
	}
	run, _ := f.run(t)
	if run.ReviewCount != int32(len(ids)) {
		t.Errorf("review_count = %d, want %d", run.ReviewCount, len(ids))
	}
	// Seven rows, three at a time: three chunks, and the third is short.
	if run.ChunkCount != 3 {
		t.Errorf("chunk_count = %d, want 3 -- seven rows paged three at a time",
			run.ChunkCount)
	}
}

// ---------------------------------------------------------------------------
// Tasks 6.4 and 6.6 -- an unknown category rejects the whole response
// ---------------------------------------------------------------------------

// One bad proposal in an otherwise-valid chunk stores nothing from that chunk.
//
// ARCHITECTURE.md §3.5's last row, and the reasoning is worth keeping: a
// response naming a category core never offered is a response core cannot
// reason about. The safe reading is that the other side is answering a
// different question, not that one line of its answer is wrong -- so the run
// fails rather than storing the part that happened to parse.
func TestAnUnknownCategoryRejectsTheWholeChunk(t *testing.T) {
	for name, bad := range map[string]string{
		"a code that does not exist": "no-such-code",
		// '91' is GM: a computed line, excluded from what core sends, because
		// a transaction landing in one would be counted twice -- once where it
		// belongs and once in the formula that already includes it.
		"a computed line": computedGM,
	} {
		t.Run(name, func(t *testing.T) {
			// Its own fixture per case: a run is refused a second time for the
			// same batch, so two cases sharing one would have the second assert
			// nothing.
			f := newFixture(t)
			ids := f.transactions(t, 3)
			f.stub.proposals = []classify.Proposal{
				proposal(ids[0], revenueLeaf), // good
				proposal(ids[1], bad),         // and this one poisons the chunk
			}

			if err := f.work(t); err != nil {
				t.Fatalf("Work returned %v; a refused response is a recorded failure, not a "+
					"retry -- the same request would be refused identically", err)
			}
			run, ok := f.run(t)
			if !ok {
				t.Fatal("no run was recorded")
			}
			if run.Status != "failed" || run.FailureCode != FailureUnknownCategory {
				t.Errorf("run = %q/%q, want failed/%s", run.Status, run.FailureCode,
					FailureUnknownCategory)
			}
			if run.ClassifiedCount != 0 {
				t.Errorf("classified_count = %d, want 0 -- the good proposal in a poisoned "+
					"chunk is not stored either", run.ClassifiedCount)
			}
			if got := f.liveClassifications(t); got != 0 {
				t.Errorf("%d classifications survived a rejected chunk, want 0", got)
			}
		})
	}
}

// A response naming a transaction that was not in the chunk is the same class
// of answer: the other side is not answering the question that was asked.
func TestAProposalForARowNotInTheChunkIsRefused(t *testing.T) {
	f := newFixture(t)
	f.transactions(t, 2)
	f.stub.proposals = []classify.Proposal{proposal(uuid.New(), revenueLeaf)}

	// The stub only echoes proposals for transactions in the request, so plant
	// one directly.
	f.stub.perCall = func(int) (classify.BatchResponse, error) {
		return classify.BatchResponse{
			EngineVersion: "engine-test", RulesetVersion: rulesetV,
			Proposals: []classify.Proposal{proposal(uuid.New(), revenueLeaf)},
		}, nil
	}

	if err := f.work(t); err != nil {
		t.Fatalf("Work: %v", err)
	}
	run, _ := f.run(t)
	if run.Status != "failed" {
		t.Errorf("status = %q, want failed", run.Status)
	}
	if got := f.liveClassifications(t); got != 0 {
		t.Errorf("%d classifications were written, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Task 6.5 -- an unreachable classifier is retryable
// ---------------------------------------------------------------------------

// The run stays `running` and Work returns an error, so River retries with
// backoff. ARCHITECTURE.md §3.5's first row: the user sees "still working",
// because an engine outage is not a failed import.
func TestAnUnreachableClassifierLeavesTheRunRunning(t *testing.T) {
	f := newFixture(t)
	f.transactions(t, 2)
	f.stub.err = status.Error(codes.Unavailable, "connection refused")

	err := f.work(t)
	if err == nil {
		t.Fatal("Work succeeded against an unreachable classifier; River retries on an " +
			"error and swallowing it would mean the batch is never classified")
	}

	run, ok := f.run(t)
	if !ok {
		t.Fatal("no run was recorded")
	}
	if run.Status != "running" {
		t.Errorf("status = %q, want running -- an outage that will pass must not be "+
			"written down as a failure that will not", run.Status)
	}
	if !run.FinishedAt.IsZero() {
		t.Error("a running run claims a finish time")
	}
}

// A refusal is the opposite: the engine answered, and said no. Retrying sends
// the identical request -- which is the property that makes a retry safe, and
// here it is the property that makes one pointless.
func TestARefusedRequestFailsTheRun(t *testing.T) {
	for name, code := range map[string]codes.Code{
		"invalid argument":    codes.InvalidArgument,
		"failed precondition": codes.FailedPrecondition,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.transactions(t, 2)
			f.stub.err = fmt.Errorf("classifier: classify batch: %w",
				status.Error(code, "unsupported_normalize_version"))

			if err := f.work(t); err != nil {
				t.Fatalf("Work returned %v; a refusal is recorded, not retried", err)
			}
			run, _ := f.run(t)
			if run.Status != "failed" || run.FailureCode != FailureRejected {
				t.Errorf("run = %q/%q, want failed/%s", run.Status, run.FailureCode,
					FailureRejected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tasks 6.1 and 6.3 -- isolation, and one run per batch
// ---------------------------------------------------------------------------

func TestOneOrganisationCannotReadAnothersRun(t *testing.T) {
	a := newFixture(t)
	b := newFixture(t)
	a.transactions(t, 1)
	if err := a.work(t); err != nil {
		t.Fatalf("Work: %v", err)
	}

	// B, asking for A's batch under B's own tenant context.
	var found bool
	err := b.db.InTx(context.Background(), b.org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		_, found, err = ForBatch(ctx, tx, a.batch)
		return err
	})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if found {
		t.Error("organisation B read organisation A's classification run")
	}

	// And it is the same answer as a batch that never existed: anything else
	// makes this endpoint confirm the existence of somebody else's identifier.
	err = b.db.InTx(context.Background(), b.org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		_, found, err = ForBatch(ctx, tx, uuid.New())
		return err
	})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if found {
		t.Error("a batch that does not exist has a run")
	}
}

// A second job for the same batch does not classify it twice.
//
// The unique constraint is the guard and not a check-then-insert, because a
// check has a window between its two halves and that window is exactly where a
// duplicate enqueue lands. The loser stops, quietly: the winner is already
// doing the work, and there is nothing to report.
func TestASecondRunForOneBatchIsRefused(t *testing.T) {
	f := newFixture(t)
	ids := f.transactions(t, 2)
	f.stub.proposals = []classify.Proposal{proposal(ids[0], revenueLeaf)}

	if err := f.work(t); err != nil {
		t.Fatalf("first Work: %v", err)
	}
	before := f.liveClassifications(t)

	if err := f.work(t); err != nil {
		t.Fatalf("second Work returned %v; a duplicate enqueue is not an error", err)
	}
	if got := f.liveClassifications(t); got != before {
		t.Errorf("%d classifications after a second run, %d after the first", got, before)
	}

	run, _ := f.run(t)
	if run.ChunkCount != 1 {
		t.Errorf("chunk_count = %d after two jobs, want 1 -- the second did not run", run.ChunkCount)
	}
	if f.stub.calls != 1 {
		t.Errorf("the classifier was called %d times, want 1", f.stub.calls)
	}
}

// ---------------------------------------------------------------------------
// Task 6.2 -- fail-closed
// ---------------------------------------------------------------------------

// Every statement in classify_run.sql raises 42704 outside a tenant
// transaction, rather than returning nothing.
//
// The distinction is the whole of the invariant: a query with no tenant context
// that returned zero rows would let a screen render "this batch was never
// classified" for a batch that was, and nothing anywhere would say why.
func TestTheRunQueriesAreFailClosed(t *testing.T) {
	f := newFixture(t)
	batchID := pgtype.UUID{Bytes: f.batch, Valid: true}

	// One transaction per statement, deliberately. The first 42704 aborts the
	// transaction it was raised in, and every statement after it in the same
	// one fails with 25P02 instead -- which would make three of these four
	// assertions pass or fail on map iteration order rather than on what they
	// are about.
	for name, call := range map[string]func(context.Context, *gendb.Queries) error{
		"InsertClassificationRun": func(ctx context.Context, q *gendb.Queries) error {
			_, err := q.InsertClassificationRun(ctx, batchID)
			return err
		},
		"GetClassificationRun": func(ctx context.Context, q *gendb.Queries) error {
			_, err := q.GetClassificationRun(ctx, batchID)
			return err
		},
		"RecordChunkResult": func(ctx context.Context, q *gendb.Queries) error {
			_, err := q.RecordChunkResult(ctx, gendb.RecordChunkResultParams{BatchID: batchID})
			return err
		},
		"FinishClassificationRun": func(ctx context.Context, q *gendb.Queries) error {
			_, err := q.FinishClassificationRun(ctx, gendb.FinishClassificationRunParams{
				BatchID: batchID, Status: "classified",
			})
			return err
		},
	} {
		var got string
		_ = f.db.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
			got = pgCode(call(ctx, gendb.New(tx)))
			return nil
		})
		if got != "42704" {
			t.Errorf("%s outside a tenant transaction raised %q, want 42704: a read "+
				"with no tenant context must fail, never quietly return nothing", name, got)
		}
	}
}

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		if errors.Is(err, pgx.ErrNoRows) {
			return "no rows"
		}
		if err == nil {
			return "no error"
		}
		return err.Error()
	}
	return pgErr.Code
}
