package ingest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
)

// membershipRole sets the caller's own membership role directly -- role
// enforcement (task 0.2) needs a caller who is not the default 'owner'
// testOrgAndEntity's own CreateOrganization always creates.
func setMembershipRole(t *testing.T, d *db.DB, org db.OrgID, userID uuid.UUID, role string) {
	t.Helper()
	err := d.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE memberships SET role = $2 WHERE user_id = $1`, pgtype.UUID{Bytes: userID, Valid: true}, role)
		return err
	})
	if err != nil {
		t.Fatalf("setting membership role: %v", err)
	}
}

// uploadAndSettle runs a batch through the real pipeline to whatever
// terminal state its content produces, and returns it. body content that
// fails to parse settles on 'failed'; a rejected file settles on
// 'rejected'; a real Priorbank fixture -- valid or valid_with_warnings --
// settles on 'imported' now that add-dedup's persist job (change 2.6)
// chains automatically once validation reaches 'validated', so callers
// checking validation-report behaviour pass StatusImported, not
// StatusValidated, to avoid racing that chain.
func uploadAndSettle(t *testing.T, env *testEnv, owner uuid.UUID, org db.OrgID, entityID uuid.UUID, body []byte, want Status) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	batch, upload, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID: entityID, SourceKind: SourceKindBank, FileName: "x.csv",
		DeclaredBytes: int64(len(body)), DeclaredType: "text/csv",
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}
	putBytes(t, upload.URL, upload.Headers, body)
	if _, err := env.svc.ConfirmImportUpload(ctx, owner, org.UUID(), batch.ID); err != nil {
		t.Fatalf("ConfirmImportUpload: %v", err)
	}
	env.waitForStatus(t, org, batch.ID, want)
	return batch.ID
}

// realFixtureWithWarnings is a real (redacted) Priorbank export, mutated so
// its balance no longer reconciles -- an easy, deterministic way to reach
// valid_with_warnings without depending on which real fixture happens to
// carry a natural warning.
//
// The fixture on disk is windows-1251 or CP866 (DetectCharset), so the
// search below runs against this package's own decoded UTF-8 text rather
// than the raw bytes -- a Cyrillic label's byte pattern in either legacy
// encoding is not its UTF-8 one. ParsePriorbank re-detects the charset of
// whatever it is given, and UTF-8 is one of the three it recognises, so
// uploading the already-decoded text (rather than re-encoding back to the
// original charset) parses identically.
func realFixtureWithWarnings(t *testing.T) []byte {
	t.Helper()
	raw := realFixtureBody(t)
	text, err := Decode(raw, DetectCharset(raw))
	if err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}

	i := strings.LastIndex(text, "Исходящее сальдо")
	if i < 0 {
		t.Fatal("fixture has no closing-balance line to perturb")
	}
	lineEnd := strings.IndexByte(text[i:], '\n')
	if lineEnd < 0 {
		lineEnd = len(text) - i
	}
	line := text[i : i+lineEnd]
	mutated := strings.Replace(line, ",00;", ",01;", 1)
	if mutated == line {
		t.Fatal("could not perturb the closing balance line")
	}
	return []byte(text[:i] + mutated + text[i+lineEnd:])
}

func realFixtureBody(t *testing.T) []byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "priorbank-by", "*.csv"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no Priorbank fixtures found: %v", err)
	}
	body, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return body
}

// --- task 6.8: a correctness error cannot be overridden -------------------

func TestCorrectnessErrorCannotBeOverridden(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	// Not a recognised export -> 'failed', not a validation outcome at all,
	// so there is no import_validations row to override in the first place.
	// A batch that *does* reach 'rejected' needs a correctness error inside
	// an otherwise-parseable file; the simplest reliable one available here
	// without a hand-built fixture is a batch that never resolves an
	// account -- but every batch here has a real entity. Instead: drive
	// ValidateStatement directly against a statement with a correctness
	// error, write it exactly as validateWorker would, and confirm the
	// handler and the constraint both refuse an override of it.
	st := &Statement{
		Currency: "EUR",
		Rows: []Row{{LineNo: 1, BookedOn: "not a date", Description: "x",
			Debit: mustMoney(t, "EUR", 0), Credit: mustMoney(t, "EUR", 100),
			DebitRaw: "0,00", CreditRaw: "1,00"}},
	}
	result := ValidateStatement(st, fixedNow)
	if result.Outcome != OutcomeRejected {
		t.Fatalf("test setup: outcome = %s, want rejected", result.Outcome)
	}

	batch, upload, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID: entityID, SourceKind: SourceKindBank, FileName: "x.csv",
		DeclaredBytes: 10, DeclaredType: "text/csv",
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}
	putBytes(t, upload.URL, upload.Headers, []byte("irrelevant"))

	reportJSON, err := BuildReportJSON(result)
	if err != nil {
		t.Fatalf("BuildReportJSON: %v", err)
	}
	err = env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := gendb.New(tx).InsertValidation(ctx, gendb.InsertValidationParams{
			OrgID: pgtype.UUID{Bytes: org.UUID(), Valid: true}, BatchID: pgtype.UUID{Bytes: batch.ID, Valid: true},
			Outcome: string(result.Outcome), RowCount: 1, ErrorCount: int32(len(result.Errors)),
			WarningCount: 0, ReportJsonb: reportJSON,
		})
		return err
	})
	if err != nil {
		t.Fatalf("InsertValidation: %v", err)
	}

	// The handler refuses it with a clean code.
	if _, err := env.svc.OverrideValidation(ctx, owner, org.UUID(), batch.ID, "a good reason here"); !errors.Is(err, ErrOverrideOnlyOverWarnings) {
		t.Errorf("OverrideValidation on a rejected batch = %v, want ErrOverrideOnlyOverWarnings", err)
	}

	// The constraint refuses it independently, if the handler is bypassed.
	err = env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		affected, err := gendb.New(tx).RecordOverride(ctx, gendb.RecordOverrideParams{
			BatchID:        pgtype.UUID{Bytes: batch.ID, Valid: true},
			OverriddenBy:   pgtype.UUID{Bytes: owner, Valid: true},
			OverrideReason: pgtype.Text{String: "bypassing the handler check", Valid: true},
		})
		return db.ExactlyOneRow(affected, err)
	})
	if err == nil {
		t.Fatal("the database accepted an override of a rejected batch")
	}
	if code := pgCode(err); code != "23514" {
		t.Errorf("SQLSTATE = %q (%v), want 23514 (check_violation)", code, err)
	}
}

// --- task 0.2: role enforcement --------------------------------------------

func TestOverrideRequiresApproverRole(t *testing.T) {
	env := newTestEnv(t)
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)
	setMembershipRole(t, env.db, org, owner, "viewer")

	batchID := uploadAndSettle(t, env, owner, org, entityID, realFixtureWithWarnings(t), StatusImported)

	if _, err := env.svc.OverrideValidation(context.Background(), owner, org.UUID(), batchID, "a good reason here"); !errors.Is(err, ErrOverrideRequiresApproverRole) {
		t.Errorf("a viewer's override = %v, want ErrOverrideRequiresApproverRole", err)
	}
}

func TestOverrideReasonTooShort(t *testing.T) {
	env := newTestEnv(t)
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	batchID := uploadAndSettle(t, env, owner, org, entityID, realFixtureWithWarnings(t), StatusImported)

	if _, err := env.svc.OverrideValidation(context.Background(), owner, org.UUID(), batchID, "too short"); !errors.Is(err, ErrOverrideReasonTooShort) {
		t.Errorf("a 9-character reason = %v, want ErrOverrideReasonTooShort", err)
	}
}

// --- task 6.13: override is written once -----------------------------------

func TestOverrideIsWrittenOnce(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	batchID := uploadAndSettle(t, env, owner, org, entityID, realFixtureWithWarnings(t), StatusImported)

	report, err := env.svc.OverrideValidation(ctx, owner, org.UUID(), batchID, "the customer confirmed by phone")
	if err != nil {
		t.Fatalf("first OverrideValidation: %v", err)
	}
	if report.OverriddenBy != owner {
		t.Errorf("OverriddenBy = %s, want %s", report.OverriddenBy, owner)
	}

	if _, err := env.svc.OverrideValidation(ctx, owner, org.UUID(), batchID, "a second, different reason"); !errors.Is(err, ErrAlreadyOverridden) {
		t.Errorf("second OverrideValidation = %v, want ErrAlreadyOverridden", err)
	}

	final, err := env.svc.GetValidationReport(ctx, owner, org.UUID(), batchID)
	if err != nil {
		t.Fatalf("GetValidationReport: %v", err)
	}
	if final.OverrideReason != "the customer confirmed by phone" {
		t.Errorf("OverrideReason = %q, want the first reason unchanged", final.OverrideReason)
	}
}

// --- task 6.10: cross-tenant isolation -------------------------------------

func TestValidationCrossTenantIsolation(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	ownerA := testUser(t, env.db)
	ownerB := testUser(t, env.db)
	orgA, entityA := testOrgAndEntity(t, env.db, ownerA)
	orgB, _ := testOrgAndEntity(t, env.db, ownerB)

	batchID := uploadAndSettle(t, env, ownerA, orgA, entityA, realFixtureWithWarnings(t), StatusImported)

	if _, err := env.svc.GetValidationReport(ctx, ownerB, orgB.UUID(), batchID); !errors.Is(err, ErrValidationNotFound) {
		t.Errorf("B reading A's validation = %v, want ErrValidationNotFound", err)
	}
	if _, err := env.svc.OverrideValidation(ctx, ownerB, orgB.UUID(), batchID, "an override from org B"); !errors.Is(err, ErrValidationNotFound) {
		t.Errorf("B overriding A's validation = %v, want ErrValidationNotFound", err)
	}
}

// --- task 6.11: fail-closed -------------------------------------------------

func TestValidationQueriesFailClosedOutsideATenantTransaction(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	someID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	cases := map[string]func(tx pgx.Tx) error{
		"InsertValidation": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).InsertValidation(ctx, gendb.InsertValidationParams{
				OrgID: someID, BatchID: someID, Outcome: "valid", ReportJsonb: []byte(`{}`),
			})
			return err
		},
		"GetValidationForBatch": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).GetValidationForBatch(ctx, someID)
			return err
		},
		"RecordOverride": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).RecordOverride(ctx, gendb.RecordOverrideParams{BatchID: someID})
			return err
		},
		"OverriddenBatchesForPeriod": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).OverriddenBatchesForPeriod(ctx, gendb.OverriddenBatchesForPeriodParams{EntityID: someID})
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

// --- task 6.7: atomicity ----------------------------------------------------

// A rejected batch still gets exactly one import_validations row -- the
// receipt -- and contributes no transaction (2.5 does not exist yet to
// write one, so this also just confirms nothing here reaches into that
// table at all).
func TestRejectedBatchWritesAReceiptAndNoTransaction(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	body := []byte("date,amount\n2026-01-01,1\n") // unrecognised -> 'failed', not 'rejected'
	batchID := uploadAndSettle(t, env, owner, org, entityID, body, StatusFailed)

	var validationCount, txnCount int
	err := env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM import_validations WHERE batch_id = $1`,
			pgtype.UUID{Bytes: batchID, Valid: true}).Scan(&validationCount); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE batch_id = $1`,
			pgtype.UUID{Bytes: batchID, Valid: true}).Scan(&txnCount)
	})
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	// An unrecognised format never reaches validation at all (design D3 in
	// add-file-upload: 'failed' is a defect in us, decided before this
	// change's own checks run), so there is deliberately no receipt here --
	// asserting that is what proves 'failed' and 'rejected' really are kept
	// apart rather than collapsed.
	if validationCount != 0 {
		t.Errorf("import_validations rows for a 'failed' (not rejected) batch = %d, want 0", validationCount)
	}
	if txnCount != 0 {
		t.Errorf("transactions rows = %d, want 0", txnCount)
	}
}

// The other half of task 6.7: a batch that *does* reach validation and gets
// rejected -- a genuine correctness error in an otherwise-recognisable
// Priorbank export, through the real pipeline -- still leaves exactly one
// receipt and contributes no transaction.
func TestRealCorrectnessErrorReachesRejectedWithAReceipt(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	const doc = "Приорбанк Открытое акционерное общество, БИК PJCBBY2X;01.01.2026;\n" +
		"\n" +
		"Дата док.;N док.;Код опер;Корреспондент.Код;Корреспондент.Счет;Корреспондент.Название;Номинал.Дебет;Номинал.Кредит;Назначение;\n" +
		"02.01.2026;2;100;EUR;BY00TEST2;BAD ROW;NOTANUMBER;0,00;payment two;\n"

	batchID := uploadAndSettle(t, env, owner, org, entityID, []byte(doc), StatusRejected)

	report, err := env.svc.GetValidationReport(ctx, owner, org.UUID(), batchID)
	if err != nil {
		t.Fatalf("GetValidationReport: %v", err)
	}
	if report.Outcome != OutcomeRejected {
		t.Errorf("outcome = %s, want rejected", report.Outcome)
	}
	if !containsCode(report.Errors, CodeAmountUnparseable) {
		t.Errorf("errors = %+v, want %s", report.Errors, CodeAmountUnparseable)
	}

	var validationCount, txnCount int
	err = env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM import_validations WHERE batch_id = $1`,
			pgtype.UUID{Bytes: batchID, Valid: true}).Scan(&validationCount); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE batch_id = $1`,
			pgtype.UUID{Bytes: batchID, Valid: true}).Scan(&txnCount)
	})
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if validationCount != 1 {
		t.Errorf("import_validations rows = %d, want exactly 1 -- the receipt", validationCount)
	}
	if txnCount != 0 {
		t.Errorf("transactions rows = %d, want 0 -- a rejected batch persists no transaction", txnCount)
	}
}
