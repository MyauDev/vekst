package server

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	vektv1 "github.com/MyauDev/vekst/core/gen/vekst/v1"
	"github.com/MyauDev/vekst/core/internal/identity"
)

// identityHandler implements vekst.v1.IdentityService.
//
// It reads the person the auth middleware already resolved -- by the time a
// Connect call reaches here, the session has been looked up once for the
// request -- and, since change 5.3, asks identity.Service for that person's
// organisations, which does touch the database: unlike the person's own
// identity, which organisation list is not settled the interceptor.
type identityHandler struct {
	svc *identity.Service
}

// GetCurrentUser returns the signed-in person and every organisation they
// belong to. An empty organisations list is the first-run signal, and a
// success, not an error: a person who has just signed in for the first time
// has not failed at anything.
func (h *identityHandler) GetCurrentUser(
	ctx context.Context,
	_ *connect.Request[vektv1.GetCurrentUserRequest],
) (*connect.Response[vektv1.GetCurrentUserResponse], error) {
	user, ok := identity.FromContext(ctx)
	if !ok {
		// Unreachable in practice -- the interceptor rejects anonymous calls
		// before they arrive -- but a handler that assumes authentication
		// without saying so is one refactor away from leaking.
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(identity.CodeUnauthenticated))
	}

	orgs, err := h.svc.Organisations(ctx, user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	out := &vektv1.GetCurrentUserResponse{
		User: &vektv1.User{
			Id:     user.ID.String(),
			Email:  user.Email,
			Name:   user.Name,
			Locale: user.Locale,
		},
		Organisations: make([]*vektv1.Organisation, 0, len(orgs)),
	}
	for _, o := range orgs {
		entities := make([]*vektv1.Entity, 0, len(o.Entities))
		for _, e := range o.Entities {
			entities = append(entities, &vektv1.Entity{Id: e.ID.String(), Name: e.Name})
		}
		out.Organisations = append(out.Organisations, &vektv1.Organisation{
			Id:           o.ID.String(),
			Name:         o.Name,
			BaseCurrency: o.BaseCurrency,
			Role:         o.Role,
			Entities:     entities,
		})
	}
	return connect.NewResponse(out), nil
}
