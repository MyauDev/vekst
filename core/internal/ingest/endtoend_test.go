package ingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MyauDev/vekst/core/classify"
	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/report"
)

// noProposalsClassifier answers every chunk successfully, with no proposal
// for any transaction -- the wire shape of "below threshold" (add-classification-run
// design), not an error. It exists so this test can drive a classification
// run to a real terminal status ("classified") without a Python process: the
// engine itself is tested in its own language, against its own fixtures; what
// this package is testing is what core does with an answer, and "propose
// nothing" is as real an answer as any other. Every transaction in the
// fixture therefore stays unclassified, which is what this test's report
// assertion depends on.
type noProposalsClassifier struct{}

func (noProposalsClassifier) Version(context.Context) (classify.VersionInfo, error) {
	return classify.VersionInfo{EngineVersion: "test-stub"}, nil
}

func (noProposalsClassifier) Classify(_ context.Context, req classify.BatchRequest) (classify.BatchResponse, error) {
	return classify.BatchResponse{EngineVersion: "test-stub", RulesetVersion: "test-stub"}, nil
}

// Task 7.8. The whole pipeline, end to end: create an organisation, upload a
// real (redacted) Priorbank fixture, let validation, dedup and persistence
// run, and read a figure GetManagementPNL computed from what landed.
//
// This asserts on the classification run's outcome rather than on a category
// (design §6): make ci's Go job has no Python service to reach, so this uses
// noProposalsClassifier, not a real engine or even classify.Unavailable --
// Unavailable's error is not a rejection, so classifyrun treats it as
// retryable and the run never leaves "running" (found while writing this
// test; every other test in this package that touches classification only
// checks that persist *enqueued* it, never that it finished, for the same
// reason). noProposalsClassifier answers instantly with no proposal for any
// transaction, which is a real, successful outcome on the wire -- "classified,
// nothing met the threshold" -- and reaches "classified" deterministically.
// What proves the pipeline actually ran the money through is the report:
// every one of the fixture's transactions stays unclassified, and
// BucketUnclassified's total is the non-zero figure this task asks for --
// computed by the report from rows the pipeline persisted, not invented by
// the test.
func TestEndToEndOrganisationToReport(t *testing.T) {
	env := newTestEnvWithClassifier(t,
		Config{UploadMaxBytes: 26_214_400, UploadURLLifetime: 15 * time.Minute},
		noProposalsClassifier{})
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "priorbank-by", "*.csv"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no Priorbank fixtures found: %v", err)
	}
	path := paths[0] // "23-...": the 2023 statement, per its own header row
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	batch, upload, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID: entityID, SourceKind: SourceKindBank, FileName: filepath.Base(path),
		DeclaredBytes: int64(len(body)), DeclaredType: "text/csv",
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}
	putBytes(t, upload.URL, upload.Headers, body)
	if _, err := env.svc.ConfirmImportUpload(ctx, owner, org.UUID(), batch.ID); err != nil {
		t.Fatalf("ConfirmImportUpload: %v", err)
	}

	// Validation, dedup and persistence, chained automatically -- the same
	// pipeline TestRealPriorbankFixtureReachesImported already proves in
	// isolation. Reaching StatusImported is also what confirms persist's job
	// enqueued classification: it does so in the same transaction that lands
	// the batch there (see newTestEnvWithConfig's own comment).
	final := env.waitForStatus(t, org, batch.ID, StatusImported)
	if final.FailureCode.Valid {
		t.Fatalf("an imported batch should carry no failure_code, got %q", final.FailureCode.String)
	}

	run := waitForClassificationRun(t, env.db, org, batch.ID)
	if run.Status == "running" {
		t.Fatalf("classification run for the batch never left status running")
	}
	t.Logf("classification run finished: status=%s failure_code=%s classified=%d review=%d",
		run.Status, run.FailureCode.String, run.ClassifiedCount, run.ReviewCount)

	reporter := report.New(env.db)
	rep, err := reporter.ManagementPNL(ctx, owner, org.UUID(), report.Request{
		EntityID:    entityID,
		From:        "2023-01",
		To:          "2023-12",
		Granularity: report.Monthly,
		Basis:       report.BasisBank,
	})
	if err != nil {
		t.Fatalf("ManagementPNL: %v", err)
	}

	unclassified, ok := rep.Totals[report.BucketUnclassified]
	if !ok {
		t.Fatal("the report's totals carry no unclassified bucket at all")
	}
	if unclassified.MinorUnits == 0 {
		t.Fatal("GetManagementPNL's unclassified total is zero; the fixture's transactions should all have landed there, since classify.Unavailable never classifies anything")
	}
	if unclassified.CurrencyCode == "" {
		t.Error("the unclassified total carries no currency code")
	}
	t.Logf("unclassified total: %d %s minor units, across %d line(s)",
		unclassified.MinorUnits, unclassified.CurrencyCode, len(rep.Lines))
}

// waitForClassificationRun polls classification_runs for the batch until its
// status leaves "running" or five seconds pass -- the same shape
// testEnv.waitForStatus already uses for the import batch itself, since both
// are River jobs finishing asynchronously.
func waitForClassificationRun(t *testing.T, d *db.DB, org db.OrgID, batchID uuid.UUID) gendb.ClassificationRun {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last gendb.ClassificationRun
	var found bool
	for time.Now().Before(deadline) {
		err := d.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
			row, err := gendb.New(tx).GetClassificationRun(ctx, pgtype.UUID{Bytes: batchID, Valid: true})
			if err != nil {
				return err
			}
			last, found = row, true
			return nil
		})
		if err == nil && found && last.Status != "running" {
			return last
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !found {
		t.Fatal("no classification run was ever recorded for the batch")
	}
	t.Fatalf("classification run never left status running; last seen status %q", last.Status)
	return last
}
