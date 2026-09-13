package server

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	vektv1 "github.com/MyauDev/vekst/core/gen/vekst/v1"
	typev1 "github.com/MyauDev/vekst/core/gen/vekstype/v1"
	"github.com/MyauDev/vekst/core/internal/identity"
	"github.com/MyauDev/vekst/core/internal/money"
	"github.com/MyauDev/vekst/core/internal/review"
)

// reviewHandler implements vekst.v1.ReviewService.
//
// It translates and nothing else: a request in, review.Service out, a Connect
// code back.
//
// It holds no database handle, and not merely as a matter of taste -- change
// 0.2 guard test forbids this package from importing core/internal/db at all,
// because the health handlers live here and liveness must never be able to
// reach a dependency. The service owns the tenancy binding, which is also
// where it belongs: proving membership and establishing the role is one step,
// and doing it twice is how a stale-permissions bug is built.
type reviewHandler struct {
	svc *review.Service
}

func (h *reviewHandler) ListReviewGroups(
	ctx context.Context,
	req *connect.Request[vektv1.ListReviewGroupsRequest],
) (*connect.Response[vektv1.ListReviewGroupsResponse], error) {
	user, orgID, err := caller(ctx, req.Msg.GetOrganizationId())
	if err != nil {
		return nil, err
	}
	entity, err := parseUUID(req.Msg.GetEntityId(), "entity_id")
	if err != nil {
		return nil, err
	}

	// A page size of zero is a client that did not say, not a client asking
	// for nothing. Capped, because the queue is unbounded and a browser
	// rendering ten thousand groups helps nobody.
	limit := req.Msg.GetLimit()
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	groups, totals, err := h.svc.Queue(ctx, user, orgID, entity, limit, req.Msg.GetOffset())
	if err != nil {
		return nil, connectErr(err)
	}

	out := &vektv1.ListReviewGroupsResponse{
		TotalRowCount:          int32(totals.RowCount),
		TotalCounterpartyCount: int32(totals.CounterpartyCount),
		TotalAbsolute:          toMoney(totals.Absolute),
		Groups:                 make([]*vektv1.ReviewGroup, 0, len(groups)),
	}
	for _, g := range groups {
		out.Groups = append(out.Groups, &vektv1.ReviewGroup{
			CounterpartyKey: g.CounterpartyKey,
			DisplayName:     g.DisplayName,
			RowCount:        int32(g.RowCount),
			Total:           toMoney(g.Total),
			FirstSeen:       g.FirstSeen,
			LastSeen:        g.LastSeen,
		})
	}
	return connect.NewResponse(out), nil
}

func (h *reviewHandler) ListGroupTransactions(
	ctx context.Context,
	req *connect.Request[vektv1.ListGroupTransactionsRequest],
) (*connect.Response[vektv1.ListGroupTransactionsResponse], error) {
	user, orgID, err := caller(ctx, req.Msg.GetOrganizationId())
	if err != nil {
		return nil, err
	}
	entity, err := parseUUID(req.Msg.GetEntityId(), "entity_id")
	if err != nil {
		return nil, err
	}

	rows, err := h.svc.GroupRows(ctx, user, orgID, entity, req.Msg.GetCounterpartyKey())
	if err != nil {
		return nil, connectErr(err)
	}

	out := &vektv1.ListGroupTransactionsResponse{
		Transactions: make([]*vektv1.QueuedTransaction, 0, len(rows)),
	}
	for _, t := range rows {
		out.Transactions = append(out.Transactions, &vektv1.QueuedTransaction{
			Id:              t.ID.String(),
			BookedOn:        t.BookedOn,
			Direction:       t.Direction,
			Amount:          toMoney(t.Amount),
			BaseAmount:      toMoney(t.BaseAmount),
			Description:     t.Description,
			CounterpartyRaw: t.CounterpartyRaw,
			RegulatedCode:   t.RegulatedCode,
			SourceKind:      t.SourceKind,
		})
	}
	return connect.NewResponse(out), nil
}

func (h *reviewHandler) ResolveGroup(
	ctx context.Context,
	req *connect.Request[vektv1.ResolveGroupRequest],
) (*connect.Response[vektv1.ResolveGroupResponse], error) {
	user, orgID, err := caller(ctx, req.Msg.GetOrganizationId())
	if err != nil {
		return nil, err
	}
	entity, err := parseUUID(req.Msg.GetEntityId(), "entity_id")
	if err != nil {
		return nil, err
	}
	outcome, err := fromProtoOutcome(req.Msg.GetOutcome())
	if err != nil {
		return nil, err
	}

	decision, err := h.svc.Resolve(ctx, user, orgID, entity,
		req.Msg.GetCounterpartyKey(), outcome, req.Msg.GetCategoryCode())
	if err != nil {
		return nil, connectErr(err)
	}

	return connect.NewResponse(&vektv1.ResolveGroupResponse{
		DecisionId:   decision.ID.String(),
		CoveredCount: decision.CoveredCount,
		CoveredTotal: toMoney(decision.Covered),
	}), nil
}

func (h *reviewHandler) UndoDecision(
	ctx context.Context,
	req *connect.Request[vektv1.UndoDecisionRequest],
) (*connect.Response[vektv1.UndoDecisionResponse], error) {
	user, orgID, err := caller(ctx, req.Msg.GetOrganizationId())
	if err != nil {
		return nil, err
	}
	decisionID, err := parseUUID(req.Msg.GetDecisionId(), "decision_id")
	if err != nil {
		return nil, err
	}

	retracted, err := h.svc.Undo(ctx, user, orgID, decisionID)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&vektv1.UndoDecisionResponse{
		RetractedCount: int32(retracted),
	}), nil
}

// caller is the authenticated person and the organisation they named. Proving
// they belong to it is the service's job, done in the same step that
// establishes their role.
func caller(ctx context.Context, orgID string) (uuid.UUID, uuid.UUID, error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		// Unreachable in practice -- the interceptor rejects anonymous calls
		// before they arrive -- but a handler that assumes authentication
		// without saying so is one refactor away from leaking.
		return uuid.Nil, uuid.Nil, connect.NewError(
			connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}
	org, err := parseUUID(orgID, "organization_id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return user.ID, org, nil
}

func fromProtoOutcome(o vektv1.ReviewOutcome) (review.Outcome, error) {
	switch o {
	case vektv1.ReviewOutcome_REVIEW_OUTCOME_CATEGORISED:
		return review.OutcomeCategorised, nil
	case vektv1.ReviewOutcome_REVIEW_OUTCOME_INTERNAL_TRANSFER:
		return review.OutcomeInternalTransfer, nil
	case vektv1.ReviewOutcome_REVIEW_OUTCOME_NON_PNL:
		return review.OutcomeNonPNL, nil
	case vektv1.ReviewOutcome_REVIEW_OUTCOME_SKIPPED:
		return review.OutcomeSkipped, nil
	default:
		// Proto3 cannot tell an unset enum from its zero value, so the zero
		// value is refused rather than defaulted to something destructive.
		return "", connect.NewError(connect.CodeInvalidArgument,
			errors.New(review.CodeOutcomeRequired))
	}
}

func parseUUID(s, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid_%s", field))
	}
	return id, nil
}

// toMoney leaves an absent amount absent. "No conversion happened" and "zero
// of a currency nobody named" are different claims.
func toMoney(m money.Money) *typev1.Money {
	if m.CurrencyCode == "" && m.MinorUnits == 0 {
		return nil
	}
	return &typev1.Money{CurrencyCode: m.CurrencyCode, MinorUnits: m.MinorUnits}
}

// connectErr maps the service's coded failures onto Connect codes. The code is
// the message: translation is the client's.
func connectErr(err error) error {
	var coded *review.Err
	if errors.As(err, &coded) {
		switch coded.Code {
		case review.CodeForbidden:
			return connect.NewError(connect.CodePermissionDenied, errors.New(coded.Code))
		case review.CodeEmptyGroup, review.CodeAlreadyUndone:
			return connect.NewError(connect.CodeFailedPrecondition, errors.New(coded.Code))
		default:
			return connect.NewError(connect.CodeInvalidArgument, errors.New(coded.Code))
		}
	}
	return connect.NewError(connect.CodeInternal, err)
}
