package server

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	vektv1 "github.com/MyauDev/vekst/core/gen/vekst/v1"
	typev1 "github.com/MyauDev/vekst/core/gen/vekstype/v1"
	"github.com/google/uuid"

	"github.com/MyauDev/vekst/core/internal/money"
	"github.com/MyauDev/vekst/core/internal/report"
)

// reportHandler implements vekst.v1.ReportService.
//
// It translates and nothing else: a request in, report.Service out, a Connect
// code back. The tenancy binding lives in the service for the same reason the
// review queue's does -- change 0.2's guard test forbids this package from
// importing core/internal/db at all, because the health handlers live here and
// liveness must never be able to reach a dependency.
type reportHandler struct {
	svc *report.Service
}

func (h *reportHandler) GetManagementPNL(
	ctx context.Context,
	req *connect.Request[vektv1.GetManagementPNLRequest],
) (*connect.Response[vektv1.GetManagementPNLResponse], error) {
	user, orgID, err := caller(ctx, req.Msg.GetOrganizationId())
	if err != nil {
		return nil, err
	}
	entity, err := parseUUID(req.Msg.GetEntityId(), "entity_id")
	if err != nil {
		return nil, err
	}
	basis, err := fromProtoBasis(req.Msg.GetBasis())
	if err != nil {
		return nil, err
	}
	granularity, err := fromProtoGranularity(req.Msg.GetGranularity())
	if err != nil {
		return nil, err
	}

	r, err := h.svc.ManagementPNL(ctx, user, orgID, report.Request{
		EntityID:    entity,
		From:        req.Msg.GetFrom(),
		To:          req.Msg.GetTo(),
		Granularity: granularity,
		Basis:       basis,
	})
	if err != nil {
		return nil, reportErr(err)
	}

	out := &vektv1.GetManagementPNLResponse{
		Basis:        req.Msg.GetBasis(),
		Granularity:  req.Msg.GetGranularity(),
		From:         r.Spec.From,
		To:           r.Spec.To,
		BaseCurrency: r.Spec.BaseCurrency,
		Periods:      r.Spec.Periods,
		Lines:        make([]*vektv1.ReportLine, 0, len(r.Lines)),
		Buckets:      make([]*vektv1.ReportBucketLine, 0, len(report.BucketOrder)),
		Versions: &vektv1.ReportVersions{
			Taxonomy:  r.Spec.TaxonomyVersions,
			Ruleset:   r.Spec.RulesetVersions,
			Engine:    r.Spec.EngineVersions,
			Normalize: r.Spec.NormalizeVersions,
		},
	}

	for _, l := range r.Lines {
		line := &vektv1.ReportLine{
			Code:     l.Code,
			Label:    l.Label,
			Formula:  l.Formula,
			Computed: l.Computed,
			ByPeriod: make([]*vektv1.ReportFigure, 0, len(l.ByPeriod)),
			Total:    toFigure(l.Total),
		}
		for _, f := range l.ByPeriod {
			line.ByPeriod = append(line.ByPeriod, toFigure(f))
		}
		out.Lines = append(out.Lines, line)
	}

	// Iterated over the package's own order rather than over the map, so the
	// buckets arrive in the same order every time. Ranging a Go map would
	// reorder them on every call, and a client rendering them in wire order
	// would shuffle the footer of the report between two identical requests.
	for _, kind := range report.BucketOrder {
		bucket := &vektv1.ReportBucketLine{
			Kind:     toProtoBucket(kind),
			ByPeriod: make([]*typev1.Money, 0, len(r.Spec.Periods)),
			Total:    toReportMoney(r.Totals[kind]),
		}
		for _, m := range r.Buckets[kind] {
			bucket.ByPeriod = append(bucket.ByPeriod, toReportMoney(m))
		}
		out.Buckets = append(out.Buckets, bucket)
	}

	for _, strip := range r.Reconciliation {
		out.Reconciliation = append(out.Reconciliation, &vektv1.ReconciliationLine{
			Period:           strip.Period,
			Opening:          toReportMoney(strip.Opening),
			In:               toReportMoney(strip.In),
			Out:              toReportMoney(strip.Out),
			Transfers:        toReportMoney(strip.Transfers),
			TransferRowCount: int32(strip.TransferRowCount),
			Closing:          toReportMoney(strip.Closing),
			Balances:         strip.Balances,
			Derived:          strip.Derived,
		})
	}

	return connect.NewResponse(out), nil
}

func (h *reportHandler) ListLineTransactions(
	ctx context.Context,
	req *connect.Request[vektv1.ListLineTransactionsRequest],
) (*connect.Response[vektv1.ListLineTransactionsResponse], error) {
	user, orgID, err := caller(ctx, req.Msg.GetOrganizationId())
	if err != nil {
		return nil, err
	}
	entity, err := parseUUID(req.Msg.GetEntityId(), "entity_id")
	if err != nil {
		return nil, err
	}
	basis, err := fromProtoBasis(req.Msg.GetBasis())
	if err != nil {
		return nil, err
	}
	granularity, err := fromProtoGranularity(req.Msg.GetGranularity())
	if err != nil {
		return nil, err
	}
	line, err := report.ParseLineRef(req.Msg.GetLine())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New(report.CodeUnknownLine))
	}
	cursor, err := report.ParseCursor(req.Msg.GetCursor())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New(report.CodeBadCursor))
	}

	d, err := h.svc.LineTransactions(ctx, user, orgID, report.Request{
		EntityID:    entity,
		From:        req.Msg.GetFrom(),
		To:          req.Msg.GetTo(),
		Granularity: granularity,
		Basis:       basis,
	}, req.Msg.GetPeriod(), line, cursor, req.Msg.GetLimit())
	if err != nil {
		return nil, reportErr(err)
	}

	out := &vektv1.ListLineTransactionsResponse{
		Kind:       toProtoAnswer(d.Kind),
		RowCount:   int32(d.RowCount),
		Total:      toReportMoney(d.Total),
		NextCursor: d.NextCursor,
	}
	for _, o := range d.Operands {
		out.Operands = append(out.Operands, &vektv1.ReportOperand{
			Code: o.Code, Label: o.Label, Subtracted: o.Subtracted,
		})
	}
	for _, r := range d.Rows {
		row := &vektv1.DrillTransaction{
			Id:              r.ID.String(),
			BookedOn:        r.BookedOn,
			BatchId:         r.BatchID.String(),
			LineNo:          r.LineNo,
			PostingNo:       r.PostingNo,
			DocumentRef:     r.DocumentRef,
			Amount:          toReportMoney(r.Amount),
			BaseAmount:      toMoney(r.BaseAmount),
			CounterpartyRaw: r.CounterpartyRaw,
			Description:     r.Description,
			RegulatedCode:   r.RegulatedCode,
			SourceKind:      r.SourceKind,
			CategoryCode:    r.CategoryCode,
			CategoryName:    r.CategoryName,
			EngineLayer:     r.EngineLayer,
			Evidence:        r.Evidence,
		}
		if r.HasConfidence {
			c := r.Confidence
			row.Confidence = &c
		}
		if r.DecidedBy != uuid.Nil {
			row.DecidedBy = r.DecidedBy.String()
		}
		out.Transactions = append(out.Transactions, row)
	}
	return connect.NewResponse(out), nil
}

func toProtoAnswer(a report.Answer) vektv1.ReportAnswerKind {
	if a == report.AnswerOperands {
		return vektv1.ReportAnswerKind_REPORT_ANSWER_KIND_OPERANDS
	}
	return vektv1.ReportAnswerKind_REPORT_ANSWER_KIND_TRANSACTIONS
}

// toFigure carries a zero amount as a zero amount and an absent percentage as
// absent. A report that has no revenue this period has no percentage of it --
// neither zero nor infinity, because a client would render both as a fact.
func toFigure(f report.Figure) *vektv1.ReportFigure {
	out := &vektv1.ReportFigure{
		Amount: &typev1.Money{
			CurrencyCode: f.Amount.CurrencyCode,
			MinorUnits:   f.Amount.MinorUnits,
		},
	}
	if f.HasPercentOfRevenue {
		pct := f.PercentOfRevenue
		out.PercentOfRevenue = &pct
	}
	return out
}

// A figure of zero in the report's own currency is a fact -- "this section had
// no movement this month" -- and not the absence toMoney elides, so the bucket
// amounts are built here rather than through it.
func toReportMoney(m money.Money) *typev1.Money {
	return &typev1.Money{CurrencyCode: m.CurrencyCode, MinorUnits: m.MinorUnits}
}

func fromProtoBasis(b vektv1.ReportBasis) (report.Basis, error) {
	switch b {
	case vektv1.ReportBasis_REPORT_BASIS_LEDGER:
		return report.BasisLedger, nil
	case vektv1.ReportBasis_REPORT_BASIS_BANK:
		return report.BasisBank, nil
	default:
		// Proto3 cannot tell an unset enum from its zero value, so the zero
		// value is refused rather than defaulted. Defaulting the basis would
		// pick which half of the business the owner is looking at, silently.
		return "", connect.NewError(connect.CodeInvalidArgument,
			errors.New(report.CodeUnknownBasis))
	}
}

func fromProtoGranularity(g vektv1.ReportGranularity) (report.Granularity, error) {
	switch g {
	case vektv1.ReportGranularity_REPORT_GRANULARITY_MONTH:
		return report.Monthly, nil
	case vektv1.ReportGranularity_REPORT_GRANULARITY_QUARTER:
		return report.Quarterly, nil
	case vektv1.ReportGranularity_REPORT_GRANULARITY_YEAR:
		return report.Yearly, nil
	default:
		return "", connect.NewError(connect.CodeInvalidArgument,
			errors.New(report.CodeUnknownGranularity))
	}
}

func toProtoBucket(b report.Bucket) vektv1.ReportBucketKind {
	switch b {
	case report.BucketUnclassified:
		return vektv1.ReportBucketKind_REPORT_BUCKET_KIND_UNCLASSIFIED
	case report.BucketNonPNL:
		return vektv1.ReportBucketKind_REPORT_BUCKET_KIND_NON_PNL
	case report.BucketUnallocated:
		return vektv1.ReportBucketKind_REPORT_BUCKET_KIND_UNALLOCATED
	case report.BucketOtherBasis:
		return vektv1.ReportBucketKind_REPORT_BUCKET_KIND_OTHER_BASIS
	default:
		return vektv1.ReportBucketKind_REPORT_BUCKET_KIND_UNSPECIFIED
	}
}

// reportErr maps the service's coded failures onto Connect codes. The code is
// the message: translation is the client's.
func reportErr(err error) error {
	var coded *report.Err
	if errors.As(err, &coded) {
		switch coded.Code {
		case report.CodeForbidden:
			return connect.NewError(connect.CodePermissionDenied, errors.New(coded.Code))
		case report.CodeNoBaseCurrency:
			return connect.NewError(connect.CodeFailedPrecondition, errors.New(coded.Code))
		default:
			return connect.NewError(connect.CodeInvalidArgument, errors.New(coded.Code))
		}
	}
	return connect.NewError(connect.CodeInternal, err)
}
