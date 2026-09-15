package report

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/money"
)

// Error codes. The backend returns codes, never sentences -- translation is
// the client's (CLAUDE.md, Conventions).
const (
	CodeForbidden          = "report_forbidden"
	CodeBadRange           = "report_bad_range"
	CodeUnknownBasis       = "report_unknown_basis"
	CodeUnknownGranularity = "report_unknown_granularity"
	CodeNoBaseCurrency     = "report_no_base_currency"
)

// Err is a failure with a code a client can translate.
type Err struct {
	Code string
	err  error
}

func (e *Err) Error() string { return e.Code }
func (e *Err) Unwrap() error { return e.err }

func codeErr(code string, cause error) error { return &Err{Code: code, err: cause} }

// Request is what a caller asked for. Months at both ends, because a period is
// a closed interval on booked_on -- a date, which carries no time zone
// deliberately, since a booking date has none.
type Request struct {
	EntityID uuid.UUID

	// From and To are months, YYYY-MM, and the range includes both.
	From string
	To   string

	Granularity Granularity
	Basis       Basis
}

// Service produces the report. It holds the database and nothing else.
type Service struct {
	db *db.DB
}

// New builds the service.
func New(database *db.DB) *Service {
	return &Service{db: database}
}

// bind resolves the organisation a caller asked to act for, proving membership
// in the same step.
//
// It lives here rather than in the handler so that core/internal/server never
// imports core/internal/db: change 0.2's guard test forbids that import
// outright, because the health handlers share the package and liveness must
// not be able to reach a database.
//
// The role is read and discarded. Any member may read a report, a viewer
// included -- reading the numbers is the whole reason a viewer exists.
func (s *Service) bind(ctx context.Context, userID, orgID uuid.UUID) (db.OrgID, error) {
	org, _, err := s.db.OrgIDForSession(ctx, userID, orgID)
	if err != nil {
		if errors.Is(err, db.ErrNotAMember) {
			// The same answer whether the organisation does not exist or the
			// caller simply does not belong to it: telling them apart makes
			// this a membership oracle.
			return db.OrgID{}, codeErr(CodeForbidden, err)
		}
		return db.OrgID{}, err
	}
	return org, nil
}

// ManagementPNL reads the rows and computes the report.
//
// Everything that shapes the answer is read inside one transaction: the rows,
// the versions they were classified under, and the currency they are summed
// in. A currency read earlier, or versions taken from this binary's own
// constants, would describe a moment other than the one the figures came from.
func (s *Service) ManagementPNL(ctx context.Context, userID, orgID uuid.UUID, req Request) (Report, error) {
	from, to, err := monthRange(req.From, req.To)
	if err != nil {
		return Report{}, codeErr(CodeBadRange, err)
	}
	switch req.Basis {
	case BasisLedger, BasisBank:
	default:
		return Report{}, codeErr(CodeUnknownBasis,
			fmt.Errorf("report: %q is not a basis", req.Basis))
	}
	periods, err := Periods(req.From, req.To, req.Granularity)
	if err != nil {
		return Report{}, codeErr(CodeUnknownGranularity, err)
	}

	org, err := s.bind(ctx, userID, orgID)
	if err != nil {
		return Report{}, err
	}

	spec := Spec{
		Basis:       req.Basis,
		Periods:     periods,
		Granularity: req.Granularity,
		From:        req.From,
		To:          req.To,
	}
	var rows []Row
	var strip []Reconciliation

	err = s.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)

		baseCcy, err := q.OrganizationBaseCurrency(ctx)
		if err != nil {
			return fmt.Errorf("report: reading the reporting currency: %w", err)
		}
		if baseCcy == "" {
			return codeErr(CodeNoBaseCurrency,
				errors.New("report: the organisation names no reporting currency"))
		}
		spec.BaseCurrency = baseCcy

		args := gendb.ReportLinesParams{
			EntityID:   pgUUID(req.EntityID),
			SourceKind: string(req.Basis),
			FromDate:   pgDate(from),
			ToDate:     pgDate(to),
		}

		lines, err := q.ReportLines(ctx, args)
		if err != nil {
			return fmt.Errorf("report: reading the classified rows: %w", err)
		}
		for _, l := range lines {
			period, err := req.Granularity.Label(l.Period)
			if err != nil {
				return err
			}
			rows = append(rows, Row{
				Period:             period,
				CategoryCode:       l.CategoryCode,
				Section:            l.Section,
				Amount:             money.Money{CurrencyCode: baseCcy, MinorUnits: l.AmountMinor},
				IsPNL:              l.IsPnl,
				RequiresAllocation: l.RequiresAllocation,
				SourceKind:         req.Basis,
			})
		}

		// Unclassified: of this basis, and answered by nothing. An empty
		// category code is not a missing value here -- it is the fact that
		// puts the row in the bucket.
		unclassified, err := q.ReportUnclassifiedTotals(ctx, gendb.ReportUnclassifiedTotalsParams(args))
		if err != nil {
			return fmt.Errorf("report: reading the unclassified total: %w", err)
		}
		for _, u := range unclassified {
			period, err := req.Granularity.Label(u.Period)
			if err != nil {
				return err
			}
			rows = append(rows, Row{
				Period:     period,
				Amount:     money.Money{CurrencyCode: baseCcy, MinorUnits: u.AmountMinor},
				SourceKind: req.Basis,
			})
		}

		// The other basis. Counted, never summed into a line: the row's own
		// source kind is what Compute buckets on, so the rule lives in one
		// place and this only has to be honest about which kind each row is.
		other, err := q.ReportOtherBasisTotals(ctx, gendb.ReportOtherBasisTotalsParams(args))
		if err != nil {
			return fmt.Errorf("report: reading the other basis: %w", err)
		}
		otherKind := BasisLedger
		if req.Basis == BasisLedger {
			otherKind = BasisBank
		}
		for _, o := range other {
			period, err := req.Granularity.Label(o.Period)
			if err != nil {
				return err
			}
			rows = append(rows, Row{
				Period:     period,
				Amount:     money.Money{CurrencyCode: baseCcy, MinorUnits: o.AmountMinor},
				SourceKind: otherKind,
			})
		}

		strip, err = reconciliation(ctx, q, req, periods, baseCcy)
		if err != nil {
			return err
		}

		versions, err := q.ReportVersions(ctx, gendb.ReportVersionsParams(args))
		if err != nil {
			return fmt.Errorf("report: reading the pinned versions: %w", err)
		}
		for _, v := range versions {
			spec.TaxonomyVersions = appendDistinct(spec.TaxonomyVersions, v.TaxonomyVersion)
			spec.RulesetVersions = appendDistinct(spec.RulesetVersions, v.RulesetVersion)
			spec.EngineVersions = appendDistinct(spec.EngineVersions, v.EngineVersion)
			spec.NormalizeVersions = appendDistinct(spec.NormalizeVersions, v.NormalizeVersion)
		}
		return nil
	})
	if err != nil {
		return Report{}, err
	}

	report, err := Compute(rows, spec)
	if err != nil {
		return Report{}, err
	}
	report.Reconciliation = strip
	return report, nil
}

// appendDistinct keeps the version sets small and stable. The query already
// returns them sorted and distinct per row; across rows the same string
// recurs, and a report listing "v1, v1, v1" would read as three versions.
func appendDistinct(xs []string, x string) []string {
	for _, existing := range xs {
		if existing == x {
			return xs
		}
	}
	return append(xs, x)
}

func pgUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func pgDate(t time.Time) pgtype.Date { return pgtype.Date{Time: t, Valid: true} }
