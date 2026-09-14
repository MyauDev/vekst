package ledger

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
)

// InsertClassification stores one classification. It is always born live --
// SupersededBy is never set here, only by SupersedeClassification -- which
// is design D4's "a correction is an insert plus a pointer" in code: there
// is no third way to produce a live row.
func InsertClassification(ctx context.Context, tx pgx.Tx, org db.OrgID, c Classification) (Classification, error) {
	confidence, err := numericFromConfidence(c.Confidence)
	if err != nil {
		return Classification{}, fmt.Errorf("ledger: confidence: %w", err)
	}
	row, err := gendb.New(tx).InsertClassification(ctx, gendb.InsertClassificationParams{
		OrgID:            pgtype.UUID{Bytes: org.UUID(), Valid: true},
		TransactionID:    pgtype.UUID{Bytes: c.TransactionID, Valid: true},
		CategoryID:       pgtype.UUID{Bytes: c.CategoryID, Valid: true},
		EngineLayer:      c.EngineLayer,
		Confidence:       confidence,
		Evidence:         c.Evidence,
		TaxonomyVersion:  c.TaxonomyVersion,
		RulesetVersion:   c.RulesetVersion,
		EngineVersion:    c.EngineVersion,
		NormalizeVersion: c.NormalizeVersion,
		DecidedBy:        pgtype.UUID{Bytes: c.DecidedBy, Valid: c.DecidedBy != uuid.Nil},
	})
	if err != nil {
		return Classification{}, fmt.Errorf("ledger: inserting classification: %w", err)
	}
	return classificationFromRow(row)
}

// SupersedeClassification stores c as a new, live classification, then
// points oldID at it -- an ordinary insert followed by an ordinary update,
// both inside the caller's own db.InTx. The moment between them where both
// the old and the new row are live is safe because
// classifications_one_live_per_transaction (migration 007) is a deferred
// constraint trigger, not a plain unique index: it is checked once, at
// commit, by which point this function has already run both statements.
// See that trigger's own comment, and SupersedeClassification's SQL, for
// why that distinction is load-bearing rather than incidental.
func SupersedeClassification(ctx context.Context, tx pgx.Tx, org db.OrgID, oldID uuid.UUID, c Classification) (Classification, error) {
	live, err := InsertClassification(ctx, tx, org, c)
	if err != nil {
		return Classification{}, fmt.Errorf("ledger: superseding classification: %w", err)
	}
	affected, err := gendb.New(tx).SupersedeClassification(ctx, gendb.SupersedeClassificationParams{
		ID:           pgtype.UUID{Bytes: oldID, Valid: true},
		SupersededBy: pgtype.UUID{Bytes: live.ID, Valid: true},
	})
	if err := db.ExactlyOneRow(affected, err); err != nil {
		return Classification{}, fmt.Errorf("ledger: pointing %s at its replacement: %w", oldID, err)
	}
	return live, nil
}

// CurrentClassification reads the one live classification for a
// transaction. ErrNoRows (via pgx) if none exists yet.
func CurrentClassification(ctx context.Context, tx pgx.Tx, transactionID uuid.UUID) (Classification, error) {
	row, err := gendb.New(tx).CurrentClassification(ctx, pgtype.UUID{Bytes: transactionID, Valid: true})
	if err != nil {
		return Classification{}, err
	}
	return classificationFromRow(row)
}

func classificationFromRow(row gendb.Classification) (Classification, error) {
	confidence, err := confidenceFromNumeric(row.Confidence)
	if err != nil {
		return Classification{}, fmt.Errorf("ledger: confidence: %w", err)
	}
	c := Classification{
		ID:               uuid.UUID(row.ID.Bytes),
		TransactionID:    uuid.UUID(row.TransactionID.Bytes),
		CategoryID:       uuid.UUID(row.CategoryID.Bytes),
		EngineLayer:      row.EngineLayer,
		Confidence:       confidence,
		Evidence:         row.Evidence,
		TaxonomyVersion:  row.TaxonomyVersion,
		RulesetVersion:   row.RulesetVersion,
		EngineVersion:    row.EngineVersion,
		NormalizeVersion: row.NormalizeVersion,
		DecidedAt:        row.DecidedAt.Time,
	}
	if row.DecidedBy.Valid {
		c.DecidedBy = uuid.UUID(row.DecidedBy.Bytes)
	}
	if row.SupersededBy.Valid {
		c.SupersededBy = uuid.UUID(row.SupersededBy.Bytes)
	}
	return c, nil
}
