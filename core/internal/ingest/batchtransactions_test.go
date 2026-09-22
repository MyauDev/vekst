package ingest

import (
	"context"
	"errors"
	"testing"
)

// The rows a batch persisted, back in file order, each carrying whatever
// classification it has right now -- the whole reason ListBatchTransactions
// exists: a person with the file open, checking what this product did with
// it, one row at a time.
func TestListBatchTransactions(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	doc := priorbankDoc("03.03.2026 09:00:00", "BY00ROWS", "NOK",
		row("05.01.2026", "1", "CP ONE", "0,00", "50,00", "first row"),
		row("06.01.2026", "2", "CP TWO", "30,00", "0,00", "second row"),
	)
	batchID := uploadAndSettle(t, env, owner, org, entityID, doc, StatusImported)

	rows, err := env.svc.ListBatchTransactions(ctx, owner, org.UUID(), batchID)
	if err != nil {
		t.Fatalf("ListBatchTransactions: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	// File order, not booked-on or amount order -- line_no is where a
	// person would go looking, in the file they still have open.
	if rows[0].LineNo >= rows[1].LineNo {
		t.Errorf("rows are not in file order: line_no %d then %d", rows[0].LineNo, rows[1].LineNo)
	}

	first, second := rows[0], rows[1]
	if first.CounterpartyRaw != "CP ONE" || first.Description != "first row" {
		t.Errorf("first row = %+v, want counterparty CP ONE, description %q", first, "first row")
	}
	if first.Amount.MinorUnits != 5000 {
		t.Errorf("first row amount = %d minor units, want 5000 (50,00 credit)", first.Amount.MinorUnits)
	}
	if second.CounterpartyRaw != "CP TWO" || second.Description != "second row" {
		t.Errorf("second row = %+v, want counterparty CP TWO, description %q", second, "second row")
	}
	if second.Amount.MinorUnits != -3000 {
		t.Errorf("second row amount = %d minor units, want -3000 (30,00 debit)", second.Amount.MinorUnits)
	}

	// The classification is a LEFT JOIN: present or absent together, never
	// half of one. Which side it lands on depends on whether classify_run
	// (async, enqueued after persist) has completed by the time this reads
	// -- a race this test does not resolve and should not assert a side of.
	for _, r := range rows {
		hasCategory := r.CategoryCode != ""
		hasLayer := r.EngineLayer != ""
		if hasCategory != hasLayer {
			t.Errorf("row %d: category_code %q, engine_layer %q -- classification fields disagree on whether one exists",
				r.LineNo, r.CategoryCode, r.EngineLayer)
		}
	}
}

func TestListBatchTransactionsNotAMember(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	doc := priorbankDoc("03.03.2026 09:00:00", "BY00MEMBER", "NOK",
		row("05.01.2026", "1", "CP ONE", "0,00", "50,00", "row"),
	)
	batchID := uploadAndSettle(t, env, owner, org, entityID, doc, StatusImported)

	otherOwner := testUser(t, env.db)
	_, err := env.svc.ListBatchTransactions(ctx, otherOwner, org.UUID(), batchID)
	if !errors.Is(err, ErrNotAMember) {
		t.Errorf("ListBatchTransactions as a non-member = %v, want ErrNotAMember", err)
	}
}

// A batch with no rows -- rejected before persist, or genuinely empty --
// answers with an empty list, not an error: "nothing to show" and "could
// not read the batch" are different claims about very different problems.
func TestListBatchTransactionsEmptyBatch(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	doc := priorbankDoc("03.03.2026 09:00:00", "BY00EMPTY", "NOK")
	batchID := uploadAndSettle(t, env, owner, org, entityID, doc, StatusImported)

	rows, err := env.svc.ListBatchTransactions(ctx, owner, org.UUID(), batchID)
	if err != nil {
		t.Fatalf("ListBatchTransactions: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows for an empty statement, want 0", len(rows))
	}
}
