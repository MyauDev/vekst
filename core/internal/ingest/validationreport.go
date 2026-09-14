package ingest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
)

// overrideReasonMinLength mirrors migration 010's own
// length(btrim(override_reason)) >= 10 -- checked here too so a caller gets
// a clean error code (task 5.3) rather than a constraint violation, though
// the constraint is what actually makes it true regardless of this check
// (design D1, task 6.8).
const overrideReasonMinLength = 10

// ErrValidationNotFound is GetValidationReport and OverrideValidation's
// answer when no import_validations row exists for the batch -- not yet
// validated, or row-level security filtered it (task 6.10: a policy denial,
// not a distinguishable not-found).
var ErrValidationNotFound = errors.New("ingest: no validation for that batch")

// ErrOverrideReasonTooShort names the check migration 010's own CHECK
// constraint enforces regardless.
var ErrOverrideReasonTooShort = errors.New("ingest: override reason must be at least 10 characters")

// ErrOverrideRequiresApproverRole is change 2.3's own decision (task 0.2,
// confirmed 2026-09-13): pulled forward from Product's future role
// enforcement rather than left unchecked for the Demo.
var ErrOverrideRequiresApproverRole = errors.New("ingest: override requires an approver role")

// ErrAlreadyOverridden: an override is written once (design D4). A second
// attempt is refused here for a clean error code; RecordOverride's own
// unconditional UPDATE would otherwise silently rewrite it.
var ErrAlreadyOverridden = errors.New("ingest: already overridden")

// ErrOverrideOnlyOverWarnings names the other half of design D1's
// constraint: a correctness error (outcome = rejected) can never be
// overridden. import_validations_override_only_over_warnings enforces this
// independently of this check (task 6.8).
var ErrOverrideOnlyOverWarnings = errors.New("ingest: only a batch with warnings, not errors, may be overridden")

// ValidationReport is the domain view of an import_validations row.
type ValidationReport struct {
	BatchID            uuid.UUID
	Outcome            Outcome
	RowCount           int
	ErrorCount         int
	WarningCount       int
	BalanceCheckPassed *bool
	Errors             []ValidationError
	Warnings           []ValidationWarning

	// OverriddenBy is uuid.Nil when the batch has never been overridden.
	OverriddenBy   uuid.UUID
	OverrideReason string
	OverriddenAt   *time.Time
}

// canOverride is task 0.2's decision: owner, admin or approver may override;
// viewer may not. An override is a financial control -- the same reasoning
// that puts it behind a role at all puts it behind every role capable of
// running the organisation, not "approver" read as the one literal string.
func canOverride(role db.Role) bool {
	switch role {
	case "owner", "admin", "approver":
		return true
	default:
		return false
	}
}

// GetValidationReport reads one batch's validation.
func (s *Service) GetValidationReport(ctx context.Context, userID, requestedOrgID, batchID uuid.UUID) (ValidationReport, error) {
	org, _, err := s.resolveOrg(ctx, userID, requestedOrgID)
	if err != nil {
		return ValidationReport{}, err
	}

	var row gendb.ImportValidation
	err = s.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, err = gendb.New(tx).GetValidationForBatch(ctx, pgtype.UUID{Bytes: batchID, Valid: true})
		return err
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ValidationReport{}, ErrValidationNotFound
	case err != nil:
		return ValidationReport{}, fmt.Errorf("ingest: get_validation_report: %w", err)
	}
	return reportFromRow(row)
}

// OverrideValidation records an override and returns the updated report.
// design D1's whole point: a correctness error is unrepresentable here, not
// merely refused -- import_validations_override_only_over_warnings is what
// actually holds if this check is ever bypassed.
func (s *Service) OverrideValidation(ctx context.Context, userID, requestedOrgID, batchID uuid.UUID, reason string) (ValidationReport, error) {
	org, role, err := s.resolveOrg(ctx, userID, requestedOrgID)
	if err != nil {
		return ValidationReport{}, err
	}
	if !canOverride(role) {
		return ValidationReport{}, ErrOverrideRequiresApproverRole
	}
	if len(strings.TrimSpace(reason)) < overrideReasonMinLength {
		return ValidationReport{}, ErrOverrideReasonTooShort
	}

	var row gendb.ImportValidation
	err = s.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)
		batchPg := pgtype.UUID{Bytes: batchID, Valid: true}

		current, err := q.GetValidationForBatch(ctx, batchPg)
		if err != nil {
			return err
		}
		if current.OverriddenAt.Valid {
			return ErrAlreadyOverridden
		}
		if current.Outcome != string(OutcomeValidWithWarnings) {
			return ErrOverrideOnlyOverWarnings
		}

		affected, err := q.RecordOverride(ctx, gendb.RecordOverrideParams{
			BatchID:        batchPg,
			OverriddenBy:   pgtype.UUID{Bytes: userID, Valid: true},
			OverrideReason: pgtype.Text{String: reason, Valid: true},
		})
		if err := db.ExactlyOneRow(affected, err); err != nil {
			return err
		}

		row, err = q.GetValidationForBatch(ctx, batchPg)
		return err
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ValidationReport{}, ErrValidationNotFound
	case errors.Is(err, ErrAlreadyOverridden), errors.Is(err, ErrOverrideOnlyOverWarnings):
		return ValidationReport{}, err
	case err != nil:
		return ValidationReport{}, fmt.Errorf("ingest: override_validation: %w", err)
	}
	return reportFromRow(row)
}

func reportFromRow(row gendb.ImportValidation) (ValidationReport, error) {
	errs, warns, err := ParseReportJSON(row.ReportJsonb)
	if err != nil {
		return ValidationReport{}, fmt.Errorf("ingest: parsing report_jsonb: %w", err)
	}

	var balancePassed *bool
	if row.BalanceCheckPassed.Valid {
		b := row.BalanceCheckPassed.Bool
		balancePassed = &b
	}
	var overriddenAt *time.Time
	if row.OverriddenAt.Valid {
		t := row.OverriddenAt.Time
		overriddenAt = &t
	}
	var overriddenBy uuid.UUID
	if row.OverriddenBy.Valid {
		overriddenBy = uuid.UUID(row.OverriddenBy.Bytes)
	}

	return ValidationReport{
		BatchID:            uuid.UUID(row.BatchID.Bytes),
		Outcome:            Outcome(row.Outcome),
		RowCount:           int(row.RowCount),
		ErrorCount:         int(row.ErrorCount),
		WarningCount:       int(row.WarningCount),
		BalanceCheckPassed: balancePassed,
		Errors:             errs,
		Warnings:           warns,
		OverriddenBy:       overriddenBy,
		OverrideReason:     row.OverrideReason.String,
		OverriddenAt:       overriddenAt,
	}, nil
}
