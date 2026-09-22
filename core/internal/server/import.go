package server

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	vektv1 "github.com/MyauDev/vekst/core/gen/vekst/v1"
	"github.com/MyauDev/vekst/core/internal/dedup"
	"github.com/MyauDev/vekst/core/internal/identity"
	"github.com/MyauDev/vekst/core/internal/ingest"
)

// importHandler implements vekst.v1.ImportService.
//
// It never imports core/internal/db (TestHealthPathImportsNoDatabase's
// forbidden-import list applies to this whole package, not only the health
// path). Every method here resolves the authenticated caller from
// identity.FromContext and passes the raw ids through to *ingest.Service,
// which is the package that actually holds a *db.DB and resolves the
// organisation against the caller's memberships.
type importHandler struct {
	svc *ingest.Service
}

func (h *importHandler) CreateImportBatch(
	ctx context.Context,
	req *connect.Request[vektv1.CreateImportBatchRequest],
) (*connect.Response[vektv1.CreateImportBatchResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}
	entityID, err := uuid.Parse(req.Msg.GetEntityId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}

	var profileID uuid.UUID
	if req.Msg.GetImportProfileId() != "" {
		profileID, err = uuid.Parse(req.Msg.GetImportProfileId())
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
		}
	}

	batch, upload, err := h.svc.CreateImportBatch(ctx, user.ID, orgID, ingest.CreateBatchInput{
		EntityID:        entityID,
		SourceKind:      sourceKindFromProto(req.Msg.GetSourceKind()),
		FileName:        req.Msg.GetFileName(),
		DeclaredBytes:   req.Msg.GetDeclaredBytes(),
		DeclaredType:    req.Msg.GetDeclaredType(),
		ImportProfileID: profileID,
	})
	if err != nil {
		return nil, importError(err)
	}

	return connect.NewResponse(&vektv1.CreateImportBatchResponse{
		Batch:         batchToProto(batch),
		UploadUrl:     upload.URL,
		UploadHeaders: upload.Headers,
		ExpiresAt:     timestamppb.New(upload.ExpiresAt),
	}), nil
}

func (h *importHandler) ConfirmImportUpload(
	ctx context.Context,
	req *connect.Request[vektv1.ConfirmImportUploadRequest],
) (*connect.Response[vektv1.ConfirmImportUploadResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}
	batchID, err := uuid.Parse(req.Msg.GetBatchId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}

	batch, err := h.svc.ConfirmImportUpload(ctx, user.ID, orgID, batchID)
	if err != nil {
		return nil, importError(err)
	}
	return connect.NewResponse(&vektv1.ConfirmImportUploadResponse{Batch: batchToProto(batch)}), nil
}

func (h *importHandler) GetImportBatch(
	ctx context.Context,
	req *connect.Request[vektv1.GetImportBatchRequest],
) (*connect.Response[vektv1.GetImportBatchResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}
	batchID, err := uuid.Parse(req.Msg.GetBatchId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}

	batch, err := h.svc.GetImportBatch(ctx, user.ID, orgID, batchID)
	if err != nil {
		return nil, importError(err)
	}
	return connect.NewResponse(&vektv1.GetImportBatchResponse{Batch: batchToProto(batch)}), nil
}

func (h *importHandler) ListBatchTransactions(
	ctx context.Context,
	req *connect.Request[vektv1.ListBatchTransactionsRequest],
) (*connect.Response[vektv1.ListBatchTransactionsResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}
	batchID, err := uuid.Parse(req.Msg.GetBatchId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}

	rows, err := h.svc.ListBatchTransactions(ctx, user.ID, orgID, batchID)
	if err != nil {
		return nil, importError(err)
	}

	out := &vektv1.ListBatchTransactionsResponse{Rows: make([]*vektv1.ImportedRow, len(rows))}
	for i, r := range rows {
		out.Rows[i] = importedRowToProto(r)
	}
	return connect.NewResponse(out), nil
}

func (h *importHandler) ListImportBatches(
	ctx context.Context,
	req *connect.Request[vektv1.ListImportBatchesRequest],
) (*connect.Response[vektv1.ListImportBatchesResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}
	entityID, err := uuid.Parse(req.Msg.GetEntityId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}

	batches, err := h.svc.ListImportBatches(ctx, user.ID, orgID, entityID)
	if err != nil {
		return nil, importError(err)
	}

	resp := &vektv1.ListImportBatchesResponse{Batches: make([]*vektv1.ImportBatch, len(batches))}
	for i, b := range batches {
		resp.Batches[i] = batchToProto(b)
	}
	return connect.NewResponse(resp), nil
}

func (h *importHandler) GetValidationReport(
	ctx context.Context,
	req *connect.Request[vektv1.GetValidationReportRequest],
) (*connect.Response[vektv1.GetValidationReportResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}
	batchID, err := uuid.Parse(req.Msg.GetBatchId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}

	report, err := h.svc.GetValidationReport(ctx, user.ID, orgID, batchID)
	if err != nil {
		return nil, importError(err)
	}
	return connect.NewResponse(&vektv1.GetValidationReportResponse{Report: reportToProto(report)}), nil
}

func (h *importHandler) OverrideValidation(
	ctx context.Context,
	req *connect.Request[vektv1.OverrideValidationRequest],
) (*connect.Response[vektv1.OverrideValidationResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}
	batchID, err := uuid.Parse(req.Msg.GetBatchId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}

	report, err := h.svc.OverrideValidation(ctx, user.ID, orgID, batchID, req.Msg.GetReason())
	if err != nil {
		return nil, importError(err)
	}
	return connect.NewResponse(&vektv1.OverrideValidationResponse{Report: reportToProto(report)}), nil
}

func (h *importHandler) CreateImportProfile(
	ctx context.Context,
	req *connect.Request[vektv1.CreateImportProfileRequest],
) (*connect.Response[vektv1.CreateImportProfileResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}

	profile, err := h.svc.CreateImportProfile(ctx, user.ID, orgID, profileInputFromProto(req.Msg))
	if err != nil {
		return nil, importError(err)
	}
	return connect.NewResponse(&vektv1.CreateImportProfileResponse{Profile: profileToProto(profile)}), nil
}

func (h *importHandler) GetImportProfile(
	ctx context.Context,
	req *connect.Request[vektv1.GetImportProfileRequest],
) (*connect.Response[vektv1.GetImportProfileResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}
	profileID, err := uuid.Parse(req.Msg.GetProfileId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}

	profile, err := h.svc.GetImportProfile(ctx, user.ID, orgID, profileID)
	if err != nil {
		return nil, importError(err)
	}
	return connect.NewResponse(&vektv1.GetImportProfileResponse{Profile: profileToProto(profile)}), nil
}

func (h *importHandler) ListImportProfiles(
	ctx context.Context,
	req *connect.Request[vektv1.ListImportProfilesRequest],
) (*connect.Response[vektv1.ListImportProfilesResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}

	profiles, err := h.svc.ListImportProfiles(ctx, user.ID, orgID)
	if err != nil {
		return nil, importError(err)
	}

	resp := &vektv1.ListImportProfilesResponse{Profiles: make([]*vektv1.ImportProfile, len(profiles))}
	for i, p := range profiles {
		resp.Profiles[i] = profileToProto(p)
	}
	return connect.NewResponse(resp), nil
}

func (h *importHandler) UpdateImportProfile(
	ctx context.Context,
	req *connect.Request[vektv1.UpdateImportProfileRequest],
) (*connect.Response[vektv1.UpdateImportProfileResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}
	profileID, err := uuid.Parse(req.Msg.GetProfileId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}

	in := ingest.ProfileInput{
		Name:       req.Msg.GetName(),
		ColumnMap:  ingest.ColumnMap(req.Msg.GetColumnMap()),
		Charset:    req.Msg.GetCharset(),
		Delimiter:  req.Msg.GetDelimiter(),
		DecimalSep: req.Msg.GetDecimalSep(),
		DateFormat: req.Msg.GetDateFmt(),
	}
	profile, err := h.svc.UpdateImportProfile(ctx, user.ID, orgID, profileID, in)
	if err != nil {
		return nil, importError(err)
	}
	return connect.NewResponse(&vektv1.UpdateImportProfileResponse{Profile: profileToProto(profile)}), nil
}

func (h *importHandler) DeleteImportProfile(
	ctx context.Context,
	req *connect.Request[vektv1.DeleteImportProfileRequest],
) (*connect.Response[vektv1.DeleteImportProfileResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}
	profileID, err := uuid.Parse(req.Msg.GetProfileId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}

	if err := h.svc.DeleteImportProfile(ctx, user.ID, orgID, profileID); err != nil {
		return nil, importError(err)
	}
	return connect.NewResponse(&vektv1.DeleteImportProfileResponse{}), nil
}

// importError translates the sentinels *ingest.Service returns into a
// Connect code plus a code string, never a sentence (CLAUDE.md: the backend
// returns error codes, translation is the client's).
func (h *importHandler) GetDedupSummary(
	ctx context.Context,
	req *connect.Request[vektv1.GetDedupSummaryRequest],
) (*connect.Response[vektv1.GetDedupSummaryResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}
	batchID, err := uuid.Parse(req.Msg.GetBatchId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}

	summary, err := h.svc.GetDedupSummary(ctx, user.ID, orgID, batchID)
	if err != nil {
		return nil, importError(err)
	}
	return connect.NewResponse(&vektv1.GetDedupSummaryResponse{Summary: dedupSummaryToProto(summary)}), nil
}

func (h *importHandler) ListSkippedRows(
	ctx context.Context,
	req *connect.Request[vektv1.ListSkippedRowsRequest],
) (*connect.Response[vektv1.ListSkippedRowsResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}
	batchID, err := uuid.Parse(req.Msg.GetBatchId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}

	skips, err := h.svc.ListSkippedRows(ctx, user.ID, orgID, batchID)
	if err != nil {
		return nil, importError(err)
	}

	resp := &vektv1.ListSkippedRowsResponse{Rows: make([]*vektv1.SkippedRow, len(skips))}
	for i, s := range skips {
		resp.Rows[i] = skippedRowToProto(s)
	}
	return connect.NewResponse(resp), nil
}

func (h *importHandler) ListInternalTransfers(
	ctx context.Context,
	req *connect.Request[vektv1.ListInternalTransfersRequest],
) (*connect.Response[vektv1.ListInternalTransfersResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}
	entityID, err := uuid.Parse(req.Msg.GetEntityId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}

	transfers, err := h.svc.ListInternalTransfers(ctx, user.ID, orgID, entityID)
	if err != nil {
		return nil, importError(err)
	}

	resp := &vektv1.ListInternalTransfersResponse{Transfers: make([]*vektv1.InternalTransfer, len(transfers))}
	for i, t := range transfers {
		resp.Transfers[i] = internalTransferToProto(t)
	}
	return connect.NewResponse(resp), nil
}

func (h *importHandler) DismissInternalTransfer(
	ctx context.Context,
	req *connect.Request[vektv1.DismissInternalTransferRequest],
) (*connect.Response[vektv1.DismissInternalTransferResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}
	transferID, err := uuid.Parse(req.Msg.GetTransferId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_argument"))
	}

	transfer, err := h.svc.DismissInternalTransfer(ctx, user.ID, orgID, transferID)
	if err != nil {
		return nil, importError(err)
	}
	return connect.NewResponse(&vektv1.DismissInternalTransferResponse{Transfer: internalTransferToProto(transfer)}), nil
}

func importError(err error) error {
	switch {
	case errors.Is(err, ingest.ErrNotAMember):
		return connect.NewError(connect.CodePermissionDenied, errors.New("not_a_member"))
	case errors.Is(err, ingest.ErrBatchNotFound):
		return connect.NewError(connect.CodeNotFound, errors.New("batch_not_found"))
	case errors.Is(err, ingest.ErrObjectStoreNotConfigured):
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("object_store_not_configured"))
	case errors.Is(err, ingest.ErrValidationNotFound):
		return connect.NewError(connect.CodeNotFound, errors.New("validation_not_found"))
	case errors.Is(err, ingest.ErrOverrideReasonTooShort):
		return connect.NewError(connect.CodeInvalidArgument, errors.New("override_reason_too_short"))
	case errors.Is(err, ingest.ErrOverrideRequiresApproverRole):
		return connect.NewError(connect.CodePermissionDenied, errors.New("override_requires_approver_role"))
	case errors.Is(err, ingest.ErrAlreadyOverridden):
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("already_overridden"))
	case errors.Is(err, ingest.ErrOverrideOnlyOverWarnings):
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("override_only_over_warnings"))
	case errors.Is(err, ingest.ErrProfileNotFound):
		return connect.NewError(connect.CodeNotFound, errors.New("profile_not_found"))
	case errors.Is(err, ingest.ErrProfileNameTaken):
		return connect.NewError(connect.CodeAlreadyExists, errors.New("profile_name_taken"))
	case errors.Is(err, ingest.ErrProfileSourceKindMismatch):
		return connect.NewError(connect.CodeInvalidArgument, errors.New("profile_source_kind_mismatch"))
	case errors.Is(err, ingest.ErrProfileInUse):
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("profile_in_use"))
	case errors.Is(err, dedup.ErrTransferNotFound):
		return connect.NewError(connect.CodeNotFound, errors.New("transfer_not_found"))
	}
	var invalid *ingest.ErrInvalidArgument
	if errors.As(err, &invalid) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New(invalid.Code))
	}
	var badField *ingest.ErrInvalidCanonicalField
	if errors.As(err, &badField) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("invalid_canonical_field"))
	}
	return connect.NewError(connect.CodeInternal, errors.New("internal"))
}

func validationOutcomeToProto(o ingest.Outcome) vektv1.ValidationOutcome {
	switch o {
	case ingest.OutcomeValid:
		return vektv1.ValidationOutcome_VALIDATION_OUTCOME_VALID
	case ingest.OutcomeValidWithWarnings:
		return vektv1.ValidationOutcome_VALIDATION_OUTCOME_VALID_WITH_WARNINGS
	case ingest.OutcomeRejected:
		return vektv1.ValidationOutcome_VALIDATION_OUTCOME_REJECTED
	default:
		return vektv1.ValidationOutcome_VALIDATION_OUTCOME_UNSPECIFIED
	}
}

func reportToProto(r ingest.ValidationReport) *vektv1.ValidationReport {
	out := &vektv1.ValidationReport{
		BatchId:      r.BatchID.String(),
		Outcome:      validationOutcomeToProto(r.Outcome),
		RowCount:     int32(r.RowCount),
		ErrorCount:   int32(r.ErrorCount),
		WarningCount: int32(r.WarningCount),
	}
	if r.BalanceCheckPassed != nil {
		out.BalanceCheckPassed = wrapperspb.Bool(*r.BalanceCheckPassed)
	}
	for _, e := range r.Errors {
		out.Errors = append(out.Errors, &vektv1.ValidationError{
			Line: int32(e.Line), Field: e.Field, Code: e.Code, Raw: e.Raw,
		})
	}
	for _, w := range r.Warnings {
		pw := &vektv1.ValidationWarning{Code: w.Code}
		if w.Balance != nil {
			pw.BalanceMismatch = &vektv1.BalanceMismatchDetail{
				Opening: w.Balance.Opening, Movements: w.Balance.Movements,
				Closing: w.Balance.Closing, Difference: w.Balance.Difference,
				Currency: w.Balance.Currency,
			}
		}
		out.Warnings = append(out.Warnings, pw)
	}
	if r.OverriddenBy != uuid.Nil {
		out.OverriddenByUserId = r.OverriddenBy.String()
		out.OverrideReason = r.OverrideReason
		if r.OverriddenAt != nil {
			out.OverriddenAt = timestamppb.New(*r.OverriddenAt)
		}
	}
	return out
}

func sourceKindFromProto(k vektv1.SourceKind) string {
	if k == vektv1.SourceKind_SOURCE_KIND_LEDGER {
		return ingest.SourceKindLedger
	}
	return ingest.SourceKindBank
}

func sourceKindToProto(k string) vektv1.SourceKind {
	if k == ingest.SourceKindLedger {
		return vektv1.SourceKind_SOURCE_KIND_LEDGER
	}
	return vektv1.SourceKind_SOURCE_KIND_BANK
}

// importStatusToProto maps every ingest.Status to its proto counterpart.
// Deliberately exhaustive over all eleven, even though this change only ever
// produces three of them -- the same reasoning as the transition table
// itself (design D3): later changes drive the rest, and this switch is where
// a value the database now admits but this function does not know about
// would otherwise fall through to UNSPECIFIED silently.
func importStatusToProto(s ingest.Status) vektv1.ImportStatus {
	switch s {
	case ingest.StatusAwaitingUpload:
		return vektv1.ImportStatus_IMPORT_STATUS_AWAITING_UPLOAD
	case ingest.StatusAbandoned:
		return vektv1.ImportStatus_IMPORT_STATUS_ABANDONED
	case ingest.StatusUploaded:
		return vektv1.ImportStatus_IMPORT_STATUS_UPLOADED
	case ingest.StatusParsing:
		return vektv1.ImportStatus_IMPORT_STATUS_PARSING
	case ingest.StatusParsed:
		return vektv1.ImportStatus_IMPORT_STATUS_PARSED
	case ingest.StatusValidating:
		return vektv1.ImportStatus_IMPORT_STATUS_VALIDATING
	case ingest.StatusValidated:
		return vektv1.ImportStatus_IMPORT_STATUS_VALIDATED
	case ingest.StatusRejected:
		return vektv1.ImportStatus_IMPORT_STATUS_REJECTED
	case ingest.StatusPersisting:
		return vektv1.ImportStatus_IMPORT_STATUS_PERSISTING
	case ingest.StatusImported:
		return vektv1.ImportStatus_IMPORT_STATUS_IMPORTED
	case ingest.StatusFailed:
		return vektv1.ImportStatus_IMPORT_STATUS_FAILED
	default:
		return vektv1.ImportStatus_IMPORT_STATUS_UNSPECIFIED
	}
}

func profileInputFromProto(req *vektv1.CreateImportProfileRequest) ingest.ProfileInput {
	return ingest.ProfileInput{
		Name:       req.GetName(),
		SourceKind: sourceKindFromProto(req.GetSourceKind()),
		ColumnMap:  ingest.ColumnMap(req.GetColumnMap()),
		Charset:    req.GetCharset(),
		Delimiter:  req.GetDelimiter(),
		DecimalSep: req.GetDecimalSep(),
		DateFormat: req.GetDateFmt(),
	}
}

func profileToProto(p ingest.Profile) *vektv1.ImportProfile {
	return &vektv1.ImportProfile{
		Id:         p.ID.String(),
		Name:       p.Name,
		SourceKind: sourceKindToProto(p.SourceKind),
		ColumnMap:  p.ColumnMap,
		Charset:    p.Charset,
		Delimiter:  p.Delimiter,
		DecimalSep: p.DecimalSep,
		DateFmt:    p.DateFormat,
		CreatedAt:  timestamppb.New(p.CreatedAt),
		UpdatedAt:  timestamppb.New(p.UpdatedAt),
	}
}

func batchToProto(b ingest.Batch) *vektv1.ImportBatch {
	return &vektv1.ImportBatch{
		Id:          b.ID.String(),
		EntityId:    b.EntityID.String(),
		SourceKind:  sourceKindToProto(b.SourceKind),
		Status:      importStatusToProto(b.Status),
		FileName:    b.FileName,
		ByteLength:  b.ByteLength,
		FailureCode: b.FailureCode,
		CreatedAt:   timestamppb.New(b.CreatedAt),

		// Absent until something has classified this batch. A zero-valued
		// struct here would say "a run that did nothing", which is what a
		// failed run also looks like from a distance.
		ClassificationRun: classificationRunToProto(b),
	}
}

func classificationRunToProto(b ingest.Batch) *vektv1.ClassificationRun {
	if !b.HasClassificationRun {
		return nil
	}
	out := &vektv1.ClassificationRun{
		Status:          classificationRunStatusToProto(b.Classification.Status),
		FailureCode:     b.Classification.FailureCode,
		ChunkCount:      b.Classification.ChunkCount,
		ClassifiedCount: b.Classification.ClassifiedCount,
		ReviewCount:     b.Classification.ReviewCount,
		StartedAt:       timestamppb.New(b.Classification.StartedAt),
	}
	if !b.Classification.FinishedAt.IsZero() {
		out.FinishedAt = timestamppb.New(b.Classification.FinishedAt)
	}
	return out
}

func classificationRunStatusToProto(s string) vektv1.ClassificationRunStatus {
	switch s {
	case "running":
		return vektv1.ClassificationRunStatus_CLASSIFICATION_RUN_STATUS_RUNNING
	case "classified":
		return vektv1.ClassificationRunStatus_CLASSIFICATION_RUN_STATUS_CLASSIFIED
	case "failed":
		return vektv1.ClassificationRunStatus_CLASSIFICATION_RUN_STATUS_FAILED
	default:
		// The column's CHECK admits three values, so this is unreachable
		// unless the schema and this switch have drifted -- and an unspecified
		// status a client cannot render says so, where guessing "running"
		// would show a finished import as still working forever.
		return vektv1.ClassificationRunStatus_CLASSIFICATION_RUN_STATUS_UNSPECIFIED
	}
}

func dedupSummaryToProto(s ingest.Summary) *vektv1.DedupSummary {
	return &vektv1.DedupSummary{
		ImportedRows:      s.ImportedRows,
		SkippedInBatch:    s.SkippedInBatch,
		SkippedCrossBatch: s.SkippedCrossBatch,
		InternalTransfers: s.InternalTransfers,
	}
}

func importedRowToProto(r ingest.BatchRow) *vektv1.ImportedRow {
	out := &vektv1.ImportedRow{
		Id:              r.ID.String(),
		BookedOn:        r.BookedOn,
		LineNo:          r.LineNo,
		PostingNo:       r.PostingNo,
		DocumentRef:     r.DocumentRef,
		Amount:          toMoney(r.Amount),
		BaseAmount:      toMoney(r.BaseAmount),
		CounterpartyRaw: r.CounterpartyRaw,
		Description:     r.Description,
		RegulatedCode:   r.RegulatedCode,
		CategoryCode:    r.CategoryCode,
		CategoryName:    r.CategoryName,
		EngineLayer:     r.EngineLayer,
		Evidence:        r.Evidence,
	}
	if r.HasConfidence {
		c := r.Confidence
		out.Confidence = &c
	}
	return out
}

func skippedRowToProto(s dedup.Skip) *vektv1.SkippedRow {
	out := &vektv1.SkippedRow{
		LineNo:    int32(s.LineNo),
		PostingNo: s.PostingNo,
		Level:     s.Level,
		DedupHash: s.DedupHash,
		CreatedAt: timestamppb.New(s.CreatedAt),
	}
	if s.MatchedTransactionID != uuid.Nil {
		out.MatchedTransactionId = s.MatchedTransactionID.String()
	}
	if s.MatchedBatchID != uuid.Nil {
		out.MatchedBatchId = s.MatchedBatchID.String()
	}
	return out
}

func internalTransferToProto(t dedup.Transfer) *vektv1.InternalTransfer {
	out := &vektv1.InternalTransfer{
		Id:               t.ID.String(),
		OutTransactionId: t.OutTxnID.String(),
		InTransactionId:  t.InTxnID.String(),
		DetectedAt:       timestamppb.New(t.DetectedAt),
	}
	if t.DismissedBy != uuid.Nil {
		out.DismissedByUserId = t.DismissedBy.String()
	}
	if !t.DismissedAt.IsZero() {
		out.DismissedAt = timestamppb.New(t.DismissedAt)
	}
	return out
}
