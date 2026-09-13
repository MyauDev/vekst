// Package review is the queue where a person settles what the engine could
// not, and where every decision becomes memory.
//
// The queue is not stored. A transaction needing review is one with no live
// classification, which `classifications` already says; a second
// representation of that fact would drift, and the drift is silent -- a row
// marked resolved with no classification is absent from the report and from
// the queue at once. What is stored is the human act: who decided, covering
// how many rows, worth how much.
//
// One decision covers a counterparty rather than a transaction. That is the
// whole reason twelve months of first-time data is settled in fifteen minutes
// rather than in five hundred keystrokes, and it is why `Resolve` fans one
// call out into a vendor row, a classification per covered transaction and a
// decision -- atomically, or not at all.
package review

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/money"
	"github.com/MyauDev/vekst/core/internal/normalize"
)

// Outcome is what a person decided about a counterparty.
type Outcome string

const (
	// OutcomeCategorised is the only outcome that writes vendor memory. The
	// others are properties of the movement rather than facts about the
	// counterparty, and remembering one would make L0 answer next month with
	// something that is not a category.
	OutcomeCategorised Outcome = "categorised"

	// OutcomeInternalTransfer and OutcomeNonPNL both resolve to the same
	// out-of-P&L category. The distinction is kept in the decision so that the
	// change which matches the two legs of a transfer can upgrade a claim into
	// a confirmed pair.
	OutcomeInternalTransfer Outcome = "internal_transfer"
	OutcomeNonPNL           Outcome = "non_pnl"

	// OutcomeSkipped leaves the rows in the queue. It records that somebody
	// looked, which is worth knowing when the queue is still full tomorrow.
	OutcomeSkipped Outcome = "skipped"
)

// OutOfPNLCode is the seeded leaf that a transfer or a non-P&L marking lands
// on. Both are excluded from every report line by `is_pnl`; the taxonomy has
// no separate node for transfers and does not need one, because
// `review_decisions.outcome` keeps the two apart.
const OutOfPNLCode = "09"

// Error codes. The backend returns codes, never sentences -- translation is
// the client's (CLAUDE.md, Conventions).
const (
	CodeForbidden        = "review_forbidden"
	CodeOutcomeRequired  = "review_outcome_required"
	CodeCategoryRequired = "review_category_required"
	CodeCategoryRefused  = "review_category_not_allowed"
	CodeUnknownCategory  = "review_unknown_category"
	CodeEmptyGroup       = "review_group_is_empty"
	CodeAlreadyUndone    = "review_decision_already_undone"
)

// Err is a failure with a code a client can translate.
type Err struct {
	Code string
	err  error
}

func (e *Err) Error() string { return e.Code }
func (e *Err) Unwrap() error { return e.err }

func codeErr(code string, cause error) error { return &Err{Code: code, err: cause} }

// CanResolve reports whether a role may settle or reverse a decision.
//
// The role arrives from db.OrgIDForSession, which established it while proving
// membership. It is not looked up again here: a second lookup at enforcement
// time is how a stale-permissions bug is built, and the authority to act was
// settled when the request was bound to an organisation.
//
// This check lives in the application and not in a row-level security policy.
// RLS is containment -- it guarantees a transaction bound to one organisation
// touches only its rows, and cannot know whether the caller was entitled to
// act within it. A role test inside a policy would make tenancy a
// two-mechanism problem.
func CanResolve(role db.Role) bool {
	switch role {
	case "owner", "admin", "approver":
		return true
	default:
		return false
	}
}

// Group is one counterparty's worth of unsettled transactions.
type Group struct {
	CounterpartyKey string
	DisplayName     string
	RowCount        int64

	// Signed, in the organisation's base currency.
	Total money.Money

	FirstSeen string // YYYY-MM-DD
	LastSeen  string
}

// Totals is the whole queue, not one page of it.
type Totals struct {
	RowCount          int64
	CounterpartyCount int64

	// The sum of absolute amounts, so it reads as "worth settling" rather than
	// netting an expense against an income to nearly nothing.
	Absolute money.Money
}

// QueuedTxn is one row behind a group.
type QueuedTxn struct {
	ID              uuid.UUID
	BookedOn        string
	Direction       string
	Amount          money.Money
	BaseAmount      money.Money // zero when the row needed no conversion
	Description     string
	CounterpartyRaw string
	RegulatedCode   string
	SourceKind      string
}

// Decision is what a person settled, and what it covered at the time.
type Decision struct {
	ID              uuid.UUID
	CounterpartyKey string
	KeyVersion      string
	Outcome         Outcome
	CategoryCode    string
	CoveredCount    int32
	Covered         money.Money
}

// Versions pins what a classification written here records. A report reproduces
// only if every input to it is stored beside the output, and a human decision
// is as much an input as an engine's.
type Versions struct {
	Taxonomy  string
	Ruleset   string
	Engine    string
	Normalize string
}

// HumanVersions is what a decision made by a person records. The engine and
// ruleset strings say "no engine answered this", which is the truth and is
// more useful than the version of an engine that did not run.
func HumanVersions(taxonomy, ruleset string) Versions {
	return Versions{
		Taxonomy:  taxonomy,
		Ruleset:   ruleset,
		Engine:    "human",
		Normalize: normalize.Version,
	}
}

// Service reads and empties the queue. It holds the database and nothing else:
// no clock it did not receive, no cache, no globals.
type Service struct {
	db       *db.DB
	versions Versions
}

// New builds the service.
//
// The organisation's reporting currency is not a field: it is read inside the
// transaction that sums into it, because a total and the code beside it have
// to come from one moment. A currency cached at construction is one that can
// be stale by the time it labels a number.
func New(database *db.DB, versions Versions) *Service {
	return &Service{db: database, versions: versions}
}

// bind resolves the organisation a caller asked to act for and the role they
// hold in it, in one step.
//
// It lives here rather than in the handler so that core/internal/server never
// imports core/internal/db. That is not tidiness: change 0.2's guard test
// forbids the import outright, because the health path shares the package and
// liveness must not be able to reach a database. A handler that translates and
// nothing else cannot break that rule by accident.
func (s *Service) bind(ctx context.Context, userID, orgID uuid.UUID) (db.OrgID, db.Role, error) {
	org, role, err := s.db.OrgIDForSession(ctx, userID, orgID)
	if err != nil {
		if errors.Is(err, db.ErrNotAMember) {
			// The same answer whether the organisation does not exist or the
			// caller simply does not belong to it: telling them apart makes
			// this a membership oracle.
			return db.OrgID{}, "", codeErr(CodeForbidden, err)
		}
		return db.OrgID{}, "", err
	}
	return org, role, nil
}

// Queue returns a page of the queue and the totals for the whole of it.
func (s *Service) Queue(ctx context.Context, userID, orgID, entityID uuid.UUID, limit, offset int32) ([]Group, Totals, error) {
	var groups []Group
	var totals Totals

	// Any member may read the queue, a viewer included. Only the writes below
	// ask what the role is.
	org, _, err := s.bind(ctx, userID, orgID)
	if err != nil {
		return nil, Totals{}, err
	}

	err = s.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)

		baseCcy, err := q.OrganizationBaseCurrency(ctx)
		if err != nil {
			return fmt.Errorf("review: reading the reporting currency: %w", err)
		}

		rows, err := q.ReviewGroups(ctx, gendb.ReviewGroupsParams{
			EntityID: pgUUID(entityID),
			Limit:    limit,
			Offset:   offset,
		})
		if err != nil {
			return fmt.Errorf("review: listing groups: %w", err)
		}
		for _, r := range rows {
			groups = append(groups, Group{
				CounterpartyKey: r.CounterpartyKey,
				DisplayName:     r.DisplayName,
				RowCount:        r.RowCount,
				Total:           money.Money{CurrencyCode: baseCcy, MinorUnits: r.TotalMinor},
				FirstSeen:       pgDate(r.FirstSeen),
				LastSeen:        pgDate(r.LastSeen),
			})
		}

		t, err := q.UnclassifiedTotals(ctx, pgUUID(entityID))
		if err != nil {
			return fmt.Errorf("review: totals: %w", err)
		}
		totals = Totals{
			RowCount:          t.RowCount,
			CounterpartyCount: t.CounterpartyCount,
			Absolute:          money.Money{CurrencyCode: baseCcy, MinorUnits: t.TotalMinor},
		}
		return nil
	})
	return groups, totals, err
}

// GroupRows returns the transactions behind one group.
func (s *Service) GroupRows(ctx context.Context, userID, orgID, entityID uuid.UUID, key string) ([]QueuedTxn, error) {
	org, _, err := s.bind(ctx, userID, orgID)
	if err != nil {
		return nil, err
	}

	var out []QueuedTxn
	err = s.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := gendb.New(tx).ReviewGroupRows(ctx, gendb.ReviewGroupRowsParams{
			EntityID:        pgUUID(entityID),
			CounterpartyKey: key,
		})
		if err != nil {
			return fmt.Errorf("review: listing group rows: %w", err)
		}
		for _, r := range rows {
			out = append(out, QueuedTxn{
				ID:              uuid.UUID(r.ID.Bytes),
				BookedOn:        pgDate(r.BookedOn),
				Direction:       r.Direction,
				Amount:          money.Money{CurrencyCode: r.Currency, MinorUnits: r.AmountMinor},
				BaseAmount:      optionalMoney(r.BaseCurrency, r.BaseAmountMinor),
				Description:     r.DescriptionRaw,
				CounterpartyRaw: r.CounterpartyRaw,
				RegulatedCode:   r.RegulatedCode,
				SourceKind:      r.SourceKind,
			})
		}
		return nil
	})
	return out, err
}

// Resolve settles a whole counterparty.
//
// Four writes in one transaction, in this order and for this reason: the read
// establishes the covered set, and that set is what the decision records — so
// a row imported between the read and the commit is not silently swept into a
// decision the user never saw.
func (s *Service) Resolve(
	ctx context.Context,
	userID uuid.UUID,
	orgID uuid.UUID,
	entityID uuid.UUID,
	key string,
	outcome Outcome,
	categoryCode string,
) (Decision, error) {
	org, role, err := s.bind(ctx, userID, orgID)
	if err != nil {
		return Decision{}, err
	}
	if !CanResolve(role) {
		return Decision{}, codeErr(CodeForbidden, fmt.Errorf("role %q may not resolve", role))
	}
	switch outcome {
	case OutcomeCategorised:
		if categoryCode == "" {
			return Decision{}, codeErr(CodeCategoryRequired, nil)
		}
	case OutcomeInternalTransfer, OutcomeNonPNL, OutcomeSkipped:
		if categoryCode != "" {
			return Decision{}, codeErr(CodeCategoryRefused, nil)
		}
	default:
		return Decision{}, codeErr(CodeOutcomeRequired, fmt.Errorf("outcome %q", outcome))
	}

	var decision Decision
	err = s.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)

		baseCcy, err := q.OrganizationBaseCurrency(ctx)
		if err != nil {
			return fmt.Errorf("review: reading the reporting currency: %w", err)
		}

		rows, err := q.ReviewGroupRows(ctx, gendb.ReviewGroupRowsParams{
			EntityID:        pgUUID(entityID),
			CounterpartyKey: key,
		})
		if err != nil {
			return fmt.Errorf("review: reading the group: %w", err)
		}
		if len(rows) == 0 {
			// Not an empty success. A decision that covered nothing is
			// either a double submit or a counterparty that belongs to
			// somebody else, and both deserve to be visible.
			return codeErr(CodeEmptyGroup, nil)
		}

		// Skipping records that somebody looked, and changes nothing else.
		if outcome == OutcomeSkipped {
			decision, err = writeDecision(ctx, q, baseCcy, userID, key, outcome, pgtype.UUID{}, rows)
			return err
		}

		code := categoryCode
		if outcome != OutcomeCategorised {
			code = OutOfPNLCode
		}
		category, err := q.CategoryByCode(ctx, gendb.CategoryByCodeParams{
			TaxonomyVersion: s.versions.Taxonomy,
			OrgID:           pgtype.UUID{},
			Code:            code,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return codeErr(CodeUnknownCategory, fmt.Errorf("category %q", code))
			}
			return fmt.Errorf("review: resolving category %q: %w", code, err)
		}

		// Memory is earned only by a category. The other outcomes are facts
		// about the movement, and L0 answering next month with "internal
		// transfer" for every payment to that vendor would be worse than not
		// answering.
		if outcome == OutcomeCategorised {
			if _, err := q.UpsertVendor(ctx, gendb.UpsertVendorParams{
				Key:         key,
				KeyVersion:  normalize.Version,
				DisplayName: displayName(rows),
				CategoryID:  category.ID,
				DecidedBy:   pgUUID(userID),
			}); err != nil {
				return fmt.Errorf("review: writing vendor memory: %w", err)
			}
		}

		for _, r := range rows {
			if _, err := q.InsertClassification(ctx, gendb.InsertClassificationParams{
				TransactionID:    r.ID,
				CategoryID:       category.ID,
				EngineLayer:      "human",
				Confidence:       pgNumericOne(),
				Evidence:         string(outcome),
				TaxonomyVersion:  s.versions.Taxonomy,
				RulesetVersion:   s.versions.Ruleset,
				EngineVersion:    s.versions.Engine,
				NormalizeVersion: s.versions.Normalize,
				DecidedBy:        pgUUID(userID),
			}); err != nil {
				return fmt.Errorf("review: classifying: %w", err)
			}
		}

		// The decision records the outcome; the classification records the
		// category. Only a categorisation names one here, because "this is out
		// of the P&L" is not a statement about which leaf -- the CHECK on the
		// table says the same thing and this is what satisfies it.
		decided := pgtype.UUID{}
		if outcome == OutcomeCategorised {
			decided = category.ID
		}
		decision, err = writeDecision(ctx, q, baseCcy, userID, key, outcome, decided, rows)
		if err != nil {
			return err
		}
		decision.CategoryCode = code
		return nil
	})
	return decision, err
}

func writeDecision(
	ctx context.Context,
	q *gendb.Queries,
	baseCcy string,
	userID uuid.UUID,
	key string,
	outcome Outcome,
	categoryID pgtype.UUID,
	rows []gendb.ReviewGroupRowsRow,
) (Decision, error) {
	count, total := coverage(rows)
	row, err := q.InsertReviewDecision(ctx, gendb.InsertReviewDecisionParams{
		CounterpartyKey: key,
		KeyVersion:      normalize.Version,
		Outcome:         string(outcome),
		CategoryID:      categoryID,
		DecidedBy:       pgUUID(userID),
		CoveredCount:    count,
		CoveredMinor:    total,
		CoveredCurrency: baseCcy,
	})
	if err != nil {
		return Decision{}, fmt.Errorf("review: recording the decision: %w", err)
	}
	return Decision{
		ID:              uuid.UUID(row.ID.Bytes),
		CounterpartyKey: key,
		KeyVersion:      normalize.Version,
		Outcome:         outcome,
		CoveredCount:    count,
		Covered:         money.Money{CurrencyCode: baseCcy, MinorUnits: total},
	}, nil
}

// Undo reverses a decision without erasing it.
//
// The classifications are retracted rather than superseded: supersession names
// the classification that replaced this one, and an undo has no replacement --
// the rows go back to having no answer at all. The vendor row is deleted
// rather than retracted, because `vendors` is a lookup with no supersession
// model and a stale row would keep answering L0 with a category the user has
// just taken back. Memory is current state; classifications are history.
func (s *Service) Undo(ctx context.Context, userID, orgID, decisionID uuid.UUID) (int64, error) {
	org, role, err := s.bind(ctx, userID, orgID)
	if err != nil {
		return 0, err
	}
	if !CanResolve(role) {
		return 0, codeErr(CodeForbidden, fmt.Errorf("role %q may not undo", role))
	}

	var retracted int64
	err = s.db.InTx(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)

		decision, err := q.ReviewDecisionByID(ctx, pgUUID(decisionID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// The same answer as "belongs to somebody else", which is
				// what the tenant policy already makes it.
				return db.ErrNoRowsAffected
			}
			return fmt.Errorf("review: reading the decision: %w", err)
		}

		live, err := q.LiveClassificationsForCounterparty(ctx, decision.CounterpartyKey)
		if err != nil {
			return fmt.Errorf("review: finding what it wrote: %w", err)
		}
		ids := make([]pgtype.UUID, 0, len(live))
		for _, c := range live {
			ids = append(ids, c.TransactionID)
		}

		if len(ids) > 0 {
			retracted, err = q.RetractClassificationsOfTransactions(ctx,
				gendb.RetractClassificationsOfTransactionsParams{
					RetractedBy:    pgUUID(userID),
					TransactionIds: ids,
				})
			if err != nil {
				return fmt.Errorf("review: retracting: %w", err)
			}
		}

		if decision.Outcome == string(OutcomeCategorised) {
			if _, err := q.DeleteVendor(ctx, gendb.DeleteVendorParams{
				KeyVersion: decision.KeyVersion,
				Key:        decision.CounterpartyKey,
			}); err != nil {
				return fmt.Errorf("review: forgetting the vendor: %w", err)
			}
		}

		if _, err := q.UndoReviewDecision(ctx, gendb.UndoReviewDecisionParams{
			ID:       pgUUID(decisionID),
			UndoneBy: pgUUID(userID),
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return codeErr(CodeAlreadyUndone, nil)
			}
			return fmt.Errorf("review: stamping the decision: %w", err)
		}
		return nil
	})
	return retracted, err
}

// coverage is what the decision covered: how many rows, and what they were
// worth in the organisation's base currency.
func coverage(rows []gendb.ReviewGroupRowsRow) (int32, int64) {
	var total int64
	for _, r := range rows {
		// A row with no conversion is already in the base currency, so this
		// is exact rather than an approximation.
		if r.BaseAmountMinor.Valid {
			total += r.BaseAmountMinor.Int64
			continue
		}
		total += r.AmountMinor
	}
	return int32(len(rows)), total
}

// displayName is what the statement called the counterparty, for a person to
// read later. The last spelling wins: rows come back in date order, so it is
// the most recent one the bank used.
func displayName(rows []gendb.ReviewGroupRowsRow) string {
	name := ""
	for _, r := range rows {
		if r.CounterpartyRaw != "" {
			name = r.CounterpartyRaw
		}
	}
	return name
}

func optionalMoney(currency pgtype.Text, minor pgtype.Int8) money.Money {
	if !currency.Valid || !minor.Valid {
		return money.Money{}
	}
	return money.Money{CurrencyCode: currency.String, MinorUnits: minor.Int64}
}

func pgUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func pgDate(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

// A human decision is certain by construction: a person looked at the rows and
// said what they were.
func pgNumericOne() pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan("1.000")
	return n
}
