package ingest

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/dedup"
)

// priorbankDoc builds a synthetic but validly-shaped Priorbank export:
// enough preamble for ResolveAccount (a "Счет клиента" line) and period
// detection to succeed, no declared balances (so the balance check is
// null, not failed -- these tests are not about D-1/D-2/D-3's own
// completeness checks). timestamp only affects the file's own bytes, never
// its parsed content, which is what lets two calls with different
// timestamps produce two different file_sha256 values over otherwise
// identical rows -- exactly what a D3 test needs.
func priorbankDoc(timestamp, account, currency string, rows ...string) []byte {
	doc := "Приорбанк Открытое акционерное общество, БИК PJCBBY2X;" + timestamp + ";\n" +
		"ВЫПИСКА ПО СЧЕТУ " + account + " С 01.01.2026 ПО 31.01.2026;\n" +
		"Счет клиента*************** " + account + " " + currency + " Пассивный;\n" +
		"Наименование счета********* TEST HOLDER;\n" +
		"\n" +
		"Дата док.;N док.;Код опер;Корреспондент.Код;Корреспондент.Счет;Корреспондент.Название;Номинал.Дебет;Номинал.Кредит;Назначение;\n"
	for _, r := range rows {
		doc += r + "\n"
	}
	return []byte(doc)
}

// row builds one data row for priorbankDoc. debit and credit are decimal
// strings with a comma, matching the real export's own locale.
func row(bookedOn, docNo, counterparty, debit, credit, description string) string {
	return fmt.Sprintf("%s;%s;100;BANKCODE;BY00OTHER1;%s;%s;%s;%s;",
		bookedOn, docNo, counterparty, debit, credit, description)
}

// summaryRow builds an opening- or closing-balance line. label is the
// literal prefix ParsePriorbank matches on ("Входящее сальдо" or
// "Исходящее сальдо"); debit and credit are the last two numeric-looking
// cells in the row, which is how summaryPair finds them on an account with
// no Эквивалент columns (this package's own synthetic docs never have
// one).
func summaryRow(label, debit, credit string) string {
	return fmt.Sprintf("%s;;;;;;%s;%s;;", label, debit, credit)
}

func countTransactionsForBatch(t *testing.T, env *testEnv, org db.OrgID, batchID uuid.UUID) int {
	t.Helper()
	var n int64
	err := env.db.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE batch_id = $1`,
			pgtype.UUID{Bytes: batchID, Valid: true}).Scan(&n)
	})
	if err != nil {
		t.Fatalf("counting transactions: %v", err)
	}
	return int(n)
}

func countSkipsForBatch(t *testing.T, env *testEnv, org db.OrgID, batchID uuid.UUID, level string) int {
	t.Helper()
	var n int64
	err := env.db.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM dedup_skips WHERE batch_id = $1 AND level = $2`,
			pgtype.UUID{Bytes: batchID, Valid: true}, level).Scan(&n)
	})
	if err != nil {
		t.Fatalf("counting %s skips: %v", level, err)
	}
	return int(n)
}

// --- D1: the same file twice -----------------------------------------------

// Task 6.1: D1 is a guarantee. Bypasses the measurement job's own early
// check (task 3.1) entirely -- inserting a second batch and setting it to
// imported directly, with the first batch's own file_sha256 -- to prove
// the guarantee is import_batches_file_once (migration 012), not
// carefulness in application code that happens to check first.
func TestD1IsAGuarantee(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	doc := priorbankDoc("01.01.2026 10:00:00", "BY00ACCT1", "NOK",
		row("05.01.2026", "1", "CP ONE", "0,00", "50,00", "payment one"))
	batch1 := uploadAndSettle(t, env, owner, org, entityID, doc, StatusImported)

	var sha256 []byte
	err := env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT file_sha256 FROM import_batches WHERE id = $1`,
			pgtype.UUID{Bytes: batch1, Valid: true}).Scan(&sha256)
	})
	if err != nil {
		t.Fatalf("reading batch1's file_sha256: %v", err)
	}

	batch2 := uuid.New()
	err = env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO import_batches (
				org_id, id, entity_id, source_kind, status, uploaded_by,
				file_name, declared_bytes, declared_type, file_key,
				file_sha256, byte_length, content_type, upload_expires_at
			) VALUES ($1,$2,$3,'bank','uploaded',$4,'x2.csv',10,'text/csv',$5,$6,10,'text/csv',now())`,
			org.UUID(), batch2, entityID, owner, "other-key-for-"+batch2.String(), sha256)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE import_batches SET status = 'imported' WHERE id = $1`, batch2)
		return err
	})
	if code := pgCode(err); code != "23505" {
		t.Fatalf("a second batch reaching imported with the same file_sha256: SQLSTATE = %q (%v), want 23505", code, err)
	}
}

// Task 6.2: D1 is scoped. The same bytes uploaded by two organisations are
// two imports -- org_id leads import_batches_file_once.
func TestD1IsScopedToOneOrganisation(t *testing.T) {
	env := newTestEnv(t)
	ownerA, ownerB := testUser(t, env.db), testUser(t, env.db)
	orgA, entityA := testOrgAndEntity(t, env.db, ownerA)
	orgB, entityB := testOrgAndEntity(t, env.db, ownerB)

	doc := priorbankDoc("01.01.2026 10:00:00", "BY00ACCT1", "NOK",
		row("05.01.2026", "1", "CP ONE", "0,00", "50,00", "payment one"))

	uploadAndSettle(t, env, ownerA, orgA, entityA, doc, StatusImported)
	uploadAndSettle(t, env, ownerB, orgB, entityB, doc, StatusImported)
}

// Task 6.3: D1 is partial. A batch that never reached imported -- rejected,
// here, on a correctness error -- does not block a later upload of the
// same bytes: only status = 'imported' participates in the index.
func TestD1IsPartial(t *testing.T) {
	env := newTestEnv(t)
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	// A row with neither amount parseable is a correctness error --
	// rejected, never imported. (A malformed *date*, tried first, does not
	// work for this: datePrefix's own regex is what tells a data row from
	// a summary row, so a row failing it is silently not a row at all,
	// rather than a correctness error.)
	doc := priorbankDoc("01.01.2026 10:00:00", "BY00ACCT1", "NOK",
		row("05.01.2026", "1", "CP ONE", "garbage", "garbage", "payment one"))

	batch1 := uploadAndSettle(t, env, owner, org, entityID, doc, StatusRejected)
	final1 := env.rawBatch(t, org, batch1)
	if final1.FailureCode.String == failureAlreadyImported {
		t.Fatal("the first upload of a never-before-seen file was refused as already_imported")
	}

	// The same bytes again: not blocked by D1, because batch1 never
	// reached imported.
	batch2 := uploadAndSettle(t, env, owner, org, entityID, doc, StatusRejected)
	final2 := env.rawBatch(t, org, batch2)
	if final2.FailureCode.String == failureAlreadyImported {
		t.Error("a retry of a rejected file's bytes was refused as already_imported, but D1 only blocks against an imported batch")
	}
}

// --- D2/D3: the same row twice ----------------------------------------------

// Tasks 6.4, 6.7 and 6.8, corrected together. add-transaction-ledger's own
// dedup_hash already includes an occurrence term (design D5) specifically
// so that two genuinely distinct rows sharing every other field -- two
// coffees, same day, same amount, same wording, no bank reference -- do
// not collide. Confirmed with the founder during this change's own
// implementation (see tasks.md 0's note): D2 keeps both rows rather than
// skipping the second, because they are not really duplicates. This test
// is what the original tasks 6.4/6.8 ("imports one, skips one") would have
// asserted, corrected: two rows, identical in every field dedup_hash
// reads, both import, and D2 records nothing.
func TestTwoIndistinguishableRowsInOneFileBothImport(t *testing.T) {
	env := newTestEnv(t)
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	doc := priorbankDoc("01.01.2026 10:00:00", "BY00ACCT1", "NOK",
		row("05.01.2026", "", "CP ONE", "0,00", "50,00", "coffee"),
		row("05.01.2026", "", "CP ONE", "0,00", "50,00", "coffee"),
	)
	batch := uploadAndSettle(t, env, owner, org, entityID, doc, StatusImported)

	if n := countTransactionsForBatch(t, env, org, batch); n != 2 {
		t.Errorf("transactions = %d, want 2 -- two genuinely repeated rows are both real payments", n)
	}
	if n := countSkipsForBatch(t, env, org, batch, dedup.LevelD2); n != 0 {
		t.Errorf("D2 skips = %d, want 0", n)
	}
}

// Task 6.7, as originally stated: two rows differing only by bank
// reference are obviously distinct, and both import. Kept alongside the
// test above to show occurrence is not the only thing that can tell two
// rows apart -- a real bank_ref does too, and always did.
func TestTwoRowsDifferingOnlyByBankRefBothImport(t *testing.T) {
	env := newTestEnv(t)
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	doc := priorbankDoc("01.01.2026 10:00:00", "BY00ACCT1", "NOK",
		row("05.01.2026", "1", "CP ONE", "0,00", "50,00", "coffee"),
		row("05.01.2026", "2", "CP ONE", "0,00", "50,00", "coffee"),
	)
	batch := uploadAndSettle(t, env, owner, org, entityID, doc, StatusImported)

	if n := countTransactionsForBatch(t, env, org, batch); n != 2 {
		t.Errorf("transactions = %d, want 2", n)
	}
}

// Task 6.5: D3. Re-uploading last month's export -- here, a different file
// (a different preamble timestamp, so a different file_sha256, which is
// what keeps D1 out of this test entirely) that happens to repeat one row
// from an already-imported batch -- imports only the new row, and the
// skip names the transaction and batch it matched.
//
// Task 6.9 is proven by the same test: the matched transaction from batch1
// is read back afterward, unchanged -- no code path here deletes it.
func TestD3SkipsACrossBatchDuplicateAndNamesIt(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	sharedRow := row("05.01.2026", "1", "CP ONE", "0,00", "50,00", "rent")
	doc1 := priorbankDoc("01.01.2026 10:00:00", "BY00SHARED", "NOK",
		sharedRow,
		row("06.01.2026", "2", "CP TWO", "0,00", "30,00", "utilities"),
	)
	batch1 := uploadAndSettle(t, env, owner, org, entityID, doc1, StatusImported)
	if n := countTransactionsForBatch(t, env, org, batch1); n != 2 {
		t.Fatalf("batch1 transactions = %d, want 2", n)
	}

	var sharedRowTxnID uuid.UUID
	err := env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id FROM transactions WHERE batch_id = $1 AND bank_ref = '1'`,
			pgtype.UUID{Bytes: batch1, Valid: true}).Scan(&sharedRowTxnID)
	})
	if err != nil {
		t.Fatalf("reading batch1's shared-row transaction: %v", err)
	}

	// doc2: a different file (different preamble timestamp -> different
	// file_sha256), same account, repeats the shared row exactly and adds
	// one new one. Balances declared so task 6.6 can ride along: opening 0
	// + this file's own movements (50 + 20) = closing 70, which must hold
	// regardless of what dedup later decides to skip.
	doc2 := priorbankDoc("02.02.2026 11:00:00", "BY00SHARED", "NOK",
		summaryRow("Входящее сальдо  на  01.01.2026", "0,00", "0,00"),
		sharedRow,
		row("07.01.2026", "3", "CP THREE", "0,00", "20,00", "new payment"),
		summaryRow("Исходящее сальдо  на  31.01.2026", "0,00", "70,00"),
	)
	batch2 := uploadAndSettle(t, env, owner, org, entityID, doc2, StatusImported)

	if n := countTransactionsForBatch(t, env, org, batch2); n != 1 {
		t.Errorf("batch2 transactions = %d, want 1 (only the new row)", n)
	}
	if n := countSkipsForBatch(t, env, org, batch2, dedup.LevelD3); n != 1 {
		t.Fatalf("batch2 D3 skips = %d, want 1", n)
	}

	var skip gendb.DedupSkip
	err = env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := gendb.New(tx).ListDedupSkipsForBatch(ctx, pgtype.UUID{Bytes: batch2, Valid: true})
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			t.Fatalf("got %d skip rows, want 1", len(rows))
		}
		skip = rows[0]
		return nil
	})
	if err != nil {
		t.Fatalf("reading batch2's skip: %v", err)
	}
	if uuid.UUID(skip.MatchedTransactionID.Bytes) != sharedRowTxnID {
		t.Errorf("skip's matched_transaction_id = %s, want %s", uuid.UUID(skip.MatchedTransactionID.Bytes), sharedRowTxnID)
	}
	if uuid.UUID(skip.MatchedBatchID.Bytes) != batch1 {
		t.Errorf("skip's matched_batch_id = %s, want batch1 %s", uuid.UUID(skip.MatchedBatchID.Bytes), batch1)
	}

	// Task 6.6: the balance check ran over every parsed row of batch2's
	// own file, before dedup, and passed -- a skip does not retroactively
	// break reconciliation.
	var validation gendb.ImportValidation
	err = env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		validation, err = gendb.New(tx).GetValidationForBatch(ctx, pgtype.UUID{Bytes: batch2, Valid: true})
		return err
	})
	if err != nil {
		t.Fatalf("GetValidationForBatch: %v", err)
	}
	if !validation.BalanceCheckPassed.Valid || !validation.BalanceCheckPassed.Bool {
		t.Errorf("batch2 balance_check_passed = %+v, want true", validation.BalanceCheckPassed)
	}

	// Task 6.9: skips are never deletes. batch1's own transaction for the
	// shared row still exists, exactly as it was.
	var stillThere int
	err = env.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE id = $1`,
			pgtype.UUID{Bytes: sharedRowTxnID, Valid: true}).Scan(&stillThere)
	})
	if err != nil {
		t.Fatalf("re-reading batch1's transaction: %v", err)
	}
	if stillThere != 1 {
		t.Errorf("batch1's original transaction no longer exists after batch2's persist skipped its duplicate")
	}
}
