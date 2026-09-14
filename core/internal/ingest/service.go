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
	"github.com/MyauDev/vekst/core/internal/blob"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/jobs"
)

// SourceKind values. A CHECK constraint on both import_batches and
// transactions admits exactly these two strings (migration 007) -- this
// package validates against the same two rather than letting a third
// spelling reach the database and fail there instead.
const (
	SourceKindLedger = "ledger"
	SourceKindBank   = "bank"
)

// ErrObjectStoreNotConfigured is CreateImportBatch's answer when D-6 is
// unanswered (design D5, add-file-upload proposal non-goals): an empty
// object-store endpoint disables upload the way an empty Google client ID
// disables sign-in. core still serves; only this one call fails.
var ErrObjectStoreNotConfigured = errors.New("ingest: object store is not configured")

// ErrBatchNotFound is returned by GetImportBatch and ConfirmImportUpload when
// no row exists for the given id -- which row-level security makes the same
// answer whether the batch never existed or belongs to another organisation
// (task 6.1: a policy denial, not a distinguishable not-found).
var ErrBatchNotFound = errors.New("ingest: no such import batch")

// ErrNotAMember mirrors db.ErrNotAMember behind this package's boundary.
// core/internal/server's own guard test (TestHealthPathImportsNoDatabase)
// forbids that package from importing core/internal/db at all, so every
// request here carries a caller's user id and requested org_id rather than an
// already-resolved db.OrgID, and this package is what turns "not a member"
// into something the server package can recognise with errors.Is without
// importing db just to name the sentinel.
var ErrNotAMember = errors.New("ingest: caller does not act for that organisation")

// ErrInvalidArgument names which of CreateImportBatch's arguments failed
// validation, as a code rather than a sentence (CLAUDE.md: the backend
// returns error codes, translation is the client's).
type ErrInvalidArgument struct{ Code string }

func (e *ErrInvalidArgument) Error() string { return e.Code }

// Batch is the domain view of an import_batches row -- what
// core/internal/server translates to and from vekst.v1.ImportBatch, so
// nothing outside this package and server needs to know sqlc's generated
// shapes.
type Batch struct {
	ID          uuid.UUID
	EntityID    uuid.UUID
	SourceKind  string
	Status      Status
	FileName    string
	ByteLength  int64 // 0 until the measurement job has run.
	FailureCode string
	CreatedAt   time.Time
}

// CreateBatchInput is what the caller of CreateImportBatch supplies, aside
// from the authenticated user and the organisation, which travel as
// CreateImportBatch's own parameters rather than fields here -- there is no
// way to construct one from a value a client sent.
type CreateBatchInput struct {
	EntityID      uuid.UUID
	SourceKind    string
	FileName      string
	DeclaredBytes int64
	DeclaredType  string

	// ImportProfileID is named on the batch, never inferred (design D3 in
	// add-import-profiles): uuid.Nil means no profile, the detector decides
	// everything, exactly as before profiles existed.
	ImportProfileID uuid.UUID
}

// PresignedUpload is what the browser needs to PUT the file directly to the
// object store (design D1). Headers deliberately excludes Content-Length --
// see blob.ObjectStore.PresignPut.
type PresignedUpload struct {
	URL       string
	Headers   map[string]string
	ExpiresAt time.Time
}

// Service is the request-handling side of this change: the four RPCs
// ImportService answers. store may be nil, meaning D-6 is unanswered --
// CreateImportBatch is the only method that needs it, and it fails closed
// with ErrObjectStoreNotConfigured rather than dereferencing a nil interface.
type Service struct {
	database *db.DB
	store    blob.ObjectStore
	jobs     *jobs.Client
	cfg      Config
	now      func() time.Time
}

// NewService wires the request-handling side. It is built after jobs.New
// returns, not passed into it -- see Workers' doc comment for why the cycle
// runs this direction.
func NewService(database *db.DB, store blob.ObjectStore, jobsClient *jobs.Client, cfg Config) *Service {
	return &Service{database: database, store: store, jobs: jobsClient, cfg: cfg, now: time.Now}
}

// resolveOrg is add-tenancy-and-rls design D7's wire shape, exercised for the
// first time: a client names the organisation it wants to act for, and this
// verifies that against the authenticated caller's own memberships before
// anything reaches db.InTx. A org a caller does not belong to is
// ErrNotAMember, not a wider read -- db.OrgIDForSession returns the same
// error whether the organisation exists or the caller is simply not in it.
// The role is returned alongside for the one caller that needs it
// (OverrideValidation); every other caller here discards it.
func (s *Service) resolveOrg(ctx context.Context, userID, requestedOrgID uuid.UUID) (db.OrgID, db.Role, error) {
	org, role, err := s.database.OrgIDForSession(ctx, userID, requestedOrgID)
	if err != nil {
		if errors.Is(err, db.ErrNotAMember) {
			return db.OrgID{}, "", ErrNotAMember
		}
		return db.OrgID{}, "", fmt.Errorf("ingest: resolving organisation: %w", err)
	}
	return org, role, nil
}

// CreateImportBatch reserves a batch in awaiting_upload and returns a
// presigned PUT (design D1). The insert and the expiry job's enqueue share
// one transaction: the job exists if and only if the batch does.
func (s *Service) CreateImportBatch(ctx context.Context, userID, requestedOrgID uuid.UUID, in CreateBatchInput) (Batch, PresignedUpload, error) {
	org, _, err := s.resolveOrg(ctx, userID, requestedOrgID)
	if err != nil {
		return Batch{}, PresignedUpload{}, err
	}
	if s.store == nil {
		return Batch{}, PresignedUpload{}, ErrObjectStoreNotConfigured
	}
	if in.SourceKind != SourceKindLedger && in.SourceKind != SourceKindBank {
		return Batch{}, PresignedUpload{}, &ErrInvalidArgument{Code: "invalid_argument"}
	}
	if strings.TrimSpace(in.FileName) == "" {
		return Batch{}, PresignedUpload{}, &ErrInvalidArgument{Code: "invalid_argument"}
	}
	if in.DeclaredBytes <= 0 {
		return Batch{}, PresignedUpload{}, &ErrInvalidArgument{Code: "invalid_argument"}
	}
	if strings.TrimSpace(in.DeclaredType) == "" {
		return Batch{}, PresignedUpload{}, &ErrInvalidArgument{Code: "invalid_argument"}
	}

	batchID := uuid.New()
	key := blob.Key(org.UUID(), batchID)
	expiresAt := s.now().Add(s.cfg.UploadURLLifetime)

	var row gendb.InsertImportBatchRow
	err = s.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)

		// Named on the batch, never inferred (add-import-profiles design D3).
		// Looked up in the same transaction: row-level security is what makes
		// "belongs to another organisation" and "does not exist" the same
		// answer (task 5.1), the same non-disclosure every composite foreign
		// key in this schema already provides.
		var profilePg pgtype.UUID
		if in.ImportProfileID != uuid.Nil {
			profile, err := q.GetImportProfile(ctx, pgtype.UUID{Bytes: in.ImportProfileID, Valid: true})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrProfileNotFound
				}
				return fmt.Errorf("resolving import profile: %w", err)
			}
			if profile.SourceKind != in.SourceKind {
				return ErrProfileSourceKindMismatch
			}
			profilePg = pgtype.UUID{Bytes: in.ImportProfileID, Valid: true}
		}

		var err error
		row, err = q.InsertImportBatch(ctx, gendb.InsertImportBatchParams{
			OrgID:           pgtype.UUID{Bytes: org.UUID(), Valid: true},
			ID:              pgtype.UUID{Bytes: batchID, Valid: true},
			EntityID:        pgtype.UUID{Bytes: in.EntityID, Valid: true},
			SourceKind:      in.SourceKind,
			UploadedBy:      pgtype.UUID{Bytes: userID, Valid: true},
			FileName:        in.FileName,
			DeclaredBytes:   in.DeclaredBytes,
			DeclaredType:    in.DeclaredType,
			FileKey:         key,
			UploadExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
			ImportProfileID: profilePg,
		})
		if err != nil {
			return fmt.Errorf("inserting batch: %w", err)
		}

		// Scheduled for upload_expires_at, not enqueued to run immediately:
		// a batch is allowed the full UploadURLLifetime to receive its
		// upload before the expiry job is even eligible to look at it.
		return s.jobs.InsertTxAt(ctx, tx, ExpireBatchArgs{
			TenantJobArgs: db.TenantJobArgs{OrgID: org.UUID()},
			BatchID:       batchID,
		}, expiresAt)
	})
	if err != nil {
		return Batch{}, PresignedUpload{}, fmt.Errorf("ingest: create_import_batch: %w", err)
	}

	url, headers, err := s.store.PresignPut(ctx, key, in.DeclaredBytes, in.DeclaredType, s.cfg.UploadURLLifetime)
	if err != nil {
		return Batch{}, PresignedUpload{}, fmt.Errorf("ingest: create_import_batch: presigning: %w", err)
	}

	return batchFromInsertRow(row), PresignedUpload{URL: url, Headers: headers, ExpiresAt: expiresAt}, nil
}

// ConfirmImportUpload enqueues the measurement job and returns the batch as
// it stands right now -- still awaiting_upload, or whatever a race with that
// job already made it. It records nothing from the request (design D2): the
// only argument that matters is which batch, and the job re-derives every
// fact about the upload from the object itself.
func (s *Service) ConfirmImportUpload(ctx context.Context, userID, requestedOrgID, batchID uuid.UUID) (Batch, error) {
	org, _, err := s.resolveOrg(ctx, userID, requestedOrgID)
	if err != nil {
		return Batch{}, err
	}
	id := pgtype.UUID{Bytes: batchID, Valid: true}

	var row gendb.GetImportBatchRow
	err = s.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)
		var err error
		if row, err = q.GetImportBatch(ctx, id); err != nil {
			return err
		}
		return s.jobs.InsertTx(ctx, tx, MeasureUploadArgs{
			TenantJobArgs: db.TenantJobArgs{OrgID: org.UUID()},
			BatchID:       batchID,
		})
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Batch{}, ErrBatchNotFound
	case err != nil:
		return Batch{}, fmt.Errorf("ingest: confirm_import_upload: %w", err)
	}
	return batchFromGetRow(row), nil
}

// GetImportBatch reads one batch. No org_id predicate is needed here any
// more than in the SQL beneath it: the row simply is not visible under
// another organisation's tenant context (task 6.1).
func (s *Service) GetImportBatch(ctx context.Context, userID, requestedOrgID, batchID uuid.UUID) (Batch, error) {
	org, _, err := s.resolveOrg(ctx, userID, requestedOrgID)
	if err != nil {
		return Batch{}, err
	}

	var row gendb.GetImportBatchRow
	err = s.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, err = gendb.New(tx).GetImportBatch(ctx, pgtype.UUID{Bytes: batchID, Valid: true})
		return err
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Batch{}, ErrBatchNotFound
	case err != nil:
		return Batch{}, fmt.Errorf("ingest: get_import_batch: %w", err)
	}
	return batchFromGetRow(row), nil
}

// ListImportBatches lists one entity's batches, most recent first.
func (s *Service) ListImportBatches(ctx context.Context, userID, requestedOrgID, entityID uuid.UUID) ([]Batch, error) {
	org, _, err := s.resolveOrg(ctx, userID, requestedOrgID)
	if err != nil {
		return nil, err
	}

	var rows []gendb.ListImportBatchesRow
	err = s.database.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = gendb.New(tx).ListImportBatches(ctx, pgtype.UUID{Bytes: entityID, Valid: true})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("ingest: list_import_batches: %w", err)
	}

	batches := make([]Batch, len(rows))
	for i, row := range rows {
		batches[i] = batchFromListRow(row)
	}
	return batches, nil
}

// batchFromInsertRow, batchFromGetRow and batchFromListRow convert sqlc's
// three separately generated row types, which are structurally identical but
// not the same Go type. Three short functions rather than one generic one:
// sqlc does not export a shared row interface, and inventing one here to
// collapse three assignments each used once is the wrong trade.

func batchFromInsertRow(r gendb.InsertImportBatchRow) Batch {
	return Batch{
		ID:          uuid.UUID(r.ID.Bytes),
		EntityID:    uuid.UUID(r.EntityID.Bytes),
		SourceKind:  r.SourceKind,
		Status:      Status(r.Status),
		FileName:    r.FileName,
		ByteLength:  r.ByteLength.Int64,
		FailureCode: r.FailureCode.String,
		CreatedAt:   r.CreatedAt.Time,
	}
}

func batchFromGetRow(r gendb.GetImportBatchRow) Batch {
	return Batch{
		ID:          uuid.UUID(r.ID.Bytes),
		EntityID:    uuid.UUID(r.EntityID.Bytes),
		SourceKind:  r.SourceKind,
		Status:      Status(r.Status),
		FileName:    r.FileName,
		ByteLength:  r.ByteLength.Int64,
		FailureCode: r.FailureCode.String,
		CreatedAt:   r.CreatedAt.Time,
	}
}

func batchFromListRow(r gendb.ListImportBatchesRow) Batch {
	return Batch{
		ID:          uuid.UUID(r.ID.Bytes),
		EntityID:    uuid.UUID(r.EntityID.Bytes),
		SourceKind:  r.SourceKind,
		Status:      Status(r.Status),
		FileName:    r.FileName,
		ByteLength:  r.ByteLength.Int64,
		FailureCode: r.FailureCode.String,
		CreatedAt:   r.CreatedAt.Time,
	}
}
