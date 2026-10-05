package server

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	vektv1 "github.com/MyauDev/vekst/core/gen/vekst/v1"
	"github.com/MyauDev/vekst/core/internal/identity"
	"github.com/MyauDev/vekst/core/internal/tenancy"
)

// orgHandler implements vekst.v1.OrgService.
//
// CreateOrganization is the one RPC in this whole server that a caller with a
// session but no membership may call -- every other handler resolves an
// organisation from a membership the caller already holds, which is exactly
// what this one caller does not have yet. It needs the session (the auth
// option every handler shares), and deliberately nothing more: no
// db.OrgIDForSession call, no membership to resolve, because there is none
// to find.
type orgHandler struct {
	svc *tenancy.Service
}

func (h *orgHandler) CreateOrganization(
	ctx context.Context,
	req *connect.Request[vektv1.CreateOrganizationRequest],
) (*connect.Response[vektv1.CreateOrganizationResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		// Unreachable in practice -- the interceptor rejects anonymous calls
		// before they arrive -- but a handler that assumes authentication
		// without saying so is one refactor away from leaking.
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	created, err := h.svc.Create(ctx, user.ID, tenancy.New{
		Name:         req.Msg.GetName(),
		Country:      req.Msg.GetCountry(),
		BaseCurrency: req.Msg.GetBaseCurrency(),
		EntityName:   req.Msg.GetEntityName(),
	})
	if err != nil {
		return nil, tenancyErr(err)
	}

	return connect.NewResponse(&vektv1.CreateOrganizationResponse{
		OrganizationId: created.OrgID.String(),
		EntityId:       created.EntityID.String(),
	}), nil
}

// tenancyErr maps tenancy's coded failures onto Connect codes. The code is
// the message: translation is the client's.
func tenancyErr(err error) error {
	var coded *tenancy.Err
	if errors.As(err, &coded) {
		switch coded.Code {
		case tenancy.CodeAlreadyMember:
			return connect.NewError(connect.CodeAlreadyExists, errors.New(coded.Code))
		case tenancy.CodeNameRequired, tenancy.CodeEntityNameRequired,
			tenancy.CodeUnsupportedCountry, tenancy.CodeUnsupportedCurrency:
			return connect.NewError(connect.CodeInvalidArgument, errors.New(coded.Code))
		default:
			return connect.NewError(connect.CodeInvalidArgument, errors.New(coded.Code))
		}
	}
	return connect.NewError(connect.CodeInternal, err)
}
