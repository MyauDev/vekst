package ingest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/blob"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/jobs"
)

// --- test harness -----------------------------------------------------
//
// Every test below skips itself when either dependency is absent, the same
// way core/internal/db's own tests do: this exercises real Postgres
// row-level security and a real object store's signature verification,
// neither of which a mock can stand in for.

func testDB(t *testing.T) *db.DB {
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
	return d
}

func testStore(t *testing.T) blob.ObjectStore {
	t.Helper()
	endpoint := os.Getenv("OBJECT_STORE_ENDPOINT")
	if endpoint == "" {
		t.Skip("OBJECT_STORE_ENDPOINT not set; skipping a test that needs a live object store")
	}
	bucket := os.Getenv("OBJECT_STORE_BUCKET")
	if bucket == "" {
		bucket = "vekst"
	}
	return blob.New(blob.Config{
		Endpoint:    endpoint,
		Bucket:      bucket,
		Region:      "us-east-1",
		AccessKeyID: envOr("OBJECT_STORE_ACCESS_KEY_ID", "minioadmin"),
		SecretKey:   envOr("OBJECT_STORE_SECRET_KEY", "minioadmin"),
		PathStyle:   true,
	})
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// testUser and testOrgAndEntity mirror core/internal/db's own test helpers
// of the same purpose: a throwaway user (users is global and outside
// row-level security, written through InSystemTx) and a complete
// organisation created through the real CreateOrganization path, which also
// gives it the one entity every organisation has in the Demo.
func testUser(t *testing.T, d *db.DB) uuid.UUID {
	t.Helper()
	var id pgtype.UUID
	err := d.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO users (email) VALUES ($1) RETURNING id`,
			uuid.NewString()+"@ingest.test").Scan(&id)
	})
	if err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	return uuid.UUID(id.Bytes)
}

func testOrgAndEntity(t *testing.T, d *db.DB, owner uuid.UUID) (db.OrgID, uuid.UUID) {
	t.Helper()
	org, err := d.CreateOrganization(context.Background(), db.NewOrganization{
		Name:         "Test " + uuid.NewString()[:8],
		Country:      "NO",
		BaseCurrency: "NOK",
		EntityName:   "Test AS",
		CreatorID:    owner,
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	var entityID uuid.UUID
	err = d.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
		entities, err := gendb.New(tx).ListEntities(ctx)
		if err != nil {
			return err
		}
		entityID = uuid.UUID(entities[0].ID.Bytes)
		return nil
	})
	if err != nil {
		t.Fatalf("reading the organisation's entity: %v", err)
	}
	return org, entityID
}

type testEnv struct {
	db    *db.DB
	store blob.ObjectStore
	jobs  *jobs.Client
	svc   *Service
}

func newTestEnvWithConfig(t *testing.T, cfg Config) *testEnv {
	t.Helper()
	d := testDB(t)
	store := testStore(t)

	workers := NewWorkers(d, store, cfg)
	jobsClient, err := jobs.New(d, workers)
	if err != nil {
		t.Fatalf("jobs.New: %v", err)
	}
	if err := jobsClient.Start(context.Background()); err != nil {
		t.Fatalf("jobs Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = jobsClient.Stop(stopCtx)
	})

	return &testEnv{db: d, store: store, jobs: jobsClient, svc: NewService(d, store, jobsClient, cfg)}
}

func newTestEnv(t *testing.T) *testEnv {
	return newTestEnvWithConfig(t, Config{UploadMaxBytes: 26_214_400, UploadURLLifetime: 15 * time.Minute})
}

func (env *testEnv) rawBatch(t *testing.T, org db.OrgID, batchID uuid.UUID) gendb.GetImportBatchRow {
	t.Helper()
	var row gendb.GetImportBatchRow
	err := env.db.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, err = gendb.New(tx).GetImportBatch(ctx, pgtype.UUID{Bytes: batchID, Valid: true})
		return err
	})
	if err != nil {
		t.Fatalf("reading raw batch: %v", err)
	}
	return row
}

// waitForStatus polls the batch until it reaches want or five seconds pass.
// Both jobs this change enqueues run asynchronously through a real River
// client, the same way jobs/noop_test.go polls river_job directly.
func (env *testEnv) waitForStatus(t *testing.T, org db.OrgID, batchID uuid.UUID, want Status) gendb.GetImportBatchRow {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last gendb.GetImportBatchRow
	for time.Now().Before(deadline) {
		last = env.rawBatch(t, org, batchID)
		if Status(last.Status) == want {
			return last
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("batch never reached status %s; last seen status %q, failure_code %q", want, last.Status, last.FailureCode.String)
	return last
}

func putBytes(t *testing.T, url string, headers map[string]string, body []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("building PUT: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT status = %d: %s", resp.StatusCode, b)
	}
}

// pgCode returns the SQLSTATE of a Postgres error, or "" if err is not one.
// Asserting on the code rather than the message keeps these tests from
// breaking on a Postgres wording change -- the same reasoning as
// core/internal/db's own pgCode.
func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ""
	}
	return pgErr.Code
}

// --- the happy path, exercised end to end ------------------------------

// The whole pipeline, end to end, on a real (redacted) customer file:
// upload, measure, parse, persist raw rows, validate with a clean report,
// dedup, and land in transactions as imported. This is add-file-upload
// (2.1), add-statement-parsing (2.2), add-ingest-validation (2.3) and
// add-dedup (2.6) working together for the first time.
func TestRealPriorbankFixtureReachesImported(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "priorbank-by", "*.csv"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no Priorbank fixtures found: %v", err)
	}
	body, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	batch, upload, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID: entityID, SourceKind: SourceKindBank, FileName: filepath.Base(paths[0]),
		DeclaredBytes: int64(len(body)), DeclaredType: "text/csv",
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}
	putBytes(t, upload.URL, upload.Headers, body)
	if _, err := env.svc.ConfirmImportUpload(ctx, owner, org.UUID(), batch.ID); err != nil {
		t.Fatalf("ConfirmImportUpload: %v", err)
	}

	// validated is no longer where a clean batch rests: add-dedup's persist
	// job (change 2.6) chains automatically the moment validation reaches
	// it, warnings or not, so a fixture with nothing to skip and nothing to
	// pair settles at imported instead.
	final := env.waitForStatus(t, org, batch.ID, StatusImported)
	if final.FailureCode.Valid {
		t.Errorf("an imported batch should carry no failure_code, got %q", final.FailureCode.String)
	}

	var validation gendb.ImportValidation
	err = env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		validation, err = gendb.New(tx).GetValidationForBatch(ctx, pgtype.UUID{Bytes: batch.ID, Valid: true})
		return err
	})
	if err != nil {
		t.Fatalf("GetValidationForBatch: %v", err)
	}
	if validation.Outcome != string(OutcomeValid) {
		t.Errorf("outcome = %s, want valid (this fixture reconciles exactly)", validation.Outcome)
	}
	if validation.ErrorCount != 0 {
		t.Errorf("error_count = %d, want 0", validation.ErrorCount)
	}
	if !validation.BalanceCheckPassed.Valid || !validation.BalanceCheckPassed.Bool {
		t.Errorf("balance_check_passed = %+v, want true", validation.BalanceCheckPassed)
	}

	var rawRowCount int
	err = env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM raw_rows WHERE batch_id = $1`,
			pgtype.UUID{Bytes: batch.ID, Valid: true}).Scan(&rawRowCount)
	})
	if err != nil {
		t.Fatalf("counting raw_rows: %v", err)
	}
	if rawRowCount == 0 {
		t.Error("no raw_rows were persisted for a successfully parsed batch")
	}

	var txnCount int
	err = env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE batch_id = $1`,
			pgtype.UUID{Bytes: batch.ID, Valid: true}).Scan(&txnCount)
	})
	if err != nil {
		t.Fatalf("counting transactions: %v", err)
	}
	if txnCount != rawRowCount {
		t.Errorf("transactions for this batch = %d, want %d (one per raw row, nothing skipped)", txnCount, rawRowCount)
	}
}

func TestCreateConfirmMeasureSucceeds(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	body := []byte("date,amount,description\n2026-01-01,100,coffee\n")
	batch, upload, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID:      entityID,
		SourceKind:    SourceKindBank,
		FileName:      "statement.csv",
		DeclaredBytes: int64(len(body)),
		DeclaredType:  "text/csv",
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}
	if batch.Status != StatusAwaitingUpload {
		t.Errorf("Status = %s, want awaiting_upload", batch.Status)
	}
	if batch.SourceKind != SourceKindBank {
		t.Errorf("SourceKind = %s, want bank", batch.SourceKind)
	}

	putBytes(t, upload.URL, upload.Headers, body)

	if _, err := env.svc.ConfirmImportUpload(ctx, owner, org.UUID(), batch.ID); err != nil {
		t.Fatalf("ConfirmImportUpload: %v", err)
	}

	final := env.waitForStatus(t, org, batch.ID, StatusUploaded)
	if final.ByteLength.Int64 != int64(len(body)) {
		t.Errorf("ByteLength = %d, want %d", final.ByteLength.Int64, len(body))
	}
	if len(final.FileSha256) != 32 {
		t.Errorf("FileSha256 has %d bytes, want 32", len(final.FileSha256))
	}
	if final.ContentType.String != contentTypeText {
		t.Errorf("ContentType = %q, want %q", final.ContentType.String, contentTypeText)
	}
}

// --- task 6.1: cross-tenant isolation -----------------------------------

func TestCrossTenantIsolation(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	ownerA := testUser(t, env.db)
	ownerB := testUser(t, env.db)
	orgA, entityA := testOrgAndEntity(t, env.db, ownerA)
	orgB, entityB := testOrgAndEntity(t, env.db, ownerB)

	batch, _, err := env.svc.CreateImportBatch(ctx, ownerA, orgA.UUID(), CreateBatchInput{
		EntityID: entityA, SourceKind: SourceKindBank, FileName: "a.csv",
		DeclaredBytes: 10, DeclaredType: "text/csv",
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}

	// B naming A's organisation is refused before a transaction ever opens:
	// db.OrgIDForSession rejects a caller who is not a member, which is the
	// permission failure -- not a wider read that RLS then narrows.
	if _, err := env.svc.GetImportBatch(ctx, ownerB, orgA.UUID(), batch.ID); !errors.Is(err, ErrNotAMember) {
		t.Errorf("GetImportBatch with B acting for A's org = %v, want ErrNotAMember", err)
	}

	// B, acting for its own real organisation, addressing A's batch by id.
	// Row-level security is what refuses this -- the row is not visible under
	// B's tenant context -- and the answer is the same ErrBatchNotFound a
	// nonexistent id would produce (task 6.1: a policy denial, not a
	// distinguishable not-found).
	if _, err := env.svc.GetImportBatch(ctx, ownerB, orgB.UUID(), batch.ID); !errors.Is(err, ErrBatchNotFound) {
		t.Errorf("GetImportBatch for another tenant's batch id = %v, want ErrBatchNotFound", err)
	}
	if _, err := env.svc.ConfirmImportUpload(ctx, ownerB, orgB.UUID(), batch.ID); !errors.Is(err, ErrBatchNotFound) {
		t.Errorf("ConfirmImportUpload for another tenant's batch id = %v, want ErrBatchNotFound", err)
	}

	// Listing B's own entity never surfaces A's batch.
	batches, err := env.svc.ListImportBatches(ctx, ownerB, orgB.UUID(), entityB)
	if err != nil {
		t.Fatalf("ListImportBatches: %v", err)
	}
	for _, b := range batches {
		if b.ID == batch.ID {
			t.Error("ListImportBatches leaked another tenant's batch")
		}
	}

	// And the update path: RecordUploadMeasurement and SetImportBatchStatus
	// are keyed by id alone, exactly like GetImportBatch, so the same
	// row-level-security policy is what stands between B and a write to A's
	// row -- exercised here as an update that must affect zero rows.
	err = env.db.InTx(ctx, orgB, func(ctx context.Context, tx pgx.Tx) error {
		affected, err := gendb.New(tx).SetImportBatchStatus(ctx, gendb.SetImportBatchStatusParams{
			ID:     pgtype.UUID{Bytes: batch.ID, Valid: true},
			Status: string(StatusFailed),
		})
		return db.ExactlyOneRow(affected, err)
	})
	if !errors.Is(err, db.ErrNoRowsAffected) {
		t.Errorf("B updating A's batch = %v, want db.ErrNoRowsAffected", err)
	}
}

// A batch cannot be created for another organisation's entity (file-ingestion
// spec, "A batch exists before its bytes do"). The composite foreign key
// `(org_id, entity_id) REFERENCES entities (org_id, id)` is what refuses
// this -- A naming B's entity_id has no row to match, the same shape a
// nonexistent entity_id would produce, so the two are indistinguishable by
// construction rather than by a check this code has to remember to make.
func TestCreateImportBatchRefusesAnotherOrganisationsEntity(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	ownerA := testUser(t, env.db)
	ownerB := testUser(t, env.db)
	orgA, _ := testOrgAndEntity(t, env.db, ownerA)
	_, entityB := testOrgAndEntity(t, env.db, ownerB)

	_, _, errForeignEntity := env.svc.CreateImportBatch(ctx, ownerA, orgA.UUID(), CreateBatchInput{
		EntityID: entityB, SourceKind: SourceKindBank, FileName: "x.csv",
		DeclaredBytes: 10, DeclaredType: "text/csv",
	})
	if errForeignEntity == nil {
		t.Fatal("CreateImportBatch succeeded naming another organisation's entity")
	}

	_, _, errNoSuchEntity := env.svc.CreateImportBatch(ctx, ownerA, orgA.UUID(), CreateBatchInput{
		EntityID: uuid.New(), SourceKind: SourceKindBank, FileName: "x.csv",
		DeclaredBytes: 10, DeclaredType: "text/csv",
	})
	if errNoSuchEntity == nil {
		t.Fatal("CreateImportBatch succeeded naming a nonexistent entity")
	}

	if errForeignEntity.Error() != errNoSuchEntity.Error() {
		t.Errorf("the two refusals differ, which discloses that B's entity exists:\n"+
			"  another org's entity: %v\n  no such entity:        %v", errForeignEntity, errNoSuchEntity)
	}
}

// --- task 6.2: fail-closed ----------------------------------------------

func TestQueriesFailClosedOutsideATenantTransaction(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	someID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	cases := map[string]func(tx pgx.Tx) error{
		"InsertImportBatch": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).InsertImportBatch(ctx, gendb.InsertImportBatchParams{
				OrgID: someID, ID: someID, EntityID: someID, SourceKind: "bank",
				UploadedBy: someID, FileName: "x", DeclaredBytes: 1, DeclaredType: "text/csv",
				FileKey: "x", UploadExpiresAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			})
			return err
		},
		"GetImportBatch": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).GetImportBatch(ctx, someID)
			return err
		},
		"ListImportBatches": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).ListImportBatches(ctx, someID)
			return err
		},
		"RecordUploadMeasurement": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).RecordUploadMeasurement(ctx, gendb.RecordUploadMeasurementParams{ID: someID})
			return err
		},
		"SetImportBatchStatus": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).SetImportBatchStatus(ctx, gendb.SetImportBatchStatusParams{ID: someID, Status: "failed"})
			return err
		},
		"AbandonExpiredBatch": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).AbandonExpiredBatch(ctx, someID)
			return err
		},
	}

	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			err := env.db.InSystemTx(context.Background(), func(_ context.Context, tx pgx.Tx) error {
				return run(tx)
			})
			if err == nil {
				t.Fatalf("%s succeeded with no tenant context", name)
			}
			if code := pgCode(err); code != "42704" {
				t.Fatalf("%s: SQLSTATE = %q (%v), want 42704", name, code, err)
			}
		})
	}
}

// --- task 6.3: the client lies about size --------------------------------

func TestMeasurementFailsAnOversizedUpload(t *testing.T) {
	// A small limit, not a small file: what matters is that the object
	// measures larger than UploadMaxBytes, whatever the client declared it
	// would be.
	env := newTestEnvWithConfig(t, Config{UploadMaxBytes: 16, UploadURLLifetime: 15 * time.Minute})
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	body := bytes.Repeat([]byte("x"), 100)
	batch, upload, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID: entityID, SourceKind: SourceKindBank, FileName: "big.csv",
		DeclaredBytes: int64(len(body)), DeclaredType: "text/csv",
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}
	putBytes(t, upload.URL, upload.Headers, body)

	if _, err := env.svc.ConfirmImportUpload(ctx, owner, org.UUID(), batch.ID); err != nil {
		t.Fatalf("ConfirmImportUpload: %v", err)
	}

	final := env.waitForStatus(t, org, batch.ID, StatusFailed)
	if final.FailureCode.String != failureFileTooLarge {
		t.Errorf("FailureCode = %q, want %q", final.FailureCode.String, failureFileTooLarge)
	}

	key := blob.Key(org.UUID(), batch.ID)
	if err := env.store.Head(ctx, key); !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("Head after file_too_large = %v, want ErrNotFound -- the oversized object must be deleted", err)
	}
}

// --- task 6.4: the client lies about type --------------------------------

func TestMeasurementSniffsContentTypeIgnoringDeclaredType(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	body := append([]byte("PK\x03\x04"), []byte("the rest of a fake xlsx archive")...)
	batch, upload, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID: entityID, SourceKind: SourceKindBank, FileName: "statement.xlsx",
		DeclaredBytes: int64(len(body)),
		DeclaredType:  "text/csv", // the lie: declared as CSV
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}
	putBytes(t, upload.URL, upload.Headers, body)

	if _, err := env.svc.ConfirmImportUpload(ctx, owner, org.UUID(), batch.ID); err != nil {
		t.Fatalf("ConfirmImportUpload: %v", err)
	}

	final := env.waitForStatus(t, org, batch.ID, StatusUploaded)
	if final.ContentType.String != contentTypeXLSX {
		t.Errorf("ContentType = %q, want %q (from the magic bytes, not declared_type)", final.ContentType.String, contentTypeXLSX)
	}
}

// --- task 6.5 / 6.6: the upload that never arrives, and the one that does --

func TestExpiryAbandonsAnUploadThatNeverArrives(t *testing.T) {
	env := newTestEnvWithConfig(t, Config{UploadMaxBytes: 26_214_400, UploadURLLifetime: 200 * time.Millisecond})
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	batch, _, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID: entityID, SourceKind: SourceKindBank, FileName: "never-uploaded.csv",
		DeclaredBytes: 10, DeclaredType: "text/csv",
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}

	// No PUT. Just wait for the expiry job.
	env.waitForStatus(t, org, batch.ID, StatusAbandoned)

	// A confirm afterwards does not resurrect it: the row exists (it was
	// abandoned, not deleted), so ConfirmImportUpload succeeds and enqueues
	// a measurement job -- but that job finds no object and fails it
	// upload_missing rather than reviving awaiting_upload. Abandoned stays
	// terminal either way: CheckTransition admits no edge out of it.
	if _, err := env.svc.ConfirmImportUpload(ctx, owner, org.UUID(), batch.ID); err != nil {
		t.Fatalf("ConfirmImportUpload after abandonment: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	still := env.rawBatch(t, org, batch.ID)
	if Status(still.Status) != StatusAbandoned {
		t.Errorf("status after a late confirm = %s, want it to stay abandoned", still.Status)
	}
}

func TestExpiryDoesNotUndoARealUpload(t *testing.T) {
	// At least whole seconds: SigV4's X-Amz-Expires is an integer number of
	// seconds, so anything under a second rounds down to an already-expired
	// URL and the PUT below would fail before the expiry job is even the
	// point of the test.
	env := newTestEnvWithConfig(t, Config{UploadMaxBytes: 26_214_400, UploadURLLifetime: 2 * time.Second})
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	// Not a recognised export -- add-ingest-validation's ValidateImportArgs
	// job, chained automatically from a successful measurement, reaches
	// 'failed' almost immediately regardless of the expiry job below. That is
	// the point: whatever terminal state a real upload reaches, a late
	// expiry job must not revert it to 'abandoned'.
	body := []byte("date,amount\n2026-01-01,1\n")
	batch, upload, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID: entityID, SourceKind: SourceKindBank, FileName: "real-upload.csv",
		DeclaredBytes: int64(len(body)), DeclaredType: "text/csv",
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}
	putBytes(t, upload.URL, upload.Headers, body)
	if _, err := env.svc.ConfirmImportUpload(ctx, owner, org.UUID(), batch.ID); err != nil {
		t.Fatalf("ConfirmImportUpload: %v", err)
	}
	env.waitForStatus(t, org, batch.ID, StatusFailed)

	// Outlive the expiry job's scheduled time -- if it were going to abandon
	// a real upload, this is long enough for that to have happened.
	time.Sleep(2500 * time.Millisecond)

	still := env.rawBatch(t, org, batch.ID)
	if Status(still.Status) != StatusFailed {
		t.Errorf("status after outliving the expiry time = %s, want it to stay failed (untouched by expiry)", still.Status)
	}
	// And its object is still there -- the expiry job must not have deleted
	// it either.
	if err := env.store.Head(ctx, blob.Key(org.UUID(), batch.ID)); err != nil {
		t.Errorf("Head after outliving the expiry time = %v, want the object to still exist", err)
	}
}

// --- task 6.7: source_kind is immutable ----------------------------------

func TestSourceKindIsImmutable(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	batch, _, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID: entityID, SourceKind: SourceKindBank, FileName: "x.csv",
		DeclaredBytes: 10, DeclaredType: "text/csv",
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}

	err = env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE import_batches SET source_kind = 'ledger' WHERE id = $1`,
			pgtype.UUID{Bytes: batch.ID, Valid: true})
		return err
	})
	if err == nil {
		t.Fatal("updating source_kind succeeded; migration 008's column grant should have refused it")
	}
	if code := pgCode(err); code != "42501" {
		t.Errorf("SQLSTATE = %q (%v), want 42501 (insufficient_privilege)", code, err)
	}
}

// --- task 6.8 / 6.8b: import_batches_measured_past_upload ----------------

func TestMeasuredPastUploadConstraintHolds(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	batch, _, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID: entityID, SourceKind: SourceKindBank, FileName: "x.csv",
		DeclaredBytes: 10, DeclaredType: "text/csv",
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}

	// Go's ExactlyOneRow is not what stands between a client and this row:
	// the database itself refuses it, by SQLSTATE, not merely by a Go check
	// that a determined caller could route around.
	err = env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE import_batches SET status = 'uploaded' WHERE id = $1`,
			pgtype.UUID{Bytes: batch.ID, Valid: true})
		return err
	})
	if err == nil {
		t.Fatal("setting status = uploaded with a NULL sha256 succeeded")
	}
	if code := pgCode(err); code != "23514" {
		t.Errorf("SQLSTATE = %q (%v), want 23514 (check_violation)", code, err)
	}
}

func TestFailureBeforeMeasurementAdmitsNullMeasurements(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	// Created, never uploaded, confirmed anyway: the measurement job HEADs
	// an object that was never written.
	batch, _, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID: entityID, SourceKind: SourceKindBank, FileName: "missing.csv",
		DeclaredBytes: 10, DeclaredType: "text/csv",
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}
	if _, err := env.svc.ConfirmImportUpload(ctx, owner, org.UUID(), batch.ID); err != nil {
		t.Fatalf("ConfirmImportUpload: %v", err)
	}

	final := env.waitForStatus(t, org, batch.ID, StatusFailed)
	if final.FailureCode.String != failureUploadMissing {
		t.Errorf("FailureCode = %q, want %q", final.FailureCode.String, failureUploadMissing)
	}
	if final.FileSha256 != nil {
		t.Errorf("FileSha256 = %x, want NULL -- this is the row the first draft of the constraint refused", final.FileSha256)
	}
	if final.ByteLength.Valid {
		t.Errorf("ByteLength = %d, want NULL", final.ByteLength.Int64)
	}
	if final.ContentType.Valid {
		t.Errorf("ContentType = %q, want NULL", final.ContentType.String)
	}
}

// --- task 6.10: idempotent confirm ---------------------------------------

func TestConfirmImportUploadTwiceIsIdempotent(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	body := []byte("date,amount\n2026-01-01,1\n")
	batch, upload, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID: entityID, SourceKind: SourceKindBank, FileName: "x.csv",
		DeclaredBytes: int64(len(body)), DeclaredType: "text/csv",
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}
	putBytes(t, upload.URL, upload.Headers, body)

	if _, err := env.svc.ConfirmImportUpload(ctx, owner, org.UUID(), batch.ID); err != nil {
		t.Fatalf("first ConfirmImportUpload: %v", err)
	}
	// Not a recognised export, so the chained validation job
	// (add-ingest-validation) settles the batch on 'failed' deterministically
	// rather than racing to catch the transient 'uploaded' state a
	// successful measurement now passes through almost immediately.
	first := env.waitForStatus(t, org, batch.ID, StatusFailed)

	// A second confirm, once the batch has already settled: the measurement
	// job it enqueues finds a status that is neither awaiting_upload nor
	// uploaded and is a guarded no-op (measure.go), so nothing is
	// re-measured -- but nothing is corrupted or duplicated either, which is
	// task 6.10's actual property. Re-measuring the same immutable object
	// twice while still in a re-measurable window is exercised structurally
	// by RecordUploadMeasurement's own :execrows semantics; the timing to
	// force that window deterministically through the real River pipeline
	// isn't worth manufacturing here.
	if _, err := env.svc.ConfirmImportUpload(ctx, owner, org.UUID(), batch.ID); err != nil {
		t.Fatalf("second ConfirmImportUpload: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	second := env.rawBatch(t, org, batch.ID)

	if Status(second.Status) != StatusFailed {
		t.Errorf("status after a second confirm = %s, want it to stay failed", second.Status)
	}
	if !bytes.Equal(first.FileSha256, second.FileSha256) {
		t.Errorf("FileSha256 changed between runs: %x != %x", first.FileSha256, second.FileSha256)
	}
	if first.ByteLength != second.ByteLength {
		t.Errorf("ByteLength changed between runs: %v != %v", first.ByteLength, second.ByteLength)
	}
}
