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
// It reads the person the auth middleware already resolved and never touches
// the database itself: by the time a Connect call reaches here, the session
// has been looked up once for the request.
type identityHandler struct{}

// GetCurrentUser returns the signed-in person.
//
// The response carries no organisation and no role, deliberately. There is no
// organisation model until change 1.1, and a field that exists before it means
// anything is a field the front end starts trusting.
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

	return connect.NewResponse(&vektv1.GetCurrentUserResponse{
		User: &vektv1.User{
			Id:     user.ID.String(),
			Email:  user.Email,
			Name:   user.Name,
			Locale: user.Locale,
		},
	}), nil
}
