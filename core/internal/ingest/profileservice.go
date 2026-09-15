package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
)

// isUniqueViolation and isForeignKeyViolation check a Postgres error by
// SQLSTATE rather than by message, the same reasoning core/internal/db's own
// tests use it for: a wording change in a later Postgres release should not
// break either check.
func isUniqueViolation(err error) bool     { return pgErrorCode(err) == "23505" }
func isForeignKeyViolation(err error) bool { return pgErrorCode(err) == "23503" }

func pgErrorCode(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ""
	}
	return pgErr.Code
}

// ErrProfileNotFound is returned when no import_profiles row exists for the
// given id -- row-level security makes this the same answer whether the
// profile never existed or belongs to another organisation (task 6.7: a
// policy denial, not a distinguishable not-found).
var ErrProfileNotFound = errors.New("ingest: no such import profile")

// ErrProfileNameTaken mirrors import_profiles' own UNIQUE (org_id, name).
var ErrProfileNameTaken = errors.New("ingest: an import profile with that name already exists")

// ErrInvalidCanonicalField names a column_map value migration 011's
// constraint trigger refused -- a code, surfaced from the write itself
// (task 6.4), not discovered later when a file is parsed with the profile.
type ErrInvalidCanonicalField struct{ Field string }

func (e *ErrInvalidCanonicalField) Error() string {
	return fmt.Sprintf("ingest: %q is not a canonical field", e.Field)
}

// ErrProfileSourceKindMismatch is task 6.5: a profile's source_kind must
// agree with the batch's.
var ErrProfileSourceKindMismatch = errors.New("ingest: profile source_kind does not match the batch's")

// ErrProfileInUse mirrors import_batches.import_profile_id's own ON DELETE
// RESTRICT (task 6.9): deleting a profile a batch was parsed with would
// delete the record of how that batch's numbers were produced.
var ErrProfileInUse = errors.New("ingest: this profile has been used by an import and cannot be deleted")

// invalidCanonicalFieldMarker is the distinctive text migration 011's own
// RAISE EXCEPTION carries. import_profiles' other CHECK constraints (name,
// delimiter, decimal_sep) also raise 23514, so the SQLSTATE alone cannot
// tell them apart -- a constraint trigger's RAISE does not populate
// pgconn.PgError's ConstraintName the way a plain CHECK violation does. This
// message is this package's own wording, not Postgres's, so matching it is
// no more fragile than the trigger and this file drifting apart, which
// TestCanonicalFieldListMatchesTheMigrationsTrigger already guards against.
const invalidCanonicalFieldMarker = "which is not a canonical field"

// Profile is the domain view of an import_profiles row.
type Profile struct {
	ID         uuid.UUID
	Name       string
	SourceKind string
	ColumnMap  ColumnMap
	Charset    string
	Delimiter  string
	DecimalSep string
	DateFormat string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ProfileInput is what Create and Update both take. Update leaves
// SourceKind untouched -- UpdateImportProfile's own query does not set it,
// the same reasoning as import_batches' immutable source_kind.
type ProfileInput struct {
	Name       string
	SourceKind string // ignored by UpdateImportProfile
	ColumnMap  ColumnMap
	Charset    string
	Delimiter  string
	DecimalSep string
	DateFormat string
}

// resolvedParameters is what a batch's resolved_parameters column stores
// (task 5.3): what it was actually parsed with, independent of a profile
// that may be edited afterward.
type resolvedParameters struct {
	Charset    string    `json:"charset,omitempty"`
	Delimiter  string    `json:"delimiter,omitempty"`
	DecimalSep string    `json:"decimal_sep,omitempty"`
	DateFormat string    `json:"date_format,omitempty"`
	ColumnMap  ColumnMap `json:"column_map,omitempty"`
}

func (s *Service) CreateImportProfile(ctx context.Context, userID, requestedOrgID uuid.UUID, in ProfileInput) (Profile, error) {
	org, _, err := s.resolveOrg(ctx, userID, requestedOrgID)
	if err != nil {
		return Profile{}, err
	}
	if err := validateProfileInput(in, true); err != nil {
		return Profile{}, err
	}

	columnMapJSON, err := json.Marshal(in.ColumnMap)
	if err != nil {
		return Profile{}, fmt.Errorf("ingest: marshaling column_map: %w", err)
	}

	var row gendb.ImportProfile
	err = s.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, err = gendb.New(tx).InsertImportProfile(ctx, gendb.InsertImportProfileParams{
			OrgID:      pgtype.UUID{Bytes: org.UUID(), Valid: true},
			Name:       in.Name,
			SourceKind: in.SourceKind,
			ColumnMap:  columnMapJSON,
			Charset:    textOrNull(in.Charset),
			Delimiter:  textOrNull(in.Delimiter),
			DecimalSep: textOrNull(in.DecimalSep),
			DateFmt:    textOrNull(in.DateFormat),
		})
		return err
	})
	if err != nil {
		return Profile{}, translateProfileWriteError(err)
	}
	return profileFromRow(row)
}

func (s *Service) GetImportProfile(ctx context.Context, userID, requestedOrgID, profileID uuid.UUID) (Profile, error) {
	org, _, err := s.resolveOrg(ctx, userID, requestedOrgID)
	if err != nil {
		return Profile{}, err
	}
	var row gendb.ImportProfile
	err = s.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, err = gendb.New(tx).GetImportProfile(ctx, pgtype.UUID{Bytes: profileID, Valid: true})
		return err
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Profile{}, ErrProfileNotFound
	case err != nil:
		return Profile{}, fmt.Errorf("ingest: get_import_profile: %w", err)
	}
	return profileFromRow(row)
}

func (s *Service) ListImportProfiles(ctx context.Context, userID, requestedOrgID uuid.UUID) ([]Profile, error) {
	org, _, err := s.resolveOrg(ctx, userID, requestedOrgID)
	if err != nil {
		return nil, err
	}
	var rows []gendb.ImportProfile
	err = s.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = gendb.New(tx).ListImportProfiles(ctx)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("ingest: list_import_profiles: %w", err)
	}
	profiles := make([]Profile, len(rows))
	for i, row := range rows {
		p, err := profileFromRow(row)
		if err != nil {
			return nil, err
		}
		profiles[i] = p
	}
	return profiles, nil
}

func (s *Service) UpdateImportProfile(ctx context.Context, userID, requestedOrgID, profileID uuid.UUID, in ProfileInput) (Profile, error) {
	org, _, err := s.resolveOrg(ctx, userID, requestedOrgID)
	if err != nil {
		return Profile{}, err
	}
	if err := validateProfileInput(in, false); err != nil {
		return Profile{}, err
	}

	columnMapJSON, err := json.Marshal(in.ColumnMap)
	if err != nil {
		return Profile{}, fmt.Errorf("ingest: marshaling column_map: %w", err)
	}

	var row gendb.ImportProfile
	err = s.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)
		affected, err := q.UpdateImportProfile(ctx, gendb.UpdateImportProfileParams{
			ID:         pgtype.UUID{Bytes: profileID, Valid: true},
			Name:       in.Name,
			ColumnMap:  columnMapJSON,
			Charset:    textOrNull(in.Charset),
			Delimiter:  textOrNull(in.Delimiter),
			DecimalSep: textOrNull(in.DecimalSep),
			DateFmt:    textOrNull(in.DateFormat),
		})
		if err := db.ExactlyOneRow(affected, err); err != nil {
			return err
		}
		row, err = q.GetImportProfile(ctx, pgtype.UUID{Bytes: profileID, Valid: true})
		return err
	})
	switch {
	case errors.Is(err, db.ErrNoRowsAffected), errors.Is(err, pgx.ErrNoRows):
		return Profile{}, ErrProfileNotFound
	case err != nil:
		return Profile{}, translateProfileWriteError(err)
	}
	return profileFromRow(row)
}

func (s *Service) DeleteImportProfile(ctx context.Context, userID, requestedOrgID, profileID uuid.UUID) error {
	org, _, err := s.resolveOrg(ctx, userID, requestedOrgID)
	if err != nil {
		return err
	}
	err = s.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		affected, err := gendb.New(tx).DeleteImportProfile(ctx, pgtype.UUID{Bytes: profileID, Valid: true})
		return db.ExactlyOneRow(affected, err)
	})
	switch {
	case errors.Is(err, db.ErrNoRowsAffected):
		return ErrProfileNotFound
	case isForeignKeyViolation(err):
		return ErrProfileInUse
	case err != nil:
		return fmt.Errorf("ingest: delete_import_profile: %w", err)
	}
	return nil
}

// validateProfileInput checks the shape this package owns before the write
// even reaches the database -- name and source_kind are still enforced by
// their own CHECK constraints regardless, the same "clean code, and the
// constraint underneath it" shape as OverrideValidation's checks.
func validateProfileInput(in ProfileInput, requireSourceKind bool) error {
	if strings.TrimSpace(in.Name) == "" {
		return &ErrInvalidArgument{Code: "invalid_argument"}
	}
	if requireSourceKind && in.SourceKind != SourceKindLedger && in.SourceKind != SourceKindBank {
		return &ErrInvalidArgument{Code: "invalid_argument"}
	}
	if len(in.ColumnMap) == 0 {
		return &ErrInvalidArgument{Code: "invalid_argument"}
	}
	return nil
}

func translateProfileWriteError(err error) error {
	if strings.Contains(err.Error(), invalidCanonicalFieldMarker) {
		return &ErrInvalidCanonicalField{Field: extractBadField(err.Error())}
	}
	if isUniqueViolation(err) {
		return ErrProfileNameTaken
	}
	return fmt.Errorf("ingest: writing import profile: %w", err)
}

// extractBadField pulls the field name back out of migration 011's own
// error message ("column_map names % which is not a canonical field") for a
// caller that wants to name it. Best-effort: if the shape ever changes, the
// zero value is still a correctly-typed, if less specific, error.
func extractBadField(msg string) string {
	const prefix = "column_map names "
	i := strings.Index(msg, prefix)
	if i < 0 {
		return ""
	}
	rest := msg[i+len(prefix):]
	j := strings.Index(rest, " "+invalidCanonicalFieldMarker)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func profileFromRow(row gendb.ImportProfile) (Profile, error) {
	var cm ColumnMap
	if len(row.ColumnMap) > 0 {
		if err := json.Unmarshal(row.ColumnMap, &cm); err != nil {
			return Profile{}, fmt.Errorf("ingest: parsing column_map: %w", err)
		}
	}
	return Profile{
		ID:         uuid.UUID(row.ID.Bytes),
		Name:       row.Name,
		SourceKind: row.SourceKind,
		ColumnMap:  cm,
		Charset:    row.Charset.String,
		Delimiter:  row.Delimiter.String,
		DecimalSep: row.DecimalSep.String,
		DateFormat: row.DateFmt.String,
		CreatedAt:  row.CreatedAt.Time,
		UpdatedAt:  row.UpdatedAt.Time,
	}, nil
}

func textOrNull(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

// toParameters builds what validatejob.go actually parses and validates
// with: the profile's own fields, unconverted -- a zero field already means
// "let the detector decide" in both Profile and Parameters (design D1), so
// this is a type change (Charset is a Charset, Delimiter is a single byte),
// not a decision.
func (p Profile) toParameters() *Parameters {
	var delim byte
	if len(p.Delimiter) > 0 {
		delim = p.Delimiter[0]
	}
	return &Parameters{
		Charset:    Charset(p.Charset),
		Delimiter:  delim,
		DecimalSep: p.DecimalSep,
		DateFormat: p.DateFormat,
		ColumnMap:  p.ColumnMap,
	}
}
