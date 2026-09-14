package ingest

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/MyauDev/vekst/core/internal/dedup"
)

// GetDedupSummary, ListSkippedRows, ListInternalTransfers and
// DismissInternalTransfer, exercised end to end through a real batch: the
// same fixture TestD3SkipsACrossBatchDuplicateAndNamesIt already proves
// produces one skip, so the summary and the list are checked against a
// batch that is not simply empty.
func TestDedupServiceMethods(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	doc := priorbankDoc("03.03.2026 09:00:00", "BY00SUMMARY", "NOK",
		row("05.01.2026", "1", "CP ONE", "0,00", "50,00", "rent"),
		row("06.01.2026", "2", "CP TWO", "0,00", "30,00", "utilities"),
	)
	batchID := uploadAndSettle(t, env, owner, org, entityID, doc, StatusImported)

	t.Run("GetDedupSummary", func(t *testing.T) {
		summary, err := env.svc.GetDedupSummary(ctx, owner, org.UUID(), batchID)
		if err != nil {
			t.Fatalf("GetDedupSummary: %v", err)
		}
		if summary.ImportedRows != 2 {
			t.Errorf("ImportedRows = %d, want 2", summary.ImportedRows)
		}
		if summary.SkippedInBatch != 0 || summary.SkippedCrossBatch != 0 {
			t.Errorf("skips = %+v, want zero for a first-time import", summary)
		}
	})

	t.Run("GetDedupSummary, not a member", func(t *testing.T) {
		otherOwner := testUser(t, env.db)
		_, err := env.svc.GetDedupSummary(ctx, otherOwner, org.UUID(), batchID)
		if !errors.Is(err, ErrNotAMember) {
			t.Errorf("GetDedupSummary as a non-member = %v, want ErrNotAMember", err)
		}
	})

	t.Run("ListSkippedRows", func(t *testing.T) {
		skips, err := env.svc.ListSkippedRows(ctx, owner, org.UUID(), batchID)
		if err != nil {
			t.Fatalf("ListSkippedRows: %v", err)
		}
		if len(skips) != 0 {
			t.Errorf("got %d skips, want 0", len(skips))
		}
	})

	t.Run("ListSkippedRows, not a member", func(t *testing.T) {
		otherOwner := testUser(t, env.db)
		_, err := env.svc.ListSkippedRows(ctx, otherOwner, org.UUID(), batchID)
		if !errors.Is(err, ErrNotAMember) {
			t.Errorf("ListSkippedRows as a non-member = %v, want ErrNotAMember", err)
		}
	})

	t.Run("ListInternalTransfers", func(t *testing.T) {
		transfers, err := env.svc.ListInternalTransfers(ctx, owner, org.UUID(), entityID)
		if err != nil {
			t.Fatalf("ListInternalTransfers: %v", err)
		}
		if len(transfers) != 0 {
			t.Errorf("got %d transfers, want 0", len(transfers))
		}
	})

	t.Run("ListInternalTransfers, not a member", func(t *testing.T) {
		otherOwner := testUser(t, env.db)
		_, err := env.svc.ListInternalTransfers(ctx, otherOwner, org.UUID(), entityID)
		if !errors.Is(err, ErrNotAMember) {
			t.Errorf("ListInternalTransfers as a non-member = %v, want ErrNotAMember", err)
		}
	})

	t.Run("DismissInternalTransfer, not found", func(t *testing.T) {
		_, err := env.svc.DismissInternalTransfer(ctx, owner, org.UUID(), uuid.New())
		if !errors.Is(err, dedup.ErrTransferNotFound) {
			t.Errorf("DismissInternalTransfer on a nonexistent transfer = %v, want dedup.ErrTransferNotFound", err)
		}
	})

	t.Run("DismissInternalTransfer, not a member", func(t *testing.T) {
		otherOwner := testUser(t, env.db)
		_, err := env.svc.DismissInternalTransfer(ctx, otherOwner, org.UUID(), uuid.New())
		if !errors.Is(err, ErrNotAMember) {
			t.Errorf("DismissInternalTransfer as a non-member = %v, want ErrNotAMember", err)
		}
	})
}

// The other half of D4/D5, end to end through the Service layer rather
// than through dedup's own package tests: a real pair, actually dismissed.
func TestDismissInternalTransferReturnsThePairToTheReports(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	// Two accounts, one statement each, an outgoing and an incoming leg two
	// days apart -- a transfer detection candidate.
	docOut := priorbankDoc("04.04.2026 09:00:00", "BY00TROUT", "NOK",
		row("10.01.2026", "1", "SELF", "100,00", "0,00", "internal move"))
	docIn := priorbankDoc("04.04.2026 09:05:00", "BY00TRIN", "NOK",
		row("12.01.2026", "1", "SELF", "0,00", "100,00", "internal move"))

	uploadAndSettle(t, env, owner, org, entityID, docOut, StatusImported)
	uploadAndSettle(t, env, owner, org, entityID, docIn, StatusImported)

	transfers, err := env.svc.ListInternalTransfers(ctx, owner, org.UUID(), entityID)
	if err != nil {
		t.Fatalf("ListInternalTransfers: %v", err)
	}
	if len(transfers) != 1 {
		t.Fatalf("got %d transfers, want 1", len(transfers))
	}

	dismissed, err := env.svc.DismissInternalTransfer(ctx, owner, org.UUID(), transfers[0].ID)
	if err != nil {
		t.Fatalf("DismissInternalTransfer: %v", err)
	}
	if dismissed.DismissedBy != owner {
		t.Errorf("DismissedBy = %s, want %s", dismissed.DismissedBy, owner)
	}

	after, err := env.svc.ListInternalTransfers(ctx, owner, org.UUID(), entityID)
	if err != nil {
		t.Fatalf("ListInternalTransfers after dismissal: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("got %d active transfers after dismissal, want 0", len(after))
	}
}
