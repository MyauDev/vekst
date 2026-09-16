// Package classifyrun is the call nothing was making.
//
// `core/internal/ledger` can read a page of transactions with no live
// classification and can store one. `core/classify` can turn transactions into
// proposals. Between those two there was nothing, so every row that landed in
// `transactions` stayed unclassified forever: the review queue showed the whole
// import and the report showed one bucket where a business should be.
//
// This package is that call, as a River job -- one per import batch, enqueued
// the moment the batch reaches `imported`, chunked and retried per
// ARCHITECTURE.md §3.5, atomic per chunk, with an outcome a screen can read.
//
// The engine is a separate process and is treated like one. It holds no
// database credentials, so everything it reasons with -- the categories, the
// rules, the vendor memory -- is read here and travels in the request; and
// because the request carries everything the answer depends on, asking twice
// gives the same answer, which is what makes a retried job safe.
package classifyrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/MyauDev/vekst/core/classify"
	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/ledger"
	"github.com/MyauDev/vekst/core/internal/money"
	"github.com/MyauDev/vekst/core/internal/normalize"
)

// ARCHITECTURE.md §3.5, unchanged: this change wires the table those rules
// already describe, it does not renegotiate them.
const (
	// ChunkSize is how many transactions travel in one request.
	ChunkSize = 5_000

	// ChunkTimeout bounds one call. A classifier that is not answering is a
	// retryable condition, not a reason to hold a worker open.
	ChunkTimeout = 60 * time.Second
)

// Failure codes recorded on a run. Codes, never sentences: translation is the
// client's (CLAUDE.md, Conventions).
const (
	// FailureUnknownCategory is the other side naming a category core did not
	// send it. The whole response is rejected rather than the one proposal,
	// because a response containing an id core never offered is a response
	// core cannot reason about at all.
	FailureUnknownCategory = "classify_unknown_category"

	// FailureBadMatcher is a rule this process cannot read. Skipping it would
	// be worse than failing: a rule that loses a condition does not stop
	// matching, it starts matching everything the remaining conditions allow.
	FailureBadMatcher = "classify_unreadable_rule"

	// FailureRejected is the engine refusing the request -- an unsupported
	// normalisation version, a rule pointing at a category the request did not
	// carry. Retrying sends the identical request and gets the identical
	// refusal, so the run fails rather than looping.
	FailureRejected = "classify_request_rejected"
)

// Versions pins what a classification written by this job records. A report
// reproduces only if every input to it is stored beside the output.
//
// Taxonomy and ruleset are what core asked the engine to reason with; engine
// and ruleset come back on the response, because what answered is the engine's
// fact to state and not this process's to assume.
type Versions struct {
	Taxonomy string
	Ruleset  string
}

// ClassifyBatchArgs names one batch and the organisation that owns it.
//
// The organisation travels in the arguments and is read back through
// db.OrgIDFromJobArgs, never from ambient state: River's tables carry no
// org_id and no policy, so a job payload is untrusted input for tenancy
// purposes the same way an HTTP request is.
type ClassifyBatchArgs struct {
	db.TenantJobArgs
	BatchID uuid.UUID `json:"batch_id"`
}

// Kind satisfies river.JobArgs.
func (ClassifyBatchArgs) Kind() string { return "classify_batch" }

// Workers owns this package's River workers.
type Workers struct {
	database   *db.DB
	classifier classify.Classifier
	versions   Versions
}

// NewWorkers builds the registry entry.
func NewWorkers(database *db.DB, classifier classify.Classifier, versions Versions) *Workers {
	return &Workers{database: database, classifier: classifier, versions: versions}
}

// Register satisfies core/internal/jobs.WorkerRegistrar.
func (w *Workers) Register(rw *river.Workers) []*river.PeriodicJob {
	river.AddWorker(rw, &classifyWorker{
		database:   w.database,
		classifier: w.classifier,
		versions:   w.versions,
		chunkSize:  ChunkSize,
	})
	// No periodic schedule. Every job here is enqueued for a named batch by
	// the job that imported it; a sweep would have no tenant context of its
	// own to run under.
	return nil
}

type classifyWorker struct {
	river.WorkerDefaults[ClassifyBatchArgs]
	database   *db.DB
	classifier classify.Classifier
	versions   Versions

	// chunkSize is ChunkSize everywhere but in this package's own tests, which
	// set it small so that paging across chunks is exercised by seven rows
	// rather than by fifteen thousand. The cursor is the thing being tested and
	// it does not care how big a page is; a fixture that could only reach the
	// second page by seeding a production-sized batch would be a test nobody
	// runs.
	chunkSize int32
}

// Work classifies one batch.
//
// The shape is: open a run, read what the engine needs once, then page the
// batch's unclassified rows and, for each page, ask and write.
//
// **The ask happens outside a transaction, and the write inside one.** The
// change's own design says one `db.InTx` per chunk containing the call -- which
// would hold a database transaction open across a network round trip of up to
// sixty seconds, for every chunk, which is how a pool runs out of connections
// and how autovacuum stops being able to clean up behind the import. The
// property that design was protecting is atomicity of the *write*, and
// splitting keeps it: the write is still one transaction that commits every
// classification of a chunk together or none of them.
//
// What splitting adds is a window where somebody else could classify a row
// this job has already read. That is safe and worth stating: the insert would
// violate `classifications_one_live_per_transaction`, the whole chunk's
// transaction rolls back, River retries, the page is re-read and the row is
// gone from it. No partial write, no double classification.
func (w *classifyWorker) Work(ctx context.Context, job *river.Job[ClassifyBatchArgs]) error {
	org, err := db.OrgIDFromJobArgs(job.Args.TenantJobArgs)
	if err != nil {
		return fmt.Errorf("classifyrun: %w", err)
	}
	batchID := pgtype.UUID{Bytes: job.Args.BatchID, Valid: true}

	started, err := w.open(ctx, org, batchID)
	if err != nil {
		return err
	}
	if !started {
		// Another job already opened this batch's run. Not an error: a
		// duplicate enqueue losing the race is what the unique constraint is
		// for, and the winner is already doing the work.
		return nil
	}

	req, err := w.requestContext(ctx, org)
	if err != nil {
		var fatal *fatalError
		if errors.As(err, &fatal) {
			return w.fail(ctx, org, batchID, fatal.code)
		}
		return err
	}

	var cursor ledger.Cursor
	for {
		page, err := w.page(ctx, org, batchID, cursor)
		if err != nil {
			return err
		}
		if len(page) == 0 {
			break
		}

		// Outside any transaction, and bounded. An engine that is not
		// answering is retryable: returning the error leaves the run `running`
		// and River tries again with backoff, which is ARCHITECTURE.md §3.5's
		// first row.
		callCtx, cancel := context.WithTimeout(ctx, ChunkTimeout)
		resp, err := w.classifier.Classify(callCtx, chunkRequest(req, w.versions, page))
		cancel()
		if err != nil {
			if classify.IsRejected(err) {
				// The engine refused the request itself. A retry sends the
				// identical request -- the whole point of the request carrying
				// everything -- and gets the identical refusal.
				return w.fail(ctx, org, batchID, FailureRejected)
			}
			return fmt.Errorf("classifyrun: classifying a chunk of %d: %w", len(page), err)
		}

		if err := w.writeChunk(ctx, org, batchID, req, page, resp); err != nil {
			var fatal *fatalError
			if errors.As(err, &fatal) {
				return w.fail(ctx, org, batchID, fatal.code)
			}
			return err
		}

		last := page[len(page)-1]
		cursor = ledger.Cursor{BookedOn: last.BookedOn, ID: last.ID}
	}

	return w.finish(ctx, org, batchID)
}

// open starts the run, and reports whether this job is the one doing the work.
func (w *classifyWorker) open(ctx context.Context, org db.OrgID, batchID pgtype.UUID) (bool, error) {
	err := w.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := gendb.New(tx).InsertClassificationRun(ctx, batchID)
		return err
	})
	switch {
	case err == nil:
		return true, nil
	case isUniqueViolation(err, "classification_runs_org_id_batch_id_key"):
		return false, nil
	default:
		return false, fmt.Errorf("classifyrun: opening the run: %w", err)
	}
}

// batchContext is everything the engine reasons with, read once per run.
//
// Read once rather than per chunk because it does not change while a run is in
// flight, and because two chunks reasoning with different rule sets would
// produce a batch whose classifications disagree with each other for no reason
// a report could explain.
type batchContext struct {
	categories []classify.Category
	rules      []classify.Rule
	vendors    []classify.VendorMemory

	// codes is what turns a proposal back into a row: the engine answers in
	// codes, because an id is meaningless to a stateless service and would
	// invite it to hold one.
	codes map[string]pgtype.UUID
}

func (w *classifyWorker) requestContext(ctx context.Context, org db.OrgID) (batchContext, error) {
	var out batchContext
	err := w.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)

		cats, err := q.ClassifiableCategories(ctx, w.versions.Taxonomy)
		if err != nil {
			return fmt.Errorf("reading the categories: %w", err)
		}
		out.codes = make(map[string]pgtype.UUID, len(cats))
		for _, c := range cats {
			out.categories = append(out.categories, classify.Category{
				Code:               c.Code,
				Name:               c.Name,
				RequiresAllocation: c.RequiresAllocation,
			})
			out.codes[c.Code] = c.ID
		}

		rules, err := q.EffectiveRules(ctx, gendb.EffectiveRulesParams{
			TaxonomyVersion: w.versions.Taxonomy,
			RulesetVersion:  w.versions.Ruleset,
		})
		if err != nil {
			return fmt.Errorf("reading the rules: %w", err)
		}
		for _, r := range rules {
			rule, err := toRule(r)
			if err != nil {
				return &fatalError{code: FailureBadMatcher, err: err}
			}
			out.rules = append(out.rules, rule)
		}

		vendors, err := q.VendorMemory(ctx, normalize.Version)
		if err != nil {
			return fmt.Errorf("reading the vendor memory: %w", err)
		}
		for _, v := range vendors {
			out.vendors = append(out.vendors, classify.VendorMemory{
				Key:          v.Key,
				CategoryCode: v.CategoryCode,
				DisplayName:  v.DisplayName,
			})
		}
		return nil
	})
	if err != nil {
		var fatal *fatalError
		if errors.As(err, &fatal) {
			return batchContext{}, fatal
		}
		return batchContext{}, fmt.Errorf("classifyrun: assembling the request: %w", err)
	}
	return out, nil
}

func (w *classifyWorker) page(
	ctx context.Context, org db.OrgID, batchID pgtype.UUID, cursor ledger.Cursor,
) ([]ledger.Transaction, error) {
	var page []ledger.Transaction
	err := w.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		size := w.chunkSize
		if size <= 0 {
			size = ChunkSize
		}
		page, err = ledger.UnclassifiedTransactions(ctx, tx,
			uuid.UUID(batchID.Bytes), cursor, size)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("classifyrun: reading a page: %w", err)
	}
	return page, nil
}

// writeChunk stores one chunk's answers and its counters, together or not at
// all.
func (w *classifyWorker) writeChunk(
	ctx context.Context,
	org db.OrgID,
	batchID pgtype.UUID,
	batch batchContext,
	page []ledger.Transaction,
	resp classify.BatchResponse,
) error {
	rows, err := w.chunkClassifications(batch, page, resp)
	if err != nil {
		return err
	}

	return w.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		for _, c := range rows {
			if _, err := ledger.InsertClassification(ctx, tx, org, c); err != nil {
				return fmt.Errorf("classifyrun: storing a classification: %w", err)
			}
		}
		// ExactlyOneRow, because the WHERE clause names `status = 'running'`:
		// a chunk landing on a run somebody already finished means two jobs
		// are working the same batch, and that is worth an error rather than a
		// silently discarded update.
		_, err := gendb.New(tx).RecordChunkResult(ctx, gendb.RecordChunkResultParams{
			BatchID:         batchID,
			ClassifiedCount: int32(len(rows)),
			ReviewCount:     int32(len(page) - len(rows)),
		})
		if err != nil {
			return fmt.Errorf("classifyrun: recording the chunk: %w", err)
		}
		return nil
	})
}

// chunkClassifications turns a response into rows, refusing the whole response
// if any of it names something core did not send.
//
// Whole response, not the offending proposal: ARCHITECTURE.md §3.5's last row.
// A response containing a category core never offered is a response core cannot
// reason about -- the safe reading is that the other side is answering a
// different question, not that one line of its answer is wrong.
func (w *classifyWorker) chunkClassifications(
	batch batchContext, page []ledger.Transaction, resp classify.BatchResponse,
) ([]ledger.Classification, error) {
	inPage := make(map[string]uuid.UUID, len(page))
	for _, t := range page {
		inPage[t.ID.String()] = t.ID
	}

	out := make([]ledger.Classification, 0, len(resp.Proposals))
	seen := make(map[uuid.UUID]bool, len(resp.Proposals))
	for _, p := range resp.Proposals {
		txnID, ok := inPage[p.TransactionID]
		if !ok {
			return nil, &fatalError{code: FailureUnknownCategory, err: fmt.Errorf(
				"proposal names transaction %s, which was not in the chunk", p.TransactionID)}
		}
		if seen[txnID] {
			// Two answers for one transaction. The append-only trigger would
			// refuse the second at commit; refusing here says which response
			// was wrong instead of which insert failed.
			return nil, &fatalError{code: FailureUnknownCategory, err: fmt.Errorf(
				"two proposals for transaction %s", p.TransactionID)}
		}
		seen[txnID] = true

		categoryID, ok := batch.codes[p.CategoryCode]
		if !ok {
			return nil, &fatalError{code: FailureUnknownCategory, err: fmt.Errorf(
				"proposal names category %q, which was not in the request", p.CategoryCode)}
		}

		out = append(out, ledger.Classification{
			TransactionID: txnID,
			CategoryID:    uuid.UUID(categoryID.Bytes),
			EngineLayer:   p.EngineLayer,
			Confidence:    p.Confidence,
			Evidence:      p.Evidence,

			TaxonomyVersion: w.versions.Taxonomy,
			// From the response, not from configuration: what answered is the
			// engine's fact to state. A constant here would say what this
			// build would classify with today, which is the same string only
			// until the next deploy.
			RulesetVersion:   resp.RulesetVersion,
			EngineVersion:    resp.EngineVersion,
			NormalizeVersion: normalize.Version,
		})
	}
	return out, nil
}

func (w *classifyWorker) finish(ctx context.Context, org db.OrgID, batchID pgtype.UUID) error {
	err := w.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := gendb.New(tx).FinishClassificationRun(ctx, gendb.FinishClassificationRunParams{
			BatchID: batchID,
			Status:  "classified",
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("classifyrun: finishing the run: %w", err)
	}
	return nil
}

// fail records why and stops. It returns nil: a run that failed because the
// other side named a category that does not exist will fail identically on
// every retry, and burning River's attempts on it only delays the moment
// somebody reads the code.
func (w *classifyWorker) fail(ctx context.Context, org db.OrgID, batchID pgtype.UUID, code string) error {
	err := w.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := gendb.New(tx).FinishClassificationRun(ctx, gendb.FinishClassificationRunParams{
			BatchID:     batchID,
			Status:      "failed",
			FailureCode: pgtype.Text{String: code, Valid: true},
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("classifyrun: recording %s: %w", code, err)
	}
	return nil
}

// chunkRequest is the whole of what one call depends on.
func chunkRequest(c batchContext, v Versions, page []ledger.Transaction) classify.BatchRequest {
	return classify.BatchRequest{
		// Derived from the chunk rather than minted, so two runs of the same
		// chunk carry the same identifier and a log can be read across a
		// retry.
		RequestID:        page[0].ID.String(),
		TaxonomyVersion:  v.Taxonomy,
		RulesetVersion:   v.Ruleset,
		NormalizeVersion: normalize.Version,
		Categories:       c.categories,
		Rules:            c.rules,
		Vendors:          c.vendors,
		Txns:             ledger.ToClassifyBatch(page),
		// Left unset, which the engine reads as its own documented default of
		// 0.80 (decision D-10). The number lives in one place, and that place
		// is the side that computed the confidence it is compared against.
		// When an organisation can set its own, this is the line that reads it.
		Threshold: 0,
	}
}

// fatalError is a failure that will recur identically on a retry, so the run is
// marked failed rather than attempted again.
type fatalError struct {
	code string
	err  error
}

func (e *fatalError) Error() string { return e.code + ": " + e.err.Error() }
func (e *fatalError) Unwrap() error { return e.err }

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

// matcher is the stored JSON shape of `classification_rules.matcher`.
type matcher struct {
	SourceKind       string             `json:"source_kind"`
	NormalizeVersion string             `json:"normalize_version"`
	All              []matcherCondition `json:"all"`
}

type matcherCondition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`

	// For the amount comparisons, which no seeded rule uses yet and which a
	// customer's own rule will. Money is minor units plus a code here as
	// everywhere else -- a float in a rule would compare a customer's threshold
	// against their transactions approximately.
	Amount *struct {
		CurrencyCode string `json:"currency_code"`
		MinorUnits   int64  `json:"minor_units"`
	} `json:"amount"`
}

// toRule reads one stored rule.
//
// It refuses what it cannot read rather than dropping it, and that asymmetry is
// the point: a rule that loses a condition does not stop matching, it starts
// matching everything its remaining conditions allow. A rule that silently
// widened is a customer's money on the wrong line with nothing in any log.
func toRule(row gendb.EffectiveRulesRow) (classify.Rule, error) {
	var m matcher
	if err := json.Unmarshal(row.Matcher, &m); err != nil {
		return classify.Rule{}, fmt.Errorf("rule %s: unreadable matcher: %w", row.ID, err)
	}
	if len(m.All) == 0 {
		// The table's own CHECK refuses this, so reaching it means the column
		// holds a shape the constraint admits and this cannot read.
		return classify.Rule{}, fmt.Errorf("rule %s: matcher has no conditions", row.ID)
	}

	rule := classify.Rule{
		Priority:     row.Priority,
		CategoryCode: row.CategoryCode,
		Scope:        row.Scope,
		SourceKind:   m.SourceKind,
	}
	for _, c := range m.All {
		if c.Field == "" || c.Op == "" {
			return classify.Rule{}, fmt.Errorf("rule %s: a condition names no field or no operator", row.ID)
		}
		cond := classify.Condition{Field: c.Field, Op: c.Op, Value: c.Value}
		if c.Amount != nil {
			amount, err := money.New(c.Amount.CurrencyCode, c.Amount.MinorUnits)
			if err != nil {
				return classify.Rule{}, fmt.Errorf("rule %s: %w", row.ID, err)
			}
			cond.AmountValue = amount
		}
		rule.All = append(rule.All, cond)
	}
	return rule, nil
}

// Run is what happened when a batch was classified, for a screen to read.
type Run struct {
	Status      string // running | classified | failed
	FailureCode string

	ChunkCount      int32
	ClassifiedCount int32
	ReviewCount     int32

	StartedAt  time.Time
	FinishedAt time.Time // zero while the run is still going
}

// ForBatch reads one batch's run inside the caller's own transaction.
//
// The second return value is whether there is one at all, and absent is a real
// answer rather than a missing one: a batch that has just reached `imported`
// has not been classified yet, which a screen renders differently from a run
// that failed. Returning a zero Run for both would make the first look like the
// second.
//
// It lives here rather than in the package that shows it because this is the
// package that owns the table. A reader elsewhere would be a second place that
// decides what these three statuses mean.
func ForBatch(ctx context.Context, tx pgx.Tx, batchID uuid.UUID) (Run, bool, error) {
	row, err := gendb.New(tx).GetClassificationRun(ctx,
		pgtype.UUID{Bytes: batchID, Valid: true})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Run{}, false, nil
	case err != nil:
		return Run{}, false, fmt.Errorf("classifyrun: reading the run: %w", err)
	}

	out := Run{
		Status:          row.Status,
		FailureCode:     row.FailureCode.String,
		ChunkCount:      row.ChunkCount,
		ClassifiedCount: row.ClassifiedCount,
		ReviewCount:     row.ReviewCount,
		StartedAt:       row.StartedAt.Time,
	}
	if row.FinishedAt.Valid {
		out.FinishedAt = row.FinishedAt.Time
	}
	return out, true, nil
}
